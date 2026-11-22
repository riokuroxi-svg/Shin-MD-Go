// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package wa

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

func TestReadReceiptGroupsAreBatchedPerSender(t *testing.T) {
	base := IncomingMessage{
		Chat: NewJID("grupo", GroupServer), ID: "m1", IsGroup: true, HasContent: true,
	}
	a := base
	a.Sender = NewJID("111", DefaultUserServer)
	b := base
	b.ID = "m2"
	b.Sender = NewJID("222", DefaultUserServer)
	keyA, chatA, senderA, ok := readReceiptTarget(&a)
	if !ok {
		t.Fatal("se rechazó un mensaje de grupo válido")
	}
	keyB, _, senderB, ok := readReceiptTarget(&b)
	if !ok {
		t.Fatal("se rechazó el segundo mensaje de grupo")
	}
	if keyA == keyB {
		t.Fatal("dos remitentes del grupo caerían en el mismo MarkRead")
	}
	if chatA.Server != GroupServer || senderA.User != "111" || senderB.User != "222" {
		t.Fatalf("destinos incorrectos: chat=%v senderA=%v senderB=%v", chatA, senderA, senderB)
	}
}

func TestReadReceiptPrivateDoesNotSetParticipant(t *testing.T) {
	msg := &IncomingMessage{
		Chat: NewJID("111", DefaultUserServer), Sender: NewJID("111", DefaultUserServer),
		ID: "m1", HasContent: true,
	}
	_, _, sender, ok := readReceiptTarget(msg)
	if !ok || !sender.IsEmpty() {
		t.Fatalf("privado: ok=%v sender=%v", ok, sender)
	}
}

func TestReadReceiptRejectsOwnAndStatusMessages(t *testing.T) {
	cases := []*IncomingMessage{
		{Chat: NewJID("111", DefaultUserServer), ID: "x", HasContent: true, IsFromMe: true},
		{Chat: NewJID("status", BroadcastServer), ID: "x", HasContent: true},
		{Chat: NewJID("111", DefaultUserServer), HasContent: true},
	}
	for i, msg := range cases {
		if _, _, _, ok := readReceiptTarget(msg); ok {
			t.Errorf("caso %d no debía generar recibo", i)
		}
	}
}

func TestComposingUsesReferencesAndEndsPaused(t *testing.T) {
	events := make(chan types.ChatPresence, 4)
	c := &Client{
		baseCtx: t.Context(),
		presenceSender: func(_ context.Context, _ JID, state types.ChatPresence) {
			events <- state
		},
	}
	chat := NewJID("111", DefaultUserServer)
	stopA := c.BeginComposing(t.Context(), chat)
	stopB := c.BeginComposing(t.Context(), chat)
	wantPresence(t, events, types.ChatPresenceComposing)

	stopA()
	select {
	case got := <-events:
		t.Fatalf("la primera referencia no debía cambiar presencia: %v", got)
	case <-time.After(30 * time.Millisecond):
	}

	stopB()
	wantPresence(t, events, types.ChatPresencePaused)
}

func TestComposingDoesNotPauseANewerGeneration(t *testing.T) {
	events := make(chan types.ChatPresence, 8)
	pauseEntered := make(chan struct{})
	releasePause := make(chan struct{})
	var pauses atomic.Int32
	c := &Client{
		baseCtx: t.Context(),
		presenceSender: func(_ context.Context, _ JID, state types.ChatPresence) {
			if state == types.ChatPresencePaused && pauses.Add(1) == 1 {
				close(pauseEntered)
				<-releasePause
			}
			events <- state
		},
	}
	chat := NewJID("222", DefaultUserServer)
	stopOld := c.BeginComposing(t.Context(), chat)
	wantPresence(t, events, types.ChatPresenceComposing)
	stopOld()
	select {
	case <-pauseEntered:
	case <-time.After(time.Second):
		t.Fatal("el primer paused no comenzó")
	}

	newStop := make(chan func(), 1)
	go func() { newStop <- c.BeginComposing(t.Context(), chat) }()
	select {
	case <-newStop:
		t.Fatal("la generación nueva se adelantó al paused anterior")
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case got := <-events:
		t.Fatalf("se emitió presencia mientras paused estaba bloqueado: %v", got)
	default:
	}

	close(releasePause)
	wantPresence(t, events, types.ChatPresencePaused)
	var stopNew func()
	select {
	case stopNew = <-newStop:
	case <-time.After(time.Second):
		t.Fatal("la generación nueva no arrancó al terminar paused")
	}
	wantPresence(t, events, types.ChatPresenceComposing)
	stopNew()
	wantPresence(t, events, types.ChatPresencePaused)
}

func wantPresence(t *testing.T, events <-chan types.ChatPresence, want types.ChatPresence) {
	t.Helper()
	select {
	case got := <-events:
		if got != want {
			t.Fatalf("presencia: got %v, want %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("no llegó presencia %v", want)
	}
}
