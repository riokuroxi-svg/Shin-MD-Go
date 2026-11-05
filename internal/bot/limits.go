// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// limits.go — Presupuestos por usuario para lo que NO es un comando.
//
// El cooldown del router solo protege los comandos. Los hooks (antilink, afk)
// ven todos los mensajes, y ahí no había ningún tope: alguien podía escribir
// una línea con un enlace cada segundo y el bot le contestaba a cada una. Eso
// es, literalmente, la receta para que WhatsApp marque el número como spam.
//
// Reglas que aplica RateGuard:
//
//	ventana de 1 minuto, contador por usuario y por acción
//	al pasarse, se calla (no se responde al abusón: responder también es tráfico)
//	avisa UNA vez por ventana, con un mínimo de 5 minutos entre avisos
//	mapa con techo: nunca crece más de 2048 usuarios
package bot

import (
	"sync"
	"time"
)

type rateEntry struct {
	hits      []time.Time
	lastWarn  time.Time
	warnedFor bool
}

// RateGuard cuenta acciones por usuario dentro de una ventana.
type RateGuard struct {
	window time.Duration
	max    int

	mu      sync.Mutex
	entries map[string]*rateEntry
}

const (
	guardMaxKeys    = 2048
	guardWarnEvery  = 5 * time.Minute
	guardWarnWindow = time.Minute
)

// NewRateGuard: `max` acciones por `window`.
func NewRateGuard(max int, window time.Duration) *RateGuard {
	if max < 1 {
		max = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	return &RateGuard{window: window, max: max, entries: make(map[string]*rateEntry)}
}

// Allow devuelve true si la acción está permitida. `warn` dice si toca avisar
// al usuario (una vez cada 5 minutos como mucho).
func (g *RateGuard) Allow(key string) (allow bool, warn bool) {
	if g == nil {
		return true, false
	}
	now := time.Now()

	g.mu.Lock()
	defer g.mu.Unlock()

	e, ok := g.entries[key]
	if !ok {
		// Techo duro: si el mapa está lleno se limpia lo caducado y, si aun
		// así sigue lleno, se deja pasar (mejor pasar que bloquear a todos).
		if len(g.entries) >= guardMaxKeys {
			g.pruneLocked(now)
			if len(g.entries) >= guardMaxKeys {
				return true, false
			}
		}
		e = &rateEntry{}
		g.entries[key] = e
	}

	// Ventana deslizante.
	kept := e.hits[:0]
	for _, t := range e.hits {
		if now.Sub(t) < g.window {
			kept = append(kept, t)
		}
	}
	e.hits = kept

	if len(e.hits) < g.max {
		e.hits = append(e.hits, now)
		return true, false
	}

	if !e.warnedFor || now.Sub(e.lastWarn) > guardWarnEvery {
		e.warnedFor = true
		e.lastWarn = now
		return false, true
	}
	return false, false
}

// pruneLocked borra a quien no tiene actividad reciente.
func (g *RateGuard) pruneLocked(now time.Time) {
	for k, e := range g.entries {
		if len(e.hits) == 0 || now.Sub(e.hits[len(e.hits)-1]) >= g.window {
			if now.Sub(e.lastWarn) >= guardWarnWindow {
				delete(g.entries, k)
			}
		}
	}
}
