// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// grupos.go — Lo que el bot le pide a WhatsApp para administrar un grupo.
//
// Todo pasa por aquí y no directamente desde los comandos por dos motivos:
//
//  1. Un solo sitio donde se recortan los datos (nombre, descripción, foto)
//     antes de salir a la red. WhatsApp rechaza o recorta por su cuenta y el
//     usuario se queda sin saber por qué.
//  2. Un solo sitio donde se traduce el error de WhatsApp a algo que se pueda
//     leer en el chat: la diferencia entre "no soy admin" (403) y "ese usuario
//     no está en el grupo" (404) es la que hace inútil un comando de admin.
package wa

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// Topes de WhatsApp para los datos del grupo.
const (
	MaxGroupName  = 100     // caracteres del nombre
	MaxGroupTopic = 1024    // descripción
	MaxGroupPhoto = 2 << 20 // 2 MB de foto
)

// ErrNoEsAdmin: WhatsApp rechazó la operación porque el bot no administra.
var ErrNoEsAdmin = errors.New("no soy administrador del grupo")

// expulsarTexto convierte los errores de whatsmeow en algo entendible.
func groupError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "403"), strings.Contains(msg, "forbidden"), strings.Contains(msg, "not authorized"):
		return ErrNoEsAdmin
	case strings.Contains(msg, "404"), strings.Contains(msg, "not found"), strings.Contains(msg, "item-not-found"):
		return errors.New("ese usuario no está en el grupo")
	case strings.Contains(msg, "409"), strings.Contains(msg, "conflict"):
		return errors.New("el grupo cambió mientras se hacía la operación; prueba otra vez")
	case strings.Contains(msg, "429"), strings.Contains(msg, "rate"):
		return errors.New("WhatsApp pidió esperar; intenta en un rato")
	}
	return err
}

// Kick saca a alguien del grupo.
func (c *Client) Kick(ctx context.Context, group types.JID, who types.JID) error {
	if group.IsEmpty() || who.IsEmpty() {
		return errors.New("grupo o usuario vacío")
	}
	_, err := c.WA.UpdateGroupParticipants(ctx, group, []types.JID{who}, whatsmeow.ParticipantChangeRemove)
	return groupError(err)
}

// Promote / Demote cambian el rol de un miembro.
func (c *Client) Promote(ctx context.Context, group, who types.JID) error {
	_, err := c.WA.UpdateGroupParticipants(ctx, group, []types.JID{who}, whatsmeow.ParticipantChangePromote)
	return groupError(err)
}

func (c *Client) Demote(ctx context.Context, group, who types.JID) error {
	_, err := c.WA.UpdateGroupParticipants(ctx, group, []types.JID{who}, whatsmeow.ParticipantChangeDemote)
	return groupError(err)
}

// SetAnnounce cierra (true) o abre (false) el grupo. Es una sola llamada, no
// dos comandos distintos: el JS tenía `.open` y `.close` separados y con la
// misma lógica duplicada.
func (c *Client) SetAnnounce(ctx context.Context, group types.JID, closed bool) error {
	return groupError(c.WA.SetGroupAnnounce(ctx, group, closed))
}

// InviteLink devuelve el enlace del grupo. Con `reset` lo restablece (el viejo
// deja de servir), que es el `.revoke` del bot de JS.
func (c *Client) InviteLink(ctx context.Context, group types.JID, reset bool) (string, error) {
	link, err := c.WA.GetGroupInviteLink(ctx, group, reset)
	return link, groupError(err)
}

// SetGroupName / SetGroupTopic / SetGroupPhoto con los topes ya aplicados.
func (c *Client) SetGroupName(ctx context.Context, group types.JID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("el nombre no puede quedar vacío")
	}
	if utf8.RuneCountInString(name) > MaxGroupName {
		return fmt.Errorf("el nombre no puede pasar de %d caracteres", MaxGroupName)
	}
	return groupError(c.WA.SetGroupName(ctx, group, name))
}

// SetGroupTopic pone la descripción. whatsmeow pide el id de la anterior: se
// lee del grupo y si falla se manda vacío (WhatsApp lo acepta así).
func (c *Client) SetGroupTopic(ctx context.Context, group types.JID, topic string) error {
	topic = strings.TrimSpace(topic)
	if utf8.RuneCountInString(topic) > MaxGroupTopic {
		return fmt.Errorf("la descripción no puede pasar de %d caracteres", MaxGroupTopic)
	}

	prevID := ""
	if info, err := c.WA.GetGroupInfo(ctx, group); err == nil {
		prevID = info.TopicID
	}
	return groupError(c.WA.SetGroupTopic(ctx, group, prevID, "", topic))
}

// SetGroupPhoto cambia la imagen del grupo.
func (c *Client) SetGroupPhoto(ctx context.Context, group types.JID, photo []byte) error {
	if len(photo) == 0 {
		return errors.New("la imagen está vacía")
	}
	if len(photo) > MaxGroupPhoto {
		return fmt.Errorf("la imagen no puede pasar de %d KB", MaxGroupPhoto/1024)
	}
	_, err := c.WA.SetGroupPhoto(ctx, group, photo)
	return groupError(err)
}

// SendMentions manda texto con menciones reales (@número azul en WhatsApp).
//
// Va como mensaje extendido con `mentionedJID`: sin eso, "@52155…" se queda en
// texto plano y el mencionado no recibe notificación, que es justo lo que
// necesita un tagall o un aviso de advertencia.
func (c *Client) SendMentions(ctx context.Context, chat types.JID, text string, mentions []types.JID, priority bool) error {
	if len(mentions) == 0 {
		return c.SendText(ctx, chat, text, priority)
	}
	if len(mentions) > MaxMentions {
		mentions = mentions[:MaxMentions]
	}

	lista := make([]string, 0, len(mentions))
	for _, j := range mentions {
		if !j.IsEmpty() {
			lista = append(lista, j.ToNonAD().String())
		}
	}

	msg := &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{
		Text:        proto.String(text),
		ContextInfo: &waE2E.ContextInfo{MentionedJID: lista},
	}}
	return c.Queue.Do(ctx, len([]rune(text)), priority, func() error {
		_, err := c.WA.SendMessage(ctx, chat, msg)
		return err
	})
}

// MaxMentions: WhatsApp corta alrededor de 50 menciones por mensaje y aplicar
// el tope aquí evita un mensaje que se queda a medias en grupos grandes.
const MaxMentions = 50

// SendWelcome manda el mensaje de bienvenida/despedida mencionando a quien
// entró o salió.
func (c *Client) SendWelcome(ctx context.Context, chat types.JID, text string, who types.JID) error {
	return c.SendMentions(ctx, chat, text, []types.JID{who}, true)
}

// GroupInfo devuelve la info del grupo sin caché (los comandos de admin
// necesitan el estado real justo antes de actuar).
func (c *Client) GroupInfo(ctx context.Context, group types.JID) (*types.GroupInfo, error) {
	info, err := c.WA.GetGroupInfo(ctx, group)
	return info, groupError(err)
}

// DownloadMedia descarga el contenido de un mensaje (foto, vídeo, audio…). Lo
// usa `.setgpbanner` para poner de foto del grupo la imagen citada.
func (c *Client) DownloadMedia(ctx context.Context, d whatsmeow.DownloadableMessage) ([]byte, error) {
	if d == nil {
		return nil, errors.New("no hay nada que descargar en ese mensaje")
	}
	data, err := c.WA.Download(ctx, d)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxGroupPhoto {
		return nil, fmt.Errorf("la imagen no puede pasar de %d KB", MaxGroupPhoto/1024)
	}
	return data, nil
}

// esperaEntreExpulsiones: pausa entre expulsiones en lote. WhatsApp trata una
// ráfaga de kicks como abuso y puede limitar el número; el JS usaba 3 s.
const esperaEntreExpulsiones = 3 * time.Second

// KickLote expulsa a varios, de uno en uno y con pausa. Devuelve cuántos
// salieron y los errores. Se usa solo desde comandos que lo piden de forma
// explícita (nunca automático): es la operación más peligrosa del bot.
func (c *Client) KickLote(ctx context.Context, group types.JID, quienes []types.JID, max int) (int, []string, error) {
	if max <= 0 {
		max = 0 // sin lote
	}
	if len(quienes) > max {
		quienes = quienes[:max]
	}

	hechos := 0
	var errores []string
	for i, who := range quienes {
		if err := c.Kick(ctx, group, who); err != nil {
			errores = append(errores, fmt.Sprintf("%s: %v", who.User, err))
		} else {
			hechos++
		}
		if i < len(quienes)-1 {
			select {
			case <-ctx.Done():
				return hechos, errores, ctx.Err()
			case <-time.After(esperaEntreExpulsiones):
			}
		}
	}
	return hechos, errores, nil
}
