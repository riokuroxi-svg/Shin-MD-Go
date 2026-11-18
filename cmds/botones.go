// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// botones.go — Comandos de demostración de TODO el catálogo de tarjetas.
// Sirven para probar en el teléfono cuál pinta WhatsApp y cuál no, sin tocar
// el resto del bot. Es el banco de pruebas de la migración.
package cmds

import (
	"strings"
	"time"

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

			// Topes de WhatsApp: pregunta de 255, opción de 100 y 12 opciones
			// como máximo. Sin esto, un mensaje largo se manda igual y el
			// teléfono lo tira sin avisar (o peor: el envío queda a medias).
			pregunta := bot.ClipText(partes[0], 255)
			opciones := make([]string, 0, len(partes)-1)
			for _, op := range partes[1:] {
				if op = bot.ClipText(op, 100); op != "" {
					opciones = append(opciones, op)
				}
				if len(opciones) == 12 {
					break
				}
			}
			if len(opciones) < 2 {
				return c.Reply("Necesito al menos dos opciones con texto.")
			}
			return c.Bot.SendPoll(c.Ctx, c.Chat, pregunta, opciones, 1, c.Priority())
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
			if c.Event == nil || c.Event.Quoted == nil || c.Event.Quoted.ID == "" {
				return c.Reply("Responde a un mensaje con .pin para fijarlo.")
			}

			// El adaptador ya resolvió el participante de la cita. En chats
			// privados WhatsApp no requiere el campo participant.
			var participant wa.JID
			if c.IsGroup {
				participant = c.Sender
				if !c.Event.Quoted.Participant.IsEmpty() {
					participant = c.Event.Quoted.Participant
				}
			}
			return c.Bot.PinMessage(c.Ctx, c.Chat, wa.MessageKey{
				Chat: c.Chat, Sender: participant, ID: c.Event.Quoted.ID,
			}, true)
		},
	})

	// .rich → respuesta enriquecida estilo IA (experimental, RICH_AI=1)
	// Nota: el alias «ai» lo tenía este comando como placeholder; en el bloque
	// B5.2 pasa a la IA de verdad (.ai → Gemini), como en el bot de JS.
	bot.Register(bot.Command{
		Name:     "rich",
		Aliases:  []string{"markdown", "richai"},
		Category: bot.CatUtils,
		Desc:     "Prueba de respuesta enriquecida (markdown/tabla) · experimental",
		Run: func(c *bot.Context) error {
			fallback := "**SHIN-MD** — si ves esto en texto plano, tu teléfono no pintó la respuesta enriquecida."
			card := wa.RichCard{
				Text:     "# Shin-MD\nPrueba de *respuesta enriquecida*:\n- markdown\n- bloques de código\n- tablas",
				Language: "go",
				Code:     "func main() {\n\tfmt.Println(\"shin\")\n}",
			}
			return c.Bot.SendRichCard(c.Ctx, c.Chat, card, fallback, c.Priority())
		},
	})
}

// splitSemi parte por ';' con techo: 20 trozos es más que de sobra para una
// encuesta y evita procesar un texto lleno de separadores.
func splitSemi(s string) []string {
	const maxPartes = 20
	var out []string
	cur := ""
	for _, r := range s {
		if r == ';' {
			if len(out) >= maxPartes {
				return out
			}
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
