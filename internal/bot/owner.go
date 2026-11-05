// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// owner.go — Quién es el dueño del bot.
//
// En el JS esto se comparaba a pelo contra el JID y fallaba en dos casos que
// aparecen siempre en producción:
//
//  1. En el .env el número va con "+" o con espacios ("+52 155 1234 5678") y
//     el JID viene limpio ("5215512345678@s.whatsapp.net") → nunca coincidía.
//  2. Los chats nuevos de WhatsApp usan un LID (un identificador interno) en
//     lugar del teléfono, así que `sender.User` no era el número. whatsmeow
//     manda el teléfono real en `Info.SenderAlt`: hay que mirar los dos.
//
// Aquí se normaliza todo a dígitos y se comparan ambos identificadores.
package bot

import (
	"strings"
	"unicode"

	"go.mau.fi/whatsmeow/types"
)

// NormalizeNumber deja solo los dígitos del número: "+52 155-1234-5678",
// "5215512345678@s.whatsapp.net" y "5215512345678:12@s.whatsapp.net" acaban
// todos en "5215512345678".
func NormalizeNumber(s string) string {
	s = strings.TrimSpace(s)
	// corta el dominio del JID y el sufijo de dispositivo (:12)
	if i := strings.IndexAny(s, "@"); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexAny(s, ":"); i >= 0 {
		s = s[:i]
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NormalizeOwners limpia la lista de OWNERS del entorno y quita duplicados.
// Los valores que no tengan ningún dígito se descartan (una coma de más en el
// .env no debe convertir a nadie en owner).
func NormalizeOwners(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		n := NormalizeNumber(raw)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

// IsOwner compara el número del remitente (y su alternativo, el teléfono
// cuando el chat va por LID) contra la lista de owners ya normalizada.
// IsBotOwner comprueba un JID contra la lista de dueños del proceso (la que
// cargó main). Es la misma comprobación que hace el router, expuesta para los
// comandos que tienen que proteger al dueño (nadie puede expulsarlo ni
// degradarlo).
func IsBotOwner(jid types.JID) bool {
	if activeRouter == nil || jid.IsEmpty() {
		return false
	}
	return IsOwner(jid, types.EmptyJID, activeRouter.owner)
}

func IsOwner(jid, alt types.JID, owners []string) bool {
	if len(owners) == 0 {
		return false
	}
	candidates := [2]string{
		NormalizeNumber(jid.User),
		NormalizeNumber(alt.User),
	}
	for _, n := range owners {
		for _, c := range candidates {
			if c != "" && c == n {
				return true
			}
		}
	}
	return false
}
