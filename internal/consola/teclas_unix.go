// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// teclas_unix.go — modo «tecla a tecla» sin cargarse Ctrl+C.
//
//go:build linux || android

package consola

import (
	"golang.org/x/sys/unix"
)

// modoTeclas pone el terminal «tecla a tecla» —sin eco y sin esperar Enter—
// para poder leer atajos y flechas, y devuelve la función que lo devuelve a
// su estado anterior.
//
// No se usa term.MakeRaw a propósito: MakeRaw apaga ISIG, y entonces Ctrl+C
// deja de ser una señal y llega como el byte 0x03. Para quien corre el bot en
// Termux o por ssh eso es quedarse sin la forma de parar de toda la vida (y
// sin `kill -INT`, nada menos). Aquí se apaga solo ECHO e ICANON: las teclas
// se leen al instante y Ctrl+C sigue matando el proceso.
func modoTeclas(fd int) (func(), error) {
	ts, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return func() {}, err
	}
	guardado := *ts
	ts.Lflag &^= unix.ECHO | unix.ICANON // ISIG se queda intacto, a propósito
	ts.Iflag &^= unix.ICRNL | unix.INLCR | unix.IGNCR
	// OPOST se queda como está, y es un detalle con historia: apagarlo (como
	// hace MakeRaw) deja de traducir \n a \r\n en el terminal, y como el modo
	// de teclas dura minutos y lo usan también el log, las cajas y el registro,
	// toda esa salida salía en escalera —línea que empieza donde acabó la
	// anterior— al bajar al registro con `e`. No se toca: \n se traduce solo, y
	// el \r\n que escribe el panel queda en un \r\r\n inofensivo.
	ts.Cc[unix.VMIN] = 1
	ts.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, ts); err != nil {
		return func() {}, err
	}
	return func() { _ = unix.IoctlSetTermios(fd, unix.TCSETS, &guardado) }, nil
}
