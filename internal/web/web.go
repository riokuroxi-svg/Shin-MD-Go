// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// web.go — panel web del bot: puerto de src/web/server.js.
// / → ficha | /health → conexión + cola + repartidor | /queue → antiban |
// /memoria → RSS, heap y goroutines | cualquier otra ruta es 404.
//
// Seguridad conservada del JS (y la reseña que lo auditó):
//   - escucha en 127.0.0.1 por defecto; solo con LOOPBACK=0 **y**
//     PANEL_PASSWORD puesta pasa a 0.0.0.0 — si falta la contraseña vuelve
//     solo a loopback (el riesgo, la cola y la memoria no se enseñan a la
//     red abierta).
//   - panel expuesto ⇒ Basic auth con usuario fijo "admin" y la clave
//     comparada por SHA-256 en tiempo constante (así no filtra la longitud
//     por temporización: siempre se comparan 32 bytes).
//   - candado anti fuerza bruta: 10 fallos seguidos desde una IP bloquean el
//     panel para ella durante 5 minutos (429 + Retry-After). Cada acierto
//     borra la cuenta; bloqueada, ni con clave buena entra.
//
// Net/http en vez de express: un panel con 5 rutas no necesita framework —
// el mux de la librería estándar basta y no suma dependencias.
package web

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Los tamaños del candado anti fuerza bruta (los mismos del JS).
const (
	maxFallos  = 10
	bloqueoMin = 5 * time.Minute
)

// Config es lo que el panel necesita de fuera.
type Config struct {
	Puerto   int    // PORT (defecto 3000)
	Publico  bool   // LOOPBACK=0
	Password string // PANEL_PASSWORD (obligatoria si Publico)
}

// Estado es la foto que el panel vende. La rellena el main con un callback
// para no importar internal/bot ni wa desde aquí (web no les tira).
type Estado struct {
	Conectado   bool
	Version     string
	Arranque    time.Time
	SentToday   int
	Budget      int
	ModoLento   bool
	Anchor      time.Time
	Workers     int
	EnCola      int
	ColaMax     int
	Recibidos   int64
	Procesados  int64
	Descartados int64
}

// Server es el panel. Log es opcional (nil → nada).
type Server struct {
	cfg    Config
	estado func() Estado
	log    func(string) // p.ej. log.Warn

	mu     sync.Mutex
	fallos map[string]int
	hasta  map[string]time.Time

	srv *http.Server
	Dir string // la dirección a la que ESCUCHA (127.0.0.1 o 0.0.0.0)
}

// Nuevo prepara el panel; si pide público sin contraseña, cae a loopback con
// un aviso (el failsafe del JS, respice doble: no se consuela configurando).
func Nuevo(cfg Config, estado func() Estado, log func(string)) *Server {
	if cfg.Puerto <= 0 || cfg.Puerto > 65535 {
		cfg.Puerto = 3000
	}
	if estado == nil {
		estado = func() Estado { return Estado{} }
	}
	if log == nil {
		log = func(string) {}
	}
	s := &Server{cfg: cfg, estado: estado, log: log, fallos: map[string]int{}, hasta: map[string]time.Time{}}

	ip := "127.0.0.1"
	if cfg.Publico && cfg.Password == "" {
		log(`[web] LOOPBACK=0 pero falta PANEL_PASSWORD; sigue en loopback. El panel expone riesgo, cola y memoria: sin contraseña no se abre.`)
		cfg.Publico = false
	}
	if cfg.Publico {
		ip = "0.0.0.0"
	}
	s.Dir = net.JoinHostPort(ip, strconv.Itoa(cfg.Puerto))
	return s
}

// Arrancar lanza el panel y se queda sirviendo hasta que ctx muere. Devuelve
// error solo si el puerto falla al abrirse (para que el main decida si
// avisar o morir); los cierres limpios no son error.
func (s *Server) Arrancar(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.inicio) // / → ficha; lo demás 404
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/queue", s.queue)
	mux.HandleFunc("/memoria", s.memoria)
	s.srv = &http.Server{Addr: s.Dir, Handler: s.conter(s.middleware(mux))}

	oyente, err := net.Listen("tcp", s.Dir)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		c2, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = s.srv.Shutdown(c2)
		cancel()
	}()
	go func() { _ = s.srv.Serve(oyente) }()
	return nil
}

// conter quita el ServerHeader de Go que delata la versión (el JS no
// mandaba; aquí tampoco).
func (s *Server) conter(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		h.ServeHTTP(w, r)
	})
}

// middleware: Basic auth solo si el panel está expuesto y hay contraseña.
// Si está en loopback, no hay contraseña que pedir (127.0.0.1 ya es local).
func (s *Server) middleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Publico && s.cfg.Password != "" {
			if !s.autorizado(w, r) {
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// autorizado aplica candado y Basic auth con comparación en tiempo constante.
// Devuelve true si la petición puede pasar.
func (s *Server) autorizado(w http.ResponseWriter, r *http.Request) bool {
	ip := ipDe(r)
	s.mu.Lock()
	if hasta, ok := s.hasta[ip]; ok && time.Now().Before(hasta) {
		restante := time.Until(hasta)
		s.mu.Unlock()
		w.Header().Set("Retry-After", strconv.Itoa(int(restante.Seconds())+1))
		http.Error(w, "429 Too Many Requests — panel bloqueado por intentos. Vuelve en "+restante.Round(time.Second).String(), http.StatusTooManyRequests)
		return false
	}
	s.mu.Unlock()

	u, p, ok := r.BasicAuth()
	if !ok || !s.claveCoincide(u, p) {
		s.apuntaFallo(ip)
		w.Header().Set("WWW-Authenticate", `Basic realm="shin-md-panel", charset="UTF-8"`)
		http.Error(w, "401 — se necesita acceso (usuario: admin)", http.StatusUnauthorized)
		return false
	}
	// Acierto: olvido los fallos de esa IP.
	s.mu.Lock()
	delete(s.fallos, ip)
	s.mu.Unlock()
	return true
}

// apuntaFallo: al llegar a 10, 5 minutos de 429 para esa IP.
func (s *Server) apuntaFallo(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.fallos[ip] + 1
	if n >= maxFallos {
		s.hasta[ip] = time.Now().Add(bloqueoMin)
		delete(s.fallos, ip)
		s.log(fmt.Sprintf("[web] fuerza bruta desde %s: panel bloqueado %s", ip, bloqueoMin))
		return
	}
	s.fallos[ip] = n
}

// claveCoincide: el usuario es siempre "admin" y la clave se compara tras
// SHA-256 (tiempo constante: el ataque de temporización no filtra la clave
// ni su longitud; siempre se comparan 32 bytes).
func (s *Server) claveCoincide(usuario, clave string) bool {
	if usuario != "admin" {
		return false
	}
	h1 := sha256.Sum256([]byte(clave))
	h2 := sha256.Sum256([]byte(s.cfg.Password))
	return subtle.ConstantTimeCompare(h1[:], h2[:]) == 1
}

// ── Endpoints ───────────────────────────────────────────────────────────────

// escribir: siempre JSON, nunca HTML (un panel de consola no necesita vistas).
func escribir(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// inicio: la ficha del bot. Cualquier otra ruta desde "/" es 404, como el
// express del JS ("/inexistente" no devuelve el panel adrede).
func (s *Server) inicio(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	e := s.estado()
	escribir(w, map[string]any{
		"ok":          true,
		"name":        "Shin-MD Go",
		"version":     e.Version,
		"fingerprint": "shin-gauss-0.25-1200ms",
		"engine":      estadoConn(e.Conectado),
		"ts":          time.Now().Format("2006-01-02 15:04:05"),
	})
}

// health: conexión, uptime, cola y repartidor.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	e := s.estado()
	escribir(w, map[string]any{
		"ok":        true,
		"state":     estadoConn(e.Conectado),
		"uptimeSec": int(time.Since(e.Arranque).Seconds()),
		"version":   e.Version,
		"queue": map[string]any{
			"sentToday":  e.SentToday,
			"budget":     e.Budget,
			"slowMode":   e.ModoLento,
			"anchorAge":  edad(e.Anchor),
			"pendientes": e.EnCola,
		},
		"dispatcher": map[string]any{
			"workers":   e.Workers,
			"colaCap":   e.ColaMax,
			"pending":   e.EnCola,
			"recibidos": e.Recibidos,
		},
		"memoryMB": rssMB(),
	})
}

// queue: la cola antiban en detalle.
func (s *Server) queue(w http.ResponseWriter, r *http.Request) {
	e := s.estado()
	escribir(w, map[string]any{
		"ok":          true,
		"queueLen":    e.EnCola,
		"queueMax":    e.ColaMax,
		"workers":     e.Workers,
		"recibidos":   e.Recibidos,
		"procesados":  e.Procesados,
		"descartados": e.Descartados,
		"antiban": map[string]any{
			"sentToday": e.SentToday,
			"budget":    e.Budget,
			"slowMode":  e.ModoLento,
			"anchorAge": edad(e.Anchor),
			"gauss":     "μ=0.25 · techo 1200 ms",
		},
	})
}

// memoria: RSS leído de /proc (el real, no el runtime) + heap + GC.
func (s *Server) memoria(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	escribir(w, map[string]any{
		"ok":          true,
		"rssMB":       rssMB(),
		"heapAllocMB": m.Alloc >> 20,
		"heapSysMB":   m.Sys >> 20,
		"numGC":       m.NumGC,
		"goroutines":  runtime.NumGoroutine(),
		"goVersion":   runtime.Version(),
		"numCPU":      runtime.NumCPU(),
	})
}

// metrics: los mismos números en forma plana para un cURL/graficador externo.
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	e := s.estado()
	escribir(w, map[string]any{
		"ok":          true,
		"uptimeSec":   int(time.Since(e.Arranque).Seconds()),
		"queueSent":   e.SentToday,
		"queuePend":   e.EnCola,
		"descartados": e.Descartados,
		"rssMB":       rssMB(),
		"goroutines":  runtime.NumGoroutine(),
		"goVersion":   runtime.Version(),
		"state":       estadoConn(e.Conectado),
	})
}

// ── pelo ───────────────────────────────────────────────────────────────────

// ipDe saca la IP sin puerto (XF ware no: detrás de proxy no vale — el panel
// solo debe exponerse directo. Si no se puede, se usa la cruda).
func ipDe(r *http.Request) string {
	remoto := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoto); err == nil {
		return host
	}
	return remoto
}

func estadoConn(ok bool) string {
	if ok {
		return "conectado"
	}
	return "reconectando"
}

// edad: legible para el ancla antiban ("hace 12 min" defrauda menos que un
// timestamp crudo cuando la revisas desde el teléfono).
func edad(t time.Time) string {
	if t.IsZero() {
		return "0 s"
	}
	d := time.Since(t).Round(time.Second)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("hace %ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("hace %dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("hace %dh %dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// rssMB: la memoria REAL del proceso, leída del kernel — lo mismo que /health
// del JS enseñaba. En Linux es /proc/self/stat (el campo 24, en páginas y
// hay que saltarlo el nombre entre paréntesis); fuera de Linux vuelve al
// runtime, que pinta menos pero no miente con ceros.
func rssMB() uint64 {
	if raw, err := osReadProc(); err == nil {
		return raw
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Sys >> 20
}

// osReadProc abre /proc/self/stat aparte (para poder probarlo y no confundir
// el formato: el campo 2 puede tener espacios dentro de paréntesis).
var osReadProc = func() (uint64, error) {
	b, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return 0, err
	}
	linea := string(b)
	fin := strings.LastIndex(linea, ")")
	if fin < 0 {
		return 0, fmt.Errorf("formato raro")
	}
	campos := strings.Fields(linea[fin+2:]) // tras ") " quedan los campos 3..
	if len(campos) < 22 {                   // (3=state … 24=rss, aquí índice 21)
		return 0, fmt.Errorf("pocos campos")
	}
	rssPaginas, err := strconv.ParseUint(campos[21], 10, 64)
	if err != nil {
		return 0, err
	}
	return (rssPaginas * uint64(os.Getpagesize())) >> 20, nil
}
