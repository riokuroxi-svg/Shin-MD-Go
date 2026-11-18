// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package wa

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestNormalizeMessageKeepsConsumedMetadata(t *testing.T) {
	chat := types.NewJID("123-456", types.GroupServer)
	sender := types.NewJID("12345", types.HiddenUserServer)
	alt := types.NewJID("5215512345678", types.DefaultUserServer)
	quotedAuthor := types.NewJID("5215598765432", types.DefaultUserServer)
	ts := time.Date(2026, 11, 17, 12, 30, 0, 0, time.UTC)
	evt := &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{Chat: chat, Sender: sender, SenderAlt: alt, IsGroup: true},
			ID:            "msg-1", Timestamp: ts, PushName: "Río",
		},
		Message: &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(".ping"),
			ContextInfo: &waE2E.ContextInfo{
				MentionedJID: []string{quotedAuthor.String()},
				Participant:  proto.String(quotedAuthor.String()),
				StanzaID:     proto.String("quoted-7"),
				QuotedMessage: &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
					Caption: proto.String("foto citada"),
				}},
			},
		}},
	}

	got := NormalizeMessage(evt, []string{"."})
	if got == nil || got.ID != "msg-1" || !got.IsGroup || !got.HasContent {
		t.Fatalf("metadatos básicos perdidos: %+v", got)
	}
	if got.Chat.String() != chat.String() || got.Sender.String() != sender.String() || got.SenderAlt.String() != alt.String() {
		t.Fatalf("JID normalizados incorrectos: chat=%s sender=%s alt=%s", got.Chat, got.Sender, got.SenderAlt)
	}
	if got.Timestamp != ts || got.PushName != "Río" || !got.Parsed.IsCommand || got.Parsed.Command != "ping" {
		t.Fatalf("texto o metadatos de comando incorrectos: %+v", got)
	}
	if len(got.Mentions) != 1 || got.Mentions[0].String() != quotedAuthor.String() {
		t.Fatalf("menciones/participante perdidos: %v", got.Mentions)
	}
	if got.Quoted == nil || got.Quoted.ID != "quoted-7" || got.Quoted.Participant.String() != quotedAuthor.String() || !got.Quoted.HasImage || got.Quoted.Text != "foto citada" {
		t.Fatalf("metadatos de cita incompletos: %+v", got.Quoted)
	}
	if medio, ok := MedioCitado(got); !ok || medio.Tipo != MedioImagen {
		t.Fatalf("el handle opaco no conservó el medio citado: %+v, ok=%t", medio, ok)
	}
}

func TestNormalizeMessageNilAndEmptyPayload(t *testing.T) {
	if NormalizeMessage(nil, nil) != nil {
		t.Fatal("nil debe seguir siendo nil")
	}
	got := NormalizeMessage(&events.Message{}, nil)
	if got == nil || got.HasContent || !got.Chat.IsEmpty() {
		t.Fatalf("evento vacío mal normalizado: %+v", got)
	}
}
