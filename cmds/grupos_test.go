// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// grupos_test.go — Comprobaciones de los comandos de grupo que se pueden hacer
// sin WhatsApp: el registro, las comparaciones de identidad (que es donde el
// JS fallaba), las plantillas de bienvenida y los topes.
package cmds

import (
	"strings"
	"testing"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/bot"
)

// ─── registro ───────────────────────────────────────────────────────────────

func TestComandosDeGrupoRegistrados(t *testing.T) {
	esperados := []string{
		"warn", "warns", "delwarn", "setwarnlimit",
		"kick", "promote", "demote",
		"open", "close", "link", "revoke",
		"tagall", "hidetag",
		"groupinfo", "setgpname", "setgpdesc", "setgpbanner",
		"count", "topcount", "topinactive",
		"bot", "antistatus", "welcome", "goodbye", "setwelcome", "setgoodbye",
	}
	for _, nombre := range esperados {
		cmd, ok := bot.Lookup(nombre)
		if !ok {
			t.Fatalf("falta el comando %q", nombre)
		}
		if cmd.Category != bot.CatGroup {
			t.Fatalf("%q está en la categoría %q", nombre, cmd.Category)
		}
		if !cmd.GroupOnly {
			t.Fatalf("%q debería ser solo para grupos", nombre)
		}
		if cmd.Desc == "" {
			t.Fatalf("%q no tiene descripción", nombre)
		}
	}
}

// Los comandos que solo debería usar un admin tienen que estar marcados; si no,
// cualquier miembro puede expulsar a otro.
func TestPermisosDeLosComandosDeGrupo(t *testing.T) {
	// Estos los protege el propio handler (necesitan comprobar también si el
	// BOT es admin), así que aquí solo se comprueba que existen y son de grupo.
	porHandler := []string{"warn", "warns", "delwarn", "setwarnlimit", "kick",
		"promote", "demote", "open", "close", "link", "revoke", "tagall",
		"hidetag", "setgpname", "setgpdesc", "setgpbanner", "bot", "antistatus",
		"welcome", "goodbye", "setwelcome", "setgoodbye"}

	for _, nombre := range porHandler {
		cmd, ok := bot.Lookup(nombre)
		if !ok {
			t.Fatalf("falta %q", nombre)
		}
		if !cmd.GroupOnly {
			t.Fatalf("%q tiene que ser GroupOnly", nombre)
		}
		if cmd.Run == nil {
			t.Fatalf("%q no tiene handler", nombre)
		}
	}

	// Y los de consulta no pueden estar restringidos: .count lo usa cualquiera.
	for _, nombre := range []string{"count", "topcount"} {
		cmd, _ := bot.Lookup(nombre)
		if cmd.AdminOnly || cmd.OwnerOnly {
			t.Fatalf("%q no debería estar restringido", nombre)
		}
	}
}

// ─── identidad: a quién se le está hablando ─────────────────────────────────

// La comparación de JID es el punto donde el bot de JS se equivocaba: el mismo
// usuario llega como teléfono o como LID y comparar cadenas a pelo falla.
func TestSameUser(t *testing.T) {
	anaPN := types.NewJID("5215500000101", types.DefaultUserServer)
	anaLID := types.NewJID("198765432101", types.HiddenUserServer)

	casos := []struct {
		nombre string
		a, b   types.JID
		igual  bool
	}{
		{"mismo jid", anaPN, anaPN, true},
		{"teléfono contra sí mismo", anaPN, types.NewJID("5215500000101", types.DefaultUserServer), true},
		{"LID contra sí mismo", anaLID, types.NewJID("198765432101", types.HiddenUserServer), true},
		{"dos usuarios distintos", anaPN, types.NewJID("5215500000102", types.DefaultUserServer), false},
		{"vacío contra algo", types.EmptyJID, anaPN, false},
		{"los dos vacíos", types.EmptyJID, types.EmptyJID, false},
	}
	for _, caso := range casos {
		if got := sameUser(caso.a, caso.b); got != caso.igual {
			t.Fatalf("%s: sameUser = %v, se esperaba %v", caso.nombre, got, caso.igual)
		}
	}

	// El teléfono con y sin el "9" que mete WhatsApp al registrarse en algunos
	// países se considera la misma persona (se comparan los últimos dígitos).
	conNueve := types.NewJID("5215551234567", types.DefaultUserServer)
	sinNueve := types.NewJID("525551234567", types.DefaultUserServer)
	if !sameUser(conNueve, sinNueve) {
		t.Fatal("el mismo número con y sin el 9 no se reconoció")
	}
}

// ─── plantillas de bienvenida ───────────────────────────────────────────────

func TestTextoPlantilla(t *testing.T) {
	plantilla := "👋 Hola @user, bienvenido a #grupo (#miembros miembros)."
	got := textoPlantilla(plantilla, "Los Roxis", 42, "@5215500000101")
	esperado := "👋 Hola @5215500000101, bienvenido a Los Roxis (42 miembros)."
	if got != esperado {
		t.Fatalf("textoPlantilla =\n%q\nse esperaba\n%q", got, esperado)
	}

	// #nombre es alias de #grupo, como en el JS.
	if got := textoPlantilla("#nombre", "X", 1, "@a"); got != "X" {
		t.Fatalf("alias #nombre = %q", got)
	}

	// Una plantilla gigante se recorta: si no, cada entrada al grupo manda
	// 40 KB de texto y WhatsApp lo corta a su manera.
	gigante := strings.Repeat("x", 5000)
	if n := len([]rune(textoPlantilla(gigante, "X", 1, "@a"))); n != 900 {
		t.Fatalf("la plantilla gigante quedó en %d caracteres", n)
	}

	// Sin marcadores no se toca nada.
	if got := textoPlantilla("hola", "X", 1, "@a"); got != "hola" {
		t.Fatalf("se modificó un texto sin marcadores: %q", got)
	}
}

// ─── argumentos ─────────────────────────────────────────────────────────────

func TestSkipMention(t *testing.T) {
	args := []string{"@5215500000101", "spam", "en", "el", "grupo"}
	got := skipMention(args)
	if len(got) != 4 || got[0] != "spam" {
		t.Fatalf("skipMention = %v", got)
	}
	if len(skipMention(nil)) != 0 {
		t.Fatal("skipMention(nil) no devolvió vacío")
	}
	// Un arroba en medio del texto no es una mención al principio: se respeta.
	if got := skipMention([]string{"hola", "@todos"}); len(got) != 1 || got[0] != "hola" {
		t.Fatalf("skipMention con @ al final = %v", got)
	}
}

func TestArgAt(t *testing.T) {
	args := []string{"a", "b"}
	if argAt(args, 0) != "a" || argAt(args, 1) != "b" {
		t.Fatal("argAt devolvió lo que no era")
	}
	if argAt(args, 5) != "" || argAt(args, -1) != "" {
		t.Fatal("argAt fuera de rango debería devolver vacío")
	}
}

func TestFecha(t *testing.T) {
	if fecha(0) != "sin fecha" {
		t.Fatalf("fecha(0) = %q", fecha(0))
	}
	if got := fecha(1_700_000_000_000); got == "" || got == "sin fecha" {
		t.Fatalf("fecha válida = %q", got)
	}
}

// ─── avisos ─────────────────────────────────────────────────────────────────

// Las plantillas por defecto tienen que usar los marcadores que existen, o la
// bienvenida saldría con "#grupo" escrito tal cual.
func TestPlantillasPorDefecto(t *testing.T) {
	for nombre, plantilla := range map[string]string{
		"bienvenida": bienvenidaPorDefecto,
		"despedida":  despedidaPorDefecto,
	} {
		if !strings.Contains(plantilla, "@user") {
			t.Fatalf("la %s no menciona a @user", nombre)
		}
		if !strings.Contains(plantilla, "#grupo") {
			t.Fatalf("la %s no usa #grupo", nombre)
		}
		if len([]rune(plantilla)) > MaxPlantilla {
			t.Fatalf("la %s pasa del tope de %d caracteres", nombre, MaxPlantilla)
		}
	}
}

// Sin router no hay dueño reconocido: la comprobación no puede decir "sí" por
// defecto, o cualquiera pasaría por owner y podría expulsar.
func TestIsBotOwnerSinRouter(t *testing.T) {
	quien := types.NewJID("5215500000101", types.DefaultUserServer)
	if bot.IsBotOwner(quien) {
		t.Fatal("sin router, IsBotOwner debe ser false")
	}
	if bot.IsBotOwner(types.EmptyJID) {
		t.Fatal("un JID vacío nunca es el dueño")
	}
}
