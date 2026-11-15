// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// web_test.go — el panel B7 se prueba sin abrir WhatsApp: failsafe de
// loopback, contraseña en tiempo constante, candado anti fuerza bruta y la
// forma del JSON de cada endpoint.
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
		Recibidos: 50, Procesados: 48, Descartados: 0,
	}
}

func panelPrueba(t *testing.T, cfg Config) (*Server, string) {
	t.Helper()
	s := Nuevo(cfg, estadoPrueba, func(string) {})
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.inicio)
	mux.HandleFunc("/health", s.health)
	mux.HandleFunc("/queue", s.queue)
	mux.HandleFunc("/memoria", s.memoria)
	if strings.HasPrefix(s.Dir, "0.0.0.0") {
		// En modo expuesto las rutas pasan por la contraseña.
		mux2 := http.NewServeMux()
		envuelto := func(h http.HandlerFunc) http.HandlerFunc { return s.envolver(h) }
		mux2.HandleFunc("/", envuelto(s.inicio))
		mux2.HandleFunc("/health", envuelto(s.health))
		mux2.HandleFunc("/queue", envuelto(s.queue))
		mux2.HandleFunc("/memoria", envuelto(s.memoria))
		srv := httptest.NewServer(mux2)
		t.Cleanup(srv.Close)
		return s, srv.URL
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return s, srv.URL
}

func cogerJSON(t *testing.T, url string) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
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

// Pedir panel público SIN contraseña vuelve a loopback, igual que el JS.
func TestFailsafeLoopback(t *testing.T) {
	s := Nuevo(Config{Puerto: 0, Publico: true, Password: ""}, estadoPrueba, func(string) {})
	if !strings.HasPrefix(s.Dir, "127.0.0.1:") {
		t.Fatalf("LOOPBACK=0 sin contraseña debía quedar en loopback: %q", s.Dir)
	}
	s2 := Nuevo(Config{Puerto: 8080, Publico: true, Password: "x"}, estadoPrueba, func(string) {})
	if !strings.HasPrefix(s2.Dir, "0.0.0.0:") {
		t.Fatalf("con contraseña sí puede exponerse: %q", s2.Dir)
	}
	if s3 := Nuevo(Config{Puerto: 99999}, estadoPrueba, func(string) {}); !strings.HasSuffix(s3.Dir, ":3000") {
		t.Fatalf("puerto absurdo cae al 3000: %q", s3.Dir)
	}
}

// ── Endpoints (loopback, sin contraseña) ────────────────────────────────────

func TestEndpoints(t *testing.T) {
	_, base := panelPrueba(t, Config{Publico: false})

	inicio := cogerJSON(t, base+"/")
	if inicio["fingerprint"] != Huella || inicio["version"] != "0.14.0" || inicio["engine"] != "conectado" {
		t.Fatalf("/ raro: %v", inicio)
	}
	if _, ok := inicio["ts"].(float64); !ok {
		t.Fatalf("/ sin ts: %v", inicio)
	}

	health := cogerJSON(t, base+"/health")
	if health["state"] != "conectado" || health["uptimeSec"].(float64) < 100 {
		t.Fatalf("/health raro: %v", health)
	}
	cola := health["queue"].(map[string]any)
	if cola["sentToday"] != float64(7) || cola["budget"] != float64(120) {
		t.Fatalf("/health queue: %v", cola)
	}
	if d := health["dispatcher"].(map[string]any); d["workers"] != float64(4) || d["pending"] != float64(2) {
		t.Fatalf("/health dispatcher: %v", d)
	}

	q := cogerJSON(t, base+"/queue")
	if q["antiban"].(map[string]any)["budget"] != float64(120) {
		t.Fatalf("/queue antiban: %v", q)
	}

	mem := cogerJSON(t, base+"/memoria")
	if mem["goroutines"].(float64) < 1 || mem["heapAllocMB"].(float64) <= 0 {
		t.Fatalf("/memoria raro: %v", mem)
	}

	// Ruta no montada: 404 como el express del JS, no la ficha.
	resp, err := http.Get(base + "/loquesea")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("/loquesea debía ser 404, salió %d", resp.StatusCode)
	}
}

// ── Auth expuesta: usuario fijo, tiempo constante y candado ──────────────────

func TestAuthExpuesta(t *testing.T) {
	s, base := panelPrueba(t, Config{Publico: true, Password: "secreto-largo"})
	s.VentanaFallos = 2 * time.Minute // en pruebas no se espera de verdad

	// Sin contraseña → 401 con el realm.
	resp, err := http.Get(base + "/health")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("sin contraseña debía ser 401, salió %d", resp.StatusCode)
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
	_ = resp.Body.Close()
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
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin ok debía ser 200, salió %d", resp.StatusCode)
	}
}

func TestCandadoAntiFuerzaBruta(t *testing.T) {
	s, base := panelPrueba(t, Config{Publico: true, Password: "secreto-largo"})
	s.VentanaFallos = 10 * time.Minute

	client := &http.Client{}
	intentar := func() int {
		req, _ := http.NewRequest("GET", base+"/health", nil)
		req.SetBasicAuth("admin", "malisima")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	for i := 0; i < maxFallos; i++ {
		if got := intentar(); got != http.StatusUnauthorized {
			t.Fatalf("intento %d: 401 esperado, salió %d", i+1, got)
		}
	}
	if got := intentar(); got != http.StatusTooManyRequests {
		t.Fatalf("tras %d fallos debía ser 429, salió %d", maxFallos, got)
	}
	// Y con la contraseña BUENA también es 429: el candado protege la cuenta.
	req, _ := http.NewRequest("GET", base+"/health", nil)
	req.SetBasicAuth("admin", "secreto-largo")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("bloqueada aunque la clave sea buena: %d", resp.StatusCode)
	}
}
