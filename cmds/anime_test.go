// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// anime_test.go — la tabla de reacciones: cada alias llega a una sola fila,
// waifu.pics se finge con httptest y la frase conserva el doble destino del
// JS (sola / hacia alguien).
package cmds

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTablaAcciones(t *testing.T) {
	// Todo comando registrado existe en la tabla (ni un alias suelto).
	for nombre := range acciones {
		a := acciones[nombre]
		if a.sola == "" || a.hacia == "" {
			t.Fatalf("%s sin frases", nombre)
		}
	}
	if len(acciones) < 120 {
		t.Fatalf("faltan acciones: %d < 120", len(acciones))
	}
	// Las de siempre tienen slug; las raras no, y no es un bug.
	if acciones["hug"].slug != "hug" || acciones["slap"].slug != "slap" {
		t.Fatalf("slugs base: %+v %+v", acciones["hug"], acciones["slap"])
	}
	// Alias que apuntan a la misma acción (el JS declaraba «hug | abrazar»).
	if acciones["abrazar"].slug != acciones["hug"].slug {
		t.Fatal("hug y abrazar deben ser la misma imagen")
	}
}

func TestGifDeHttptest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/hug") {
			_ = json.NewEncoder(w).Encode(map[string]any{"url": "http://" + r.Host + "/falso.gif"})
			return
		}
		w.Header().Set("Content-Type", "image/gif")
		_, _ = w.Write([]byte("GIF89a-falso"))
	}))
	defer srv.Close()

	prev := urlWaifuPics
	urlWaifuPics = srv.URL
	defer func() { urlWaifuPics = prev }()

	gif, err := gifDe(t.Context(), "hug")
	if err != nil || string(gif) != "GIF89a-falso" {
		t.Fatalf("gifDe: %v %q", err, gif)
	}
	// Slug desconocido → el servidor fingido no entra en /{slug} pero nuestra
	// llamada iría a /nada: waifu.pics de verdad contesta 404; aquí miramos
	// que un 404 del "gis" se propague como error, no como pánico.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := gifDe(t.Context(), "inventado"); err == nil {
		t.Fatal("un 404 debía ser error")
	}
}
