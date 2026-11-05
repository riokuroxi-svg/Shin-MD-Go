// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// fax.go — Genera la imagen del menú (.fax) sin dependencias: fuente propia
// (font.go) + image/png del paquete estándar.
//
// Todo tiene techo, porque esto lo dispara cualquiera desde el chat:
//
//	líneas      → 40 (lo que sobra se recorta y se avisa con "…")
//	carácter    → 40 columnas por línea
//	ancho/alto  → 1600 × 3000 px (nunca se reserva memoria proporcional al texto)
//	escala      → la mínima que quepa, entre 2 y 6
//
// Es lo único del bot que hace trabajo de CPU por mensaje, así que va sin
// prioridad: si el número está en calentamiento, espera su turno como todos.
package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
)

const (
	faxMaxLines  = 40
	faxMaxCols   = 40
	faxMaxWidth  = 1600
	faxMaxHeight = 3000
	faxMaxBytes  = 4 << 20 // 4 MB: la imagen del menú nunca debería pasar de aquí
)

// FaxDoc es lo que se quiere dibujar.
type FaxDoc struct {
	Title    string   // barra superior
	Subtitle string   // debajo del título, en pequeño
	Lines    []string // cuerpo (se recorta a faxMaxLines)
	Footer   string   // pie
}

// Paleta del "fax": fondo oscuro del repo, tinta rosa.
var (
	faxPaper  = color.RGBA{R: 0x1A, G: 0x12, B: 0x16, A: 0xFF}
	faxInk    = color.RGBA{R: 0xEE, G: 0x3D, B: 0x86, A: 0xFF}
	faxText   = color.RGBA{R: 0xF2, G: 0xE8, B: 0xEC, A: 0xFF}
	faxMuted  = color.RGBA{R: 0x9A, G: 0x7F, B: 0x8B, A: 0xFF}
	faxBorder = color.RGBA{R: 0x3A, G: 0x25, B: 0x2F, A: 0xFF}
	faxLine   = color.RGBA{R: 0x24, G: 0x18, B: 0x1E, A: 0xFF}
)

// colorValue y drawTarget permiten que el dibujo funcione sobre cualquier
// imagen sin que el resto del código sepa de qué tipo es.
type colorValue = color.RGBA

type drawTarget interface {
	fillRect(x, y, w, h int, c colorValue)
}

// RenderFax devuelve un PNG con el menú dibujado.
func RenderFax(doc FaxDoc) ([]byte, error) {
	lines := sanitizeLines(doc.Lines)
	title := clip(doc.Title, faxMaxCols)
	subtitle := clip(doc.Subtitle, faxMaxCols)
	footer := clip(doc.Footer, faxMaxCols)

	// Ancho: lo que mida la línea más larga (título y pie incluidos).
	cols := widest(title, subtitle, footer, lines...)
	if cols < 12 {
		cols = 12
	}

	scale := chooseScale(cols, len(lines))
	if scale < 1 {
		return nil, fmt.Errorf("el menú no cabe en la imagen")
	}

	pad := 6 * scale
	lineH := (glyphH + 3) * scale
	barH := glyphH*scale + 2*scale // la barra del título tiene que caber el texto

	// `cols` ya viene en píxeles de fuente (TextWidth), así que el ancho es
	// padding + texto escalado. Multiplicar otra vez por el ancho de glifo
	// hacía que el menú "no cupiera" nunca.
	width := pad*2 + cols*scale
	height := pad*3 + barH + lineH*2 + len(lines)*lineH + pad
	if width > faxMaxWidth || height > faxMaxHeight || width <= 0 || height <= 0 {
		return nil, fmt.Errorf("el menú no cabe en la imagen (%dx%d)", width, height)
	}

	img := newCanvas(width, height, faxPaper)

	// Papel de fax: rayas horizontales muy tenues (da la textura sin costar).
	for y := 0; y < height; y += 4 * scale {
		img.fillRect(0, y, width, scale/2+1, faxLine)
	}

	// Marco
	img.strokeRect(0, 0, width, height, scale/2+1, faxBorder)

	y := pad
	// Barra de título (texto centrado verticalmente dentro de la barra)
	img.fillRect(pad, y, width-pad*2, barH, faxInk)
	drawText(img, title, pad+scale*2, y+scale, scale, faxPaper)
	y += barH + scale

	if subtitle != "" {
		drawText(img, subtitle, pad, y, scale, faxMuted)
		y += lineH
	}

	// Separador
	img.fillRect(pad, y-scale/2, width-pad*2, maxInt(1, scale/2), faxBorder)
	y += scale

	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			y += lineH / 2
			continue
		}
		if y+glyphH*scale > height-pad-lineH {
			break
		}
		drawText(img, ln, pad, y, scale, faxText)
		y += lineH
	}

	// Pie
	if footer != "" {
		fy := height - pad - glyphH*scale
		img.fillRect(pad, fy-scale*2, width-pad*2, maxInt(1, scale/2), faxBorder)
		drawText(img, footer, pad, fy, scale, faxMuted)
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img.Image()); err != nil {
		return nil, err
	}
	if buf.Len() > faxMaxBytes {
		return nil, fmt.Errorf("la imagen del menú es demasiado grande (%d bytes)", buf.Len())
	}
	return buf.Bytes(), nil
}

// ─── lienzo ─────────────────────────────────────────────────────────────────

type canvas struct {
	*image.RGBA
}

func newCanvas(w, h int, bg colorValue) *canvas {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	c := &canvas{img}
	c.fillRect(0, 0, w, h, bg)
	return c
}

func (c *canvas) Image() *image.RGBA { return c.RGBA }

func (c *canvas) fillRect(x, y, w, h int, col colorValue) {
	if w <= 0 || h <= 0 {
		return
	}
	// Se recorta al lienzo: nada de escribir fuera de los límites.
	x0, y0 := maxInt(x, 0), maxInt(y, 0)
	x1, y1 := minInt(x+w, c.Rect.Dx()), minInt(y+h, c.Rect.Dy())
	for py := y0; py < y1; py++ {
		for px := x0; px < x1; px++ {
			c.SetRGBA(px, py, col)
		}
	}
}

func (c *canvas) strokeRect(x, y, w, h, t int, col colorValue) {
	c.fillRect(x, y, w, t, col)     // arriba
	c.fillRect(x, y+h-t, w, t, col) // abajo
	c.fillRect(x, y, t, h, col)     // izquierda
	c.fillRect(x+w-t, y, t, h, col) // derecha
}

// ─── utilidades de texto ────────────────────────────────────────────────────

// sanitizeLines recorta, limpia y limita las líneas del documento. Es también
// la barrera contra texto raro: nada de saltos de línea, controles ni
// invisibles que rompan el dibujo o se usen para disfrazar contenido.
func sanitizeLines(in []string) []string {
	out := make([]string, 0, minInt(len(in), faxMaxLines))
	for _, ln := range in {
		if len(out) >= faxMaxLines {
			out = append(out, "…")
			break
		}
		out = append(out, clip(sanitizeText(ln), faxMaxCols))
	}
	return out
}

// sanitizeText deja solo caracteres imprimibles y espacios simples.
//
// Además de los controles se quitan los invisibles (U+200B…U+200F,
// U+202A…U+202E, U+2060, U+FEFF): U+202E invierte el orden de lo que se
// dibuja, así que sirve para falsificar la imagen del menú.
func sanitizeText(s string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t' || r == '\r':
			r = ' '
		case r < 0x20 || r == 0x7F:
			continue
		case r >= 0x200B && r <= 0x200F,
			r >= 0x202A && r <= 0x202E,
			r == 0x2060, r == 0xFEFF:
			continue
		case r == 0x00A0:
			r = ' '
		}
		if r == ' ' {
			if lastSpace {
				continue
			}
			lastSpace = true
		} else {
			lastSpace = false
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// clip corta a `n` caracteres (no bytes) y marca el corte.
func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func widest(title, subtitle, footer string, lines ...string) int {
	w := TextWidth(title)
	for _, s := range append([]string{subtitle, footer}, lines...) {
		if n := TextWidth(s); n > w {
			w = n
		}
	}
	return w
}

// chooseScale busca el tamaño de letra más grande que quepa en los límites.
func chooseScale(cols, lines int) int {
	for s := 6; s >= 2; s-- {
		w := (12 + cols) * s
		h := (18 + lines*(glyphH+3)) * s
		if w <= faxMaxWidth && h <= faxMaxHeight {
			return s
		}
	}
	return 0
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
