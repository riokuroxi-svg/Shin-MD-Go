// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// vitales_unix.go — disco y RAM del propio equipo (Linux y Android).
//
// syscall.Sysinfo y Statfs son llamadas corrientes y el seccomp de Android las
// deja pasar. Va limitada a linux/android a propósito: «Sysinfo» es un invento
// de Linux —en macOS la RAM se pregunta por sysctl (vitales_bsd.go)— y colgar
// de que el paquete syscall tenga el mismo nombre en todos lados es la forma
// clásica de romper un build que nadie prueba.
//
//go:build linux || android

package consola

import "syscall"

// vitales: usados/total. Si algo falla (sandbox rara), ceros y la línea se
// omite en el panel — la estética jamás tumba el bot.
func vitales() (discoUsado, discoTotal, ramUsada, ramTotal uint64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(".", &st); err == nil && st.Bsize > 0 {
		discoTotal = st.Blocks * uint64(st.Bsize)
		if libre := st.Bavail * uint64(st.Bsize); libre <= discoTotal {
			discoUsado = discoTotal - libre
		}
	}
	var si syscall.Sysinfo_t
	if err := syscall.Sysinfo(&si); err == nil && si.Unit > 0 {
		u := uint64(si.Unit)
		ramTotal = si.Totalram * u
		if libre := (si.Freeram + si.Bufferram) * u; libre <= ramTotal {
			ramUsada = ramTotal - libre
		}
	}
	return
}
