// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// descarga_test.go — B6.1 sin yt-dlp real ni red: se apunta el binario a un
// script en sh que finge ser yt-dlp (un MP4 de juguete, un título, y los
// errores conocidos) y se comprueban el tope, los motivos y la traducción.
package descarga

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ytFalso deja en PATH un "yt-dlp" de mentira con el guion dado y devuelve
// el directorio temporal donde escribe. El guion recibe la URL la última.
func ytFalso(t *testing.T, guion string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"+guion), 0o755); err != nil {
		t.Fatal(err)
	}
	prev := Ytdlp
	Ytdlp = bin
	t.Cleanup(func() { Ytdlp = prev })
	return dir
}

// yt-dlp de verdad imprime con --print: filepath y luego title.
const guionOK = `
for a in "$@"; do URL="$a"; done
# plantilla: descarga-%(id)s.%(ext)s — el %(…) se resuelve aquí a mano
f="descarga-qwerty.mp4"
mkdir -p "$(pwd)"
printf 'PKfake-mp4-contenido' > "$(pwd)/$f"
echo "$(pwd)/$f"
echo "Gatos volando (canal prueba)"
exit 0
`

func TestBajarVideoOK(t *testing.T) {
	ytFalso(t, guionOK)
	res, err := Bajar(context.Background(), "https://youtu.be/qwerty", t.TempDir(), "video")
	if err != nil {
		t.Fatalf("Bajar: %v", err)
	}
	if res.Titulo != "Gatos volando (canal prueba)" {
		t.Fatalf("título: %q", res.Titulo)
	}
	datos, err := res.Leer()
	if err != nil || !strings.Contains(string(datos), "fake-mp4") {
		t.Fatalf("Leer: %v %q", err, datos)
	}
	if _, err := os.Stat(res.Archivo); !os.IsNotExist(err) {
		t.Fatal("Leer debía borrar el temporal")
	}
}

func TestSinBinario(t *testing.T) {
	prev := Ytdlp
	Ytdlp = "yt-dlp-que-no-existe-para-este-test"
	t.Cleanup(func() { Ytdlp = prev })
	_, err := Bajar(context.Background(), "https://x", t.TempDir(), "video")
	if !errors.Is(err, ErrNoYtdlp) {
		t.Fatalf("sin binario debía ser ErrNoYtdlp: %v", err)
	}
}

func TestBotCheckTraducido(t *testing.T) {
	ytFalso(t, `echo "ERROR: Sign in to confirm you're not a bot" >&2; exit 1`)
	_, err := Bajar(context.Background(), "https://youtu.be/x", t.TempDir(), "video")
	if !errors.Is(err, ErrBotCheck) {
		t.Fatalf("bot check debía ser ErrBotCheck: %v", err)
	}
}

func TestPrivado(t *testing.T) {
	ytFalso(t, `echo "ERROR: Private video. Sign in if you've been granted access" >&2; exit 1`)
	_, err := Bajar(context.Background(), "https://youtu.be/x", t.TempDir(), "video")
	if err == nil || !strings.Contains(err.Error(), "privado") {
		t.Fatalf("privado: %v", err)
	}
}

func TestDemasiadoGrande(t *testing.T) {
	// Marca el límite: archivo mayor a 100 MB. No generamos 100 MB de verdad:
	// se crea sparse con truncate para que el test sea instantáneo.
	ytFalso(t, `
f="descarga-big.mp4"
truncate -s 105M "$(pwd)/$f" 2>/dev/null || dd if=/dev/zero of="$(pwd)/$f" bs=1M count=105 2>/dev/null
echo "$(pwd)/$f"
echo "enorme"
exit 0
`)
	_, err := Bajar(context.Background(), "https://youtu.be/big", t.TempDir(), "video")
	if err == nil || !strings.Contains(err.Error(), "tope") {
		t.Fatalf("grande: %v", err)
	}
}

func TestPlazoDuro(t *testing.T) {
	ytFalso(t, `sleep 60`)
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	inicio := time.Now()
	_, err := Bajar(ctx, "https://youtu.be/lento", t.TempDir(), "video")
	if err == nil || !strings.Contains(err.Error(), "5 minutos") {
		t.Fatalf("plazo: %v", err)
	}
	if time.Since(inicio) > 10*time.Second {
		t.Fatalf("la cancelación tardó demasiado: %v", time.Since(inicio))
	}
}

// Windows no tendría /bin/sh; el test del wrapper solo corre donde hay sh.
func TestMain(m *testing.M) {
	if runtime.GOOS == "windows" {
		os.Exit(0)
	}
	os.Exit(m.Run())
}
