// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// output.go — Todo lo que el bot mete en un mensaje pasa por aquí.
//
// Hay dos problemas reales que esto resuelve:
//
//  1. **Inyección de formato.** WhatsApp interpreta *negrita*, _cursiva_,
//     `código` y ```bloques```. Un usuario llamado "Shin_Admin" o que escribe
//     `.afk *modo dios*` puede hacer que el aviso del bot salga con formato
//     arbitrario (o que un mensaje del bot parezca otra cosa). Todo dato que
//     venga del usuario se escapa antes de mezclarlo con texto del bot.
//  2. **Menciones falsas.** Escribir "@521555…" en un mensaje NO menciona a
//     nadie; hace falta el JID en el campo `mentions`. Pero al revés sí hay un
//     riesgo: si el bot copia texto con "@" de un usuario, puede parecer que
//     está mencionando a alguien. Se normaliza.
package bot

import (
	"strings"
	"unicode/utf8"
)

// MaxUserText es el tope de texto de usuario que el bot repite en un mensaje.
// Un mensaje de WhatsApp admite ~65 000 caracteres, pero el bot no tiene por
// qué reenviar 60 000: con 1000 sobra para un motivo de afk o el nombre de
// una canción, y evita que alguien use el bot como amplificador.
const MaxUserText = 1000

// Esc escapa el formato de WhatsApp sin destrozar el texto.
//
//   - _ ` ~ ``` → se separan con un espacio cero-ancho… no: se escapan con \
//
// Lo práctico: si el texto tiene comillas invertidas o asteriscos sueltos, se
// cambian por caracteres parecidos (′ ″) que WhatsApp no interpreta. Es más
// legible que un montón de barras invertidas y no altera lo que quiso escribir
// la persona.
func Esc(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '*':
			b.WriteRune('∗') // asterisco de operador: no es negrita
		case '_':
			b.WriteRune('‗')
		case '`':
			b.WriteRune('´')
		case '~':
			b.WriteRune('∼')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CleanInvisible quita caracteres de control e invisibles sin escapar el
// formato: es para ANALIZAR texto (detectar enlaces), no para mostrarlo.
func CleanInvisible(s string) string {
	if s == "" {
		return ""
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r < 0x20 && r != '\n' && r != '\t':
			return -1
		case r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E, r == 0x2060, r == 0xFEFF:
			return -1
		case r == 0x00A0:
			return ' '
		}
		return r
	}, s)
}

// CleanName prepara un nombre de usuario para mostrarlo: quita saltos de
// línea, controles, invisibles y limita el largo. Un PushName puede traer
// cualquier cosa (es texto libre del perfil de la persona).
func CleanName(name string, max int) string {
	if max <= 0 {
		max = 32
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7F:
			return -1
		case r >= 0x200B && r <= 0x200F, r >= 0x202A && r <= 0x202E, r == 0x2060, r == 0xFEFF:
			return -1 // invisibles: sirven para falsificar nombres
		}
		return r
	}, name)

	name = strings.Join(strings.Fields(name), " ") // espacios colapsados
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > max {
		name = string([]rune(name)[:max]) + "…"
	}
	return Esc(name)
}

// ClipText limpia un texto libre del usuario y lo deja listo para mostrar.
// Se usa para motivos de afk, argumentos de comandos, etc.
func ClipText(s string, max int) string {
	if max <= 0 {
		max = MaxUserText
	}
	s = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > max {
		s = strings.TrimRight(string([]rune(s)[:max]), " ") + "…"
	}
	return Esc(s)
}

// Chunk parte un texto largo en trozos que quepan en un mensaje sin cortar
// palabras. `size` es en caracteres (no bytes) y se limita a 3500 por defecto
// para no acercarse al máximo de WhatsApp (que además se reduce al añadir
// formato, saltos y algún que otro escape).
func Chunk(text string, size int) []string {
	if size <= 0 || size > 3500 {
		size = 3500
	}
	if utf8.RuneCountInString(text) <= size {
		return []string{text}
	}

	var out []string
	var cur strings.Builder
	curLen := 0

	for _, line := range strings.Split(text, "\n") {
		runes := []rune(line)
		// Línea sola más larga que el trozo: se parte a lo bruto.
		for len(runes) > size {
			if curLen > 0 {
				out = append(out, cur.String())
				cur.Reset()
				curLen = 0
			}
			out = append(out, string(runes[:size]))
			runes = runes[size:]
		}
		add := string(runes)
		if curLen+len(runes)+1 > size && curLen > 0 {
			out = append(out, cur.String())
			cur.Reset()
			curLen = 0
		}
		if curLen > 0 {
			cur.WriteByte('\n')
			curLen++
		}
		cur.WriteString(add)
		curLen += len(runes)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

// IsSafeCommandName: el router solo acepta nombres de comando con letras,
// números, guion y guion bajo. Así un "comando" no puede colarse como ruta,
// como clave de la base o como parte de una URL.
func IsSafeCommandName(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}
