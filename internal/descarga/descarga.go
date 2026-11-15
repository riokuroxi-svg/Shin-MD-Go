// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// descarga.go — B6.1: vídeos sociales cortos (.yt .tiktok .ig .fb .x)
// a través de yt-dlp cuando está instalado.
//
// POR QUÉ UN WRAPPER Y NO UN SCRAPER EN GO (decisión auditada nov-2026)
//
// YouTube en 2026 exige PoToken (Proof-of-Origin) por casi cada request y
// bloquea IPs de datacenter; los clientes «web» quedaron en SABR-only y los
// que sobreviven (android_vr que vuelve, tv_downgraded, mweb) rotan cada
// pocas semanas. Mantener un descargador de YouTube en Go sería prometer una
// cosa que se rompe sola — y aquí una utilidad que miente es peor que una
// que falta. Así que la honestidad queda así:
//
//   - yt-dlp sigue vivo precisamente porque está al día con YouTube; si el
//     usuario lo tiene, estos comandos funcionan con él.
//   - Si no está, el comando lo dice y enseña a ponerlo (un binario, sin
//     Python), en vez de fingir una descarga imposible.
//   - Sin cookies ni PoToken provider esto hoy falla bastante en servidores;
//     el error también lo dice tal cual («sign in to confirm you're not a
//     bot» se traduce, no se esconde).
//
// Topes heredados del play.js: vídeo ≤ 100 MB (MaxVideo), audio ≤ 24 MB
// (musica.MaxAudio), y plazo duro de 5 min para que un extractor colgado no
// retenga una goroutine del bot.
package descarga

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MaxVideo es el tope del JS para vídeo (100 MB por mensaje de WhatsApp).
const MaxVideo = 100 << 20

// Plazo duro de una bajada. yt-dlp con PoToken/roto puede colgar: 5 min y
// el proceso muere (kill, no cancel tibio).
const Plazo = 5 * time.Minute

// Ytdlp es el binario a ejecutar. Exportado y variable para que las pruebas
// lo apunten a un script falso (mismo patrón que las URLs de apis).
var Ytdlp = "yt-dlp"

// ErrNoYtdlp: el binario no está. La UI lo traduce a instrucciones.
var ErrNoYtdlp = errors.New("yt-dlp no está instalado")

// ErrBotCheck: YouTube pide verificación («sign in to confirm you're not a
// bot» / PoToken). La UI lo traduce a la explicación honesta.
var ErrBotCheck = errors.New("la plataforma pide comprobar que no somos un bot")

// Resultado es lo que salió de una bajada.
type Resultado struct {
	Archivo string // ruta temporal; el que llama la borra
	Titulo  string
	Bytes   int64
	EsAudio bool
}

// Bajar descarga url con yt-dlp en directorio dir.
//
//	formato: "video" (mejor mp4 bajo el tope) o "audio" (mejor audio → mp3).
//
// Devuelve el archivo listo o un error que ya viene traducido si era uno de
// los motivos conocidos (bot-check, privado, no soportado).
func Bajar(ctx context.Context, urlVideo, dir string, formato string) (*Resultado, error) {
	if _, err := exec.LookPath(Ytdlp); err != nil {
		return nil, ErrNoYtdlp
	}
	ctx, cancel := context.WithTimeout(ctx, Plazo)
	defer cancel()

	plantilla := filepath.Join(dir, "descarga-%(id)s.%(ext)s")
	args := []string{
		"--no-progress", "--no-playlist", "--no-warnings",
		"--socket-timeout", "20",
		"--print", "after_move:filepath",
		"--print", "title",
		"-o", plantilla,
	}
	switch formato {
	case "audio":
		args = append(args, "-x", "--audio-format", "mp3",
			"-f", "bestaudio/best")
	default: // vídeo corto: el mejor MP4 entero que QUEPA en el tope
		args = append(args,
			"-f", fmt.Sprintf("best[filesize<=%d][ext=mp4]/best[ext=mp4]/best", MaxVideo),
			"--max-filesize", strconv.Itoa(MaxVideo),
			"--merge-output-format", "mp4")
	}
	args = append(args, urlVideo)

	// Kill: un cancel tibio deja el proceso; WaitDelay corta también cuando el
	// hijo cierra los pipes pero se niega a morir (sleep en el test sin esto
	// de secuestraba la prueba un minuto entero: la salida la guarda Go, no el
	// hijo, así que el pipe no se cierra hasta que muere él).
	cmd := exec.CommandContext(ctx, Ytdlp, args...)
	cmd.Dir = dir
	cmd.Cancel = func() error { return cmd.Process.Kill() }
	cmd.WaitDelay = 3 * time.Second
	salida, err := cmd.Output()
	var stderr []byte
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		stderr = ee.Stderr
	}
	if err != nil {
		return nil, traducirError(err, string(salida)+string(stderr))
	}

	// yt-dlp imprime las líneas --print en orden: filepath y title.
	linea := strings.Split(strings.TrimSpace(string(salida)), "\n")
	if len(linea) == 0 || linea[0] == "" {
		return nil, errors.New("descarga: yt-dlp no devolvió archivo")
	}
	archivo := strings.TrimSpace(linea[0])
	info, err := os.Stat(archivo)
	if err != nil {
		return nil, fmt.Errorf("descarga: el archivo que dijo yt-dlp no está: %w", err)
	}
	if info.Size() > MaxVideo {
		_ = os.Remove(archivo)
		return nil, fmt.Errorf("descarga: pesa %d MB y el tope de WhatsApp son %d MB",
			info.Size()>>20, MaxVideo>>20)
	}
	titulo := ""
	if len(linea) > 1 {
		titulo = strings.TrimSpace(linea[1])
	}
	return &Resultado{Archivo: archivo, Titulo: titulo, Bytes: info.Size(), EsAudio: formato == "audio"}, nil
}

// Leer carga el archivo bajado y lo borra del disco de un tirón.
func (r *Resultado) Leer() ([]byte, error) {
	datos, err := os.ReadFile(r.Archivo)
	if err == nil {
		_ = os.Remove(r.Archivo)
	}
	return datos, err
}

// traducirError convierte lo que vomita yt-dlp en motivos con nombre. La UI
// no debe enseñar el stack; debe explicar qué pasó.
func traducirError(err error, salida string) error {
	s := strings.ToLower(salida)
	switch {
	case errors.Is(err, context.DeadlineExceeded), strings.Contains(err.Error(), "signal: killed"):
		return errors.New("la descarga tardó más de 5 minutos; cancelada")
	case strings.Contains(s, "sign in to confirm"), strings.Contains(s, "bot"),
		strings.Contains(s, "po token"), strings.Contains(s, "cookies"):
		return ErrBotCheck
	case strings.Contains(s, "private"), strings.Contains(s, "login required"):
		return errors.New("ese contenido es privado o pide iniciar sesión")
	case strings.Contains(s, "unsupported url"), strings.Contains(s, "no suitable"):
		return errors.New("esa URL no la entiende yt-dlp (o la página cambió el formato)")
	case strings.Contains(s, "file is larger"), strings.Contains(s, "max-filesize"):
		return fmt.Errorf("el vídeo pesa más del tope de %d MB", MaxVideo>>20)
	case strings.Contains(s, "geo"), strings.Contains(s, "not available"):
		return errors.New("el vídeo no está disponible desde esta región o fue retirado")
	default:
		return fmt.Errorf("la descarga falló (%s)", ultimaPalabra(s, 140))
	}
}

// ultimaPalabra recorta a la última frase del stderr, útil como pista.
func ultimaPalabra(s string, max int) string {
	s = strings.TrimSpace(s)
	if corte := strings.LastIndex(s, "\n"); corte >= 0 && len(s)-corte < max {
		s = s[corte+1:]
	}
	if len(s) > max {
		return "…" + s[len(s)-max:]
	}
	return s
}

// Progreso existe como hook para el futuro: hoy la descarga no reporta
// porcentaje (yt-dlp emite por stderr en bruto y no miente la UI).
var _ = io.EOF
