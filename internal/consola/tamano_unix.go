// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// tamano_unix.go — repintar cuando se cambia el tamaño de la ventana.
//
// En Termux es más común de lo que parece: girar el teléfono, abrir el teclado
// o pasar de vertical a horizontal. Sin esto el encuadre sigue dibujado para
// las filas de antes hasta el siguiente tick.
//go:build !windows

package consola

import (
	"os"
	"os/signal"
	"syscall"
)

func vigilarTamaño(alCambiar func()) func() {
	señ := make(chan os.Signal, 1)
	signal.Notify(señ, syscall.SIGWINCH)
	hecho := make(chan struct{})
	salir := make(chan struct{})
	go func() {
		defer close(hecho)
		for {
			select {
			case <-salir:
				return // hay que cerrar a mano: signal.Stop no cierra el canal
			case <-señ:
				alCambiar()
			}
		}
	}()
	return func() {
		signal.Stop(señ)
		close(salir)
		<-hecho
	}
}
