// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// eventos.go — Lo que el bot hace cuando cambia un grupo (no cuando alguien
// escribe): entradas, salidas, ascensos, cambios de nombre o de descripción.
//
// Por qué existe este archivo: la caché de grupos (`group.go`) tiene un minuto
// de vida, y los permisos se deciden con ella. Si alguien asciende a otro y le
// escribe justo después, con la caché vieja el bot diría que no es admin. Con
// los eventos, la caché se tira en el momento.
//
// La bienvenida y la despedida viven aquí como "oyentes": `cmds` registra el
// suyo y este archivo no sabe nada de textos ni de ajustes. Así el paquete bot
// no depende de los comandos (que sí dependen de bot).
package bot

import (
	"context"
	"runtime/debug"
	"sync"

	"go.mau.fi/whatsmeow/types/events"

	"shin-go/internal/store"
	"shin-go/internal/xlog"
)

// GroupEvent es un oyente de cambios de grupo.
type GroupEvent func(ctx context.Context, db *store.Store, log *xlog.Logger, evt *events.GroupInfo)

type registeredGroupEvent struct {
	name string
	run  GroupEvent
}

var (
	groupEventsMu sync.RWMutex
	groupEvents   []registeredGroupEvent
)

// RegisterGroupEvent añade un oyente de cambios de grupo.
func RegisterGroupEvent(name string, fn GroupEvent) {
	if fn == nil {
		panic("bot.RegisterGroupEvent: oyente nil")
	}
	groupEventsMu.Lock()
	defer groupEventsMu.Unlock()
	groupEvents = append(groupEvents, registeredGroupEvent{name: name, run: fn})
}

// ResetGroupEvents vacía los oyentes (pruebas).
func ResetGroupEvents() {
	groupEventsMu.Lock()
	groupEvents = nil
	groupEventsMu.Unlock()
}

// HandleGroupInfo atiende un cambio de grupo: primero tira la caché (siempre,
// pase lo que pase después) y luego avisa a los oyentes. Un oyente roto no
// puede impedir que los demás se enteren.
func HandleGroupInfo(ctx context.Context, db *store.Store, log *xlog.Logger, evt *events.GroupInfo) {
	if evt == nil {
		return
	}

	if r := activeRouter; r != nil && !evt.JID.IsEmpty() {
		r.Groups().Invalidate(evt.JID)
	}

	groupEventsMu.RLock()
	chain := make([]registeredGroupEvent, len(groupEvents))
	copy(chain, groupEvents)
	groupEventsMu.RUnlock()

	for _, g := range chain {
		runGroupEvent(g, ctx, db, log, evt)
	}
}

func runGroupEvent(g registeredGroupEvent, ctx context.Context, db *store.Store, log *xlog.Logger, evt *events.GroupInfo) {
	defer func() {
		if rec := recover(); rec != nil {
			if log != nil {
				log.Err("pánico en el oyente de grupo " + g.name + ": " + toText(rec) + "\n" + string(debug.Stack()))
			}
		}
	}()
	g.run(ctx, db, log, evt)
}

func toText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if e, ok := v.(error); ok {
		return e.Error()
	}
	return "error desconocido"
}

// GroupEventCount: cuántos oyentes hay (lo imprime el arranque).
func GroupEventCount() int {
	groupEventsMu.RLock()
	defer groupEventsMu.RUnlock()
	return len(groupEvents)
}
