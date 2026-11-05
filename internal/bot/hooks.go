// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// hooks.go — El "antes" del router: código que ve TODOS los mensajes, no solo
// los comandos. Es donde viven antilink, antiflood, afk y todo lo que tiene
// que actuar sobre lo que la gente escribe normalmente.
//
// Orden de ejecución (igual que el JS):
//
//	mensaje → filtros duros (propio, estado, difusión) → hooks → comando
//
// Un hook devuelve `true` cuando ya atendió el mensaje: el router se detiene y
// no se ejecuta ningún comando. Así un antilink puede borrar un mensaje sin que
// además el bot intente interpretarlo como comando.
package bot

import (
	"fmt"
	"runtime/debug"
	"sort"
	"sync"

	"shin-go/internal/wa"
)

// Hook se ejecuta antes de los comandos. `parsed` trae el texto ya
// desenvuelto (efímeros incluidos) y el comando detectado, si lo hay.
type Hook func(c *Context, parsed wa.Parsed) bool

type registeredHook struct {
	name  string
	order int
	run   Hook
}

var (
	hooksMu    sync.RWMutex
	hooks      []registeredHook
	hooksOrder int
)

// RegisterHook añade un hook al final de la cadena. `name` es solo para logs.
// Los hooks se ejecutan en el orden en que se registran.
func RegisterHook(name string, h Hook) {
	if h == nil {
		panic("bot.RegisterHook: hook nil")
	}
	hooksMu.Lock()
	defer hooksMu.Unlock()
	hooksOrder++
	hooks = append(hooks, registeredHook{name: name, order: hooksOrder, run: h})
	sort.SliceStable(hooks, func(i, j int) bool { return hooks[i].order < hooks[j].order })
}

// RunHooks pasa el mensaje por toda la cadena. Devuelve true si algún hook ya
// lo atendió. Un pánico en un hook se registra y se sigue con el siguiente:
// un antilink roto no puede dejar el bot sordo.
func (r *Router) RunHooks(c *Context, parsed wa.Parsed) (handled bool) {
	hooksMu.RLock()
	chain := make([]registeredHook, len(hooks))
	copy(chain, hooks)
	hooksMu.RUnlock()

	for _, h := range chain {
		if stopped := runOne(h, c, parsed); stopped {
			return true
		}
	}
	return false
}

func runOne(h registeredHook, c *Context, parsed wa.Parsed) (stopped bool) {
	defer func() {
		if rec := recover(); rec != nil {
			c.Log.Err(fmt.Sprintf("pánico en el hook %s: %v\n%s", h.name, rec, debug.Stack()))
		}
	}()
	return h.run(c, parsed)
}

// ResetHooks vacía la cadena. Sirve para las pruebas (el registro es global) y
// para una recarga en caliente futura.
func ResetHooks() {
	hooksMu.Lock()
	hooks = nil
	hooksOrder = 0
	hooksMu.Unlock()
}

// HookCount: cuántos hooks hay (lo imprime el arranque).
func HookCount() int {
	hooksMu.RLock()
	defer hooksMu.RUnlock()
	return len(hooks)
}
