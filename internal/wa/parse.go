// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// parse.go — Leer un mensaje entrante: texto normal, respuesta de botón
// nativo, o selección de lista. Puerto de parseButtonResponse() del JS,
// más dos cosas que en el JS se resolvían a mano en cada comando:
//
//	Unwrap()      → quita los envoltorios (efímero, ver una vez, edición…)
//	ContextInfo() → saca el "mensaje citado" sin importar de qué tipo sea
package wa

import (
	"encoding/json"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

// Parsed es el resultado de mirar un mensaje entrante.
type Parsed struct {
	Text      string // texto plano (vacío si es botón)
	ButtonID  string // id del botón tocado (formato "comando:args")
	IsButton  bool
	IsCommand bool // el texto empieza con prefijo
	Command   string
	Args      []string
	Prefix    string
}

// ParseMessage extrae lo relevante. Orden: botones → listas → texto.
func ParseMessage(msg *waE2E.Message, prefixes []string) Parsed {
	var out Parsed
	msg = Unwrap(msg)
	if msg == nil {
		return out
	}

	switch {
	case msg.GetConversation() != "":
		out.Text = msg.GetConversation()
	case msg.GetExtendedTextMessage() != nil:
		out.Text = msg.GetExtendedTextMessage().GetText()
	case msg.GetButtonsResponseMessage() != nil:
		out.ButtonID = msg.GetButtonsResponseMessage().GetSelectedButtonID()
		out.IsButton = true
		return out
	case msg.GetTemplateButtonReplyMessage() != nil:
		out.ButtonID = msg.GetTemplateButtonReplyMessage().GetSelectedID()
		out.IsButton = true
		return out
	case msg.GetListResponseMessage() != nil:
		out.ButtonID = msg.GetListResponseMessage().GetSingleSelectReply().GetSelectedRowID()
		out.IsButton = true
		return out
	case msg.GetInteractiveResponseMessage() != nil:
		// native_flow: el cliente devuelve {"id":"...","display_text":"..."} en paramsJSON
		if nf := msg.GetInteractiveResponseMessage().GetNativeFlowResponseMessage(); nf != nil {
			var p struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal([]byte(nf.GetParamsJSON()), &p)
			out.ButtonID = p.ID
			out.IsButton = true
		}
		return out
	}

	out.Text = strings.TrimSpace(out.Text)
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(out.Text, p) {
			rest := strings.TrimSpace(strings.TrimPrefix(out.Text, p))
			fields := strings.Fields(rest)
			if len(fields) == 0 {
				break
			}
			out.IsCommand = true
			out.Prefix = p
			out.Command = strings.ToLower(fields[0])
			out.Args = fields[1:]
			break
		}
	}
	return out
}

// Unwrap quita los envoltorios con los que WhatsApp manda el mensaje real.
// Sin esto, un comando escrito en un chat con mensajes efímeros, o un botón
// dentro de un "ver una vez", llega como un sobre vacío y el bot no contesta
// nada (sin error, que es lo peor para depurar). Se recorre en cadena porque
// los envoltorios se pueden anidar (efímero + ver una vez, por ejemplo).
func Unwrap(msg *waE2E.Message) *waE2E.Message {
	const maxDepth = 8 // corta un envoltorio mal formado que se refiera a sí mismo
	for i := 0; msg != nil && i < maxDepth; i++ {
		var inner *waE2E.Message
		switch {
		case msg.GetEphemeralMessage() != nil:
			inner = msg.GetEphemeralMessage().GetMessage()
		case msg.GetViewOnceMessage() != nil:
			inner = msg.GetViewOnceMessage().GetMessage()
		case msg.GetViewOnceMessageV2() != nil:
			inner = msg.GetViewOnceMessageV2().GetMessage()
		case msg.GetViewOnceMessageV2Extension() != nil:
			inner = msg.GetViewOnceMessageV2Extension().GetMessage()
		case msg.GetDocumentWithCaptionMessage() != nil:
			inner = msg.GetDocumentWithCaptionMessage().GetMessage()
		case msg.GetEditedMessage() != nil:
			inner = msg.GetEditedMessage().GetMessage()
		case msg.GetDeviceSentMessage() != nil:
			inner = msg.GetDeviceSentMessage().GetMessage()
		default:
			return msg
		}
		if inner == nil {
			return msg
		}
		msg = inner
	}
	return msg
}

// ContextInfo devuelve la información del mensaje citado (el "respondo a
// esto"). Vive dentro del contenido: en un texto va en ExtendedTextMessage,
// pero en una foto va en ImageMessage, en un audio en AudioMessage… Por eso
// hay que mirar todos los tipos antes de decirle al usuario "no estás
// respondiendo a nada" (que es lo que hacía el .pin con las fotos).
func ContextInfo(msg *waE2E.Message) *waE2E.ContextInfo {
	msg = Unwrap(msg)
	if msg == nil {
		return nil
	}
	switch {
	case msg.GetExtendedTextMessage() != nil:
		return msg.GetExtendedTextMessage().GetContextInfo()
	case msg.GetImageMessage() != nil:
		return msg.GetImageMessage().GetContextInfo()
	case msg.GetVideoMessage() != nil:
		return msg.GetVideoMessage().GetContextInfo()
	case msg.GetAudioMessage() != nil:
		return msg.GetAudioMessage().GetContextInfo()
	case msg.GetDocumentMessage() != nil:
		return msg.GetDocumentMessage().GetContextInfo()
	case msg.GetStickerMessage() != nil:
		return msg.GetStickerMessage().GetContextInfo()
	case msg.GetContactMessage() != nil:
		return msg.GetContactMessage().GetContextInfo()
	case msg.GetLocationMessage() != nil:
		return msg.GetLocationMessage().GetContextInfo()
	case msg.GetButtonsResponseMessage() != nil:
		return msg.GetButtonsResponseMessage().GetContextInfo()
	case msg.GetListResponseMessage() != nil:
		return msg.GetListResponseMessage().GetContextInfo()
	case msg.GetInteractiveResponseMessage() != nil:
		return msg.GetInteractiveResponseMessage().GetContextInfo()
	}
	return nil
}

// CommandFromButton traduce un id de botón a comando + args.
// Convención (idéntica al JS): "ping", "menu:categorias", "play:nombre canción".
func CommandFromButton(id string) (cmd string, args []string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", nil
	}
	parts := strings.SplitN(id, ":", 2)
	cmd = strings.ToLower(strings.TrimSpace(parts[0]))
	if len(parts) == 2 {
		if rest := strings.TrimSpace(parts[1]); rest != "" {
			args = strings.Fields(rest)
		}
	}
	return cmd, args
}
