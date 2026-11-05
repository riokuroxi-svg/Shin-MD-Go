// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package bot

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/wa"
)

func jid(t *testing.T, s string) types.JID {
	t.Helper()
	j, err := types.ParseJID(s)
	if err != nil {
		t.Fatalf("JID inválido %q: %v", s, err)
	}
	return j
}

// El caso que rompía los permisos: el grupo usa LID en los participantes y el
// mensaje llega con teléfono (o al revés). Comparar cadenas a pelo fallaba en
// los dos sentidos: admin tratado como usuario normal y al revés.
func TestParticipantIsAdminAcrossLID(t *testing.T) {
	ctx := context.Background()
	ids := wa.NewResolver(nil, nil)

	pnAdmin := jid(t, "5215512345678@s.whatsapp.net")
	lidAdmin := jid(t, "111111111111111@lid")
	pnUser := jid(t, "5215599999999@s.whatsapp.net")

	info := &types.GroupInfo{
		JID: jid(t, "123-456@g.us"),
		Participants: []types.GroupParticipant{
			// el admin aparece con LID + teléfono, como lo manda WhatsApp
			{JID: lidAdmin, LID: lidAdmin, PhoneNumber: pnAdmin, IsAdmin: true},
			{JID: pnUser, PhoneNumber: pnUser},
		},
	}

	if !participantIsAdmin(ctx, ids, info, pnAdmin, types.EmptyJID) {
		t.Error("el admin por teléfono no fue reconocido")
	}
	if !participantIsAdmin(ctx, ids, info, lidAdmin, types.EmptyJID) {
		t.Error("el admin por LID no fue reconocido")
	}
	if participantIsAdmin(ctx, ids, info, pnUser, types.EmptyJID) {
		t.Error("un participante normal pasó como admin")
	}

	// Y cuando el remitente llega por LID con el teléfono en el alternativo.
	if !participantIsAdmin(ctx, ids, info, lidAdmin, pnAdmin) {
		t.Error("admin por LID con alternativo no reconocido")
	}
}

// El creador del grupo (superadmin) también manda.
func TestParticipantSuperAdmin(t *testing.T) {
	ctx := context.Background()
	ids := wa.NewResolver(nil, nil)
	creador := jid(t, "5215500000001@s.whatsapp.net")

	info := &types.GroupInfo{
		Participants: []types.GroupParticipant{
			{JID: creador, IsSuperAdmin: true},
		},
	}
	if !participantIsAdmin(ctx, ids, info, creador, types.EmptyJID) {
		t.Fatal("el creador del grupo debería contar como admin")
	}
}

func TestParticipantIsAdminEdgeCases(t *testing.T) {
	ctx := context.Background()
	ids := wa.NewResolver(nil, nil)

	if participantIsAdmin(ctx, ids, nil, jid(t, "1@s.whatsapp.net"), types.EmptyJID) {
		t.Fatal("sin información del grupo no se puede afirmar que alguien es admin")
	}
	if participantIsAdmin(ctx, ids, &types.GroupInfo{}, types.EmptyJID, types.EmptyJID) {
		t.Fatal("un remitente vacío no es admin")
	}
}
