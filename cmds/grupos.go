// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// grupos.go — Puerto de cmds/group/*.js: advertencias, expulsiones, ascensos,
// abrir/cerrar, enlace, menciones masivas, info y datos del grupo.
//
// Dos reglas que en el JS no existían y aquí sí:
//
//   - **Nadie se expulsa solo.** El JS tenía `.kick all`, `.kick num +54` y
//     `.kick inactive`, que borraban del grupo a decenas de personas de una
//     tacada. Aquí esos modos solo LISTAN a quién afectarían: la expulsión es
//     siempre uno a uno y con nombre y apellido. Un comando mal escrito no
//     puede vaciar un grupo de 500 personas.
//   - **El bot no se expulsa ni se degrada a sí mismo, ni al creador del grupo,
//     ni al dueño del bot.** Y no se expulsa a un admin.
package cmds

import (
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/bot"
	"shin-go/internal/store"
)

func init() {
	registerWarns()
	registerKick()
	registerPromote()
	registerGroupSettings()
	registerTagAll()
	registerGroupInfo()
	registerActivity()
	registerBotToggle()
}

// ─── utilidades compartidas ─────────────────────────────────────────────────

// adminCtx exige que el que escribe sea admin y que el bot también lo sea,
// porque todo lo que hay en este archivo cambia el grupo.
func mustBeAdmin(c *bot.Context) (bool, error) {
	if !c.IsGroup {
		return false, c.Reply("✕ Ese comando es solo para grupos.")
	}
	if !c.IsOwner && !c.Admin() {
		return false, c.Reply("✕ Necesitas ser admin del grupo para eso.")
	}
	return true, nil
}

func mustBotBeAdmin(c *bot.Context) (bool, error) {
	if !c.BotAdmin() {
		return false, c.Reply("✕ Necesito ser admin del grupo para hacer eso.")
	}
	return true, nil
}

// protected dice si alguien no se puede tocar: el creador del grupo, un admin,
// el dueño del bot o el propio bot.
func protected(c *bot.Context, evt *types.GroupInfo, who types.JID) (string, bool) {
	if evt == nil {
		// Sin info del grupo no se puede comprobar nada: se protege por
		// defecto en vez de expulsar a ciegas.
		return "no pude leer el grupo", true
	}
	if !evt.OwnerJID.IsEmpty() && sameUser(evt.OwnerJID, who) {
		return "es el creador del grupo", true
	}
	if c.Bot.WA.Store.ID != nil && sameUser(*c.Bot.WA.Store.ID, who) {
		return "soy yo", true
	}
	if bot.IsBotOwner(who) {
		return "es el dueño del bot", true
	}
	for _, p := range evt.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			if sameUser(p.JID, who) || sameUser(p.PhoneNumber, who) || sameUser(p.LID, who) {
				return "es administrador del grupo", true
			}
		}
	}
	return "", false
}

// sameUser compara dos JID por el número/LID, que es lo único estable cuando
// uno viene como teléfono y el otro como LID.
func sameUser(a, b types.JID) bool {
	if a.IsEmpty() || b.IsEmpty() {
		return false
	}
	if a.User == b.User {
		return true
	}
	// El teléfono puede venir con o sin el "9" de WhatsApp y con prefijo:
	// se comparan los últimos 10 dígitos, que es lo que no cambia.
	return tail(a.User, 10) == tail(b.User, 10) && len(a.User) >= 10
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// resolveTarget: a quién va el comando (mención o cita).
func resolveTarget(c *bot.Context) (types.JID, bool) {
	if j, ok := target(c); ok {
		return j, true
	}
	return types.EmptyJID, false
}

// resolveTargetWithParticipants: como resolveTarget, pero además devuelve el
// participante real del grupo (con su teléfono si se conoce). Es lo que
// necesita WhatsApp para ascender o expulsar: si le pasas un LID cuando el
// grupo usa teléfono, rechaza la operación.
func resolveTargetWithParticipants(c *bot.Context, evt *types.GroupInfo) (types.JID, bool) {
	who, ok := resolveTarget(c)
	if !ok {
		return types.EmptyJID, false
	}
	if evt == nil {
		return who, true // sin info del grupo se usa el JID tal cual
	}
	for _, p := range evt.Participants {
		if sameUser(p.JID, who) || sameUser(p.PhoneNumber, who) || sameUser(p.LID, who) {
			if !p.PhoneNumber.IsEmpty() {
				return p.PhoneNumber, true
			}
			if !p.JID.IsEmpty() {
				return p.JID, true
			}
			if !p.LID.IsEmpty() {
				return p.LID, true
			}
		}
	}
	return who, true
}

// ─── advertencias ───────────────────────────────────────────────────────────

func registerWarns() {
	// .warn
	bot.Register(bot.Command{
		Name:      "warn",
		Aliases:   []string{"advertir", "advertencia"},
		Category:  bot.CatGroup,
		Desc:      "Advertir a un miembro del grupo",
		GroupOnly: true,
		Cooldown:  2,
		Run: func(c *bot.Context) error {
			ok, err := mustBeAdmin(c)
			if err != nil || !ok {
				return err
			}
			who, ok := resolveTargetWithParticipants(c, groupInfo(c))
			if !ok {
				return c.Reply("《✧》 Debes mencionar o responder al usuario que quieres advertir.")
			}

			// La razón: si hubo mención, el primer argumento es la mención y
			// el resto es la razón; si fue cita, todos los argumentos lo son.
			reason := strings.TrimSpace(strings.Join(skipMention(c.Args), " "))
			if reason == "" {
				reason = "Sin razón."
			}

			total, err := c.DB.AddWarn(c.Ctx, c.Chat.String(), userKey(c, who), reason, c.Key())
			if err != nil {
				return err
			}

			limite, expulsar, err := c.DB.WarnLimit(c.Ctx, c.Chat.String())
			if err != nil {
				return err
			}

			msg := fmt.Sprintf("✐ Se ha añadido una advertencia a @%s.\n✿ Advertencias totales (%d)", who.User, total)

			if limite > 0 && total >= limite {
				if !expulsar {
					msg += "\n\n> ❖ Alcanzó el límite de advertencias del grupo."
				} else if err := c.Bot.Kick(c.Ctx, c.Chat, who); err != nil {
					msg += "\n\n> ❖ Alcanzó el límite, pero no se pudo expulsar: " + err.Error()
				} else {
					if _, err := c.DB.ClearWarns(c.Ctx, c.Chat.String(), userKey(c, who)); err != nil {
						return err
					}
					c.Groups.Invalidate(c.Chat)
					msg += "\n\n> ❖ Alcanzó el límite de advertencias y fue expulsado del grupo."
				}
			}
			return c.Bot.SendMentions(c.Ctx, c.Chat, msg, []types.JID{who}, c.CmdPriority)
		},
	})

	// .warns
	bot.Register(bot.Command{
		Name:      "warns",
		Aliases:   []string{"advertencias", "warnings"},
		Category:  bot.CatGroup,
		Desc:      "Ver las advertencias de alguien",
		GroupOnly: true,
		Cooldown:  2,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			who, ok := resolveTarget(c)
			if !ok {
				return c.Reply("《✧》 Menciona o responde a quien quieras revisar.")
			}
			warns, err := c.DB.Warns(c.Ctx, c.Chat.String(), userKey(c, who))
			if err != nil {
				return err
			}
			if len(warns) == 0 {
				return c.Bot.SendMentions(c.Ctx, c.Chat,
					fmt.Sprintf("《✧》 @%s no tiene advertencias registradas.", who.User),
					[]types.JID{who}, c.CmdPriority)
			}

			var b strings.Builder
			fmt.Fprintf(&b, "✐ Advertencias de @%s\n> ✧ Total: (%d)\n\n", who.User, len(warns))
			for i, w := range warns {
				fmt.Fprintf(&b, "`#%d` » %s\n> » Fecha: %s", len(warns)-i, w.Reason, fecha(w.At))
				if w.ByID != "" {
					fmt.Fprintf(&b, "\n> » Por: %s", strings.SplitN(w.ByID, "@", 2)[0])
				}
				b.WriteString("\n")
			}
			return c.Bot.SendMentions(c.Ctx, c.Chat, b.String(), []types.JID{who}, c.CmdPriority)
		},
	})

	// .delwarn
	bot.Register(bot.Command{
		Name:      "delwarn",
		Aliases:   []string{"deladvertencia", "quitarwarn"},
		Category:  bot.CatGroup,
		Desc:      "Borrar una advertencia (número o all)",
		GroupOnly: true,
		Cooldown:  2,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			who, ok := resolveTarget(c)
			if !ok {
				return c.Reply("《✧》 Debes mencionar o responder al usuario cuya advertencia quieres borrar.")
			}
			key := userKey(c, who)

			arg := ""
			for _, a := range skipMention(c.Args) {
				arg = strings.ToLower(strings.TrimSpace(a))
				break
			}

			if arg == "all" || arg == "todo" {
				n, err := c.DB.ClearWarns(c.Ctx, c.Chat.String(), key)
				if err != nil {
					return err
				}
				return c.Bot.SendMentions(c.Ctx, c.Chat,
					fmt.Sprintf("✐ Se borraron *%d* advertencias de @%s.", n, who.User),
					[]types.JID{who}, c.CmdPriority)
			}

			idx := 0
			if _, err := fmt.Sscanf(arg, "%d", &idx); err != nil {
				return c.Reply("《✧》 Dime el número de la advertencia o `all` para borrarlas todas.")
			}
			restantes, okDel, err := c.DB.DelWarn(c.Ctx, c.Chat.String(), key, idx)
			if err != nil {
				return err
			}
			if !okDel {
				return c.Reply(fmt.Sprintf("ꕥ El número tiene que estar entre 1 y %d.", restantes))
			}
			return c.Bot.SendMentions(c.Ctx, c.Chat,
				fmt.Sprintf("ꕥ Se borró la advertencia #%d de @%s. Quedan *%d*.", idx, who.User, restantes),
				[]types.JID{who}, c.CmdPriority)
		},
	})

	// .setwarnlimit
	bot.Register(bot.Command{
		Name:      "setwarnlimit",
		Aliases:   []string{"limitewarn", "warnlimit"},
		Category:  bot.CatGroup,
		Desc:      "Límite de advertencias antes de expulsar (0 = desactivado)",
		GroupOnly: true,
		Cooldown:  2,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			limite, expulsar, err := c.DB.WarnLimit(c.Ctx, c.Chat.String())
			if err != nil {
				return err
			}
			estado := "Desactivado"
			if limite > 0 && expulsar {
				estado = fmt.Sprintf("%d advertencias", limite)
			}

			if len(c.Args) == 0 {
				return c.Reply(fmt.Sprintf("✐ Límite actual: %s\n> Uso: *%ssetwarnlimit <1-%d>* o *0* para desactivar.\n"+
					"> Con el límite puesto, al alcanzarlo el usuario se expulsa solo.",
					estado, c.Bot.FirstPrefix(), store.MaxWarnLimit))
			}

			n := -1
			if _, err := fmt.Sscanf(c.Args[0], "%d", &n); err != nil || n < 0 || n > store.MaxWarnLimit {
				return c.Reply(fmt.Sprintf("✐ El límite tiene que ser un número entre *1* y *%d*, o *0* para desactivar.\n"+
					"> Estado actual: %s", store.MaxWarnLimit, estado))
			}
			if err := c.DB.SetWarnLimit(c.Ctx, c.Chat.String(), n, n > 0); err != nil {
				return err
			}
			if n == 0 {
				return c.Reply("✐ Desactivado: ya no se expulsará a nadie por acumular advertencias.")
			}
			return c.Reply(fmt.Sprintf("✐ Límite de advertencias: *%d*.\n> Al llegar, el usuario se expulsa solo.", n))
		},
	})
}

// ─── expulsar ───────────────────────────────────────────────────────────────

func registerKick() {
	bot.Register(bot.Command{
		Name:      "kick",
		Aliases:   []string{"ban", "bang", "expulsar", "sacar"},
		Category:  bot.CatGroup,
		Desc:      "Expulsar a un miembro (o listar a quién se podría)",
		GroupOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			if ok, err := mustBotBeAdmin(c); err != nil || !ok {
				return err
			}
			info := groupInfo(c)
			if info == nil {
				return c.Reply("✕ No pude leer la información del grupo.")
			}

			modo := strings.ToLower(firstArg(c))

			// Los modos por lote SOLO listan. No se expulsa a nadie sin nombre.
			if modo == "all" || modo == "inactive" || modo == "inactivos" || modo == "num" || modo == "listnum" {
				return listarLote(c, info, modo)
			}

			who, ok := resolveTargetWithParticipants(c, info)
			if !ok {
				return c.Reply(fmt.Sprintf("《✧》 Menciona o responde a quien quieras expulsar.\n"+
					"> *%skick all* lista a todos los que se podrían sacar (no expulsa).", c.Bot.FirstPrefix()))
			}
			if motivo, bloqueado := protected(c, info, who); bloqueado {
				return c.Reply("《✧》 No puedo expulsar a ese usuario: " + motivo + ".")
			}
			if err := c.Bot.Kick(c.Ctx, c.Chat, who); err != nil {
				return c.Reply("✕ No se pudo expulsar: " + err.Error())
			}
			c.Groups.Invalidate(c.Chat)
			return c.Bot.SendMentions(c.Ctx, c.Chat,
				fmt.Sprintf("✿ @%s fue expulsado del grupo.", who.User), []types.JID{who}, c.CmdPriority)
		},
	})
}

// listarLote responde con la lista de gente a la que afectaría un kick masivo,
// sin tocar a nadie. Es lo que el JS hacía directamente con `groupParticipants
// Update: remove` en bucle (3 s por persona, decenas de expulsiones).
func listarLote(c *bot.Context, info *types.GroupInfo, modo string) error {
	var prefijo string
	if modo == "num" || modo == "listnum" {
		prefijo = strings.TrimPrefix(strings.TrimSpace(argAt(c.Args, 1)), "+")
		if prefijo == "" {
			return c.Reply(fmt.Sprintf("《✧》 Dime el prefijo del país.\n> Ejemplo: *%skick num +52*",
				c.Bot.FirstPrefix()))
		}
	}

	var candidatos []types.JID
	for _, p := range info.Participants {
		if p.IsAdmin || p.IsSuperAdmin {
			continue
		}
		jid := p.JID
		if jid.IsEmpty() {
			jid = p.PhoneNumber
		}
		if jid.IsEmpty() {
			continue
		}
		if _, bloqueado := protected(c, info, jid); bloqueado {
			continue
		}
		switch modo {
		case "num", "listnum":
			if !strings.HasPrefix(jid.User, prefijo) {
				continue
			}
		case "inactive", "inactivos":
			activo, err := activitySince(c, jid, 30*24*time.Hour)
			if err != nil {
				return err
			}
			if activo > 0 {
				continue
			}
		}
		candidatos = append(candidatos, jid)
	}

	if len(candidatos) == 0 {
		return c.Reply("《✧》 No hay nadie que cumpla esa condición (sin contar admins ni al creador).")
	}

	limit := len(candidatos)
	if limit > 40 {
		limit = 40
	}
	var b strings.Builder
	fmt.Fprintf(&b, "《✧》 Con esa condición saldrían *%d* personas:\n\n", len(candidatos))
	for i := 0; i < limit; i++ {
		fmt.Fprintf(&b, "⭔ @%s\n", candidatos[i].User)
	}
	if len(candidatos) > limit {
		fmt.Fprintf(&b, "… y %d más.\n", len(candidatos)-limit)
	}
	b.WriteString("\n> ⚠ *No he expulsado a nadie.* Las expulsiones en lote están desactivadas a propósito: " +
		"borrar decenas de personas de golpe es la vía más rápida a que WhatsApp limite el número.\n" +
		"> Expulsa de una en una: *" + c.Bot.FirstPrefix() + "kick @persona*")
	return c.Bot.SendMentions(c.Ctx, c.Chat, b.String(), candidatos[:limit], c.CmdPriority)
}

// ─── promover / degradar ────────────────────────────────────────────────────

func registerPromote() {
	bot.Register(bot.Command{
		Name:      "promote",
		Aliases:   []string{"promover", "ascender"},
		Category:  bot.CatGroup,
		Desc:      "Hacer admin a un miembro",
		GroupOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			return cambiarRol(c, true)
		},
	})
	bot.Register(bot.Command{
		Name:      "demote",
		Aliases:   []string{"degradar", "quitaradmin"},
		Category:  bot.CatGroup,
		Desc:      "Quitar el admin a un miembro",
		GroupOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			return cambiarRol(c, false)
		},
	})
}

func cambiarRol(c *bot.Context, ascender bool) error {
	if ok, err := mustBeAdmin(c); err != nil || !ok {
		return err
	}
	if ok, err := mustBotBeAdmin(c); err != nil || !ok {
		return err
	}
	info := groupInfo(c)
	if info == nil {
		return c.Reply("✕ No pude leer la información del grupo.")
	}
	who, ok := resolveTargetWithParticipants(c, info)
	if !ok {
		accion := "ascender"
		if !ascender {
			accion = "degradar"
		}
		return c.Reply("《✧》 Menciona a quién quieres " + accion + ".")
	}

	esAdmin := false
	for _, p := range info.Participants {
		if sameUser(p.JID, who) || sameUser(p.PhoneNumber, who) || sameUser(p.LID, who) {
			esAdmin = p.IsAdmin || p.IsSuperAdmin
			break
		}
	}

	if ascender && esAdmin {
		return c.Bot.SendMentions(c.Ctx, c.Chat,
			fmt.Sprintf("《✧》 @%s ya es administrador del grupo.", who.User), []types.JID{who}, c.CmdPriority)
	}
	if !ascender && !esAdmin {
		return c.Bot.SendMentions(c.Ctx, c.Chat,
			fmt.Sprintf("《✧》 @%s no es administrador del grupo.", who.User), []types.JID{who}, c.CmdPriority)
	}

	if !ascender {
		// Degradar al creador o a sí mismo rompe el grupo (o deja al bot sin
		// permisos): se bloquea antes de intentarlo.
		if motivo, bloqueado := protected(c, info, who); bloqueado {
			return c.Reply("《✧》 No puedo quitarle el admin a ese usuario: " + motivo + ".")
		}
	}

	var err error
	if ascender {
		err = c.Bot.Promote(c.Ctx, c.Chat, who)
	} else {
		err = c.Bot.Demote(c.Ctx, c.Chat, who)
	}
	if err != nil {
		return c.Reply("✕ No se pudo cambiar el rol: " + err.Error())
	}
	c.Groups.Invalidate(c.Chat)

	texto := fmt.Sprintf("✿ @%s ahora es administrador del grupo.", who.User)
	if !ascender {
		texto = fmt.Sprintf("✿ @%s ya no es administrador del grupo.", who.User)
	}
	return c.Bot.SendMentions(c.Ctx, c.Chat, texto, []types.JID{who}, c.CmdPriority)
}

// ─── abrir, cerrar, enlace, datos del grupo ─────────────────────────────────

func registerGroupSettings() {
	// .open / .close
	bot.Register(bot.Command{
		Name:      "open",
		Aliases:   []string{"abrir", "abrirgrupo"},
		Category:  bot.CatGroup,
		Desc:      "Abrir el grupo para que todos escriban",
		GroupOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			return abrirCerrar(c, false)
		},
	})
	bot.Register(bot.Command{
		Name:      "close",
		Aliases:   []string{"cerrar", "cerrargrupo"},
		Category:  bot.CatGroup,
		Desc:      "Cerrar el grupo: solo admins escriben",
		GroupOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			return abrirCerrar(c, true)
		},
	})

	// .link / .revoke
	bot.Register(bot.Command{
		Name:      "link",
		Aliases:   []string{"enlace", "linkgrupo"},
		Category:  bot.CatGroup,
		Desc:      "Ver el enlace del grupo",
		GroupOnly: true,
		Cooldown:  5,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			if ok, err := mustBotBeAdmin(c); err != nil || !ok {
				return err
			}
			link, err := c.Bot.InviteLink(c.Ctx, c.Chat, false)
			if err != nil {
				return c.Reply("✕ No pude obtener el enlace: " + err.Error())
			}
			return c.Reply("🔗 Enlace del grupo:\n" + link)
		},
	})
	bot.Register(bot.Command{
		Name:      "revoke",
		Aliases:   []string{"restablecer", "resetlink"},
		Category:  bot.CatGroup,
		Desc:      "Restablecer el enlace (el viejo deja de servir)",
		GroupOnly: true,
		Cooldown:  5,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			if ok, err := mustBotBeAdmin(c); err != nil || !ok {
				return err
			}
			link, err := c.Bot.InviteLink(c.Ctx, c.Chat, true)
			if err != nil {
				return c.Reply("✕ No pude restablecer el enlace: " + err.Error())
			}
			return c.Reply("✿ Enlace restablecido. El anterior ya no funciona:\n" + link)
		},
	})

	// .setgpname
	bot.Register(bot.Command{
		Name:      "setgpname",
		Aliases:   []string{"nombregrupo", "setname"},
		Category:  bot.CatGroup,
		Desc:      "Cambiar el nombre del grupo",
		GroupOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			if ok, err := mustBotBeAdmin(c); err != nil || !ok {
				return err
			}
			nombre := strings.TrimSpace(c.Text)
			if nombre == "" {
				return c.Reply(fmt.Sprintf("《✧》 Escribe el nombre nuevo.\n> Ejemplo: *%ssetgpname Mi grupo*",
					c.Bot.FirstPrefix()))
			}
			if err := c.Bot.SetGroupName(c.Ctx, c.Chat, nombre); err != nil {
				return c.Reply("✕ No se pudo cambiar el nombre: " + err.Error())
			}
			c.Groups.Invalidate(c.Chat)
			return c.Reply("✿ Nombre del grupo cambiado a *" + nombre + "*.")
		},
	})

	// .setgpdesc
	bot.Register(bot.Command{
		Name:      "setgpdesc",
		Aliases:   []string{"descripciongrupo", "setdesc"},
		Category:  bot.CatGroup,
		Desc:      "Cambiar la descripción del grupo",
		GroupOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			if ok, err := mustBotBeAdmin(c); err != nil || !ok {
				return err
			}
			desc := strings.TrimSpace(c.Text)
			if desc == "" {
				return c.Reply(fmt.Sprintf("《✧》 Escribe la descripción nueva.\n> Ejemplo: *%ssetgpdesc Grupo de prueba*",
					c.Bot.FirstPrefix()))
			}
			if err := c.Bot.SetGroupTopic(c.Ctx, c.Chat, desc); err != nil {
				return c.Reply("✕ No se pudo cambiar la descripción: " + err.Error())
			}
			c.Groups.Invalidate(c.Chat)
			return c.Reply("✿ Descripción del grupo actualizada.")
		},
	})

	// .setgpbanner: pone como foto del grupo la imagen que cites
	bot.Register(bot.Command{
		Name:      "setgpbanner",
		Aliases:   []string{"fotogrupo", "setbanner"},
		Category:  bot.CatGroup,
		Desc:      "Poner como foto del grupo la imagen que respondes",
		GroupOnly: true,
		Cooldown:  5,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			if ok, err := mustBotBeAdmin(c); err != nil || !ok {
				return err
			}
			// La imagen tiene que venir citada: así no hay que descargar nada
			// que no haya pedido un admin.
			citado := quotedImage(c)
			if citado == nil {
				return c.Reply("《✧》 Responde a una *imagen* para ponerla como foto del grupo.")
			}
			data, err := c.Bot.DownloadMedia(c.Ctx, citado)
			if err != nil {
				return c.Reply("✕ No pude descargar la imagen: " + err.Error())
			}
			if err := c.Bot.SetGroupPhoto(c.Ctx, c.Chat, data); err != nil {
				return c.Reply("✕ No pude cambiar la foto: " + err.Error())
			}
			return c.Reply("✿ Foto del grupo actualizada.")
		},
	})
}

func abrirCerrar(c *bot.Context, cerrar bool) error {
	if ok, err := mustBeAdmin(c); err != nil || !ok {
		return err
	}
	if ok, err := mustBotBeAdmin(c); err != nil || !ok {
		return err
	}
	if err := c.Bot.SetAnnounce(c.Ctx, c.Chat, cerrar); err != nil {
		return c.Reply("✕ No se pudo cambiar el grupo: " + err.Error())
	}
	c.Groups.Invalidate(c.Chat)
	if cerrar {
		return c.Reply("🔒 Grupo cerrado: solo los administradores pueden escribir.")
	}
	return c.Reply("🔓 Grupo abierto: todos pueden escribir.")
}

// ─── menciones masivas ──────────────────────────────────────────────────────

func registerTagAll() {
	registrar := func(nombre string, oculto bool) {
		bot.Register(bot.Command{
			Name:      nombre,
			Category:  bot.CatGroup,
			Desc:      "Mencionar a todos los del grupo",
			GroupOnly: true,
			Cooldown:  10,
			Run: func(c *bot.Context) error {
				if ok, err := mustBeAdmin(c); err != nil || !ok {
					return err
				}
				info := groupInfo(c)
				if info == nil {
					return c.Reply("✕ No pude leer la información del grupo.")
				}

				texto := strings.TrimSpace(c.Text)
				if texto == "" {
					texto = "📢 Atención a todos."
				}

				var mencionados []types.JID
				for _, p := range info.Participants {
					j := p.JID
					if !p.PhoneNumber.IsEmpty() {
						j = p.PhoneNumber
					}
					if !j.IsEmpty() {
						mencionados = append(mencionados, j)
					}
				}
				if len(mencionados) == 0 {
					return c.Reply("《✧》 No hay a quién mencionar en este grupo.")
				}

				// hidetag: las menciones van en el contexto pero sin el texto
				// "@numero", así que el mensaje se lee limpio y todos reciben
				// la notificación (es lo que hace WhatsApp con una mención).
				if oculto {
					return c.Bot.SendMentions(c.Ctx, c.Chat, texto, mencionados, c.CmdPriority)
				}

				var b strings.Builder
				b.WriteString(texto)
				b.WriteString("\n\n")
				for _, j := range mencionados {
					fmt.Fprintf(&b, "⭔ @%s\n", j.User)
				}
				return c.Bot.SendMentions(c.Ctx, c.Chat, b.String(), mencionados, c.CmdPriority)
			},
		})
	}
	registrar("tagall", false)
	bot.Register(bot.Command{
		Name:      "hidetag",
		Aliases:   []string{"tag", "mencionar"},
		Category:  bot.CatGroup,
		Desc:      "Mencionar a todos sin listar los números",
		GroupOnly: true,
		Cooldown:  10,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			info := groupInfo(c)
			if info == nil {
				return c.Reply("✕ No pude leer la información del grupo.")
			}
			texto := strings.TrimSpace(c.Text)
			if texto == "" {
				texto = "📢 Atención a todos."
			}
			var mencionados []types.JID
			for _, p := range info.Participants {
				j := p.PhoneNumber
				if j.IsEmpty() {
					j = p.JID
				}
				if !j.IsEmpty() {
					mencionados = append(mencionados, j)
				}
			}
			if len(mencionados) == 0 {
				return c.Reply("《✧》 No hay a quién mencionar en este grupo.")
			}
			return c.Bot.SendMentions(c.Ctx, c.Chat, texto, mencionados, c.CmdPriority)
		},
	})
}

// ─── información del grupo ──────────────────────────────────────────────────

func registerGroupInfo() {
	bot.Register(bot.Command{
		Name:      "groupinfo",
		Aliases:   []string{"gp", "infogrupo", "grupinfo"},
		Category:  bot.CatGroup,
		Desc:      "Información del grupo",
		GroupOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			info := groupInfo(c)
			if info == nil {
				return c.Reply("✕ No pude leer la información del grupo.")
			}
			admins := 0
			for _, p := range info.Participants {
				if p.IsAdmin || p.IsSuperAdmin {
					admins++
				}
			}
			creador := "Desconocido"
			if !info.OwnerJID.IsEmpty() {
				creador = "@" + info.OwnerJID.User
			}
			estado := "abierto 🔓"
			if info.IsAnnounce {
				estado = "cerrado 🔒 (solo admins)"
			}
			desc := strings.TrimSpace(info.Topic)
			if desc == "" {
				desc = "(sin descripción)"
			}
			limite, expulsar, err := c.DB.WarnLimit(c.Ctx, c.Chat.String())
			if err != nil {
				return err
			}
			limiteTxt := "desactivado"
			if limite > 0 && expulsar {
				limiteTxt = fmt.Sprintf("%d advertencias", limite)
			}

			return c.Reply(fmt.Sprintf(
				"╭─「 *GRUPO* 」\n│ 📛 %s\n│ 👥 Miembros: *%d* · Admins: *%d*\n│ 👑 Creador: %s\n"+
					"│ 🔒 Modo: %s\n│ ✐ Límite de warns: %s\n│ 📝 %s\n╰─",
				info.Name, len(info.Participants), admins, creador, estado, limiteTxt, desc))
		},
	})
}

// ─── actividad ──────────────────────────────────────────────────────────────

func registerActivity() {
	bot.Register(bot.Command{
		Name:      "count",
		Aliases:   []string{"mensajes", "messages", "msgcount"},
		Category:  bot.CatGroup,
		Desc:      "Cuántos mensajes lleva alguien",
		GroupOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			who := c.Sender
			if j, ok := resolveTarget(c); ok {
				who = j
			}
			total, hoy, err := c.DB.Messages(c.Ctx, c.Chat.String(), userKey(c, who), time.Now().AddDate(0, 0, -1))
			if err != nil {
				return err
			}
			return c.Bot.SendMentions(c.Ctx, c.Chat, fmt.Sprintf(
				"✿ @%s\n> ⌦ Mensajes totales: *%s*\n> ⌦ Últimas 24 h: *%s*",
				who.User, miles(total), miles(hoy)), []types.JID{who}, c.CmdPriority)
		},
	})

	bot.Register(bot.Command{
		Name:      "topcount",
		Aliases:   []string{"topmensajes", "topmsgcount", "topmessages"},
		Category:  bot.CatGroup,
		Desc:      "Ranking de mensajes del mes",
		GroupOnly: true,
		Cooldown:  5,
		Run: func(c *bot.Context) error {
			filas, err := c.DB.TopActivity(c.Ctx, c.Chat.String(), time.Now().AddDate(0, 0, -30), 10)
			if err != nil {
				return err
			}
			if len(filas) == 0 {
				return c.Reply("ꕥ Todavía no hay mensajes contados en este grupo.")
			}
			var b strings.Builder
			b.WriteString("✿ Top de mensajes (últimos 30 días)\n\n")
			var menciones []types.JID
			for i, f := range filas {
				fmt.Fprintf(&b, "✩ %d › @%s · *%s*\n", i+1, strings.SplitN(f.UserID, "@", 2)[0], miles(f.Msgs))
				if j, err := types.ParseJID(f.UserID); err == nil {
					menciones = append(menciones, j)
				}
			}
			return c.Bot.SendMentions(c.Ctx, c.Chat, b.String(), menciones, c.CmdPriority)
		},
	})

	bot.Register(bot.Command{
		Name:      "topinactive",
		Aliases:   []string{"topinactivos", "inactivos"},
		Category:  bot.CatGroup,
		Desc:      "Quiénes no han escrito en 30 días",
		GroupOnly: true,
		Cooldown:  10,
		Run: func(c *bot.Context) error {
			info := groupInfo(c)
			if info == nil {
				return c.Reply("✕ No pude leer la información del grupo.")
			}
			activos, err := c.DB.ActivityOf(c.Ctx, c.Chat.String(), time.Now().AddDate(0, 0, -30))
			if err != nil {
				return err
			}

			var inactivos []types.JID
			for _, p := range info.Participants {
				j := p.PhoneNumber
				if j.IsEmpty() {
					j = p.JID
				}
				if j.IsEmpty() || p.IsAdmin || p.IsSuperAdmin {
					continue
				}
				if activos[userKey(c, j)] == 0 {
					inactivos = append(inactivos, j)
				}
			}
			if len(inactivos) == 0 {
				return c.Reply("ꕥ Este grupo está activo: nadie lleva 30 días callado.")
			}

			limit := len(inactivos)
			if limit > 40 {
				limit = 40
			}
			var b strings.Builder
			fmt.Fprintf(&b, "✿ Sin escribir en 30 días: *%d*\n\n", len(inactivos))
			for i := 0; i < limit; i++ {
				fmt.Fprintf(&b, "⭔ @%s\n", inactivos[i].User)
			}
			if len(inactivos) > limit {
				fmt.Fprintf(&b, "… y %d más.\n", len(inactivos)-limit)
			}
			return c.Bot.SendMentions(c.Ctx, c.Chat, b.String(), inactivos[:limit], c.CmdPriority)
		},
	})
}

// ─── apagar el bot en el grupo ──────────────────────────────────────────────

func registerBotToggle() {
	bot.Register(bot.Command{
		Name:      "bot",
		Category:  bot.CatGroup,
		Desc:      "Apagar o encender el bot en este grupo",
		GroupOnly: true,
		Cooldown:  3,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			off, err := c.DB.BotOff(c.Ctx, c.Chat.String())
			if err != nil {
				return err
			}
			switch strings.ToLower(firstArg(c)) {
			case "":
				estado := "encendido ✅"
				if off {
					estado = "apagado ⛔"
				}
				return c.Reply(fmt.Sprintf("「✿」El bot está *%s* en este grupo.\n> *%sbot off* lo apaga, *%sbot on* lo enciende.",
					estado, c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			case "off", "apagar", "0", "false":
				if err := c.DB.SetSetting(c.Ctx, c.Chat.String(), store.KeyBotOff, "off"); err != nil {
					return err
				}
				bot.ForgetChat(c.Chat)
				return c.Reply("⛔ Bot *apagado* en este grupo. Solo responderá a este comando desde el panel del dueño.")
			case "on", "encender", "1", "true":
				if err := c.DB.SetSetting(c.Ctx, c.Chat.String(), store.KeyBotOff, "on"); err != nil {
					return err
				}
				bot.ForgetChat(c.Chat)
				return c.Reply("✅ Bot *encendido* en este grupo.")
			default:
				return c.Reply(fmt.Sprintf("《✧》 Usa *%sbot off* o *%sbot on*",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}
		},
	})
}
