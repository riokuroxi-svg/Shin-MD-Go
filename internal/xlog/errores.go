// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// errores.go — los últimos avisos y errores, en memoria, para `.health`.
//
// El bot de JS tenía un módulo de diagnóstico que el dueño podía consultar por
// WhatsApp («¿qué se rompió en la última hora?»). Aquí los errores solo iban al
// log del sistema: si estás en el móvil y el bot falla, no te enteras hasta
// abrir Termux.
//
// Esto NO es un log: es un anillo de los últimos N mensajes de nivel aviso o
// error, que se pierde al reiniciar (el log de verdad sigue en el archivo). Se
// guarda corto a propósito: si algo falla en bucle no puede comerse la RAM.
package xlog

import (
	"sync"
	"time"
)

// Registro es un aviso o error guardado para consultarlo desde el chat.
type Registro struct {
	Cuando  time.Time
	Nivel   string // "aviso" o "error"
	Mensaje string
}

const maxRegistros = 64

var (
	erroresMu  sync.Mutex
	errores    []Registro
	erroresTot int
)

// Record guarda un aviso o error. Lo llaman Warn y Err; no hace falta usarlo a
// mano. Si un mensaje es muy largo se recorta: en WhatsApp tampoco cabe.
func Record(nivel, mensaje string) {
	if mensaje == "" {
		return
	}
	if len([]rune(mensaje)) > 300 {
		mensaje = string([]rune(mensaje)[:300]) + "…"
	}
	erroresMu.Lock()
	defer erroresMu.Unlock()
	erroresTot++
	errores = append(errores, Registro{Cuando: time.Now(), Nivel: nivel, Mensaje: mensaje})
	if len(errores) > maxRegistros {
		// Se copia para que el array viejo (y su memoria) se libere de verdad:
		// `errores[1:]` dejaría el primero vivo mientras dure el slice.
		nuevo := make([]Registro, maxRegistros)
		copy(nuevo, errores[len(errores)-maxRegistros:])
		errores = nuevo
	}
}

// UltimosErrores devuelve los últimos `n` registros, del más viejo al más
// nuevo. Con n <= 0 devuelve todos los que haya.
func UltimosErrores(n int) []Registro {
	erroresMu.Lock()
	defer erroresMu.Unlock()
	if n <= 0 || n > len(errores) {
		n = len(errores)
	}
	out := make([]Registro, n)
	copy(out, errores[len(errores)-n:])
	return out
}

// ContarErrores es el total desde que arrancó el proceso (no solo los últimos).
func ContarErrores() int {
	erroresMu.Lock()
	defer erroresMu.Unlock()
	return erroresTot
}

// LimpiarErrores vacía el anillo y pone el contador a cero. Lo usa
// `.health clear`, que es solo del dueño.
func LimpiarErrores() {
	erroresMu.Lock()
	defer erroresMu.Unlock()
	errores = nil
	erroresTot = 0
}
