// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// vinculacion.go — a quién se vincula el bot, y quién pregunta.
// Port 1:1 de src/lib/vinculacion.js del JS, con su razón de ser:
//
//	LA REGLA
//	  · Quien tiene la terminal pregunta. Si hay terminal, se pregunta el
//	    número; si no, se para antes de arrancar y se dice la orden exacta.
//	  · Un número puesto a mano (--code 52…, PAIRING_NUMBER) nunca se
//	    pregunta dos veces.
//	  · Si ya hay una sesión buena, no se pregunta nada.
//	  · El QR no es una salida de emergencia: no existe. En el móvil donde
//	    corre el bot no hay cámara que lo escanee.
//
// La decisión vive en PlanDeVinculacion, pura y testeable sin arrancar
// el bot ni abrir una terminal de verdad.
package wa

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"golang.org/x/term"
)

// Accion: qué hay que hacer con el número de vinculación.
type Accion int

const (
	AccionNada      Accion = iota // sesión registrada: no vincular nada
	AccionUsar                    // número escrito a mano: usarlo y ya
	AccionPreguntar               // hay terminal: preguntar el número
	AccionSalir                   // sin terminal ni número: explicar y parar
)

// Plan de vinculación, sin efectos: el resultado de decidir.
type Plan struct {
	Accion Accion
	Numero string // solo en AccionUsar
	Motivo string // por qué no hay sesión buena, o «sesión registrada»
}

// NormalizarTelefono: "+52 1 55 1234 5678", "0155…" y "52 55 …" tienen
// que acabar siendo el mismo dígito tras dígito que WA entiende. Mismas
// reglas que el JS:
//
//	· se quedan solo los dígitos; sin dígitos, cadena vacía;
//	· ceros delante, fuera;
//	· México: 52… (12+ dígitos) sin el 1 móvil → 521…;
//	· Argentina: 54… (11+ dígitos) sin el 9 móvil → 549….
func NormalizarTelefono(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	num := strings.TrimLeft(b.String(), "0")
	if strings.HasPrefix(num, "52") && !strings.HasPrefix(num, "521") && len(num) >= 12 {
		num = "521" + num[2:]
	}
	if strings.HasPrefix(num, "54") && !strings.HasPrefix(num, "549") && len(num) >= 11 {
		num = "549" + num[2:]
	}
	return num
}

// PlanDeVinculacion: la decisión, sin tocar nada.
//
//	sesionRegistrada  ¿hay una sesión que WhatsApp ya dio por buena?
//	forzada           --code pasado aunque sea vacío (re-vincular a propósito)
//	numero            --code <num> o PAIRING_NUMBER, ya normalizado
//	hayTerminal       ¿se puede preguntar (stdin TTY o /dev/tty detrás)?
func PlanDeVinculacion(sesionRegistrada, forzada bool, numero string, hayTerminal bool, motivo string) Plan {
	if sesionRegistrada && !forzada {
		return Plan{Accion: AccionNada, Motivo: "sesión registrada"}
	}
	// Un número escrito a mano manda siempre: ni se pregunta ni se adivina.
	if numero != "" {
		return Plan{Accion: AccionUsar, Numero: numero, Motivo: "número indicado"}
	}
	if hayTerminal {
		return Plan{Accion: AccionPreguntar, Motivo: motivo}
	}
	return Plan{Accion: AccionSalir, Motivo: motivo}
}

// HayTerminal: ¿se puede preguntar? El caso que importa: el proceso nació
// de un «curl | sh» o un servicio —stdin no es TTY— pero HAY una terminal
// de verdad detrás (/dev/tty). Callarse ahí sería dejar al usuario con un
// bot que no arranca sin decir por qué.
func HayTerminal() bool {
	if esTTY(os.Stdin) {
		return true
	}
	if runtime.GOOS == "windows" {
		return false
	}
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func esTTY(f *os.File) bool {
	// IsTerminal, no ModeCharDevice: /dev/null TAMBIÉN es modo-carácter y
	// con él el bot «veía terminal» dentro de un `curl | sh` o un servicio.
	return term.IsTerminal(int(f.Fd()))
}

// PreguntarNumero dibuja la petición y lee la respuesta de donde toca:
// si stdin es terminal, de stdin; si no, de /dev/tty (terminal que hay
// detrás aunque la entrada esté redirigida —el caso «curl | sh»—).
// Devuelve el número ya normalizado, o "" si no se pudo leer o no tiene
// dígitos usables.
func PreguntarNumero(salida io.Writer) string {
	const pregunta = "  ❯ Número de WhatsApp (ej. +52 1 55 1234 5678): "
	if esTTY(os.Stdin) {
		fmt.Fprint(salida, pregunta)
		linea, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		return NormalizarTelefono(strings.TrimSpace(linea))
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return ""
	}
	defer func() { _ = tty.Close() }()
	fmt.Fprint(tty, pregunta)
	linea, _ := bufio.NewReader(tty).ReadString('\n')
	return NormalizarTelefono(strings.TrimSpace(linea))
}

// ─── dibujo ──────────────────────────────────────────────────────────────

const ancho = 58 // columnas totales del panel, como en el JS

// caja: ╭ título ─…╮ / │ línea │ / ╰─…╯. Las líneas ya vienen con su texto;
// se paddean al interior. Sin colores: esto se lee también en logs.
func caja(w io.Writer, titulo string, lineas []string) {
	relleno := ancho - 4 - len([]rune(titulo)) // "╭ " + título + " " + dashes + "╮"
	if relleno < 0 {
		relleno = 0
	}
	fmt.Fprintf(w, "╭ %s %s╮\n", titulo, strings.Repeat("─", relleno))
	for _, l := range lineas {
		n := len([]rune(l))
		if n > ancho-4 {
			n = ancho - 4 // una línea larga jamás rompe el borde; se recorta el relleno
		}
		fmt.Fprintf(w, "│ %s%s │\n", l, strings.Repeat(" ", ancho-4-n))
	}
	fmt.Fprintln(w, "╰"+strings.Repeat("─", ancho-2)+"╯")
}

// centrar dentro del interior (ancho-4 columnas útiles entre «│ » y « │»).
func centrar(s string) string {
	n := len([]rune(s))
	if n >= ancho-4 {
		return s
	}
	izq := (ancho - 4 - n) / 2
	return strings.Repeat(" ", izq) + s
}

// Espaciar: mete un espacio fino entre los caracteres del código para que
// se lea sin confundir 8 con B. No cambia lo que hay que teclear.
func Espaciar(codigo string) string {
	return strings.Join(strings.Split(codigo, ""), " ")
}

// FormatearCodigo: whatsmeow devuelve a veces «ABCD1234» y a veces ya con
// guion; el panel siempre enseña la forma «ABCD-1234» que WhatsApp espera.
func FormatearCodigo(codigo string) string {
	c := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(codigo))
	if len(c) == 8 {
		return c[:4] + "-" + c[4:]
	}
	return c
}

// PanelVinculacion: el aviso de que va a preguntar el número.
func PanelVinculacion(w io.Writer) {
	caja(w, "vinculación", []string{
		"",
		"Vamos a vincular tu WhatsApp con el bot.",
		"No hay que escanear nada: WhatsApp te da un",
		"código de 8 letras y lo escribes en el teléfono.",
		"",
		"Ajustes › Dispositivos vinculados ›",
		"Vincular un dispositivo › Vincular con número",
		"",
	})
}

// PanelCodigo: el momento importante de todo el arranque. Se dibuja grande,
// aparte del resto, para que nadie tenga que buscarlo entre líneas de log.
func PanelCodigo(w io.Writer, numero, codigo string) {
	codigo = FormatearCodigo(codigo)
	fmt.Fprintln(w)
	caja(w, "tu código", []string{
		"",
		centrar("escríbelo en el teléfono"),
		"",
		centrar(Espaciar(codigo)),
		"",
		centrar("caduca en ~60 s · si expira, vuelve a arrancar"),
		"",
	})
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  cuenta: %s\n", numero)
	fmt.Fprintln(w, "  1. abre WhatsApp en ese teléfono")
	fmt.Fprintln(w, "  2. Ajustes › Dispositivos vinculados")
	fmt.Fprintln(w, "  3. Vincular un dispositivo › con número")
	fmt.Fprintln(w, "  4. teclea el código de arriba")
	fmt.Fprintln(w)
}

// ExplicarSinTerminal: cuando no hay dónde preguntar (servicio, tubo, ssh
// sin tty). No se arranca a ciegas: se para y se dice la orden exacta, con
// «521XXXXXXXXXX» porque es lo único que hay que cambiar.
func ExplicarSinTerminal(w io.Writer, motivo string) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  ✕ no hay una terminal donde preguntarte el número (%s).\n", motivo)
	fmt.Fprintln(w, "    Se arranca sin preguntar de dos maneras:")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "      shin --code 521XXXXXXXXXX")
	fmt.Fprintln(w, "      PAIRING_NUMBER=521XXXXXXXXXX shin")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "    El número es el tuyo, el de la cuenta de WhatsApp que va a llevar el bot.")
	fmt.Fprintln(w)
}
