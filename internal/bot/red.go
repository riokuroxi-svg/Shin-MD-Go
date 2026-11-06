// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// red.go — puente entre la configuración y el rotador de direcciones.
//
// El rotador vive en internal/red. Aquí solo se guarda el que usa el proceso
// para que `.info` pueda enseñar su estado sin acoplarse a main.
package bot

import (
	"sync/atomic"

	"shin-go/internal/red"
)

// redActivo es el rotador del proceso, igual que el repartidor: se guarda en un
// atómico porque lo escriben el arranque y lo leen los workers.
var redActivo atomic.Pointer[red.Rotador]

// SetRed registra el rotador del proceso (nil = salir por la dirección del
// sistema, que es el comportamiento de siempre).
func SetRed(r *red.Rotador) { redActivo.Store(r) }

// RedActivo devuelve el rotador configurado, o nil si no hay.
func RedActivo() *red.Rotador { return redActivo.Load() }
