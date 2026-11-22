// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// ffmpeg.go — Los dos trabajos que Go no puede hacer solo: vídeo → sticker
// animado y sticker animado → vídeo.
//
// El JS llamaba a `ffmpeg` por `child_process` para lo primero y a la **web
// pública ezgif.com** para lo segundo: cada `.toimg` de un sticker animado
// subía el sticker a un tercero y esperaba a que respondiera. Eso es una
// dependencia que se cae, que filtra lo que manda el usuario y que no se puede
// usar si el servidor no tiene salida a internet (Termux con datos, por
// ejemplo). Aquí se usa `ffmpeg` local para las dos cosas, y si no está
// instalado se dice claramente en vez de fallar de forma rara.
package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	imagegif "image/gif"
	"os/exec"
	"strings"
	"time"

	"shin-go/internal/utilidades"
)

// ErrSinFFmpeg: el sistema no tiene `ffmpeg` (ni `ffprobe`).
var ErrSinFFmpeg = errors.New("ffmpeg no está instalado")

// Espera máxima de una conversión. Un sticker de 6 segundos tarda 1–3 s; si
// pasa de aquí es que algo se colgó y no se deja el proceso del bot esperando.
const esperaFFmpeg = 60 * time.Second

// HayFFmpeg dice si el sistema tiene ffmpeg. Se comprueba en cada llamada:
// BuscarBin recorre el PATH y cuesta microsegundos, y así no hay que reiniciar
// el bot si el administrador acaba de instalarlo. (LookPath quedó vetado: en
// linux/arm64 baja a faccessat2 y varios kernels de Android responden SIGSYS.)
func HayFFmpeg() bool {
	return utilidades.HayBin("ffmpeg")
}

// VideoAWebP convierte los primeros `segundos` de un vídeo en un sticker
// animado. Los ajustes son los del JS (512×512, 10 fps, bucle infinito).
//
// Se recorta a 3 segundos por defecto: WhatsApp corta los stickers animados a
// ~6 s y un vídeo largo tarda una eternidad en convertirse en un teléfono.
func VideoAWebP(ctx context.Context, video []byte, segundos int) ([]byte, error) {
	if !HayFFmpeg() {
		return nil, ErrSinFFmpeg
	}
	if segundos <= 0 || segundos > 6 {
		segundos = 3
	}
	entrada := "pipe:0"
	salida := "pipe:1"

	// scale con force_original_aspect_ratio=decrease + pad para que quepa
	// entero y con fondo transparente, igual que el `.sticker` del JS.
	filtro := fmt.Sprintf(
		"fps=10,scale=%d:%d:force_original_aspect_ratio=decrease,"+
			"pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=#00000000,format=rgba",
		Lado, Lado, Lado, Lado)

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-t", fmt.Sprintf("%d", segundos),
		"-i", entrada,
		"-vf", filtro,
		"-c:v", "libwebp_anim",
		"-q:v", "70",
		"-loop", "0",
		"-an", "-f", "webp", salida,
	}
	return ejecutar(ctx, args, video)
}

// GIFAVideo convierte una reacción GIF a MP4 con reproducción en bucle. Los
// clientes de WhatsApp representan un GIF como VideoMessage MP4 con
// gifPlayback=true; subir los bytes GIF fingiendo que son video/mp4 produce un
// mensaje roto o invisible.
func GIFAVideo(ctx context.Context, data []byte) ([]byte, error) {
	if !HayFFmpeg() {
		return nil, ErrSinFFmpeg
	}
	cfg, err := imagegif.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("GIF inválido: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 ||
		int64(cfg.Width)*int64(cfg.Height) > 16_000_000 {
		return nil, fmt.Errorf("GIF con dimensiones no admitidas: %dx%d", cfg.Width, cfg.Height)
	}
	filtro := "scale=w='min(640,iw)':h='min(640,ih)':force_original_aspect_ratio=decrease," +
		"pad=ceil(iw/2)*2:ceil(ih/2)*2,format=yuv420p"
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0", "-t", "15",
		"-vf", filtro,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "27",
		"-pix_fmt", "yuv420p", "-an",
		"-movflags", "frag_keyframe+empty_moov",
		"-f", "mp4", "pipe:1",
	}
	return ejecutar(ctx, args, data)
}

// WebPAVideo convierte un sticker (fijo o animado) en MP4, en local.
//
// Se genera un MP4 con fondo blanco en vez de negro: los stickers tienen alfa y
// al pasarlos a vídeo ese alfa queda negro, que es lo que hacía ezgif y lo que
// no le gusta a nadie. Con `-vf` se pinta el fondo antes de codificar.
func WebPAVideo(ctx context.Context, webp []byte, segundos int) ([]byte, error) {
	if !HayFFmpeg() {
		return nil, ErrSinFFmpeg
	}
	if segundos <= 0 || segundos > 30 {
		segundos = 3
	}
	filtro := "scale=512:512:force_original_aspect_ratio=decrease," +
		"pad=512:512:(ow-iw)/2:(oh-ih)/2:color=white," +
		"format=yuv420p"

	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-t", fmt.Sprintf("%d", segundos),
		"-vf", filtro,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "26",
		"-pix_fmt", "yuv420p", "-movflags", "+faststart",
		"-an", "-f", "mp4", "pipe:1",
	}
	return ejecutar(ctx, args, webp)
}

// ejecutar lanza ffmpeg con los datos por la entrada estándar y recoge la
// salida. Nada de archivos temporales: así no queda basura si el proceso muere.
func ejecutar(ctx context.Context, args []string, entrada []byte) ([]byte, error) {
	ctx, cancelar := context.WithTimeout(ctx, esperaFFmpeg)
	defer cancelar()

	bin, err := utilidades.BuscarBin("ffmpeg")
	if err != nil {
		return nil, ErrSinFFmpeg
	}
	cmd := exec.CommandContext(ctx, bin, args...) // ruta absoluta: sin LookPath interno
	cmd.Stdin = bytes.NewReader(entrada)
	var salida, errores bytes.Buffer
	cmd.Stdout = &salida
	cmd.Stderr = &errores

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ffmpeg tardó más de %s y se canceló", esperaFFmpeg)
		}
		return nil, fmt.Errorf("ffmpeg falló: %v: %s", err, recortarError(errores.String()))
	}
	if salida.Len() == 0 {
		return nil, fmt.Errorf("ffmpeg no devolvió nada: %s", recortarError(errores.String()))
	}
	return salida.Bytes(), nil
}

// recortarError deja el mensaje de ffmpeg en una sola línea corta (el suyo
// puede traer 20 avisos que no le dicen nada a quien usa el bot).
func recortarError(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "sin detalles"
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	return s
}

// EfectoAudio aplica un filtro de ffmpeg (-af) a un audio y devuelve OPUS en
// un ogg (lo que WhatsApp enseña como nota de voz). El filtro lo decide el
// mapa del comando; este no confía en él para nada más que ir a -af.
func EfectoAudio(ctx context.Context, audio []byte, filtro string) ([]byte, error) {
	if !HayFFmpeg() {
		return nil, ErrSinFFmpeg
	}
	if filtro == "" {
		return nil, fmt.Errorf("filtro vacío")
	}
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-af", filtro,
		"-c:a", "libopus",
		"-b:a", "64k",
		"-f", "ogg", "pipe:1",
	}
	return ejecutar(ctx, args, audio)
}
