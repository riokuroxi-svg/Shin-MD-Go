// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package cmds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// El límite arquitectónico pedido: cmds/ llama a ReplyMenu/ReplyButtons/
// ReplyList/ReplyCarousel (bot.Messenger). No conoce protobufs, nodos biz, el
// paquete concreto ni wa.Client.Send*. Si alguien se salta el puerto, falla CI.
func TestComandosNoSaltanElPuertoInteractivo(t *testing.T) {
	archivos, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	prohibido := []string{
		"shin-go/internal/interactivos",
		"go.mau.fi/whatsmeow/proto/waE2E",
		".Bot.SendMenu(",
		".Bot.SendButtons(",
		".Bot.SendButtonsImage(",
		".Bot.SendList(",
		".Bot.SendCarousel(",
		"InteractiveMessage{",
		"Tag: \"biz\"",
	}
	for _, path := range archivos {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, marca := range prohibido {
			if strings.Contains(string(raw), marca) {
				t.Errorf("%s se salta bot.Messenger: contiene %q", path, marca)
			}
		}
	}
}
