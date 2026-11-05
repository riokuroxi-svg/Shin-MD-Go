// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package config

import (
	"os"
	"path/filepath"
	"testing"
)

// Un .env con comentarios, comillas y export tiene que leerse igual que en el
// bot de JS: si esto falla, el número arranca sin owners y sin prefijos.
func TestLoadFromDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := `# configuración de Shin-MD
BOT_PREFIX=".,/,!"
OWNERS=+525555555555
LOG_LEVEL=debug
STATE_DIR=demo
ANTIBAN_BASE_MS=900
ANTIBAN_SIGMA=0.3
WARMUP_START=40
WARMUP_END=300
RICH_AI=1
esta línea no es una variable
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := LoadFrom(path)

	if cfg.EnvFile != path {
		t.Fatalf("EnvFile = %q", cfg.EnvFile)
	}
	if len(cfg.Prefixes) != 3 || cfg.Prefixes[0] != "." || cfg.Prefixes[2] != "!" {
		t.Fatalf("prefijos = %v", cfg.Prefixes)
	}
	if cfg.StateDir != "demo" || filepath.Base(cfg.DBFile) != "shin.db" {
		t.Fatalf("rutas = %q / %q", cfg.StateDir, cfg.DBFile)
	}
	if cfg.LogLevel != "debug" || !cfg.RichAI {
		t.Fatalf("nivel = %q rich = %v", cfg.LogLevel, cfg.RichAI)
	}
	if cfg.BaseDelay.Milliseconds() != 900 || cfg.JitterSigma != 0.3 {
		t.Fatalf("anti-ban = %s / %v", cfg.BaseDelay, cfg.JitterSigma)
	}
	if cfg.WarmupStart != 40 || cfg.WarmupEnd != 300 {
		t.Fatalf("warm-up = %d→%d", cfg.WarmupStart, cfg.WarmupEnd)
	}
}

// Sin .env todo tiene que caer en los valores por defecto y el bot arrancar.
func TestLoadDefaultsNoEnvFile(t *testing.T) {
	cfg := LoadFrom(filepath.Join(t.TempDir(), "no-existe.env"))

	if cfg.EnvFile != "" {
		t.Fatalf("EnvFile = %q, se esperaba vacío", cfg.EnvFile)
	}
	if len(cfg.Prefixes) != 4 || filepath.Join("data", "auth.db") != cfg.AuthFile {
		t.Fatalf("defaults = %v / %q", cfg.Prefixes, cfg.AuthFile)
	}
	if cfg.BaseDelay.Milliseconds() != 1200 || cfg.WarmupStart != 20 || cfg.WarmupEnd != 500 {
		t.Fatalf("anti-ban por defecto = %s %d→%d", cfg.BaseDelay, cfg.WarmupStart, cfg.WarmupEnd)
	}
}

// Valores imposibles en el .env no deben romper el motor anti-ban.
func TestLoadClampsAbsurdValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "ANTIBAN_BASE_MS=10\nANTIBAN_SIGMA=-2\nWARMUP_START=0\nWARMUP_END=5\nBOT_PREFIX=,,\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := LoadFrom(path)
	if cfg.BaseDelay.Milliseconds() != 400 {
		t.Fatalf("delay = %s, se esperaba el suelo de 400 ms", cfg.BaseDelay)
	}
	if cfg.JitterSigma != 0 {
		t.Fatalf("sigma = %v", cfg.JitterSigma)
	}
	if cfg.WarmupStart != 1 || cfg.WarmupEnd != 5 { // 0 → 1 (suelo), el final se respeta
		t.Fatalf("warm-up = %d→%d", cfg.WarmupStart, cfg.WarmupEnd)
	}
	if len(cfg.Prefixes) != 1 || cfg.Prefixes[0] != "." {
		t.Fatalf("prefijos = %v", cfg.Prefixes)
	}
}

// El entorno del sistema gana sobre el archivo (así `OWNERS=… ./shin` sirve).
func TestEnvOverridesDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("STATE_DIR=archivo\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("STATE_DIR", "entorno")
	if cfg := LoadFrom(path); cfg.StateDir != "entorno" {
		t.Fatalf("StateDir = %q, el entorno debería ganar", cfg.StateDir)
	}

	t.Setenv("STATE_DIR", "")
	if cfg := LoadFrom(path); cfg.StateDir != "archivo" {
		t.Fatalf("StateDir = %q, debería venir del .env", cfg.StateDir)
	}
}

// Los owners del .env tienen que llegar al bot. Antes main() los leía directo
// del entorno del sistema, así que ponerlos en el .env no servía de nada.
func TestOwnersFromDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("OWNERS=\"+52 155 1234 5678, 5219999999999;5218888888888\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := LoadFrom(path)
	// "+52 155 1234 5678" es UN número (con espacios), no cuatro.
	want := []string{"5215512345678", "5219999999999", "5218888888888"}
	if len(cfg.Owners) != len(want) {
		t.Fatalf("owners = %v", cfg.Owners)
	}
	for i, w := range want {
		if cfg.Owners[i] != w {
			t.Fatalf("owners = %v, se esperaba %v", cfg.Owners, want)
		}
	}

	// Lista separada solo por espacios, con números completos: sí son dos.
	otro := filepath.Join(dir, "dos.env")
	if err := os.WriteFile(otro, []byte("OWNERS=5211111111111 5222222222222\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := LoadFrom(otro).Owners; len(got) != 2 || got[0] != "5211111111111" {
		t.Fatalf("lista por espacios = %v", got)
	}

	if got := LoadFrom(filepath.Join(dir, "sin.env")).Owners; len(got) != 0 {
		t.Fatalf("sin .env debería haber 0 owners, hay %v", got)
	}
}
