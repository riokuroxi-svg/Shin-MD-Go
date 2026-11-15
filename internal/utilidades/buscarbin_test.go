// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// buscarbin_test.go — BuscarBin recorre el PATH sin faccessat2.
package utilidades

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBuscarBinEncuentraEjecutable(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "falso-sh")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	plano := filepath.Join(dir, "notas.txt")
	if err := os.WriteFile(plano, []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	p, err := BuscarBin("falso-sh")
	if err != nil {
		t.Fatalf("debió encontrarlo: %v", err)
	}
	if p != bin {
		t.Fatalf("ruta rara: %q y esperaba %q", p, bin)
	}
	if !HayBin("falso-sh") {
		t.Fatal("HayBin dijo que no habiendo")
	}
}

func TestBuscarBinRechazaNoEjecutableYAusente(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plano"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	if _, err := BuscarBin("plano"); !errors.Is(err, ErrNoBinario) {
		t.Fatalf("archivo sin bit x debió dar ErrNoBinario, dio %v", err)
	}
	if _, err := BuscarBin("no-existo-para-nada"); !errors.Is(err, ErrNoBinario) {
		t.Fatalf("ausente debió dar ErrNoBinario, dio %v", err)
	}
	if HayBin("plano") || HayBin("no-existo-para-nada") {
		t.Fatal("HayBin mintió")
	}
}

func TestBuscarBinRutaDirecta(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "mio")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "") // sin PATH: la ruta directa no lo necesita
	p, err := BuscarBin(bin)
	if err != nil || p != bin {
		t.Fatalf("ruta directa: p=%q err=%v", p, err)
	}
	if _, err := BuscarBin("./nada-por-aqui"); !errors.Is(err, ErrNoBinario) {
		t.Fatalf("ruta mala debió dar ErrNoBinario, dio %v", err)
	}
}

func TestBuscarBinShDelSistema(t *testing.T) {
	// El propio bot asume que existe un sh: si ni eso aparece, el entorno de
	// pruebas está roto y conviene enterarse en voz alta.
	if _, err := BuscarBin("sh"); err != nil {
		t.Fatalf("no hay sh en el PATH del entorno de pruebas: %v", err)
	}
}
