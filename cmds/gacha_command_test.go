// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package cmds

import "testing"

func TestIDRollValidoSoloWhatsmeowWebID(t *testing.T) {
	if !idRollValido("3EB01234567890ABCDEF12") {
		t.Fatal("se rechazó un ID válido generado por whatsmeow")
	}
	for _, id := range []string{
		"", "roll-1", "3EB0../../../../etc", "3EB01234567890ABCDEG12", "3EB01234567890ABCDEF123",
	} {
		if idRollValido(id) {
			t.Errorf("se aceptó ID manipulable: %q", id)
		}
	}
}
