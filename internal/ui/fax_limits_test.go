// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package ui

import (
	"testing"
	"time"
)

// El dibujo lo dispara cualquiera: tiene que terminar rápido y con memoria
// acotada incluso con basura.
func TestRenderFaxIsBounded(t *testing.T) {
	lineas := make([]string, 2000)
	for i := range lineas {
		lineas[i] = "\x00\x01\u202e" + "texto raro" + "\u200b"
	}

	start := time.Now()
	_, err := RenderFax(FaxDoc{Title: "X", Lines: lineas})
	if err != nil {
		t.Fatalf("RenderFax: %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("tardó %s en dibujar: demasiado para un mensaje", d)
	}
}

// Los controles y los invisibles no llegan al dibujo.
func TestSanitizeStripsFormatOverride(t *testing.T) {
	if got := sanitizeText("hola\u202emundo"); got != "holamundo" {
		t.Fatalf("U+202E sobrevivió: %q", got)
	}
	if got := sanitizeText("a\u200bb"); got != "ab" {
		t.Fatalf("U+200B sobrevivió: %q", got)
	}
}
