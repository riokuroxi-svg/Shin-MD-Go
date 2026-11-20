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
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"shin-go/internal/consola"

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

// PreguntarNumero pregunta el número y no se rinde al primer intento: si lo
// tecleado no tiene dígitos usables lo dice y vuelve a preguntar (hasta 3
// veces). Antes, un dedazo terminaba en «ese número no tiene dígitos
// usables» y a empezar de cero.
//
// Devuelve el número ya normalizado, o "" si no se pudo leer o se agotaron
// los intentos. Lee de stdin si es terminal y de /dev/tty si no (el caso
// «curl | sh», donde hay terminal detrás aunque la entrada esté redirigida).
func PreguntarNumero(salida io.Writer) string {
	const pregunta = "Tu número (ej. +52 1 55 1234 5678):"
	for intento := 1; intento <= 3; intento++ {
		resp, err := consola.LeerTexto(salida, pregunta, "")
		if err != nil {
			return ""
		}
		num := NormalizarTelefono(resp)
		if num != "" {
			return num
		}
		// Se dice qué se esperaba, no solo que está mal.
		fmt.Fprintln(salida, "  "+consola.Ambar("✕ no veo un número ahí (país + número, con o sin +)"))
	}
	return ""
}

// ─── dibujo ──────────────────────────────────────────────────────────────
// Los paneles los dibuja internal/consola (pterm): caja con título, borde
// sakura y texto envuelto al ancho real de la terminal — en un móvil en
// vertical la caja nunca desborda la pantalla.

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

// PanelVinculacion: el aviso de lo que va a pasar, ANTES de pedir nada. caja de
// 58 columnas: en Termux en vertical entra entero y no se parte por la mitad.
func PanelVinculacion(w io.Writer) {
	ancho := consola.AnchoFijo()
	filas := consola.Marco(consola.SakuraFuerte("vinculación"), []string{
		"",
		consola.Papel("Vamos a vincular tu WhatsApp con el bot."),
		consola.Tenue("No hay que escanear nada: WhatsApp te da un"),
		consola.Tenue("código de 8 letras y lo escribes en el teléfono."),
		"",
		consola.Tenue("Ajustes › Dispositivos vinculados ›"),
		consola.Tenue("Vincular un dispositivo › Vincular con número"),
		"",
	}, ancho)
	fmt.Fprintln(w)
	for _, l := range filas {
		fmt.Fprintln(w, l)
	}
	fmt.Fprintln(w)
}

// PanelCodigo: el momento importante de todo el arranque, y el único que de
// verdad hay que leer. Es salida ESTÁTICA de terminal: nada de Marco, clear,
// búfer alternativo ni refrescos. El panel de estado no existe todavía y este
// texto se queda visible hasta que WhatsApp confirme events.Connected.
//
// El código lleva espacios entre letras —así se distingue la R de la 2— en
// vez de arte de bloques: ese ocupaba seis filas, se veía fatal con la fuente
// de Termux y no se podía seleccionar con el dedo para copiarlo.
func PanelCodigo(w io.Writer, numero, codigo string) {
	codigo = FormatearCodigo(codigo)
	// Se reservan dos columnas de sangría y hasta tres para «1. »: envolver
	// antes de añadir la marca evita que una instrucción caiga en la columna
	// final de Termux. El código espaciado mide 17, así que 18 es el mínimo.
	anchoTexto := consola.Ancho() - 6
	if anchoTexto < 18 {
		anchoTexto = 18
	}
	if anchoTexto > 52 {
		anchoTexto = 52
	}

	fmt.Fprintln(w)
	lineaEnvolviendo(w, "", consola.Menta, "TU CÓDIGO DE VINCULACIÓN", anchoTexto)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  "+consola.SakuraFuerte(consola.Espaciar(codigo)))
	fmt.Fprintln(w)
	lineaEnvolviendo(w, "", consola.Tenue, "Se quedará en pantalla hasta que WhatsApp confirme.", anchoTexto)
	lineaEnvolviendo(w, "", consola.Tenue, "Si caduca, reinicia para pedir otro.", anchoTexto)
	fmt.Fprintln(w)

	pasos := []string{
		"abre WhatsApp en ese teléfono",
		"Ajustes › Dispositivos vinculados",
		"Vincular un dispositivo › con número",
		"teclea el código de arriba",
	}
	lineaEnvolviendo(w, "", consola.Tenue, "cuenta: "+consola.SinColor(consola.Papel(numero)), anchoTexto)
	for n, paso := range pasos {
		lineaEnvolviendo(w, consola.Paso(n+1, ""), func(x string) string { return x }, paso, anchoTexto)
	}
	fmt.Fprintln(w)
}

// lineaEnvolviendo: una frase, escrita en tantas líneas como hagan falta.
//
// `marca` (el «1.» del paso) va sólo en la primera; `pintar` se aplica a cada
// línea YA envuelta: colorear antes de medir metería los escapes en medio del
// cálculo del ancho y salían líneas de más. Es el motivo por el que en Termux
// un «3. Vincular un dispositivo › con número» se partía en la «o» final.
func lineaEnvolviendo(w io.Writer, marca string, pintar func(string) string, s string, ancho int) {
	for i, l := range consola.Envolver(s, ancho) {
		if i == 0 {
			fmt.Fprintln(w, "  "+marca+pintar(l))
			continue
		}
		fmt.Fprintln(w, "    "+pintar(l))
	}
}

// PanelQR: la otra forma de vincular, para quien prefiera escanear (un VPS
// con navegador de terminal, por ejemplo). En el móvil donde corre el bot no
// sirve — no hay cámara que lo escanee— y por eso el menú lo avisa.
//
// `png` (opcional) es donde se guarda el QR como imagen CUANDO EL TERMINAL ES
// ESTRECHO. Un ref de WhatsApp necesita ~81 columnas; en Termux en vertical
// (50) dibujarlo lo partiría en dos líneas y ningún lector lo escanearía. Con
// la imagen en disco el usuario la abre (`termux-open data/vincular-qr.png`) o
// se la lleva al ordenador, y el bot no se queda sin camino.
func PanelQR(w io.Writer, codigo, png string) {
	if consola.DibujarQR(w, codigo, "Escanea con WhatsApp › Dispositivos vinculados › Vincular un dispositivo") {
		return
	}
	necesitas := consola.AnchoQR(codigo)
	// Una línea por frase, ENVUELTA al ancho real: en Termux un Fprintln largo
	// se parte por la mitad y se lee fatal.
	ancho := consola.Ancho() - 6
	if ancho < 30 {
		ancho = 30
	}
	sin := func(s string) string { return s }
	fmt.Fprintln(w)
	lineaEnvolviendo(w, "", consola.Ambar, fmt.Sprintf("el QR necesita %d columnas y tu terminal tiene %d: dibujarlo aquí lo partiría y no escanearía.", necesitas, consola.Ancho()), ancho)
	lineaEnvolviendo(w, "", consola.Tenue, "tres salidas, la que te venga bien:", ancho)
	lineaEnvolviendo(w, consola.Paso(1, ""), sin, fmt.Sprintf("ensancha la ventana (o gira el teléfono) a %d columnas y vuelve a arrancar", necesitas+2), ancho)
	if png != "" {
		switch err := consola.GuardarQRPNG(png, codigo); {
		case err != nil:
			lineaEnvolviendo(w, consola.Paso(2, ""), sin, "guardar el QR en imagen falló: "+err.Error(), ancho)
		case esTermux():
			lineaEnvolviendo(w, consola.Paso(2, ""), sin, "ábrelo con la galería:   "+consola.Sakura("termux-open "+png), ancho)
		default:
			lineaEnvolviendo(w, consola.Paso(2, ""), sin, "cópialo y ábrelo en el ordenador: "+consola.Sakura(png), ancho)
		}
	}
	lineaEnvolviendo(w, consola.Paso(3, ""), sin, "o elige 1) Código de 8 letras, que no necesita escanear nada", ancho)
	fmt.Fprintln(w)
}

// esTermux: mismo criterio que en cmd/shin (se vuelve a comprobar aquí para no
// depender de que quien llama lo sepa).
func esTermux() bool {
	return strings.Contains(os.Getenv("PREFIX"), "/com.termux/") || os.Getenv("TERMUX_APP_PACKAGE") != ""
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
