// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// morse.go — Código morse, de ida y vuelta.
//
// En el JS era un objeto de 45 entradas y dos `map`. Aquí es lo mismo, pero con
// una diferencia que importa: el JS devolvía el carácter tal cual cuando no
// sabía codificarlo (así que un emoji salía «crasheado» dentro del morse) y al
// decodificar escribía «?» sin avisar. Aquí se dice cuántos caracteres no se
// pudieron convertir, y el que llama decide qué hacer con eso.
package utilidades

import (
	"strings"
)

// morseTexto es la tabla oficial (ITU-R M.1677-1) más los signos que usa la
// gente al escribir.
var morseTexto = map[rune]string{
	'A': ".-", 'B': "-...", 'C': "-.-.", 'D': "-..", 'E': ".", 'F': "..-.",
	'G': "--.", 'H': "....", 'I': "..", 'J': ".---", 'K': "-.-", 'L': ".-..",
	'M': "--", 'N': "-.", 'O': "---", 'P': ".--.", 'Q': "--.-", 'R': ".-.",
	'S': "...", 'T': "-", 'U': "..-", 'V': "...-", 'W': ".--", 'X': "-..-",
	'Y': "-.--", 'Z': "--..",
	'0': "-----", '1': ".----", '2': "..---", '3': "...--", '4': "....-",
	'5': ".....", '6': "-....", '7': "--...", '8': "---..", '9': "----.",
	'.': ".-.-.-", ',': "--..--", '?': "..--..", '\'': ".----.", '!': "-.-.--",
	'/': "-..-.", '(': "-.--.", ')': "-.--.-", '&': ".-...", ':': "---...",
	';': "-.-.-.", '=': "-...-", '+': ".-.-.", '-': "-....-", '_': "..--.-",
	'"': ".-..-.", '$': "...-..-", '@': ".--.-.",
}

// morseCodigo es la tabla al revés, construida una sola vez al arrancar.
var morseCodigo = func() map[string]rune {
	m := make(map[string]rune, len(morseTexto))
	for r, codigo := range morseTexto {
		m[codigo] = r
	}
	return m
}()

// ASimbolo convierte texto a morse. Las palabras se separan con «/», igual que
// en el JS (es la convención que se lee bien en un chat).
//
// Devuelve también cuántos caracteres se quedaron fuera: si alguien manda
// «hola 😀», el resultado no puede fingir que el emoji estaba ahí.
func ASimbolo(texto string) (codigo string, fuera int) {
	texto = strings.ToUpper(strings.TrimSpace(texto))
	if texto == "" {
		return "", 0
	}
	partes := make([]string, 0, len(texto))
	for _, r := range texto {
		switch {
		case r == ' ' || r == '\n' || r == '\t':
			partes = append(partes, "/")
		case morseTexto[r] != "":
			partes = append(partes, morseTexto[r])
		default:
			fuera++
		}
	}
	// Los espacios repetidos dejan «/ /»: se juntan en uno solo para que el
	// mensaje no salga con huecos.
	salida := strings.Join(partes, " ")
	for strings.Contains(salida, "/ /") {
		salida = strings.ReplaceAll(salida, "/ /", "/")
	}
	return salida, fuera
}

// ATexto decodifica morse. Separa por espacios y usa «/» como espacio entre
// palabras, que es como lo escribe `.morse`.
//
// Los grupos que no se entienden se dejan como «?» y se cuentan aparte.
func ATexto(codigo string) (texto string, fuera int) {
	var b strings.Builder
	for _, grupo := range strings.Fields(strings.TrimSpace(codigo)) {
		if grupo == "/" || grupo == "|" {
			b.WriteRune(' ')
			continue
		}
		if r, ok := morseCodigo[grupo]; ok {
			b.WriteRune(r)
			continue
		}
		b.WriteRune('?')
		fuera++
	}
	return strings.ToLower(b.String()), fuera
}
