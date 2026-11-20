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

// 反魂 sakura: la paleta de Shin, igual que el instalador y el JS.
var (
	stSakura = pterm.NewStyle(pterm.FgLightMagenta, pterm.Bold)
	stPapel  = pterm.NewStyle(pterm.FgLightWhite)
	stTenue  = pterm.NewStyle(pterm.FgGray)
	stMenta  = pterm.NewStyle(pterm.FgLightGreen)
	stRojo   = pterm.NewStyle(pterm.FgLightRed, pterm.Bold)
	stAmbar  = pterm.NewStyle(pterm.FgLightYellow)
	// stSakuraFuerte: el mismo rosa con negrita, para la opción activa del
	// menú y la cabecera del panel.
	stSakuraFuerte = pterm.NewStyle(pterm.FgLightMagenta, pterm.Bold)
)

var coloresOn = term.IsTerminal(int(os.Stdout.Fd())) &&
	os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"

func init() {
	if !coloresOn {
		pterm.DisableColor() // no hay ANSI en tuberías ni en NO_COLOR, jamás
	}
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
func Ancho() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// ─── banner ──────────────────────────────────────────────────────────────

// Banner: lo primero que se ve. Con pantalla suficiente, SHIN-MD en letras
// grandes con degradado sakura→magenta; en pantalla estrecha o sin color,
// el título compacto. Debajo, siempre, el subtítulo 反魂 y los metadatos.
func Banner(version string) string {
	var b strings.Builder
	b.WriteString("\n")
	if w := Ancho(); w >= 62 && coloresOn {
		b.WriteString(grandeConDegradado("SHIN-MD"))
		b.WriteString("\n")
	} else {
		b.WriteString("  " + stSakura.Sprint("S H I N — M D") + "\n")
	}
	b.WriteString("  " + stSakura.Sprint("反魂") +
		stTenue.Sprint("  ·  bot de WhatsApp multi-device") + "\n")
	b.WriteString(stTenue.Sprint("  ┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈") + "\n")
	b.WriteString("  " + stPapel.Sprintf("versión %s", version) +
		stTenue.Sprintf("  ·  %s%s", runtime.Version(), etiquetaEntorno()) + "\n")
	b.WriteString(stTenue.Sprint("  hecho por riokuroxi-svg  ·  AGPL-3.0-only") + "\n\n")
	return b.String()
}

// grandeConDegradado: BigText con fade rosa→magenta letra a letra.
func grandeConDegradado(s string) string {
	ini := pterm.NewRGB(244, 114, 182) // sakura claro
	fin := pterm.NewRGB(168, 60, 160)  // magenta profundo
	runas := []rune(s)
	letras := make(pterm.Letters, 0, len(runas))
	for i, r := range runas {
		// Cada letra lleva AMBOS: el RGB si la terminal es truecolor y un
		// Style de respaldo — pterm usa el Style cuando no (y con Style nil
		// entraría un nil-deref). Sakura y magenta base aquí.
		letras = append(letras, pterm.Letter{
			String: string(r),
			RGB:    ini.Fade(0, float32(len(runas)), float32(i), fin),
			Style:  stSakura,
		})
	}
	txt, err := pterm.DefaultBigText.WithLetters(letras).Srender()
	if err != nil {
		return "  " + stSakura.Sprint(s) + "\n"
	}
	return txt + "\n"
}

// etiquetaEntorno añade « · termux» cuando toca, como el banner del JS.
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

// Caja: un panel pterm con título arriba a la izquierda, borde sakura y
// relleno, envolviendo el texto al ancho útil de la terminal (clave en un
// móvil en vertical: la caja jamás desborda la pantalla). Las líneas vacías
// respiran: una sola abre aire, cualquier racha colapsa en ella.
func Caja(w io.Writer, titulo string, lineas ...string) {
	anchoUtil := Ancho() - 8
	if anchoUtil > 68 {
		anchoUtil = 68
	}
	if anchoUtil < 24 {
		anchoUtil = 24
	}
	envueltas := make([]string, 0, len(lineas))
	ultimoBlanco := true // el título ya hace de borde; no arrancar con aire
	for _, l := range lineas {
		piezas := envolver(l, anchoUtil)
		if len(piezas) == 1 && strings.TrimSpace(l) == "" {
			if ultimoBlanco {
				continue
			}
			ultimoBlanco = true
			envueltas = append(envueltas, "")
			continue
		}
		ultimoBlanco = false
		envueltas = append(envueltas, piezas...)
	}
	caja := pterm.DefaultBox.
		WithTitle(titulo).WithTitleTopLeft().
		WithLeftPadding(1).WithRightPadding(1).
		WithTopPadding(1).WithBottomPadding(1).
		WithBoxStyle(pterm.NewStyle(pterm.FgLightMagenta)).
		WithTitleStyle(pterm.NewStyle(pterm.FgLightMagenta, pterm.Bold)).
		WithWriter(w)
	caja.Println(strings.Join(envueltas, "\n"))
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
			w := runewidth.StringWidth(palabra)
			sep := 0
			if anchoActual > 0 {
				sep = 1
			}
			if anchoActual+sep+w > limite {
				cerrar()
				if w > limite { // palabra gigante: partirla por columnas
					for w > limite {
						corte := runewidth.Truncate(palabra, limite, "")
						out = append(out, corte)
						palabra = strings.TrimPrefix(palabra, corte)
						w = runewidth.StringWidth(palabra)
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

// ─── la caja de «listo» con vitales ───────────────────────────────────────

// Listo: los datos del panel que cierra el arranque.
type Listo struct {
	Version  string
	Cuenta   string
	Comandos int
	Prefijos []string
	Web      string
	PID      int
}

// PanelListo: cierra el arranque — ● ACTIVO · pid, cuenta, comandos,
// vitales con barras y la web del panel. Sabor a la consola del JS.
func PanelListo(w io.Writer, d Listo) {
	lineas := []string{stMenta.Sprint("● ") + stPapel.Sprint("ACTIVO") +
		stTenue.Sprintf("  pid %d", d.PID)}
	if d.Cuenta != "" {
		lineas = append(lineas, stTenue.Sprint("cuenta  ")+stPapel.Sprint(d.Cuenta))
	}
	if d.Comandos > 0 {
		lineas = append(lineas, stTenue.Sprintf("comandos  ")+
			stPapel.Sprintf("%d · prefijos [%s]", d.Comandos, strings.Join(d.Prefijos, " ")))
	}
	du, dt, ru, rt := vitales()
	if dt > 0 {
		lineas = append(lineas, stTenue.Sprint("disco   ")+
			barra(float64(du)/float64(dt))+" "+stPapel.Sprint(Humanizar(du)))
	}
	if rt > 0 {
		lineas = append(lineas, stTenue.Sprint("ram     ")+
			barra(float64(ru)/float64(rt))+" "+stPapel.Sprintf("%s/%s", Humanizar(ru), Humanizar(rt)))
	}
	if d.Web != "" {
		lineas = append(lineas, stTenue.Sprint("web     ")+stPapel.Sprint(d.Web))
	}
	Caja(w, "SHIN-MD-GO "+d.Version, lineas...)
}

// barra: ▓▓▓▓░░░░░░ con color según lo lleno que vaya el recurso.
// Grande: letras grandes con degradado sakura para lo que TODO el flujo
// gira alrededor (hoy: el código de vinculación). Sin color, el liso.
func Grande(s string) string {
	if coloresOn {
		return grandeConDegradado(s)
	}
	return stPapel.Sprint(s) + "\n"
}

// barra: ▓▓▓▓░░░░░░ con color según lo lleno que vaya el recurso.
func barra(frac float64) string {
	const n = 12
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	llenos := int(frac*float64(n) + 0.5)
	relleno := strings.Repeat("▓", llenos)
	resto := strings.Repeat("░", n-llenos)
	estilo := stMenta
	switch {
	case frac >= 0.9:
		estilo = stRojo
	case frac >= 0.7:
		estilo = stAmbar
	}
	return estilo.Sprint(relleno) + stTenue.Sprint(resto)
}

// Humanizar: 2.6G, 512M, 128K. Un decimal cuando hace falta, como el JS.
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
	s.Style = stSakura       // el asterisco girando en rosa Shin
	s.MessageStyle = stPapel // el texto en blanco papel
	if sp, err := s.Start(texto); err == nil {
		return sp
	}
	return nil
}

// pelarEstilo quita un estilo uniforme: devuelve el prefijo de estilo, el
// texto limpio y si era uniforme (estilo al principio, reset al final, nada
// más en medio).
func pelarEstilo(s string) (estilo, limpio string, ok bool) {
	if !strings.HasPrefix(s, "\x1b[") {
		return "", s, false
	}
	fin := strings.Index(s, "m")
	if fin < 0 {
		return "", s, false
	}
	estilo, resto := s[:fin+1], s[fin+1:]
	if !strings.HasSuffix(resto, "\x1b[0m") || strings.Contains(resto[:len(resto)-4], "\x1b[") {
		return "", s, false
	}
	return estilo, strings.TrimSuffix(resto, "\x1b[0m"), true
}

// ─── semáforos de prefijo (los tags [INFO]/[OK]/…) del log bonito ────────

// Prefijo devuelve el mensaje ya con su tag de nivel estilo herramienta:
// el tiempo en gris y el tag con la píldora de pterm (INFO magenta, OK
// verde, AVISO ámbar, ERROR rojo).
func Prefijo(w io.Writer, hora, tag, msg string) {
	pp := ptermInfo
	switch tag {
	case "OK":
		pp = pterm.Success
	case "AVISO":
		pp = pterm.Warning
	case "ERROR":
		pp = pterm.Error
	case "DEBUG":
		pp = pterm.Debug
	}
	pp.Writer = w
	pp.Println(stTenue.Sprint(hora+"  ") + alinear(hora, msg))
}

// sangríaLog: columnas que reserva pterm en cada línea de continuación del
// mensaje (el tag con su barra). Está MEDIDO sobre su impresor, no estimado:
// si se calcula por encima, la línea vuelve a salirse de la pantalla y se corta
// a media palabra, que es justo lo que se quiere evitar.
const sangríaLog = 22

// alinear parte el mensaje para que quepa en la terminal.
//
// pterm NO envuelve el texto largo (lo parte el terminal, a mitad de palabra),
// pero sí sangra por sí solo lo que venga después de un «\n» dentro del
// mensaje. Con esa sangría puesta, aquí únicamente se parte el texto: no se
// añade indentación propia. En Termux (50 columnas) un aviso de tres renglones
// se lee de corrido en vez de quedar hecho un rompecabezas.
func alinear(hora, msg string) string {
	ancho := Ancho()
	// A partir de 16 columnas de texto útil merece la pena partir; por debajo
	// (una consola de 38) cada renglón sería de dos palabras y se lee peor que
	// el corte del propio terminal.
	if !coloresOn || ancho < sangríaLog+16 {
		return msg
	}
	// Si el mensaje va entero de un color (así lo manda xlog), se quita el
	// estilo antes de medir: envolver texto con ANSI dentro cuenta los escapes
	// como si fueran letras y el cálculo se va al doble.
	plano, estilo := msg, ""
	if e, limpio, ok := pelarEstilo(msg); ok {
		plano, estilo = limpio, e
	}
	lineas := envolver(plano, ancho-sangríaLog)
	if len(lineas) < 2 {
		return msg
	}
	if estilo != "" {
		for i := range lineas {
			lineas[i] = estilo + lineas[i] + "\x1b[0m"
		}
	}
	return strings.Join(lineas, "\n")
}

var ptermInfo = pterm.Info

// LettersFromString sin estilo: útil para pruebas del banner grande.
