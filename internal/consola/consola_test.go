// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// consola_test.go — lo que el banner y la caja al pterm deben decir, y la
// garantía de las pantallas estrechas: envolver jamás entrega una línea más
// larga que el límite. En CI no hay terminal con color: el banner sale en
// su forma compacta, la caja igual de legible, y eso es lo que se mide.
package consola

import (
	"strings"
	"testing"
)

// largo: runas visibles de un string (los códigos ANSI no cuentan); lo usan
// las pruebas para medir líneas con color ya metido.
func largo(s string) int {
	n := 0
	dentro := false
	for _, r := range s {
		switch {
		case r == '\033':
			dentro = true
		case dentro && r == 'm':
			dentro = false
		case !dentro:
			n++
		}
	}
	return n
}

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
	b := Banner("0.20.0")
	// En CI (sin color) el título compacto; la parte invariable se exige aquí.
	for _, trozo := range []string{"S H I N", "反魂", "0.20.0", "riokuroxi-svg", "AGPL-3.0-only", "go1."} {
		if !strings.Contains(b, trozo) {
			t.Fatalf("el banner debe contener %q\n%s", trozo, b)
		}
	}
}

func TestLargoDescuentaANSI(t *testing.T) {
	if n := largo("\x1b[38;5;205m●\x1b[0m ACTIVO"); n != 8 { // «● ACTIVO»: 8 runas visibles
		t.Fatalf("largo con ANSI = %d, quería 8", n)
	}
}

func TestEnvolverNoDesborda(t *testing.T) {
	largo := strings.Repeat("palabrajuna ", 30)
	for _, limite := range []int{24, 40, 68, 80} {
		for _, l := range envolver(largo, limite) {
			if n := len([]rune(l)); n > limite {
				t.Fatalf("envolver(%d) dio línea de %d runas: %q", limite, n, l)
			}
		}
	}
}

func TestPanelListoDiceLoSuyo(t *testing.T) {
	var b strings.Builder
	PanelListo(&b, Listo{
		Version: "0.20.0", Cuenta: "Fulano", Comandos: 213,
		Prefijos: []string{"."}, Web: "http://127.0.0.1:3000", PID: 1234,
	})
	s := b.String()
	for _, trozo := range []string{"ACTIVO", "Fulano", "213", "[.]", "127.0.0.1:3000", "│", "SHIN-MD-GO 0.20.0"} {
		if !strings.Contains(s, trozo) {
			t.Fatalf("el panel debe contener %q\n%s", trozo, s)
		}
	}
	// Vitales presentes en este sandbox (linux): disco y ram con barra.
	if !strings.Contains(s, "▓") || !strings.Contains(s, "ram") {
		t.Fatalf("el panel debe llevar vitales con barras:\n%s", s)
	}
	// Y ninguna línea desborda el ancho de esta terminal (o sus 80 por omisión).
	for i, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if n := largo(l); n > 80 { // en CI Ancho() cae a 80; nunca más que eso
			t.Fatalf("línea %d con %d runas visibles, máximo 80: %q", i, n, l)
		}
	}
}
