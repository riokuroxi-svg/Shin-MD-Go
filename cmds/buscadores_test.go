// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package cmds

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuscarPipedDecodificaItemsYUsaFallback(t *testing.T) {
	caido := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no", http.StatusServiceUnavailable)
	}))
	defer caido.Close()
	vivo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "lofi" || r.URL.Query().Get("filter") != "videos" {
			t.Errorf("query inesperada: %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(pipedSearchResp{Items: []pipedVideo{{
			Type: "stream", Title: "Lofi", URL: "/watch?v=abcdefghijk", Uploader: "Canal", Duration: 61,
		}}})
	}))
	defer vivo.Close()

	prevPrimary, prevFallbacks := urlPiped, urlPipedFallbacks
	urlPiped, urlPipedFallbacks = caido.URL, []string{vivo.URL}
	defer func() { urlPiped, urlPipedFallbacks = prevPrimary, prevFallbacks }()

	videos, err := buscarPiped(context.Background(), "lofi")
	if err != nil {
		t.Fatal(err)
	}
	if len(videos) != 1 || videos[0].Title != "Lofi" {
		t.Fatalf("videos = %+v", videos)
	}
}

func TestYoutubeURLPipedValidaID(t *testing.T) {
	if got, ok := youtubeURLPiped("/watch?v=abcdefghijk"); !ok || got != "https://youtu.be/abcdefghijk" {
		t.Fatalf("URL válida = %q, %v", got, ok)
	}
	for _, raw := range []string{
		"/channel/abcdefghijk", "/watch?v=../../etc", "https://evil.test/watch?v=short", "/watch?v=abcdefghijk%26x=y",
	} {
		if got, ok := youtubeURLPiped(raw); ok {
			t.Errorf("URL peligrosa aceptada: %q → %q", raw, got)
		}
	}
}

func TestAptoideURLSeguraSoloHostsOficiales(t *testing.T) {
	if !aptoideURLSegura("https://pool.apk.aptoide.com/store/app.apk", "pool.apk.aptoide.com") {
		t.Fatal("se rechazó el host oficial")
	}
	for _, raw := range []string{
		"http://pool.apk.aptoide.com/app.apk",
		"https://pool.apk.aptoide.com.evil.test/app.apk",
		"https://user@pool.apk.aptoide.com/app.apk",
	} {
		if aptoideURLSegura(raw, "pool.apk.aptoide.com") {
			t.Errorf("se aceptó URL no segura: %s", raw)
		}
	}
}

func TestMediafireURLSeguraNoAceptaCoincidenciasDeTexto(t *testing.T) {
	for _, raw := range []string{
		"https://mediafire.com/file/abc",
		"https://www.mediafire.com/file/abc",
		"https://download1.mediafire.com/path/file.zip",
	} {
		if !mediafireURLSegura(raw) {
			t.Errorf("se rechazó URL oficial: %s", raw)
		}
	}
	for _, raw := range []string{
		"http://www.mediafire.com/file/abc",
		"https://mediafire.com.evil.test/file/abc",
		"https://evil.test/?next=mediafire.com",
		"https://user@mediafire.com/file/abc",
		"https://127.0.0.1/?mediafire.com",
	} {
		if mediafireURLSegura(raw) {
			t.Errorf("se aceptó URL falsa: %s", raw)
		}
	}
}
