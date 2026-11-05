// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// imagen_test.go — La imagen la sirve un servicio de fuera, así que todo lo que
// llega de ahí se trata como no confiable: tipo, tamaño y tiempo.
package gacha

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// conServidorFalso apunta el buscador a un servidor local y lo restaura.
func conServidorFalso(t *testing.T, h http.HandlerFunc) {
	t.Helper()
	srv := httptest.NewServer(h)
	previo := safebooruEndpoint
	safebooruEndpoint = srv.URL + "/index.php"
	t.Cleanup(func() {
		safebooruEndpoint = previo
		srv.Close()
	})
}

func TestBuscarImagenEligeJPG(t *testing.T) {
	conServidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.php" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[
				{"directory":"12","image":"a.jpg"},
				{"directory":"13","image":"b.png"},
				{"directory":"14","image":"c.gif"},
				{"directory":"15","image":"../etc/passwd"},
				{"directory":"","image":"d.jpg"}
			]`))
			return
		}
		// Cualquier descarga devuelve un JPEG mínimo.
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xFF\xD8\xFF\xE0imagen"))
	})

	img, err := BuscarImagen(context.Background(), "sakura haruno", func(n int) int { return 0 })
	if err != nil {
		t.Fatalf("BuscarImagen: %v", err)
	}
	if len(img) == 0 || img[0] != 0xFF || img[1] != 0xD8 {
		t.Fatalf("no llegó un JPEG: %v", img)
	}
}

// Si el tablero dice que hay imágenes pero devuelve HTML, no se envía basura.
func TestBuscarImagenRechazaNoImagen(t *testing.T) {
	conServidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.php" {
			_, _ = w.Write([]byte(`[{"directory":"12","image":"a.jpg"}]`))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>no eres una imagen</html>"))
	})
	if _, err := BuscarImagen(context.Background(), "x", nil); err == nil {
		t.Fatal("se aceptó HTML como imagen")
	}
}

func TestBuscarImagenTopeDeTamano(t *testing.T) {
	conServidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/index.php" {
			_, _ = w.Write([]byte(`[{"directory":"12","image":"a.jpg"}]`))
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		// Un byte más que el tope.
		_, _ = w.Write(make([]byte, MaxImageBytes+1))
	})
	if _, err := BuscarImagen(context.Background(), "x", nil); err == nil {
		t.Fatal("se aceptó una imagen por encima del tope")
	}
}

func TestBuscarImagenSinResultados(t *testing.T) {
	conServidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	if _, err := BuscarImagen(context.Background(), "noexiste", nil); err == nil {
		t.Fatal("sin resultados debía devolver error")
	}
}

// Un tablero lento no puede dejar el comando colgado: el contexto corta.
func TestBuscarImagenRespetaElTiempo(t *testing.T) {
	conServidorFalso(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_, _ = w.Write([]byte(`[]`))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	inicio := time.Now()
	_, err := BuscarImagen(ctx, "x", nil)
	if err == nil {
		t.Fatal("debía fallar por tiempo")
	}
	if transcurrido := time.Since(inicio); transcurrido > 2*time.Second {
		t.Fatalf("tardó %v en rendirse", transcurrido)
	}
}

func TestBuscarImagenURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("\x89PNG\r\n"))
	}))
	defer srv.Close()

	if _, err := BuscarImagenURL(context.Background(), srv.URL+"/x.png"); err != nil {
		t.Fatalf("BuscarImagenURL: %v", err)
	}
	// Extensiones que no son imagen ni de http(s): se rechazan sin salir a la red.
	for _, u := range []string{"", "file:///etc/passwd", "ftp://x/y.png", srv.URL + "/x.svg", srv.URL + "/x.html"} {
		if _, err := BuscarImagenURL(context.Background(), u); err == nil {
			t.Fatalf("se aceptó la url %q", u)
		}
	}
}

func TestNormalizarTag(t *testing.T) {
	casos := map[string]string{
		"Sakura Haruno":         "sakura_haruno",
		"  NARUTO  ":            "naruto",
		"a.b;c":                 "abc",
		"oso-ñandú":             "oso-nandu",
		strings.Repeat("x", 80): strings.Repeat("x", 64),
	}
	for entrada, esperado := range casos {
		if got := normalizarTag(entrada); got != esperado {
			t.Fatalf("normalizarTag(%q) = %q, se esperaba %q", entrada, got, esperado)
		}
	}
}
