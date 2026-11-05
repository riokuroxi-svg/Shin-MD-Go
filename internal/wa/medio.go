// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// medio.go — Sacar "de qué es este mensaje" sin adivinar.
//
// Un mismo contenido llega envuelto de cinco formas distintas (ver una vez,
// temporal, con pie de foto, reenviado, editado) y el usuario además puede
// **citar** una foto para pedir su sticker. En Baileys esto se resolvía con
// `m.type` y `m.quoted`; aquí se recorre el envoltorio y, si no hay medio, se
// mira lo citado. Si tampoco, se dice que no hay nada: nunca se manda un
// sticker vacío.
package wa

import (
	"errors"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

// errVacio: se intentó mandar un medio de 0 bytes.
var errVacio = errors.New("wa: el archivo está vacío")

// TipoMedio dice qué se ha encontrado.
type TipoMedio string

const (
	MedioImagen    TipoMedio = "imagen"
	MedioVideo     TipoMedio = "video"
	MedioSticker   TipoMedio = "sticker"
	MedioAudio     TipoMedio = "audio"
	MedioDocumento TipoMedio = "documento"
)

// Medio es un archivo descargable de un mensaje.
type Medio struct {
	Data     whatsmeow.DownloadableMessage
	Tipo     TipoMedio
	Mimetype string
	Animado  bool
	Citado   bool // venía en el mensaje citado, no en el que se escribió
}

// MedioDe busca un archivo en el mensaje o, si no lo hay, en el citado.
func MedioDe(msg *waE2E.Message) (Medio, bool) {
	if msg == nil {
		return Medio{}, false
	}
	if m, ok := medioDirecto(Unwrap(msg)); ok {
		return m, true
	}
	// Citar una foto o un sticker es la forma normal de pedir "hazme esto".
	if ci := ContextInfo(Unwrap(msg)); ci != nil && ci.GetQuotedMessage() != nil {
		if m, ok := medioDirecto(Unwrap(ci.GetQuotedMessage())); ok {
			m.Citado = true
			return m, true
		}
	}
	return Medio{}, false
}

// medioDirecto mira el mensaje ya desenvuelto.
func medioDirecto(msg *waE2E.Message) (Medio, bool) {
	if msg == nil {
		return Medio{}, false
	}
	switch {
	case msg.GetStickerMessage() != nil:
		s := msg.GetStickerMessage()
		return Medio{Data: s, Tipo: MedioSticker, Mimetype: s.GetMimetype(), Animado: s.GetIsAnimated()}, true
	case msg.GetImageMessage() != nil:
		i := msg.GetImageMessage()
		return Medio{Data: i, Tipo: MedioImagen, Mimetype: i.GetMimetype()}, true
	case msg.GetVideoMessage() != nil:
		v := msg.GetVideoMessage()
		// Un GIF de WhatsApp llega como vídeo con gif_playback: se trata como
		// animado para que salga sticker en movimiento.
		return Medio{Data: v, Tipo: MedioVideo, Mimetype: v.GetMimetype(), Animado: v.GetGifPlayback()}, true
	case msg.GetAudioMessage() != nil:
		a := msg.GetAudioMessage()
		return Medio{Data: a, Tipo: MedioAudio, Mimetype: a.GetMimetype()}, true
	case msg.GetDocumentMessage() != nil:
		d := msg.GetDocumentMessage()
		return Medio{Data: d, Tipo: MedioDocumento, Mimetype: d.GetMimetype()}, true
	}
	return Medio{}, false
}
