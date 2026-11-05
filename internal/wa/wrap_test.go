// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package wa

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

// Un comando escrito en un chat con mensajes efímeros llega envuelto. Antes
// se ignoraba en silencio (el bot "no contestaba" y no había ningún error).
func TestParseCommandInsideEphemeral(t *testing.T) {
	msg := &waE2E.Message{
		EphemeralMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{Conversation: proto.String(".ping")},
		},
	}
	p := ParseMessage(msg, prefixes)
	if !p.IsCommand || p.Command != "ping" {
		t.Fatalf("comando dentro de efímero mal leído: %+v", p)
	}
}

// Los envoltorios se anidan (efímero + ver una vez + edición).
func TestParseCommandDeeplyWrapped(t *testing.T) {
	inner := &waE2E.Message{Conversation: proto.String(".menu economia")}
	msg := &waE2E.Message{
		EphemeralMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				ViewOnceMessageV2: &waE2E.FutureProofMessage{
					Message: &waE2E.Message{
						EditedMessage: &waE2E.FutureProofMessage{Message: inner},
					},
				},
			},
		},
	}
	p := ParseMessage(msg, prefixes)
	if !p.IsCommand || p.Command != "menu" || len(p.Args) != 1 || p.Args[0] != "economia" {
		t.Fatalf("mensaje anidado mal leído: %+v", p)
	}
}

// Un envoltorio vacío o mal formado no debe colgar ni reventar.
func TestUnwrapBounded(t *testing.T) {
	msg := &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{}}
	if got := Unwrap(msg); got == nil {
		t.Fatal("Unwrap devolvió nil con un envoltorio vacío")
	}
	if got := Unwrap(nil); got != nil {
		t.Fatal("Unwrap(nil) debería ser nil")
	}

	chain := &waE2E.Message{Conversation: proto.String("hola")}
	for i := 0; i < 40; i++ {
		chain = &waE2E.Message{EphemeralMessage: &waE2E.FutureProofMessage{Message: chain}}
	}
	if got := Unwrap(chain); got == nil {
		t.Fatal("Unwrap devolvió nil con una cadena larga")
	}
}

// El mensaje citado de una FOTO vive en ImageMessage. Mirar solo
// ExtendedTextMessage hacía que .pin dijera "no respondiste a nada".
func TestContextInfoFromMedia(t *testing.T) {
	msg := &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Caption:     proto.String("mira"),
			ContextInfo: &waE2E.ContextInfo{StanzaID: proto.String("3EB0ABCD")},
		},
	}
	ci := ContextInfo(msg)
	if ci == nil || ci.GetStanzaID() != "3EB0ABCD" {
		t.Fatalf("no se encontró el citado de la imagen: %+v", ci)
	}

	if got := ContextInfo(&waE2E.Message{Conversation: proto.String(".pin")}); got != nil {
		t.Fatalf("un mensaje sin citado devolvió %+v", got)
	}
	if got := ContextInfo(nil); got != nil {
		t.Fatal("ContextInfo(nil) debería ser nil")
	}
}

// Y también cuando la foto viene dentro de un efímero.
func TestContextInfoInsideEphemeral(t *testing.T) {
	msg := &waE2E.Message{
		EphemeralMessage: &waE2E.FutureProofMessage{
			Message: &waE2E.Message{
				VideoMessage: &waE2E.VideoMessage{
					ContextInfo: &waE2E.ContextInfo{
						StanzaID:    proto.String("VID123"),
						Participant: proto.String("5215555555555@s.whatsapp.net"),
					},
				},
			},
		},
	}
	ci := ContextInfo(msg)
	if ci == nil || ci.GetStanzaID() != "VID123" {
		t.Fatalf("no se encontró el citado del vídeo efímero: %+v", ci)
	}
}
