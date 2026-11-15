// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// traducir_test.go — el cerebro de `.tr` se prueba entero sin internet:
// resolución de idiomas, parseo de la petición, detector propio, parseo de
// cada proveedor y la cadena de respaldo (Google caído → MyMemory), con
// httptest.Server haciendo de API.
package apis

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResolverIdioma(t *testing.T) {
	casos := map[string]string{
		"en": "en", "inglés": "en", "ingles": "en", "english": "en", "EN": "en",
		"pt-BR": "pt", "español": "es", "ES": "es", "mandarín": "zh",
		"japonés": "ja", "francés": "fr", "br": "pt", "latín": "la",
	}
	for entrada, quiero := range casos {
		if got := ResolverIdioma(entrada); got != quiero {
			t.Errorf("ResolverIdioma(%q) = %q, se esperaba %q", entrada, got, quiero)
		}
	}
	for _, raro := range []string{"", "klingon", "123", "--"} {
		if got := ResolverIdioma(raro); got != "" {
			t.Errorf("ResolverIdioma(%q) = %q, se esperaba vacío", raro, got)
		}
	}
}

func TestParsePeticion(t *testing.T) {
	// Las cuatro formas que entiende el JS.
	if p := ParsePeticion([]string{"inglés", "hola"}, "", "es"); !p.OK || p.Idioma != "en" || p.Texto != "hola" {
		t.Errorf("con idioma y texto: %+v", p)
	}
	if p := ParsePeticion([]string{"hola"}, "", "es"); !p.OK || p.Idioma != "es" || p.Texto != "hola" {
		t.Errorf("texto suelto va al idioma del bot: %+v", p)
	}
	if p := ParsePeticion([]string{"en"}, "buenos días", "es"); !p.OK || p.Idioma != "en" || p.Texto != "buenos días" {
		t.Errorf("idioma con mensaje citado: %+v", p)
	}
	if p := ParsePeticion(nil, "bonjour", "es"); !p.OK || p.Idioma != "es" || p.Texto != "bonjour" {
		t.Errorf("solo citado: %+v", p)
	}
	if p := ParsePeticion(nil, "", "es"); p.OK || p.Motivo != "sin-texto" {
		t.Errorf("vacío debe fallar con sin-texto: %+v", p)
	}
	largo := strings.Repeat("a", TraducirMaxTexto+10)
	if p := ParsePeticion([]string{largo}, "", "es"); !p.OK || !p.Recortado || len(p.Texto) != TraducirMaxTexto {
		t.Errorf("texto largo se recorta marcándolo: %+v len=%d", p, len(p.Texto))
	}
}

func TestDetectarIdioma(t *testing.T) {
	casos := []struct {
		texto, idioma string
	}{
		{"hola, ¿cómo estás? gracias por venir hoy", "es"},
		{"hello guys, thanks for the help and see you tomorrow", "en"},
		{"obrigado por tudo, até amanha", "pt"},
		{"bonjour, merci beaucoup pour tout", "fr"},
		{"danke schön, guten morgen und guten abend", "de"},
		{"привет, как дела", "ru"}, // escritura que canta sola
		{"こんにちは", "ja"},            // hiragana
		{"안녕하세요", "ko"},            // hangul
		{"مرحبا كيف حالك", "ar"},   // árabe
	}
	for _, c := range casos {
		if got, _ := DetectarIdioma(c.texto); got != c.idioma {
			t.Errorf("DetectarIdioma(%q) = %q, se esperaba %q", c.texto, got, c.idioma)
		}
	}
	// Textos sin patrón claro: no se atreve.
	if got, _ := DetectarIdioma("ok"); got != "" {
		t.Errorf("«ok» es demasiado corto: %q", got)
	}
	if got, _ := DetectarIdioma("123 456 789"); got != "" {
		t.Errorf("solo números: %q", got)
	}
}

func TestParseGoogle(t *testing.T) {
	if txt, de := parseGoogle([]any{
		[]any{[]any{"hola ", "hello"}, []any{"mundo", "world"}}, nil, "en",
	}); txt != "hola mundo" || de != "en" {
		t.Errorf("parseGoogle = (%q, %q)", txt, de)
	}
	if txt, _ := parseGoogle([]any{"no-es-una-lista"}); txt != "" {
		t.Errorf("respuesta rara debe salir vacía: %q", txt)
	}
	if txt, _ := parseGoogle([]any{}); txt != "" {
		t.Errorf("vacía debe salir vacía: %q", txt)
	}
}

func TestParseMyMemory(t *testing.T) {
	ok := map[string]any{"responseStatus": float64(200),
		"responseData": map[string]any{"translatedText": "hola mundo"}}
	if txt := parseMyMemory(ok); txt != "hola mundo" {
		t.Errorf("parseMyMemory = %q", txt)
	}
	ko := map[string]any{"responseStatus": float64(429),
		"responseData": map[string]any{"translatedText": "algo"}}
	if txt := parseMyMemory(ko); txt != "" {
		t.Errorf("estado 429 no puede salir como traducción: %q", txt)
	}
}

// Los dos proveedores se prueban con un servidor de mentira por cada uno.
func TestTraducirConRespaldo(t *testing.T) {
	LimpiarCacheTraduccion()

	google := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // Google caído
	}))
	defer google.Close()
	mymemory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "langpair=autodetect%7Cen") {
			t.Errorf("langpair raro: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"responseStatus":200,"responseData":{"translatedText":"good morning friend"}}`))
	}))
	defer mymemory.Close()

	viejoG, viejoM := urlGoogleTraductor, urlMyMemory
	urlGoogleTraductor, urlMyMemory = google.URL, mymemory.URL
	t.Cleanup(func() { urlGoogleTraductor, urlMyMemory = viejoG, viejoM })

	r := Traducir(context.Background(), "buenos días amigo", "en")
	if !r.OK || r.Texto != "good morning friend" || r.Proveedor != "MyMemory" {
		t.Fatalf("con Google caído debía responder MyMemory: %+v", r)
	}
	// MyMemory no dice el origen: lo adivina el detector propio.
	if r.De != "es" {
		t.Fatalf("De = %q, el detector debía adivinar español", r.De)
	}
}

func TestTraducirCache(t *testing.T) {
	LimpiarCacheTraduccion()
	llamadas := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		llamadas++
		_, _ = w.Write([]byte(`[[["hola","hello"]],null,"en"]`))
	}))
	defer srv.Close()
	viejoG := urlGoogleTraductor
	urlGoogleTraductor = srv.URL
	t.Cleanup(func() { urlGoogleTraductor = viejoG })

	Traducir(context.Background(), "hello", "es")
	r := Traducir(context.Background(), "hello", "es")
	if !r.OK || r.Texto != "hola" || !r.Cacheado {
		t.Fatalf("segunda llamada debía venir de la caché: %+v", r)
	}
	if llamadas != 1 {
		t.Fatalf("llamadas = %d, la caché debía evitar la segunda", llamadas)
	}
}

func TestTraducirErrores(t *testing.T) {
	if r := Traducir(context.Background(), "", "es"); r.OK || r.Motivo != "sin-texto" {
		t.Errorf("vacío: %+v", r)
	}
	if r := Traducir(context.Background(), "hola", "klingon"); r.OK || r.Motivo != "idioma-desconocido" {
		t.Errorf("idioma raro: %+v", r)
	}
}
