// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// servicio_test.go — que «shin stop» mate de verdad y «shin status» no mienta.
//
// Lo importante aquí no es el formato: es que una orden escrita en un VPS sin
// teclado encuentre el proceso, le mande la señal correcta y no deje un
// fichero de PID mentiroso detrás. Se prueba con procesos reales (`sleep`), no
// con imitaciones de la lógica.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// capturar se lleva lo que la orden imprime, para poder exigirlo después. Se
// cambia os.Stdout por un fichero: las órdenes escriben por él en cada llamada,
// así que no hay que duplicar descriptores ni engañar a nadie.
func capturar(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	tmp, err := os.CreateTemp(t.TempDir(), "salida")
	if err != nil {
		t.Fatal(err)
	}
	real := os.Stdout
	os.Stdout = tmp
	codigo := fn()
	os.Stdout = real
	_ = tmp.Sync()
	b, _ := os.ReadFile(tmp.Name())
	tmp.Close()
	return string(b), codigo
}

func TestOrdenDeServicioReconoceLasCuatro(t *testing.T) {
	for _, o := range []string{"start", "stop", "restart", "status", "logs"} {
		if !ordenDeServicio(o) {
			t.Errorf("%q tiene que ser una orden de servicio", o)
		}
	}
	for _, o := range []string{"--code", "info", "-h", "", "estado"} {
		if ordenDeServicio(o) {
			t.Errorf("%q no es una orden de servicio", o)
		}
	}
}

func TestLeerPIDToleraBasura(t *testing.T) {
	casos := []struct {
		nombre, contenido string
		quiere            int
	}{
		{"normal", "1234\n", 1234},
		{"con espacios", "  99  ", 99},
		{"vacío", "", 0},
		{"rayado", "abc\n", 0},
		{"pid imposible", "1\n", 0},          // el 1 es init: nunca es el bot
		{"pid negativo", "-5\n", 0},          //
		{"basurilla", "4242 basura\n", 4242}, // el resto de la línea no importa
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			ruta := filepath.Join(t.TempDir(), "shin.pid")
			if err := os.WriteFile(ruta, []byte(c.contenido), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := leerPID(ruta); got != c.quiere {
				t.Errorf("leerPID(%q) = %d, quería %d", c.contenido, got, c.quiere)
			}
		})
	}
	if got := leerPID(filepath.Join(t.TempDir(), "no-existe.pid")); got != 0 {
		t.Errorf("sin fichero tiene que dar 0, dio %d", got)
	}
}

// TestPararSirveMataYSuFichero: el caso de uso entero. Se levanta un proceso
// real, se le pone en el fichero y «shin stop» tiene que matarlo y limpiar.
func TestPararSirveMataYSuFichero(t *testing.T) {
	dir := t.TempDir()
	rutaPID := filepath.Join(dir, "shin.pid")

	hijo := execSleep(t)
	if err := os.WriteFile(rutaPID, []byte(strconv.Itoa(hijo.Process.Pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	salida, codigo := capturar(t, func() int { return pararServicio(rutaPID) })
	if codigo != 0 {
		t.Errorf("stop devolvió %d\n%s", codigo, salida)
	}
	if !strings.Contains(salida, "parado") {
		t.Errorf("stop debe decir que paró: %q", salida)
	}
	if _, err := os.Stat(rutaPID); !os.IsNotExist(err) {
		t.Error("stop dejó el fichero de PID: un «start» después diría que ya corre")
	}
	// Y el proceso tiene que estar muerto de verdad.
	done := make(chan error, 1)
	go func() { done <- hijo.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Error("el proceso siguió vivo tras el SIGTERM")
	}
}

// TestPararSirveConFicheroRayado: si el PID ya no existe (tumbaron la máquina,
// kill -9), stop no puede fallar: tiene que limpiar y decirlo.
func TestPararSirveConFicheroRayado(t *testing.T) {
	dir := t.TempDir()
	rutaPID := filepath.Join(dir, "shin.pid")
	// Un PID que no existe: los números altos del rango de pruebas casi seguro
	// están libres; si no, se busca uno libre.
	fantasma := 999_998
	for procesoVivo(fantasma) && fantasma > 60_000 {
		fantasma--
	}
	if procesoVivo(fantasma) {
		t.Skip("no se encontró un PID libre para la prueba")
	}
	if err := os.WriteFile(rutaPID, []byte(strconv.Itoa(fantasma)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	salida, codigo := capturar(t, func() int { return pararServicio(rutaPID) })
	if codigo != 0 {
		t.Errorf("stop con pid muerto debe salir 0 (no hay nada que parar), salió %d", codigo)
	}
	if !strings.Contains(salida, "ya no existe") {
		t.Errorf("hay que decir que el pid no existe: %q", salida)
	}
	if _, err := os.Stat(rutaPID); !os.IsNotExist(err) {
		t.Error("el fichero de un proceso muerto no puede quedarse")
	}
}

func TestEstadoSinFicheroDiceParado(t *testing.T) {
	dir := t.TempDir()
	salida, codigo := capturar(t, func() int {
		return estadoServicio(filepath.Join(dir, "shin.pid"), filepath.Join(dir, "shin.log"))
	})
	if codigo != 1 {
		t.Errorf("«status» de algo parado debe salir con 1 (para usarlo en scripts), salió %d", codigo)
	}
	if !strings.Contains(salida, "parado") {
		t.Errorf("debe decirlo con la palabra: %q", salida)
	}
}

func TestEstadoConProcesoVivoLoMuestra(t *testing.T) {
	dir := t.TempDir()
	rutaPID := filepath.Join(dir, "shin.pid")
	log := filepath.Join(dir, "shin.log")
	hijo := execSleep(t)
	defer func() { _ = hijo.Process.Kill(); _ = hijo.Wait() }()

	if err := os.WriteFile(rutaPID, []byte(strconv.Itoa(hijo.Process.Pid)+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(log, []byte("12:00:01 una línea\n12:00:02 otra línea\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	salida, codigo := capturar(t, func() int { return estadoServicio(rutaPID, log) })
	if codigo != 0 {
		t.Errorf("status de algo vivo sale 0, salió %d", codigo)
	}
	for _, trozo := range []string{"en ejecución", strconv.Itoa(hijo.Process.Pid), "otra línea"} {
		if !strings.Contains(salida, trozo) {
			t.Errorf("el estado debe contener %q\n%s", trozo, salida)
		}
	}
	// Y sin mentir: «status» no puede afirmar que WhatsApp está conectado.
	if strings.Contains(salida, "conectado") {
		t.Error("«status» no puede prometer la conexión de WhatsApp: eso lo sabe el bot")
	}
}

func TestUltimasLineasDesdeElFinal(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "shin.log")
	var b strings.Builder
	for i := 0; i < 4000; i++ {
		b.WriteString("línea " + strconv.Itoa(i) + " con algo de texto para que el fichero pese\n")
	}
	if err := os.WriteFile(log, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	lineas, err := ultimasLineas(log, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(lineas) != 3 || lineas[2] != "línea 3999 con algo de texto para que el fichero pese" {
		t.Errorf("últimas 3 = %q", lineas)
	}
	if lineas, _ := ultimasLineas(log, 1); len(lineas) != 1 {
		t.Errorf("con n=1 tiene que dar una: %q", lineas)
	}
	// Un log recién creado (o vacío) no puede dar pánico.
	vacio := filepath.Join(dir, "vacio.log")
	if err := os.WriteFile(vacio, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if lineas, err := ultimasLineas(vacio, 10); err != nil || len(lineas) > 1 {
		t.Errorf("log vacío: %q / %v", lineas, err)
	}
	if _, err := ultimasLineas(filepath.Join(dir, "no-esta.log"), 5); err == nil {
		t.Error("sin fichero tiene que devolver error (lo cuenta la orden)")
	}
}

// ─── utilidades de la prueba ──────────────────────────────────────────────

// execSleep: un proceso real y aburrido, que se deja matar con SIGTERM. Se
// usa «sleep» del sistema: si no está (un CI mínimo), la prueba se salta en vez
// de mentir con un proceso de juguete.
func execSleep(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip("no se pudo levantar «sleep» para la prueba: " + err.Error())
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	return cmd
}
