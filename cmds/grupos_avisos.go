// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// grupos_avisos.go — Lo que el bot hace SIN que le hablen: contar mensajes,
// recibir a quien entra, despedir a quien sale y borrar los estados que se
// comparten en el grupo (antistatus).
//
// Diferencias con el JS:
//
//   - **El contador de mensajes no reescribe un JSON.** El JS guardaba
//     `chat_users.stats = {día: {msgs}}` y en CADA mensaje leía el objeto
//     entero y lo volvía a escribir. Aquí es un UPSERT de una fila.
//   - **antistatus no adivina a quién borrar.** El JS borraba a partir de
//     campos que a veces venían vacíos y acababa borrando mensajes de otra
//     persona. Aquí solo se borra lo que se identifica con certeza: el mensaje
//     que ES un estado de grupo, o el que se citó, con su autor.
//   - **La bienvenida solo se manda si está activada.** En el JS el aviso se
//     disparaba y el ajuste se consultaba tarde.
package cmds

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"shin-go/internal/bot"
	"shin-go/internal/store"
	"shin-go/internal/wa"
	"shin-go/internal/xlog"
)

// Textos por defecto de bienvenida y despedida.
const (
	bienvenidaPorDefecto = "👋 Bienvenido *@user* a *#grupo*.\n> Ya somos #miembros."
	despedidaPorDefecto  = "👋 *@user* salió de *#grupo*.\n> Quedamos #miembros."
	// MaxPlantilla: lo que se puede guardar como texto de bienvenida. Sin tope,
	// alguien guarda 40 KB de texto y el bot los manda cada vez que entra gente.
	MaxPlantilla = 500
)

func init() {
	// Orden importante: actividad primero (cuenta todo, incluso lo que luego
	// se borra) y antistatus después.
	bot.RegisterHook("actividad", hookActividad)
	bot.RegisterHook("antistatus", hookAntiStatus)

	registerAntiStatusCmd()
	registerWelcomeCmds()

	bot.RegisterGroupEvent("bienvenida", onGroupChange)
}

// ─── contador de actividad ──────────────────────────────────────────────────

// hookActividad suma un mensaje por persona y día. No devuelve true nunca: es
// un contador, no un filtro, y el mensaje sigue su curso.
func hookActividad(c *bot.Context, parsed wa.Parsed) bool {
	if !c.IsGroup {
		return false // en privado no hay grupo que medir
	}
	// Un fallo al contar no puede tumbar el mensaje: se registra y se sigue.
	if err := c.DB.AddActivity(c.Ctx, c.Chat.String(), c.Key(), time.Now()); err != nil {
		c.Log.Warn("no se pudo contar el mensaje: " + err.Error())
	}
	return false
}

// ─── antistatus ─────────────────────────────────────────────────────────────

// hookAntiStatus borra los estados de grupo compartidos en el chat. Es el
// `antistatus` del JS: si alguien comparte un estado en el grupo, se borra y
// queda advertido.
//
// Condiciones para actuar (las mismas del JS): el chat es un grupo, el ajuste
// está activo, el que escribe NO es admin y el bot SÍ es admin (sin permiso de
// borrar, intentarlo solo genera errores y spam).
func hookAntiStatus(c *bot.Context, parsed wa.Parsed) bool {
	if !c.IsGroup || c.Event == nil || c.Event.Message == nil {
		return false
	}
	activo, err := c.DB.Flag(c.Ctx, c.Chat.String(), store.KeyAntiStatus, false)
	if err != nil || !activo {
		return false
	}
	if c.Admin() || !c.BotAdmin() {
		return false
	}

	msg := wa.Unwrap(c.Event.Message)
	if msg == nil {
		return false
	}

	// Caso 1: este mismo mensaje ES un estado de grupo.
	if msg.GetGroupStatusMentionMessage() != nil {
		return borrarEstado(c, c.Event.Info.ID, c.Sender)
	}

	// Caso 2: el mensaje cita un estado de grupo.
	ci := wa.ContextInfo(msg)
	if ci == nil {
		return false
	}
	citado := ci.GetQuotedMessage()
	if citado == nil {
		return false
	}
	inner := wa.Unwrap(citado)
	if inner == nil || inner.GetGroupStatusMentionMessage() == nil {
		return false
	}

	autor := c.Sender
	if p := ci.GetParticipant(); p != "" {
		if j, err := types.ParseJID(p); err == nil {
			autor = j.ToNonAD()
		}
	}
	borrarEstado(c, types.MessageID(ci.GetStanzaID()), autor)
	// Y el mensaje que lo cita (el "comando" del que abusa) también se va.
	borrarEstado(c, c.Event.Info.ID, c.Sender)
	return true
}

// borrarEstado borra el mensaje y apunta la advertencia. Devuelve true si el
// hook ya atendió el mensaje.
func borrarEstado(c *bot.Context, id types.MessageID, autor types.JID) bool {
	if id == "" {
		return false
	}
	if err := c.Bot.DeleteMessage(c.Ctx, c.Chat, autor, id); err != nil {
		c.Log.Warn("antistatus: no se pudo borrar el mensaje: " + err.Error())
		// Sin borrado no se avisa ni se advierte: sería acusar sin pruebas.
		return false
	}

	total, err := c.DB.AddWarn(c.Ctx, c.Chat.String(), userKey(c, autor), "Anti-Status detectado", estadoBy())
	if err != nil {
		c.Log.Warn("antistatus: no se pudo advertir: " + err.Error())
		return true
	}

	limite, expulsar, err := c.DB.WarnLimit(c.Ctx, c.Chat.String())
	if err == nil && limite > 0 && total >= limite && expulsar {
		if err := c.Bot.Kick(c.Ctx, c.Chat, autor); err == nil {
			_, _ = c.DB.ClearWarns(c.Ctx, c.Chat.String(), userKey(c, autor))
			c.Groups.Invalidate(c.Chat)
			_ = c.Bot.SendMentions(c.Ctx, c.Chat,
				fmt.Sprintf("✐ @%s compartió un estado y alcanzó el límite de advertencias: expulsado.",
					autor.User), []types.JID{autor}, true)
			return true
		}
	}

	_ = c.Bot.SendMentions(c.Ctx, c.Chat,
		fmt.Sprintf("✐ @%s: los estados no se comparten en este grupo (advertencia %d).",
			autor.User, total), []types.JID{autor}, true)
	return true
}

// estadoBy firma las advertencias automáticas. Es una marca fija y visible, no
// el número de nadie: así en `.warns` se ve que la puso el bot y no otro
// miembro del grupo.
func estadoBy() string { return "antistatus" }

// ─── .antistatus on|off ─────────────────────────────────────────────────────

func registerAntiStatusCmd() {
	bot.Register(bot.Command{
		Name:      "antistatus",
		Aliases:   []string{"antiestados", "antiestado"},
		Category:  bot.CatGroup,
		Desc:      "Borrar los estados que se comparten en el grupo",
		GroupOnly: true,
		Cooldown:  2,
		Run: func(c *bot.Context) error {
			if ok, err := mustBeAdmin(c); err != nil || !ok {
				return err
			}
			on, err := c.DB.Flag(c.Ctx, c.Chat.String(), store.KeyAntiStatus, false)
			if err != nil {
				return err
			}
			switch strings.ToLower(firstArg(c)) {
			case "":
				estado := "desactivado ⛔"
				if on {
					estado = "activado ✅"
				}
				return c.Reply(fmt.Sprintf("「✿」Anti-estados está *%s* en este grupo.\n> *%santistatus on* / *%santistatus off*",
					estado, c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			case "on", "si", "sí", "1", "true":
				if err := c.DB.SetFlag(c.Ctx, c.Chat.String(), store.KeyAntiStatus, true); err != nil {
					return err
				}
				return c.Reply("ꕥ Anti-estados *activado*: se borrarán los estados que se compartan aquí.")
			case "off", "no", "0", "false":
				if err := c.DB.SetFlag(c.Ctx, c.Chat.String(), store.KeyAntiStatus, false); err != nil {
					return err
				}
				return c.Reply("ꕥ Anti-estados *desactivado*.")
			default:
				return c.Reply(fmt.Sprintf("《✧》 Usa *%santistatus on* o *%santistatus off*",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}
		},
	})
}

// ─── bienvenida y despedida ─────────────────────────────────────────────────

func registerWelcomeCmds() {
	// .welcome / .goodbye on|off
	interruptor := func(nombre string, clave string, etiqueta string) {
		bot.Register(bot.Command{
			Name:      nombre,
			Category:  bot.CatGroup,
			Desc:      "Activar o desactivar " + etiqueta,
			GroupOnly: true,
			Cooldown:  2,
			Run: func(c *bot.Context) error {
				if ok, err := mustBeAdmin(c); err != nil || !ok {
					return err
				}
				on, err := c.DB.Flag(c.Ctx, c.Chat.String(), clave, false)
				if err != nil {
					return err
				}
				switch strings.ToLower(firstArg(c)) {
				case "":
					estado := "desactivada ⛔"
					if on {
						estado = "activada ✅"
					}
					return c.Reply(fmt.Sprintf("「✿」%s está *%s*.\n> *%s%s on* / *%s%s off*",
						etiqueta, estado, c.Bot.FirstPrefix(), nombre, c.Bot.FirstPrefix(), nombre))
				case "on", "si", "sí", "1", "true":
					if err := c.DB.SetFlag(c.Ctx, c.Chat.String(), clave, true); err != nil {
						return err
					}
					return c.Reply(fmt.Sprintf("ꕥ %s *activada*.", etiqueta))
				case "off", "no", "0", "false":
					if err := c.DB.SetFlag(c.Ctx, c.Chat.String(), clave, false); err != nil {
						return err
					}
					return c.Reply(fmt.Sprintf("ꕥ %s *desactivada*.", etiqueta))
				default:
					return c.Reply(fmt.Sprintf("《✧》 Usa *%s%s on* o *%s%s off*",
						c.Bot.FirstPrefix(), nombre, c.Bot.FirstPrefix(), nombre))
				}
			},
		})
	}
	interruptor("welcome", store.KeyWelcome, "la bienvenida")
	interruptor("goodbye", store.KeyGoodbye, "la despedida")

	// .setwelcome / .setgoodbye
	plantilla := func(nombre, clave, claveTexto, etiqueta, porDefecto string) {
		bot.Register(bot.Command{
			Name:      nombre,
			Category:  bot.CatGroup,
			Desc:      "Texto de " + etiqueta,
			GroupOnly: true,
			Cooldown:  2,
			Run: func(c *bot.Context) error {
				if ok, err := mustBeAdmin(c); err != nil || !ok {
					return err
				}
				if len(c.Args) == 0 {
					actual, err := c.DB.Text(c.Ctx, c.Chat.String(), clave, "")
					if err != nil {
						return err
					}
					if actual == "" {
						actual = porDefecto
					}
					return c.Reply(fmt.Sprintf("✐ Texto de %s actual:\n%s\n\n> Marcadores: *@user* · *#grupo* · *#miembros*\n"+
						"> Cambiarlo: *%s%s <texto>*  ·  Volver al de fábrica: *%s%s reset*",
						etiqueta, actual, c.Bot.FirstPrefix(), nombre, c.Bot.FirstPrefix(), nombre))
				}

				texto := strings.TrimSpace(c.Text)
				if strings.EqualFold(texto, "reset") {
					if err := c.DB.SetSetting(c.Ctx, c.Chat.String(), claveTexto, ""); err != nil {
						return err
					}
					return c.Reply(fmt.Sprintf("ꕥ Texto de %s restablecido al de fábrica.", etiqueta))
				}
				if len([]rune(texto)) > MaxPlantilla {
					return c.Reply(fmt.Sprintf("《✧》 El texto no puede pasar de %d caracteres.", MaxPlantilla))
				}
				if err := c.DB.SetSetting(c.Ctx, c.Chat.String(), claveTexto, texto); err != nil {
					return err
				}
				// Guardar el texto no activa el aviso: se hace con .welcome on.
				on, err := c.DB.Flag(c.Ctx, c.Chat.String(), clave, false)
				if err != nil {
					return err
				}
				aviso := ""
				if !on {
					aviso = fmt.Sprintf("\n> ⚠ %s está desactivada: actívala con *%s%s on*", etiqueta,
						c.Bot.FirstPrefix(), strings.TrimPrefix(nombre, "set"))
				}
				return c.Reply(fmt.Sprintf("✐ Texto de %s guardado:%s\n%s", etiqueta, aviso, texto))
			},
		})
	}
	plantilla("setwelcome", store.KeyWelcome, store.KeySetWelcome, "bienvenida", bienvenidaPorDefecto)
	plantilla("setgoodbye", store.KeyGoodbye, store.KeySetGoodbye, "despedida", despedidaPorDefecto)
}

// onGroupChange atiende las entradas y salidas del grupo: manda la bienvenida
// o la despedida si están activadas.
//
// Un matiz que en el JS no se tenía en cuenta: los eventos de grupo llegan
// también cuando alguien cambia de nombre o de foto. Aquí solo se mira
// Join/Leave, y si el mismo usuario aparece en los dos (entrar y salir en la
// misma operación) se manda solo uno, para no escribir dos mensajes seguidos.
func onGroupChange(ctx context.Context, db *store.Store, log *xlog.Logger, evt *events.GroupInfo) {
	if evt == nil || evt.JID.IsEmpty() {
		return
	}

	if len(evt.Join) == 0 && len(evt.Leave) == 0 {
		return
	}

	nombreGrupo := "el grupo"
	miembros := 0
	if info, err := groupInfoDe(ctx, db, evt); err == nil && info != nil {
		if info.Name != "" {
			nombreGrupo = info.Name
		}
		miembros = len(info.Participants)
	}

	if len(evt.Join) > 0 {
		avisar(ctx, db, log, evt, evt.Join, store.KeyWelcome, store.KeySetWelcome, true, nombreGrupo, miembros)
	}
	if len(evt.Leave) > 0 {
		avisar(ctx, db, log, evt, evt.Leave, store.KeyGoodbye, store.KeySetGoodbye, false, nombreGrupo, miembros)
	}
}

// ─── envío de los avisos ────────────────────────────────────────────────────

// groupInfoDe lee el grupo desde el router (el oyente de eventos no tiene un
// Context de mensaje del que tirar).
func groupInfoDe(ctx context.Context, db *store.Store, evt *events.GroupInfo) (*types.GroupInfo, error) {
	r := bot.ActiveRouter()
	if r == nil {
		return nil, fmt.Errorf("sin router")
	}
	return r.Groups().Info(ctx, evt.JID)
}

// avisar manda la bienvenida (o la despedida) de cada persona, si el ajuste
// está activo. Se manda de una en una, con la cola anti-ban de por medio: en
// un grupo grande entran varios a la vez y una ráfaga de avisos es una firma
// de bot.
func avisar(ctx context.Context, db *store.Store, log *xlog.Logger, evt *events.GroupInfo,
	quienes []types.JID, claveFlag, claveTexto string, esBienvenida bool,
	nombreGrupo string, miembros int) {

	chatID := evt.JID.String()

	on, err := db.Flag(ctx, chatID, claveFlag, false)
	if err != nil {
		if log != nil {
			log.Warn("no se pudo leer el ajuste del aviso: " + err.Error())
		}
		return
	}
	if !on {
		return // desactivado: silencio, como debe ser
	}

	defecto := bienvenidaPorDefecto
	if !esBienvenida {
		defecto = despedidaPorDefecto
	}
	plantilla, err := db.Text(ctx, chatID, claveTexto, "")
	if err != nil || strings.TrimSpace(plantilla) == "" {
		plantilla = defecto
	}

	r := bot.ActiveRouter()
	if r == nil {
		return
	}

	max := len(quienes)
	if max > 5 {
		max = 5 // un solo evento no debe generar 40 mensajes
	}
	for i := 0; i < max; i++ {
		quien := quienes[i]
		if quien.IsEmpty() {
			continue
		}
		mencion := "@" + quien.User
		texto := textoPlantilla(plantilla, nombreGrupo, miembros, mencion)
		if err := r.Cliente().SendWelcome(ctx, evt.JID, texto, quien); err != nil {
			if log != nil {
				log.Warn("no se pudo mandar el aviso de grupo: " + err.Error())
			}
			return
		}
	}
}
