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
	if !cortado && !strings.ContainsRune(s, 0x1b) {
		return s // ni se cortó ni había color: devolver tal cual, sin reset suelto
	}
	b.WriteString("\033[0m") // cerrar lo que haya quedado abierto por el corte
	return b.String()
}

// Duracion: «1h 04m» o «1m 48s». Exportada para que «shin status» desde fuera
// use exactamente el mismo formato que el panel, no otro parecido.
func Duracion(d time.Duration) string { return duracion(d) }

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
	mu         sync.Mutex
	Max        int
	cola       []string
	desplazado int // líneas hacia atrás que pide el usuario (RePág)
}

// Anadir apunta un aviso con la hora.
func (h *Historial) Anadir(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.Max <= 0 {
		h.Max = 40
	}
	// La línea se guarda TAL CUAL la pintó Prefijo, con su hora dentro: si se le
	// pusiera otra aquí, el panel saldría con dos relojes en cada línea (el
	// fallo que se veía en Termux: «22:35 ◐ 22:35:14 …»).
	h.cola = append(h.cola, msg)
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

// Ventana: las n líneas que hay que enseñar ahora mismo, respetando cuánto se
// haya retrocedido. Se usa desde la pantalla; al llegar un aviso nuevo no se
// mueve nada (quien está leyendo hacia atrás no pierde su sitio).
func (h *Historial) Ventana(n int) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	if n <= 0 || len(h.cola) == 0 {
		return nil
	}
	final := len(h.cola) - h.desplazado
	if final < 0 {
		final = 0
	}
	inicio := final - n
	if inicio < 0 {
		inicio = 0
	}
	out := make([]string, final-inicio)
	copy(out, h.cola[inicio:final])
	return out
}

// Atras/Adelante: Retroceder y Avanzar sobre el registro.
func (h *Historial) Atras(n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if tope := len(h.cola) - 1; h.desplazado+n > tope {
		h.desplazado = tope
		if h.desplazado < 0 {
			h.desplazado = 0
		}
		return
	}
	h.desplazado += n
}

func (h *Historial) Adelante(n int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.desplazado -= n; h.desplazado < 0 {
		h.desplazado = 0
	}
}

// Abajo: el aviso más reciente vuelve a estar a la vista (lo hace el propio
// panel al repintar cuando el usuario pulsa v, o al llegar algo nuevo).
func (h *Historial) Abajo() { h.mu.Lock(); h.desplazado = 0; h.mu.Unlock() }

// Viejos: si el usuario está releyendo, no se le interrumpe.
func (h *Historial) Releyendo() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.desplazado > 0
}

// ImprimirHistorial pinta los últimos avisos, o dice que no hay.
func ImprimirHistorial(w io.Writer, avisos []string) {
	if len(avisos) == 0 {
		fmt.Fprintln(w, "  "+stTenue.Sprint("todavía no hay avisos"))
		return
	}
	fmt.Fprintln(w)
	// Se deja aire de sobra (6, no 4): el sangrado son 2 columnas y hay glifos
	// (◐ ● !) que algunas fuentes miden como dobles — si se envuelve justo al
	// borde, el terminal parte la línea por su cuenta a media palabra.
	ancho := Ancho() - 6
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

// ─── la cabecera del estado ──────────────────────────────────────────────

// LineasEstado: las filas del estado, «etiqueta … valor», sin bordes y sin
// barras de bloque. Los ▓░ se veían macizos con la fuente de Termux y
// descuadraban el ancho; aquí el dato va alineado a la derecha y se lee igual
// en 40 que en 100 columnas.
func LineasEstado(e Estado, ancho int) []string {
	ancho = encajar(ancho, 40, AnchoCasa)
	conexion := stRojo.Sprint("○ desconectado")
	if e.Conectado {
		conexion = stMenta.Sprint("● conectado")
	}
	filas := []string{
		Fila("conexión", conexion, ancho),
		Fila("encendido", duracion(e.Uptime), ancho),
	}
	if e.Cuenta != "" {
		filas = append(filas, Fila("cuenta", e.Cuenta, ancho))
	}
	if e.Comandos > 0 {
		pf := "prefijos"
		if len(e.Prefijos) == 1 {
			pf = "prefijo"
		}
		filas = append(filas, Fila("comandos",
			fmt.Sprintf("%d · %s [%s]", e.Comandos, pf, strings.Join(e.Prefijos, " ")), ancho))
	}
	if e.Cupo > 0 {
		filas = append(filas, Fila("cupo de hoy",
			fmt.Sprintf("%d/%d no prioritarios", e.Enviados, e.Cupo), ancho))
	}
	filas = append(filas,
		Fila("mensajes", fmt.Sprintf("%d recibidos · %d respondidos", e.Recibidos, e.Procesados), ancho),
		Fila("cola", fmt.Sprintf("%d en espera", e.EnCola), ancho))
	du, dt, ru, rt := vitales()
	if dt > 0 {
		filas = append(filas, Fila("disco", fmt.Sprintf("%s de %s", Humanizar(du), Humanizar(dt)), ancho))
	}
	if rt > 0 {
		filas = append(filas, Fila("memoria", fmt.Sprintf("%s/%s", Humanizar(ru), Humanizar(rt)), ancho))
	}
	if e.Web != "" {
		filas = append(filas, Fila("panel web", e.Web, ancho))
	}
	// Por si el terminal es más estrecho que 58: nada debe partirse.
	for i, l := range filas {
		filas[i] = truncarANSI(l, ancho)
	}
	return filas
}

// Cabecera: el estado enmarcado, listo para ir arriba de la pantalla.
func Cabecera(e Estado) []string {
	ancho := AnchoFijo()
	titulo := stSakuraFuerte.Sprint("shin") + stTenue.Sprint(" · ") +
		stPapel.Sprint("v"+e.Version) + stTenue.Sprint(" · en línea")
	return Marco(titulo, LineasEstado(e, ancho), ancho)
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
