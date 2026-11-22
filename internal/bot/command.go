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

	"shin-go/internal/interactivos"
	"shin-go/internal/store"
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
// Messenger es el contrato mínimo de respuestas que usa el núcleo. La
// implementación WhatsApp traduce estos valores a botones/protobuf y gestiona
// los fallbacks; los comandos no construyen payloads externos.
type Messenger interface {
	SendText(context.Context, wa.JID, string, bool) error
	SendMenu(context.Context, wa.JID, string, string, string, bool, ...Button) error
	SendButtons(context.Context, wa.JID, string, string, string, bool, ...Button) error
	SendButtonsImage(context.Context, wa.JID, []byte, string, string, string, bool, ...Button) error
	SendButtonsImageWithID(context.Context, wa.JID, string, []byte, string, string, string, bool, ...Button) error
	SendList(context.Context, wa.JID, string, string, string, string, bool, ...ListSection) error
	SendCarousel(context.Context, wa.JID, string, bool, ...Card) error
}

type Context struct {
	Ctx      context.Context
	Bot      *wa.Client
	Outbox   Messenger
	Log      *xlog.Logger
	Event    *wa.IncomingMessage
	DB       *store.Store
	Groups   *GroupCache
	Chat     wa.JID
	Sender   wa.JID
	IsGroup  bool
	IsOwner  bool
	PushName string

	// Identidad del remitente. WhatsApp puede mandar al mismo usuario como
	// teléfono o como LID: `SenderKey` es la llave estable para la base (el
	// teléfono si se conoce) y `SenderAlt` el otro JID, si vino.
	Identity  wa.Identity
	SenderKey string
	SenderAlt wa.JID

	Command string
	Args    []string
	Text    string // argumentos completos después del nombre del comando
	Raw     string // texto recibido, ya limpio de controles invisibles

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
func (c *Context) Mentioned() []wa.JID {
	if c.Event == nil || len(c.Event.Mentions) == 0 {
		return nil
	}
	return append([]wa.JID(nil), c.Event.Mentions...)
}

// Reply contesta en el mismo chat (pasando por la cola).
func (c *Context) Reply(text string) error {
	return c.outbox().SendText(c.Ctx, c.Chat, text, c.CmdPriority)
}

func (c *Context) outbox() Messenger {
	if c.Outbox != nil {
		return c.Outbox
	}
	return c.Bot
}

// ReplyPriority contesta saltando el cupo de calentamiento (.menu, .ping…).
func (c *Context) ReplyPriority(text string) error {
	return c.outbox().SendText(c.Ctx, c.Chat, text, true)
}

// ReplyMenu, ReplyButtons, ReplyList y ReplyCarousel son el ÚNICO puerto que
// usan los comandos para interactivos. La cadena concreta vive fuera de cmds/.
func (c *Context) ReplyMenu(title, body, footer string, buttons ...Button) error {
	return c.outbox().SendMenu(c.Ctx, c.Chat, title, body, footer, c.CmdPriority, buttons...)
}

func (c *Context) ReplyButtons(title, body, footer string, buttons ...Button) error {
	return c.outbox().SendButtons(c.Ctx, c.Chat, title, body, footer, c.CmdPriority, buttons...)
}

func (c *Context) ReplyImage(image []byte, title, body, footer string, buttons ...Button) error {
	return c.outbox().SendButtonsImage(c.Ctx, c.Chat, image, title, body, footer, c.CmdPriority, buttons...)
}

func (c *Context) ReplyImageWithID(messageID string, image []byte, title, body, footer string, buttons ...Button) error {
	return c.outbox().SendButtonsImageWithID(c.Ctx, c.Chat, messageID, image, title, body, footer, c.CmdPriority, buttons...)
}

func (c *Context) ReplyList(title, body, buttonText, footer string, sections ...ListSection) error {
	return c.outbox().SendList(c.Ctx, c.Chat, title, body, buttonText, footer, c.CmdPriority, sections...)
}

func (c *Context) ReplyCarousel(body string, cards ...Card) error {
	return c.outbox().SendCarousel(c.Ctx, c.Chat, body, c.CmdPriority, cards...)
}

// Alias para no obligar a importar internal/interactivos en cada comando.
type Button = interactivos.Button
type ListRow = interactivos.ListRow
type ListSection = interactivos.ListSection
type Card = interactivos.Card

// Atajos de botones para los handlers (mismo nombre que en JS: quickReply…).
var (
	QuickReply   = interactivos.QuickReply
	CTAURL       = interactivos.CTAURL
	CTACopy      = interactivos.CTACopy
	CTACall      = interactivos.CTACall
	OpenWebview  = interactivos.OpenWebview
	SendLocation = interactivos.SendLocation
	SingleSelect = interactivos.SingleSelect
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
