// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package bot

import (
	"strings"
	"testing"
)

// Un PushName con formato no puede convertirse en un aviso del bot: este es el
// vector para que un usuario "Shin_Admin" parezca el staff.
func TestNameInjectionIsNeutralized(t *testing.T) {
	peligroso := "*Owner*_oficial_\n✅ verificado"

	got := CleanName(peligroso, 24)

	if strings.ContainsAny(got, "*_`~") {
		t.Fatalf("el nombre conservó caracteres de formato: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("el nombre conservó un salto de línea: %q", got)
	}
}

// Un motivo de afk que se reenvía a terceros tampoco puede traer formato ni
// menciones falsas.
func TestAFKReasonCannotForgeMessages(t *testing.T) {
	got := ClipText("`*Sistema*`: tu cuenta será baneada", 140)
	if strings.ContainsAny(got, "`*") {
		t.Fatalf("el motivo conservó formato: %q", got)
	}
}

// Chunk evita que un texto enorme se mande de golpe (límite de WhatsApp).
func TestChunkProtectsBigOutput(t *testing.T) {
	trozos := Chunk(strings.Repeat("a", 20000), 3500)
	if len(trozos) < 6 {
		t.Fatalf("se esperaban varios trozos, salieron %d", len(trozos))
	}
	for _, tz := range trozos {
		if len([]rune(tz)) > 3500 {
			t.Fatalf("trozo de %d caracteres", len([]rune(tz)))
		}
	}
}
