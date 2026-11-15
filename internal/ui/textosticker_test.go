// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package ui

import (
	"bytes"
	"testing"
)

func TestRenderBratYQuote(t *testing.T) {
	png1, err := RenderBrat("hola hola hola hola hola hola hola hola hola")
	if err != nil || len(png1) == 0 {
		t.Fatalf("brat: %v", err)
	}
	if !bytes.Equal(png1[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}) {
		t.Fatal("no es PNG")
	}
	png2, err := RenderQuoteSticker("cita de prueba con nombre", "Ana")
	if err != nil || len(png2) == 0 {
		t.Fatalf("qc: %v", err)
	}
	if bytes.Equal(png1, png2) {
		t.Fatal("brat y qc no debían ser la misma imagen")
	}
	// Vacío no pinta
	if _, err := RenderBrat("   "); err == nil {
		t.Fatal("vacío debía fallar")
	}
	// Kilómetros de texto no explotan el lienzo: se recorta.
	largo := ""
	for i := 0; i < 300; i++ {
		largo += "palabra "
	}
	if _, err := RenderBrat(largo); err != nil {
		t.Fatalf("largo: %v", err)
	}
}
