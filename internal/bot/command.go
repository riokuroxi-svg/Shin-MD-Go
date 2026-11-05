// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// command.go — Registro de comandos. En el Shin-MD de JS cada comando es un
// archivo que se carga con import() dinámico; aquí se registra en un mapa al
// arrancar: nada de leer el disco en caliente, nada de rutas roscadas.
package bot

import (
	"context"
	"errors"
	"sort"
	"strings"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"shin-go/internal/store"
	"shin-go/internal/ui"
	"shin-go/internal/wa"
	"shin-go/internal/xlog"
)

type Category string

const (
	CatMain       Category = "main"
	CatEconomy    Category = "economia"
	CatDownload   Category = "descargas"
	CatGroup      Category = "grupo"
	CatStickers   Category = "stickers"
	CatGacha      Category = "gacha"
	CatOwner      Category = "owner"
	CatUtils      Category = "utilidades"
	CatNSFW       Category = "nsfw"
	CatGames      Category = "juegos"
	CatPerfil     Category = "perfil"
	CatAnime      Category = "anime"
	CatMedia      Category = "media"
	CatSocket     Category = "socket"
	CatEventsCode Category = "eventos"
)

// Context es lo que recibe un handler: equivale al objeto ctx/m del JS
// (msg, args, sock, isGroup, isOwner, reply…) pero tipado.
type Context struct {
	Ctx      context.Context
	Bot      *wa.Client
	Log      *xlog.Logger
	Event    *events.Message
	DB       *store.Store
	Groups   *GroupCache
	Chat     types.JID
	Sender   types.JID
	IsGroup  bool
	IsOwner  bool
	PushName string

	// Identidad del remitente. WhatsApp puede mandar al mismo usuario como
	// teléfono o como LID: `SenderKey` es la llave estable para la base (el
	// teléfono si se conoce) y `SenderAlt` el otro JID, si vino.
	Identity  wa.Identity
	SenderKey string
	SenderAlt types.JID

	Command string
	Args    []string
	Text    string // texto completo sin el prefijo
	Raw     string // texto tal cual llegó

	// CmdPriority es la prioridad del comando que se está ejecutando. La pone
	// el router a partir de Command.Priority y la respetan TODOS los envíos
	// del contexto: así .menu no gasta cupo del calentamiento y .daily sí.
	CmdPriority bool

	fromButton bool
}

// Priority dice si el comando que corre es prioritario.
func (c *Context) Priority() bool { return c.CmdPriority }

// Key es la llave del usuario para la base de datos: teléfono si se conoce,
// LID si no. Todos los comandos deberían guardar datos con esto y no con
// c.Sender.String(), o el mismo usuario acabaría con dos cuentas.
func (c *Context) Key() string {
	if c.SenderKey != "" {
		return c.SenderKey
	}
	return c.Sender.String()
}

// Admin: ¿el remitente es admin del grupo? (solo tiene sentido en grupos)
func (c *Context) Admin() bool {
	if !c.IsGroup || c.Groups == nil {
		return false
	}
	return c.Groups.IsAdmin(c.Ctx, c.Chat, c.Sender, c.SenderAlt)
}

// BotAdmin: ¿el bot es admin del grupo? Lo necesitan los comandos que borran,
// expulsan o cambian el grupo.
func (c *Context) BotAdmin() bool {
	if !c.IsGroup || c.Groups == nil {
		return false
	}
	return c.Groups.IsBotAdmin(c.Ctx, c.Chat)
}

// Mentioned devuelve los JID mencionados en el mensaje (@número o respuesta a
// alguien). Los usa el afk y los comandos de admin.
func (c *Context) Mentioned() []types.JID {
	msg := wa.Unwrap(c.Event.Message)
	if msg == nil {
		return nil
	}

	var out []types.JID
	if ci := wa.ContextInfo(msg); ci != nil {
		for _, raw := range ci.GetMentionedJID() {
			if jid, err := types.ParseJID(raw); err == nil {
				out = append(out, jid.ToNonAD())
			}
		}
		if p := ci.GetParticipant(); p != "" {
			if jid, err := types.ParseJID(p); err == nil {
				out = append(out, jid.ToNonAD())
			}
		}
	}
	return out
}

// Reply contesta en el mismo chat (pasando por la cola).
func (c *Context) Reply(text string) error {
	return c.Bot.SendText(c.Ctx, c.Chat, text, c.CmdPriority)
}

// ReplyPriority contesta saltando el cupo de calentamiento (.menu, .ping…).
func (c *Context) ReplyPriority(text string) error {
	return c.Bot.SendText(c.Ctx, c.Chat, text, true)
}

// ReplyButtons contesta con tarjeta y botones nativos.
func (c *Context) ReplyButtons(title, body, footer string, buttons ...Button) error {
	return c.Bot.SendButtons(c.Ctx, c.Chat, title, body, footer, c.CmdPriority, buttons...)
}

// ReplyImage contesta con una tarjeta que lleva imagen de cabecera.
func (c *Context) ReplyImage(image []byte, title, body, footer string, buttons ...Button) error {
	return c.Bot.SendButtonsImage(c.Ctx, c.Chat, image, title, body, footer, c.CmdPriority, buttons...)
}

// Button es alias para no obligar a importar internal/ui en cada comando.
type Button = ui.Button

// Atajos de botones para los handlers (mismo nombre que en JS: quickReply…).
var (
	QuickReply   = ui.QuickReply
	CTAURL       = ui.CTAURL
	CTACopy      = ui.CTACopy
	CTACall      = ui.CTACall
	OpenWebview  = ui.OpenWebview
	SendLocation = ui.SendLocation
	SingleSelect = ui.SingleSelect
)

// FromButton indica si la invocación vino de tocar un botón.
func (c *Context) FromButton() bool { return c.fromButton }

// Command es la definición de un comando.
type Command struct {
	Name     string
	Aliases  []string
	Category Category
	Desc     string

	OwnerOnly    bool
	GroupOnly    bool
	AdminOnly    bool // el que escribe tiene que ser admin del grupo
	BotAdminOnly bool // el bot tiene que ser admin (para borrar, expulsar…)
	Priority     bool // salta el cupo diario del calentamiento
	Cooldown     int  // segundos; 0 = usar el del router

	Run func(c *Context) error
}

var (
	registry   = map[string]*Command{}
	categories = map[Category][]*Command{}
)

// Register añade un comando al registro global. Se llama desde los init()
// de la carpeta cmds/ — el equivalente a "el archivo existe, luego carga".
func Register(cmd Command) {
	if cmd.Run == nil || cmd.Name == "" {
		panic("bot.Register: comando sin nombre o sin Run")
	}
	if cmd.Cooldown == 0 {
		cmd.Cooldown = 3
	}
	c := &cmd
	registry[strings.ToLower(cmd.Name)] = c
	for _, a := range cmd.Aliases {
		registry[strings.ToLower(a)] = c
	}
	categories[cmd.Category] = append(categories[cmd.Category], c)
}

// All devuelve los comandos únicos (sin alias) ordenados por categoría.
func All() []*Command {
	seen := map[*Command]bool{}
	var out []*Command
	for _, c := range registry {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Category == out[j].Category {
			return out[i].Name < out[j].Name
		}
		return out[i].Category < out[j].Category
	})
	return out
}

// ByCategory agrupa para el .menu.
func ByCategory() map[Category][]*Command {
	out := map[Category][]*Command{}
	for cat, cmds := range categories {
		seen := map[*Command]bool{}
		for _, c := range cmds {
			if !seen[c] {
				seen[c] = true
				out[cat] = append(out[cat], c)
			}
		}
		sort.Slice(out[cat], func(i, j int) bool { return out[cat][i].Name < out[cat][j].Name })
	}
	return out
}

func Lookup(name string) (*Command, bool) {
	c, ok := registry[strings.ToLower(name)]
	return c, ok
}

func Count() (unique int, withAliases int) {
	unique = len(All())
	withAliases = len(registry)
	return
}

var ErrNotAllowed = errors.New("no permitido")
