// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// catalogo.go — Objetos de la tienda. En el JS estaban repartidos entre
// cmds/economy/shop.js y cmds/economy/invoke.js, con los precios escritos a
// mano en cada uno (y ya se habían desincronizado). Aquí hay una sola lista:
// si cambia un precio, cambia en el comando y en el inventario a la vez.
package store

import "strings"

// Kind clasifica el objeto para que los comandos sepan qué se puede usar.
const (
	KindWeapon     = "arma"
	KindTool       = "herramienta"
	KindConsumible = "consumible"
)

// Item es un objeto comprable.
type Item struct {
	ID    string
	Name  string
	Emoji string
	Price int64
	Kind  string
	Uses  int64 // usos que aguanta; 0 = no se gasta
	Desc  string
}

// Items es el catálogo cerrado. El orden de la tienda lo fija ItemsOrder.
var Items = map[string]Item{
	"espada": {ID: "espada", Name: "Espada", Emoji: "⚔️", Price: 8000, Kind: KindWeapon, Desc: "La más cara y la que más daño hace."},
	"hacha":  {ID: "hacha", Name: "Hacha", Emoji: "🪓", Price: 7500, Kind: KindWeapon, Desc: "Buena para talar y para pelear."},
	"arco":   {ID: "arco", Name: "Arco", Emoji: "🏹", Price: 7000, Kind: KindWeapon, Desc: "Pega desde lejos."},
	"pico":   {ID: "pico", Name: "Pico", Emoji: "⛏️", Price: 6500, Kind: KindTool, Desc: "Necesario para .mine."},
	"cana":   {ID: "cana", Name: "Caña de pescar", Emoji: "🎣", Price: 6000, Kind: KindTool, Uses: 0, Desc: "Necesaria para .fish."},
	"totem":  {ID: "totem", Name: "Tótem", Emoji: "🗿", Price: 4000, Kind: KindConsumible, Uses: 3, Desc: "3 usos: recupera salud y magia."},
	"pocion": {ID: "pocion", Name: "Poción", Emoji: "🧪", Price: 1500, Kind: KindConsumible, Uses: 1, Desc: "1 uso: cura al instante."},
}

// ItemsOrder: el orden en que se pintan en .shop.
var ItemsOrder = []string{"espada", "hacha", "arco", "pico", "cana", "totem", "pocion"}

// ValidItem dice si un id existe en el catálogo. Todo lo que venga de un
// mensaje pasa por aquí antes de tocar el inventario.
func ValidItem(id string) bool {
	_, ok := Items[id]
	return ok
}

// FindItem busca por id o por nombre visible ("Espada", "caña de pescar").
// Devuelve el id canónico.
func FindItem(q string) (string, bool) {
	if ValidItem(q) {
		return q, true
	}
	for _, id := range ItemsOrder {
		if normalizeName(Items[id].Name) == normalizeName(q) {
			return id, true
		}
	}
	return "", false
}

// normalizeName pasa a minúsculas sin acentos ni espacios de más, para que
// ".shop caña de pescar" y ".shop cana de pescar" compren lo mismo.
func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	out := make([]rune, 0, len(s))
	prevSpace := false
	for _, r := range s {
		switch r {
		case 'á', 'à', 'ä', 'â', 'ã':
			r = 'a'
		case 'é', 'è', 'ë', 'ê':
			r = 'e'
		case 'í', 'ì', 'ï', 'î':
			r = 'i'
		case 'ó', 'ò', 'ö', 'ô', 'õ':
			r = 'o'
		case 'ú', 'ù', 'ü', 'û':
			r = 'u'
		case 'ñ':
			r = 'n'
		case 'ç':
			r = 'c'
		case ' ':
			if prevSpace || len(out) == 0 {
				continue
			}
			prevSpace = true
		default:
			prevSpace = false
		}
		out = append(out, r)
	}
	// Quitar el espacio final si lo hubiera.
	for len(out) > 0 && out[len(out)-1] == ' ' {
		out = out[:len(out)-1]
	}
	return string(out)
}
