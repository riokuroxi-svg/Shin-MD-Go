// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// spotdl.go — adaptador opcional al ejecutable spotDL. No usa shell, no baja
// binarios por su cuenta y no convierte Node en dependencia del bot.
package musica

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	FuenteSpotDL          = "spotdl"
	BackendSpotDL         = "spotdl"
	maxSpotDLOutput       = 4 << 20
	maxSpotDLQueryRunes   = 500
	maxSpotDLTrackSeconds = 20 * 60
)

var (
	ErrSpotDLNoConfigurado = errors.New("musica: spotDL no está configurado")
	ErrSpotDLNoDisponible  = errors.New("musica: spotDL no está disponible")
	ansiSpotDL             = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)
	spotifyTrackID         = regexp.MustCompile(`^[A-Za-z0-9]{22}$`)
)

type spotDL struct {
	path     string
	timeout  time.Duration
	baseArgs []string
}

type OpcionesSpotDL struct {
	Path       string
	FFmpeg     string
	CookieFile string
	Audio      string
	Timeout    time.Duration
}

type spotDLSong struct {
	Name      string   `json:"name"`
	Artists   []string `json:"artists"`
	Artist    string   `json:"artist"`
	AlbumName string   `json:"album_name"`
	Duration  int      `json:"duration"`
	SongID    string   `json:"song_id"`
	CoverURL  string   `json:"cover_url"`
	URL       string   `json:"url"`
}

// UsarSpotDL habilita el proveedor externo. Tanto spotDL como ffmpeg deben
// existir ya: el bot no descarga ejecutables ni ejecuta instaladores durante
// el arranque.
func (c *Cliente) UsarSpotDL(options OpcionesSpotDL) error {
	// Una reconfiguración fallida no debe dejar activas rutas o cookies de la
	// configuración anterior.
	c.spotdl = nil
	path := strings.TrimSpace(options.Path)
	if path == "" {
		return nil
	}

	resolved, err := exec.LookPath(path)
	if err != nil {
		return fmt.Errorf("%w: no se encontró %q", ErrSpotDLNoDisponible, path)
	}
	ffmpeg := strings.TrimSpace(options.FFmpeg)
	if ffmpeg == "" {
		ffmpeg = "ffmpeg"
	}
	resolvedFFmpeg, err := exec.LookPath(ffmpeg)
	if err != nil {
		return fmt.Errorf("%w: no se encontró ffmpeg (%q)", ErrSpotDLNoDisponible, ffmpeg)
	}
	if options.Timeout <= 0 {
		options.Timeout = 5 * time.Minute
	}

	baseArgs := []string{"--simple-tui", "--headless"}
	providers, err := spotDLAudioProviders(options.Audio)
	if err != nil {
		return err
	}
	if len(providers) > 0 {
		baseArgs = append(baseArgs, "--audio")
		baseArgs = append(baseArgs, providers...)
	}
	// --audio tiene nargs variable en argparse. Una opción de valor fijo debe
	// ir después para que no intente consumir `save`/`download` como proveedor.
	baseArgs = append(baseArgs, "--ffmpeg", resolvedFFmpeg)
	if cookie := strings.TrimSpace(options.CookieFile); cookie != "" {
		cookie, err = filepath.Abs(cookie)
		if err != nil {
			return fmt.Errorf("%w: ruta de cookies inválida", ErrSpotDLNoDisponible)
		}
		info, statErr := os.Stat(cookie)
		if statErr != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("%w: archivo de cookies no disponible", ErrSpotDLNoDisponible)
		}
		baseArgs = append(baseArgs, "--cookie-file", cookie)
	}

	c.spotdl = &spotDL{
		path:     resolved,
		timeout:  options.Timeout,
		baseArgs: baseArgs,
	}
	return nil
}

func spotDLAudioProviders(raw string) ([]string, error) {
	allowed := map[string]bool{
		"youtube":       true,
		"youtube-music": true,
		"soundcloud":    true,
		"bandcamp":      true,
		"piped":         true,
	}
	var providers []string
	seen := map[string]bool{}
	for _, provider := range strings.FieldsFunc(strings.ToLower(raw), func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	}) {
		if !allowed[provider] {
			return nil, fmt.Errorf("%w: proveedor de audio spotDL desconocido %q", ErrSpotDLNoDisponible, provider)
		}
		if !seen[provider] {
			providers = append(providers, provider)
			seen[provider] = true
		}
	}
	return providers, nil
}

func (c *Cliente) TieneSpotDL() bool { return c != nil && c.spotdl != nil }

func (c *Cliente) VersionSpotDL(ctx context.Context) (string, error) {
	if !c.TieneSpotDL() {
		return "", ErrSpotDLNoConfigurado
	}
	stdout, _, err := c.spotdl.run(ctx, 15*time.Second, "--version")
	if err != nil {
		return "", err
	}
	version := recortaSpotDL(stdout, 80)
	if version == "" {
		return "", fmt.Errorf("%w: versión vacía", ErrSpotDLNoDisponible)
	}
	return version, nil
}

// BuscarSpotDL resuelve texto o una URL de Spotify a pistas individuales. Una
// playlist puede traer muchas, pero el bot conserva solo el límite solicitado.
func (c *Cliente) BuscarSpotDL(ctx context.Context, query string, limit int) ([]Pista, error) {
	if !c.TieneSpotDL() {
		return nil, ErrSpotDLNoConfigurado
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("musica: consulta spotDL vacía")
	}
	if utf8.RuneCountInString(query) > maxSpotDLQueryRunes {
		return nil, fmt.Errorf("musica: consulta spotDL supera %d caracteres", maxSpotDLQueryRunes)
	}
	// spotDL usa argparse y no reconoce `--` como fin de opciones para este
	// argumento posicional. Rechazar un guion inicial impide que texto del chat
	// se convierta en una bandera del proceso.
	if strings.HasPrefix(query, "-") {
		return nil, errors.New("musica: la consulta spotDL no puede comenzar con guion")
	}
	if limit <= 0 || limit > 10 {
		limit = 5
	}

	metadataTimeout := c.spotdl.timeout
	if metadataTimeout > time.Minute {
		metadataTimeout = time.Minute
	}
	args := append([]string{}, c.spotdl.baseArgs...)
	args = append(args, "save", query, "--save-file", "-")
	stdout, _, err := c.spotdl.run(ctx, metadataTimeout, args...)
	if err != nil {
		return nil, err
	}
	songs, err := parseSpotDLSongs(stdout)
	if err != nil {
		return nil, err
	}

	pistas := make([]Pista, 0, min(limit, len(songs)))
	for _, song := range songs {
		if len(pistas) >= limit {
			break
		}
		trackURL := canonicalSpotifyTrackURL(song.URL, song.SongID)
		title := recortaSpotDL(song.Name, 180)
		if title == "" || trackURL == "" || song.Duration <= 0 || song.Duration > maxSpotDLTrackSeconds {
			continue
		}
		artist := recortaSpotDL(song.Artist, 160)
		if artist == "" {
			artist = recortaSpotDL(strings.Join(song.Artists, ", "), 160)
		}
		if artist == "" {
			artist = "Artista desconocido"
		}
		pistas = append(pistas, Pista{
			Fuente:   FuenteSpotDL,
			ID:       strings.TrimPrefix(trackURL, "https://open.spotify.com/track/"),
			Titulo:   title,
			Artista:  artist,
			Album:    recortaSpotDL(song.AlbumName, 180),
			Duracion: song.Duration,
			Caratula: safeSpotDLHTTPSURL(song.CoverURL),
			Origen:   trackURL,
			Backend:  BackendSpotDL,
			Entrada:  trackURL,
		})
	}
	return pistas, nil
}

func parseSpotDLSongs(output string) ([]spotDLSong, error) {
	for start := strings.IndexByte(output, '['); start >= 0; {
		var songs []spotDLSong
		dec := json.NewDecoder(strings.NewReader(output[start:]))
		if err := dec.Decode(&songs); err == nil {
			return songs, nil
		}
		next := strings.IndexByte(output[start+1:], '[')
		if next < 0 {
			break
		}
		start += next + 1
	}
	return nil, errors.New("musica: spotDL no devolvió un arreglo JSON válido")
}

func canonicalSpotifyTrackURL(raw, id string) string {
	if u, err := url.Parse(strings.TrimSpace(raw)); err == nil &&
		u.Scheme == "https" && strings.EqualFold(u.Hostname(), "open.spotify.com") {
		parts := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(parts) == 2 && parts[0] == "track" && spotifyTrackID.MatchString(parts[1]) {
			return "https://open.spotify.com/track/" + parts[1]
		}
	}
	id = strings.TrimSpace(id)
	if spotifyTrackID.MatchString(id) {
		return "https://open.spotify.com/track/" + id
	}
	return ""
}

func safeSpotDLHTTPSURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) > 2048 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return ""
	}
	return u.String()
}

func (s *spotDL) descargar(ctx context.Context, p Pista, w io.Writer) (int64, error) {
	if p.Backend != BackendSpotDL || p.Entrada == "" {
		return 0, ErrNoLibre
	}
	trackURL := canonicalSpotifyTrackURL(p.Entrada, p.ID)
	if trackURL == "" {
		return 0, errors.New("musica: spotDL rechazó una URL de pista no canónica")
	}
	if p.Duracion <= 0 {
		return 0, errors.New("musica: spotDL rechazó una pista sin duración verificable")
	}
	if p.Duracion > maxSpotDLTrackSeconds {
		return 0, fmt.Errorf("musica: spotDL rechazó una pista de %s; el máximo es 20:00", p.Minutos())
	}

	dir, err := os.MkdirTemp("", "shin-spotdl-")
	if err != nil {
		return 0, fmt.Errorf("musica: crear temporal spotDL: %w", err)
	}
	defer os.RemoveAll(dir)

	output := filepath.Join(dir, "audio.{output-ext}")
	args := append([]string{}, s.baseArgs...)
	args = append(args,
		"--threads", "1",
		"download", trackURL,
		"--format", "mp3",
		"--bitrate", "128k",
		"--overwrite", "force",
		"--output", output,
	)
	stdout, stderr, err := s.run(ctx, s.timeout, args...)
	if err != nil {
		return 0, err
	}

	path := filepath.Join(dir, "audio.mp3")
	info, statErr := os.Lstat(path)
	if statErr != nil {
		detail := strings.TrimSpace(stderr + "\n" + stdout)
		if detail == "" {
			detail = "spotDL terminó sin crear audio.mp3"
		}
		return 0, fmt.Errorf("%w: %s", ErrSpotDLNoDisponible, recortaSpotDL(detail, 400))
	}
	if !info.Mode().IsRegular() {
		return 0, errors.New("musica: la salida spotDL no es un archivo regular")
	}
	if info.Size() <= 0 || info.Size() > MaxAudio {
		return 0, fmt.Errorf("musica: el audio spotDL pesa %.1f MB y el tope es %d MB",
			float64(info.Size())/(1<<20), MaxAudio>>20)
	}

	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("musica: abrir salida spotDL: %w", err)
	}
	defer f.Close()

	header := make([]byte, 512)
	n, readErr := io.ReadFull(f, header)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return 0, fmt.Errorf("musica: leer salida spotDL: %w", readErr)
	}
	header = header[:n]
	if !pareceMP3(header) {
		return 0, errors.New("musica: spotDL produjo un archivo que no parece MP3")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, fmt.Errorf("musica: rebobinar salida spotDL: %w", err)
	}

	written, err := io.Copy(w, io.LimitReader(f, MaxAudio+1))
	if err != nil {
		return written, fmt.Errorf("musica: copiar salida spotDL: %w", err)
	}
	if written > MaxAudio {
		return written, fmt.Errorf("musica: el audio spotDL superó el tope de %d MB", MaxAudio>>20)
	}
	return written, nil
}

func pareceMP3(b []byte) bool {
	return len(b) >= 3 && bytes.Equal(b[:3], []byte("ID3")) ||
		len(b) >= 2 && b[0] == 0xFF && b[1]&0xE0 == 0xE0
}

type limitedSpotDLBuffer struct {
	buf      bytes.Buffer
	max      int
	overflow bool
}

func (b *limitedSpotDLBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := b.max - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.buf.Write(p)
	}
	if original > remaining {
		b.overflow = true
	}
	return original, nil
}

func (s *spotDL) run(ctx context.Context, timeout time.Duration, args ...string) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, s.path, args...)
	configureSpotDLProcess(cmd)
	cmd.Env = append(os.Environ(), "NO_COLOR=1", "TERM=dumb")
	stdout := &limitedSpotDLBuffer{max: maxSpotDLOutput}
	stderr := &limitedSpotDLBuffer{max: maxSpotDLOutput}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()

	outText := ansiSpotDL.ReplaceAllString(stdout.buf.String(), "")
	errText := ansiSpotDL.ReplaceAllString(stderr.buf.String(), "")
	if ctx.Err() != nil {
		return outText, errText, fmt.Errorf("%w: proceso cancelado o agotó %s", ErrSpotDLNoDisponible, timeout)
	}
	if stdout.overflow || stderr.overflow {
		return outText, errText, fmt.Errorf("%w: salida superior a %d MB", ErrSpotDLNoDisponible, maxSpotDLOutput>>20)
	}
	if err != nil {
		detail := strings.TrimSpace(errText + "\n" + outText)
		if detail == "" {
			detail = err.Error()
		}
		return outText, errText, fmt.Errorf("%w: %s", ErrSpotDLNoDisponible, recortaSpotDL(detail, 600))
	}
	return outText, errText, nil
}

func recortaSpotDL(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
