// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// economia.go — Puerto de cmds/economy/daily.js y perfil.js.
// Muestra cómo se usa la capa de datos (store) desde un comando.
package cmds

import (
	"fmt"
	"math/rand"
	"time"

	"shin-go/internal/bot"
)

func init() {
	bot.Register(bot.Command{
		Name:     "daily",
		Aliases:  []string{"diario", "recompensa"},
		Category: bot.CatEconomy,
		Desc:     "Recompensa diaria",
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			ctx := c.Ctx
			jid := c.Key() // teléfono si se conoce, LID si no (nunca dos cuentas)
			day := time.Now().UTC().Format("2006-01-02")

			// El día se reserva ANTES de pagar: si el usuario ya lo reclamó, la
			// base de datos lo dice y no se suma nada. Una sola sentencia, sin
			// carrera entre dos mensajes seguidos.
			claimed, err := c.DB.ClaimDaily(ctx, jid, day)
			if err != nil {
				return err
			}
			if !claimed {
				acc, err := c.DB.Get(ctx, jid)
				if err != nil {
					return err
				}
				return c.ReplyButtons(
					"🎁 Recompensa diaria",
					fmt.Sprintf("╭─「 *DAILY* 」\n│ Ya reclamaste tu recompensa hoy.\n│ Vuelve a partir de las 00:00 UTC.\n│ Cartera: *%d*\n╰─", acc.Coins),
					"Una por día",
					bot.QuickReply("👤 Perfil", "perfil"),
				)
			}

			premio := int64(100 + rand.Intn(151))
			coins, err := c.DB.AddCoins(ctx, jid, premio)
			if err != nil {
				return err
			}
			acc, err := c.DB.AddXP(ctx, jid, 50)
			if err != nil {
				return err
			}

			return c.ReplyButtons(
				"🎁 Recompensa diaria",
				fmt.Sprintf("╭─「 *DAILY* 」\n│ Ganaste: *%d* monedas\n│ Cartera: *%d*\n│ Nivel: *%d* (%d XP)\n╰─ vuelve mañana",
					premio, coins, acc.Level, acc.XP),
				"Shin-MD Go",
				bot.QuickReply("👤 Perfil", "perfil"),
				bot.QuickReply("📋 Menú", "menu"),
			)
		},
	})

	bot.Register(bot.Command{
		Name:     "perfil",
		Aliases:  []string{"profile", "yo"},
		Category: bot.CatPerfil,
		Desc:     "Tu perfil y economía",
		Run: func(c *bot.Context) error {
			acc, err := c.DB.Get(c.Ctx, c.Key())
			if err != nil {
				return err
			}
			nombre := c.PushName
			if nombre == "" {
				nombre = c.Sender.User
			}
			last, err := c.DB.LastDaily(c.Ctx, c.Key())
			if err != nil {
				return err
			}
			daily := "disponible ✓"
			if last == time.Now().UTC().Format("2006-01-02") {
				daily = "reclamado hoy"
			}
			return c.Reply(fmt.Sprintf(
				"╭─「 *PERFIL* 」\n│ 👤 %s\n│ 💰 Cartera: %d\n│ 🏦 Banco: %d\n│ ⭐ Nivel: %d (%d XP)\n│ 🎁 Daily: %s\n╰─",
				nombre, acc.Coins, acc.Bank, acc.Level, acc.XP, daily))
		},
	})
}
