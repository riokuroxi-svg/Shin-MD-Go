// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package ui

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waAICommonDeprecated"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

func TestRichMessages(t *testing.T) {
	text := RichText("# Hola **mundo**")
	rm := text.GetRichResponseMessage()
	if rm == nil || len(rm.GetSubmessages()) != 1 {
		t.Fatal("rich text mal armado")
	}
	if rm.GetSubmessages()[0].GetMessageType() != waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_TEXT {
		t.Fatal("tipo de submensaje incorrecto")
	}
	if rm.GetSubmessages()[0].GetMessageText() != "# Hola **mundo**" {
		t.Fatal("texto perdido")
	}

	code := RichCode("go", "fmt.Println(1)")
	if code.GetRichResponseMessage().GetSubmessages()[0].GetCodeMetadata().GetCodeLanguage() != "go" {
		t.Fatal("rich code mal armado")
	}

	tbl := RichTable("Niveles", []string{"user", "xp"}, [][]string{{"ana", "10"}})
	rows := tbl.GetRichResponseMessage().GetSubmessages()[0].GetTableMetadata().GetRows()
	if len(rows) != 2 || !rows[0].GetIsHeading() || rows[1].GetItems()[1] != "10" {
		t.Fatal("rich table mal armada")
	}

	if RichTextCard("txt", "go", "x").GetRichResponseMessage() == nil {
		t.Fatal("rich combinado")
	}
}

func TestBuildEventAndPin(t *testing.T) {
	start := time.Now()
	msg := BuildEvent("Reunión", "grupo", start, start.Add(time.Hour), true, 10*time.Minute)
	ev := msg.GetEventMessage()
	if ev == nil || ev.GetName() != "Reunión" || !ev.GetHasReminder() {
		t.Fatal("evento mal armado")
	}
	if ev.GetReminderOffsetSec() != 600 {
		t.Fatalf("recordatorio = %d s", ev.GetReminderOffsetSec())
	}

	pin := BuildPin(Key(chatJID(t), false, "ABC123", senderJID(t)), true)
	pm := pin.GetPinInChatMessage()
	if pm == nil || pm.GetType() != waE2E.PinInChatMessage_PIN_FOR_ALL || pm.GetKey().GetID() != "ABC123" {
		t.Fatal("pin mal armado")
	}
	unpin := BuildPin(Key(chatJID(t), false, "ABC123", senderJID(t)), false)
	if unpin.GetPinInChatMessage().GetType() != waE2E.PinInChatMessage_UNPIN_FOR_ALL {
		t.Fatal("unpin mal armado")
	}
}

func TestBuildPollAddOption(t *testing.T) {
	msg := BuildPollAddOption(Key(chatJID(t), true, "POLL1", senderJID(t)), "opción nueva")
	pa := msg.GetPollAddOptionMessage()
	if pa == nil || pa.GetAddOption().GetOptionName() != "opción nueva" {
		t.Fatal("poll add option mal armado")
	}
	if pa.GetPollCreationMessageKey().GetID() != "POLL1" {
		t.Fatal("clave de encuesta perdida")
	}
}

// ── helpers de prueba ────────────────────────────────────────────────────────

func privJID(t *testing.T) types.JID {
	t.Helper()
	j, err := types.ParseJID("5215555555555@s.whatsapp.net")
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func chatJID(t *testing.T) types.JID   { return privJID(t) }
func senderJID(t *testing.T) types.JID { return privJID(t) }
