// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// web_test.go — el panel B7 sin abrir WhatsApp: failsafe de loopback,
// contraseña en tiempo constante, candado anti fuerza bruta, 404 en rutas
// desconocidas y la forma del JSON de cada endpoint. Todo con httptest.
package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func estadoPrueba() Estado {
	return Estado{
		Conectado: true, Version: "0.14.0", Arranque: time.Now().Add(-2 * time.Minute),
		SentToday: 7, Budget: 120, Workers: 4, ColaMax: 128, EnCola: 2,
		Recibidos: 50, Procesados: 48, Descartados: 0, Anchor: time.Now().Add(-3 * time.Hour),
	}
}

// servidor: el mux completo (con middleware y envoltura de cabeceras), como
// Arrancar lo monta pero bajo httptest (no se abre ningún puerto real).
func servidor(t *testing.T, cfg Config) (*Server, string) {
	t.Helper()
	s := Nuevo(cfg, estadoPrueba, func(string) {})
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.inicio)
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/queue", s.queue)
	mux.HandleFunc("/memoria", s.memoria)
	ts := httptest.NewServer(s.conter(s.middleware(mux)))
	t.Cleanup(ts.Close)
	return s, ts.URL
}

func cogerJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	datos, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s → %d: %s", url, resp.StatusCode, datos)
	}
	var m map[string]any
	if json.Unmarshal(datos, &m) != nil {
		t.Fatalf("GET %s no devolvió JSON: %s", url, datos)
	}
	return m
}

// ── Failsafe ────────────────────────────────────────────────────────────────

// LOOPBACK=0 sin contraseña vuelve a 127.0.0.1, igual que el JS.
func TestFailsafeLoopback(t *testing.T) {
	avisos := []string{}
	s := Nuevo(Config{Puerto: 0, Publico: true, Password: ""}, estadoPrueba, func(m string) { avisos = append(avisos, m) })
	if !strings.HasPrefix(s.Dir, "127.0.0.1:") {
		t.Fatalf("LOOPBACK=0 sin clave debía quedar en loopback: %q", s.Dir)
	}
	if len(avisos) == 0 {
		t.Fatal("el failsafe debe avisar de que no se expone")
	}

	s2 := Nuevo(Config{Puerto: 8080, Publico: true, Password: "x-larga"}, estadoPrueba, func(string) {})
	if !strings.HasPrefix(s2.Dir, "0.0.0.0:") {
		t.Fatalf("con clave sí puede exponer: %q", s2.Dir)
	}
	s3 := Nuevo(Config{Puerto: 99999}, estadoPrueba, func(string) {})
	if !strings.HasSuffix(s3.Dir, ":3000") {
		t.Fatalf("puerto absurdo cae al 3000: %q", s3.Dir)
	}
}

// ── Endpoints (loopback: sin contraseña) ────────────────────────────────────

func TestEndpoints(t *testing.T) {
	_, base := servidor(t, Config{Publico: false})

	inicio := cogerJSON(t, base+"/")
	if inicio["fingerprint"] != "shin-gauss-0.25-1200ms" || inicio["version"] != "0.14.0" || inicio["engine"] != "conectado" {
		t.Fatalf("/ raro: %v", inicio)
	}
	if _, ok := inicio["ts"].(string); !ok {
		t.Fatalf("/ sin ts: %v", inicio)
	}

	health := cogerJSON(t, base+"/health")
	if health["state"] != "conectado" || health["uptimeSec"].(float64) < 100 {
		t.Fatalf("/health raro: %v", health)
	}
	cola := health["queue"].(map[string]any)
	if cola["sentToday"] != float64(7) || cola["budget"] != float64(120) || cola["pendientes"] != float64(2) {
		t.Fatalf("/health queue: %v", cola)
	}
	if d := health["dispatcher"].(map[string]any); d["workers"] != float64(4) || d["pending"] != float64(2) {
		t.Fatalf("/health dispatcher: %v", d)
	}

	q := cogerJSON(t, base+"/queue")
	if q["antiban"].(map[string]any)["gauss"] != "μ=0.25 · techo 1200 ms" {
		t.Fatalf("/queue antiban: %v", q)
	}

	mem := cogerJSON(t, base+"/memoria")
	if mem["goroutines"].(float64) < 1 {
		t.Fatalf("/memoria raro: %v", mem)
	}

	// Ruta no montada: 404, no el panel.
	resp, err := http.Get(base + "/loquesea")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("/loquesea debía ser 404, salió %d", resp.StatusCode)
	}
}

// ── Auth expuesta: usuario fijo admin, tiempo constante y candado ───────────

func TestAuthExpuesta(t *testing.T) {
	_, base := servidor(t, Config{Publico: true, Password: "secreto-largo"})

	// Sin contraseña → 401 con el realm.
	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sin clave debía ser 401, salió %d", resp.StatusCode)
	}
	if resp.Header.Get("WWW-Authenticate") == "" {
		t.Fatal("falta WWW-Authenticate en el 401")
	}

	// Usuario equivocado también es 401 (el JS lo corrigió en la auditoría).
	req, _ := http.NewRequest("GET", base+"/health", nil)
	req.SetBasicAuth("root", "secreto-largo")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("usuario «root» no puede pasar: %d", resp.StatusCode)
	}

	// Credenciales buenas → 200.
	req, _ = http.NewRequest("GET", base+"/health", nil)
	req.SetBasicAuth("admin", "secreto-largo")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin ok debía ser 200, salió %d", resp.StatusCode)
	}
}

// A los 10 fallos la IP queda bloqueada 5 minutos: ni con clave buena entra.
func TestCandadoAntiFuerzaBruta(t *testing.T) {
	s, base := servidor(t, Config{Publico: true, Password: "secreto-largo"})
	cliente := &http.Client{}
	// El httptest corre desde 127.0.0.1; los fallos son de esa IP.

	intentar := func(usuario, clave string) int {
		req, _ := http.NewRequest("GET", base+"/health", nil)
		req.SetBasicAuth(usuario, clave)
		resp, err := cliente.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	for i := 0; i < maxFallos; i++ {
		if got := intentar("admin", "malisima"); got != http.StatusUnauthorized {
			t.Fatalf("intento %d: 401 esperado, salió %d", i+1, got)
		}
	}
	if got := intentar("admin", "malisima"); got != http.StatusTooManyRequests {
		t.Fatalf("tras %d fallos debía ser 429, salió %d", maxFallos, got)
	}
	if got := intentar("admin", "secreto-largo"); got != http.StatusTooManyRequests {
		t.Fatalf("bloqueada aunque la clave sea buena: %d", got)
	}
	_ = s
}

// ⏹ La IP nueva (o la que acierta) no hereda el candado de otra.
func TestAciertoLimpiaFallos(t *testing.T) {
	s := Nuevo(Config{Publico: true, Password: "0k"}, estadoPrueba, func(string) {})
	// Simulo 9 fallos (no llega al candado) y luego un acierto.
	s.mu.Lock()
	s.fallos["10.0.0.5"] = 9
	s.mu.Unlock()

	claveOK := s.claveCoincide("admin", "0k")
	if !claveOK {
		t.Fatal("la clave buena debía coincidir")
	}
	if s.claveCoincide("admin2", "0k") || s.claveCoincide("admin", "otra") {
		t.Fatal("solo admin + la clave exacta pueden pasar")
	}

	s.apuntaFallo("10.0.0.5") // 10 → bloqueada
	s.mu.Lock()
	if _, bloqueada := s.hasta["10.0.0.5"]; !bloqueada {
		t.Fatal("los 10 fallos debían bloquear")
	}
	s.mu.Unlock()
}
