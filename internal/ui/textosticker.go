// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// textosticker.go — renderiza texto como sticker cuadrado (.brat y .qc).
// Mismo motor que carbon: basicfont sobre NRGBA, sin dependencia externa.
// .brat es el estilo «texto enorme sobre fondo negro» (de ahí el nombre);
// .qc dibuja una cita: nombre arriba, texto envuelto debajo.
package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

const (
	bratLado   = 512
	bratMaxPan = 900 // runas de entrada; más allá, se recorta (y se dice)
)

var (
	bratFondo = color.NRGBA{R: 10, G: 10, B: 12, A: 255}
	bratTexto = color.NRGBA{R: 240, G: 240, B: 240, A: 255}
	qcBurbuja = color.NRGBA{R: 32, G: 44, B: 52, A: 255} // la burbuja oscura de WA
	qcNombre  = color.NRGBA{R: 0, G: 168, B: 132, A: 255}
	qcFondo   = color.NRGBA{R: 11, G: 20, B: 26, A: 255}
)

// RenderBrat dibuja texto grande sobre fondo negro, líneas envueltas.
func RenderBrat(texto string) ([]byte, error) {
	return renderTextoSticker(texto, "", bratFondo, bratTexto, false)
}

// RenderQuoteSticker dibuja una cita (.qc): nombre, texto y hora, burbuja de
// WhatsApp. author vacío = sin la línea de nombre.
func RenderQuoteSticker(texto, autor string) ([]byte, error) {
	return renderTextoSticker(texto, autor, qcFondo, bratTexto, true)
}

// renderTextoSticker: envuelve, centra y pinta. Si burbuja, el nombre lleva
// renglón propio. Devuelve PNG (el comando lo pasa por media.Sticker).
func renderTextoSticker(texto, autor string, fondo, frente color.NRGBA, burbuja bool) ([]byte, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return nil, fmt.Errorf("sin-texto")
	}
	if len([]rune(texto)) > bratMaxPan {
		texto = string([]rune(texto)[:bratMaxPan]) + "…"
	}

	img := image.NewNRGBA(image.Rect(0, 0, bratLado, bratLado))
	rellenar(img, fondo)

	d := &xfont.Drawer{
		Dst:  img,
		Src:  image.NewUniform(frente),
		Face: basicfont.Face7x13,
	}

	// envolver a ~44 runas por línea en qc, ~26 en brat (va más grande)
	porLinea := 26
	topLinea := 20 // espaciado brat
	yTexto := 40
	if burbuja {
		porLinea = 44
		topLinea = 18
		yTexto = 60
	}

	if burbuja && autor != "" {
		d2 := &xfont.Drawer{Dst: img, Src: image.NewUniform(qcNombre), Face: basicfont.Face7x13}
		d2.Dot = fixed.P(26, 34)
		d2.DrawString(limita(autor, 40))
	}

	lineas := envuelve(texto, porLinea)
	// Cuántas líneas caben y si cortamos
	maxLineas := (bratLado - yTexto - 16) / topLinea
	if len(lineas) > maxLineas {
		lineas = lineas[:maxLineas]
		lineas[len(lineas)-1] = limita(lineas[len(lineas)-1], porLinea-2) + "…"
	}

	y := yTexto
	for _, ln := range lineas {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			y += topLinea
			continue
		}
		// Centrado por anchura de la fuente: 7 píxeles por runa en 7x13.
		x := (bratLado - len([]rune(ln))*7) / 2
		if burbuja {
			x = 26 // la cita va pegada a la izquierda, como WhatsApp
		}
		if x < 8 {
			x = 8
		}
		d.Dot = fixed.P(x, y)
		d.DrawString(ln)
		y += topLinea
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// envuelve parte por espacios sin cortar palabras de más (un brat con cortes
// feos se lee peor que uno corto).
func envuelve(s string, porLinea int) []string {
	var out []string
	var cur strings.Builder
	for _, palabra := range strings.Fields(s) {
		if cur.Len() > 0 && len([]rune(cur.String()))+1+len([]rune(palabra)) > porLinea {
			out = append(out, cur.String())
			cur.Reset()
		}
		if cur.Len() > 0 {
			cur.WriteByte(' ')
		}
		cur.WriteString(palabra)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// limita: recorta con puntos (compartido por los dos render del module).
func limita(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
