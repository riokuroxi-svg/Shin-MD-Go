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
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/ui"
)

// trySend intenta el envío rico con sus nodos; si falla, manda el texto.
func (c *Client) trySend(ctx context.Context, chat types.JID, msg *waE2E.Message, nodes []binary.Node, fallback string) error {
	var extra []whatsmeow.SendRequestExtra
	if len(nodes) > 0 {
		n := nodes
		extra = []whatsmeow.SendRequestExtra{{AdditionalNodes: &n}}
	}

	if _, err := c.WA.SendMessage(ctx, chat, msg, extra...); err == nil {
		return nil
	} else if fallback == "" {
		return err
	} else {
		c.Log.Warn("envío rico falló (" + err.Error() + "), cayendo a texto plano")
	}

	_, err := c.WA.SendMessage(ctx, chat, &waE2E.Message{Conversation: proto.String(fallback)})
	return err
}

// ─── Tarjetas con botones ────────────────────────────────────────────────────

// SendButtons: tarjeta con botones (el comportamiento de siempre).
func (c *Client) SendButtons(ctx context.Context, chat types.JID, title, body, footer string, priority bool, buttons ...ui.Button) error {
	msg := ui.BuildButtons(title, body, footer, buttons...)
	return c.Queue.Do(ctx, len([]rune(body)), priority, func() error {
		return c.trySend(ctx, chat, msg, ui.BizNodes(chat, true), ui.Plain(body, footer, buttons...))
	})
}

// SendButtonsImage: tarjeta con IMAGEN de cabecera + botones. Es la que más
// luce en un menú: foto arriba, texto y botones. Si la subida falla, degrada
// a la tarjeta de texto (nunca se queda sin responder).
func (c *Client) SendButtonsImage(ctx context.Context, chat types.JID, image []byte, title, body, footer string, priority bool, buttons ...ui.Button) error {
	imgMsg, err := c.uploadImage(ctx, image)
	if err != nil {
		return c.SendButtons(ctx, chat, title, body, footer, priority, buttons...)
	}
	msg := ui.BuildButtonsImage(imgMsg, title, body, footer, buttons...)
	return c.Queue.Do(ctx, len([]rune(body)), priority, func() error {
		return c.trySend(ctx, chat, msg, ui.BizNodes(chat, true), ui.Plain(body, footer, buttons...))
	})
}

// SendCarousel: carrusel de tarjetas deslizables (con el envoltorio carousel).
func (c *Client) SendCarousel(ctx context.Context, chat types.JID, body string, priority bool, cards ...ui.Card) error {
	msg := ui.BuildCarousel(cards...)
	return c.Queue.Do(ctx, len([]rune(body)), priority, func() error {
		return c.trySend(ctx, chat, msg, ui.BizNodesCarousel(chat, true), ui.PlainCarousel(body, cards...))
	})
}

// SendList: desplegable clásico. No lleva nodos <biz>: el propio servidor le
// añade sus atributos (v=2, type=single_select).
func (c *Client) SendList(ctx context.Context, chat types.JID, title, body, buttonText, footer string, priority bool, sections ...ui.ListSection) error {
	msg := ui.BuildList(title, body, buttonText, footer, sections...)
	return c.Queue.Do(ctx, len([]rune(body)), priority, func() error {
		return c.trySend(ctx, chat, msg, nil, ui.PlainList(title, body, buttonText, footer, sections...))
	})
}

// ─── Encuestas y extras del proto ────────────────────────────────────────────

// SendPoll usa el helper OFICIAL de whatsmeow (BuildPollCreation). En Baileys
// esto se arma a mano: el PollCreationMessage lo pone el usuario.
func (c *Client) SendPoll(ctx context.Context, chat types.JID, question string, options []string, selectable int, priority bool) error {
	if selectable < 1 {
		selectable = 1
	}
	msg := c.WA.BuildPollCreation(question, options, selectable)
	return c.Queue.Do(ctx, len([]rune(question)), priority, func() error {
		_, err := c.WA.SendMessage(ctx, chat, msg)
		return err
	})
}

// SendPollAddOption: añade una opción a una encuesta EN VIVO (sin helper
// oficial en ninguna lib: el proto está, aquí lo armamos).
func (c *Client) SendPollAddOption(ctx context.Context, chat types.JID, pollKey *waCommon.MessageKey, option string, priority bool) error {
	msg := ui.BuildPollAddOption(pollKey, option)
	return c.Queue.Do(ctx, len([]rune(option)), priority, func() error {
		_, err := c.WA.SendMessage(ctx, chat, msg)
		return err
	})
}

// PinMessage: fija/desfija un mensaje en el chat. WhatsApp no lo trata como
// mensaje de chat (no consume cupo del calentamiento), pero se manda por la
// cola igual para no romper el espaciado entre envíos.
func (c *Client) PinMessage(ctx context.Context, chat types.JID, key *waCommon.MessageKey, pin bool) error {
	msg := ui.BuildPin(key, pin)
	return c.Queue.Do(ctx, 0, true, func() error {
		_, err := c.WA.SendMessage(ctx, chat, msg)
		return err
	})
}

// SendEvent: evento de calendario (lo avisa WhatsApp, no el bot).
func (c *Client) SendEvent(ctx context.Context, chat types.JID, name, desc string, start, end time.Time, extraGuests bool, reminder time.Duration, priority bool) error {
	msg := ui.BuildEvent(name, desc, start, end, extraGuests, reminder)
	return c.Queue.Do(ctx, len([]rune(name)), priority, func() error {
		_, err := c.WA.SendMessage(ctx, chat, msg)
		return err
	})
}

// SendRich: respuesta enriquecida estilo IA (markdown/código/tabla).
// Gateada por RICH_AI=1 y SIEMPRE con fallback: falta verificar en teléfono
// que la app la pinte desde una cuenta normal (por eso no va por defecto).
func (c *Client) SendRich(ctx context.Context, chat types.JID, msg *waE2E.Message, fallback string, priority bool) error {
	if !c.cfg.RichAI {
		return c.SendText(ctx, chat, fallback, priority)
	}
	return c.Queue.Do(ctx, len([]rune(fallback)), priority, func() error {
		return c.trySend(ctx, chat, msg, ui.BizNodes(chat, true), fallback)
	})
}

// DeleteMessage borra un mensaje para todos (revoke). El bot necesita ser
// admin del grupo Y el mensaje tiene que ser de otra persona: WhatsApp no deja
// revocar los mensajes propios del bot por esta vía… pero sí los ajenos.
//
// Pasa por la cola anti-ban: un borrado es un envío más, y antes salía por
// fuera de la cola (sin espaciado) justo en los casos en los que el bot borra
// en ráfaga, que es donde más se nota un patrón de abuso.
func (c *Client) DeleteMessage(ctx context.Context, chat, sender types.JID, id types.MessageID) error {
	if id == "" {
		return nil
	}
	msg := c.WA.BuildRevoke(chat, sender, id)
	return c.Queue.Do(ctx, 0, false, func() error {
		_, err := c.WA.SendMessage(ctx, chat, msg)
		return err
	})
}

// uploadImage sube una imagen y devuelve el ImageMessage listo para encabezar
// una tarjeta interactiva.
func (c *Client) uploadImage(ctx context.Context, data []byte) (*waE2E.ImageMessage, error) {
	up, err := c.WA.Upload(ctx, data, whatsmeow.MediaImage)
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
		Mimetype:      proto.String("image/jpeg"),
	}, nil
}
