// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// mascaras.go — Las formas del `.sticker` (-c círculo, -v corazón…).
//
// En el JS cada forma era un archivo SVG distinto que había que traer con
// `sharp(...).composite()`. Aquí son **máscaras dibujadas a mano**: se define la
// figura (círculo, polígono o curva) y se rasteriza con 4×4 muestras por píxel
// para que el borde quede suave. Sin archivos, sin dependencias y con el mismo
// resultado en cualquier máquina.
package media

import (
	"image"
	"math"
)

// Mascara es una forma que recorta el sticker.
type Forma struct {
	Nombre string
	// dentro dice si el punto (x, y) —en coordenadas 0..1— está dentro.
	dentro func(x, y float64) bool
}

// Aplicar deja transparente todo lo que cae fuera de la forma.
func (m Forma) Aplicar(img *image.NRGBA) {
	if m.dentro == nil {
		return
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return
	}
	const muestras = 4
	cobertura := make([]float64, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dentro := 0
			for sy := 0; sy < muestras; sy++ {
				for sx := 0; sx < muestras; sx++ {
					fx := (float64(x) + (float64(sx)+0.5)/muestras) / float64(w)
					fy := (float64(y) + (float64(sy)+0.5)/muestras) / float64(h)
					if m.dentro(fx, fy) {
						dentro++
					}
				}
			}
			cobertura[y*w+x] = float64(dentro) / float64(muestras*muestras)
		}
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := cobertura[y*w+x]
			if c >= 1 {
				continue
			}
			o := img.PixOffset(b.Min.X+x, b.Min.Y+y)
			if c <= 0 {
				img.Pix[o+3] = 0
				continue
			}
			img.Pix[o+3] = uint8(float64(img.Pix[o+3])*c + 0.5)
		}
	}
}

// ─── figuras básicas ────────────────────────────────────────────────────────

func circulo(x, y float64) bool {
	dx, dy := x-0.5, y-0.5
	return dx*dx+dy*dy <= 0.25
}

func elipse(x, y float64) bool {
	dx, dy := x-0.5, y-0.5
	return (dx*dx)/(0.25*0.98)+(dy*dy)/(0.25*0.55) <= 1
}

// rectRedondeado: rectángulo con esquinas de radio `r` (en fracción del lado).
func rectRedondeado(r float64) func(x, y float64) bool {
	return func(x, y float64) bool {
		if x >= r && x <= 1-r || y >= r && y <= 1-r {
			return x >= 0 && x <= 1 && y >= 0 && y <= 1
		}
		cx := r
		if x > 0.5 {
			cx = 1 - r
		}
		cy := r
		if y > 0.5 {
			cy = 1 - r
		}
		dx, dy := x-cx, y-cy
		return dx*dx+dy*dy <= r*r
	}
}

// poligono regular de `n` lados, con el primer vértice arriba.
func poligono(n int, rotacion float64) func(x, y float64) bool {
	puntos := make([][2]float64, n)
	for i := 0; i < n; i++ {
		ang := rotacion + 2*math.Pi*float64(i)/float64(n)
		puntos[i] = [2]float64{0.5 + 0.5*math.Sin(ang), 0.5 - 0.5*math.Cos(ang)}
	}
	return func(x, y float64) bool { return enPoligono(x, y, puntos) }
}

// estrella de `puntas` puntas con radio interior `interior`.
func estrella(puntas int, interior float64) func(x, y float64) bool {
	puntos := make([][2]float64, puntas*2)
	for i := range puntos {
		ang := 2 * math.Pi * float64(i) / float64(len(puntos))
		r := 0.5
		if i%2 == 1 {
			r = 0.5 * interior
		}
		puntos[i] = [2]float64{0.5 + r*math.Sin(ang), 0.5 - r*math.Cos(ang)}
	}
	return func(x, y float64) bool { return enPoligono(x, y, puntos) }
}

// enPoligono: teorema del rayo (ray casting), el de toda la vida.
func enPoligono(x, y float64, puntos [][2]float64) bool {
	dentro := false
	n := len(puntos)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		xi, yi := puntos[i][0], puntos[i][1]
		xj, yj := puntos[j][0], puntos[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			dentro = !dentro
		}
	}
	return dentro
}

// corazon: la curva clásica (x²+y²−1)³ − x²y³ ≤ 0, girada para que quede
// apuntando hacia abajo como un corazón de verdad.
func corazon(x, y float64) bool {
	cx := (x - 0.5) * 2.6
	cy := (0.5 - y) * 2.6
	cy -= 0.35
	a := cx*cx + cy*cy - 0.6
	return a*a*a-cx*cx*cy*cy*cy*0.6 <= 0
}

// cruz: dos brazos centrados.
func cruz(x, y float64) bool {
	dx, dy := math.Abs(x-0.5), math.Abs(y-0.5)
	return dx <= 0.16 && dy <= 0.5 || dy <= 0.16 && dx <= 0.5
}

// marco: el cuadrado entero con las esquinas apenas redondeadas, para que el
// sticker se vea "enmarcado" en vez de recortado. (En el JS `-f` era un PNG de
// marco que se pegaba encima; aquí es simplemente el contorno del sticker.)
func marco(grosor float64) func(x, y float64) bool {
	return rectRedondeado(grosor)
}

// ─── catálogo ───────────────────────────────────────────────────────────────

// CatalogoMascaras: nombre corto → forma. Los nombres son los del bot de JS,
// para que quien venía usando `.s -c` siga escribiendo lo mismo.
var CatalogoMascaras = map[string]Forma{
	"circulo":    {"circulo", circulo},
	"elipse":     {"elipse", elipse},
	"redondeado": {"redondeado", rectRedondeado(0.18)},
	"cuadrado":   {"cuadrado", rectRedondeado(0.04)},
	"triangulo":  {"triangulo", poligono(3, 0)},
	"rombo":      {"rombo", poligono(4, 0)},
	"pentagono":  {"pentagono", poligono(5, 0)},
	"hexagono":   {"hexagono", poligono(6, 0)},
	"octogono":   {"octogono", poligono(8, math.Pi/8)},
	"estrella":   {"estrella", estrella(5, 0.45)},
	"estrella6":  {"estrella6", estrella(6, 0.55)},
	"corazon":    {"corazon", corazon},
	"cruz":       {"cruz", cruz},
	"marco":      {"marco", marco(0.06)},
}

// Mascara busca una forma por nombre. Acepta también los atajos de una letra
// del JS (`-c` círculo, `-t` triángulo, `-s` estrella, `-v` corazón…).
func Mascara(nombre string) (Forma, bool) {
	if m, ok := CatalogoMascaras[nombre]; ok {
		return m, true
	}
	alias := map[string]string{
		"c": "circulo", "t": "triangulo", "s": "estrella", "r": "redondeado",
		"o": "octogono", "y": "pentagono", "e": "elipse", "z": "cruz",
		"v": "corazon", "d": "rombo", "h": "hexagono", "f": "marco",
	}
	if largo, ok := alias[nombre]; ok {
		if m, ok := CatalogoMascaras[largo]; ok {
			return m, true
		}
	}
	return Forma{}, false
}

// NombresMascaras devuelve las formas en orden alfabético (para la ayuda).
func NombresMascaras() []string {
	out := make([]string, 0, len(CatalogoMascaras))
	for k := range CatalogoMascaras {
		out = append(out, k)
	}
	// orden alfabético sin depender de `sort` en un archivo de dibujo
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
