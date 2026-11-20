// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// consola.go — la cara del bot en la terminal, construida sobre pterm:
// banner grande con degradado sakura, cajas con título, barras de vitales,
// spinner y semáforos de prefijo. Todo se degrada solo a texto plano cuando
// no hay terminal de verdad o NO_COLOR=1 (pterm respeta DisableColor).
//
// Sabor de referencia: boot/index.js + scripts/consola.mjs del JS, y el
// idioma visual de las herramientas modernas de CLI (lazygit, pterm…).
package consola

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/pterm/pterm"
	"golang.org/x/term"
)

// ─── la paleta de la casa ────────────────────────────────────────────────
//
// Mismos HEX que el instalador, el perfil y el panel web, para que la terminal
// y GitHub hablen el mismo idioma. Están aquí en vez de en los colores
// aproximados de pterm porque «FgLightMagenta» no es el sakura de Shin.
type estilo struct {
	fg      string // secuencia ANSI de foreground, vacía = sin color
	negrita bool
}

// color: un color de la casa. Se escribe a mano el 24 bits (#RRGGBB) porque
// pterm no expone Hex en esta versión y porque así el único criterio para
// pintar es «¿hay terminal?», no el estado global de un paquete externo.
func tinte(hex string) estilo {
	r, g, b := parte(hex, 1), parte(hex, 3), parte(hex, 5)
	return estilo{fg: fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)}
}

func parte(hex string, i int) int {
	dig := func(c byte) int {
		switch {
		case c >= '0' && c <= '9':
			return int(c - '0')
		case c >= 'a' && c <= 'f':
			return int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			return int(c-'A') + 10
		}
		return 0
	}
	if i+1 >= len(hex) {
		return 0
	}
	return dig(hex[i])*16 + dig(hex[i+1])
}

func (e estilo) conNegrita() estilo { e.negrita = true; return e }

// Sprint: el texto con el color puesto, o tal cual si no hay terminal.
func (e estilo) Sprint(txt string) string {
	if !coloresOn || e.fg == "" {
		return txt
	}
	if e.negrita {
		return "\x1b[1m" + e.fg + txt + "\x1b[0m"
	}
	return e.fg + txt + "\x1b[0m"
}

// Sprintf: igual, con formato.
func (e estilo) Sprintf(formato string, a ...any) string { return e.Sprint(fmt.Sprintf(formato, a...)) }

// ps*: los mismos tonos, pero en el formato que exigen los imprimidores de
// pterm (BigText y el spinner), que no aceptan un estilo propio.
// Los colores de 256 que usa el instalador (205 sakura, 245 papel, 114 menta,
// 203 rojo, 221 ambar): los imprimidores de pterm no aceptan un color propio,
// sólo uno de esta escala, así que en vez de aproximarlo se toma el mismo
// número que ya aparece en shin.sh.
var (
	psSakura = pterm.NewStyle(pterm.Color(205))
	psPapel  = pterm.NewStyle(pterm.Color(245))
	psMenta  = pterm.NewStyle(pterm.Color(114))
	psRojo   = pterm.NewStyle(pterm.Color(203))
	psAmbar  = pterm.NewStyle(pterm.Color(221))
)

var (
	stSakura       = tinte("#F28FB4")
	stSakuraFuerte = tinte("#E4668F").conNegrita()
	stLila         = tinte("#9B7BD4")
	stLilaFuerte   = tinte("#7A5BC0").conNegrita()
	stMenta        = tinte("#5FC6A9")
	stAmbar        = tinte("#E0A34E")
	stTinta        = tinte("#CFC6E0")
	stTenue        = tinte("#8B82A3")
	stPapel        = tinte("#F6F1FA")
	stRojo         = tinte("#E36049").conNegrita()
	// stSakuraGrande: el rosa de siempre para lo que hay que ver primero.
	stSakuraGrande = stSakuraFuerte
)

var coloresOn = term.IsTerminal(int(os.Stdout.Fd())) &&
	os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"

func init() {
	if !coloresOn {
		pterm.DisableColor() // no hay ANSI en tuberías ni en NO_COLOR, jamás
	}
	// Los imprimidores que aún usan paquetes ajenos al nuestro (el spinner de
	// la vinculación, sobre todo) llevan prefijos de librería: «SUCCESS»,
	// «ERROR». En una consola con la casa cuidada eso desentona, y en un log de
	// systemd además ocupa. Se cambian a los mismos símbolos que pinta Prefijo,
	// para que el ✓ de una caja y el ✓ de un spinner sean el mismo ✓.
	pterm.Success.Prefix.Text = "✓"
	pterm.Error.Prefix.Text = "✕"
	pterm.Warning.Prefix.Text = "!"
	pterm.Info.Prefix.Text = "◐"
	for _, imp := range []*pterm.PrefixPrinter{&pterm.Success, &pterm.Error, &pterm.Warning, &pterm.Info} {
		imp.MessageStyle = psPapel
	}
	pterm.Success.Prefix.Style = psMenta
	pterm.Error.Prefix.Style = psRojo
	pterm.Warning.Prefix.Style = psAmbar
	pterm.Info.Prefix.Style = pterm.NewStyle(pterm.Color(245))
}

// ConColor: ¿hay terminal de verdad donde estilizar?
func ConColor() bool { return coloresOn }

// Los accesos directos que usa el logger y los paneles del bot.
func Tenue(s string) string  { return stTenue.Sprint(s) }
func Sakura(s string) string { return stSakura.Sprint(s) }
func Menta(s string) string  { return stMenta.Sprint(s) }
func Ambar(s string) string  { return stAmbar.Sprint(s) }
func Rojo(s string) string   { return stRojo.Sprint(s) }
func Papel(s string) string  { return stPapel.Sprint(s) }

// Ancho de la terminal; 80 si no se puede saber (servicio, tubo).
// anchoDePrueba: para que las pruebas puedan ensayar una ventana de 30
// columnas sin tener que fabricar un terminal de verdad (0 = medir de verdad).
var anchoDePrueba int

func Ancho() int {
	if anchoDePrueba > 0 {
		return anchoDePrueba
	}
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// ─── banner ──────────────────────────────────────────────────────────────

// Banner: lo primero que se ve, y ni una línea de más. Seis renglones, sin
// arte de bloques: las letras gigantes de pterm ocupaban media pantalla en
// Termux, se partían en cuanto la ventana tenía menos de ~62 columnas y no se
// podían seleccionar con el dedo. El nombre con espacios (S H I N - M D) se
// lee igual en cualquier fuente.
func Banner(version string) string {
	// Todo se mide contra la ventana: una regla de 41 guiones en 40 columnas se
	// partía por la mitad, y la firma salía cortada en «AGPL-3.0-o / nly».
	ancho := Ancho()
	if ancho > AnchoCasa+2 {
		ancho = AnchoCasa + 2
	}
	útil := maxInt(16, ancho-6)
	var b strings.Builder
	b.WriteString("\n")
	// Las dos reglas terminan en la MISMA columna. Se cuenta el ancho real del
	// adorno (❀ mide distinto según la fuente) en vez de restar cuatro a ciegas:
	// un banner descuadrado por un guion es justo lo que hacía parecer que
	// «la consola iba fatal» en Termux.
	adorno := "  " + stSakura.Sprint("❀") + "  "
	reglas := maxInt(6, útil)
	b.WriteString(adorno + stTenue.Sprint(strings.Repeat("─", maxInt(6, 2+reglas-anchoVisible(stripANSI(adorno))))) + "\n\n")
	// El nombre y el lema: si la ventana es tan estrecha que no cabe el apunte,
	// se quita el apunte antes que partir la palabra a la mitad (en 30 columnas
	// «bot de WhatsApp mul / ti-device» quedaba fatal).
	titulo := stSakuraFuerte.Sprint("S H I N - M D")
	b.WriteString("  " + elegir(ancho,
		titulo+stTenue.Sprint("  ·  whatsmeow"),
		titulo,
		stSakuraFuerte.Sprint("S HIN - MD"),
		stSakuraFuerte.Sprint("SHIN")) + "\n")
	lema := stLila.Sprint("反魂")
	b.WriteString("  " + elegir(ancho,
		lema+stTenue.Sprint("  ·  bot de WhatsApp multi-device"),
		lema+stTenue.Sprint("  ·  bot de WhatsApp"),
		lema) + "\n\n")
	b.WriteString("  " + stTenue.Sprint(strings.Repeat("─", reglas)) + "\n")

	// Los datos, en piezas con su propio color: si la línea entera no cabe, se
	// parte por piezas (nunca a media palabra, que es lo que hacía el envoltorio
	// genérico con el texto ya coloreado).
	meta := stTenue.Sprint("versión ") + stPapel.Sprint(version)
	rasgo := stTenue.Sprint("  ·  " + runtime.Version() + etiquetaEntorno())
	if cabe(ancho, meta+rasgo) {
		b.WriteString("  " + meta + rasgo + "\n")
	} else {
		b.WriteString("  " + meta + "\n  " + rasgo + "\n")
	}
	firma := stTenue.Sprint("hecho por ") + stSakura.Sprint("riokuroxi-svg") +
		stTenue.Sprint("  ·  AGPL-3.0-only")
	if cabe(ancho, firma) {
		b.WriteString("  " + firma + "\n")
	} else {
		b.WriteString("  " + elegir(ancho,
			stTenue.Sprint("hecho por ")+stSakura.Sprint("riokuroxi-svg"),
			stSakura.Sprint("riokuroxi-svg")) + "\n")
		b.WriteString("  " + stTenue.Sprint("AGPL-3.0-only") + "\n")
	}
	b.WriteString("\n")
	return b.String()
} // etiquetaEntorno añade « · termux» cuando toca, como el banner del JS.
func etiquetaEntorno() string {
	if strings.Contains(os.Getenv("PREFIX"), "com.termux") || os.Getenv("TERMUX_VERSION") != "" {
		return "  ·  termux"
	}
	switch runtime.GOOS {
	case "windows":
		return "  ·  windows"
	case "darwin":
		return "  ·  macos"
	default:
		return ""
	}
}

// ─── cajas ────────────────────────────────────────────────────────────────

// Caja: un aviso enmarcado con el ancho de la casa. Usa Marco, el mismo que el
// panel del bot, para que un mensaje suelto y la pantalla viva se vean
// idénticos: borde de una columna (`│`, no `█` ni `▓`), título arriba a la
// izquierda y 58 columnas —que es lo que entra de una pieza en Termux en
// vertical— o lo que haya si la ventana es aún más estrecha.
func Caja(w io.Writer, titulo string, lineas ...string) {
	ancho := AnchoFijo()
	filas := Marco(stSakuraFuerte.Sprint(titulo), lineas, ancho)
	fmt.Fprintln(w)
	for _, l := range filas {
		fmt.Fprintln(w, l)
	}
	fmt.Fprintln(w)
}

// envolver: corta por palabras al límite de COLUMNAS visibles (runewidth,
// con códigos ANSI que no cuentan). Una palabra que sola exceda el límite
// se parte. Aquí jamás se rompe una línea coloreada a la mitad, porque el
// corte responde a lo que el ojo mide, no a los bytes del escape.
// Envolver parte un texto en líneas de a lo sumo `limite` columnas. Se
// exporta porque los paneles de otros paquetes (vinculación, avisos) necesitan
// el mismo criterio: en Termux, una línea larga se parte por la mitad y
// queda ilegible a media frase.
func Envolver(s string, limite int) []string { return envolver(s, limite) }

func envolver(s string, limite int) []string {
	if limite <= 0 {
		return []string{s}
	}
	var out []string
	for _, parrafo := range strings.Split(s, "\n") {
		if parrafo == "" {
			out = append(out, "")
			continue
		}
		var actual strings.Builder
		anchoActual := 0
		cerrar := func() {
			if actual.Len() > 0 {
				out = append(out, actual.String())
				actual.Reset()
				anchoActual = 0
			}
		}
		for _, palabra := range strings.Fields(parrafo) {
			w := anchoVisible(palabra) // sin los escapes: medir el coloreado salía el doble
			conColor := strings.ContainsRune(palabra, 0x1b)
			sep := 0
			if anchoActual > 0 {
				sep = 1
			}
			if anchoActual+sep+w > limite {
				cerrar()
				if conColor {
					// Una palabra con códigos dentro NO se parte: el corte
					// partiría el escape por la mitad y la terminal se queda con
					// el estilo comido. Se suelta entera, PERO CONTANDO: si se
					// ignorara su ancho, la línea entera mide más de lo que se
					// midió y el terminal parte por su cuenta (la palabra
					// partida en dos líneas que se veía en Termux).
					if actual.Len() > 0 {
						out = append(out, actual.String())
						actual.Reset()
					}
					out = append(out, palabra)
					anchoActual = 0
					continue
				}
				if w > limite { // palabra gigante: partirla por columnas
					for w > limite {
						corte := runewidth.Truncate(stripANSI(palabra), limite, "")
						out = append(out, corte)
						palabra = strings.TrimPrefix(palabra, corte)
						w = anchoVisible(palabra)
					}
					actual.WriteString(palabra)
					anchoActual = w
					continue
				}
			}
			if anchoActual > 0 {
				actual.WriteString(" ")
			}
			actual.WriteString(palabra)
			anchoActual += sep + w
			if sep+w >= 0 && anchoActual >= limite {
				cerrar()
			}
		}
		cerrar()
	}
	return out
}

// ─── bytes legibles ──────────────────────────────────────────────────────

// Humanizar: 2844098560 → «2.6G». Como el formateador del instalador: el
// límite justo en 1024 y una sola coma, porque en una caja de 58 columnas más
// decimales no caben y nadie los lee.
func Humanizar(n uint64) string {
	const g = 1 << 30
	const m = 1 << 20
	switch {
	case n >= g:
		v := float64(n) / g
		if v >= 10 || v == float64(uint64(v)) {
			return fmt.Sprintf("%dG", uint64(v+0.5))
		}
		return fmt.Sprintf("%.1fG", v)
	case n >= m:
		return fmt.Sprintf("%dM", (n+m/2)>>20)
	default:
		return fmt.Sprintf("%dK", (n+512)>>10)
	}
}

// ─── spinner ──────────────────────────────────────────────────────────────

// Spinner: «Conectando con WhatsApp…» que gira; Stop/Success/Fail lo
// cierran. Fuera de terminal devuelve nil y los llamadores no preguntan.
func Spinner(w io.Writer, texto string) *pterm.SpinnerPrinter {
	if !coloresOn {
		return nil
	}
	s := pterm.DefaultSpinner
	s.Writer = w
	s.Style = psSakura       // el asterisco girando en rosa Shin
	s.MessageStyle = psPapel // el texto en blanco papel
	if sp, err := s.Start(texto); err == nil {
		return sp
	}
	return nil
}

// LettersFromString sin estilo: útil para pruebas del banner grande.

// ─── la línea de log ─────────────────────────────────────────────────────

// SakuraFuerte: el rosa de la casa con negrita, para lo que hay que ver
// primero (la opción activa del menú, el código de vinculación).
func SakuraFuerte(s string) string { return stSakuraFuerte.Sprint(s) }

// Prefijo pinta UNA línea de registro: el icono del nivel, la hora y el texto.
//
// A propósito NO lleva la píldora «INFO/WARNING» que ponía pterm: ese prefijo
// de ~22 columnas es lo que hacía que, en las 50 de Termux, cada aviso saliera
// partido y con sangrías raras. Un símbolo basta para recorrer la lista de un
// vistazo, y el archivo de log (handlerTexto) sigue llevando el nivel escrito
// para quien grepee.
func Prefijo(w io.Writer, hora, tag, msg string) {
	simbolo, color := "◐", stLila
	switch tag {
	case "OK":
		simbolo, color = "✓", stMenta
	case "AVISO":
		simbolo, color = "!", stAmbar
	case "ERROR":
		simbolo, color = "✕", stSakuraFuerte
	case "DEBUG":
		simbolo, color = "·", stTenue
	}
	tinta := stTinta
	if tag == "ERROR" || tag == "AVISO" {
		tinta = color
	}
	// El mensaje se pela y se vuelve a colorear TROZO a TROZO: si se envolviera
	// ya coloreado, el cierre (\e[0m) quedaría sólo al final y la continuación
	// saldría pintada con el estilo del aviso.
	plano := strings.TrimSpace(stripANSI(msg))
	cabecera := "  " + color.Sprint(simbolo) + " " + stTenue.Sprint(hora) + "  "
	útil := Ancho() - 2 - anchoVisible("  ◐ 00:00:00  ")
	if útil < 12 {
		útil = 12
	}
	piezas := envolver(plano, útil)
	if len(piezas) == 0 {
		piezas = []string{""}
	}
	fmt.Fprintln(w, cabecera+tinta.Sprint(piezas[0]))
	// La continuación se sangra hasta la columna del texto: en un móvil de 50
	// columnas un aviso de dos líneas sin esto parece DOS avisos.
	sangría := strings.Repeat(" ", maxInt(2, anchoVisible("  ◐ 00:00:00  ")))
	for _, l := range piezas[1:] {
		fmt.Fprintln(w, sangría+tinta.Sprint(l))
	}
}

// elegir: la primera de las versiones que quepa en `ancho` columnas (la última
// se acepta siempre: es la forma corta de verdad).
func elegir(ancho int, versiones ...string) string {
	for _, v := range versiones {
		if cabe(ancho, v) {
			return v
		}
	}
	return versiones[len(versiones)-1]
}

// cabe: si una línea entra en la ventana dejando aire a los lados. Se resta 4
// porque Termux y compañía a veces tienen la última columna ocupada, y porque
// un glifo ambiguo (反魂, ●) puede medirse al doble según la fuente.
func cabe(ancho int, v string) bool { return anchoVisible(v) <= maxInt(4, ancho-4) }

// Frase: un párrafo suelto, envuelto al ancho de la terminal. Se escribe con
// sangría de dos y las continuaciones a cuatro, para que un aviso de dos líneas
// no se lea como dos avisos.
func Frase(texto string) {
	ancho := Ancho() - 6
	if ancho < 28 {
		ancho = 28
	}
	for i, l := range envolver(texto, ancho) {
		if i == 0 {
			fmt.Fprintln(os.Stdout, "  "+l)
			continue
		}
		fmt.Fprintln(os.Stdout, "    "+l)
	}
}

// ─── el pie de pantalla ───────────────────────────────────────────────────

// Ayuda: una línea del bloque que se mete en el pie con ?.
func Ayuda(s string) string { return stTenue.Sprint(s) }

// EsTermux: el bot corre en el propio teléfono. Se cambia el advice: ahí no
// hay cámara que escanee un QR y «ssh» no viene a cuenta.
func EsTermux() bool {
	return strings.Contains(os.Getenv("PREFIX"), "/com.termux/") ||
		os.Getenv("TERMUX_APP_PACKAGE") != "" || os.Getenv("TERMUX_VERSION") != ""
}

// EsServicio: systemd, pm2, docker, un `nohup … &`. Ahí no hay teclado: ni
// Ctrl+C ni teclas, y decir «pulsad Ctrl+C para salir» es mentir.
func EsServicio() bool {
	if os.Getenv("INVOCATION_ID") != "" || os.Getenv("PM2_HOME") != "" ||
		os.Getenv("SUPERVISOR_PID") != "" {
		return true
	}
	if pid := os.Getenv("PPID"); pid == "1" {
		return true
	}
	return !term.IsTerminal(int(os.Stdin.Fd())) && !term.IsTerminal(int(os.Stdout.Fd()))
}

// Pie: las dos últimas líneas de la pantalla: las teclas y cómo se para.
//
// No se trunca a media palabra: hay una variante por ancho (los números salen
// de medir las frases con anchoVisible, no de adivinar) y sólo si aun así la
// ventana es más estrecha que la variante corta se corta con «…». Un pie
// partido en Termux es lo que hacía parecer que «la consola iba fatal».
func Pie(ancho int) []string {
	teclas := "q salir · e logs · ? ayuda"
	switch {
	case ancho >= 65:
		teclas = "q salir · e registros · r reconectar · flechas releer · ? ayuda"
	case ancho >= 48:
		teclas = "q salir · e registros · r reconectar · ? ayuda"
	case ancho >= 43:
		teclas = "q salir · e logs · r reconectar · ? ayuda"
	}
	parar := "para parar: q · «shin stop»"
	switch {
	case EsServicio():
		// Ahí no hay teclado: prometer Ctrl+C sería mentira.
		parar = "sin teclado (servicio): «shin stop» o el gestor de servicios"
		if ancho < 62 {
			parar = "sin teclado: «shin stop»"
		}
	case ancho >= 58:
		parar = "para parar: q · Ctrl+C · o «shin stop» desde otra sesión"
	case ancho >= 38:
		parar = "para parar: q · Ctrl+C · «shin stop»"
	}
	out := make([]string, 0, 2)
	for _, l := range []string{"  " + teclas, "  " + parar} {
		out = append(out, stTenue.Sprint(Truncar(l, ancho)))
	}
	return out
}
