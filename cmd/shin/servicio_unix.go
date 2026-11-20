// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// servicio_unix.go — soltar el proceso del terminal (Linux, Android, macOS).
//
// setsid es lo que hace que «shin start» sobreviva a que cierres la SSH o a
// que Termux se vaya a segundo plano: el hijo no es ya hijo de la shell, sino
// líder de su propia sesión, y no recibe SIGHUP cuando el terminal muere.
//go:build unix

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"shin-go/internal/consola"
)

// desprender: arrancar `exe` en segundo plano y devolver su PID. La salida va
// al log (ya abierto por quien llama): si se lanzara sin redirigir, el hijo
// heredaría el terminal y se moriría con él.
func desprender(exe string, args []string, salida *os.File) (int, error) {
	cmd := exec.Command(exe, args...)
	cmd.Dir = directoriosDelHijo(exe)
	cmd.Stdin = nil
	cmd.Stdout = salida
	cmd.Stderr = salida
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return 0, err
	}
	pid := cmd.Process.Pid
	// No se espera: hay que irse. Se hace un Wait en diferido para que no se
	// quede como zombí hasta que muera esta shell (en Termux, donde el bot puede
	// vivir semanas dentro de otra sesión, eso sí se nota).
	go func() { _ = cmd.Wait() }()
	return pid, nil
}

// directoriosDelHijo: el bot se ejecuta desde la ruta de datos, así que se le
// pone el mismo directorio de trabajo que tenía el instalador: si alguien
// arrancó «shin» desde $HOME con ./data relativo, el hijo debe verlo igual.
func directoriosDelHijo(exe string) string {
	if d := filepath.Dir(exe); d != "" && d != "." {
		if _, err := os.Stat(d); err == nil {
			return d
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

// procesoVivo: si hay un proceso con ese PID que todavía sea un proceso. La
// señal 0 no le hace nada, sólo pregunta, y EPERM significa «existe pero es de
// otro» (cuenta como vivo: no hay que borrar el fichero de otro usuario).
//
// El detalle que no es de adorno: un proceso ya terminado que nadie esperó se
// queda como ZOMBI, y matar(0) responde «sí hay algo». Sin esta comprobación
// «shin status» diría «en ejecución» de un bot que murió hace horas y «shin
// stop» se pasaría nueve segundos esperando a alguien que ya no existe.
func procesoVivo(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	if estado, ok := estadoProc(pid); ok && estado == 'Z' {
		return false
	}
	return true
}

// estadoProc: la letra de estado de /proc/<pid>/stat (R, S, D, T, Z…). Si no
// hay /proc (o el proceso es de otro usuario y no deja leerlo), ok=false y el
// que llama se queda con la respuesta de kill(0).
func estadoProc(pid int) (byte, bool) {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, false
	}
	// El segundo campo es el nombre entre paréntesis y PUEDE contener espacios:
	// hay que buscar el último ')' y contar desde ahí.
	fin := strings.LastIndexByte(string(b), ')')
	if fin < 0 || fin+2 >= len(b) {
		return 0, false
	}
	campos := strings.Fields(string(b[fin+1:]))
	if len(campos) == 0 {
		return 0, false
	}
	return campos[0][0], true
}

// terminarProceso: SIGTERM, el «cierra limpio» que el bot sabe atender.
func terminarProceso(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// matarAhora: el último recurso, cuando el bot no cerró en 9 s.
func matarAhora(pid int) error { return syscall.Kill(pid, syscall.SIGKILL) }

// medidasProceso: «· 1h 4m · 24M», leyendo /proc. Si no está (otro SO, un
// contenedor con /proc montado raro), se dice sólo el pid y tan anchos: el
// estado del bot no depende de saber esto.
func medidasProceso(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return ""
	}
	campos := strings.Fields(string(b))
	if len(campos) < 22 {
		return ""
	}
	salida := ""
	if t, err := strconv.ParseInt(campos[21], 10, 64); err == nil && t > 0 {
		salida += " · " + duracionDesdeArranque(t)
	}
	if r, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm"); err == nil {
		palabras := strings.Fields(string(r))
		if len(palabras) >= 2 {
			if pag, err := strconv.ParseInt(palabras[1], 10, 64); err == nil && pag > 0 {
				salida += " · " + consola.Humanizar(uint64(pag)*uint64(os.Getpagesize()))
			}
		}
	}
	return salida
}

// duracionDesdeArranque: cuántos segundos lleva vivo el proceso. El campo 22
// de /proc/<pid>/stat son ticks DESDE EL ARRANQUE DEL SISTEMA, así que la
// cuentas se hacen contra /proc/uptime y no contra el reloj: así no influye ni
// el ajuste de hora ni la zona horaria.
func duracionDesdeArranque(ticks int64) string {
	const hz = 100 // CONFIG_HZ en casi todos los núcleos
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return ""
	}
	campos := strings.Fields(string(b))
	if len(campos) == 0 {
		return ""
	}
	up, err := strconv.ParseFloat(campos[0], 64)
	if err != nil {
		return ""
	}
	segundos := int64(up) - ticks/hz
	if segundos < 0 {
		return ""
	}
	return consola.Duracion(time.Duration(segundos) * time.Second)
}
