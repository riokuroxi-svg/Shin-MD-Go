// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package wa

import (
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waE2E"

	"shin-go/internal/config"
)

var prefixes = []string{".", "/", "#", "!"}

func TestParseTextCommand(t *testing.T) {
	msg := &waE2E.Message{Conversation: proto.String(".play una canción")}
	p := ParseMessage(msg, prefixes)

	if !p.IsCommand || p.Command != "play" || len(p.Args) != 2 || p.Args[1] != "canción" {
		t.Fatalf("parse mal: %+v", p)
	}
	if p.Prefix != "." {
		t.Fatalf("prefijo = %q", p.Prefix)
	}
}

func TestParseNoPrefixIsNotCommand(t *testing.T) {
	msg := &waE2E.Message{Conversation: proto.String("hola a todos")}
	if p := ParseMessage(msg, prefixes); p.IsCommand {
		t.Fatal("no debería ser comando")
	}
}

func TestParseButtonResponse(t *testing.T) {
	msg := &waE2E.Message{
		ButtonsResponseMessage: &waE2E.ButtonsResponseMessage{
			SelectedButtonID: proto.String("menu:economia"),
		},
	}
	p := ParseMessage(msg, prefixes)
	if !p.IsButton || p.ButtonID != "menu:economia" {
		t.Fatalf("botón mal leído: %+v", p)
	}

	cmd, args := CommandFromButton(p.ButtonID)
	if cmd != "menu" || len(args) != 1 || args[0] != "economia" {
		t.Fatalf("CommandFromButton: %q %v", cmd, args)
	}
}

func TestParseListResponse(t *testing.T) {
	msg := &waE2E.Message{
		ListResponseMessage: &waE2E.ListResponseMessage{
			SingleSelectReply: &waE2E.ListResponseMessage_SingleSelectReply{
				SelectedRowID: proto.String("ping"),
			},
		},
	}
	p := ParseMessage(msg, prefixes)
	if !p.IsButton || p.ButtonID != "ping" {
		t.Fatalf("lista mal leída: %+v", p)
	}
}

func TestParseNativeFlowResponse(t *testing.T) {
	msg := &waE2E.Message{
		InteractiveResponseMessage: &waE2E.InteractiveResponseMessage{
			InteractiveResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage_{
				NativeFlowResponseMessage: &waE2E.InteractiveResponseMessage_NativeFlowResponseMessage{
					Name:       proto.String("single_select"),
					ParamsJSON: proto.String(`{"id":"menu:economia","display_text":"Economía"}`),
					Version:    proto.Int32(1),
				},
			},
		},
	}
	p := ParseMessage(msg, prefixes)
	if !p.IsButton || p.ButtonID != "menu:economia" {
		t.Fatalf("native_flow mal leído: %+v", p)
	}
	cmd, args := CommandFromButton(p.ButtonID)
	if cmd != "menu" || len(args) != 1 || args[0] != "economia" {
		t.Fatalf("respuesta native_flow no enrutable: %q %v", cmd, args)
	}
}

// El jitter debe caer siempre dentro de los límites duros.
func TestQueueDelayWithinBounds(t *testing.T) {
	q := NewQueue(testConfig())
	for i := 0; i < 500; i++ {
		d := q.delay(i * 12)
		if d < 400*1000*1000 || d > 5e9 {
			t.Fatalf("delay fuera de rango: %s", d)
		}
	}
}

func testConfig() config.Config {
	return config.Config{
		BaseDelay:   1200 * time.Millisecond,
		JitterSigma: 0.25,
		PerCharMS:   25,
		MaxDelay:    5 * time.Second,
		WarmupStart: 20,
		WarmupEnd:   500,
	}
}
