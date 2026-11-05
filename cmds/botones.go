// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// botones.go — Comandos de demostración de TODO el catálogo de tarjetas.
// Sirven para probar en el teléfono cuál pinta WhatsApp y cuál no, sin tocar
// el resto del bot. Es el banco de pruebas de la migración.
package cmds

import (
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/bot"
	"shin-go/internal/ui"
	"shin-go/internal/wa"
)

func init() {
	// .botones → tarjeta con botones (y con imagen de cabecera si la mandas respondiendo a una)
	bot.Register(bot.Command{
		Name:     "botones",
		Aliases:  []string{"buttons", "tarjeta"},
		Category: bot.CatMain,
		Desc:     "Prueba de tarjeta con botones nativos",
		Run: func(c *bot.Context) error {
			return c.ReplyButtons(
				"SHIN-MD · botones",
				"╭─「 *TARJETA* 」\n│ quick_reply · cta_url\n│ cta_copy · cta_call\n╰─ toca cualquiera",
				"Si no ves botones, llegará el texto de respaldo",
				bot.QuickReply("🏓 Ping", "ping"),
				bot.CTAURL("🌐 Repo", "https://github.com/riokuroxi-svg/Shin-MD"),
				bot.CTACopy("📮 Copiar prefijo", c.Bot.FirstPrefix()),
				bot.CTACall("📞 Llamar", "+5215555555555"),
			)
		},
	})

	// .lista → desplegable clásico (ListMessage)
	bot.Register(bot.Command{
		Name:     "lista",
		Aliases:  []string{"desplegable", "list"},
		Category: bot.CatMain,
		Desc:     "Prueba de lista desplegable clásica",
		Run: func(c *bot.Context) error {
			return c.Bot.SendList(c.Ctx, c.Chat,
				"SHIN-MD", "Elige una opción del desplegable", "Ver opciones", "Lista clásica (ListMessage)", c.Priority(),
				ui.ListSection{Title: "General", Rows: []ui.ListRow{
					{ID: "ping", Title: "🏓 Ping", Desc: "latencia del bot"},
					{ID: "menu", Title: "📋 Menú", Desc: "todos los comandos"},
				}},
				ui.ListSection{Title: "Economía", Rows: []ui.ListRow{
					{ID: "daily", Title: "🎁 Daily", Desc: "recompensa diaria"},
					{ID: "perfil", Title: "👤 Perfil", Desc: "tus monedas"},
				}},
			)
		},
	})

	// .carrusel → tarjetas deslizables
	bot.Register(bot.Command{
		Name:     "carrusel",
		Aliases:  []string{"carousel", "cartas"},
		Category: bot.CatMain,
		Desc:     "Prueba de carrusel (tarjetas deslizables)",
		Run: func(c *bot.Context) error {
			return c.Bot.SendCarousel(c.Ctx, c.Chat, "Desliza las tarjetas →", c.Priority(),
				ui.Card{Title: "Carta 1", Body: "Ping y estado", Buttons: []ui.Button{bot.QuickReply("🏓 Ping", "ping")}},
				ui.Card{Title: "Carta 2", Body: "Tu perfil", Buttons: []ui.Button{bot.QuickReply("👤 Perfil", "perfil")}},
				ui.Card{Title: "Carta 3", Body: "Menú completo", Buttons: []ui.Button{bot.QuickReply("📋 Menú", "menu")}},
			)
		},
	})

	// .encuesta → BuildPollCreation oficial de whatsmeow
	bot.Register(bot.Command{
		Name:     "encuesta",
		Aliases:  []string{"poll", "votacion"},
		Category: bot.CatUtils,
		Desc:     "Crea una encuesta (.encuesta pregunta; op1; op2; …)",
		Run: func(c *bot.Context) error {
			if len(c.Args) == 0 {
				return c.Reply("Uso: .encuesta Pregunta; opción1; opción2")
			}
			todo := strings.Join(c.Args, " ")
			partes := splitSemi(todo)
			if len(partes) < 3 {
				return c.Reply("Necesito pregunta + 2 opciones: .encuesta ¿Café?; sí; no")
			}
			return c.Bot.SendPoll(c.Ctx, c.Chat, partes[0], partes[1:], 1, c.Priority())
		},
	})

	// .evento → evento de calendario con recordatorio
	bot.Register(bot.Command{
		Name:     "evento",
		Aliases:  []string{"event", "calendario"},
		Category: bot.CatUtils,
		Desc:     "Crea un evento de calendario",
		Run: func(c *bot.Context) error {
			start := time.Now().Add(time.Hour)
			return c.Bot.SendEvent(c.Ctx, c.Chat,
				"Evento de Shin-MD", "Creado con EventMessage desde Go",
				start, start.Add(30*time.Minute), true, 10*time.Minute, c.Priority())
		},
	})

	// .pin → fija el mensaje al que respondes
	bot.Register(bot.Command{
		Name:     "pin",
		Aliases:  []string{"fijar"},
		Category: bot.CatUtils,
		Desc:     "Fija el mensaje al que respondes",
		Run: func(c *bot.Context) error {
			// El mensaje citado vive dentro del contenido: si respondes a una
			// foto viene en ImageMessage, si es un audio en AudioMessage… Mirar
			// solo ExtendedTextMessage hacía que .pin dijera "no respondiste a
			// nada" con fotos, vídeos, audios y stickers.
			ctxInfo := wa.ContextInfo(c.Event.Message)
			if ctxInfo == nil || ctxInfo.GetStanzaID() == "" {
				return c.Reply("Responde a un mensaje con .pin para fijarlo.")
			}

			// El autor del mensaje citado puede ser otro: se saca del propio
			// ContextInfo (y solo tiene sentido en grupos).
			var participant types.JID
			if c.IsGroup {
				participant = c.Sender
				if raw := ctxInfo.GetParticipant(); raw != "" {
					if jid, err := types.ParseJID(raw); err == nil {
						participant = jid.ToNonAD()
					}
				}
			}

			key := ui.Key(c.Chat, false, ctxInfo.GetStanzaID(), participant)
			return c.Bot.PinMessage(c.Ctx, c.Chat, key, true)
		},
	})

	// .rich → respuesta enriquecida estilo IA (experimental, RICH_AI=1)
	bot.Register(bot.Command{
		Name:     "rich",
		Aliases:  []string{"ai", "markdown"},
		Category: bot.CatUtils,
		Desc:     "Prueba de respuesta enriquecida (markdown/tabla) · experimental",
		Run: func(c *bot.Context) error {
			fallback := "**SHIN-MD** — si ves esto en texto plano, tu teléfono no pintó la respuesta enriquecida."
			msg := ui.RichTextCard(
				"# Shin-MD\nPrueba de *respuesta enriquecida*:\n- markdown\n- bloques de código\n- tablas",
				"go", "func main() {\n\tfmt.Println(\"shin\")\n}",
			)
			return c.Bot.SendRich(c.Ctx, c.Chat, msg, fallback, c.Priority())
		},
	})
}

func splitSemi(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ';' {
			out = append(out, strings.TrimSpace(cur))
			cur = ""
			continue
		}
		cur += string(r)
	}
	if strings.TrimSpace(cur) != "" {
		out = append(out, strings.TrimSpace(cur))
	}
	return out
}
