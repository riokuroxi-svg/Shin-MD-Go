// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// xp.go — Cuánta experiencia pide cada nivel, en un solo sitio.
//
// EL PROBLEMA QUE RESUELVE
// En el bot de JS esta fórmula vivía copiada en cuatro comandos (level,
// profile, profile/level, lboard) y, peor, con DOS curvas distintas convivían:
// el perfil usaba CRECIMIENTO y `.level` usaba CRECIMIENTO_LEVEL (×2 de curva).
// Un mismo usuario veía dos niveles diferentes según el comando que escribiera.
// El propio archivo del JS lo dejaba anotado como pendiente de decidir.
//
// DECISIÓN TOMADA AQUÍ: una sola curva, la del perfil (CRECIMIENTO), que es la
// que usaban tres de los cuatro comandos y la única que ve el usuario en su
// ficha. El nivel que se guarda en la base y el que se enseña salen siempre de
// esta función, así que no pueden volver a discrepar.
//
// Y LOS NIVELES EMPIEZAN EN 1. La curva del JS admite un nivel 0, pero un
// usuario recién creado con «nivel 0» ya fue un fallo cazado aquí: la cuenta
// nueva del JS nace en 1 y la de Go nacía en 0, así que el perfil decía una
// cosa y la base otra. El tramo del nivel 1 absorbe el del 0 (empieza en 0 de
// XP), de modo que quien no ha hecho nada está en el nivel 1 al 0 % en vez de
// en un nivel que no existe.
//
// Los números NO son inventados: son la fórmula que ya tenía el bot, con el
// multiplicador global (2) que estaba fijo en el JS.
package xp

import "math"

// crecimiento es el exponente de la curva del perfil en el JS:
// (π/e)^1.618 · e · 0.75 ≈ 1.5120…
var crecimiento = math.Pow(math.Pi/math.E, 1.618) * math.E * 0.75

// Minimo es el primer nivel que puede tener una persona.
const Minimo = 1

// Multiplicador es el `global.multiplier` del bot de JS. Se deja como constante
// y no como global mutable: si algún día hay eventos de XP doble, se pasa como
// argumento explícito en vez de cambiando una variable del proceso.
const Multiplicador = 2.0

// Rango es el tramo de experiencia de un nivel.
type Rango struct {
	Level int   // nivel al que pertenece el tramo
	Min   int64 // XP mínimo para estar en ese nivel
	Max   int64 // XP que pide el siguiente nivel
}

// Ancho: cuánta XP hay que ganar dentro de este nivel (Max - Min).
func (r Rango) Ancho() int64 {
	if r.Max <= r.Min {
		return 0
	}
	return r.Max - r.Min
}

// De devuelve el tramo de un nivel. Un nivel por debajo del mínimo se trata
// como el mínimo: el JS lanzaba excepción con un nivel negativo y eso tumbaba
// el comando entero por un dato corrupto de la base.
func De(nivel int) Rango {
	if nivel < Minimo {
		nivel = Minimo
	}
	// El primer nivel arranca en 0 de XP: se lleva por delante el tramo del
	// «nivel 0» que no debe existir.
	var min int64
	if nivel > Minimo {
		min = round(math.Pow(float64(nivel), crecimiento)*Multiplicador) + 1
	}
	max := round(math.Pow(float64(nivel+1), crecimiento) * Multiplicador)
	if max < min {
		max = min
	}
	return Rango{Level: nivel, Min: min, Max: max}
}

// NivelPara calcula el nivel que corresponde a una cantidad de XP. Recorre la
// curva hacia arriba con un tope: una XP absurda (base migrada a mano) no debe
// dejar el bot calculando para siempre.
func NivelPara(total int64) int {
	if total < 0 {
		return Minimo
	}
	const tope = 10_000 // niveles; la curva llega ahí con una XP astronómica
	for nivel := Minimo; nivel < tope; nivel++ {
		if total < De(nivel+1).Min {
			return nivel
		}
	}
	return tope
}

// Progreso: cuánta XP lleva dentro de su nivel y qué porcentaje es.
type Progreso struct {
	Level      int
	Dentro     int64 // XP ganada dentro del nivel (para la barra)
	Necesaria  int64 // XP que pide el nivel completo
	Porcentaje int   // 0..100
	Total      int64 // XP acumulada
}

// Calcular devuelve el progreso de una XP acumulada.
func Calcular(total int64) Progreso {
	if total < 0 {
		total = 0
	}
	nivel := NivelPara(total)
	r := De(nivel)
	dentro := total - r.Min
	if dentro < 0 {
		dentro = 0
	}
	necesaria := r.Ancho()
	if necesaria <= 0 {
		return Progreso{Level: nivel, Dentro: dentro, Necesaria: 0, Porcentaje: 100, Total: total}
	}
	pct := int(math.Floor(float64(dentro) / float64(necesaria) * 100))
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	return Progreso{Level: nivel, Dentro: dentro, Necesaria: necesaria, Porcentaje: pct, Total: total}
}

// Barra dibuja la barra de progreso con los mismos caracteres que el JS.
func Barra(p Progreso, ancho int) string {
	if ancho <= 0 {
		ancho = 10
	}
	llenos := p.Porcentaje * ancho / 100
	if llenos > ancho {
		llenos = ancho
	}
	out := make([]rune, 0, ancho*2)
	for i := 0; i < ancho; i++ {
		if i < llenos {
			out = append(out, '▰')
		} else {
			out = append(out, '▱')
		}
	}
	return string(out)
}

// round replica Math.round del JS: .5 hacia arriba, también en negativos.
func round(v float64) int64 {
	if v < 0 {
		return -int64(math.Floor(-v + 0.5))
	}
	return int64(math.Floor(v + 0.5))
}
