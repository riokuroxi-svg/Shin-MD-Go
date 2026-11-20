// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// estado.go — la pantalla «viva» mientras el bot corre.
//
// El problema real: en un móvil (Termux, 50-60 columnas) cualquier línea que
// se envuelva rompe las cuentas de «subir N líneas» y el panel se duplica.
// Por eso aquí no se usa el Area de pterm: se trunca cada línea al ancho
// real y se repinta con movimientos relativos, y mientras el panel está
// visible los logs NO se imprimen sueltos —se guardan y se enseñan dentro
// del propio panel—, que es lo que evita el mezclado.
//
// Todo se apaga solo sin terminal: en un servicio o un tubo el bot funciona
// igual y nadie se queda esperando una tecla.
package consola

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// Estado es una foto de lo que el panel enseña. Lo rellena main() cada
// segundo; aquí no se consulta nada por dentro.
type Estado struct {
	Version    string
	Cuenta     string
	Conectado  bool
	Prefijos   []string
	Comandos   int
	Uptime     time.Duration
	Enviados   int
	Cupo       int
	EnCola     int
	Procesados uint64
	Recibidos  uint64
	Web        string
}

// LineasEstado: el panel como líneas de texto YA coloreadas y truncadas al
// ancho `ancho`. Sin ANSI de por medio, contar líneas es exacto.
func LineasEstado(e Estado, ancho int) []string {
	estado := stRojo.Sprint("○ desconectado")
	if e.Conectado {
		estado = stMenta.Sprint("● conectado")
	}
	var out []string
	out = append(out, stSakura.Sprint("反魂")+" "+stPapel.Sprint("SHIN-MD")+
		stTenue.Sprint(" v"+e.Version)+"   "+estado)
	if e.Cuenta != "" {
		out = append(out, stTenue.Sprint("cuenta     ")+stPapel.Sprint(e.Cuenta))
	}
	out = append(out, stTenue.Sprint("encendido  ")+stPapel.Sprint(duracion(e.Uptime)))
	if e.Cupo > 0 {
		out = append(out, stTenue.Sprint("cupo hoy   ")+barra(float64(e.Enviados)/float64(e.Cupo))+
			" "+stPapel.Sprintf("%d/%d", e.Enviados, e.Cupo))
	}
	if e.Comandos > 0 {
		out = append(out, stTenue.Sprint("comandos   ")+
			stPapel.Sprintf("%d · prefijos [%s]", e.Comandos, strings.Join(e.Prefijos, " ")))
	}
	out = append(out, stTenue.Sprint("recibidos  ")+stPapel.Sprintf("%d · %d respondidos · %d en cola",
		e.Recibidos, e.Procesados, e.EnCola))
	du, dt, ru, rt := vitales()
	if dt > 0 {
		out = append(out, stTenue.Sprint("disco      ")+barra(float64(du)/float64(dt))+" "+stPapel.Sprint(Humanizar(du)))
	}
	if rt > 0 {
		out = append(out, stTenue.Sprint("ram        ")+barra(float64(ru)/float64(rt))+
			" "+stPapel.Sprintf("%s/%s", Humanizar(ru), Humanizar(rt)))
	}
	if e.Web != "" {
		out = append(out, stTenue.Sprint("panel web  ")+stPapel.Sprint(e.Web))
	}
	// Truncado por ANCHO VISIBLE (runewidth no cuenta los escapes): nada se
	// envuelve y las cuentas de repintado cuadran.
	for i, l := range out {
		out[i] = truncarANSI(l, ancho)
	}
	return out
}

// truncarANSI corta a `ancho` columnas visibles respetando los códigos ANSI.
func truncarANSI(s string, ancho int) string {
	if ancho <= 0 {
		return s
	}
	var b strings.Builder
	visto := 0
	cortado := false
	for i := 0; i < len(s); {
		if s[i] == 0x1b { // secuencia CSI: se copia entera sin contar
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && !(s[j] >= '@' && s[j] <= '~') {
					j++
				}
				if j < len(s) {
					j++
				}
			}
			b.WriteString(s[i:j])
			i = j
			continue
		}
		r := []rune(s[i:])[0]
		w := runewidth.RuneWidth(r)
		if visto+w > ancho {
			// Algo se queda fuera: se marca con «…» en vez de cortar en seco
			// (en un móvil una fila cortada sin más parece un fallo).
			if i < len(s) && ancho >= 1 {
				cortado = true
			}
			break
		}
		b.WriteRune(r)
		visto += w
		i += len(string(r))
	}
	if cortado && visto > 0 {
		// Se quita el último carácter visible para hacer hueco a la elipsis:
		// el ancho total no cambia y el bloque no se descoloca.
		sinUltimo := strings.TrimSuffix(b.String(), "\x1b[0m")
		for n := 0; n < 1; n++ {
			rs := []rune(stripANSI(sinUltimo))
			if len(rs) == 0 {
				break
			}
			sinUltimo = strings.TrimSuffix(sinUltimo, string(rs[len(rs)-1]))
		}
		b.Reset()
		b.WriteString(sinUltimo + "…")
	}
	b.WriteString("\033[0m")
	return b.String()
}

func duracion(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	return fmt.Sprintf("%dm %02ds", m, s)
}

// ─── teletipo: los logs van donde el panel diga ──────────────────────────

// Teletipo es el destino de los logs. Cuando el panel está visible guarda las
// líneas en el historial (para enseñarlas dentro del panel y con la tecla
// `l`) en vez de soltarlas en mitad del dibujo; cuando no lo está, escribe
// al terminal como siempre.
type Teletipo struct {
	mu        sync.Mutex
	w         io.Writer
	activo    bool
	historial *Historial
	alCambiar func() // lo pone el panel para repintarse al llegar un log
}

// NuevoTeletipo envuelve la salida de consola.
func NuevoTeletipo(w io.Writer, h *Historial) *Teletipo {
	return &Teletipo{w: w, historial: h}
}

func (t *Teletipo) Write(p []byte) (int, error) {
	t.mu.Lock()
	activo, alCambiar := t.activo, t.alCambiar
	t.mu.Unlock()

	if activo {
		if t.historial != nil {
			// UN registro = UNA entrada del historial, aunque venga envuelto en
			// varias líneas para la pantalla: así `l` enseña avisos, no trozos.
			var b strings.Builder
			for _, l := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
				l = strings.TrimLeft(l, " \t")
				if strings.TrimSpace(stripANSI(l)) == "" {
					continue
				}
				if b.Len() > 0 {
					b.WriteString(" ")
				}
				b.WriteString(l)
			}
			if b.Len() > 0 {
				t.historial.Anadir(b.String())
			}
		}
		if alCambiar != nil {
			alCambiar()
		}
		return len(p), nil
	}
	t.mu.Lock()
	w := t.w
	t.mu.Unlock()
	return w.Write(p)
}

// Activar(conectar bool, alCambiar func()) apaga o enciende el modo panel.
func (t *Teletipo) Activar(activo bool, alCambiar func()) {
	t.mu.Lock()
	t.activo = activo
	t.alCambiar = alCambiar
	t.mu.Unlock()
}

// EsTeletipo: para que xlog sepa que puede seguir pintando bonito aunque el
// writer no sea un *os.File.
func (t *Teletipo) EsTeletipo() bool { return true }

// stripANSI quita los códigos de color (para medir y para el historial).
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && !(s[j] >= '@' && s[j] <= '~') {
					j++
				}
				if j < len(s) {
					j++
				}
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// ─── historial de avisos ─────────────────────────────────────────────────

// Historial guarda los últimos mensajes sin crecer sin fin.
type Historial struct {
	mu   sync.Mutex
	Max  int
	cola []string
}

// Anadir apunta un aviso con la hora.
func (h *Historial) Anadir(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Max <= 0 {
		h.Max = 40
	}
	h.cola = append(h.cola, time.Now().Format("15:04")+"  "+msg)
	if len(h.cola) > h.Max {
		h.cola = h.cola[len(h.cola)-h.Max:]
	}
}

// Ultimos devuelve una copia (las n últimas, o todas si n<=0).
func (h *Historial) Ultimos(n int) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	inicio := 0
	if n > 0 && len(h.cola) > n {
		inicio = len(h.cola) - n
	}
	out := make([]string, len(h.cola)-inicio)
	copy(out, h.cola[inicio:])
	return out
}

// ImprimirHistorial pinta los últimos avisos, o dice que no hay.
func ImprimirHistorial(w io.Writer, avisos []string) {
	if len(avisos) == 0 {
		fmt.Fprintln(w, "  "+stTenue.Sprint("todavía no hay avisos"))
		return
	}
	fmt.Fprintln(w)
	ancho := Ancho() - 4
	for _, a := range avisos {
		// Cada aviso envuelto a su ancho, con sangría: en 50 columnas una
		// frase larga sin esto se parte a media palabra.
		for i, l := range envolver(a, ancho) {
			if i == 0 {
				fmt.Fprintln(w, "  "+stTenue.Sprint(l))
				continue
			}
			fmt.Fprintln(w, "    "+stTenue.Sprint(l))
		}
	}
	fmt.Fprintln(w)
}

// ─── el panel que se repinta en su sitio ─────────────────────────────────

// Panel dibuja un bloque de líneas fijas y lo refresca sin dejar restos.
type Panel struct {
	w       io.Writer
	foto    func() []string
	extra   func() []string // líneas de abajo (últimos avisos)
	mu      sync.Mutex
	lineas  []string
	alto    int
	visible bool
	parar   chan struct{}
	hecho   chan struct{}
}

// NuevoPanel prepara el panel (no lo dibuja todavía).
func NuevoPanel(w io.Writer, foto func() []string, extra func() []string) *Panel {
	// Guarda contra dos escritores a la vez: el tick y la tecla `e`.
	return &Panel{w: &escritorSeguro{w: w}, foto: foto, extra: extra}
}

// escritorSeguro serializa los Write del panel (tick, teclas, logs).
type escritorSeguro struct {
	mu sync.Mutex
	w  io.Writer
}

func (e *escritorSeguro) Write(p []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.w.Write(p)
}

// Arrancar lo dibuja y lo refresca cada `cada` hasta Parar.
func (p *Panel) Arrancar(cada time.Duration) {
	if !coloresOn {
		return
	}
	p.mu.Lock()
	p.visible = true
	p.parar = make(chan struct{})
	p.hecho = make(chan struct{})
	p.mu.Unlock()

	p.repintar()
	// Los canales se capturan ANTES de lanzar el goroutine: Parar() los pone
	// a nil y leerlos desde dentro sería una carrera (el ticker podría
	// quedarse esperando para siempre sobre un canal cerrado a medias).
	parar, hecho := p.parar, p.hecho
	go func() {
		defer close(hecho)
		t := time.NewTicker(cada)
		defer t.Stop()
		for {
			select {
			case <-parar:
				return
			case <-t.C:
				p.repintar()
			}
		}
	}()
}

// Repintar fuerza un refresco (lo llama el teletipo al llegar un log).
func (p *Panel) Repintar() { p.repintar() }

// Ocultar borra el panel de la pantalla.
func (p *Panel) Ocultar() {
	p.mu.Lock()
	p.visible = false
	p.borrar()
	p.mu.Unlock()
}

// Mostrar vuelve a dibujarlo.
func (p *Panel) Mostrar() {
	p.mu.Lock()
	p.visible = true
	p.alto = 0
	p.mu.Unlock()
	p.repintar()
}

// Parar apaga el refresco. NO borra el bloque: al salir lo que estaba en
// pantalla queda como último estado (y así el mensaje de cierre, que se
// imprime justo debajo, no se borra a sí mismo).
func (p *Panel) Parar() {
	p.mu.Lock()
	parar, hecho := p.parar, p.hecho
	p.parar, p.hecho = nil, nil
	p.mu.Unlock()
	if parar != nil {
		close(parar)
		<-hecho
	}
	p.mu.Lock()
	p.visible = false
	p.alto = 0
	p.mu.Unlock()
	if coloresOn {
		fmt.Fprint(p.w, "\r\n")
	}
}

// borrar quita el bloque actual (con el candado tomado).
func (p *Panel) borrar() {
	if p.alto == 0 || !coloresOn {
		return
	}
	fmt.Fprintf(p.w, "\033[%dA\r\033[J", p.alto)
	p.alto = 0
}

func (p *Panel) repintar() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.visible || !coloresOn {
		return
	}
	ancho := Ancho() - 1
	if ancho < 20 {
		ancho = 20
	}
	lineas := make([]string, 0, 20)
	lineas = append(lineas, strings.Split(strings.Join(p.foto(), "\n"), "\n")...)
	if p.extra != nil {
		extras := p.extra()
		if len(extras) > 0 {
			lineas = append(lineas, stTenue.Sprint(strings.Repeat("┈", minInt(ancho-2, 40))))
			lineas = append(lineas, extras...)
		}
	}
	// Ayuda al pie, en versión corta si la pantalla es de móvil: nunca se
	// corta a media palabra porque se elige por ancho.
	ayuda := "e ocultar · s resumen · p comandos · l avisos · q salir"
	if ancho < 60 {
		ayuda = "e panel · s estado · p cmd · l log · q salir"
	}
	lineas = append(lineas, stTenue.Sprint(ayuda))
	for i, l := range lineas {
		// Dos espacios de aire a la izquierda; el ancho útil descuenta esos 2.
		lineas[i] = "  " + truncarANSI(l, ancho-2)
	}

	// Subir al principio del bloque anterior y borrar de ahí abajo: así un
	// bloque que encoge tampoco deja restos.
	p.borrar()
	for _, l := range lineas {
		// En modo crudo (raw) \n no vuelve al principio de la línea: con \r\n
		// el bloque se apila bien también con las teclas activas.
		fmt.Fprint(p.w, "\r\033[K"+l+"\r\n")
	}
	p.alto = len(lineas)
	p.lineas = lineas
	// El cursor se queda en la línea siguiente al bloque, NO encima: al
	// repintar se sube `alto` y se borra desde ahí. Si el cursor viviera en la
	// primera fila, el repintado borraría también la línea de arriba (un log
	// recién escrito) y el usuario lo vería parpadear/desaparecer.
	fmt.Fprint(p.w, "\r")
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ─── atajos de teclado ───────────────────────────────────────────────────

// Atajos escucha una tecla a la vez mientras el bot corre.
//
// En un servicio, en un tubo o sin tty no se enciende: el bot sigue igual y
// nadie queda leyendo stdin (que ahí es basura).
func Atajos(ctx context.Context, acciones map[string]func()) func() {
	tty := abrirTTY()
	if tty == nil || !term.IsTerminal(int(tty.Fd())) {
		return func() {}
	}
	restaurar, err := modoTeclas(int(tty.Fd()))
	if err != nil {
		return func() {}
	}
	// El mismo lector que usó el menú y el número: crear otro se comería las
	// teclas que ya estuvieran en el búfer (pegar número + pulsar e).
	lector := readerDe(tty)
	fin := make(chan struct{})
	go func() {
		defer close(fin)
		for {
			if ctx.Err() != nil {
				return
			}
			_ = tty.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
			var b [1]byte
			n, err := lector.Read(b[:])
			if err != nil {
				return // terminal cerrado o error: aquí se acabaron los atajos
			}
			if n == 0 {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			tecla := string(b[:])
			if tecla == "\x03" {
				tecla = "ctrl-c" // en crudo es un byte; quien llama lo mapea
			}
			if fn, ok := acciones[tecla]; ok {
				fn()
			}
		}
	}()

	// Devolver el terminal a su estado normal SIEMPRE, aunque la lectura no
	// termine: si el usuario paró el bot con Ctrl+C y aquí se espera a que
	// alguien pulse una tecla, el proceso se queda vivo para siempre (y con
	// el terminal del usuario en modo crudo, que es lo peor).
	return func() {
		restaurar()
		if tty != os.Stdin {
			olvidarLector(tty)
			_ = tty.Close() // despierta la lectura bloqueada
		}
		select {
		case <-fin:
		case <-time.After(time.Second):
		}
	}
}

// Resumen: una sola línea para la tecla `s`.
func Resumen(e Estado) string {
	conexion := "desconectado"
	if e.Conectado {
		conexion = "conectado"
	}
	return fmt.Sprintf("%s · %s · %s encendido · cupo %d/%d · cola %d",
		e.Cuenta, conexion, duracion(e.Uptime), e.Enviados, e.Cupo, e.EnCola)
}

// EsTerminal: si el usuario puede ver el panel.
func EsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) && coloresOn
}
