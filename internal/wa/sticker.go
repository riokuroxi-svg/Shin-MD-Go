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
	"go.mau.fi/whatsmeow/types"
)

// SendSticker sube y manda un sticker WebP.
//
// `animado` es obligatorio y no se adivina: WhatsApp usa ese campo para
// decidir si lo reproduce o lo muestra quieto.
func (c *Client) SendSticker(ctx context.Context, chat types.JID, webp []byte, animado bool, priority bool) error {
	if len(webp) == 0 {
		return errVacio
	}
	up, err := c.WA.Upload(ctx, webp, whatsmeow.MediaImage)
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
	return c.Queue.Do(ctx, 1, priority, func() error {
		_, err := c.WA.SendMessage(ctx, chat, msg)
		return err
	})
}

// SendVideo manda un MP4 (lo que devuelve `.toimg` con un sticker animado).
func (c *Client) SendVideo(ctx context.Context, chat types.JID, mp4 []byte, caption string, priority bool) error {
	if len(mp4) == 0 {
		return errVacio
	}
	up, err := c.WA.Upload(ctx, mp4, whatsmeow.MediaVideo)
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
	return c.Queue.Do(ctx, len([]rune(caption)), priority, func() error {
		_, err := c.WA.SendMessage(ctx, chat, msg)
		return err
	})
}
