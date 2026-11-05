// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// router.go — Puerto de src/commands/router.js:
//
//	mensaje → filtros duros → identidad (PN/LID) → hooks → self-mode
//	        → prefijo/botón → cooldown → permisos → handler
//
// Cooldown por usuario (10 req/min como en el JS) y recuperación de pánico:
// un handler que reviente NO tumba el bot (equivalente al anti-crash).
package bot

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"shin-go/internal/config"
	"shin-go/internal/store"
	"shin-go/internal/wa"
	"shin-go/internal/xlog"
)

type Router struct {
	cfg    config.Config
	log    *xlog.Logger
	client *wa.Client
	db     *store.Store
	owner  []string // números de owner ya normalizados (solo dígitos)
	groups *GroupCache

	mu       sync.Mutex
	cooldown map[string][]time.Time // jid → instantes de uso (ventana 1 min)

	modeMu   sync.Mutex
	mode     string // "public" | "self"
	modeRead time.Time
}

const (
	rateWindow = time.Minute
	rateMax    = 10
	// Cada cuántos usuarios nuevos se limpia el mapa de cooldowns. Sin esto el
	// mapa crece para siempre (un bot en grupos grandes acumula miles de JIDs).
	cooldownCleanupEvery = 256

	// Modo self: solo el owner puede usar comandos. Se guarda en settings.
	ModePublic = "public"
	ModeSelf   = "self"
)

const keyMode = "mode"

func NewRouter(cfg config.Config, log *xlog.Logger, client *wa.Client, db *store.Store, owners []string) *Router {
	r := &Router{
		cfg:      cfg,
		log:      log,
		client:   client,
		db:       db,
		owner:    NormalizeOwners(owners),
		groups:   NewGroupCache(client, client.IDs),
		cooldown: map[string][]time.Time{},
		mode:     ModePublic,
	}
	SetActiveRouter(r)
	return r
}

// El bot es un proceso con un solo router; los comandos que cambian ajustes
// globales (.self, .public) lo necesitan. Se expone aquí para no meter el
// router dentro del Context (que se construye por mensaje).
var activeRouter *Router

// SetActiveRouter registra el router del proceso (lo llama main / NewRouter).
func SetActiveRouter(r *Router) { activeRouter = r }

// ActiveRouter devuelve el router del proceso (nil si aún no hay).
func ActiveRouter() *Router { return activeRouter }

// Groups expone la caché de grupos (la usan los comandos de admin).
func (r *Router) Groups() *GroupCache { return r.groups }

// Handle es el punto de entrada (se registra con client.OnMessage).
func (r *Router) Handle(ctx context.Context, evt *events.Message) {
	defer func() {
		if rec := recover(); rec != nil {
			r.log.Err(fmt.Sprintf("pánico en handler: %v\n%s", rec, debug.Stack()))
		}
	}()

	if ctx == nil {
		ctx = context.Background()
	}

	info := evt.Info

	// ── filtros duros ────────────────────────────────────────────────────
	// Los mensajes que manda el propio número del bot no se procesan: si no,
	// cualquier texto con prefijo escrito desde ese mismo teléfono se
	// ejecutaría solo (y un menú que lista ".ping" acaba en bucle).
	if info.IsFromMe {
		return
	}
	// Estados y difusiones no son chats: contestar ahí es la vía rápida al ban.
	if info.Chat.Server == types.BroadcastServer || info.Chat.User == "status" {
		return
	}

	chat := info.Chat
	sender := info.Sender.ToNonAD()
	senderAlt := info.SenderAlt.ToNonAD()
	isGroup := chat.Server == types.GroupServer

	// ── identidad ────────────────────────────────────────────────────────
	// WhatsApp puede mandar al mismo usuario como teléfono o como LID. Se
	// resuelve UNA vez por mensaje y todos los comandos usan la misma llave.
	identity := r.client.IDs.Resolve(ctx, sender, senderAlt)

	parsed := wa.ParseMessage(evt.Message, r.cfg.Prefixes)

	isOwner := IsOwner(sender, senderAlt, r.owner)

	c := &Context{
		Ctx:       ctx,
		Bot:       r.client,
		DB:        r.db,
		Log:       r.log,
		Groups:    r.groups,
		Event:     evt,
		Chat:      chat,
		Sender:    sender,
		SenderAlt: senderAlt,
		SenderKey: identity.Key,
		Identity:  identity,
		IsGroup:   isGroup,
		IsOwner:   isOwner,
		PushName:  info.PushName,
		Text:      parsed.Text,
		Raw:       parsed.Text,
	}

	// ── hooks: ven TODOS los mensajes ────────────────────────────────────
	// Antilink, afk, antiflood… Si uno atiende el mensaje, aquí se acaba.
	if r.RunHooks(c, parsed) {
		return
	}

	if !parsed.IsButton && !parsed.IsCommand {
		return // no es comando y ningún hook lo quiso
	}

	var cmdName string
	var args []string
	if parsed.IsButton && parsed.ButtonID != "" {
		cmdName, args = wa.CommandFromButton(parsed.ButtonID)
		c.Command, c.Args = cmdName, args
		c.fromButton = true
	} else {
		cmdName, args = parsed.Command, parsed.Args
		c.Command, c.Args = cmdName, args
	}

	cmd, ok := Lookup(cmdName)
	if !ok {
		return
	}

	// ── modo self ────────────────────────────────────────────────────────
	// En self el bot solo obedece al owner (el resto de mensajes se ignoran en
	// silencio: contestar "no puedes" en cada grupo sería ruido).
	if !isOwner && r.selfMode(ctx) {
		return
	}

	// ── permisos ─────────────────────────────────────────────────────────
	if cmd.OwnerOnly && !isOwner {
		r.reply(ctx, chat, "✕ Solo el owner puede usar ese comando.")
		return
	}
	if cmd.GroupOnly && !isGroup {
		r.reply(ctx, chat, "✕ Ese comando es solo para grupos.")
		return
	}
	if cmd.AdminOnly && !isGroup {
		r.reply(ctx, chat, "✕ Ese comando es solo para grupos.")
		return
	}
	if (cmd.AdminOnly || cmd.BotAdminOnly) && isGroup && !isOwner {
		// El owner manda siempre; a los demás se les exige ser admin del grupo.
		if cmd.AdminOnly && !c.Admin() {
			r.reply(ctx, chat, "✕ Necesitas ser admin del grupo para eso.")
			return
		}
	}
	if cmd.BotAdminOnly && !c.BotAdmin() {
		r.reply(ctx, chat, "✕ Necesito ser admin del grupo para hacer eso.")
		return
	}

	// ── cooldown por usuario (10 req/min) ────────────────────────────────
	if !isOwner {
		if wait, limited := r.rateLimited(c.Key()); limited {
			r.log.Warn(fmt.Sprintf("rate-limit: %s (esperar %s)", sender, wait.Round(time.Second)))
			return
		}
	}

	c.CmdPriority = cmd.Priority

	start := time.Now()
	if err := cmd.Run(c); err != nil {
		if errors.Is(err, context.Canceled) {
			r.log.Info(fmt.Sprintf(".%s cancelado (cierre del bot)", cmd.Name))
			return
		}
		r.log.Err(fmt.Sprintf(".%s falló: %v", cmd.Name, err))
		return
	}
	r.log.Info(fmt.Sprintf(".%s ok en %dms", cmd.Name, time.Since(start).Milliseconds()))
}

// ─── modo self ──────────────────────────────────────────────────────────────

// SetMode cambia entre público y self (lo usa el comando .self / .public).
func (r *Router) SetMode(ctx context.Context, mode string) error {
	if mode != ModeSelf {
		mode = ModePublic
	}
	if err := r.db.SetSetting(ctx, store.SystemJID, keyMode, mode); err != nil {
		return err
	}
	r.modeMu.Lock()
	r.mode, r.modeRead = mode, time.Now()
	r.modeMu.Unlock()
	return nil
}

// selfMode lee el modo con una caché de 5 s: es una consulta por mensaje y no
// tiene sentido ir a la base en cada uno, pero tampoco quedarse con un valor
// viejo durante una hora.
func (r *Router) selfMode(ctx context.Context) bool {
	r.modeMu.Lock()
	if time.Since(r.modeRead) < 5*time.Second && !r.modeRead.IsZero() {
		mode := r.mode
		r.modeMu.Unlock()
		return mode == ModeSelf
	}
	r.modeMu.Unlock()

	mode, ok, err := r.db.GetSetting(ctx, store.SystemJID, keyMode)
	if err != nil {
		r.log.Warn("no se pudo leer el modo: " + err.Error())
		mode, ok = ModePublic, true
	}
	if !ok || mode == "" {
		mode = ModePublic
	}

	r.modeMu.Lock()
	r.mode, r.modeRead = mode, time.Now()
	r.modeMu.Unlock()
	return mode == ModeSelf
}

// reply manda un aviso del router sin tumbar el flujo si el envío falla.
func (r *Router) reply(ctx context.Context, chat types.JID, text string) {
	if err := r.client.SendText(ctx, chat, text, true); err != nil {
		r.log.Warn("no se pudo avisar al usuario: " + err.Error())
	}
}

// rateLimited: 10 peticiones por minuto y por usuario, como cooldown.js.
func (r *Router) rateLimited(key string) (time.Duration, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	kept := r.cooldown[key][:0]
	for _, t := range r.cooldown[key] {
		if now.Sub(t) < rateWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) >= rateMax {
		r.cooldown[key] = kept
		return rateWindow - now.Sub(kept[0]), true
	}
	r.cooldown[key] = append(kept, now)

	if len(r.cooldown) > cooldownCleanupEvery {
		r.pruneLocked(now)
	}
	return 0, false
}

// pruneLocked borra del mapa a quien ya no tiene marcas dentro de la ventana.
func (r *Router) pruneLocked(now time.Time) {
	for key, stamps := range r.cooldown {
		if len(stamps) == 0 || now.Sub(stamps[len(stamps)-1]) >= rateWindow {
			delete(r.cooldown, key)
		}
	}
}

// isOwner se conserva para los comandos que ya lo llamaban; delega en la
// comparación nueva (teléfono + LID).
func (r *Router) isOwner(jid types.JID) bool {
	return IsOwner(jid.ToNonAD(), types.JID{}, r.owner)
}
