// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// utilidades2_test.go — lo del bloque B5.2 que se puede probar sin abrir
// WhatsApp: la regla pura de `.ptv` y los formateadores de texto.
package cmds

import "testing"

// La misma tabla que en el JS (portada de puedeSerPtv).
func TestPuedeSerPTV(t *testing.T) {
	casos := []struct {
		segundos, bytes uint64
		esGIF           bool
		ok              bool
		motivo          string
	}{
		{30, 5 << 20, false, true, ""},
		{61, 5 << 20, false, false, "largo"},
		{10, 17 << 20, false, false, "pesado"},
		{10, 5 << 20, true, false, "gif"},
		{60, 16 << 20, false, true, ""}, // justo en el límite pasa
	}
	for _, c := range casos {
		ok, motivo := puedeSerPTV(c.segundos, c.bytes, c.esGIF)
		if ok != c.ok || motivo != c.motivo {
			t.Errorf("puedeSerPTV(%d s, %d B, gif=%v) = (%v, %q), se esperaba (%v, %q)",
				c.segundos, c.bytes, c.esGIF, ok, motivo, c.ok, c.motivo)
		}
	}
}

func TestFormatoMoneda(t *testing.T) {
	casos := []struct {
		v      float64
		dec    int
		quiero string
	}{
		{90000.5, 6, "$90,000.50"},
		{1234567.891, 6, "$1,234,567.89"},
		{0.00043218, 6, "$0.000432"},
		{0.5, 4, "$0.5000"},
		{0, 6, "—"},
	}
	for _, c := range casos {
		if got := formatoMoneda(c.v, c.dec); got != c.quiero {
			t.Errorf("formatoMoneda(%v) = %q, se esperaba %q", c.v, got, c.quiero)
		}
	}
}

func TestFormatoBytes(t *testing.T) {
	casos := map[int]string{
		512:     "512 B",
		1536:    "1.5 KB",
		1048576: "1 MB",
		7340032: "7 MB",
		2400000: "2.29 MB",
	}
	for bytes, quiero := range casos {
		if got := formatoBytes(bytes); got != quiero {
			t.Errorf("formatoBytes(%d) = %q, se esperaba %q", bytes, got, quiero)
		}
	}
}

func TestExtensionDe(t *testing.T) {
	casos := map[string]string{
		"image/jpeg":                "jpg",
		"image/png":                 "png",
		"video/mp4":                 "mp4",
		"text/plain; charset=utf-8": "plain",
		"application/octet-stream":  "bin",
		"rarísimo":                  "bin",
	}
	for mime, quiero := range casos {
		if got := extensionDe(mime); got != quiero {
			t.Errorf("extensionDe(%q) = %q, se esperaba %q", mime, got, quiero)
		}
	}
}
