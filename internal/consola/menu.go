// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// menu.go — la parte «de usar» de la consola: elegir con flechas, escribir el
// número sin pelearse con el formato y contar lo que va pasando.
//
// Regla de oro que comparte con el resto de internal/consola: si no hay una
// terminal de verdad (servicio, `curl | sh`, NO_COLOR), TODO se degrada a
// texto plano con números — nunca se queda esperando una tecla que nadie va
// a pulsar.
//
// El teclado se lee de /dev/tty cuando stdout es una terminal pero stdin no
// (el caso `curl | sh`): así el menú funciona igual en Termux, en un VPS por
// ssh y en un `bash <(curl …)`.
package consola

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// ErrCancelado: el usuario pulsó Esc, q o Ctrl+C. Quien llama decide si eso
// es «salir» o «volver al menú»; aquí solo se deja constancia.
var ErrCancelado = errors.New("cancelado por el usuario")

// Opcion es una entrada del menú. La tecla es lo que se teclea en el modo
// sin terminal (1, 2, 3…) y también atajo directo en el modo interactivo.
type Opcion struct {
	Tecla   string
	Titulo  string
	Detalle string // línea gris debajo del título; puede ir vacía
}

// Elegir muestra un menú y devuelve la Tecla de la opción escogida.
//
//	· Con terminal: flechas ↑↓ (o j/k), Enter para aceptar, 1…9 como atajo,
//	  Esc/q para cancelar.
//	· Sin terminal: se imprime la lista numerada y se lee una línea; Enter
//	  vacío acepta la primera opción.
func Elegir(w io.Writer, pregunta string, opciones []Opcion) (string, error) {
	if len(opciones) == 0 {
		return "", errors.New("menú sin opciones")
	}
	// En una terminal de verdad se dibuja y se navega; sin ella, lista y línea.
	if tty := abrirTTY(); tty != nil {
		defer func() { _ = tty.Close() }()
		if term.IsTerminal(int(tty.Fd())) {
			return elegirConTeclas(w, tty, pregunta, opciones)
		}
	}
	return elegirConLinea(w, pregunta, opciones)
}

// ─── modo interactivo ────────────────────────────────────────────────────

// elegirConTeclas dibuja el menú y navega con las flechas.
//
// El terminal se pone en modo CRUDO mientras dura el menú (las flechas solo
// llegan así, y sin eco de lo que se teclea) y se devuelve a su estado
// normal ANTES de salir, con un defer: si el usuario pulsa Ctrl+C, el
// terminal tiene que quedar usable igual.
//
// En modo crudo los saltos de línea se escriben «\r\n» (ver renderMenu).
func elegirConTeclas(w io.Writer, tty *os.File, pregunta string, opciones []Opcion) (string, error) {
	estado, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		return elegirConLinea(w, pregunta, opciones)
	}
	defer func() { _ = term.Restore(int(tty.Fd()), estado) }()

	if pregunta != "" {
		fmt.Fprint(w, "\r\n  "+stSakuraFuerte.Sprint(pregunta)+"\r\n\r\n")
	}
	fmt.Fprint(w, renderMenu(opciones, 0))

	idx := 0
	for {
		tecla, err := leerTecla(tty)
		if err != nil {
			return "", ErrCancelado
		}
		switch tecla {
		case "arriba", "k":
			idx = (idx - 1 + len(opciones)) % len(opciones)
		case "abajo", "j", "tab":
			idx = (idx + 1) % len(opciones)
		case "enter":
			cerrarMenu(w, opciones, títuloDe(opciones, opciones[idx].Tecla))
			return opciones[idx].Tecla, nil
		case "esc", "q", "ctrl-c":
			cerrarMenu(w, opciones, "cancelado")
			return "", ErrCancelado
		case "":
			continue // tecla de control sin significado: se ignora
		default:
			// Atajo: el número (o la tecla) elige esa opción y acepta de una.
			for _, o := range opciones {
				if o.Tecla == tecla {
					cerrarMenu(w, opciones, títuloDe(opciones, o.Tecla))
					return o.Tecla, nil
				}
			}
			continue
		}
		redibujarMenu(w, opciones, idx)
	}
}

// cerrarMenu quita el menú de pantalla y deja en su lugar UNA línea con lo
// que se eligió. Sin esto, al aceptar con el atajo numérico quedaba el bloque
// anterior pintado encima de la respuesta (dos menús a la vez en un móvil de
// 50 columnas, que es justo lo que se veía).
func cerrarMenu(w io.Writer, opciones []Opcion, titulo string) {
	if coloresOn {
		fmt.Fprintf(w, "\033[%dA\r\033[J", lineasMenu(opciones))
	}
	if titulo != "" {
		fmt.Fprint(w, "  "+stSakura.Sprint("❯ ")+stPapel.Sprint(titulo)+"\r\n")
	}
}

// títuloDe: el texto de la opción elegida (para cerrarMenu).
func títuloDe(opciones []Opcion, tecla string) string {
	for _, o := range opciones {
		if o.Tecla == tecla {
			return o.Titulo
		}
	}
	return tecla
}

func idxDelTecla(opciones []Opcion, tecla string) int {
	for i, o := range opciones {
		if o.Tecla == tecla {
			return i
		}
	}
	return 0
}

// lineasMenu: cuántas líneas ocupa el menú dibujado (para subir el cursor al
// repintar). Se CUENTAN las líneas del propio render en vez de asumirlas: así
// no se queda nunca un resto del dibujo anterior si cambia el pie del menú.
func lineasMenu(opciones []Opcion) int {
	return strings.Count(renderMenu(opciones, 0), "\r\n")
}

// renderMenu dibuja la lista con el selector ❯ en la opción activa.
//
// Las líneas se ENVUELTEN aquí, no las parte el terminal: el alto del menú se
// cuenta sobre este mismo render (lineasMenu), así que lo que se envuelve se
// sabe y se limpia al repintar; lo que parte el terminal, no. Y un detalle
// cortado con «…» pierde justo la pista de para qué sirve la opción.
func renderMenu(opciones []Opcion, sel int) string {
	ancho := Ancho() - 1
	if ancho < 20 {
		ancho = 20
	}
	var b strings.Builder
	// \r\n, no \n: esto se imprime en MODO CRUDO (raw), donde el terminal no
	// convierte el salto en carro+salto y cada línea seguiría en la columna
	// anterior (el menú salía en diagonal). En modo normal \r\n es inocuo.
	linea := func(s string) {
		for _, t := range partir(s, ancho) {
			if coloresOn {
				t += "\x1b[0m" // cerrar: el estilo de un detalle no puede manchar la línea de abajo
			}
			b.WriteString(t + "\r\n")
		}
	}
	for i, o := range opciones {
		marca := "  "
		titulo := stPapel.Sprint(o.Titulo)
		if i == sel {
			marca = stSakura.Sprint("❯ ")
			titulo = stSakuraFuerte.Sprint(o.Titulo)
		}
		linea("  " + marca + stTenue.Sprint(o.Tecla+") ") + titulo)
		if o.Detalle != "" {
			linea("      " + stTenue.Sprint(o.Detalle))
		}
	}
	b.WriteString("\r\n")
	ayuda := "↑↓ mover · Enter aceptar · 1-9 atajo · q salir"
	if ancho < 56 {
		ayuda = "↑↓ mover · Enter ok · 1-9 · q salir"
	}
	linea("  " + stTenue.Sprint(ayuda))
	return b.String()
}

// redibujarMenu sube el cursor las líneas del menú y lo repinta. Sin ANSI no
// se puede repintar: entonces el menú se queda como está y se acepta con
// Enter (nunca se cuelga esperando algo imposible).
func redibujarMenu(w io.Writer, opciones []Opcion, sel int) bool {
	if !coloresOn {
		return false
	}
	// El menú ocupa: una línea por opción (+ su detalle), una en blanco y la
	// ayuda. Se sube ese número de líneas, se borra cada una EN SU SITIO
	// (nada de \n al final: bajaría el cursor y haría scroll) y se repinta.
	// Se sube lo que ocupa el menú y se borra TODO lo que queda por debajo con
	// \033[J: repintar así no deja restos si el terminal envolvió alguna línea.
	n := lineasMenu(opciones)
	fmt.Fprintf(w, "\033[%dA\r\033[J", n)
	fmt.Fprint(w, renderMenu(opciones, sel))
	return true
}

// leerTecla devuelve un nombre simbólico de la tecla pulsada
// ("arriba", "abajo", "enter", "esc", "ctrl-c") o el carácter tal cual.
func leerTecla(tty *os.File) (string, error) {
	var b [1]byte
	if _, err := tty.Read(b[:]); err != nil {
		return "", err
	}
	switch b[0] {
	case 0x03: // Ctrl+C
		return "ctrl-c", nil
	case '\r', '\n':
		return "enter", nil
	case '\t':
		return "tab", nil
	case 0x1b: // posible secuencia de flecha: ESC [ A
		var sec [2]byte
		_ = tty.SetReadDeadline(time.Now().Add(60 * time.Millisecond))
		n, err := tty.Read(sec[:])
		_ = tty.SetReadDeadline(time.Time{})
		if err != nil || n < 2 {
			return "esc", nil
		}
		switch string(sec[:n]) {
		case "[A", "OA":
			return "arriba", nil
		case "[B", "OB":
			return "abajo", nil
		}
		return "esc", nil
	}
	if b[0] < 0x20 { // cualquier otra tecla de control: se ignora
		return "", nil
	}
	return string(b[:]), nil
}

// ─── modo línea (servicios, tuberías, CI) ─────────────────────────────────

func elegirConLinea(w io.Writer, pregunta string, opciones []Opcion) (string, error) {
	if pregunta != "" {
		fmt.Fprintln(w, "  "+pregunta)
	}
	for _, o := range opciones {
		fmt.Fprintf(w, "   %s) %s\n", o.Tecla, o.Titulo)
		if o.Detalle != "" {
			fmt.Fprintf(w, "      %s\n", o.Detalle)
		}
	}
	fmt.Fprint(w, "  ❯ opción ["+opciones[0].Tecla+"]: ")
	linea, _ := leerLinea()
	linea = strings.TrimSpace(linea)
	if linea == "" {
		return opciones[0].Tecla, nil
	}
	for _, o := range opciones {
		if strings.EqualFold(o.Tecla, linea) || strings.HasPrefix(strings.ToLower(o.Titulo), strings.ToLower(linea)) {
			return o.Tecla, nil
		}
	}
	return "", fmt.Errorf("no entendí %q; escribe una de las teclas: %s", linea, teclas(opciones))
}

func teclas(opciones []Opcion) string {
	var t []string
	for _, o := range opciones {
		t = append(t, o.Tecla)
	}
	return strings.Join(t, " ")
}

// ─── leer una línea y preguntar sí/no ────────────────────────────────────

// LeerTexto pregunta algo y devuelve lo tecleado (sin espacios en los
// extremos). Lee de stdin si es terminal; si no, de /dev/tty (hay terminal
// detrás aunque la entrada venga de un `curl | sh`).
func LeerTexto(w io.Writer, pregunta, porDefecto string) (string, error) {
	fmt.Fprint(w, "  ❯ "+pregunta+" ")
	resp, err := leerLinea()
	if err != nil {
		return "", err
	}
	resp = strings.TrimSpace(resp)
	if resp == "" {
		return porDefecto, nil
	}
	return resp, nil
}

// Confirmar pregunta sí/no. Enter acepta el valor por defecto.
func Confirmar(w io.Writer, pregunta string, porDefecto bool) bool {
	pista := "s/N"
	if porDefecto {
		pista = "S/n"
	}
	resp, err := LeerTexto(w, pregunta+" ["+pista+"]", "")
	if err != nil || resp == "" {
		return porDefecto
	}
	return strings.HasPrefix(strings.ToLower(resp), "s") || strings.HasPrefix(strings.ToLower(resp), "y")
}

var (
	lectoresMu sync.Mutex
	lectores   = map[*os.File]*bufio.Reader{}
)

// readerDe devuelve el lector con búfer de ESTE archivo de terminal.
//
// Uno solo por archivo, a propósito: crear un bufio.Reader nuevo por pregunta
// se come las líneas que ya estaban en el búfer (pegar el número y el código
// de golpe perdía el segundo). Y por archivo, no global: si no, un menú que
// leyó de /dev/tty y una pregunta que lee de stdin usarían el descriptor
// equivocado.
func readerDe(f *os.File) *bufio.Reader {
	lectoresMu.Lock()
	defer lectoresMu.Unlock()
	if r, ok := lectores[f]; ok {
		return r
	}
	r := bufio.NewReader(f)
	lectores[f] = r
	return r
}

// olvidarLector quita el lector de un fd que se va a cerrar (si era propio).
func olvidarLector(f *os.File) {
	lectoresMu.Lock()
	delete(lectores, f)
	lectoresMu.Unlock()
}

// leerLinea lee una línea de donde de verdad se puede escribir.
func leerLinea() (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return readerDe(os.Stdin).ReadString('\n')
	}
	tty := abrirTTY()
	if tty == nil {
		return "", errors.New("no hay terminal donde preguntar")
	}
	defer func() { _ = tty.Close() }()
	return readerDe(tty).ReadString('\n')
}

// abrirTTY devuelve la terminal del proceso, o nil si no hay ninguna.
func abrirTTY() *os.File {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return os.Stdin
	}
	if runtimeSinTTY() {
		return nil
	}
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	return f
}

// ─── avisos de un vistazo ────────────────────────────────────────────────

// Paso numerado con el número en sakura: la lista de «qué hacer ahora».
func Paso(n int, texto string) string {
	return stSakura.Sprintf("%d.", n) + " " + stPapel.Sprint(texto)
}

// Bien / Mal / Ojo: una línea de resultado con su símbolo, del mismo tamaño
// que el resto del arranque (sin cajas para lo pequeño).
func Bien(texto string) { Frase(stMenta.Sprint("✓ " + texto)) }
func Mal(texto string)  { Frase(stRojo.Sprint("✕ " + texto)) }
func Ojo(texto string)  { Frase(stAmbar.Sprint("! " + texto)) }

// Barra expone la barra ▓▓▓░░ de los vitales para quien quiera enseñar un
// progreso (el calentamiento, el cupo del día…).
