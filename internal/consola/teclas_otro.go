// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// teclas_otro.go — el mismo modo en los sistemas sin termios accesible.
//
//go:build !(linux || android)

package consola

import "golang.org/x/term"

// modoTeclas (fuera de Linux/Android): sin acceso cómodo a los termios, se usa
// el modo crudo de x/term. Ahí Ctrl+C llega como byte 0x03; Atajos lo traduce
// a «ctrl-c» y quien llama le pone la acción de salir, así que se sigue
// pudiendo parar el bot con las dos manos de siempre.
func modoTeclas(fd int) (func(), error) {
	estado, err := term.MakeRaw(fd)
	if err != nil {
		return func() {}, err
	}
	return func() { _ = term.Restore(fd, estado) }, nil
}
