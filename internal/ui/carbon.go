// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// carbon.go — Puerto de cmds/utils/carbon.js («.carbon», código → imagen).
//
// CAMBIO DE MOTOR RESPECTO AL JS
// El JS mandaba el código a la API de carbonara.solopov.dev: cuando esa web se
// cae, «.carbon» muere sin avisar y además el código sale del servidor. Aquí la
// imagen se dibuja dentro del binario con la fuente monoespaciada 7×13 de
// golang.org/x/image (dependencia ya presente): cero red, cero keys, resultado
// determinista. La ventana imita a carbon.now.sh: barra con los tres puntos,
// esquinas redondeadas y tema oscuro.
//
// Topes (lo dispara cualquiera desde el chat): 40 líneas × 88 columnas, PNG
// final ×2. Lo que no cabe se corta con «...» al final de la última línea.
package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"

	xfont "golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// TemasCarbon: los mismos nueve colores que tenía el JS.
var TemasCarbon = map[string]color.NRGBA{
	"verde":   {0x1F, 0x81, 0x6D, 0xFF},
	"negro":   {0x0F, 0x17, 0x2A, 0xFF},
	"azul":    {0x25, 0x3B, 0x7D, 0xFF},
	"morado":  {0x7C, 0x3A, 0xED, 0xFF},
	"rojo":    {0xB9, 0x1C, 0x1C, 0xFF},
	"rosa":    {0xDB, 0x27, 0x77, 0xFF},
	"naranja": {0xEA, 0x58, 0x0C, 0xFF},
	"gris":    {0x2D, 0x37, 0x48, 0xFF},
}

const (
	carbonMaxLineas  = 40
	carbonMaxCols    = 88
	carbonAnchoCar   = 7  // fuente 7×13
	carbonAltoLinea  = 15 // 13 de fuente + 2 de aire
	carbonBarra      = 34
	carbonMargenX    = 18
	carbonMargenY    = 14
	carbonEscala     = 2 // el PNG final nace al doble: nítido en el chat
	carbonMaxEntrada = 20_000
)

// Paleta sobre oscuro (estilo GitHub Dimmed).
var (
	carbonTexto      = color.NRGBA{0xAD, 0xBA, 0xC7, 0xFF}
	carbonComentario = color.NRGBA{0x76, 0x83, 0x90, 0xFF}
	carbonCadena     = color.NRGBA{0x96, 0xD0, 0xFF, 0xFF}
	carbonKeyword    = color.NRGBA{0xF4, 0x70, 0x67, 0xFF}
	carbonNumero     = color.NRGBA{0x6C, 0xB6, 0xFF, 0xFF}
	carbonLineaNum   = color.NRGBA{0x63, 0x6E, 0x7B, 0xFF}
)

// carbonKeywords: lo mínimo común de Go, JS/TS, Python, Java, C y shell.
var carbonKeywords = map[string]bool{
	"func": true, "return": true, "if": true, "else": true, "for": true,
	"while": true, "do": true, "switch": true, "case": true, "default": true,
	"break": true, "continue": true, "const": true, "let": true, "var": true,
	"import": true, "from": true, "export": true, "package": true, "class": true,
	"interface": true, "struct": true, "type": true, "def": true, "print": true,
	"public": true, "private": true, "protected": true, "static": true,
	"void": true, "int": true, "float": true, "double": true, "string": true,
	"bool": true, "char": true, "long": true, "true": true, "false": true,
	"nil": true, "null": true, "None": true, "True": true, "False": true,
	"async": true, "await": true, "go": true, "defer": true, "chan": true,
	"select": true, "range": true, "in": true, "is": true, "not": true,
	"and": true, "or": true, "try": true, "except": true, "catch": true,
	"finally": true, "throw": true, "throws": true, "new": true, "delete": true,
	"this": true, "self": true, "super": true, "extends": true, "with": true,
	"as": true, "pass": true, "raise": true, "elif": true, "echo": true,
}

// segmento es un trozo de línea con su clase de color.
type segmento struct {
	texto string
	clase color.NRGBA
}

// RenderCarbon dibuja el código y devuelve el PNG. Tema es el nombre del
// color («verde», «negro»…): desconocido = verde, como el JS, que caía al
// predeterminado cuando el tema no existía.
func RenderCarbon(codigo string, tema string) ([]byte, error) {
	codigo = strings.TrimRight(codigo, "\n")
	if strings.TrimSpace(codigo) == "" {
		return nil, fmt.Errorf("sin-codigo")
	}
	if len(codigo) > carbonMaxEntrada {
		codigo = codigo[:carbonMaxEntrada]
	}
	fondo, ok := TemasCarbon[strings.ToLower(strings.TrimSpace(tema))]
	if !ok {
		fondo = TemasCarbon["verde"]
	}

	// Preparar líneas: tabuladores a espacios (monoespaciada real), recorte
	// por columnas y por número de líneas, con marca clara de «hay más».
	lineas := strings.Split(codigo, "\n")
	cortado := false
	if len(lineas) > carbonMaxLineas {
		lineas = lineas[:carbonMaxLineas]
		cortado = true
	}
	for i, ln := range lineas {
		ln = strings.ReplaceAll(ln, "\t", "    ")
		ln = strings.TrimRight(ln, " \r")
		ln = recortarRunas(ln, carbonMaxCols)
		lineas[i] = ln
	}
	if cortado {
		lineas[len(lineas)-1] = recortarRunas(lineas[len(lineas)-1], carbonMaxCols-3) + "..."
	}

	// Medidas del lienzo (sin escala; la escala va al final).
	columnas := 1
	for _, ln := range lineas {
		if n := len([]rune(ln)); n > columnas {
			columnas = n
		}
	}
	digitos := len(strconv.Itoa(len(lineas)))
	anchoNums := (digitos + 2) * carbonAnchoCar
	ancho := carbonMargenX*2 + anchoNums + columnas*carbonAnchoCar
	alto := carbonBarra + carbonMargenY + len(lineas)*carbonAltoLinea + carbonMargenY

	img := image.NewNRGBA(image.Rect(0, 0, ancho, alto))
	rellenar(img, fondo)

	// Barra superior con los tres puntos de macOS, un tono más oscura.
	barra := mezclar(fondo, color.NRGBA{0, 0, 0, 0xFF}, 0.28)
	rellenarRect(img, 0, 0, ancho, carbonBarra, barra)
	puntos := []color.NRGBA{
		{0xFF, 0x5F, 0x57, 0xFF}, {0xFE, 0xBC, 0x2E, 0xFF}, {0x28, 0xC8, 0x40, 0xFF},
	}
	for i, pc := range puntos {
		circulo(img, carbonMargenX+4+i*18, carbonBarra/2, 6, pc)
	}

	// Líneas con su número y su color.
	d := &xfont.Drawer{Dot: fixed.P(0, 0)} // se reasigna por línea
	y := carbonBarra + carbonMargenY + 12  // 12: baseline de la primera línea
	enBloqueComentario := false
	for i, ln := range lineas {
		num := strconv.Itoa(i + 1)
		xNum := carbonMargenX + (digitos-len(num))*carbonAnchoCar
		escribir(img, d, num, xNum, y, carbonLineaNum)

		x := carbonMargenX + anchoNums
		segmentos, enBloque := colorearLinea(ln, enBloqueComentario)
		enBloqueComentario = enBloque
		for _, s := range segmentos {
			escribir(img, d, s.texto, x, y, s.clase)
			x += len([]rune(s.texto)) * carbonAnchoCar
		}
		y += carbonAltoLinea
	}

	// Esquinas redondeadas: se vacían los píxeles fuera del arco.
	redondear(img, 10)

	// PNG final al doble de tamaño (vecino más cercano: la fuente 7×13 es de
	// mapa de bits y así queda afilada, no borrosa).
	grande := image.NewNRGBA(image.Rect(0, 0, ancho*carbonEscala, alto*carbonEscala))
	for yy := 0; yy < grande.Bounds().Dy(); yy++ {
		for xx := 0; xx < grande.Bounds().Dx(); xx++ {
			sx, sy := xx/carbonEscala, yy/carbonEscala
			o := grande.PixOffset(xx, yy)
			s := img.PixOffset(sx, sy)
			copy(grande.Pix[o:o+4], img.Pix[s:s+4])
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, grande); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// colorearLinea parte la línea en segmentos con clase de color. `enBloque`
// viene de la línea anterior (comentarios /* */ de varias líneas) y la
// función devuelve cómo queda al acabar.
func colorearLinea(linea string, enBloque bool) ([]segmento, bool) {
	var out []segmento
	emitir := func(texto string, clase color.NRGBA) {
		if texto == "" {
			return
		}
		// Pegar con el segmento anterior si es del mismo color: menos llamadas.
		if n := len(out); n > 0 && out[n-1].clase == clase {
			out[n-1].texto += texto
			return
		}
		out = append(out, segmento{texto, clase})
	}

	i, n := 0, len(linea)
	for i < n {
		if enBloque {
			if j := strings.Index(linea[i:], "*/"); j >= 0 {
				emitir(linea[i:i+j+2], carbonComentario)
				i += j + 2
				enBloque = false
			} else {
				emitir(linea[i:], carbonComentario)
				i = n
			}
			continue
		}

		c := linea[i]
		switch {
		case c == '/' && i+1 < n && linea[i+1] == '/':
			emitir(linea[i:], carbonComentario)
			i = n
		case c == '#':
			emitir(linea[i:], carbonComentario)
			i = n
		case c == '/' && i+1 < n && linea[i+1] == '*':
			if j := strings.Index(linea[i+2:], "*/"); j >= 0 {
				emitir(linea[i:i+j+4], carbonComentario)
				i += j + 4
			} else {
				emitir(linea[i:], carbonComentario)
				enBloque = true
				i = n
			}
		case c == '"' || c == '\'' || c == '`':
			j := i + 1
			for j < n {
				if linea[j] == '\\' {
					j += 2
					continue
				}
				if linea[j] == c {
					j++
					break
				}
				j++
			}
			emitir(linea[i:j], carbonCadena)
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < n && (esLetra(linea[j]) || linea[j] == '.') {
				j++
			}
			emitir(linea[i:j], carbonNumero)
			i = j
		case esLetra(c) && !(c >= '0' && c <= '9'):
			j := i
			for j < n && esLetra(linea[j]) {
				j++
			}
			palabra := linea[i:j]
			if carbonKeywords[palabra] {
				emitir(palabra, carbonKeyword)
			} else {
				emitir(palabra, carbonTexto)
			}
			i = j
		default:
			emitir(string(c), carbonTexto)
			i++
		}
	}
	return out, enBloque
}

// esLetra es [A-Za-z_] y cualquier byte multibyte (≥0x80): va dentro de la
// palabra para no trocear nombres con tildes.
func esLetra(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c >= 0x80
}

// escribir dibuja texto sustituyendo lo no ASCII por «·» (la fuente 7×13 no
// tiene tildes ni emojis y su hueco en blanco confundiría la alineación).
func escribir(img *image.NRGBA, d *xfont.Drawer, s string, x, y int, col color.NRGBA) {
	d.Src = image.NewUniform(col)
	d.Dst = img
	d.Face = basicfont.Face7x13
	d.Dot = fixed.P(x, y)
	plano := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 {
			return '·'
		}
		return r
	}, s)
	d.DrawString(plano)
}

// ── Primitivas de dibujo (sin librería de shapes: son veinte líneas) ────────

func rellenar(img *image.NRGBA, col color.NRGBA) {
	for i := 0; i+4 <= len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = col.R, col.G, col.B, col.A
	}
}

func rellenarRect(img *image.NRGBA, x, y, w, h int, col color.NRGBA) {
	b := img.Bounds()
	for yy := y; yy < y+h && yy < b.Dy(); yy++ {
		if yy < 0 {
			continue
		}
		for xx := x; xx < x+w && xx < b.Dx(); xx++ {
			if xx < 0 {
				continue
			}
			o := img.PixOffset(xx, yy)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = col.R, col.G, col.B, col.A
		}
	}
}

// circulo rellena un círculo entero (los puntos de la barra).
func circulo(img *image.NRGBA, cx, cy, radio int, col color.NRGBA) {
	r2 := radio * radio
	for yy := cy - radio; yy <= cy+radio; yy++ {
		for xx := cx - radio; xx <= cx+radio; xx++ {
			if (xx-cx)*(xx-cx)+(yy-cy)*(yy-cy) > r2 {
				continue
			}
			if xx < 0 || yy < 0 || xx >= img.Bounds().Dx() || yy >= img.Bounds().Dy() {
				continue
			}
			o := img.PixOffset(xx, yy)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = col.R, col.G, col.B, col.A
		}
	}
}

// redondear vacía las esquinas fuera del arco de radio r (alfa 0). sx/sy son
// el signo que apunta hacia la esquina: solo se mira dentro de ese cuadrante.
func redondear(img *image.NRGBA, r int) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	esquinas := []struct{ cx, cy, sx, sy int }{
		{r, r, -1, -1},               // arriba-izquierda
		{w - 1 - r, r, 1, -1},        // arriba-derecha
		{r, h - 1 - r, -1, 1},        // abajo-izquierda
		{w - 1 - r, h - 1 - r, 1, 1}, // abajo-derecha
	}
	for _, e := range esquinas {
		for yy := 0; yy < h; yy++ {
			for xx := 0; xx < w; xx++ {
				dx, dy := (xx-e.cx)*e.sx, (yy-e.cy)*e.sy
				if dx < 0 || dy < 0 || dx > r || dy > r {
					continue // fuera del cuadrante de la esquina
				}
				if dx*dx+dy*dy > r*r {
					img.Pix[img.PixOffset(xx, yy)+3] = 0
				}
			}
		}
	}
}

// mezclar interpola entre dos colores (t∈[0,1] hacia el segundo).
func mezclar(a, b color.NRGBA, t float64) color.NRGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t) }
	return color.NRGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 0xFF}
}

// recortarRunas corta por runas, no por bytes (un emoji no se parte).
func recortarRunas(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
