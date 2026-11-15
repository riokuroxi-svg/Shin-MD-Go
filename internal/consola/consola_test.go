// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// consola_test.go — el banner y la caja de listo: que digan lo suyo y que
// las líneas de la caja salgan todas del mismo ancho visible (con códigos
// ANSI metidos, que no cuentan).
package consola

import (
	"strings"
	"testing"
)

func TestHumanizar(t *testing.T) {
	casos := []struct {
		in     uint64
		quiere string
	}{
		{2*1024*1024*1024 + 600*1024*1024, "2.6G"}, // el «2.6G» del JS
		{7 * 1024 * 1024 * 1024, "7G"},
		{512 * 1024 * 1024, "512M"},
		{12 * 1024, "12K"},
	}
	for _, c := range casos {
		if got := Humanizar(c.in); got != c.quiere {
			t.Errorf("Humanizar(%d) = %q, quería %q", c.in, got, c.quiere)
		}
	}
}

func TestBannerDiceLoSuyo(t *testing.T) {
	b := Banner("0.19.0")
	for _, trozo := range []string{"S H I N — M D", "反魂", "0.19.0", "riokuroxi-svg", "AGPL-3.0-only", "go1."} {
		if !strings.Contains(b, trozo) {
			t.Fatalf("el banner debe contener %q\n%s", trozo, b)
		}
	}
}

func TestLargoDescuentaANSI(t *testing.T) {
	if n := largo("\033[38;5;205m●\033[0m ACTIVO"); n != 8 { // «● ACTIVO»: 8 runas visibles
		t.Fatalf("largo con ANSI = %d, quería 8", n)
	}
}

func TestPanelListoCajaPareja(t *testing.T) {
	var b strings.Builder
	PanelListo(&b, Listo{
		Version: "0.19.0", Cuenta: "Fulano", Comandos: 213,
		Prefijos: []string{"."}, Web: "http://127.0.0.1:3000", PID: 1234,
	})
	lineas := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if len(lineas) < 4 {
		t.Fatalf("panel demasiado corto:\n%s", b.String())
	}
	ancho := -1
	for i, l := range lineas {
		n := largo(l)
		if ancho == -1 {
			ancho = n
		}
		if n != ancho {
			t.Fatalf("línea %d con ancho %d ≠ %d: %q", i, n, ancho, l)
		}
	}
	s := b.String()
	for _, trozo := range []string{"ACTIVO", "Fulano", "213 comandos", "[.]", "127.0.0.1:3000", "┌", "└"} {
		if !strings.Contains(s, trozo) {
			t.Fatalf("el panel debe contener %q\n%s", trozo, s)
		}
	}
	// Vitales presentes en este sandbox (linux): disco y ram con letra G/M.
	if !strings.Contains(s, "disco ") || !strings.Contains(s, "ram ") {
		t.Fatalf("el panel debe llevar vitales:\n%s", s)
	}
}
