// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// proteccion.go — Antilink y afk: los dos primeros hooks del bloque B1.
//
// Un hook no necesita que el mensaje sea un comando: ve todo lo que pasa en el
// chat. Es la diferencia entre "el bot responde a .menu" y "el bot está vivo
// en el grupo".
package cmds

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/bot"
	"shin-go/internal/wa"
)

// Guardas del antilink: sin esto, alguien escribiendo enlaces en bucle hace
// que el bot responda a cada uno (spam saliente = ban). 3 avisos por minuto y
// por usuario como máximo.
var (
	antilinkGuard = bot.NewRateGuard(3, time.Minute)
	afkGuard      = bot.NewRateGuard(1, 10*time.Second)
)

const (
	keyAntilink = "antilink"
	keyAFK      = "afk"       // motivo
	keyAFKSin   = "afk_desde" // fecha en que se activó
	defaultAFK  = "sin motivo"
)

func init() {
	// ── hook: antilink ───────────────────────────────────────────────────
	bot.RegisterHook("antilink", func(c *bot.Context, parsed wa.Parsed) bool {
		if !c.IsGroup || c.IsOwner {
			return false // el owner puede pasar enlaces siempre
		}
		text := parsed.Text
		if text == "" {
			return false
		}
		if !bot.HasLink(text) {
			return false
		}

		on, err := antilinkOn(c)
		if err != nil || !on {
			return false
		}
		if c.Admin() {
			return false // los admins del grupo sí pueden
		}

		// Primero el límite: si este usuario ya va por su tercer aviso del
		// minuto, se borra igual pero no se responde (no darle conversación).
		allow, warn := antilinkGuard.Allow(c.Key())

		borrado := true
		if err := c.Bot.DeleteMessage(c.Ctx, c.Chat, c.Sender, c.Event.Info.ID); err != nil {
			borrado = false
			c.Log.Warn("antilink: no se pudo borrar el mensaje: " + err.Error())
		}

		if !allow {
			if warn {
				_ = c.Reply("🔗 *Antilink* — demasiados enlaces seguidos. Se seguirán borrando, sin avisar.")
			}
			return true
		}

		// El nombre viene del perfil: es texto libre, se limpia y se escapa.
		aviso := "🔗 *Antilink* — " + nombre(c) + ", los enlaces no están permitidos en este grupo."
		if borrado {
			aviso += "\n_Mensaje borrado._"
		} else {
			aviso += "\n_No pude borrar el mensaje (necesito ser admin)._"
		}
		_ = c.Reply(aviso)
		return true
	})

	// ── hook: afk ────────────────────────────────────────────────────────
	// Dos caras: si el que escribe está ausente, se le quita el aviso; si
	// alguien menciona a un ausente, se le dice dónde está.
	bot.RegisterHook("afk", func(c *bot.Context, parsed wa.Parsed) bool {
		if c.SenderKey == "" {
			return false
		}

		// 1) el ausente habló → deja de estar ausente
		if _, ok := afkOf(c, c.SenderKey); ok && parsed.Text != "" && !parsed.IsCommand {
			_ = c.DB.SetSetting(c.Ctx, c.SenderKey, keyAFK, "")
			desde := ""
			if v, ok, _ := c.DB.GetSetting(c.Ctx, c.SenderKey, keyAFKSin); ok && v != "" {
				desde = " (estabas ausente desde " + v + ")"
			}
			_ = c.DB.SetSetting(c.Ctx, c.SenderKey, keyAFKSin, "")
			_ = c.Reply("👋 " + nombre(c) + " volvió" + desde + ". Bienvenido de nuevo.")
			return false // sigue con el mensaje: puede ser un comando
		}

		// 2) alguien mencionó a un ausente
		mentioned := c.Mentioned()
		if len(mentioned) == 0 {
			return false
		}
		if len(mentioned) > 5 { // no se avisa de 50 ausentes a la vez
			mentioned = mentioned[:5]
		}
		var avisos []string
		for _, jid := range mentioned {
			key := c.Bot.IDs.Key(c.Ctx, jid)
			if key == "" {
				continue
			}
			if motivo, ok := afkOf(c, key); ok {
				desde := ""
				if v, ok, _ := c.DB.GetSetting(c.Ctx, key, keyAFKSin); ok && v != "" {
					desde = " · desde " + v
				}
				avisos = append(avisos, fmt.Sprintf("💤 *%s* está ausente%s\n│ %s", shortJID(jid), desde, motivo))
			}
		}
		if len(avisos) == 0 {
			return false
		}
		// Un aviso de afk por persona cada 10 s: en un grupo activo se puede
		// mencionar al mismo ausente muchas veces seguidas.
		if allow, _ := afkGuard.Allow(c.Key()); !allow {
			return true
		}
		_ = c.Reply("╭─「 *AFK* 」\n│ " + strings.Join(avisos, "\n│\n│ "))
		return true
	})

	// ── comando: .antilink ───────────────────────────────────────────────
	bot.Register(bot.Command{
		Name:      "antilink",
		Aliases:   []string{"antienlaces", "nolinks"},
		Category:  bot.CatGroup,
		Desc:      "Activa o desactiva el borrado de enlaces (.antilink on/off)",
		GroupOnly: true,
		AdminOnly: true,
		Run: func(c *bot.Context) error {
			arg := "on"
			if len(c.Args) > 0 {
				arg = strings.ToLower(c.Args[0])
			}

			switch arg {
			case "on", "si", "sí", "1", "activar":
				if err := c.DB.SetSetting(c.Ctx, c.Chat.String(), keyAntilink, "on"); err != nil {
					return err
				}
				aviso := "🔗 Antilink *activado*. Los enlaces de quien no sea admin se borran."
				if !c.BotAdmin() {
					aviso += "\n⚠ No soy admin del grupo: puedo avisar, pero no borrar."
				}
				return c.Reply(aviso)

			case "off", "no", "0", "desactivar":
				if err := c.DB.SetSetting(c.Ctx, c.Chat.String(), keyAntilink, "off"); err != nil {
					return err
				}
				return c.Reply("🔗 Antilink *desactivado*.")

			case "estado", "status":
				on, err := antilinkOn(c)
				if err != nil {
					return err
				}
				estado := "desactivado"
				if on {
					estado = "activado"
				}
				return c.Reply("🔗 Antilink: *" + estado + "*")

			default:
				return c.Reply("Uso: .antilink on | off | estado")
			}
		},
	})

	// ── comando: .afk ────────────────────────────────────────────────────
	bot.Register(bot.Command{
		Name:     "afk",
		Aliases:  []string{"ausente", "away"},
		Category: bot.CatUtils,
		Desc:     "Marca que estás ausente (.afk motivo · .afk off para quitarlo)",
		Run: func(c *bot.Context) error {
			key := c.Key()
			arg := strings.TrimSpace(strings.Join(c.Args, " "))

			if strings.EqualFold(arg, "off") || strings.EqualFold(arg, "no") {
				_ = c.DB.SetSetting(c.Ctx, key, keyAFK, "")
				_ = c.DB.SetSetting(c.Ctx, key, keyAFKSin, "")
				return c.Reply("👋 Ya no estás ausente.")
			}

			motivo := arg
			if motivo == "" {
				motivo = defaultAFK
			}
			// El motivo lo escribe el usuario y el bot lo repite después a
			// terceros: se recorta, se limpia y se escapa el formato para que
			// no pueda falsificar un aviso del bot.
			motivo = bot.ClipText(motivo, 140)

			desde := time.Now().UTC().Format("02/01 15:04")
			if err := c.DB.SetSetting(c.Ctx, key, keyAFK, motivo); err != nil {
				return err
			}
			if err := c.DB.SetSetting(c.Ctx, key, keyAFKSin, desde); err != nil {
				return err
			}

			return c.ReplyButtons(
				"💤 Ausente",
				"╭─「 *AFK* 」\n│ Motivo: "+motivo+"\n│ Desde: "+desde+"\n╰─ te aviso si te mencionan",
				"Escribe cualquier cosa para volver",
				bot.QuickReply("🔓 Quitar AFK", "afk off"),
			)
		},
	})
}

// antilinkOn dice si el grupo tiene el antilink activo.
func antilinkOn(c *bot.Context) (bool, error) {
	v, ok, err := c.DB.GetSetting(c.Ctx, c.Chat.String(), keyAntilink)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return strings.EqualFold(v, "on"), nil
}

// afkOf devuelve el motivo si el usuario está ausente.
func afkOf(c *bot.Context, key string) (string, bool) {
	v, ok, err := c.DB.GetSetting(c.Ctx, key, keyAFK)
	if err != nil || !ok || strings.TrimSpace(v) == "" {
		return "", false
	}
	return v, true
}

// nombre devuelve cómo llamar al que escribe. El PushName es texto libre del
// perfil: se limpia (saltos, invisibles), se recorta y se escapa.
func nombre(c *bot.Context) string {
	if n := bot.CleanName(c.PushName, 24); n != "" {
		return "*" + n + "*"
	}
	return "*" + c.Sender.User + "*"
}

// shortJID deja el número o el LID sin el dominio, para los avisos.
func shortJID(jid types.JID) string {
	return jid.User
}
