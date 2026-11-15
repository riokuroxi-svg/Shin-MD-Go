// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// archivo.go — Log a archivo CON ROTACIÓN (bloque B7).
//
// ¿Por qué propio y no lumberjack? Porque son 60 líneas y una dependencia
// menos: este bot presume de binario pequeño y de no arrastrar node_modules
// disfrazados de go.mod.
//
// Reglas: cuando shin.log pasa del tope, shin.log.1 → .2 → .3 y se abre uno
// nuevo. Más allá de 3 atrás no queda nada: un bot que corre semanas en
// Termux no puede llenar la memoria del teléfono con texto.
package xlog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Rotativo es un io.Writer que rota el archivo al pasar del tope.
type Rotativo struct {
	mu      sync.Mutex
	nombre  string
	tope    int64
	backups int
	f       *os.File
	escrito int64
}

// AbrirRotativo abre (o crea) el archivo en modo «añadir». Si ya medía más
// que el tope, rota nada más abrir — no empieza un día nuevo heredando un
// tronco de semanas.
func AbrirRotativo(nombre string, tope int64, backups int) (*Rotativo, error) {
	if tope <= 0 {
		tope = 4 << 20 // 4 MB
	}
	if backups <= 0 {
		backups = 3
	}
	if err := os.MkdirAll(filepath.Dir(nombre), 0o755); err != nil {
		return nil, err
	}
	r := &Rotativo{nombre: nombre, tope: tope, backups: backups}
	if st, err := os.Stat(nombre); err == nil {
		if st.Size() >= tope {
			r.rotar()
		} else {
			r.escrito = st.Size()
		}
	}
	f, err := os.OpenFile(nombre, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	r.f = f
	return r, nil
}

// Write cumple io.Writer. Un error de escritura se devuelve y el log sigue
// saliendo por stdout (xlog usa MultiWriter): un disco lleno no mata al bot.
func (r *Rotativo) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return 0, fmt.Errorf("archivo cerrado")
	}
	if r.escrito+int64(len(p)) > r.tope {
		r.rotar()
	}
	n, err := r.f.Write(p)
	r.escrito += int64(n)
	return n, err
}

// rotar mueve nombre.1 → .2, etc., y reabre un archivo nuevo. Los errores de
// renombrado se ignoran a propósito: peor es dejar de escribir el log.
func (r *Rotativo) rotar() {
	if r.f != nil {
		_ = r.f.Close()
	}
	for i := r.backups; i >= 1; i-- {
		viejo := fmt.Sprintf("%s.%d", r.nombre, i)
		nuevo := fmt.Sprintf("%s.%d", r.nombre, i+1)
		if i == r.backups {
			_ = os.Remove(viejo)
			continue
		}
		_ = os.Rename(viejo, nuevo)
	}
	_ = os.Rename(r.nombre, fmt.Sprintf("%s.1", r.nombre))
	f, err := os.OpenFile(r.nombre, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		r.f = nil
		return
	}
	r.f = f
	r.escrito = 0
}

// Cerrar cierra el archivo (main lo llama al apagar).
func (r *Rotativo) Cerrar() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f != nil {
		_ = r.f.Close()
		r.f = nil
	}
}

// TamanoActual es el tamaño actual del archivo (para los tests y .health).
func (r *Rotativo) TamanoActual() int64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.escrito
}
