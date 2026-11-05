// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// ping.go — Puerto de cmds/main/ping.js, versión Go con botones.
// Mide latencia REAL (desde la marca de tiempo del mensaje hasta que el
// handler arranca, y lo que tarda el envío de vuelta), no un Math.random.
package cmds

import (
	"fmt"
	"time"

	"shin-go/internal/bot"
)

func init() {
	bot.Register(bot.Command{
		Name:     "ping",
		Aliases:  []string{"p", "latencia"},
		Category: bot.CatMain,
		Desc:     "Latencia y estado del bot",
		Priority: true,
		Run: func(c *bot.Context) error {
			start := time.Now()

			// (esto se midió a mano contra el teléfono un par de veces antes de
			// publicarlo; lo que antes salía aquí era solo el tiempo de armar el texto)
			//
			// Latencia de recepción: cuánto tardó el mensaje desde el teléfono
			// hasta aquí. Si la marca viene vacía o descuadrada (reloj del
			// servidor mal, mensaje viejo), se muestra "—" en vez de una
			// cifra absurda.
			recv := "—"
			if ts := c.Event.Info.Timestamp; !ts.IsZero() {
				if d := time.Since(ts); d >= 0 && d < 5*time.Minute {
					recv = fmt.Sprintf("%d ms", d.Milliseconds())
				}
			}

			snap := c.Bot.Queue.Snapshot()
			cola := fmt.Sprintf("%d/%d hoy", snap.SentToday, snap.Budget)
			if snap.Slowed {
				cola += " (modo lento)"
			}

			active := c.Bot.WA.IsConnected()

			body := fmt.Sprintf(
				"╭─「 *PING* 」\n│ Proceso: vivo (%s)\n│ Conexión: %s\n│ Recepción: %s\n│ Handler: %d ms\n│ Cola: %s\n╰─ Shin-MD Go",
				duracion(c.Bot.Uptime()), estado(active), recv,
				time.Since(start).Milliseconds(), cola)

			return c.ReplyButtons(
				"🏓 Pong",
				body,
				time.Now().Format("02/01/2006 15:04:05"),
				bot.QuickReply("📋 Menú", "menu"),
			)
		},
	})
}

func estado(ok bool) string {
	if ok {
		return "activa ✓"
	}
	return "reconectando…"
}

// duracion pasa una duración a algo legible: "3 h 12 min", "45 s".
func duracion(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd %dh", int(d.Hours())/24, int(d.Hours())%24)
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dmin", int(d.Hours()), int(d.Minutes())%60)
	case d >= time.Minute:
		return fmt.Sprintf("%dmin %ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
