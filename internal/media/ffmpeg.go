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
	"os/exec"
	"strings"
	"time"
)

// ErrSinFFmpeg: el sistema no tiene `ffmpeg` (ni `ffprobe`).
var ErrSinFFmpeg = errors.New("ffmpeg no está instalado")

// Espera máxima de una conversión. Un sticker de 6 segundos tarda 1–3 s; si
// pasa de aquí es que algo se colgó y no se deja el proceso del bot esperando.
const esperaFFmpeg = 60 * time.Second

// HayFFmpeg dice si el sistema tiene ffmpeg. Se comprueba en cada llamada:
// `LookPath` recorre el PATH y cuesta microsegundos, y así no hay que reiniciar
// el bot si el administrador acaba de instalarlo.
func HayFFmpeg() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
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

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
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
