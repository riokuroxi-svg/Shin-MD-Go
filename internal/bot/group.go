// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// group.go — Información de grupo con caché, y la pregunta que más se rompe
// con LID: "¿este usuario es admin?"
//
// El fallo clásico: los participantes del grupo llegan con un JID (a veces
// LID, a veces teléfono) y el remitente del mensaje puede venir con el otro.
// Comparar cadenas a pelo daba "no eres admin" a los admins reales y dejaba
// pasar a los que no lo eran cuando el grupo usaba LID. Aquí se comparan las
// TRES formas (JID, PhoneNumber, LID) y también la identidad resuelta.
package bot

import (
	"context"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/wa"
)

// groupTTL: cuánto vale la información de un grupo antes de volver a pedirla.
// WhatsApp la manda por eventos, pero entre eventos un grupo puede cambiar de
// admins; un minuto es el equilibrio entre frescura y no martillar la red.
const groupTTL = time.Minute

type groupEntry struct {
	info *types.GroupInfo
	at   time.Time
}

// GroupCache guarda la info de los grupos con caducidad.
type GroupCache struct {
	client *wa.Client
	ids    *wa.Resolver
	ttl    time.Duration

	mu      sync.Mutex
	entries map[string]groupEntry
}

func NewGroupCache(client *wa.Client, ids *wa.Resolver) *GroupCache {
	return &GroupCache{
		client:  client,
		ids:     ids,
		ttl:     groupTTL,
		entries: make(map[string]groupEntry),
	}
}

// Info devuelve la información del grupo (de caché si está fresca).
func (g *GroupCache) Info(ctx context.Context, jid types.JID) (*types.GroupInfo, error) {
	if g == nil || g.client == nil || jid.IsEmpty() {
		return nil, ErrNotAllowed
	}

	key := jid.String()

	g.mu.Lock()
	if e, ok := g.entries[key]; ok && time.Since(e.at) < g.ttl && e.info != nil {
		g.mu.Unlock()
		return e.info, nil
	}
	g.mu.Unlock()

	info, err := g.client.WA.GetGroupInfo(ctx, jid)
	if err != nil {
		// Si la red falla pero hay una copia vieja, es mejor usarla que decir
		// que el grupo no existe (un corte de red no debe romper los permisos).
		g.mu.Lock()
		old := g.entries[key].info
		g.mu.Unlock()
		if old != nil {
			return old, nil
		}
		return nil, err
	}

	g.mu.Lock()
	if len(g.entries) > 512 { // un bot en muchos grupos no debe crecer sin fin
		g.entries = make(map[string]groupEntry, 512)
	}
	g.entries[key] = groupEntry{info: info, at: time.Now()}
	g.mu.Unlock()
	return info, nil
}

// Invalidate borra la copia de un grupo (tras un kick, promote, etc.).
func (g *GroupCache) Invalidate(jid types.JID) {
	if g == nil {
		return
	}
	g.mu.Lock()
	delete(g.entries, jid.String())
	g.mu.Unlock()
}

// IsAdmin: ¿el usuario es admin (o creador) del grupo?
func (g *GroupCache) IsAdmin(ctx context.Context, group, user, alt types.JID) bool {
	info, err := g.Info(ctx, group)
	if err != nil {
		return false
	}
	return participantIsAdmin(ctx, g.ids, info, user, alt)
}

// IsBotAdmin: ¿el propio bot es admin? Lo necesitan antilink (para borrar) y
// los comandos de administración.
func (g *GroupCache) IsBotAdmin(ctx context.Context, group types.JID) bool {
	if g == nil || g.client == nil {
		return false
	}
	info, err := g.Info(ctx, group)
	if err != nil {
		return false
	}

	own := types.EmptyJID
	if id := g.client.WA.Store.ID; id != nil {
		own = id.ToNonAD()
	}
	return participantIsAdmin(ctx, g.ids, info, own, g.client.OwnLID())
}

// participantIsAdmin compara el usuario contra cada participante por todas
// las formas posibles: el JID del mensaje, su alternativo, el teléfono y el
// LID guardados en la identidad.
func participantIsAdmin(ctx context.Context, ids *wa.Resolver, info *types.GroupInfo, user, alt types.JID) bool {
	if info == nil || user.IsEmpty() {
		return false
	}

	// Si el grupo está en modo LID, los participantes vienen con LID.
	want := make(map[string]bool, 4)
	add := func(j types.JID) {
		if !j.IsEmpty() && j.User != "" {
			want[j.User] = true
		}
	}
	add(user)
	add(alt)
	if ids != nil {
		id := ids.Resolve(ctx, user, alt)
		add(id.PN)
		add(id.LID)
	}

	for _, p := range info.Participants {
		if !(p.IsAdmin || p.IsSuperAdmin) {
			continue
		}
		for _, candidate := range []types.JID{p.JID, p.PhoneNumber, p.LID} {
			if !candidate.IsEmpty() && want[candidate.User] {
				return true
			}
		}
	}
	return false
}
