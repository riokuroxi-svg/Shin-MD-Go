// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package musica

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func ejecutablesSpotDLDePrueba(t *testing.T) (spotdl, ffmpeg, argsLog string) {
	t.Helper()
	dir := t.TempDir()
	spotdl = filepath.Join(dir, "spotdl")
	ffmpeg = filepath.Join(dir, "ffmpeg")
	argsLog = filepath.Join(dir, "args.log")

	script := `#!/bin/sh
set -eu
: "${SPOTDL_ARGS_LOG:?}"
printf '%s\n' "$@" > "$SPOTDL_ARGS_LOG"
for arg in "$@"; do
  if [ "$arg" = "--version" ]; then
    printf '4.6.0\n'
    exit 0
  fi
  if [ "$arg" = "dormir" ]; then
    while :; do :; done
  fi
done
mode=''
out=''
prev=''
for arg in "$@"; do
  [ "$arg" = 'save' ] && mode='save'
  [ "$arg" = 'download' ] && mode='download'
  if [ "$prev" = '--output' ]; then out="$arg"; fi
  prev="$arg"
done
if [ "$mode" = 'save' ]; then
  cat <<'JSON'
Processing query: prueba
[
  {"name":"Canción de prueba","artists":["Rin"],"artist":"Rin","album_name":"Álbum","duration":201,"song_id":"1234567890ABCDEFGHIJKL","cover_url":"https://img.example/cover.jpg","url":"https://open.spotify.com/track/1234567890ABCDEFGHIJKL?si=tracking"},
  {"name":"Segunda","artists":["Nube"],"duration":180,"song_id":"ABCDEFGHIJKL1234567890","url":"https://open.spotify.com/track/ABCDEFGHIJKL1234567890"}
]
JSON
  exit 0
fi
if [ "$mode" = 'download' ]; then
  out=$(printf '%s' "$out" | sed 's/{output-ext}/mp3/g')
  mkdir -p "$(dirname "$out")"
  printf 'ID3audio-de-prueba' > "$out"
  printf 'Canción de prueba: Done\n'
  exit 0
fi
printf 'modo desconocido\n' >&2
exit 2
`
	if err := os.WriteFile(spotdl, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ffmpeg, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SPOTDL_ARGS_LOG", argsLog)
	return
}

func TestSpotDLBuscaSinShellYNormalizaPistas(t *testing.T) {
	binary, ffmpeg, argsLog := ejecutablesSpotDLDePrueba(t)
	cookie := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(cookie, []byte("# Netscape HTTP Cookie File\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c := Nuevo(Opciones{})
	if err := c.UsarSpotDL(OpcionesSpotDL{
		Path: binary, FFmpeg: ffmpeg, CookieFile: cookie,
		Audio: "youtube-music,soundcloud,youtube-music", Timeout: 2 * time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	if !c.TieneSpotDL() {
		t.Fatal("spotDL debía quedar habilitado")
	}
	if version, err := c.VersionSpotDL(context.Background()); err != nil || version != "4.6.0" {
		t.Fatalf("versión = %q, err=%v", version, err)
	}

	marker := filepath.Join(t.TempDir(), "no-debe-existir")
	query := "tema; touch " + marker
	pistas, err := c.BuscarSpotDL(context.Background(), query, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("la consulta se interpretó como shell: stat=%v", err)
	}
	if len(pistas) != 1 {
		t.Fatalf("pistas = %d; want 1 por el límite", len(pistas))
	}
	p := pistas[0]
	if p.Fuente != FuenteSpotDL || p.Backend != BackendSpotDL || !p.Descargable() {
		t.Fatalf("pista spotDL inválida: %+v", p)
	}
	if p.Origen != "https://open.spotify.com/track/1234567890ABCDEFGHIJKL" || p.Entrada != p.Origen {
		t.Fatalf("URL canónica = %q / entrada %q", p.Origen, p.Entrada)
	}
	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	logged := string(args)
	if !strings.Contains(logged, query+"\n") {
		t.Fatalf("la consulta no llegó como un argumento íntegro:\n%s", args)
	}
	if strings.Count(logged, "youtube-music\n") != 1 || !strings.Contains(logged, "soundcloud\n") ||
		!strings.Contains(logged, cookie+"\n") {
		t.Fatalf("opciones spotDL incompletas o duplicadas:\n%s", args)
	}
	audioAt := strings.Index(logged, "--audio\n")
	lastProviderAt := strings.Index(logged, "soundcloud\n")
	ffmpegAt := strings.Index(logged, "--ffmpeg\n")
	if audioAt < 0 || lastProviderAt <= audioAt || ffmpegAt <= lastProviderAt {
		t.Fatalf("--ffmpeg debe cerrar la lista variable de --audio:\n%s", args)
	}
}

func TestSpotDLDescargaMP3ConTope(t *testing.T) {
	binary, ffmpeg, _ := ejecutablesSpotDLDePrueba(t)
	c := Nuevo(Opciones{})
	if err := c.UsarSpotDL(OpcionesSpotDL{Path: binary, FFmpeg: ffmpeg, Timeout: 2 * time.Second}); err != nil {
		t.Fatal(err)
	}
	p := Pista{
		Titulo:   "Canción de prueba",
		Duracion: 201,
		Backend:  BackendSpotDL,
		Entrada:  "https://open.spotify.com/track/1234567890ABCDEFGHIJKL",
	}
	var out bytes.Buffer
	n, err := c.Descargar(context.Background(), p, &out)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(out.Len()) || !bytes.HasPrefix(out.Bytes(), []byte("ID3")) {
		t.Fatalf("salida = %d bytes, cabecera %q", n, out.Bytes())
	}

	p.Duracion = maxSpotDLTrackSeconds + 1
	if _, err := c.Descargar(context.Background(), p, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "20:00") {
		t.Fatalf("una pista enorme debía rechazarse: %v", err)
	}
	p.Duracion = 201
	p.Entrada = "https://example.com/audio.mp3"
	if _, err := c.Descargar(context.Background(), p, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "no canónica") {
		t.Fatalf("una entrada externa debía rechazarse: %v", err)
	}
}

func TestSpotDLRespetaPlazoYConfiguracion(t *testing.T) {
	binary, ffmpeg, _ := ejecutablesSpotDLDePrueba(t)
	c := Nuevo(Opciones{})
	if err := c.UsarSpotDL(OpcionesSpotDL{Path: binary, FFmpeg: ffmpeg, Timeout: 50 * time.Millisecond}); err != nil {
		t.Fatal(err)
	}
	inicio := time.Now()
	if _, err := c.BuscarSpotDL(context.Background(), "dormir", 1); err == nil || !errors.Is(err, ErrSpotDLNoDisponible) {
		t.Fatalf("debía vencer el proceso spotDL: %v", err)
	}
	if time.Since(inicio) > time.Second {
		t.Fatalf("el proceso no se canceló a tiempo: %s", time.Since(inicio))
	}

	if err := Nuevo(Opciones{}).UsarSpotDL(OpcionesSpotDL{Path: filepath.Join(t.TempDir(), "falta"), FFmpeg: ffmpeg, Timeout: time.Second}); !errors.Is(err, ErrSpotDLNoDisponible) {
		t.Fatalf("ejecutable ausente = %v", err)
	}
}

func TestSpotDLOpcionesRechazanProveedorYCookiesInvalidos(t *testing.T) {
	binary, ffmpeg, _ := ejecutablesSpotDLDePrueba(t)
	c := Nuevo(Opciones{})
	if err := c.UsarSpotDL(OpcionesSpotDL{Path: binary, FFmpeg: ffmpeg}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.BuscarSpotDL(context.Background(), "--cookie-file=/tmp/ajeno", 1); err == nil ||
		!strings.Contains(err.Error(), "comenzar con guion") {
		t.Fatalf("una consulta que parece bandera debía rechazarse: %v", err)
	}
	if err := c.UsarSpotDL(OpcionesSpotDL{Path: binary, FFmpeg: ffmpeg, Audio: "sitio-desconocido"}); !errors.Is(err, ErrSpotDLNoDisponible) {
		t.Fatalf("proveedor desconocido = %v", err)
	}
	if c.TieneSpotDL() {
		t.Fatal("una reconfiguración inválida debía deshabilitar el backend anterior")
	}
	if err := c.UsarSpotDL(OpcionesSpotDL{Path: binary, FFmpeg: ffmpeg, CookieFile: filepath.Join(t.TempDir(), "falta")}); !errors.Is(err, ErrSpotDLNoDisponible) {
		t.Fatalf("cookies ausentes = %v", err)
	}
}

func TestParseSpotDLRechazaSalidaInvalida(t *testing.T) {
	if _, err := parseSpotDLSongs("solo logs, sin JSON"); err == nil {
		t.Fatal("una salida sin JSON debía fallar")
	}
	if got := canonicalSpotifyTrackURL("https://open.spotify.com/playlist/abc", ""); got != "" {
		t.Fatalf("una playlist no es una pista canónica: %q", got)
	}
	if got := canonicalSpotifyTrackURL("", "1234567890ABCDEFGHIJK"); got != "" {
		t.Fatalf("un ID que no tenga 22 caracteres no es canónico: %q", got)
	}
}
