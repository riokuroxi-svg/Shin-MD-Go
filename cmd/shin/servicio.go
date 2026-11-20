// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// servicio.go — «shin start · stop · status · logs».
//
// Por qué existe esto: en un VPS no hay Ctrl+C. La sesión SSH se cae, el bot
// se queda colgado de ella o, peor, alguien lo mata con `kill -9` y nadie sabe
// cómo volver a levantarlo. Un bot que sólo se para con una tecla no sirve en
// una máquina sin pantalla.
//
// Estas cuatro órdenes son todo lo que hace falta, sin systemd y sin PM2:
//
//	shin start          arranca en segundo plano (se despega del terminal)
//	shin status         si vive, desde cuándo y qué dice el log
//	shin logs -f        seguir el log, como journalctl -f
//	shin stop           SIGTERM y esperar el cierre limpio
//
// Se guarda el PID en data/shin.pid. Nada de demonios con sockets de control:
// un fichero y una señal, que es lo que entiende cualquier servidor.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"shin-go/internal/config"
	"shin-go/internal/consola"
)

// ordenDeServicio: si el primer argumento es una de estas palabras, no se
// arranca el bot: se atiende la orden y se sale.
func ordenDeServicio(arg string) bool {
	switch arg {
	case "start", "stop", "restart", "status", "logs":
		return true
	}
	return false
}

// servicio ejecuta la orden y devuelve el código de salida.
func servicio(orden string, args []string) int {
	cfg := config.Load()
	rutaPID := filepath.Join(cfg.StateDir, "shin.pid")

	switch orden {
	case "start":
		return iniciarServicio(rutaPID, cfg.LogFile, args)
	case "stop":
		return pararServicio(rutaPID)
	case "restart":
		if pararServicio(rutaPID) != 0 {
			return 1
		}
		return iniciarServicio(rutaPID, cfg.LogFile, args)
	case "status":
		return estadoServicio(rutaPID, cfg.LogFile)
	case "logs":
		return logServicio(cfg.LogFile, args)
	}
	return 2
}

// ─── el fichero de PID ────────────────────────────────────────────────────

// leerPID: el número que hay en el fichero, o 0 si no lo hay o está rayado.
func leerPID(ruta string) int {
	b, err := os.ReadFile(ruta)
	if err != nil {
		return 0
	}
	campos := strings.Fields(string(b))
	if len(campos) == 0 {
		return 0
	}
	n, err := strconv.Atoi(campos[0])
	if err != nil || n <= 1 {
		return 0
	}
	return n
}

// ─── start ────────────────────────────────────────────────────────────────

func iniciarServicio(rutaPID, log string, argumentos []string) int {
	if pid := leerPID(rutaPID); pid > 0 && procesoVivo(pid) {
		fmt.Fprintln(os.Stdout, "  "+consola.Ambar("!")+" ya está corriendo (pid "+strconv.Itoa(pid)+").")
		fmt.Fprintln(os.Stdout, "    "+consola.Tenue("«shin restart» para reiniciarlo · «shin logs -f» para leerlo"))
		return 0
	}
	_ = os.Remove(rutaPID) // un fichero de un proceso muerto no puede quedar ahí

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "  "+consola.Rojo("✕")+" no sé qué binario arrancar: "+err.Error())
		return 1
	}
	if err := os.MkdirAll(filepath.Dir(rutaPID), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "  "+consola.Rojo("✕")+" no se pudo crear "+filepath.Dir(rutaPID)+": "+err.Error())
		return 1
	}
	salida, err := os.OpenFile(log, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		// Sin archivo de log el arranque en segundo plano no tiene sentido: no
		// se podría leer nada. Se avisa y no se arranca a ciegas.
		fmt.Fprintln(os.Stderr, "  "+consola.Rojo("✕")+" no se pudo abrir el log "+log+": "+err.Error())
		fmt.Fprintln(os.Stderr, "    "+consola.Tenue("arráncalo en primero plano («shin», a secas) o arregla los permisos"))
		return 1
	}

	pid, err := desprender(exe, argumentos, salida)
	salida.Close() // el hijo ya tiene su propio descriptor
	if err != nil {
		fmt.Fprintln(os.Stderr, "  "+consola.Rojo("✕")+" no se pudo arrancar: "+err.Error())
		return 1
	}
	if err := os.WriteFile(rutaPID, []byte(strconv.Itoa(pid)+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "  "+consola.Ambar("!")+" arrancó (pid "+strconv.Itoa(pid)+
			") pero no se pudo escribir "+rutaPID+": «shin stop» no funcionará")
	}

	// Y aquí está la gracia: se puede cerrar la SSH. El proceso va en su propia
	// sesión, así que sobrevive (nohup + setsid, sin depender de PM2).
	fmt.Fprintln(os.Stdout, "  "+consola.Menta("✓")+" Shin-MD-Go arrancado en segundo plano ("+
		consola.Papel("pid "+strconv.Itoa(pid))+").")
	fmt.Fprintln(os.Stdout, "  "+consola.Tenue("log     ")+" "+log)
	fmt.Fprintln(os.Stdout, "  "+consola.Tenue("estado  ")+" shin status")
	fmt.Fprintln(os.Stdout, "  "+consola.Tenue("a partir de ahora puedes cerrar esta sesión"))
	if sinTerminalHeredada() {
		fmt.Fprintln(os.Stdout, "  "+consola.Tenue("vincular: shin start --code 3412345678"))
	}
	return 0
}

// sinTerminalHeredada: si quien nos llama ya está sin terminal (systemd,
// cron), proponerle «shin start --code» no sirve: ahí el bot tampoco pregunta
// el número. Es el caso del VPS recién instalado.
func sinTerminalHeredada() bool { return !consola.EsTerminal() }

// ─── stop ─────────────────────────────────────────────────────────────────

func pararServicio(rutaPID string) int {
	pid := leerPID(rutaPID)
	if pid == 0 {
		fmt.Fprintln(os.Stdout, "  "+consola.Tenue("no hay shin corriendo (no existe "+rutaPID+")"))
		_ = os.Remove(rutaPID)
		return 0
	}
	if !procesoVivo(pid) {
		fmt.Fprintln(os.Stdout, "  "+consola.Ambar("!")+
			" el pid "+strconv.Itoa(pid)+" del fichero ya no existe; se limpia el fichero")
		_ = os.Remove(rutaPID)
		return 0
	}
	fmt.Print("  " + consola.Tenue("pidiendo parar al pid "+strconv.Itoa(pid)+"…"))
	if err := terminarProceso(pid); err != nil {
		fmt.Println("\r  " + consola.Rojo("✕") + " no se pudo señalar a " + strconv.Itoa(pid) + ": " + err.Error())
		return 1
	}
	// SIGTERM = «cierra limpio». Se espera: si se matara sin más, quedarían el
	// lock de la base de datos y el log a medias.
	for i := 0; i < 60; i++ { // hasta ~9 s, que es lo que tarda en cerrar web + base de datos
		time.Sleep(150 * time.Millisecond)
		if !procesoVivo(pid) {
			fmt.Print("\r")
			fmt.Fprintln(os.Stdout, "  "+consola.Menta("✓")+" parado.")
			_ = os.Remove(rutaPID)
			return 0
		}
	}
	fmt.Print("\r")
	fmt.Fprintln(os.Stdout, "  "+consola.Ambar("!")+" sigue vivo tras 9 s; se le da un segundo con SIGKILL")
	_ = matarAhora(pid)
	time.Sleep(500 * time.Millisecond)
	_ = os.Remove(rutaPID)
	return 0
}

// ─── status ───────────────────────────────────────────────────────────────

func estadoServicio(rutaPID, log string) int {
	pid := leerPID(rutaPID)
	if pid == 0 {
		fmt.Fprintln(os.Stdout, "  "+consola.Rojo("○")+" parado (sin "+filepath.Base(rutaPID)+")")
		return 1
	}
	if !procesoVivo(pid) {
		fmt.Fprintln(os.Stdout, "  "+consola.Rojo("○")+
			" NO está corriendo, aunque el fichero dice pid "+strconv.Itoa(pid))
		fmt.Fprintln(os.Stdout, "  "+consola.Tenue("«shin start» lo levanta de nuevo"))
		return 1
	}
	fmt.Fprintln(os.Stdout, "  "+consola.Menta("●")+" en ejecución · "+consola.Papel("pid "+strconv.Itoa(pid))+
		medidasProceso(pid))
	if st, err := os.Stat(log); err == nil {
		fmt.Fprintln(os.Stdout, "  "+consola.Tenue("log: "+log+" · "+
			consola.Humanizar(uint64(st.Size()))+" · actualizado hace "+
			time.Since(st.ModTime()).Truncate(time.Second).String()))
	} else {
		fmt.Fprintln(os.Stdout, "  "+consola.Ambar("!")+" sin log en "+log)
	}
	// Las últimas líneas, que es lo que uno quiere ver con «status» en un VPS:
	// si el bot está conectado o está relanzando, aquí se nota.
	if lineas, err := ultimasLineas(log, 6); err == nil && len(lineas) > 0 {
		fmt.Fprintln(os.Stdout, "  "+consola.Tenue("── lo último del log ─────────────"))
		for _, l := range lineas {
			fmt.Fprintln(os.Stdout, "  "+consola.Tenue("│ ")+consola.Papel(l))
		}
	}
	return 0
}

// ─── logs ─────────────────────────────────────────────────────────────────

func logServicio(log string, args []string) int {
	seguir := false
	n := 50
	for _, a := range args {
		switch {
		case a == "-f" || a == "--follow":
			seguir = true
		case strings.HasPrefix(a, "-n="):
			n, _ = strconv.Atoi(strings.TrimPrefix(a, "-n="))
		default:
			if v, err := strconv.Atoi(a); err == nil { // «shin logs 200»
				n = v
			}
		}
	}
	if n < 1 {
		n = 1
	}
	lineas, err := ultimasLineas(log, n)
	if err != nil {
		fmt.Fprintln(os.Stderr, "  "+consola.Rojo("✕")+" no se pudo leer "+log+": "+err.Error())
		fmt.Fprintln(os.Stderr, "  "+consola.Tenue("¿el bot arrancó alguna vez? «shin status»"))
		return 1
	}
	for _, l := range lineas {
		fmt.Fprintln(os.Stdout, l)
	}
	if !seguir {
		return 0
	}

	// Seguir el fichero por tamaño, no con inotify: un log rotado (4 MB × 3)
	// cambia de inode y quien persigue el descriptor deja de ver líneas sin
	// avisar. Reabrir cuando el fichero encoge es más tonto y funciona igual.
	f, err := os.Open(log)
	if err != nil {
		return 1
	}
	defer f.Close()
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return 1
	}
	lect := bufio.NewReader(f)
	fmt.Fprintln(os.Stdout, consola.Tenue("  ── siguiendo "+log+" · Ctrl+C para dejar de mirar ──"))
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		st, err := f.Stat()
		if err != nil {
			continue
		}
		actual, err := f.Seek(0, io.SeekCurrent)
		if err != nil {
			continue
		}
		if st.Size() < actual { // rotado o truncado: empezar de cero
			f.Close()
			if f, err = os.Open(log); err != nil {
				fmt.Fprintln(os.Stderr, "  "+consola.Rojo("✕")+" "+err.Error())
				return 1
			}
			lect = bufio.NewReader(f)
		}
		for {
			linea, err := lect.ReadString('\n')
			if linea != "" {
				fmt.Print(linea)
			}
			if err != nil {
				break
			}
		}
	}
	return 0
}

// ultimasLineas: las n últimas del fichero, leyéndolo desde el final. Un log de
// Termux puede llevar semanas; leerlo entero para enseñar 50 líneas es una
// tontería (y en un móvil, además, tarda).
func ultimasLineas(ruta string, n int) ([]string, error) {
	f, err := os.Open(ruta)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	const trozo = 8192
	var buf []byte
	for desde := st.Size(); desde > 0 && strings.Count(string(buf), "\n") <= n; {
		t := int64(trozo)
		if desde < t {
			t = desde
		}
		desde -= t
		parte := make([]byte, t)
		if _, err := f.ReadAt(parte, desde); err != nil && err != io.EOF {
			return nil, err
		}
		buf = append(parte, buf...)
	}
	recortadas := strings.Split(strings.TrimRight(string(buf), "\n"), "\n")
	if len(recortadas) > n {
		recortadas = recortadas[len(recortadas)-n:]
	}
	return recortadas, nil
}
