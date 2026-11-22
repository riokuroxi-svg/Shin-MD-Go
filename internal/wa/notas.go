// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// notas.go — Los dos envíos «de cámara/grabadora»: nota de voz (ptt) y vídeo
// redondo (ptv). En el protocolo son AudioMessage/VideoMessage con el campo
// `ptt` encendido; Baileys hacía exactamente lo mismo en messages.js:377.
package wa

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// SendAudioVoz manda un MP3 como nota de voz (el círculo con la onda). Los
// caracteres cuentan para la cola como un mensaje corto: el audio no es texto.
func (c *Client) SendAudioVoz(ctx context.Context, chat JID, mp3 []byte, priority bool) error {
	if len(mp3) == 0 {
		return errVacio
	}
	return c.queueDedup(ctx, chat, "voice-note", 1, priority, [][]byte{mp3}, func() error {
		up, err := c.waClient.Upload(ctx, mp3, whatsmeow.MediaAudio)
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
		_, err = c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		return err
	})
}

func audioMIME(data []byte) (string, error) {
	if len(data) == 0 {
		return "", errVacio
	}
	switch {
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		return "audio/mp4", nil
	case len(data) >= 4 && string(data[:4]) == "OggS":
		return "audio/ogg", nil
	case len(data) >= 4 && string(data[:4]) == "fLaC":
		return "audio/flac", nil
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WAVE":
		return "audio/wav", nil
	case len(data) >= 3 && string(data[:3]) == "ID3":
		return "audio/mpeg", nil
	case len(data) >= 2 && data[0] == 0xff && data[1]&0xf6 == 0xf0:
		return "audio/aac", nil
	case len(data) >= 2 && data[0] == 0xff && data[1]&0xe0 == 0xe0:
		return "audio/mpeg", nil
	}
	mime := strings.SplitN(http.DetectContentType(data), ";", 2)[0]
	if strings.HasPrefix(mime, "audio/") {
		return mime, nil
	}
	return "", fmt.Errorf("formato de audio no admitido: %s", mime)
}

// SendAudioPista manda audio como *canción* (no nota de voz): con duración y
// MIME real (iTunes entrega M4A; Audius/spotDL suelen entregar MP3).
func (c *Client) SendAudioPista(ctx context.Context, chat JID, audio []byte, titulo string, segundos int, origen string, priority bool) error {
	mime, err := audioMIME(audio)
	if err != nil {
		return err
	}
	parts := [][]byte{audio, []byte(titulo), stableJSON(segundos), []byte(origen), []byte(mime)}
	return c.queueDedup(ctx, chat, "audio-track", len(titulo), priority, parts, func() error {
		up, err := c.waClient.Upload(ctx, audio, whatsmeow.MediaAudio)
		if err != nil {
			return err
		}
		msg := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(up.URL),
			DirectPath:    proto.String(up.DirectPath),
			MediaKey:      up.MediaKey,
			FileEncSHA256: up.FileEncSHA256,
			FileSHA256:    up.FileSHA256,
			FileLength:    proto.Uint64(up.FileLength),
			Mimetype:      proto.String(mime),
		}}
		if segundos > 0 {
			msg.AudioMessage.Seconds = proto.Uint32(uint32(segundos))
		}
		// AudioMessage no tiene campo visible para título/origen; se conservan
		// en la huella para no confundir dos presentaciones lógicas distintas.
		_, err = c.waClient.SendMessage(ctx, chat.whatsmeow(), msg)
		return err
	})
}

// SendVideoPlano manda un MP4 normal con caption (el caso de .yt/.tiktok).
// El caption también cuenta para el anti-ban: es texto andando.
func (c *Client) SendVideoPlano(ctx context.Context, chat JID, mp4 []byte, caption string, priority bool) error {
	if len(mp4) == 0 {
		return errVacio
	}
	return c.queueDedup(ctx, chat, "video-plain", len(caption), priority,
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

// SendVideoRedondo manda un MP4 como vídeo instantáneo (el círculo). En el
// protocolo no es un flag del vídeo: es el campo `ptvMessage` del mensaje, que
// lleva dentro un VideoMessage normal — igual que hace Baileys
// (`m.ptvMessage = videoMessage`). WhatsApp recorta a círculo al pintarlo:
// el que graba debe centrar al sujeto.
func (c *Client) SendVideoRedondo(ctx context.Context, chat JID, mp4 []byte, segundos int, priority bool) error {
	if len(mp4) == 0 {
		return errVacio
	}
	return c.queueDedup(ctx, chat, "video-round", 1, priority,
		[][]byte{mp4, stableJSON(segundos)}, func() error {
			up, err := c.waClient.Upload(ctx, mp4, whatsmeow.MediaVideo)
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
			_, err = c.waClient.SendMessage(ctx, chat.whatsmeow(), &waE2E.Message{PtvMessage: video})
			return err
		})
}
