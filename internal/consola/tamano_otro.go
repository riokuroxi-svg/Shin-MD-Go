// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// tamano_otro.go — sin SIGWINCH (Windows): el refresco por tick basta.
//go:build windows

package consola

func vigilarTamaño(alCambiar func()) func() { return func() {} }
