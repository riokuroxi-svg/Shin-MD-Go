// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// links.go — Detección de enlaces para el antilink.
//
// No se usa una expresión regular gigante con dominios de todo internet: eso
// envejece mal y bloquea texto normal. La estrategia es por capas:
//
//  1. Esquemas evidentes (http://, https://, www.) y dominios de invitación
//     (chat.whatsapp.com, t.me, wa.me…).
//  2. Cualquier cosa que parezca un dominio con una extensión real.
//  3. Se ignoran las coincidencias dentro de correos electrónicos (nadie
//     quiere que el bot borre un email) y los números de teléfono.
package bot

import (
	"regexp"
	"strings"
	"unicode"
)

// Dominios que siempre se consideran enlace, aparezcan como aparezcan.
var hardLinks = []string{
	"chat.whatsapp.com", "wa.me", "whatsapp.com", "whatsapp://",
	"t.me", "telegram.me", "telegram.org", "discord.gg", "discord.com",
	"bit.ly", "tinyurl.com", "cutt.ly", "shorturl.at",
	// redes que suelen usar los spammers de grupos
	"instagram.com", "facebook.com", "fb.me", "tiktok.com", "twitter.com",
	"x.com", "youtube.com", "youtu.be", "onlyfans.com",
}

// Extensiones que de verdad se usan en enlaces. Lista corta a propósito: si
// se abre demasiado, "informe.final" o "v1.2" empiezan a borrarse solos.
var tlds = []string{
	"com", "net", "org", "io", "me", "ly", "gg", "tv", "info", "xyz", "link",
	"site", "app", "dev", "shop", "store", "online", "club", "top", "biz",
	"co", "es", "mx", "ar", "pe", "cl", "co", "br", "us", "tk", "ml", "ga", "cf", "gq", "ru", "cn",
}

// dominio.tld, opcionalmente con ruta. Se exige al menos 2 caracteres antes
// del punto para no comerse abreviaturas.
var domainRe = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9-]{1,61}\.(` + strings.Join(tlds, "|") + `)\b(/[^\s]*)?`)

// emailRe: "algo@dominio.com" es un correo, no un enlace. Se quita antes de
// analizar para no borrar mensajes legítimos.
var emailRe = regexp.MustCompile(`(?i)\b[\w.+-]+@[\w-]+\.[a-z]{2,}\b`)

// invisibleRe quita caracteres que se usan para disfrazar enlaces
// (ancho cero, marcas de dirección).
var invisibleRe = regexp.MustCompile(`[\x{200b}-\x{200f}\x{202a}-\x{202e}\x{2060}\x{feff}]`)

// HasLink decide si un texto contiene un enlace.
func HasLink(text string) bool {
	clean := NormalizeText(text)
	if clean == "" {
		return false
	}

	// Se eliminan los correos: si queda algo con forma de dominio, es enlace.
	clean = emailRe.ReplaceAllString(clean, " ")

	lower := strings.ToLower(clean)
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") ||
		strings.Contains(lower, "www.") {
		return true
	}
	for _, d := range hardLinks {
		if strings.Contains(lower, d) {
			return true
		}
	}
	return domainRe.MatchString(lower)
}

// NormalizeText prepara el texto: quita invisibles, pasa los puntos y comas
// que se usan como disfraz ("punto com" → ".com" solo si van con espacios no,
// sería demasiado agresivo) y recorta.
func NormalizeText(text string) string {
	text = invisibleRe.ReplaceAllString(text, "")
	text = strings.Map(func(r rune) rune {
		if r == '\u00a0' { // espacio duro que copia WhatsApp de la web
			return ' '
		}
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, text)
	return strings.TrimSpace(text)
}

// LinkCount devuelve cuántos dominios distintos hay (para warns futuros).
func LinkCount(text string) int {
	clean := emailRe.ReplaceAllString(NormalizeText(text), " ")
	seen := map[string]bool{}
	for _, m := range domainRe.FindAllString(strings.ToLower(clean), -1) {
		seen[m] = true
	}
	return len(seen)
}
