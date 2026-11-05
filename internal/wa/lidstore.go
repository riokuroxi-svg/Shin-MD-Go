// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// lidstore.go — Puente entre el mapeo LID ↔ teléfono de whatsmeow y la base
// del bot.
//
// El problema de fondo, que en Baileys es el dolor de cabeza de estos meses:
// WhatsApp entregó el sistema LID y ahora un mismo usuario puede llegar como
//
//	5215512345678@s.whatsapp.net   → el teléfono (PN)
//	123456789012345@lid            → un identificador interno (LID)
//
// y **la red solo resuelve en un sentido: teléfono → LID**. Para un LID que
// nunca has visto no se puede preguntar por el teléfono: no existe esa
// consulta. Lo único que funciona es ir guardando las parejas que aparecen en
// el tráfico entrante (mensajes y participantes de grupo).
//
// whatsmeow ya trae el store (store.LIDStore) y lo llena solo desde los
// mensajes, el historial y el sync de migración. Lo que faltaba es que esas
// parejas **sobrevivan** a la sesión: si el bot se reconecta o se cambia de
// dispositivo, la librería vuelve a llenar su tabla, pero el bot puede usar
// las suyas al instante. Este envoltorio implementa la misma interfaz y añade
// esa memoria permanente en la base del bot (tabla `identities`).
package wa

import (
	"context"
	"sync"

	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/xlog"
)

// lidStore implementa store.LIDStore: todo lo que la librería pide se atiende
// primero con el store de whatsmeow y, si ahí no está, con la base del bot.
type lidStore struct {
	inner store.LIDStore
	mem   IdentityStore
	log   *xlog.Logger

	mu    sync.RWMutex
	cache map[string]string // lid → pn ("" = sabemos que no hay)
}

// IdentityStore es lo mínimo que necesitamos de la base (así no hay import
// circular y las pruebas pueden usar un doble en memoria).
type IdentityStore interface {
	PutIdentity(ctx context.Context, lid, pn string) error
	PNForLID(ctx context.Context, lid string) (string, error)
	LIDForPN(ctx context.Context, pn string) (string, error)
}

const lidCacheMax = 4096

// newLIDStore envuelve el store de la librería. `inner` puede ser nil (por
// ejemplo en pruebas) y entonces solo se usa la base del bot.
func newLIDStore(inner store.LIDStore, mem IdentityStore, log *xlog.Logger) *lidStore {
	return &lidStore{
		inner: inner,
		mem:   mem,
		log:   log,
		cache: make(map[string]string, lidCacheMax),
	}
}

// ─── Interfaz store.LIDStore ────────────────────────────────────────────────

func (s *lidStore) PutManyLIDMappings(ctx context.Context, mappings []store.LIDMapping) error {
	// Primero la base: es lo que de verdad no se puede perder.
	if s.mem != nil {
		for _, m := range mappings {
			if err := s.mem.PutIdentity(ctx, m.LID.String(), m.PN.String()); err != nil {
				s.warn("no se pudo guardar la pareja LID→PN", err)
			}
			s.remember(m.LID.User, m.PN.User)
		}
	}
	if s.inner == nil {
		return nil
	}
	return s.inner.PutManyLIDMappings(ctx, mappings)
}

func (s *lidStore) PutLIDMapping(ctx context.Context, lid, jid types.JID) error {
	if s.mem != nil {
		if err := s.mem.PutIdentity(ctx, lid.String(), jid.String()); err != nil {
			s.warn("no se pudo guardar la pareja LID→PN", err)
		}
	}
	s.remember(lid.User, jid.User)
	if s.inner == nil {
		return nil
	}
	return s.inner.PutLIDMapping(ctx, lid, jid)
}

func (s *lidStore) GetPNForLID(ctx context.Context, lid types.JID) (types.JID, error) {
	if pn, ok := s.lookup(lid.User); ok {
		return jidFrom(pn), nil
	}

	if s.inner != nil {
		if pn, err := s.inner.GetPNForLID(ctx, lid); err == nil && !pn.IsEmpty() {
			s.remember(lid.User, pn.User)
			return pn, nil
		}
	}

	// Respaldo: la pareja pudo guardarse en una sesión anterior.
	if s.mem != nil {
		pn, err := s.mem.PNForLID(ctx, lid.String())
		if err != nil {
			s.warn("no se pudo leer la pareja LID→PN", err)
		}
		if pn != "" {
			if jid, perr := types.ParseJID(pn); perr == nil {
				s.remember(lid.User, jid.User)
				return jid, nil
			}
		}
	}
	return types.EmptyJID, errNoMapping
}

func (s *lidStore) GetLIDForPN(ctx context.Context, pn types.JID) (types.JID, error) {
	if s.inner != nil {
		if lid, err := s.inner.GetLIDForPN(ctx, pn); err == nil && !lid.IsEmpty() {
			if s.mem != nil {
				// Aprovecha para dejar la pareja guardada: esta dirección sí la
				// resuelve la red, así que es una fuente fiable.
				_ = s.mem.PutIdentity(ctx, lid.String(), pn.String())
			}
			s.remember(lid.User, pn.User)
			return lid, nil
		}
	}
	if s.mem != nil {
		lid, err := s.mem.LIDForPN(ctx, pn.String())
		if err != nil {
			s.warn("no se pudo leer la pareja PN→LID", err)
		}
		if lid != "" {
			if jid, perr := types.ParseJID(lid); perr == nil {
				s.remember(jid.User, pn.User)
				return jid, nil
			}
		}
	}
	return types.EmptyJID, errNoMapping
}

func (s *lidStore) GetManyLIDsForPNs(ctx context.Context, pns []types.JID) (map[types.JID]types.JID, error) {
	if s.inner != nil {
		if out, err := s.inner.GetManyLIDsForPNs(ctx, pns); err == nil && len(out) > 0 {
			if s.mem != nil {
				for pn, lid := range out {
					_ = s.mem.PutIdentity(ctx, lid.String(), pn.String())
				}
			}
			return out, nil
		}
	}
	out := make(map[types.JID]types.JID, len(pns))
	for _, pn := range pns {
		if lid, err := s.GetLIDForPN(ctx, pn); err == nil {
			out[pn] = lid
		}
	}
	return out, nil
}

// ─── Caché en memoria ───────────────────────────────────────────────────────

func (s *lidStore) remember(lidUser, pnUser string) {
	if lidUser == "" || pnUser == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) >= lidCacheMax {
		// Se vacía en bloque: es una caché, no un archivo de estado.
		s.cache = make(map[string]string, lidCacheMax)
	}
	s.cache[lidUser] = pnUser
}

func (s *lidStore) lookup(lidUser string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	pn, ok := s.cache[lidUser]
	return pn, ok && pn != ""
}

func (s *lidStore) warn(msg string, err error) {
	if s.log != nil {
		s.log.Warn(msg + ": " + err.Error())
	}
}

// jidFrom arma un JID de teléfono a partir del número.
func jidFrom(number string) types.JID {
	return types.JID{User: number, Server: types.DefaultUserServer}
}
