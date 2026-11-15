// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// apis_test.go — el resto de clientes del bloque B5.2, todos contra
// httptest.Server: ni una prueba toca internet de verdad.
package apis

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ─── .acortar ───────────────────────────────────────────────────────────────

func TestAcortar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("url") != "https://ejemplo.com/x" {
			t.Errorf("la API debía recibir el enlace codificado: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte("https://tinyurl.com/abc123\n"))
	}))
	defer srv.Close()
	viejo := urlTinyURL
	urlTinyURL = srv.URL
	t.Cleanup(func() { urlTinyURL = viejo })

	corto, err := Acortar(context.Background(), "https://ejemplo.com/x")
	if err != nil || corto != "https://tinyurl.com/abc123" {
		t.Fatalf("Acortar = (%q, %v)", corto, err)
	}
}

func TestAcortarRechazaNoEnlaces(t *testing.T) {
	if _, err := Acortar(context.Background(), "hola que tal"); err == nil || err.Error() != "no-es-enlace" {
		t.Fatalf("texto suelto debía salir con no-es-enlace: %v", err)
	}
}

func TestAcortarRespuestaRara(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Error: url invalida"))
	}))
	defer srv.Close()
	viejo := urlTinyURL
	urlTinyURL = srv.URL
	t.Cleanup(func() { urlTinyURL = viejo })
	if _, err := Acortar(context.Background(), "https://ejemplo.com"); err == nil {
		t.Fatal("una respuesta que no es un enlace no puede salir como válida")
	}
}

// ─── .crypto ────────────────────────────────────────────────────────────────

func TestPrecios(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("ids"); got != "bitcoin,ethereum" {
			t.Errorf("ids = %q (el alias btc debía resolver a bitcoin)", got)
		}
		_, _ = w.Write([]byte(`{"bitcoin":{"usd":90000.5,"mxn":1530000},"ethereum":{"usd":3100,"mxn":52700}}`))
	}))
	defer srv.Close()
	viejo := urlCoinGecko
	urlCoinGecko = srv.URL
	t.Cleanup(func() { urlCoinGecko = viejo })

	precios, err := Precios(context.Background(), []string{"btc", "ETH"})
	if err != nil {
		t.Fatalf("Precios: %v", err)
	}
	if len(precios) != 2 || precios[0].ID != "bitcoin" || precios[0].P.USD != 90000.5 {
		t.Fatalf("orden y valores: %+v", precios)
	}
	if precios[1].P.MXN != 52700 {
		t.Fatalf("mxn ethereum = %v", precios[1].P.MXN)
	}
}

func TestPreciosNoEncontrada(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	viejo := urlCoinGecko
	urlCoinGecko = srv.URL
	t.Cleanup(func() { urlCoinGecko = viejo })
	if _, err := Precios(context.Background(), []string{"moneda-inventada"}); err == nil || err.Error() != "no-encontrada" {
		t.Fatalf("debía ser no-encontrada: %v", err)
	}
}

// ─── .gitclone / .githubstalk ───────────────────────────────────────────────

func TestParseRepoURL(t *testing.T) {
	buenos := map[string][2]string{
		"https://github.com/riokuroxi-svg/Shin-MD-Go": {"riokuroxi-svg", "Shin-MD-Go"},
		"git@github.com/riokuroxi-svg/Shin-MD-Go.git": {"riokuroxi-svg", "Shin-MD-Go"},
		"https://github.com/methods/go-algorithms":    {"methods", "go-algorithms"},
	}
	for entrada, quiero := range buenos {
		if u, r := ParseRepoURL(entrada); u != quiero[0] || r != quiero[1] {
			t.Errorf("ParseRepoURL(%q) = (%q, %q)", entrada, u, r)
		}
	}
	for _, malo := range []string{"", "https://gitlab.com/a/b", "texto suelto", "https://github.com/solo"} {
		if u, r := ParseRepoURL(malo); u != "" || r != "" {
			t.Errorf("ParseRepoURL(%q) debía salir vacío: (%q, %q)", malo, u, r)
		}
	}
}

func TestStalkGitHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/fantasma") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"login":"riokuroxi-svg","name":"Rio","public_repos":2,"followers":10,"following":1,"created_at":"2024-01-15T00:00:00Z","html_url":"https://github.com/riokuroxi-svg","avatar_url":"https://avatars.githubusercontent.com/u/1"}`))
	}))
	defer srv.Close()
	viejo := urlGitHub
	urlGitHub = srv.URL
	t.Cleanup(func() { urlGitHub = viejo })

	u, err := StalkGitHub(context.Background(), "@riokuroxi-svg")
	if err != nil {
		t.Fatalf("StalkGitHub: %v", err)
	}
	if u.Login != "riokuroxi-svg" || u.Repos != 2 || u.Seguidores != 10 {
		t.Fatalf("ficha: %+v", u)
	}
	if _, err := StalkGitHub(context.Background(), "fantasma"); err == nil || err.Error() != ErrNoEncontrado.Error() {
		t.Fatalf("un 404 debía mapear a ErrNoEncontrado: %v", err)
	}
}

func TestBuscarReposMaximoCinco(t *testing.T) {
	var items []string
	for i := 0; i < 12; i++ {
		items = append(items, `{"name":"r","description":"d","html_url":"https://github.com/a/r","owner":{"login":"a","avatar_url":""}}`)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[` + strings.Join(items, ",") + `]}`))
	}))
	defer srv.Close()
	viejo := urlGitHub
	urlGitHub = srv.URL
	t.Cleanup(func() { urlGitHub = viejo })

	repos, err := BuscarRepos(context.Background(), "shin")
	if err != nil || len(repos) != 5 {
		t.Fatalf("debían salir exactamente 5: %d (%v)", len(repos), err)
	}
}

func TestZipball(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Disposition", `attachment; filename=usuario-repo-a1b2c3d.zip`)
		_, _ = w.Write([]byte("PK\x03\x04-fake-zip-pero-valido-para-la-prueba"))
	}))
	defer srv.Close()
	viejo := urlGitHub
	urlGitHub = srv.URL
	t.Cleanup(func() { urlGitHub = viejo })

	datos, nombre, err := Zipball(context.Background(), "usuario", "repo")
	if err != nil {
		t.Fatalf("Zipball: %v", err)
	}
	if nombre != "usuario-repo-a1b2c3d.zip" || !strings.HasPrefix(string(datos), "PK") {
		t.Fatalf("nombre = %q, datos = %d bytes", nombre, len(datos))
	}
}

// ─── .letra ─────────────────────────────────────────────────────────────────

func TestParseLetraPeticion(t *testing.T) {
	if a, ca := ParseLetraPeticion("Bad Bunny/Diles"); a != "Bad Bunny" || ca != "Diles" {
		t.Errorf("con barra: (%q, %q)", a, ca)
	}
	if a, ca := ParseLetraPeticion("Bad Bunny Diles"); a != "Bad" || ca != "Bunny Diles" {
		t.Errorf("sin barra: (%q, %q)", a, ca)
	}
	if a, ca := ParseLetraPeticion("solouna"); a != "" || ca != "" {
		t.Errorf("una sola palabra no vale: (%q, %q)", a, ca)
	}
}

func TestTrocearLetra(t *testing.T) {
	var lineas []string
	for i := 0; i < 200; i++ {
		lineas = append(lineas, "una línea de letra con treinta y pico caracteres")
	}
	letra := strings.Join(lineas, "\n")
	trozos := TrocearLetra(letra, 3500)
	if len(trozos) < 2 {
		t.Fatalf("debía partir en varios: %d", len(trozos))
	}
	for i, trozo := range trozos {
		if len(trozo) > 3500 {
			t.Fatalf("trozo %d mide %d > 3500", i, len(trozo))
		}
	}
	if strings.Join(trozos, "\n") != letra {
		t.Fatal("unir los trozos debe reconstruir la letra (solo se quitan saltos dobles)")
	}
	// Una letra sin saltos se corta a lo bruto pero dentro del límite.
	if muro := TrocearLetra(strings.Repeat("a", 9000), 3500); len(muro) != 3 {
		t.Fatalf("muro de 9000 en mensajes de 3500 → 3 trozos, no %d", len(muro))
	}
}

func TestLetraNoEncontrada(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"No lyrics found"}`))
	}))
	defer srv.Close()
	viejo := urlLyrics
	urlLyrics = srv.URL
	t.Cleanup(func() { urlLyrics = viejo })
	if _, err := Letra(context.Background(), "nadie", "nada"); err == nil || err.Error() != "no-encontrada" {
		t.Fatalf("un 404 de lyrics debía salir no-encontrada: %v", err)
	}
}

// ─── .tts ───────────────────────────────────────────────────────────────────

func TestTrocearVoz(t *testing.T) {
	texto := strings.Repeat("palabra ", 100) // 800 bytes con espacios
	trozos := TrocearVoz(texto, 180)
	if len(trozos) < 4 {
		t.Fatalf("800 bytes en trozos de 180 → al menos 4, no %d", len(trozos))
	}
	for i, trozo := range trozos {
		if len(trozo) > 180 {
			t.Fatalf("trozo %d mide %d bytes > 180", i, len(trozo))
		}
		if strings.HasSuffix(trozo, "palab") || strings.HasSuffix(trozo, "palabr") {
			t.Fatalf("trozo %d cortó una palabra: %q", i, trozo)
		}
	}
	// Texto multibyte: cada carácter son 2 bytes; no puede salir partido.
	cirilico := strings.Repeat("привет ", 60)
	for _, trozo := range TrocearVoz(cirilico, 100) {
		for _, r := range trozo {
			if !strings.ContainsRune("привет ", r) {
				t.Fatalf("runa partida o ajena en %q", trozo)
			}
		}
		if len(trozo) > 100 {
			t.Fatalf("trozo multibyte mide %d > 100", len(trozo))
		}
	}
}

func TestVozConcatenaTrozos(t *testing.T) {
	var peticiones []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		peticiones = append(peticiones, r.URL.Query().Get("q"))
		_, _ = w.Write(make([]byte, 600)) // MP3 de mentira pero con cuerpo
	}))
	defer srv.Close()
	viejo := urlVoz
	urlVoz = srv.URL
	t.Cleanup(func() { urlVoz = viejo })

	mp3, err := Voz(context.Background(), strings.Repeat("hola ", 80), "es")
	if err != nil {
		t.Fatalf("Voz: %v", err)
	}
	if len(peticiones) < 2 {
		t.Fatalf("400+ caracteres debían salir en varias peticiones, no %d", len(peticiones))
	}
	if len(mp3) != 600*len(peticiones) {
		t.Fatalf("el MP3 final debe ser la concatenación: %d bytes de %d peticiones", len(mp3), len(peticiones))
	}
	// Cada petición respeta el tope del endpoint.
	for i, q := range peticiones {
		if len(q) > 180 {
			t.Fatalf("petición %d mide %d caracteres > 180", i, len(q))
		}
	}
}

func TestVozVacia(t *testing.T) {
	if _, err := Voz(context.Background(), "   ", ""); err == nil || err.Error() != "sin-texto" {
		t.Fatalf("vacío debe salir sin-texto: %v", err)
	}
}

// ─── .tourl ────────────────────────────────────────────────────────────────

func TestSubirTemporal(t *testing.T) {
	Dormir = func(time.Duration) {} // en pruebas los reintentos no esperan
	t.Cleanup(func() { Dormir = time.Sleep })

	var tiempos []string
	var campoReq, nombreArchivo, tipoArchivo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("multipart no parsea: %v", err)
		}
		campoReq = r.FormValue("reqtype")
		tiempos = append(tiempos, r.FormValue("time"))
		if f, cab, err := r.FormFile("fileToUpload"); err == nil {
			nombreArchivo = cab.Filename
			tipoArchivo = cab.Header.Get("Content-Type")
			_, _ = io.Copy(io.Discard, f)
			_ = f.Close()
		}
		if len(tiempos) == 1 {
			w.WriteHeader(http.StatusInternalServerError) // primera vez caído: toca reintento
			return
		}
		_, _ = w.Write([]byte("https://litter.catbox.moe/abc.png"))
	}))
	defer srv.Close()
	viejo := urlLitterbox
	urlLitterbox = srv.URL
	t.Cleanup(func() { urlLitterbox = viejo })

	url, tiempo, err := SubirTemporal(context.Background(), []byte("datos falsos"), "image/png", "foto.png")
	if err != nil {
		t.Fatalf("SubirTemporal: %v", err)
	}
	if url != "https://litter.catbox.moe/abc.png" || tiempo != "24h" {
		t.Fatalf("url = %q tiempo = %q", url, tiempo)
	}
	if campoReq != "fileupload" || nombreArchivo != "foto.png" || tipoArchivo != "image/png" {
		t.Fatalf("multipart mal armado: reqtype=%q nombre=%q tipo=%q", campoReq, nombreArchivo, tipoArchivo)
	}
	if len(tiempos) != 2 {
		t.Fatalf("un fallo → dos intentos con 24h: %v", tiempos)
	}
}

func TestSubirTemporalAgotaTiempos(t *testing.T) {
	Dormir = func(time.Duration) {}
	t.Cleanup(func() { Dormir = time.Sleep })

	var tiempos []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseMultipartForm(1 << 20)
		tiempos = append(tiempos, r.FormValue("time"))
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	viejo := urlLitterbox
	urlLitterbox = srv.URL
	t.Cleanup(func() { urlLitterbox = viejo })

	_, _, err := SubirTemporal(context.Background(), []byte("x"), "application/octet-stream", "a.bin")
	if err == nil {
		t.Fatal("un servidor caído no puede dar enlace")
	}
	if got := len(tiempos); got != 6 { // 3 tiempos × 2 intentos, como el JS
		t.Fatalf("reintentos = %d, se esperaban 6 (%v)", got, tiempos)
	}
	if tiempos[0] != "24h" || tiempos[2] != "72h" || tiempos[4] != "1h" {
		t.Fatalf("orden de tiempos distinto al JS: %v", tiempos)
	}
}

func TestSubirTemporalMuyGrande(t *testing.T) {
	if _, _, err := SubirTemporal(context.Background(), make([]byte, MaxSubida+1), "application/octet-stream", "a.bin"); err == nil || err.Error() != "muy-grande" {
		t.Fatalf("por encima del tope debe salir muy-grande: %v", err)
	}
}

// ─── .ai ────────────────────────────────────────────────────────────────────

func TestGeminiPeticionYRespuesta(t *testing.T) {
	var cuerpo []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cuerpo, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"¡Hola! El vaso tiene 0.25 litros."}]}}]}`))
	}))
	defer srv.Close()
	viejo := urlGemini
	urlGemini = srv.URL + "/v1beta/models/%s:generateContent"
	t.Cleanup(func() { urlGemini = viejo })

	out, err := Gemini(context.Background(), "KEY123", "gemini-1.5-flash", "¿cuántos litros?", "", "")
	if err != nil || !strings.Contains(out, "0.25 litros") {
		t.Fatalf("Gemini = (%q, %v)", out, err)
	}
	// La petición debe llevar el prompt de sistema y el turno del usuario.
	if !strings.Contains(string(cuerpo), "systemInstruction") ||
		!strings.Contains(string(cuerpo), "Shin-MD") ||
		!strings.Contains(string(cuerpo), "¿cuántos litros?") {
		t.Fatalf("cuerpo de la petición incompleto: %s", cuerpo)
	}
}

func TestGeminiErrores(t *testing.T) {
	codigos := map[int]string{
		http.StatusForbidden:       "key-invalida",
		http.StatusTooManyRequests: "cupo",
	}
	for codigo, motivo := range codigos {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(codigo)
		}))
		viejo := urlGemini
		urlGemini = srv.URL + "/%s"
		_, err := Gemini(context.Background(), "k", "m", "hola", "", "")
		urlGemini = viejo
		srv.Close()
		if err == nil || err.Error() != motivo {
			t.Errorf("HTTP %d debía salir %q: %v", codigo, motivo, err)
		}
	}
}

func TestLeerGemini(t *testing.T) {
	if _, err := leerGemini([]byte(`{"candidates":[]}`)); err == nil {
		t.Fatal("sin candidatos no hay respuesta")
	}
	if _, err := leerGemini([]byte(`no es json`)); err == nil {
		t.Fatal("basura no es respuesta")
	}
}
