// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package wa

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func TestImageMIMEUsaLosMetadatosReales(t *testing.T) {
	cases := []struct {
		data []byte
		want string
	}{
		{append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...), "image/png"},
		{append([]byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00"), make([]byte, 24)...), "image/jpeg"},
		{append([]byte("GIF89a"), make([]byte, 32)...), "image/gif"},
		{append([]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), make([]byte, 24)...), "image/webp"},
	}
	for _, tc := range cases {
		if got, err := imageMIME(tc.data); err != nil || got != tc.want {
			t.Errorf("imageMIME(%q…) = %q, %v; want %q", tc.data[:4], got, err, tc.want)
		}
	}
	if _, err := imageMIME([]byte("no es una imagen")); err == nil {
		t.Fatal("datos no gráficos debían rechazarse en vez de fingir image/jpeg")
	}
}

func TestAudioMIMEReconoceFormatosAdmitidos(t *testing.T) {
	cases := []struct {
		data []byte
		want string
	}{
		{append([]byte("ID3"), make([]byte, 16)...), "audio/mpeg"},
		{append([]byte{0xff, 0xfb}, make([]byte, 16)...), "audio/mpeg"},
		{append([]byte{0, 0, 0, 24}, append([]byte("ftypM4A "), make([]byte, 16)...)...), "audio/mp4"},
		{append([]byte{0xff, 0xf1}, make([]byte, 16)...), "audio/aac"},
		{append([]byte("OggS"), make([]byte, 16)...), "audio/ogg"},
		{append([]byte("fLaC"), make([]byte, 16)...), "audio/flac"},
		{append([]byte("RIFF\x10\x00\x00\x00WAVE"), make([]byte, 16)...), "audio/wav"},
	}
	for _, tc := range cases {
		if got, err := audioMIME(tc.data); err != nil || got != tc.want {
			t.Errorf("audioMIME = %q, %v; want %q", got, err, tc.want)
		}
	}
	if _, err := audioMIME([]byte("texto")); err == nil {
		t.Fatal("texto no debe declararse audio/mpeg")
	}
}

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
