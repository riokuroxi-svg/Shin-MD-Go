// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// vitales_windows.go — en Windows no hay syscall.Sysinfo/Statfs tal cual;
// el panel de vitales se omite (ceros) y ya. Que la estética nunca rompa
// la compilación cruzada.
//
//go:build windows

package consola

func vitales() (discoUsado, discoTotal, ramUsada, ramTotal uint64) { return 0, 0, 0, 0 }
