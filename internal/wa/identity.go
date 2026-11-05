// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// identity.go — Resolver: quién es quién cuando WhatsApp manda dos JID para la
// misma persona (teléfono y LID).
//
// Reglas que aplica (y por qué):
//
//  1. La llave de la base del bot es el **teléfono** siempre que se conozca.
//     Así un usuario no tiene dos cuentas de monedas según cómo llegue el
//     mensaje, y los datos del bot de JS (que siempre guardó teléfonos) siguen
//     cuadrando.
//  2. La pareja se aprende gratis del propio mensaje: whatsmeow rellena
//     `SenderAlt` con el JID alternativo, así que en la mayoría de los casos
//     no hay que preguntar a nadie.
//  3. Si el mensaje viene solo como LID y no hay alternativo, se consulta el
//     mapeo de la librería y luego la base del bot. Si aun así no aparece, la
//     llave es el LID: **se prefiere una cuenta con llave fea a perder el
//     mensaje** (y en cuanto se aprenda la pareja, las dos llaves apuntan al
//     mismo sitio).
//  4. Nunca se hace una consulta de red para resolver un LID: WhatsApp no
//     expone LID → teléfono. Solo se resuelve lo que ya se vio antes.
package wa

import (
	"context"
	"errors"
	"strings"
	"sync"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/xlog"
)

// errNoMapping: no se conoce la pareja (todavía).
var errNoMapping = errors.New("sin mapeo LID↔teléfono")

// Identity es la misma persona vista por los dos lados.
type Identity struct {
	PN  types.JID // teléfono (puede estar vacío si aún no se conoce)
	LID types.JID // identificador interno (puede estar vacío)

	// Key es la llave estable para la base de datos: el teléfono si se conoce,
	// si no el LID. Nunca vacía cuando hay JID de entrada.
	Key string
}

// Known indica si se conoce el teléfono (es decir, si la llave es bonita).
func (i Identity) Known() bool { return !i.PN.IsEmpty() && i.Key == i.PN.String() }

// Resolver traduce JID ↔ identidad. Es seguro para usar desde varias
// goroutines y guarda una caché pequeña en memoria.
type Resolver struct {
	store IdentityStore
	log   *xlog.Logger

	mu    sync.RWMutex
	cache map[string]Identity // jid completo → identidad
}

const resolverCacheMax = 2048

// NewResolver construye el resolver sobre la base del bot.
func NewResolver(mem IdentityStore, log *xlog.Logger) *Resolver {
	return &Resolver{store: mem, log: log, cache: make(map[string]Identity, resolverCacheMax)}
}

// IsLID dice si el JID es un identificador interno de WhatsApp.
func IsLID(jid types.JID) bool {
	return jid.Server == types.HiddenUserServer || jid.Server == types.HostedLIDServer
}

// Resolve devuelve la identidad de un remitente. `alt` es el JID alternativo
// que manda WhatsApp (Info.SenderAlt / ChatAlt) y puede venir vacío.
func (r *Resolver) Resolve(ctx context.Context, jid, alt types.JID) Identity {
	jid = jid.ToNonAD()
	alt = alt.ToNonAD()

	if jid.IsEmpty() {
		return Identity{}
	}

	if id, ok := r.fromCache(jid, alt); ok {
		return id
	}

	var id Identity
	switch {
	case IsLID(jid):
		id.LID = jid
		// El alternativo del propio mensaje es la fuente más barata y fiable.
		if isPN(alt) {
			id.PN = alt
		} else {
			id.PN = r.pnForLID(ctx, jid)
		}
	default:
		id.PN = jid
		if IsLID(alt) {
			id.LID = alt
		} else {
			id.LID = r.lidForPN(ctx, jid)
		}
	}

	id.Key = keyFor(id)
	r.learn(ctx, id)
	r.remember(jid, alt, id)
	return id
}

// PNFor devuelve el teléfono de un LID (vacío si no se conoce).
func (r *Resolver) PNFor(ctx context.Context, lid types.JID) types.JID {
	if !IsLID(lid.ToNonAD()) {
		return lid.ToNonAD()
	}
	return r.Resolve(ctx, lid, types.EmptyJID).PN
}

// LIDFor devuelve el LID de un teléfono (vacío si la red no lo da).
func (r *Resolver) LIDFor(ctx context.Context, pn types.JID) types.JID {
	if IsLID(pn.ToNonAD()) {
		return pn.ToNonAD()
	}
	return r.Resolve(ctx, pn, types.EmptyJID).LID
}

// Key devuelve la llave estable de un JID sin resolverlo (lo que ya se sabe).
func (r *Resolver) Key(ctx context.Context, jid types.JID) string {
	return r.Resolve(ctx, jid, types.EmptyJID).Key
}

// SamePerson compara dos remitentes por sus dos JID. Sirve para comprobar si
// el que escribe es el mismo de una lista de participantes del grupo, aunque
// uno venga por LID y el otro por teléfono.
func (r *Resolver) SamePerson(ctx context.Context, a, aAlt, b, bAlt types.JID) bool {
	ia := r.Resolve(ctx, a, aAlt)
	ib := r.Resolve(ctx, b, bAlt)

	if ia.Key != "" && ia.Key == ib.Key {
		return true
	}
	if !ia.PN.IsEmpty() && ia.PN.User == ib.PN.User && !ib.PN.IsEmpty() {
		return true
	}
	if !ia.LID.IsEmpty() && ia.LID.User == ib.LID.User && !ib.LID.IsEmpty() {
		return true
	}
	return false
}

// ─── resolución ─────────────────────────────────────────────────────────────

func (r *Resolver) pnForLID(ctx context.Context, lid types.JID) types.JID {
	if r.store == nil {
		return types.EmptyJID
	}
	pn, err := r.store.PNForLID(ctx, lid.String())
	if err != nil {
		r.warn("leer LID→PN", err)
		return types.EmptyJID
	}
	if pn == "" {
		return types.EmptyJID
	}
	jid, err := types.ParseJID(pn)
	if err != nil {
		return types.EmptyJID
	}
	return jid.ToNonAD()
}

func (r *Resolver) lidForPN(ctx context.Context, pn types.JID) types.JID {
	if r.store == nil {
		return types.EmptyJID
	}
	lid, err := r.store.LIDForPN(ctx, pn.String())
	if err != nil {
		r.warn("leer PN→LID", err)
		return types.EmptyJID
	}
	if lid == "" {
		return types.EmptyJID
	}
	jid, err := types.ParseJID(lid)
	if err != nil {
		return types.EmptyJID
	}
	return jid.ToNonAD()
}

// learn guarda la pareja en cuanto se conocen los dos lados. Es lo que hace
// que la siguiente vez no haya que preguntar nada.
func (r *Resolver) learn(ctx context.Context, id Identity) {
	if r.store == nil || id.PN.IsEmpty() || id.LID.IsEmpty() {
		return
	}
	if err := r.store.PutIdentity(ctx, id.LID.String(), id.PN.String()); err != nil {
		r.warn("guardar pareja LID↔PN", err)
	}
}

// ─── caché ──────────────────────────────────────────────────────────────────

func (r *Resolver) fromCache(jid, alt types.JID) (Identity, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := []string{jid.String()}
	if !alt.IsEmpty() {
		keys = append(keys, alt.String())
	}
	for _, k := range keys {
		if id, ok := r.cache[k]; ok {
			if id.PN.IsEmpty() && !alt.IsEmpty() && isPN(alt) {
				continue // la caché tenía la identidad a medias: vale la pena reintentar
			}
			return id, true
		}
	}
	return Identity{}, false
}

func (r *Resolver) remember(jid, alt types.JID, id Identity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.cache) >= resolverCacheMax {
		r.cache = make(map[string]Identity, resolverCacheMax)
	}
	r.cache[jid.String()] = id
	if !alt.IsEmpty() {
		// El alternativo se guarda apuntando a la MISMA identidad, para que
		// preguntar por cualquiera de los dos cueste cero.
		swap := id
		if !swap.PN.IsEmpty() {
			swap.PN, swap.LID = id.PN, id.LID
		}
		r.cache[alt.String()] = swap
	}
}

// ─── utilidades ─────────────────────────────────────────────────────────────

func isPN(jid types.JID) bool {
	return jid.Server == types.DefaultUserServer && jid.User != ""
}

// keyFor decide la llave de la base: teléfono si se conoce, si no LID.
func keyFor(id Identity) string {
	switch {
	case !id.PN.IsEmpty() && isPN(id.PN):
		return id.PN.String()
	case !id.LID.IsEmpty():
		return id.LID.String()
	default:
		return ""
	}
}

func (r *Resolver) warn(msg string, err error) {
	if r.log != nil {
		r.log.Warn(msg + ": " + err.Error())
	}
}

// Explain devuelve una línea legible para .ping / depuración: qué se sabe de
// un remitente y de dónde salió el dato.
func (i Identity) Explain() string {
	var sb strings.Builder
	sb.WriteString("llave=" + i.Key)
	if !i.PN.IsEmpty() {
		sb.WriteString(" · PN=" + i.PN.User)
	} else {
		sb.WriteString(" · PN=desconocido")
	}
	if !i.LID.IsEmpty() {
		sb.WriteString(" · LID=" + i.LID.User)
	}
	return sb.String()
}
