// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package wa

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestCaptionImagenConservaElEnvioNoInteractivo(t *testing.T) {
	got := captionImagen("Título", "Cuerpo", "Pie")
	if got != "Título\n\nCuerpo\n\nPie" {
		t.Fatalf("caption = %q", got)
	}
	if got := captionImagen("", "  foto  ", ""); got != "foto" {
		t.Fatalf("caption simple = %q", got)
	}
}

func TestBuildPollMessageQuizContract(t *testing.T) {
	msg, err := buildPollMessage(PollSpec{
		Question: "Capital de México", Options: []string{"Guadalajara", "CDMX", "Monterrey"},
		Selectable: 1, Quiz: true, Correct: 1,
	})
	if err != nil {
		t.Fatalf("buildPollMessage: %v", err)
	}
	poll := msg.GetPollCreationMessage()
	if poll == nil || poll.GetName() != "Capital de México" {
		t.Fatalf("encuesta incorrecta: %v", poll)
	}
	if poll.GetPollType() != waE2E.PollType_QUIZ || poll.GetCorrectAnswer() == nil || poll.GetCorrectAnswer().GetOptionName() != "CDMX" {
		t.Fatalf("quiz sin respuesta correcta: %v", poll)
	}
	if len(poll.GetOptions()) != 3 || poll.GetOptions()[1] != poll.GetCorrectAnswer() {
		t.Fatal("la respuesta correcta debe ser una de las opciones enviadas")
	}
	if len(msg.GetMessageContextInfo().GetMessageSecret()) != 32 {
		t.Fatal("la encuesta debe incluir un secreto de 32 bytes para contabilizar votos")
	}
}

func TestBuildPollMessageRejectsInvalidSpecs(t *testing.T) {
	cases := []PollSpec{
		{Question: "Q", Options: []string{"solo"}, Selectable: 1},
		{Question: "Q", Options: []string{"A", "B"}, Selectable: 3},
		{Question: "Q", Options: []string{"A", "B"}, Selectable: 1, Quiz: true, Correct: 2},
	}
	for _, spec := range cases {
		if _, err := buildPollMessage(spec); err == nil {
			t.Errorf("se aceptó un contrato inválido: %+v", spec)
		}
	}
}
