// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package ui

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestRenderFaxProducesValidPNG(t *testing.T) {
	data, err := RenderFax(FaxDoc{
		Title:    "SHIN-MD",
		Subtitle: "17 COMANDOS",
		Lines:    []string{"PRINCIPAL", "> MENU  MENU CON BOTONES", "", "ECONOMIA"},
		Footer:   "ESCRIBE .MENU",
	})
	if err != nil {
		t.Fatalf("RenderFax: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("el PNG no se puede decodificar: %v", err)
	}
	b := img.Bounds()
	if b.Dx() < 100 || b.Dy() < 100 {
		t.Fatalf("imagen demasiado pequeña: %dx%d", b.Dx(), b.Dy())
	}
	if b.Dx() > faxMaxWidth || b.Dy() > faxMaxHeight {
		t.Fatalf("imagen fuera de límites: %dx%d", b.Dx(), b.Dy())
	}
	if len(data) > faxMaxBytes {
		t.Fatalf("imagen demasiado pesada: %d bytes", len(data))
	}
}

// Esto lo dispara cualquiera desde el chat: tiene que tener techo SIEMPRE.
func TestRenderFaxLimitsHugeInput(t *testing.T) {
	lineas := make([]string, 500) // 500 líneas de 5000 caracteres
	for i := range lineas {
		lineas[i] = strings.Repeat("A", 5000)
	}

	data, err := RenderFax(FaxDoc{Title: strings.Repeat("T", 1000), Lines: lineas})
	if err != nil {
		t.Fatalf("debería recortar y dibujar, no fallar: %v", err)
	}
	if len(data) > faxMaxBytes {
		t.Fatalf("se coló una imagen de %d bytes", len(data))
	}
}

func TestRenderFaxEmptyDoc(t *testing.T) {
	if _, err := RenderFax(FaxDoc{}); err != nil {
		t.Fatalf("un documento vacío debería dibujarse igual: %v", err)
	}
}

// Nada de controles ni invisibles en el dibujo: se usan para falsificar texto.
func TestSanitizeText(t *testing.T) {
	casos := map[string]string{
		"hola\nmundo":          "hola mundo",
		"hola\t\tmundo":        "hola mundo",
		"a\u200bb\u202ec":      "abc",
		"  muchos   espacios ": "muchos espacios",
		"\x00\x01binario":      "binario",
	}
	for in, want := range casos {
		if got := sanitizeText(in); got != want {
			t.Errorf("sanitizeText(%q) = %q, se esperaba %q", in, got, want)
		}
	}
}

func TestClipAndWidest(t *testing.T) {
	if got := clip("abcdef", 4); got != "abc…" {
		t.Fatalf("clip = %q", got)
	}
	if got := clip("ab", 4); got != "ab" {
		t.Fatalf("clip no debería tocar lo corto: %q", got)
	}
	// Caracteres multibyte: se corta por caracteres, no por bytes.
	if got := clip("aéióúñ", 4); got != "aéi…" {
		t.Fatalf("clip multibyte = %q", got)
	}
	if TextWidth("") != 0 || TextWidth("AB") != glyphW*2+1 {
		t.Fatalf("TextWidth mal calculado")
	}
}

// La fuente no puede tener glifos a medias: cada uno son 7 filas de 5.
func TestFontGlyphsAreWellFormed(t *testing.T) {
	for r, g := range font {
		rows := strings.Split(g, "/")
		if len(rows) != glyphH {
			t.Errorf("glifo %q tiene %d filas, se esperaban %d", r, len(rows), glyphH)
		}
		for i, row := range rows {
			if len([]rune(row)) != glyphW {
				t.Errorf("glifo %q fila %d mide %d, se esperaban %d", r, i, len([]rune(row)), glyphW)
			}
			for _, c := range row {
				if c != '#' && c != '.' {
					t.Errorf("glifo %q tiene un carácter raro: %q", r, c)
				}
			}
		}
	}
}

func TestGlyphFallback(t *testing.T) {
	// Las minúsculas y los acentos se resuelven a mayúsculas sin tilde.
	for _, in := range []string{"a", "á", "ñ", "ü"} {
		if _, ok := glyphFor([]rune(in)[0]); !ok {
			t.Errorf("glyphFor(%q) no encontró glifo", in)
		}
	}
	// Un emoji no existe: se dibuja como '?' en vez de dejar hueco.
	if _, ok := glyphFor('😀'); ok {
		t.Error("no debería existir glifo para un emoji")
	}
}
