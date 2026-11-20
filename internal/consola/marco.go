// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// marco.go — la casa dibujada como se ve en el resto del proyecto.
//
// Reglas, y por qué:
//
//	· Ancho máximo 58 columnas. La mitad de un móvil en vertical son ~40,
//	  pero 58 entra en cualquier terminal de verdad sin partir nada. Depender
//	  del ancho real es lo que hacía que Termux (50) saliera con líneas
//	  partida a mitad de palabra.
//	· Nada de arte que ocupe media pantalla: el panel se lee de un vistazo y
//	  deja sitio al registro del bot.
//	· Bordes de una sola columna (│ ╭ ╮  ╯), nunca bloques: █ y ▓ se ven mal
//	  con la fuente de Termux y descuadran el ancho.
//	· Sin color (systemd, pm2, `> log.txt`) todo esto sigue siendo texto
//	  alineado, porque el relleno se calcula sobre el ancho VISIBLE.
package consola

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// AnchoCasa: el ancho de las cajas y de la pantalla.
const AnchoCasa = 58

// encajar limita un ancho al rango útil: por debajo de 40 columnas no hay forma
// de alinear nada, y por encima de 100 una caja de 58 queda perdida en el
// desierto (se centra, no se estira).
func encajar(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

// AnchoFijo: el ancho con el que se dibuja (58, o el de la pantalla si es
// menor). Es lo que deben consultar los que componen líneas.
func AnchoFijo() int {
	return encajar(Ancho(), 40, AnchoCasa)
}

// Marco: una caja de una sola columna de borde, con título en el borde de
// arriba y el interior rellenado a la derecha (así el borde de la derecha no
// baila aunque las líneas midan distinto).
func Marco(titulo string, lineas []string, ancho int) []string {
	ancho = encajar(ancho, 40, 100)
	interior := ancho - 4
	rotulo := ""
	if titulo != "" {
		rotulo = " " + titulo + " "
	}
	guiones := strings.Repeat("─", maxInt(0, ancho-2-anchoVisible(rotulo)))
	arriba := stLila.Sprint("╭") + rotulo + stLila.Sprint(guiones+"╮")
	abajo := stLila.Sprint("╰" + strings.Repeat("─", ancho-2) + "╯")
	out := []string{arriba}
	for _, l := range lineas {
		if strings.TrimSpace(stripANSI(l)) == "" {
			out = append(out, stLila.Sprint("│")+" "+strings.Repeat(" ", interior)+" "+stLila.Sprint("│"))
			continue
		}
		// Se respeta el espaciado de la línea tal cual; solo se envuelve lo que
		// de verdad no cabe (si no, el envoltorio se come las sangrías).
		for _, trozo := range partir(l, interior) {
			out = append(out, stLila.Sprint("│")+" "+rellenar(trozo, interior)+" "+stLila.Sprint("│"))
		}
	}
	return append(out, abajo)
}

// Fila: «etiqueta … valor» con el valor alineado a la derecha, dentro de una
// caja de `ancho` columnas. Es la forma canónica de enseñar un dato: etiqueta
// tenue, valor en color de papel.
func Fila(etiqueta, valor string, ancho int) string {
	interior := encajar(ancho, 40, 100) - 4
	izq := stTenue.Sprint(etiqueta)
	der := stPapel.Sprint(valor)
	hueco := maxInt(1, interior-anchoVisible(izq)-anchoVisible(der))
	return izq + strings.Repeat(" ", hueco) + der
}

// Centrar dentro de `ancho` columnas visibles.
func Centrar(s string, ancho int) string {
	hueco := maxInt(0, encajar(ancho, 20, 200)-anchoVisible(s))
	izq := hueco / 2
	return strings.Repeat(" ", izq) + s + strings.Repeat(" ", hueco-izq)
}

// Espaciar mete un espacio entre cada carácter. Es como se enseña el código de
// vinculación: se lee sin confundir la R con la 2, y no ocupa las 6 filas de
// un banner de letras gigantes.
func Espaciar(s string) string {
	return strings.Join(strings.Split(s, ""), " ")
}

// Truncar: corta al ancho visible y avisa con «…» de que había más.
func Truncar(s string, ancho int) string { return truncarANSI(s, ancho) }

func anchoVisible(s string) int { return runewidth.StringWidth(stripANSI(s)) }

func rellenar(s string, ancho int) string {
	if h := ancho - anchoVisible(s); h > 0 {
		return s + strings.Repeat(" ", h)
	}
	return s
}

// partir envuelve sin cortar palabras, sobre el ancho VISIBLE (los códigos de
// color no cuentan: medir sobre texto coloreado salía el doble de ancho).
//
// La sangría de la izquierda se conserva en TODOS los trozos. Es el detalle que
// en el JS costó aprenderse: envolver trabaja por palabras y se comía los
// espacios iniciales, así que un código o un texto centrado quedaba pegado al
// borde del cuadro en cuanto la línea no cabía entera.
func partir(s string, ancho int) []string {
	if ancho <= 0 || anchoVisible(s) <= ancho {
		return []string{s}
	}
	sangria := sangriaIzq(s)
	anchoUtl := maxInt(8, ancho-anchoVisible(sangria))
	trozos := envolver(strings.TrimLeft(s, " \t"), anchoUtl)
	for i, t := range trozos {
		trozos[i] = Truncar(sangria+t, ancho)
	}
	return trozos
}

// sangriaIzq: los espacios (o tabuladores) con los que empieza la línea,
// saltándose los códigos de color que puedan venir delante.
func sangriaIzq(s string) string {
	i := 0
	for i < len(s) {
		if s[i] == 0x1b { // secuencia CSI: no es contenido, se salta
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && !(s[j] >= '@' && s[j] <= '~') {
					j++
				}
				if j < len(s) {
					j++
				}
			}
			i = j
			continue
		}
		if s[i] == ' ' || s[i] == '\t' {
			i++
			continue
		}
		break
	}
	return s[:i]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SinColor quita los códigos ANSI. Se exporta para quien necesite el texto
// pelado: el log a fichero, el panel web o una prueba que quiere comparar
// palabras y no secuencias de escape.
func SinColor(s string) string { return stripANSI(s) }
