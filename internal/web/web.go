// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// web.go — Panel HTTP local (bloque B7). Puerto de src/web/server.js:
//
//	/         ficha del bot (nombre, versión, huella anti-ban, estado)
//	/health   salud: conexión, uptime, cola, repartidor, RAM
//	/queue    la cola anti-ban y el calentamiento en detalle
//	/memoria  memoria del proceso y del runtime
//	/metrics  proceso + SO + motor, para graphenar frío
//
// REGLAS DE LA CASA (las mismas que el JS):
//
//  1. Solo escucha en 127.0.0.1 salvo LOOPBACK=0.
//  2. FAILSAFE: LOOPBACK=0 sin PANEL_PASSWORD → vuelve a 127.0.0.1 aunque lo
//     hayas pedido. Riesgo/cola/memoria no se enseñan a la red entera.
//  3. Panel expuesto ⇒ Basic auth con usuario fijo «admin» y contraseña
//     comparada por SHA-256 en tiempo constante (la longitud no se filtra).
//  4. Candado anti fuerza bruta: 10 fallos por IP → 429 durante 5 minutos.
package web

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Huella del motor anti-ban: otra cosa que responda distinto no es este bot.
const Huella = "shin-gauss-0.25-1200ms"

// Estado es la foto que leen los endpoints. La rellena main con un cierre
// sobre el cliente, la cola y el repartidor: este paquete no importa wa/bot
// para no liar un ciclo de dependencias.
type Estado struct {
	Conectado   bool   // hay sesión viva con WhatsApp
	Version     string // versión del binario
	Arranque    time.Time
	SentToday   int    // envíos no prioritarios de hoy
	Budget      int    // cupo de hoy según el calentamiento
	ModoLento   bool   // cupo agotado: el bot duplica esperas
	Anchor      string // día del primer arranque (calentamiento)
	Workers     int    // mensajes atendidos a la vez
	EnCola      int    // mensajes esperando turno en el repartidor
	ColaMax     int    // techo de esa espera
	Recibidos   uint64 // mensajes entrados desde el arranque
	Procesados  uint64
	Descartados uint64 // los que no cupieron en la cola
}

// Config del panel.
type Config struct {
	Puerto   int    // PORT (defecto 3000)
	Publico  bool   // LOOPBACK=0 en el .env
	Password string // PANEL_PASSWORD
}

const (
	panelUser  = "admin"
	maxFallos  = 10
	ventana429 = 5 * time.Minute
)

// Server es el panel: un net/http pelado, sin frameworks.
type Server struct {
	cfg    Config
	estado func() Estado
	avisar func(string) // mensajes de arranque (xlog.Warn/Info)

	srv *http.Server
	// Dir es la dirección final tras el failsafe (la que hay que enseñar
	// al usuario). La ponen Arrancar/Listen y la miran las pruebas.
	Dir string

	mu     sync.Mutex
	fallos map[string]*falloIP
	// VentanaFallos es variable para que las pruebas no esperen 5 minutos.
	VentanaFallos time.Duration
}

type falloIP struct {
	veces int
	desde time.Time
}

// Nuevo crea el panel con su handlers. estado se llama en cada petición.
func Nuevo(cfg Config, estado func() Estado, avisar func(string)) *Server {
	s := &Server{
		cfg:           cfg,
		estado:        estado,
		avisar:        avisar,
		fallos:        map[string]*falloIP{},
		VentanaFallos: ventana429,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.envolver(s.inicio))
	mux.HandleFunc("/health", s.envolver(s.health))
	mux.HandleFunc("/queue", s.envolver(s.queue))
	mux.HandleFunc("/memoria", s.envolver(s.memoria))
	mux.HandleFunc("/metrics", s.envolver(s.metrics))

	host := "127.0.0.1"
	if cfg.Publico {
		if cfg.Password == "" {
			// FAILSAFE (igual que el JS): exponer sin contraseña no se permite.
			s.avisar("Panel: LOOPBACK=0 sin PANEL_PASSWORD — por seguridad solo escuchará en 127.0.0.1")
		} else {
			host = "0.0.0.0"
		}
	}
	if cfg.Puerto <= 0 || cfg.Puerto > 65535 {
		cfg.Puerto = 3000
	}
	s.cfg = cfg
	s.Dir = net.JoinHostPort(host, strconv.Itoa(cfg.Puerto))
	s.srv = &http.Server{
		Addr:              s.Dir,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	return s
}

// Arrancar levanta el panel y lo cierra solo cuando muere el contexto.
// Un puerto ocupado se avisa pero NO tumba el bot: el panel es un extra.
func (s *Server) Arrancar(ctx context.Context) {
	ln, err := net.Listen("tcp", s.Dir)
	if err != nil {
		s.avisar("Panel web no pudo escuchar en " + s.Dir + ": " + err.Error())
		return
	}
	s.avisar("Panel web en http://" + s.Dir + "  (/, /health, /queue, /memoria, /metrics)")
	go func() {
		<-ctx.Done()
		cierre, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(cierre)
	}()
	go func() { _ = s.srv.Serve(ln) }()
}

// ── Auth ────────────────────────────────────────────────────────────────────

// envolver aplica la contraseña cuando el panel está expuesto (en loopback no
// hace falta: solo la ve quien ya está DENTRO del servidor).
func (s *Server) envolver(h http.HandlerFunc) http.HandlerFunc {
	expuesto := strings.HasPrefix(s.Dir, "0.0.0.0")
	if !expuesto {
		return h
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if i := strings.LastIndexByte(ip, ':'); i >= 0 {
			ip = ip[:i]
		}
		if s.bloqueada(ip) {
			w.Header().Set("Retry-After", strconv.Itoa(int(s.VentanaFallos.Seconds())))
			s.responder(w, http.StatusTooManyRequests, map[string]any{
				"ok": false, "error": "Demasiados intentos fallidos. Espera 5 minutos.",
			})
			return
		}
		user, pass, ok := r.BasicAuth()
		if !ok || !segurasIguales(user, panelUser) || !segurasIguales(pass, s.cfg.Password) {
			s.anotarFallo(ip)
			w.Header().Set("WWW-Authenticate", `Basic realm="Shin-MD"`)
			s.responder(w, http.StatusUnauthorized, map[string]any{
				"ok": false, "error": "Requiere PANEL_PASSWORD (usuario: admin)",
			})
			return
		}
		s.olvidarFallos(ip)
		h(w, r)
	}
}

// segurasIguales compara sin revelar la longitud: se comparan los SHA-256,
// que siempre miden 32 bytes (truco del JS, mismo motivo).
func segurasIguales(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

func (s *Server) bloqueada(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.fallos[ip]
	if !ok {
		return false
	}
	if time.Since(f.desde) > s.VentanaFallos {
		delete(s.fallos, ip)
		return false
	}
	return f.veces >= maxFallos
}

func (s *Server) anotarFallo(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.fallos[ip]
	if !ok || time.Since(f.desde) > s.VentanaFallos {
		s.fallos[ip] = &falloIP{veces: 1, desde: time.Now()}
		return
	}
	f.veces++
}

func (s *Server) olvidarFallos(ip string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.fallos, ip)
}

// ── Endpoints ───────────────────────────────────────────────────────────────

func (s *Server) responder(w http.ResponseWriter, codigo int, datos any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(codigo)
	_ = json.NewEncoder(w).Encode(datos)
}

func (s *Server) inicio(w http.ResponseWriter, r *http.Request) {
	// El patrón "/" del mux caza TODO lo no registrado: una ruta equivocada
	// debe ser 404, no la ficha del bot (el JS con express hacía lo mismo).
	if r.URL.Path != "/" {
		s.responder(w, http.StatusNotFound, map[string]bool{"ok": false})
		return
	}
	e := s.estado()
	s.responder(w, http.StatusOK, map[string]any{
		"ok":          true,
		"name":        "Shin-MD",
		"version":     e.Version,
		"fingerprint": Huella,
		"engine":      estadoTexto(e.Conectado),
		"ts":          time.Now().UnixMilli(),
	})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	e := s.estado()
	s.responder(w, http.StatusOK, map[string]any{
		"ok":        true,
		"state":     estadoTexto(e.Conectado),
		"uptimeSec": int64(time.Since(e.Arranque).Seconds()),
		"queue": map[string]any{
			"sentToday":   e.SentToday,
			"budget":      e.Budget,
			"slowMode":    e.ModoLento,
			"warmupSince": e.Anchor,
		},
		"dispatcher": map[string]any{
			"workers": e.Workers, "pending": e.EnCola, "maxPending": e.ColaMax,
			"received": e.Recibidos, "processed": e.Procesados, "dropped": e.Descartados,
		},
		"memoryMB": rssMB(),
	})
}

func (s *Server) queue(w http.ResponseWriter, _ *http.Request) {
	e := s.estado()
	s.responder(w, http.StatusOK, map[string]any{
		"ok": true,
		"antiban": map[string]any{
			"sentToday": e.SentToday, "budget": e.Budget,
			"slowMode": e.ModoLento, "warmupSince": e.Anchor,
			"regla": "1200 ms base · σ0.25 · +25 ms/carácter · 20→500 en 7 días",
		},
		"dispatcher": map[string]any{
			"workers": e.Workers, "pending": e.EnCola, "maxPending": e.ColaMax,
			"received": e.Recibidos, "processed": e.Procesados, "dropped": e.Descartados,
		},
	})
}

func (s *Server) memoria(w http.ResponseWriter, _ *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.responder(w, http.StatusOK, map[string]any{
		"ok":          true,
		"rssMB":       rssMB(),
		"heapAllocMB": redondearMB(m.HeapAlloc),
		"heapSysMB":   redondearMB(m.HeapSys),
		"stackSysMB":  redondearMB(m.StackSys),
		"numGC":       m.NumGC,
		"goroutines":  runtime.NumGoroutine(),
		"gomaxprocs":  runtime.GOMAXPROCS(0),
	})
}

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	e := s.estado()
	s.responder(w, http.StatusOK, map[string]any{
		"ok":           true,
		"shin_version": e.Version,
		"fingerprint":  Huella,
		"process": map[string]any{
			"pid":       os.Getpid(),
			"uptimeSec": int64(time.Since(e.Arranque).Seconds()),
			"go":        runtime.Version(),
			"platform":  runtime.GOOS + "/" + runtime.GOARCH,
		},
		"os": map[string]any{
			"cpus": runtime.NumCPU(),
		},
		"engine": map[string]any{"state": estadoTexto(e.Conectado)},
	})
}

// ── Atajos ──────────────────────────────────────────────────────────────────

func estadoTexto(conectado bool) string {
	if conectado {
		return "conectado"
	}
	return "desconectado"
}

func redondearMB(bytes uint64) float64 {
	v := float64(bytes) / 1048576
	return float64(int(v*10)) / 10
}

// rssMB lee la RSS real del proceso (en Linux, de /proc; si no se puede, 0).
func rssMB() float64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if runtime.GOOS != "linux" {
		return redondearMB(m.Sys)
	}
	datos, err := os.ReadFile("/proc/self/stat")
	if err != nil {
		return redondearMB(m.Sys)
	}
	// El campo 24 es la RSS en páginas. El nombre del proceso va entre
	// paréntesis y PUEDE llevar espacios: se busca tras el último ')'.
	campos := strings.Fields(string(datos[strings.LastIndexByte(string(datos), ')')+1:]))
	if len(campos) < 22 {
		return redondearMB(m.Sys)
	}
	paginas, err := strconv.ParseInt(campos[21], 10, 64)
	if err != nil {
		return redondearMB(m.Sys)
	}
	return redondearMB(uint64(paginas) * uint64(os.Getpagesize()))
}
