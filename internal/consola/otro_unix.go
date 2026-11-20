// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// otro_unix.go — en macOS/BSD también se puede abrir /dev/tty (es lo que salva
// el caso «curl | sh» con el bot sin terminal estándar).
//
//go:build !windows

package consola

func runtimeSinTTY() bool { return false }
