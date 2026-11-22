// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// send.go — Envíos de las tarjetas nuevas (imagen+botones, carrusel, lista,
// encuesta, pin, evento, respuestas enriquecidas). Todos pasan por la cola
// anti-ban y todos caen a texto plano si WhatsApp no los acepta.
//
// El parámetro `priority` viene del comando (.menu y .ping lo traen activado)
// y NO del tipo de mensaje: antes las tarjetas iban siempre con prioridad
// forzada, así que el cupo diario del calentamiento no se aplicaba nunca a
// los menús. Ahora la prioridad es una propiedad del comando.
package wa

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/interactivos"
	"shin-go/internal/ui"
)

// transporteInteractivo es el puente mínimo entre el paquete aislado y el
// socket de whatsmeow. Solo este archivo conoce ambas cosas.
type transporteInteractivo struct{ client *Client }

func (t transporteInteractivo) Send(ctx context.Context, chat types.JID, msg *waE2E.Message, nodes []binary.Node) error {
	var extra []whatsmeow.SendRequestExtra
	if len(nodes) > 0 {
		n := nodes
		extra = append(extra, whatsmeow.SendRequestExtra{AdditionalNodes: &n})
	}
	_, err := t.client.waClient.SendMessage(ctx, chat, msg, extra...)
	return err
}

func (t transporteInteractivo) SendWithID(ctx context.Context, chat types.JID, msg *waE2E.Message, nodes []binary.Node, messageID string) error {
	extra := whatsmeow.SendRequestExtra{ID: types.MessageID(messageID)}
	if len(nodes) > 0 {
		n := nodes
		extra.AdditionalNodes = &n
	}
	_, err := t.client.waClient.SendMessage(ctx, chat, msg, extra)
	return err
}

func (c *Client) sendTextoDirecto(ctx context.Context, chat types.JID, text string) error {
	_, err := c.waClient.SendMessage(ctx, chat, &waE2E.Message{Conversation: proto.String(text)})
	return err
}

// fallbackPorErrorDeNodos no intenta un mensaje rico sin sus metadatos: el
// servidor puede darle ACK y el receptor no dibujarlo, que es justo el fallo
// que esta versión corrige.
func (c *Client) fallbackPorErrorDeNodos(ctx context.Context, chat types.JID, fallback string, nodeErr error) error {
	if fallback == "" {
		return nodeErr
	}
	if c.Log != nil {
		c.Log.Warn("no se pudieron construir los nodos del envío rico (" + nodeErr.Error() + "); cayendo a texto plano")
	}
	if err := c.sendTextoDirecto(ctx, chat, fallback); err != nil {
		return errors.Join(nodeErr, err)
	}
	return nil
}

// trySend intenta el envío rico con sus nodos; si falla, manda el texto.
func (c *Client) trySend(ctx context.Context, chat types.JID, msg *waE2E.Message, nodes []binary.Node, fallback string) error {
	var extra []whatsmeow.SendRequestExtra
	if len(nodes) > 0 {
		n := nodes
		extra = []whatsmeow.SendRequestExtra{{AdditionalNodes: &n}}
	}

	if _, err := c.waClient.SendMessage(ctx, chat, msg, extra...); err == nil {
		return nil
	} else if fallback == "" {
		return err
	} else {
		c.Log.Warn("envío rico falló (" + err.Error() + "), cayendo a texto plano")
	}

	return c.sendTextoDirecto(ctx, chat, fallback)
}

// ─── Interactivos (un solo puerto, tres estrategias) ─────────────────────────

// SendMenu expresa la intención del comando .menu. Por defecto intenta el
// protobuf envuelto estilo Baileys, luego el nativo directo y por último texto.
func (c *Client) SendMenu(ctx context.Context, chat JID, title, body, footer string, priority bool, buttons ...interactivos.Button) error {
	parts := bytesOf(title, body, footer)
	parts = append(parts, stableJSON(buttons))
	return c.queueDedup(ctx, chat, "menu", len([]rune(body)), priority, parts, func() error {
		return c.interactivos.SendMenu(ctx, chat.whatsmeow(), title, body, footer, buttons...)
	})
}

func (c *Client) SendButtons(ctx context.Context, chat JID, title, body, footer string, priority bool, buttons ...interactivos.Button) error {
	parts := bytesOf(title, body, footer)
	parts = append(parts, stableJSON(buttons))
	return c.queueDedup(ctx, chat, "buttons", len([]rune(body)), priority, parts, func() error {
		return c.interactivos.SendButtons(ctx, chat.whatsmeow(), title, body, footer, nil, buttons...)
	})
}

func captionImagen(title, body, footer string) string {
	partes := make([]string, 0, 3)
	for _, parte := range []string{title, body, footer} {
		if parte = strings.TrimSpace(parte); parte != "" {
			partes = append(partes, parte)
		}
	}
	return strings.Join(partes, "\n\n")
}

// SendButtonsImage conserva también el uso histórico de ReplyImage como envío
// de una imagen corriente: sin botones no construye un interactivo ni pierde el
// archivo durante el fallback. Con botones sube la cabecera una vez y las tres
// estrategias reutilizan el mismo ImageMessage.
func (c *Client) SendButtonsImage(ctx context.Context, chat JID, image []byte, title, body, footer string, priority bool, buttons ...interactivos.Button) error {
	if len(buttons) == 0 {
		return c.SendImage(ctx, chat, image, captionImagen(title, body, footer), priority)
	}
	parts := [][]byte{image, []byte(title), []byte(body), []byte(footer), stableJSON(buttons)}
	return c.queueDedup(ctx, chat, "buttons-image", len([]rune(body)), priority, parts, func() error {
		imgMsg, err := c.uploadImage(ctx, image)
		if err != nil && c.Log != nil {
			c.Log.Warn("no se pudo subir la cabecera del interactivo (" + err.Error() + "); se envía sin imagen")
		}
		return c.interactivos.SendButtons(ctx, chat.whatsmeow(), title, body, footer, imgMsg, buttons...)
	})
}

// GenerateMessageID obtiene un ID opaco con el generador oficial de
// whatsmeow. No se fabrican ni alteran metadatos del protocolo.
func (c *Client) GenerateMessageID() string {
	if c == nil || c.waClient == nil {
		return ""
	}
	return string(c.waClient.GenerateMessageID())
}

// SendButtonsImageWithID usa el ID pre-generado en todas las estrategias y en
// su fallback. Así, un botón puede apuntar al propio mensaje de forma segura.
func (c *Client) SendButtonsImageWithID(ctx context.Context, chat JID, messageID string, image []byte, title, body, footer string, priority bool, buttons ...interactivos.Button) error {
	if strings.TrimSpace(messageID) == "" {
		return errors.New("ID de mensaje vacío")
	}
	if len(buttons) == 0 {
		return errors.New("el envío con ID requiere al menos un botón")
	}
	return c.Queue.Do(ctx, len([]rune(body)), priority, func() error {
		imgMsg, err := c.uploadImage(ctx, image)
		if err != nil && c.Log != nil {
			c.Log.Warn("no se pudo subir la cabecera del interactivo (" + err.Error() + "); se envía sin imagen")
		}
		return c.interactivos.SendButtonsWithID(ctx, chat.whatsmeow(), messageID, title, body, footer, imgMsg, buttons...)
	})
}

func (c *Client) SendCarousel(ctx context.Context, chat JID, body string, priority bool, cards ...interactivos.Card) error {
	parts := bytesOf(body)
	parts = append(parts, stableJSON(cards))
	return c.queueDedup(ctx, chat, "carousel", len([]rune(body)), priority, parts, func() error {
		return c.interactivos.SendCarousel(ctx, chat.whatsmeow(), body, "", cards...)
	})
}

func (c *Client) SendList(ctx context.Context, chat JID, title, body, buttonText, footer string, priority bool, sections ...interactivos.ListSection) error {
	parts := bytesOf(title, body, buttonText, footer)
	parts = append(parts, stableJSON(sections))
	return c.queueDedup(ctx, chat, "list", len([]rune(body)), priority, parts, func() error {
		return c.interactivos.SendList(ctx, chat.whatsmeow(), title, body, buttonText, footer, sections...)
	})
}

// ─── Encuestas y extras del proto ────────────────────────────────────────────

// SendPoll usa el helper OFICIAL de whatsmeow (BuildPollCreation). En Baileys
// esto se arma a mano: el PollCreationMessage lo pone el usuario.
func (c *Client) SendPoll(ctx context.Context, chat JID, question string, options []string, selectable int, priority bool) error {
	if selectable < 1 {
		selectable = 1
	}
	msg := c.waClient.BuildPollCreation(question, options, selectable)
	return c.Queue.Do(ctx, len([]rune(question)), priority, func() error {
		_, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		return err
	})
}

// SendPollSpec traduce la solicitud interna de encuesta/quiz a Protobuf dentro
// del adaptador. Los comandos solo manejan texto, opciones e índice correcto.
func (c *Client) SendPollSpec(ctx context.Context, chat JID, spec PollSpec, priority bool) error {
	msg, err := buildPollMessage(spec)
	if err != nil {
		return err
	}
	return c.Queue.Do(ctx, len([]rune(spec.Question)), priority, func() error {
		_, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		return err
	})
}

func buildPollMessage(spec PollSpec) (*waE2E.Message, error) {
	if len(spec.Options) < 2 || spec.Selectable < 1 || spec.Selectable > len(spec.Options) {
		return nil, errors.New("opciones o cantidad seleccionable inválidas")
	}
	options := make([]*waE2E.PollCreationMessage_Option, len(spec.Options))
	for i, option := range spec.Options {
		options[i] = &waE2E.PollCreationMessage_Option{OptionName: proto.String(option)}
	}
	creation := &waE2E.PollCreationMessage{
		Name:                   proto.String(spec.Question),
		Options:                options,
		SelectableOptionsCount: proto.Uint32(uint32(spec.Selectable)),
	}
	if spec.Quiz {
		if spec.Correct < 0 || spec.Correct >= len(options) {
			return nil, errors.New("índice de respuesta correcta inválido")
		}
		creation.PollType = waE2E.PollType_QUIZ.Enum()
		creation.CorrectAnswer = options[spec.Correct]
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	return &waE2E.Message{
		PollCreationMessage: creation,
		MessageContextInfo:  &waE2E.MessageContextInfo{MessageSecret: secret},
	}, nil
}

// SendPollAddOption: añade una opción a una encuesta EN VIVO (sin helper
// oficial en ninguna lib: el proto está, aquí lo armamos).
func (c *Client) SendPollAddOption(ctx context.Context, chat JID, pollKey *waCommon.MessageKey, option string, priority bool) error {
	msg := ui.BuildPollAddOption(pollKey, option)
	return c.Queue.Do(ctx, len([]rune(option)), priority, func() error {
		_, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		return err
	})
}

// PinMessage: fija/desfija un mensaje en el chat. WhatsApp no lo trata como
// mensaje de chat (no consume cupo del calentamiento), pero se manda por la
// cola igual para no romper el espaciado entre envíos.
func (c *Client) PinMessage(ctx context.Context, chat JID, key MessageKey, pin bool) error {
	msg := ui.BuildPin(ui.Key(key.Chat.whatsmeow(), key.FromMe, types.MessageID(key.ID), key.Sender.whatsmeow()), pin)
	return c.Queue.Do(ctx, 0, true, func() error {
		_, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		return err
	})
}

// SendEvent: evento de calendario (lo avisa WhatsApp, no el bot).
func (c *Client) SendEvent(ctx context.Context, chat JID, name, desc string, start, end time.Time, extraGuests bool, reminder time.Duration, priority bool) error {
	msg := ui.BuildEvent(name, desc, start, end, extraGuests, reminder)
	return c.Queue.Do(ctx, len([]rune(name)), priority, func() error {
		_, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		return err
	})
}

// SendRich: respuesta enriquecida estilo IA (markdown/código/tabla).
// Gateada por RICH_AI=1 y SIEMPRE con fallback: falta verificar en teléfono
// que la app la pinte desde una cuenta normal (por eso no va por defecto).
func (c *Client) SendRich(ctx context.Context, chat JID, msg *waE2E.Message, fallback string, priority bool) error {
	if !c.cfg.RichAI {
		return c.SendText(ctx, chat, fallback, priority)
	}
	return c.Queue.Do(ctx, len([]rune(fallback)), priority, func() error {
		nodes, err := interactivos.BizNodes(chat.whatsmeow(), true)
		if err != nil {
			return c.fallbackPorErrorDeNodos(ctx, chat.whatsmeow(), fallback, err)
		}
		return c.trySend(ctx, chat.whatsmeow(), msg, nodes, fallback)
	})
}

// SendRichCard conserva el fallback del flujo experimental y construye el
// formato externo en el adaptador, no en cmds/.
func (c *Client) SendRichCard(ctx context.Context, chat JID, card RichCard, fallback string, priority bool) error {
	if !c.cfg.RichAI {
		return c.SendText(ctx, chat, fallback, priority)
	}
	msg := ui.RichTextCard(card.Text, card.Language, card.Code)
	return c.Queue.Do(ctx, len([]rune(fallback)), priority, func() error {
		nodes, err := interactivos.BizNodes(chat.whatsmeow(), true)
		if err != nil {
			return c.fallbackPorErrorDeNodos(ctx, chat.whatsmeow(), fallback, err)
		}
		return c.trySend(ctx, chat.whatsmeow(), msg, nodes, fallback)
	})
}

// DeleteMessage borra un mensaje para todos (revoke). El bot necesita ser
// admin del grupo Y el mensaje tiene que ser de otra persona: WhatsApp no deja
// revocar los mensajes propios del bot por esta vía… pero sí los ajenos.
//
// Pasa por la cola anti-ban: un borrado es un envío más, y antes salía por
// fuera de la cola (sin espaciado) justo en los casos en los que el bot borra
// en ráfaga, que es donde más se nota un patrón de abuso.
func (c *Client) DeleteMessage(ctx context.Context, chat, sender JID, id types.MessageID) error {
	if id == "" {
		return nil
	}
	msg := c.waClient.BuildRevoke(chat.whatsmeow(), sender.whatsmeow(), types.MessageID(id))
	return c.Queue.Do(ctx, 0, false, func() error {
		_, err := c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		return err
	})
}

// uploadImage sube una imagen y devuelve el ImageMessage listo para encabezar
// una tarjeta interactiva.
func (c *Client) uploadImage(ctx context.Context, data []byte) (*waE2E.ImageMessage, error) {
	mime, err := imageMIME(data)
	if err != nil {
		return nil, err
	}
	up, err := c.waClient.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return nil, err
	}
	return &waE2E.ImageMessage{
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
		Mimetype:      proto.String(mime),
	}, nil
}
