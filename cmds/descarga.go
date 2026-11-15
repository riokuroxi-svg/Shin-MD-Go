// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// descarga.go — bloque B6.1: la capa de chat de los vídeos sociales.
//
//	.yt     <url>   vídeo MP4 (≤ 100 MB, el tope del JS)
//	.ytmp3  <url>   mismo enlace pero solo el audio
//	.tiktok .ig .fb .x   alias con la pega ya puesta
//
// Todo el peso lo lleva internal/descarga, que es deliberadamente fino:
// si yt-dlp está, baja; si no, el comando enseña a ponerlo. Nada de fingir
// descargas imposibles cuando YouTube pide PoToken — el mensaje lo explica.
package cmds

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"shin-go/internal/bot"
	"shin-go/internal/descarga"
)

func init() {
	bot.Register(bot.Command{
		Name: "yt", Aliases: []string{"ytv", "video", "ytdl", "ytdlp"}, Category: bot.CatDownload,
		Desc: "Baja un vídeo social (YouTube, TikTok, Instagram…) vía yt-dlp si está instalado.",
		Run:  func(c *bot.Context) error { return cmdDescarga(c, "video") },
	})
	bot.Register(bot.Command{
		Name: "ytmp3", Aliases: []string{"ytaudio", "musicaurl"}, Category: bot.CatDownload,
		Desc: "Baja solo el audio de un enlace (vía yt-dlp).",
		Run:  func(c *bot.Context) error { return cmdDescarga(c, "audio") },
	})
	for _, v := range []struct{ nombre, pista string }{
		{"tiktok", "TikTok"}, {"tt", "TikTok"}, {"ig", "Instagram"}, {"instagram", "Instagram"}, {"reel", "Instagram"}, {"fb", "Facebook"}, {"facebook", "Facebook"}, {"x", "X/Twitter"}, {"twitter", "X/Twitter"},
	} {
		n, p := v.nombre, v.pista
		bot.Register(bot.Command{
			Name: n, Category: bot.CatDownload,
			Desc: "Baja el vídeo de un enlace de " + p + " (vía yt-dlp).",
			Run:  func(c *bot.Context) error { return cmdDescarga(c, "video") },
		})
	}
}

// cmdDescarga: la corteza chat↔motor. El motor decide si puede; la corteza
// habla con el usuario en el idioma honesto del repo.
func cmdDescarga(c *bot.Context, formato string) error {
	enlace := strings.TrimSpace(c.Text)
	if !strings.Contains(enlace, "://") {
		return c.Reply("Pásame el enlace directo. Ejemplo: *." + c.Command + " https://youtu.be/…*")
	}

	dir, err := os.MkdirTemp("", "shin-dl-*")
	if err != nil {
		return c.Reply("No pude preparar la carpeta temporal: " + err.Error())
	}
	defer func() { _ = os.RemoveAll(dir) }()

	res, err := descarga.Bajar(c.Ctx, enlace, dir, formato)
	if err != nil {
		return c.Reply(motivoDescarga(err))
	}
	datos, err := res.Leer()
	if err != nil {
		return c.Reply("Se bajó pero no pude leerlo: " + err.Error())
	}

	titulo := res.Titulo
	if titulo == "" {
		titulo = filepath.Base(res.Archivo)
	}
	if res.EsAudio {
		return c.Bot.SendAudioPista(c.Ctx, c.Chat, datos, titulo, 0, enlace, c.Priority())
	}
	return c.Bot.SendVideoPlano(c.Ctx, c.Chat, datos, "🎬 "+titulo, c.Priority())
}

// motivoDescarga: cada error del motor tiene su explicación de usuario.
func motivoDescarga(err error) string {
	switch {
	case errors.Is(err, descarga.ErrNoYtdlp):
		return "Para bajar vídeos sociales hace falta *yt-dlp* en el servidor (un solo binario, sin Python):\n\n" +
			"`curl -L -o ~/.local/bin/yt-dlp https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp && chmod +x ~/.local/bin/yt-dlp`\n\n" +
			"Si usaste `shin-go.sh`, re-ejecútalo: lo instala él."
	case errors.Is(err, descarga.ErrBotCheck):
		return "La plataforma pide comprobar que no somos un bot (PoToken/cookies). " +
			"Hoy YouTube bloquea así gran parte de las descargas desde servidores; " +
			"no es culpa del bot ni del enlace. Para música prefiero *.play*, que baja de catálogos con licencia."
	default:
		return "No pude bajarlo: " + err.Error()
	}
}
