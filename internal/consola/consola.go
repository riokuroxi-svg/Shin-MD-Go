// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// consola.go — la cara del bot en la terminal: el banner marca Shin, la
// caja de vitales al conectar y la paleta sakura del JS. Todo es texto
// plano con color solo cuando hay una terminal de verdad detrás (nada de
// códigos ANSI dentro de un log redirigido o de un NO_COLOR=1).
//
// Espejo estético de boot/index.js + scripts/consola.mjs del JS: mismo
// banner (S H I N — M D / 反魂), misma caja ┌ SHIN-MD … ● ACTIVO ┐.
package consola

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"

	"golang.org/x/term"
)

// ─── paleta: los 256-colores del instalador y del JS ─────────────────────

type paleta struct {
	sakura, papel, tenue, menta, rojo, ambar, neg, reset string
}

var sinColor paleta

func paletaActual() paleta {
	if !ConColor() {
		return sinColor
	}
	return paleta{
		sakura: "\033[38;5;205m", papel: "\033[38;5;255m", tenue: "\033[38;5;245m",
		menta: "\033[38;5;114m", rojo: "\033[38;5;203m", ambar: "\033[38;5;221m",
		neg: "\033[1m", reset: "\033[0m",
	}
}

// ConColor: hay terminal de verdad, nadie la prohibió y no es tonta.
var ConColor = func() func() bool {
	ok := term.IsTerminal(int(os.Stdout.Fd())) &&
		os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	return func() bool { return ok }
}()

// Tenue/Sakura/… Accesores sueltos para el logger, que no dibuja cajas.
func Tenue(s string) string  { pa := paletaActual(); return pa.tenue + s + pa.reset }
func Sakura(s string) string { pa := paletaActual(); return pa.sakura + s + pa.reset }
func Menta(s string) string  { pa := paletaActual(); return pa.menta + s + pa.reset }
func Ambar(s string) string  { pa := paletaActual(); return pa.ambar + s + pa.reset }
func Rojo(s string) string   { pa := paletaActual(); return pa.rojo + s + pa.reset }

// ─── banner ──────────────────────────────────────────────────────────────

// Banner: lo primero que se ve, idéntico en sabor al del JS:
//
//	S H I N — M D
//	反魂 · bot de WhatsApp multi-device
//	─────────────
//	 versión X · goY · termux
//	 hecho por riokuroxi-svg · AGPL-3.0-only
func Banner(version string) string {
	pa := paletaActual()
	regla := strings.Repeat("─", 48)
	var b strings.Builder
	b.WriteString("\n  " + pa.tenue + "✦  " + regla + pa.reset + "\n\n")
	b.WriteString("   " + pa.sakura + pa.neg + "S H I N — M D" + pa.reset + "\n")
	b.WriteString("   " + pa.sakura + "反魂" + pa.reset +
		pa.tenue + "  ·  bot de WhatsApp multi-device" + pa.reset + "\n\n")
	b.WriteString("  " + pa.tenue + "   " + regla + pa.reset + "\n\n")
	b.WriteString("   " + pa.papel + "versión " + version + pa.reset +
		pa.tenue + "  ·  " + runtime.Version() + etiquetaEntorno() + pa.reset + "\n")
	b.WriteString("   " + pa.tenue + "hecho por riokuroxi-svg  ·  AGPL-3.0-only" + pa.reset + "\n\n")
	return b.String()
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

// ─── vitales (MEM/DISK del propio equipo, sabor a sysinfo) ───────────────

// Humaniza: 2.6G, 512M, 128K. Un decimal cuando hace falta, como el JS.
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

// (los vitales viven en vitales_unix.go / vitales_windows.go: syscall.Sysinfo
// y Statfs no existen en Windows y cruzar el tag es lo que permite que
// «make windows» siga compilando)

// ─── la caja de «listo» ───────────────────────────────────────────────────

// Listo: los datos del panel que cierra el arranque.
type Listo struct {
	Version  string
	Cuenta   string // nombre del perfil o número, una vez vinculado
	Comandos int
	Prefijos []string
	Web      string // http://127.0.0.1:3000 o "" si el panel está apagado
	PID      int
}

// PanelListo: la caja del JS con los cuatro datos que se miran tras
// vincular (cuenta, comandos, prefijo, web), más los vitales que enseña la
// consola del JS (● ACTIVO · pid · disco · ram). A stdout, tal cual.
func PanelListo(w io.Writer, d Listo) {
	pa := paletaActual()
	lineas := []string{
		" " + verdePunto(pa) + pa.neg + "ACTIVO" + pa.reset + pa.tenue +
			fmt.Sprintf("  pid %d", d.PID) + pa.reset,
	}
	if d.Cuenta != "" {
		lineas = append(lineas, " "+pa.tenue+"cuenta: "+pa.reset+pa.papel+d.Cuenta+pa.reset)
	}
	if d.Comandos > 0 {
		lineas = append(lineas, " "+pa.tenue+fmt.Sprintf("%d comandos", d.Comandos)+
			" · prefijos "+pa.reset+pa.papel+"["+strings.Join(d.Prefijos, " ")+"]"+pa.reset)
	}
	du, dt, ru, rt := vitales()
	if dt > 0 && rt > 0 {
		lineas = append(lineas, " "+pa.tenue+"disco "+pa.reset+pa.papel+Humanizar(du)+pa.reset+
			pa.tenue+" · ram "+pa.reset+pa.papel+Humanizar(ru)+"/"+Humanizar(rt)+pa.reset)
	}
	if d.Web != "" {
		lineas = append(lineas, " "+pa.tenue+"web "+pa.reset+pa.papel+d.Web+pa.reset)
	}
	cajaCuadrada(w, "SHIN-MD-GO "+d.Version, lineas)
}

func verdePunto(pa paleta) string { return pa.menta + "● " + pa.reset }

// cajaCuadrada: ┌ TÍTULO ──────┐ / │ líneas │ / └──────┘, ancho = la línea
// más larga + 4 (mínimo 56), contando runas y descontando códigos ANSI.
func cajaCuadrada(w io.Writer, titulo string, lineas []string) {
	const min = 56
	pa := paletaActual()
	ancho := min
	for _, l := range lineas {
		if n := largo(l) + 4; n > ancho {
			ancho = n
		}
	}
	// borde superior: «┌ titulo ─…─┐»
	relleno := ancho - 4 - len([]rune(titulo))
	if relleno < 0 {
		relleno = 0
	}
	fmt.Fprint(w, "  "+pa.sakura+"┌ "+titulo+" "+strings.Repeat("─", relleno)+"┐"+pa.reset+"\n")
	for _, l := range lineas {
		pad := ancho - 3 - largo(l)
		if pad < 0 {
			pad = 0
		}
		fmt.Fprint(w, "  "+pa.sakura+"│"+pa.reset+l+strings.Repeat(" ", pad)+pa.sakura+" │"+pa.reset+"\n")
	}
	fmt.Fprint(w, "  "+pa.sakura+"└"+strings.Repeat("─", ancho-2)+"┘"+pa.reset+"\n\n")
}

// largo: runas visibles — los códigos ANSI no ocupan pantalla.
func largo(s string) int {
	n := 0
	enEscape := false
	for _, r := range s {
		switch {
		case r == '\033':
			enEscape = true
		case enEscape:
			if r == 'm' {
				enEscape = false
			}
		default:
			n++
		}
	}
	return n
}
