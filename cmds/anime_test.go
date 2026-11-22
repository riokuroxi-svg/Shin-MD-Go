// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// anime_test.go — la tabla de reacciones: cada alias llega a una sola fila,
// nekos.best se finge con httptest y la frase conserva el doble destino del
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
	if acciones["pat"].slug != "pat" {
		t.Fatalf(".pat debe conservar animación: %+v", acciones["pat"])
	}

	// Lista publicada por /api/v2/endpoints: ningún slug declarado debe caer
	// silenciosamente en un 404 del proveedor.
	supported := map[string]bool{}
	for _, slug := range strings.Fields("lurk shoot sleep clap shrug stare wave poke confused smile peck wink sip blush smug tickle yeet think highfive feed wag bite teehee shocked bleh bored nom nya yawn facepalm cuddle kick happy carry hug kabedon baka bonk pat angry spin shake run nod nope kiss dance punch handshake slap cry lappillow pout blowkiss handhold salute thumbsup laugh tableflip neko waifu husbando kitsune") {
		supported[slug] = true
	}
	for name, action := range acciones {
		if action.slug != "" && !supported[action.slug] {
			t.Errorf("%s usa endpoint inexistente %q", name, action.slug)
		}
	}
}

func TestGifDeHttptest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/hug") {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{
				"url": "http://" + r.Host + "/falso.gif", "anime_name": "Prueba",
			}}})
			return
		}
		w.Header().Set("Content-Type", "image/gif")
		_, _ = w.Write([]byte("GIF89a-falso"))
	}))
	defer srv.Close()

	prev := urlNekosBest
	urlNekosBest = srv.URL
	defer func() { urlNekosBest = prev }()

	gif, err := gifDe(t.Context(), "hug")
	if err != nil || string(gif) != "GIF89a-falso" {
		t.Fatalf("gifDe: %v %q", err, gif)
	}
	// Slug desconocido: un 404 se propaga como error, no como pánico.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	if _, err := gifDe(t.Context(), "inventado"); err == nil {
		t.Fatal("un 404 debía ser error")
	}
}

func TestGetCortoBloqueaSSRFYRedirectFueraDelProveedor(t *testing.T) {
	if _, err := getCortoLimite(t.Context(), "http://127.0.0.1/secret", 1024); err == nil {
		t.Fatal("la descarga genérica aceptó loopback")
	}

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("secreto"))
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer source.Close()
	if _, err := getCortoLimite(t.Context(), source.URL, 1024, "127.0.0.1"); err == nil {
		t.Fatal("se siguió una redirección a otra autoridad")
	}
}
