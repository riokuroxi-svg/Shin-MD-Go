// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// media.go — De una foto (o un fotograma) a un sticker WebP de 512×512.
//
// El bot de JS lo hacía con `sharp` (nativo) y `node-webpmux`. Aquí se hace con
// dos dependencias en Go puro:
//
//	github.com/HugoSmits86/nativewebp  → escribe WebP (con alfa)
//	golang.org/x/image                 → lee WebP y escala
//
// Nada de binarios nativos ni de `npm install` que compile C++. Los vídeos sí
// necesitan `ffmpeg` en el sistema (ver ffmpeg.go): sin él, el comando avisa y
// no se queda colgado esperando.
package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"

	"github.com/HugoSmits86/nativewebp"
	"golang.org/x/image/webp"
)

// Lado del sticker que acepta WhatsApp. Se manda siempre en este tamaño
// cuadrado para que no lo rechace.
const Lado = 512

// ErrFormato: el archivo no es una imagen que sepamos leer.
var ErrFormato = errors.New("formato de imagen no reconocido")

// Decode lee una imagen en cualquier formato habitual. Para GIF usa el primer
// fotograma (el sticker animado de verdad sale del vídeo, no del GIF).
func Decode(data []byte) (image.Image, error) {
	switch Detect(data) {
	case "png":
		return png.Decode(bytes.NewReader(data))
	case "jpeg":
		return jpeg.Decode(bytes.NewReader(data))
	case "webp":
		// El WebP que escribimos lleva formato extendido (VP8X + VP8L) porque es
		// el único que admite el EXIF del paquete, y los decodificadores de Go
		// solo aceptan VP8L "pelado". Así que si fallan, se reconstruye el
		// archivo quitando el envoltorio VP8X y se vuelve a intentar.
		if img, err := nativewebp.Decode(bytes.NewReader(data)); err == nil {
			return img, nil
		}
		if simple := desenvolverWebP(data); simple != nil {
			if img, err := webp.Decode(bytes.NewReader(simple)); err == nil {
				return img, nil
			}
			if img, err := nativewebp.Decode(bytes.NewReader(simple)); err == nil {
				return img, nil
			}
		}
		return webp.Decode(bytes.NewReader(data))
	case "gif":
		g, err := gif.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return g, nil
	}
	// Último intento: que prueben los decodificadores en orden.
	if img, err := png.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	if img, err := jpeg.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	if img, err := webp.Decode(bytes.NewReader(data)); err == nil {
		return img, nil
	}
	return nil, ErrFormato
}

// Detect adivina el formato por la cabecera, sin fiarse de la extensión (en
// WhatsApp el nombre del archivo casi nunca está).
func Detect(data []byte) string {
	switch {
	case len(data) < 12:
		return ""
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}):
		return "png"
	case bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}):
		return "jpeg"
	case bytes.HasPrefix(data, []byte("GIF8")):
		return "gif"
	case bytes.HasPrefix(data, []byte("RIFF")) && string(data[8:12]) == "WEBP":
		return "webp"
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		return "video" // mp4/mov: lo trata ffmpeg
	}
	return ""
}

// EsAnimado dice si un WebP es animado (chunk ANIM). Un sticker animado no se
// puede convertir a PNG sin perder el movimiento, así que `.toimg` lo manda a
// ffmpeg y saca un vídeo.
func EsAnimado(webp []byte) bool {
	chunks, err := trocear(cuerpoRIFF(webp))
	if err != nil {
		return false
	}
	for _, c := range chunks {
		if string(c.cabecera[0:4]) == "ANIM" {
			return true
		}
	}
	return false
}

// cuerpoRIFF devuelve el cuerpo del RIFF (después de "RIFF????WEBP").
func cuerpoRIFF(data []byte) []byte {
	if len(data) < 12 || string(data[0:4]) != "RIFF" {
		return nil
	}
	return data[12:]
}

// PrimerFotograma saca el primer fotograma legible de un WebP, sea fijo o
// animado.
//
// Sirve de red de seguridad en `.toimg`: hay compilaciones de ffmpeg sin
// decodificador de WebP animado (la estática de Linux, por ejemplo), y en vez
// de contestar "no puedo" se manda la primera imagen del sticker. Un sticker
// animado sale quieto, pero sale.
func PrimerFotograma(data []byte) (image.Image, error) {
	chunks, err := trocear(cuerpoRIFF(data))
	if err != nil {
		return nil, err
	}
	for _, c := range chunks {
		switch string(c.cabecera[0:4]) {
		case "VP8L", "VP8 ":
			if img, err := decodificarBitstream(string(c.cabecera[0:4]), c.datos); err == nil {
				return img, nil
			}
		case "ANMF":
			if len(c.datos) <= 16 {
				continue
			}
			// El ANMF lleva 16 bytes de cabecera (posición, tamaño y duración)
			// y después los chunks del fotograma.
			sub, err := trocear(c.datos[16:])
			if err != nil {
				continue
			}
			for _, s := range sub {
				nombre := string(s.cabecera[0:4])
				if nombre != "VP8L" && nombre != "VP8 " {
					continue
				}
				if img, err := decodificarBitstream(nombre, s.datos); err == nil {
					return img, nil
				}
			}
		}
	}
	return nil, ErrFormato
}

// decodificarBitstream envuelve un trozo VP8/VP8L suelto en un RIFF mínimo
// para poder pasárselo a los decodificadores.
func decodificarBitstream(nombre string, datos []byte) (image.Image, error) {
	cuerpo := make([]byte, 8, 8+len(datos)+1)
	copy(cuerpo, nombre)
	binary.LittleEndian.PutUint32(cuerpo[4:], uint32(len(datos)))
	cuerpo = append(cuerpo, datos...)
	if len(datos)%2 == 1 {
		cuerpo = append(cuerpo, 0)
	}
	salida := make([]byte, 12, 12+len(cuerpo))
	copy(salida, "RIFF")
	binary.LittleEndian.PutUint32(salida[4:], uint32(len(cuerpo)+4))
	copy(salida[8:], "WEBP")
	salida = append(salida, cuerpo...)

	if img, err := webp.Decode(bytes.NewReader(salida)); err == nil {
		return img, nil
	}
	return nativewebp.Decode(bytes.NewReader(salida))
}

// desenvolverWebP quita de un WebP el chunk VP8X (y todo lo que no sea la
// imagen) y devuelve un RIFF mínimo con el VP8L/VP8 dentro. Solo se usa para
// leer; el archivo que se manda a WhatsApp conserva su VP8X y su EXIF.
func desenvolverWebP(data []byte) []byte {
	chunks, err := trocear(cuerpoRIFF(data))
	if err != nil {
		return nil
	}
	for _, c := range chunks {
		nombre := string(c.cabecera[0:4])
		if nombre != "VP8L" && nombre != "VP8 " {
			continue
		}
		cuerpo := make([]byte, 0, 8+len(c.datos)+1)
		cuerpo = append(cuerpo, c.cabecera...)
		cuerpo = append(cuerpo, c.datos...)
		if len(c.datos)%2 == 1 {
			cuerpo = append(cuerpo, 0)
		}
		salida := make([]byte, 12, 12+len(cuerpo))
		copy(salida, "RIFF")
		binary.LittleEndian.PutUint32(salida[4:], uint32(len(cuerpo)+4))
		copy(salida[8:], "WEBP")
		return append(salida, cuerpo...)
	}
	return nil
}

// Ajuste dice cómo encajar la imagen en el cuadrado.
type Ajuste int

const (
	// Contener: la imagen entera cabe dentro (con transparencia alrededor).
	Contener Ajuste = iota
	// Cubrir: llena el cuadrado y recorta lo que sobra.
	Cubrir
)

// Fit escala la imagen a un cuadrado de `lado`×`lado` píxeles.
//
// Se hace a mano (interpolación bilineal) en vez de con `x/image/draw` para
// poder mezclar los colores con alfa correctamente y para que el resultado sea
// el mismo en todas las plataformas.
func Fit(img image.Image, lado int, modo Ajuste) *image.NRGBA {
	if lado <= 0 {
		lado = Lado
	}
	src := toNRGBA(img)
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	if sw == 0 || sh == 0 {
		return image.NewNRGBA(image.Rect(0, 0, lado, lado))
	}

	// Contener = el menor de los dos factores (la imagen entera cabe).
	// Cubrir   = el mayor (llena el cuadrado y se recorta lo que sobra).
	fx := float64(lado) / float64(sw)
	fy := float64(lado) / float64(sh)
	escala := fx
	if modo == Cubrir {
		if fy > escala {
			escala = fy
		}
	} else if fy < escala {
		escala = fy
	}

	dw := int(float64(sw)*escala + 0.5)
	dh := int(float64(sh)*escala + 0.5)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}

	escalada := escalar(src, dw, dh)
	out := image.NewNRGBA(image.Rect(0, 0, lado, lado))
	offX := (lado - dw) / 2
	offY := (lado - dh) / 2
	draw.Draw(out, image.Rect(offX, offY, offX+dw, offY+dh), escalada, image.Point{}, draw.Over)
	return out
}

// toNRGBA convierte cualquier imagen a NRGBA (alfa no premultiplicado).
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	return out
}

// escalar hace la interpolación bilineal de verdad.
func escalar(src *image.NRGBA, dw, dh int) *image.NRGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	out := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	if sw == dw && sh == dh {
		copy(out.Pix, src.Pix)
		return out
	}
	escalaX := float64(sw) / float64(dw)
	escalaY := float64(sh) / float64(dh)
	for y := 0; y < dh; y++ {
		fy := (float64(y)+0.5)*escalaY - 0.5
		y0 := int(fy)
		if y0 < 0 {
			y0 = 0
		}
		y1 := y0 + 1
		if y1 >= sh {
			y1 = sh - 1
		}
		wy := fy - float64(y0)
		if wy < 0 {
			wy = 0
		}
		for x := 0; x < dw; x++ {
			fx := (float64(x)+0.5)*escalaX - 0.5
			x0 := int(fx)
			if x0 < 0 {
				x0 = 0
			}
			x1 := x0 + 1
			if x1 >= sw {
				x1 = sw - 1
			}
			wx := fx - float64(x0)
			if wx < 0 {
				wx = 0
			}

			i00 := (y0*sw + x0) * 4
			i10 := (y0*sw + x1) * 4
			i01 := (y1*sw + x0) * 4
			i11 := (y1*sw + x1) * 4
			o := out.PixOffset(x, y)
			for c := 0; c < 4; c++ {
				// Se mezcla en alfa-premultiplicado para que los bordes
				// transparentes no se coman el color.
				a00 := float64(src.Pix[i00+c])
				a10 := float64(src.Pix[i10+c])
				a01 := float64(src.Pix[i01+c])
				a11 := float64(src.Pix[i11+c])
				arriba := a00 + (a10-a00)*wx
				abajo := a01 + (a11-a01)*wx
				out.Pix[o+c] = uint8(arriba + (abajo-arriba)*wy + 0.5)
			}
		}
	}
	return out
}

// EncodeWebP escribe el sticker.
//
// nativewebp es un codificador **sin pérdida** (VP8L). Se activa el formato
// extendido (VP8X) porque es el único que admite el EXIF con el nombre del
// paquete; sin él, WhatsApp muestra el sticker como "pegatina sin autor".
// El tamaño de un sticker de 512×512 sin pérdida ronda los 200–600 KB, dentro
// del 1 MB que acepta WhatsApp.
func EncodeWebP(img image.Image, nivel nativewebp.CompressionLevel) ([]byte, error) {
	if nivel < nativewebp.BestSpeed || nivel > nativewebp.BestCompression {
		nivel = nativewebp.DefaultCompression
	}
	var buf bytes.Buffer
	err := nativewebp.Encode(&buf, toNRGBA(img), &nativewebp.Options{
		UseExtendedFormat: true,
		CompressionLevel:  nivel,
	})
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodePNG convierte a PNG (lo que hace `.toimg` con un sticker fijo).
func EncodePNG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodeJPEG convierte a JPEG con fondo blanco. Un sticker tiene alfa; si se
// manda a JPEG tal cual, los transparentes salen negros. Se pinta sobre blanco.
func EncodeJPEG(img image.Image, calidad int) ([]byte, error) {
	if calidad <= 0 || calidad > 100 {
		calidad = 90
	}
	src := toNRGBA(img)
	fondo := image.NewRGBA(image.Rect(0, 0, src.Bounds().Dx(), src.Bounds().Dy()))
	draw.Draw(fondo, fondo.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(fondo, fondo.Bounds(), src, src.Bounds().Min, draw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, fondo, &jpeg.Options{Quality: calidad}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Sticker: del archivo original al WebP final de 512×512, con la forma y los
// filtros ya aplicados (y sin EXIF: de eso se encarga exif.go).
func Sticker(data []byte, opciones Opciones) ([]byte, error) {
	img, err := Decode(data)
	if err != nil {
		return nil, err
	}
	if opciones.Forma != "" {
		if _, ok := Mascara(opciones.Forma); !ok {
			return nil, fmt.Errorf("forma desconocida: %q", opciones.Forma)
		}
	}
	if opciones.Efecto != "" {
		if _, ok := Filtro(opciones.Efecto); !ok {
			return nil, fmt.Errorf("efecto desconocido: %q", opciones.Efecto)
		}
	}

	modo := Contener
	if !opciones.FormaAuto {
		modo = Cubrir // con forma, la imagen llena el marco
	}
	out := Fit(img, opciones.Lado, modo)

	if opciones.Efecto != "" {
		f, _ := Filtro(opciones.Efecto)
		f.Aplicar(out)
	}
	if opciones.Forma != "" {
		m, _ := Mascara(opciones.Forma)
		m.Aplicar(out)
	}
	return EncodeWebP(out, nativewebp.DefaultCompression)
}

// Opciones son los ajustes de una conversión a sticker.
type Opciones struct {
	Lado      int
	Forma     string // "" = sin forma
	Efecto    string // "" = sin filtro
	FormaAuto bool   // true = la imagen se encaja entera (sin recortar)
}
