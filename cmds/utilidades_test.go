// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// utilidades_test.go — lo de la primera tanda de utilidades que se puede
// probar sin abrir WhatsApp: el mensaje de encuesta que se manda de verdad
// (quiz incluido), los atajos de texto y la resolución de a quién se refiere
// un comando.
package cmds

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"

	"shin-go/internal/bot"
	"shin-go/internal/utilidades"
)

// ─── .encuesta: el protobuf que se envía ────────────────────────────────────

func TestConstruirEncuestaNormal(t *testing.T) {
	e, err := utilidades.ParsearEncuesta("¿Pizza?|Pepperoni|Hawaiana|Mexicana")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	msg := construirEncuesta(e)

	creacion := msg.GetPollCreationMessage()
	if creacion == nil {
		t.Fatal("falta PollCreationMessage: WhatsApp no mostraría nada")
	}
	if creacion.GetName() != "¿Pizza?" {
		t.Fatalf("pregunta = %q", creacion.GetName())
	}
	if n := len(creacion.GetOptions()); n != 3 {
		t.Fatalf("opciones = %d, se esperaban 3", n)
	}
	for i, quiero := range []string{"Pepperoni", "Hawaiana", "Mexicana"} {
		if got := creacion.GetOptions()[i].GetOptionName(); got != quiero {
			t.Fatalf("opción %d = %q, se esperaba %q", i, got, quiero)
		}
	}
	if n := creacion.GetSelectableOptionsCount(); n != 1 {
		t.Fatalf("seleccionables = %d, una encuesta normal es 1", n)
	}
	if creacion.GetPollType() != waE2E.PollType_POLL {
		t.Fatalf("tipo = %v, se esperaba POLL", creacion.GetPollType())
	}
	if creacion.GetCorrectAnswer() != nil {
		t.Fatal("una encuesta normal no lleva respuesta correcta")
	}
}

func TestConstruirEncuestaQuiz(t *testing.T) {
	e, err := utilidades.ParsearEncuesta("Capital de México|Guadalajara|*CDMX*|Monterrey")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	msg := construirEncuesta(e)
	creacion := msg.GetPollCreationMessage()

	if creacion.GetPollType() != waE2E.PollType_QUIZ {
		t.Fatalf("tipo = %v, se esperaba QUIZ", creacion.GetPollType())
	}
	if got := creacion.GetCorrectAnswer().GetOptionName(); got != "CDMX" {
		t.Fatalf("respuesta correcta = %q, los asteriscos no se quitan de la opción", got)
	}
	// La opción correcta tiene que ser la misma que aparece en la lista: si
	// fuera un puntero copiado, WhatsApp la rechaza.
	var misma bool
	for _, o := range creacion.GetOptions() {
		if o == creacion.GetCorrectAnswer() {
			misma = true
		}
	}
	if !misma {
		t.Fatal("CorrectAnswer no apunta a una de las opciones enviadas")
	}
}

func TestConstruirEncuestaMulti(t *testing.T) {
	e, err := utilidades.ParsearEncuesta("multi ¿Qué llevas?|Hielo|Vasos|Botana")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	creacion := construirEncuesta(e).GetPollCreationMessage()
	if n := creacion.GetSelectableOptionsCount(); n != 3 {
		t.Fatalf("seleccionables = %d, con multi son todas", n)
	}
}

// Sin MessageSecret los votos no se pueden descifrar: la encuesta se ve pero
// no cuenta nada. Es el fallo más silencioso de toda la función.
func TestConstruirEncuestaLlevaSecreto(t *testing.T) {
	e, err := utilidades.ParsearEncuesta("¿Sí?|Sí|No")
	if err != nil {
		t.Fatalf("no debería fallar: %v", err)
	}
	for intento := 0; intento < 20; intento++ {
		msg := construirEncuesta(e)
		secreto := msg.GetMessageContextInfo().GetMessageSecret()
		if len(secreto) != 32 {
			t.Fatalf("secreto de %d bytes, se esperaban 32", len(secreto))
		}
	}
}

func TestSecretoAleatorioNoSeRepite(t *testing.T) {
	a, b := secretoAleatorio(), secretoAleatorio()
	if len(a) != 32 || len(b) != 32 {
		t.Fatalf("longitudes %d y %d", len(a), len(b))
	}
	if proto.Equal(&waE2E.MessageContextInfo{MessageSecret: a},
		&waE2E.MessageContextInfo{MessageSecret: b}) {
		t.Fatal("dos encuestas seguidas llevan el mismo secreto")
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

// mencionarCon arma un contexto mínimo cuyo mensaje menciona a esos JID.
func mencionarCon(jids ...types.JID) *bot.Context {
	crudos := make([]string, len(jids))
	for i, j := range jids {
		crudos[i] = j.String()
	}
	evt := &events.Message{Message: &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text:        proto.String(".algo"),
			ContextInfo: &waE2E.ContextInfo{MentionedJID: crudos},
		},
	}}
	return &bot.Context{Event: evt}
}

func TestPersonaObjetivoUsaLaMención(t *testing.T) {
	mencionado := types.NewJID("5215512345678", types.DefaultUserServer)
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

// ─── nombre verificado ──────────────────────────────────────────────────────

func TestNombreVerificado(t *testing.T) {
	if got := nombreVerificado(nil); got != "" {
		t.Fatalf("nil → %q", got)
	}
	// Verificado sin detalles (el caso que rompía el build anterior).
	if got := nombreVerificado(&types.VerifiedName{}); got != "" {
		t.Fatalf("sin detalles → %q", got)
	}
}
