// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// filtros.go — Los efectos del `.sticker` (-blur, -sepia, -invert…).
//
// En el JS cada efecto era un comando de ImageMagick/`sharp` sobre un PNG
// temporal. Aquí se trabaja sobre los píxeles en memoria: ninguna llamada
// externa, ningún archivo temporal, y el resultado es idéntico en cualquier
// máquina (un `-blur` de ImageMagick cambia según la versión instalada).
package media

import (
	"image"
	"math"
)

// Filtro transforma la imagen en su sitio.
type Filtro_ struct {
	Nombre string
	// Aplicar modifica la imagen. Se pasa siempre en NRGBA (alfa no premultiplicado).
	Aplicar func(img *image.NRGBA)
}

// CatalogoFiltros: nombre → efecto. Los nombres son los del JS sin el guion.
var CatalogoFiltros = map[string]Filtro_{
	"grayscale": {"grayscale", escalaDeGrises},
	"sepia":     {"sepia", sepia},
	"invert":    {"invert", invertir},
	"negate":    {"negate", invertir},
	"brighten":  {"brighten", brillo(45)},
	"darken":    {"darken", brillo(-45)},
	"blur":      {"blur", desenfoque(1)},
	"sharpen":   {"sharpen", enfocar},
	"rotate90":  {"rotate90", rotar90},
	"rotate180": {"rotate180", rotar180},
	"flip":      {"flip", voltearVertical},
	"flop":      {"flop", voltearHorizontal},
	"tint":      {"tint", tinte(150, 90, 200)},
	"normalice": {"normalice", normalizar},
}

// Filtro busca un efecto por su nombre.
func Filtro(nombre string) (Filtro_, bool) {
	f, ok := CatalogoFiltros[nombre]
	return f, ok
}

// NombresFiltros: los efectos en orden alfabético (para la ayuda).
func NombresFiltros() []string {
	out := make([]string, 0, len(CatalogoFiltros))
	for k := range CatalogoFiltros {
		out = append(out, k)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// ─── color ──────────────────────────────────────────────────────────────────

func escalaDeGrises(img *image.NRGBA) {
	for i := 0; i+3 < len(img.Pix); i += 4 {
		// Luminancia con los coeficientes de siempre (los del ojo humano).
		l := 0.299*float64(img.Pix[i]) + 0.587*float64(img.Pix[i+1]) + 0.114*float64(img.Pix[i+2])
		v := uint8(l + 0.5)
		img.Pix[i], img.Pix[i+1], img.Pix[i+2] = v, v, v
	}
}

func sepia(img *image.NRGBA) {
	for i := 0; i+3 < len(img.Pix); i += 4 {
		r := float64(img.Pix[i])
		g := float64(img.Pix[i+1])
		b := float64(img.Pix[i+2])
		img.Pix[i] = recorte(0.393*r + 0.769*g + 0.189*b)
		img.Pix[i+1] = recorte(0.349*r + 0.686*g + 0.168*b)
		img.Pix[i+2] = recorte(0.272*r + 0.534*g + 0.131*b)
	}
}

func invertir(img *image.NRGBA) {
	for i := 0; i+3 < len(img.Pix); i += 4 {
		img.Pix[i] = 255 - img.Pix[i]
		img.Pix[i+1] = 255 - img.Pix[i+1]
		img.Pix[i+2] = 255 - img.Pix[i+2]
	}
}

// brillo suma `delta` a cada canal (negativo oscurece).
func brillo(delta float64) func(*image.NRGBA) {
	return func(img *image.NRGBA) {
		for i := 0; i+3 < len(img.Pix); i += 4 {
			img.Pix[i] = recorte(float64(img.Pix[i]) + delta)
			img.Pix[i+1] = recorte(float64(img.Pix[i+1]) + delta)
			img.Pix[i+2] = recorte(float64(img.Pix[i+2]) + delta)
		}
	}
}

// tinte multiplica cada canal por un peso (0-255): así un `-tint` no quema la
// imagen, la colorea.
func tinte(r, g, b float64) func(*image.NRGBA) {
	return func(img *image.NRGBA) {
		for i := 0; i+3 < len(img.Pix); i += 4 {
			img.Pix[i] = recorte(float64(img.Pix[i]) * r / 255)
			img.Pix[i+1] = recorte(float64(img.Pix[i+1]) * g / 255)
			img.Pix[i+2] = recorte(float64(img.Pix[i+2]) * b / 255)
		}
	}
}

// normalizar estira el contraste usando el 2 % y el 98 % de los valores, que es
// lo que hace un "auto-levels" de verdad.
func normalizar(img *image.NRGBA) {
	var hist [256]int
	total := 0
	for i := 0; i+3 < len(img.Pix); i += 4 {
		if img.Pix[i+3] == 0 {
			continue
		}
		l := (int(img.Pix[i]) + int(img.Pix[i+1]) + int(img.Pix[i+2])) / 3
		hist[l]++
		total++
	}
	if total == 0 {
		return
	}
	minimo, maximo := 0, 255
	acumulado := 0
	for v := 0; v < 256; v++ {
		acumulado += hist[v]
		if acumulado >= total*2/100 {
			minimo = v
			break
		}
	}
	acumulado = 0
	for v := 255; v >= 0; v-- {
		acumulado += hist[v]
		if acumulado >= total*2/100 {
			maximo = v
			break
		}
	}
	if maximo <= minimo {
		return
	}
	factor := 255.0 / float64(maximo-minimo)
	for i := 0; i+3 < len(img.Pix); i += 4 {
		for c := 0; c < 3; c++ {
			img.Pix[i+c] = recorte((float64(img.Pix[i+c]) - float64(minimo)) * factor)
		}
	}
}

// ─── desenfoque y nitidez ───────────────────────────────────────────────────

// desenfoque: caja separable. Con radio 1 son 3 píxeles por lado y se nota sin
// destruir la imagen (un gaussiano grande sobre 512×512 tarda 40 veces más y el
// resultado en un sticker no se distingue).
func desenfoque(radio int) func(*image.NRGBA) {
	return func(img *image.NRGBA) {
		if radio < 1 {
			radio = 1
		}
		ancho, alto := img.Bounds().Dx(), img.Bounds().Dy()
		tmp := make([]uint8, len(img.Pix))
		copy(tmp, img.Pix)

		// Horizontal
		for y := 0; y < alto; y++ {
			for x := 0; x < ancho; x++ {
				var sr, sg, sb, sa, n float64
				for k := -radio; k <= radio; k++ {
					xx := x + k
					if xx < 0 || xx >= ancho {
						continue
					}
					o := (y*ancho + xx) * 4
					// Se pesa por alfa: si no, los bordes transparentes
					// arrastran el color hacia el negro.
					a := float64(tmp[o+3]) / 255
					sr += float64(tmp[o]) * a
					sg += float64(tmp[o+1]) * a
					sb += float64(tmp[o+2]) * a
					sa += float64(tmp[o+3])
					n += a
				}
				o := (y*ancho + x) * 4
				if n > 0 {
					img.Pix[o] = recorte(sr / n)
					img.Pix[o+1] = recorte(sg / n)
					img.Pix[o+2] = recorte(sb / n)
				}
				img.Pix[o+3] = recorte(sa / float64(2*radio+1))
			}
		}
		copy(tmp, img.Pix)

		// Vertical
		for y := 0; y < alto; y++ {
			for x := 0; x < ancho; x++ {
				var sr, sg, sb, sa, n float64
				for k := -radio; k <= radio; k++ {
					yy := y + k
					if yy < 0 || yy >= alto {
						continue
					}
					o := (yy*ancho + x) * 4
					a := float64(tmp[o+3]) / 255
					sr += float64(tmp[o]) * a
					sg += float64(tmp[o+1]) * a
					sb += float64(tmp[o+2]) * a
					sa += float64(tmp[o+3])
					n += a
				}
				o := (y*ancho + x) * 4
				if n > 0 {
					img.Pix[o] = recorte(sr / n)
					img.Pix[o+1] = recorte(sg / n)
					img.Pix[o+2] = recorte(sb / n)
				}
				img.Pix[o+3] = recorte(sa / float64(2*radio+1))
			}
		}
	}
}

// enfocar: máscara de nitidez (centro ×5, vecinos ×−1), la de siempre.
func enfocar(img *image.NRGBA) {
	ancho, alto := img.Bounds().Dx(), img.Bounds().Dy()
	copia := make([]uint8, len(img.Pix))
	copy(copia, img.Pix)
	for y := 1; y < alto-1; y++ {
		for x := 1; x < ancho-1; x++ {
			for c := 0; c < 3; c++ {
				centro := float64(copia[(y*ancho+x)*4+c]) * 5
				vecinos := float64(copia[((y-1)*ancho+x)*4+c]) +
					float64(copia[((y+1)*ancho+x)*4+c]) +
					float64(copia[(y*ancho+x-1)*4+c]) +
					float64(copia[(y*ancho+x+1)*4+c])
				img.Pix[(y*ancho+x)*4+c] = recorte(centro - vecinos)
			}
		}
	}
}

// ─── giros y espejos (reemplazan el búfer entero) ───────────────────────────

func rotar90(img *image.NRGBA)  { *img = *rotar(*img, 1) }
func rotar180(img *image.NRGBA) { *img = *rotar(*img, 2) }

// rotar crea una imagen nueva: `veces` × 90°.
func rotar(src image.NRGBA, veces int) *image.NRGBA {
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	for i := 0; i < veces; i++ {
		nueva := image.NewNRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				// (x, y) → (h−1−y, x)
				o := nueva.PixOffset(h-1-y, x)
				s := src.PixOffset(x, y)
				copy(nueva.Pix[o:o+4], src.Pix[s:s+4])
			}
		}
		src = *nueva
		w, h = h, w
	}
	return &src
}

func voltearVertical(img *image.NRGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for y := 0; y < h/2; y++ {
		for x := 0; x < w; x++ {
			a := img.PixOffset(x, y)
			b := img.PixOffset(x, h-1-y)
			for c := 0; c < 4; c++ {
				img.Pix[a+c], img.Pix[b+c] = img.Pix[b+c], img.Pix[a+c]
			}
		}
	}
}

func voltearHorizontal(img *image.NRGBA) {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	for y := 0; y < h; y++ {
		for x := 0; x < w/2; x++ {
			a := img.PixOffset(x, y)
			b := img.PixOffset(w-1-x, y)
			for c := 0; c < 4; c++ {
				img.Pix[a+c], img.Pix[b+c] = img.Pix[b+c], img.Pix[a+c]
			}
		}
	}
}

// recorte mete un valor en 0..255.
func recorte(v float64) uint8 {
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	case math.IsNaN(v):
		return 0
	}
	return uint8(v + 0.5)
}
