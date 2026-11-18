// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// utilidades_test.go — lo de la primera tanda de utilidades que se puede
// probar sin abrir WhatsApp: el mensaje de encuesta que se manda de verdad
// (quiz incluido), los atajos de texto y la resolución de a quién se refiere
// un comando.
package cmds

import (
	"testing"

	"shin-go/internal/bot"
	"shin-go/internal/utilidades"
	"shin-go/internal/wa"
)

// ─── .encuesta: contrato interno entre comando y adaptador ───────────────────

func TestEncuestaSpecNormal(t *testing.T) {
	e, err := utilidades.ParsearEncuesta("¿Pizza?|Pepperoni|Hawaiana|Mexicana")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	spec := encuestaSpec(e)
	if spec.Question != "¿Pizza?" || spec.Selectable != 1 || spec.Quiz || spec.Correct != -1 {
		t.Fatalf("spec inesperado: %+v", spec)
	}
	want := []string{"Pepperoni", "Hawaiana", "Mexicana"}
	if len(spec.Options) != len(want) {
		t.Fatalf("opciones = %v", spec.Options)
	}
	for i := range want {
		if spec.Options[i] != want[i] {
			t.Fatalf("opción %d = %q", i, spec.Options[i])
		}
	}
}

func TestEncuestaSpecQuizYMulti(t *testing.T) {
	e, err := utilidades.ParsearEncuesta("Capital de México|Guadalajara|*CDMX*|Monterrey")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	spec := encuestaSpec(e)
	if !spec.Quiz || spec.Correct != 1 || spec.Options[spec.Correct] != "CDMX" {
		t.Fatalf("quiz no conserva respuesta correcta: %+v", spec)
	}
	e, err = utilidades.ParsearEncuesta("multi ¿Qué llevas?|Hielo|Vasos|Botana")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	if spec = encuestaSpec(e); spec.Selectable != 3 || spec.Quiz {
		t.Fatalf("selección múltiple inesperada: %+v", spec)
	}
}

// ─── atajos de texto ────────────────────────────────────────────────────────

func TestEsTexto(t *testing.T) {
	texto := []string{"text/plain; charset=utf-8", "application/json", "TEXT/HTML",
		"application/xml", "application/javascript"}
	for _, tipo := range texto {
		if !esTexto(tipo) {
			t.Fatalf("%q debería ir como texto", tipo)
		}
	}
	binario := []string{"image/png", "application/pdf", "application/octet-stream", ""}
	for _, tipo := range binario {
		if esTexto(tipo) {
			t.Fatalf("%q no debería ir como texto", tipo)
		}
	}
}

func TestNombreDeArchivo(t *testing.T) {
	casos := map[string]string{
		"/ruta/foto.jpg":         "foto.jpg",
		"/ruta/":                 "archivo.bin",
		"":                       "archivo.bin",
		"/ruta/sin-nombre":       "sin-nombre",
		"https://x.test/a/b.pdf": "b.pdf",
	}
	for ruta, quiero := range casos {
		if got := nombreDeArchivo(ruta, "application/octet-stream"); got != quiero {
			t.Fatalf("%q → %q, se esperaba %q", ruta, got, quiero)
		}
	}
	// Sin nombre en la ruta, se elige por tipo.
	for tipo, quiero := range map[string]string{
		"image/png": "archivo.png", "application/pdf": "archivo.pdf",
		"audio/ogg": "archivo.ogg", "video/mp4": "archivo.mp4",
	} {
		if got := nombreDeArchivo("/", tipo); got != quiero {
			t.Fatalf("%q → %q, se esperaba %q", tipo, got, quiero)
		}
	}
}

func TestRecortarPorRunas(t *testing.T) {
	if got := recortar("hola", 10); got != "hola" {
		t.Fatalf("cortó de más: %q", got)
	}
	// Cuatro emojis: cortar por bytes partiría uno por la mitad.
	got := recortar("😀😀😀😀", 2)
	if runas := []rune(got); len(runas) != 3 { // dos emojis + el puntos suspensivos
		t.Fatalf("%q → %d runas", got, len(runas))
	}
	if len([]rune(recortar("abc", 0))) != 1 {
		t.Fatal("con máximo 0 solo debería quedar el puntos suspensivos")
	}
}

func TestSoloDigitos(t *testing.T) {
	casos := map[string]string{
		"+52 55 1234 5678": "525512345678",
		"tel:+1-555-0100":  "15550100",
		"abc":              "",
	}
	for entrada, quiero := range casos {
		if got := soloDigitos(entrada); got != quiero {
			t.Fatalf("%q → %q, se esperaba %q", entrada, got, quiero)
		}
	}
}

// ─── a quién se refiere ─────────────────────────────────────────────────────

func mencionarCon(jids ...wa.JID) *bot.Context {
	evt := &wa.IncomingMessage{Parsed: wa.Parsed{Text: ".algo"}, Mentions: append([]wa.JID(nil), jids...)}
	return &bot.Context{Event: evt}
}

func TestPersonaObjetivoUsaLaMención(t *testing.T) {
	mencionado := wa.NewJID("5215512345678", wa.DefaultUserServer)
	c := mencionarCon(mencionado)
	got, ok := personaObjetivo(c)
	if !ok {
		t.Fatal("debería haber encontrado a alguien")
	}
	if got.String() != mencionado.String() {
		t.Fatalf("jid = %s, se esperaba %s", got, mencionado)
	}
}

func TestPersonaObjetivoSinNadie(t *testing.T) {
	if _, ok := personaObjetivo(mencionarCon()); ok {
		t.Fatal("sin menciones ni cita no debería salir nadie")
	}
}
