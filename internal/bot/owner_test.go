// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package bot

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestNormalizeNumber(t *testing.T) {
	cases := map[string]string{
		"+52 155 1234 5678":               "5215512345678",
		"5215512345678@s.whatsapp.net":    "5215512345678",
		"5215512345678:12@s.whatsapp.net": "5215512345678",
		"5215512345678@lid":               "5215512345678",
		"  000000000000  ":                "000000000000",
		"":                                "",
		"sin números":                     "",
	}
	for in, want := range cases {
		if got := NormalizeNumber(in); got != want {
			t.Errorf("NormalizeNumber(%q) = %q, se esperaba %q", in, got, want)
		}
	}
}

func TestNormalizeOwners(t *testing.T) {
	got := NormalizeOwners([]string{"+52 155 1234 5678", "5215512345678@s.whatsapp.net", "", "  ", "5210000000000"})
	if len(got) != 2 {
		t.Fatalf("owners = %v, se esperaban 2 sin duplicados", got)
	}
	if got[0] != "5215512345678" {
		t.Fatalf("primer owner = %q", got[0])
	}
}

// El caso que rompía el JS: en el .env el número va con "+" y el JID llega
// limpio, o el chat viene por LID y el teléfono real está en SenderAlt.
func TestIsOwnerPhoneAndLID(t *testing.T) {
	owners := NormalizeOwners([]string{"+5215512345678"})

	phone, _ := types.ParseJID("5215512345678@s.whatsapp.net")
	if !IsOwner(phone, types.JID{}, owners) {
		t.Fatal("el owner por teléfono no se reconoció")
	}

	lid, _ := types.ParseJID("123456789012345@lid")
	if !IsOwner(lid, phone, owners) {
		t.Fatal("el owner por LID (teléfono en el alternativo) no se reconoció")
	}

	other, _ := types.ParseJID("5219999999999@s.whatsapp.net")
	if IsOwner(other, types.JID{}, owners) {
		t.Fatal("un número que no es owner pasó como owner")
	}
	if IsOwner(phone, types.JID{}, nil) {
		t.Fatal("sin owners nadie debería ser owner")
	}
}
