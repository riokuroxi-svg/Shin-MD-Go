// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// vitales_bsd.go — disco y RAM en macOS (*BSD).
//
// Mismo contrato que la versión de Linux: si algo no se puede saber, ceros, y
// el panel omite esa fila. Aquí la RAM viene de dos sysctls (hw.memsize y
// vm.page_size con vm_stat) porque macOS no tiene sysinfo: contar páginas
// libres × tamaño de página es lo que hace «top», y es lo bastante bueno para
// una línea de estado.
//
//go:build darwin || freebsd || openbsd || netbsd || dragonfly

package consola

import (
	"syscall"

	"golang.org/x/sys/unix"
)

func vitales() (discoUsado, discoTotal, ramUsada, ramTotal uint64) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(".", &st); err == nil && st.Bsize > 0 {
		// Los tipos de estos campos no son los mismos en cada BSD (en Darwin
		// Bsize es int32 y en FreeBSD Bavail es int64), así que se convierte
		// cada uno antes de multiplicar en vez de fiarse del de Linux.
		bsize := uint64(st.Bsize)
		discoTotal = uint64(st.Blocks) * bsize
		if libre := uint64(st.Bavail) * bsize; libre <= discoTotal {
			discoUsado = discoTotal - libre
		}
	}
	if n, err := unix.SysctlUint64("hw.memsize"); err == nil {
		ramTotal = n
	}
	ramUsada = ramUsadaMac()
	if ramUsada > ramTotal {
		ramUsada = 0 // un sysctl que cambia entre las dos lecturas no es dato
	}
	return
}

// ramUsadaMac: (activas + especulativas + wire) × tamaño de página, leído de
// «vm_stat». Si el formato no es el esperado, 0: mejor no enseñar la fila que
// enseñar una tontería.
func ramUsadaMac() uint64 {
	paginas, err := unix.Sysctl("vm.stat")
	if err != nil || paginas == "" {
		return 0
	}
	tam := uint64(4096)
	if n, err := unix.SysctlUint32("vm.page_size"); err == nil && n > 0 {
		tam = uint64(n)
	}
	cuentas := map[string]uint64{}
	for _, campo := range []string{"Pages active", "Pages speculative", "Pages wired down"} {
		i := indexOf(paginas, campo)
		if i < 0 {
			return 0
		}
		resto := paginas[i+len(campo):]
		num := ""
		for _, r := range resto {
			if r >= '0' && r <= '9' {
				num += string(r)
			} else if num != "" {
				break
			}
		}
		n := uint64(0)
		for _, d := range num {
			n = n*10 + uint64(d-'0')
		}
		if num == "" {
			return 0
		}
		cuentas[campo] = n
	}
	var suma uint64
	for _, n := range cuentas {
		suma += n
	}
	return suma * tam
}

func indexOf(donde, que string) int {
	for i := 0; i+len(que) <= len(donde); i++ {
		if donde[i:i+len(que)] == que {
			return i
		}
	}
	return -1
}
