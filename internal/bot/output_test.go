// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package bot

import (
	"strings"
	"testing"
)

// El nombre del perfil es texto libre: si trae formato, el bot no debe
// repetirlo tal cual (se pueden falsificar avisos).
func TestCleanNameStripsFormatAndInvisibles(t *testing.T) {
	casos := []struct {
		in   string
		want string
	}{
		{"Shin_Admin", "Shin‗Admin"},
		{"*Shin*", "∗Shin∗"},
		{"Roxi\ndel mal", "Roxi del mal"},
		{"nombre\u200boculto", "nombreoculto"},
		{"   muchos    espacios   ", "muchos espacios"},
		{"", ""},
	}
	for _, c := range casos {
		if got := CleanName(c.in, 32); got != c.want {
			t.Errorf("CleanName(%q) = %q, se esperaba %q", c.in, got, c.want)
		}
	}
}

func TestCleanNameTruncates(t *testing.T) {
	largo := strings.Repeat("a", 100)
	got := CleanName(largo, 10)
	if len([]rune(got)) > 11 { // 10 + la "…"
		t.Fatalf("CleanName no recortó: %q", got)
	}
}

// Lo que el usuario escribe y el bot repite (motivo de afk, reporte) tampoco
// puede traer formato ni caracteres de control.
func TestClipText(t *testing.T) {
	if got := ClipText("hola *mundo*", 100); got != "hola ∗mundo∗" {
		t.Fatalf("ClipText no escapó el formato: %q", got)
	}
	if got := ClipText("a\x00b\x01c", 100); got != "abc" {
		t.Fatalf("ClipText no quitó controles: %q", got)
	}
	largo := strings.Repeat("x", 500)
	if n := len([]rune(ClipText(largo, 50))); n > 51 {
		t.Fatalf("ClipText no recortó: %d caracteres", n)
	}
	if got := ClipText("   ", 10); got != "" {
		t.Fatalf("ClipText de solo espacios = %q", got)
	}
}

// Un texto enorme no puede salir en un solo mensaje (WhatsApp lo corta o lo
// rechaza); Chunk lo parte sin perder nada.
func TestChunk(t *testing.T) {
	texto := strings.Repeat("línea de prueba\n", 1000) // ~15 000 caracteres
	trozos := Chunk(texto, 3500)

	if len(trozos) < 2 {
		t.Fatalf("debería partirse en varios trozos, salió %d", len(trozos))
	}
	for i, tz := range trozos {
		if n := len([]rune(tz)); n > 3500 {
			t.Fatalf("el trozo %d mide %d caracteres", i, n)
		}
	}
	// Nada se pierde: quitando los saltos que añade el corte, el contenido es
	// el mismo.
	junto := strings.ReplaceAll(strings.Join(trozos, ""), "\n", "")
	original := strings.ReplaceAll(texto, "\n", "")
	if junto != original {
		t.Fatalf("Chunk perdió texto: %d vs %d caracteres", len(junto), len(original))
	}

	if got := Chunk("corto", 3500); len(got) != 1 || got[0] != "corto" {
		t.Fatalf("Chunk tocó un texto corto: %v", got)
	}
}

// Los nombres de comando solo admiten letras, números, guion y guion bajo.
func TestIsSafeCommandName(t *testing.T) {
	buenos := []string{"menu", "daily", "antilink", "afk", "a", "cmd-2", "snake_case"}
	for _, n := range buenos {
		if !IsSafeCommandName(n) {
			t.Errorf("%q debería ser válido", n)
		}
	}
	malos := []string{"", " ", "con espacio", "../etc", "a/b", "cmd;rm", "cmd'x", strings.Repeat("a", 33), "cmd\n"}
	for _, n := range malos {
		if IsSafeCommandName(n) {
			t.Errorf("%q no debería ser válido", n)
		}
	}
}

// El texto que se analiza (para enlaces) pierde invisibles, pero el que se
// muestra conserva la intención del usuario salvo el formato.
func TestCleanInvisible(t *testing.T) {
	if got := CleanInvisible("h\u200bt\u200dtps://x.com"); got != "https://x.com" {
		t.Fatalf("CleanInvisible = %q", got)
	}
	if got := CleanInvisible("a\x00b"); got != "ab" {
		t.Fatalf("CleanInvisible dejó un control: %q", got)
	}
	if got := CleanInvisible(""); got != "" {
		t.Fatalf("CleanInvisible(\"\") = %q", got)
	}
}
