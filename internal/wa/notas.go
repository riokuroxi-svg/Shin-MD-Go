// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// notas.go — Los dos envíos «de cámara/grabadora»: nota de voz (ptt) y vídeo
// redondo (ptv). En el protocolo son AudioMessage/VideoMessage con el campo
// `ptt` encendido; Baileys hacía exactamente lo mismo en messages.js:377.
package wa

import (
	"context"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// SendAudioVoz manda un MP3 como nota de voz (el círculo con la onda). Los
// caracteres cuentan para la cola como un mensaje corto: el audio no es texto.
func (c *Client) SendAudioVoz(ctx context.Context, chat types.JID, mp3 []byte, priority bool) error {
	if len(mp3) == 0 {
		return errVacio
	}
	up, err := c.WA.Upload(ctx, mp3, whatsmeow.MediaAudio)
	if err != nil {
		return err
	}
	msg := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
		PTT:           proto.Bool(true),
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
		Mimetype:      proto.String("audio/mpeg"),
	}}
	return c.Queue.Do(ctx, 1, priority, func() error {
		_, err := c.WA.SendMessage(ctx, chat, msg)
		return err
	})
}

// SendVideoRedondo manda un MP4 como vídeo instantáneo (el círculo). En el
// protocolo no es un flag del vídeo: es el campo `ptvMessage` del mensaje, que
// lleva dentro un VideoMessage normal — igual que hace Baileys
// (`m.ptvMessage = videoMessage`). WhatsApp recorta a círculo al pintarlo:
// el que graba debe centrar al sujeto.
func (c *Client) SendVideoRedondo(ctx context.Context, chat types.JID, mp4 []byte, segundos int, priority bool) error {
	if len(mp4) == 0 {
		return errVacio
	}
	up, err := c.WA.Upload(ctx, mp4, whatsmeow.MediaVideo)
	if err != nil {
		return err
	}
	video := &waE2E.VideoMessage{
		URL:           proto.String(up.URL),
		DirectPath:    proto.String(up.DirectPath),
		MediaKey:      up.MediaKey,
		FileEncSHA256: up.FileEncSHA256,
		FileSHA256:    up.FileSHA256,
		FileLength:    proto.Uint64(up.FileLength),
		Mimetype:      proto.String("video/mp4"),
	}
	if segundos > 0 {
		video.Seconds = proto.Uint32(uint32(segundos))
	}
	msg := &waE2E.Message{PtvMessage: video}
	return c.Queue.Do(ctx, 1, priority, func() error {
		_, err := c.WA.SendMessage(ctx, chat, msg)
		return err
	})
}
