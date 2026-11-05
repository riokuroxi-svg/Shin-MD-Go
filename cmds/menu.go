// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// menu.go — Puerto de cmds/main/menu.js + el .menucat con botones.
// Es el comando que mejor demuestra las dos cosas que te importan:
// botones nativos (con nodos <biz>) y arranque rápido (.menu con prioridad).
package cmds

import (
	"fmt"
	"sort"
	"strings"

	"shin-go/internal/bot"
	"shin-go/internal/ui"
)

func init() {
	bot.Register(bot.Command{
		Name:     "menu",
		Aliases:  []string{"ayuda", "help", "h"},
		Category: bot.CatMain,
		Desc:     "Muestra el menú con botones",
		Priority: true, // no espera el calentamiento: es el comando que más se usa
		Run: func(c *bot.Context) error {
			unique, aliases := bot.Count()

			// .menu <categoría> → imagen de esa categoría (si MENU_IMAGE=1) o texto
			if len(c.Args) > 0 {
				cat := strings.ToLower(c.Args[0])
				if c.Bot.Config().MenuImage {
					return sendFax(c, cat)
				}
				return menuCategory(c, cat)
			}

			// MENU_IMAGE=1: el menú sale como imagen dibujada (más vistoso y
			// no se puede "copiar" el formato). Por defecto va con botones,
			// que es más rápido y funciona en cualquier teléfono.
			if c.Bot.Config().MenuImage {
				return sendFax(c, "")
			}

			body := fmt.Sprintf(
				"╭─「 *%s* 」\n"+
					"│ 反魂 · núcleo en Go\n"+
					"│ %d comandos · %d nombres · %d hooks\n"+
					"╰─ prefijos: %s",
				bot.CleanName(c.Bot.Config().BotName, 24),
				unique, aliases, bot.HookCount(),
				strings.Join(c.Bot.Prefixes(), " "))

			return c.ReplyButtons(
				c.Bot.Config().BotName,
				body,
				"Toca un botón o escribe .menu <categoría>",
				bot.QuickReply("📋 Categorías", "menucat"),
				bot.QuickReply("🖼 Fax", "fax"),
				bot.QuickReply("🏓 Ping", "ping"),
				bot.CTACopy("📮 Copiar prefijo", c.Bot.FirstPrefix()),
				bot.CTAURL("🌐 Repositorio", repoURL),
			)
		},
	})

	bot.Register(bot.Command{
		Name:     "menucat",
		Aliases:  []string{"categorias", "menucat"},
		Category: bot.CatMain,
		Desc:     "Lista de categorías con sus comandos",
		Priority: true,
		Run: func(c *bot.Context) error {
			if len(c.Args) > 0 {
				return menuCategory(c, strings.ToLower(c.Args[0]))
			}

			cats := bot.ByCategory()
			names := make([]string, 0, len(cats))
			for cat := range cats {
				names = append(names, string(cat))
			}
			sort.Strings(names)

			var sb strings.Builder
			sb.WriteString("╭─「 *CATEGORÍAS* 」\n")
			for _, name := range names {
				fmt.Fprintf(&sb, "│ ▸ *%s* (%d)\n", name, len(cats[bot.Category(name)]))
			}
			sb.WriteString("╰─ .menu <categoría> para ver los comandos")

			// Un botón por categoría: WhatsApp acepta hasta 10 en native_flow.
			var btns []ui.Button
			for i, name := range names {
				if i >= 8 {
					break
				}
				btns = append(btns, bot.QuickReply("📁 "+name, "menu:"+name))
			}
			return c.ReplyButtons("SHIN-MD", sb.String(), "Elige una categoría", btns...)
		},
	})
}

func menuCategory(c *bot.Context, cat string) error {
	cats := bot.ByCategory()
	cmds, ok := cats[bot.Category(cat)]
	if !ok {
		// El nombre de categoría viene del usuario y se repite en la
		// respuesta: se limpia y se recorta antes de mostrarlo.
		return c.Reply("✕ No existe la categoría *" + bot.CleanName(cat, 24) + "*.\nUsa .menucat")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭─「 *%s* 」\n", strings.ToUpper(cat))
	for _, cmd := range cmds {
		fmt.Fprintf(&sb, "│ %s%s — %s\n",
			c.Bot.FirstPrefix(), cmd.Name, cmd.Desc)
	}
	sb.WriteString("╰─")

	return c.ReplyPriority(sb.String())
}
