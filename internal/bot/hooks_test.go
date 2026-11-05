// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package bot

import (
	"io"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waE2E"

	"shin-go/internal/wa"
	"shin-go/internal/xlog"
)

func testLogger() *xlog.Logger { return xlog.New("error", io.Discard) }

func parseText(text string) *waE2E.Message {
	return &waE2E.Message{Conversation: proto.String(text)}
}

// Un hook que dice "ya lo atendí" corta la cadena: el comando no se ejecuta.
func TestHooksStopChain(t *testing.T) {
	ResetHooks()
	t.Cleanup(ResetHooks)

	var order []string

	RegisterHook("test_uno", func(*Context, wa.Parsed) bool {
		order = append(order, "uno")
		return true // atiende el mensaje
	})
	RegisterHook("test_dos", func(*Context, wa.Parsed) bool {
		order = append(order, "dos")
		return false
	})

	r := &Router{}
	c := &Context{}
	if !r.RunHooks(c, wa.Parsed{Text: "hola"}) {
		t.Fatal("el router debería saber que el mensaje ya fue atendido")
	}
	if len(order) != 1 || order[0] != "uno" {
		t.Fatalf("cadena ejecutada = %v, se esperaba solo el primero", order)
	}
}

// Un pánico dentro de un hook no puede tumbar el bot ni romper el resto.
func TestHooksRecoverPanic(t *testing.T) {
	ResetHooks()
	t.Cleanup(ResetHooks)

	RegisterHook("test_explosivo", func(*Context, wa.Parsed) bool {
		panic("boom")
	})
	RegisterHook("test_final", func(*Context, wa.Parsed) bool {
		return false
	})

	r := &Router{}
	c := &Context{Log: testLogger()}
	if r.RunHooks(c, wa.Parsed{}) {
		t.Fatal("un hook que revienta no debe marcar el mensaje como atendido")
	}
}

// Los hooks ven también los mensajes que no son comandos: es lo que necesitan
// el antilink y el afk.
func TestHooksSeePlainText(t *testing.T) {
	ResetHooks()
	t.Cleanup(ResetHooks)

	var sawPlain bool
	RegisterHook("test_texto", func(_ *Context, p wa.Parsed) bool {
		if p.Text == "mira wa.me/123" && !p.IsCommand {
			sawPlain = true
		}
		return false
	})

	r := &Router{}
	r.RunHooks(&Context{Log: testLogger()}, wa.ParseMessage(nil, nil))
	r.RunHooks(&Context{Log: testLogger()}, wa.ParseMessage(parseText("mira wa.me/123"), []string{"."}))
	if !sawPlain {
		t.Fatal("el hook no recibió el texto plano")
	}
}
