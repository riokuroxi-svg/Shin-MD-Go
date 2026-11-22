// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// sticker.go — Mandar un sticker y sacar el medio de un mensaje.
//
// En Baileys era `sock.sendMessage(jid, {sticker: buffer})` y ya. En whatsmeow
// hay que subir el WebP como media, montar el `StickerMessage` y decir si es
// animado (si se miente en ese campo, WhatsApp lo muestra como imagen rota).
package wa

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
)

// SendSticker sube y manda un sticker WebP.
//
// `animado` es obligatorio y no se adivina: WhatsApp usa ese campo para
// decidir si lo reproduce o lo muestra quieto.
func (c *Client) SendSticker(ctx context.Context, chat JID, webp []byte, animado bool, priority bool) error {
	if len(webp) == 0 {
		return errVacio
	}
	return c.queueDedup(ctx, chat, "sticker", 1, priority,
		[][]byte{webp, []byte{boolByte(animado)}}, func() error {
			up, err := c.waClient.Upload(ctx, webp, whatsmeow.MediaImage)
			if err != nil {
				return err
			}
			msg := &waE2E.Message{StickerMessage: &waE2E.StickerMessage{
				URL:           proto.String(up.URL),
				DirectPath:    proto.String(up.DirectPath),
				MediaKey:      up.MediaKey,
				FileEncSHA256: up.FileEncSHA256,
				FileSHA256:    up.FileSHA256,
				FileLength:    proto.Uint64(up.FileLength),
				Mimetype:      proto.String("image/webp"),
				IsAnimated:    proto.Bool(animado),
			}}
			_, err = c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
			return err
		})
}

// SendGIF manda un MP4 corto con el indicador gifPlayback. WhatsApp no acepta
// los bytes image/gif dentro de VideoMessage: la conversión a MP4 ocurre antes
// en internal/media y aquí se declara el metadato correcto.
func (c *Client) SendGIF(ctx context.Context, chat JID, mp4 []byte, caption string, priority bool) error {
	if len(mp4) == 0 {
		return errVacio
	}
	return c.queueDedup(ctx, chat, "gif", len([]rune(caption)), priority,
		[][]byte{mp4, []byte(caption)}, func() error {
			up, err := c.waClient.Upload(ctx, mp4, whatsmeow.MediaVideo)
			if err != nil {
				return err
			}
			msg := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
				Caption:       proto.String(caption),
				URL:           proto.String(up.URL),
				DirectPath:    proto.String(up.DirectPath),
				MediaKey:      up.MediaKey,
				FileEncSHA256: up.FileEncSHA256,
				FileSHA256:    up.FileSHA256,
				FileLength:    proto.Uint64(up.FileLength),
				Mimetype:      proto.String("video/mp4"),
				GifPlayback:   proto.Bool(true),
			}}
			_, err = c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
			return err
		})
}

// SendVideo manda un MP4 (lo que devuelve `.toimg` con un sticker animado).
func (c *Client) SendVideo(ctx context.Context, chat JID, mp4 []byte, caption string, priority bool) error {
	if len(mp4) == 0 {
		return errVacio
	}
	return c.queueDedup(ctx, chat, "video", len([]rune(caption)), priority,
		[][]byte{mp4, []byte(caption)}, func() error {
			up, err := c.waClient.Upload(ctx, mp4, whatsmeow.MediaVideo)
			if err != nil {
				return err
			}
			msg := &waE2E.Message{VideoMessage: &waE2E.VideoMessage{
				Caption:       proto.String(caption),
				URL:           proto.String(up.URL),
				DirectPath:    proto.String(up.DirectPath),
				MediaKey:      up.MediaKey,
				FileEncSHA256: up.FileEncSHA256,
				FileSHA256:    up.FileSHA256,
				FileLength:    proto.Uint64(up.FileLength),
				Mimetype:      proto.String("video/mp4"),
			}}
			_, err = c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
			return err
		})
}
