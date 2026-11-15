// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// archivo_test.go — la rotación del log: se rota al pasar del tope, se
// conservan solo N backups y un archivo viejo heredado se rota al abrir.
package xlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotativoRota(t *testing.T) {
	dir := t.TempDir()
	nombre := filepath.Join(dir, "shin.log")
	r, err := AbrirRotativo(nombre, 100, 3)
	if err != nil {
		t.Fatalf("AbrirRotativo: %v", err)
	}
	defer r.Cerrar()

	// 4 escrituras de 60 bytes: a la segunda se rota.
	linea := strings.Repeat("x", 59) + "\n"
	for i := 0; i < 4; i++ {
		if _, err := r.Write([]byte(linea)); err != nil {
			t.Fatalf("Write %d: %v", i, err)
		}
	}
	if _, err := os.Stat(nombre + ".1"); err != nil {
		t.Fatal("tras pasar el tope debe existir shin.log.1")
	}
	if r.TamanoActual() >= 100 {
		t.Fatalf("el archivo nuevo debe empezar vacío, no con %d bytes", r.TamanoActual())
	}
}

func TestRotativoTopeBackups(t *testing.T) {
	dir := t.TempDir()
	nombre := filepath.Join(dir, "shin.log")
	r, err := AbrirRotativo(nombre, 40, 2)
	if err != nil {
		t.Fatalf("AbrirRotativo: %v", err)
	}
	defer r.Cerrar()

	linea := strings.Repeat("y", 50) + "\n" // cada línea fuerza una rotación
	for i := 0; i < 6; i++ {
		_, _ = r.Write([]byte(linea))
	}
	if _, err := os.Stat(nombre + ".2"); err != nil {
		t.Fatal("debía existir .2")
	}
	if _, err := os.Stat(nombre + ".3"); err == nil {
		t.Fatal("con 2 backups NO debe existir .3 (el más viejo se borra)")
	}
}

func TestRotativoHeredadoGrande(t *testing.T) {
	dir := t.TempDir()
	nombre := filepath.Join(dir, "shin.log")
	if err := os.WriteFile(nombre, []byte(strings.Repeat("z", 5000)), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := AbrirRotativo(nombre, 100, 3)
	if err != nil {
		t.Fatalf("AbrirRotativo: %v", err)
	}
	defer r.Cerrar()
	if r.TamanoActual() != 0 {
		t.Fatalf("un archivo heredado > tope se rota al abrir: escrito = %d", r.TamanoActual())
	}
	if _, err := os.Stat(nombre + ".1"); err != nil {
		t.Fatal("el tronco heredado debía pasar a .1")
	}
}
