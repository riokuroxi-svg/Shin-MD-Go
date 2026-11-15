// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// grupos_extra.go — las opciones del JS que faltaban: welcome/goodbye (toggle
// y texto), alerts, autotraducir, clear, tagall, topinactive y setprimary.
//
// Los toggles usan las MISMAS claves que el JS ("welcome", "goodbye"…) para
// que una base migrada se lea igual. autotraducir se conecta aquí como hook:
// si está encendido y el mensaje no es comando y no parece español, se
// contesta con la traducción debajo (como hacía el JS).
package cmds

import (
	"fmt"
	"strconv"
	"strings"

	"shin-go/internal/apis"
	"shin-go/internal/bot"
	"shin-go/internal/store"
	"shin-go/internal/wa"

	"go.mau.fi/whatsmeow/types"
)

func init() {
	// welcome / goodbye / alerts / autotraducir: todos "nombre on|off"
	for _, p := range []struct{ nombre, alias, clave, que string }{
		{"welcome", "bienvenida", store.KeyWelcome, "saludo al entrar"},
		{"goodbye", "despedida", store.KeyGoodbye, "mensaje al salir"},
		{"alerts", "", store.KeyAlerts, "avisos del grupo"},
		{"autotraducir", "autotrad,autotr", store.KeyAutoTraducir, "traducción automática"},
	} {
		nombre, clave, que := p.nombre, p.clave, p.que
		var aliases []string
		if p.alias != "" {
			aliases = strings.Split(p.alias, ",")
		}
		bot.Register(bot.Command{
			Name: nombre, Aliases: aliases, Category: bot.CatGroup, GroupOnly: true,
			Desc: "Activa o quita " + que + ": ." + nombre + " on|off.",
			Run:  func(c *bot.Context) error { return cmdToggleGrupo(c, clave, que) },
		})
	}

	bot.Register(bot.Command{
		Name: "setwelcome", Category: bot.CatGroup, GroupOnly: true,
		Desc: "Texto de bienvenida del chat (variables {user} {group}).",
		Run:  func(c *bot.Context) error { return cmdSetTextoGrupo(c, store.KeySetWelcome, "bienvenida") },
	})
	bot.Register(bot.Command{
		Name: "setgoodbye", Category: bot.CatGroup, GroupOnly: true,
		Desc: "Texto de despedida del chat (variables {user} {group}).",
		Run:  func(c *bot.Context) error { return cmdSetTextoGrupo(c, store.KeySetGoodbye, "despedida") },
	})
	bot.Register(bot.Command{
		Name: "clear", Category: bot.CatGroup, GroupOnly: true,
		Desc: "Borra mensajes propios recientes del bot: .clear [n].",
		Run:  cmdClear,
	})
	bot.Register(bot.Command{
		Name: "tagall", Category: bot.CatGroup, GroupOnly: true,
		Desc: "Menciona a todo el grupo: .tagall <mensaje>.",
		Run:  cmdTagall,
	})
	bot.Register(bot.Command{
		Name: "setprimary", Category: bot.CatGroup, GroupOnly: true,
		Desc: "Marca este bot como principal si hay varios (los demás se callan).",
		Run:  cmdSetPrimary,
	})

	// hook: autotraducir
	bot.RegisterHook("autotraducir", hookAutoTraducir)
}

func soloAdmin(c *bot.Context) (bool, error) {
	if c.IsGroup && !c.Admin() {
		return false, c.Reply("Eso lo decide un administrador del grupo.")
	}
	return true, nil
}

func cmdToggleGrupo(c *bot.Context, clave, que string) error {
	if ok, err := soloAdmin(c); !ok {
		return err
	}
	chat := c.Chat.String()
	arg := strings.ToLower(strings.TrimSpace(c.Text))
	switch arg {
	case "on", "activar", "si", "1", "true":
		if err := c.DB.SetFlag(c.Ctx, chat, clave, true); err != nil {
			return err
		}
		return c.Reply("✔ " + que + ": *on*")
	case "off", "desactivar", "no", "0", "false":
		if err := c.DB.SetFlag(c.Ctx, chat, clave, false); err != nil {
			return err
		}
		return c.Reply("✔ " + que + ": *off*")
	default:
		on, err := c.DB.Flag(c.Ctx, chat, clave, false)
		if err != nil {
			return err
		}
		estado := "off"
		if on {
			estado = "on"
		}
		return c.Reply(fmt.Sprintf("%s está *%s*. Cámbialo con *.%s on* o *.%s off*.", que, estado, c.Command, c.Command))
	}
}

func cmdSetTextoGrupo(c *bot.Context, clave, que string) error {
	if ok, err := soloAdmin(c); !ok {
		return err
	}
	texto := strings.TrimSpace(c.Text)
	if texto == "" {
		actual, err := c.DB.Text(c.Ctx, c.Chat.String(), clave, "")
		if err != nil {
			return err
		}
		if actual == "" {
			return c.Reply("no hay texto guardado; ponlo con *." + c.Command + " <texto>* (acepta {user} y {group}).")
		}
		return c.Reply("Texto de " + que + " actual:\n\n" + actual)
	}
	if err := c.DB.SetSetting(c.Ctx, c.Chat.String(), clave, texto); err != nil {
		return err
	}
	return c.Reply("✔ Texto de " + que + " guardado.")
}

func cmdClear(c *bot.Context) error {
	if ok, err := soloAdmin(c); !ok {
		return err
	}
	deseo := 10
	if len(c.Args) > 0 {
		if n, err := strconv.Atoi(c.Args[0]); err == nil && n > 0 {
			deseo = n
			if deseo > 64 {
				deseo = 64
			}
		}
	}
	n, err := c.Bot.LimpiarMios(c.Ctx, c.Chat, deseo)
	if err != nil {
		return err
	}
	if n == 0 {
		return c.Reply("No tengo mensajes míos recientes para borrar aquí (solo puedo con los que envié yo).")
	}
	return c.Reply(fmt.Sprintf("🧹 Borré %d mensajes míos de los últimos tiempos.", n))
}

func cmdTagall(c *bot.Context) error {
	if ok, err := soloAdmin(c); !ok {
		return err
	}
	info, err := c.Bot.GroupInfo(c.Ctx, c.Chat)
	if err != nil {
		return c.Reply("No pude leer el grupo: " + err.Error())
	}
	var menciones []types.JID
	var sb strings.Builder
	sb.WriteString("📢 " + strings.TrimSpace(c.Text) + "\n\n")
	for _, p := range info.Participants {
		jid := p.JID.ToNonAD()
		menciones = append(menciones, jid)
		sb.WriteString("· @" + jid.User + "\n")
	}
	if len(menciones) == 0 {
		return c.Reply("el grupo se lee vacío, algo anda raro.")
	}
	return c.Bot.SendMentions(c.Ctx, c.Chat, strings.TrimRight(sb.String(), "\n"), menciones, c.Priority())
}

func cmdSetPrimary(c *bot.Context) error {
	if ok, err := soloAdmin(c); !ok {
		return err
	}
	if err := c.DB.SetSetting(c.Ctx, c.Chat.String(), store.KeyPrimary, c.Bot.OwnID().String()); err != nil {
		return err
	}
	return c.Reply("✔ Quedo como bot principal del chat; si hay clones, se callan.")
}

// hookAutoTraducir: con la opción encendida, si el mensaje no es comando y
// no parece español, se contesta debajo con la traducción. No interfiere en
// los comandos: deja pasar siempre (devuelve false).
func hookAutoTraducir(c *bot.Context, parsed wa.Parsed) bool {
	if parsed.IsCommand || !c.IsGroup {
		return false
	}
	on, err := c.DB.Flag(c.Ctx, c.Chat.String(), store.KeyAutoTraducir, false)
	if err != nil || !on {
		return false
	}
	texto := strings.TrimSpace(parsed.Text)
	if len(texto) < 8 {
		return false
	}
	res := apis.Traducir(c.Ctx, texto, "es")
	if res.OK && strings.ToLower(res.De) != "es" {
		_ = c.Reply("🌐 (" + res.De + "→es) " + res.Texto)
	}
	return false
}
