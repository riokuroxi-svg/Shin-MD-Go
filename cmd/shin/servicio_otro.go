// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// servicio_otro.go — fuera de Unix no hay setsid ni señales: nada de demonios.
//
// Aquí «start» suelta el proceso con la salida ya redirigida al log (que es lo
// que lo hace sobrevible) y «stop» lo remata, sin prometer el cierre limpio que
// SIGTERM permite en Linux. «status» no comprueba si el PID vive: en Windows eso
// exige la API de procesos, así que se limita a decir lo que hay en el fichero
// en vez de inventarse un «● en ejecución».
//go:build !unix

package main

import (
	"os"
	"os/exec"
)

func desprender(exe string, args []string, salida *os.File) (int, error) {
	cmd := exec.Command(exe, args...)
	cmd.Stdout = salida
	cmd.Stderr = salida
	// Sin heredar la consola: con la salida en el log, el proceso ya no depende
	// de que la ventana siga abierta.
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	go func() { _ = cmd.Wait() }()
	return cmd.Process.Pid, nil
}

// procesoVivo: sin cómo preguntarlo, se da por supuesto (y «stop» mata). El
// coste de equivocarse aquí es un Kill sobre un PID que ya no existe: nada.
func procesoVivo(pid int) bool { return pid > 1 }

func terminarProceso(pid int) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill() // en Windows no hay SIGTERM que esperar: se cierra y ya
}

func matarAhora(pid int) error { return terminarProceso(pid) }

func medidasProceso(pid int) string { return "" }
