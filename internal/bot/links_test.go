// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package bot

import "testing"

func TestHasLink(t *testing.T) {
	casos := []struct {
		texto  string
		quiere bool
	}{
		// ── sí son enlaces ──────────────────────────────────────────────
		{"mira https://google.com", true},
		{"http://ejemplo.mx/algo", true},
		{"www.tienda.com", true},
		{"entra a chat.whatsapp.com/ABC123", true},
		{"wa.me/5215555555555", true},
		{"t.me/micanal", true},
		{"youtu.be/abc", true},
		{"mi pagina: perros.mx", true},
		{"compra en amazon.com/oferta", true},
		{"un discord.gg/xyz", true},
		{"mira https\u200b://ejemplo.com", true}, // con carácter invisible

		// ── no son enlaces (no se debe borrar el mensaje) ───────────────
		{"hola a todos", false},
		{"escribeme a juan.perez@gmail.com", false},
		{"tengo 3.14 de pi", false},
		{"informe.final ya está", false},
		{"actualicé a la v1.2", false},
		{"Hola. Como estas?", false},
		{"archivo.comprimido", false},
		{"mi numero es 5215555555555", false},
		{"", false},
		{"   ", false},
	}

	for _, c := range casos {
		if got := HasLink(c.texto); got != c.quiere {
			t.Errorf("HasLink(%q) = %v, se esperaba %v", c.texto, got, c.quiere)
		}
	}
}

func TestLinkCount(t *testing.T) {
	// Se exigen 2+ caracteres antes del punto (así "3.14" o "v1.2" no cuentan).
	if n := LinkCount("dos: miau.com y guau.net"); n != 2 {
		t.Fatalf("LinkCount = %d, se esperaban 2", n)
	}
	if n := LinkCount("correo@dominio.com y nada más"); n != 0 {
		t.Fatalf("un correo no debería contar como enlace, dio %d", n)
	}
}

func TestNormalizeTextStripsInvisibles(t *testing.T) {
	if got := NormalizeText("  hola\u200b\u00a0mundo  "); got != "hola mundo" {
		t.Fatalf("NormalizeText = %q", got)
	}
}
