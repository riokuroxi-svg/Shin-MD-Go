// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// identidad.go — Comandos de la capa LID: .lid (diagnóstico), .self y .public
// (modo del bot).
//
// .lid existe por una razón muy concreta: tu otro bot no puede resolver LID y
// no hay manera de ver por qué. Este imprime la identidad del que escribe, la
// de quien menciona y lo que se sabe del grupo, así se comprueba en el teléfono
// en diez segundos.
package cmds

import (
	"fmt"
	"strings"

	"shin-go/internal/bot"
	"shin-go/internal/wa"
)

func init() {
	bot.Register(bot.Command{
		Name:     "lid",
		Aliases:  []string{"id", "quien", "identidad"},
		Category: bot.CatUtils,
		Desc:     "Muestra cómo ve el bot tu identidad (teléfono · LID)",
		Run: func(c *bot.Context) error {
			id := c.Identity

			var sb strings.Builder
			sb.WriteString("╭─「 *IDENTIDAD* 」\n")
			fmt.Fprintf(&sb, "│ Llave: %s\n", orDash(c.Key()))
			fmt.Fprintf(&sb, "│ Teléfono: %s\n", orDash(id.PN.User))
			fmt.Fprintf(&sb, "│ LID: %s\n", orDash(id.LID.User))
			if c.SenderAlt.IsEmpty() {
				sb.WriteString("│ Alternativo: (no vino en el mensaje)\n")
			} else {
				fmt.Fprintf(&sb, "│ Alternativo: %s\n", c.SenderAlt.String())
			}
			if id.Known() {
				sb.WriteString("│ Estado: completo ✓ (cuenta estable)\n")
			} else {
				sb.WriteString("│ Estado: solo LID · se completará al ver un grupo o mensaje con teléfono\n")
			}

			if c.IsGroup {
				fmt.Fprintf(&sb, "│ Grupo: %s\n", c.Chat.User)
				fmt.Fprintf(&sb, "│ Eres admin: %s\n", siNo(c.Admin()))
				fmt.Fprintf(&sb, "│ Soy admin: %s\n", siNo(c.BotAdmin()))
			}
			sb.WriteString("╰─")

			return c.Reply(sb.String())
		},
	})

	bot.Register(bot.Command{
		Name:      "self",
		Aliases:   []string{"privado", "soloowner"},
		Category:  bot.CatOwner,
		Desc:      "Modo self: el bot solo obedece al owner",
		OwnerOnly: true,
		Priority:  true,
		Run: func(c *bot.Context) error {
			r := bot.ActiveRouter()
			if r == nil {
				return c.Reply("✕ El router no está disponible.")
			}
			if err := r.SetMode(c.Ctx, bot.ModeSelf); err != nil {
				return err
			}
			return c.Reply("🔒 Modo *self*: a partir de ahora solo respondo al owner.")
		},
	})

	bot.Register(bot.Command{
		Name:      "public",
		Aliases:   []string{"publico", "público", "todos"},
		Category:  bot.CatOwner,
		Desc:      "Modo público: el bot responde a todos",
		OwnerOnly: true,
		Priority:  true,
		Run: func(c *bot.Context) error {
			r := bot.ActiveRouter()
			if r == nil {
				return c.Reply("✕ El router no está disponible.")
			}
			if err := r.SetMode(c.Ctx, bot.ModePublic); err != nil {
				return err
			}
			return c.Reply("🔓 Modo *público*: respondo a todos.")
		},
	})
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func siNo(v bool) string {
	if v {
		return "sí ✓"
	}
	return "no ✕"
}

var _ = wa.IsLID
