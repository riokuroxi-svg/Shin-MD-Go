// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// catalogo.go — Lectura del catálogo de personajes (characters.json).
//
// El bot de JS tiene `loadCharacters` copiada **18 veces** (5 variantes) dentro
// de cmds/gacha/ y todas leían `./core/characters.json`, un archivo que no
// está en el repositorio: un clon limpio tenía el gacha entero roto. Aquí hay
// una sola lectura, con caché por fecha de modificación, y si el archivo falta
// se dice UNA vez y se sigue con el catálogo vacío.
//
// Además el formato se acepta en varias formas, porque el JSON que usa el bot
// de producción ha cambiado con el tiempo y nadie documentó cuál es la buena:
//
//	{"serie": {"name": "Naruto", "characters": [ {...}, ... ]}}   ← el del JS
//	[ {"id": 1, "name": "..."} , ... ]                            ← lista suelta
//	[ {"name": "Naruto", "characters": [...]} , ... ]             ← lista de series
package gacha

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Character es un personaje del catálogo.
type Character struct {
	ID     string
	Name   string
	Series string
	Gender string
	Value  int64
	Tags   []string
	Image  string // URL de imagen si el catálogo la trae
}

// Catalog es el catálogo ya normalizado.
type Catalog struct {
	Characters []Character
	byID       map[string]Character
}

// Len: cuántos personajes hay.
func (c *Catalog) Len() int {
	if c == nil {
		return 0
	}
	return len(c.Characters)
}

// Empty: catálogo vacío (sin archivo o archivo ilegible).
func (c *Catalog) Empty() bool { return c.Len() == 0 }

// ByID busca un personaje. El id se compara como texto: en el JS conviven ids
// numéricos y de texto y comparar con === fallaba entre unos y otros.
func (c *Catalog) ByID(id string) (Character, bool) {
	if c == nil {
		return Character{}, false
	}
	ch, ok := c.byID[strings.TrimSpace(id)]
	return ch, ok
}

// Random saca un personaje al azar.
func (c *Catalog) Random(rnd *rand.Rand) (Character, bool) {
	if c.Empty() {
		return Character{}, false
	}
	return c.Characters[rnd.Intn(len(c.Characters))], true
}

// ─── lectura con caché ──────────────────────────────────────────────────────

type cacheEntry struct {
	path  string
	mtime time.Time
	size  int64
	cat   *Catalog
}

var (
	cacheMu  sync.Mutex
	cache    *cacheEntry
	avisoUna sync.Once
)

// Load lee el catálogo y lo cachea mientras el archivo no cambie.
//
// No devuelve error por "archivo ausente": eso no es un fallo del comando, es
// configuración que falta. Se avisa una sola vez por texto y se sigue con el
// catálogo vacío, para que el comando conteste algo en vez de reventar.
func Load(path string) (*Catalog, string) {
	if strings.TrimSpace(path) == "" {
		return &Catalog{}, "no hay ruta de catálogo configurada (CHARACTERS_FILE)"
	}

	st, err := os.Stat(path)
	if err != nil {
		return &Catalog{}, fmt.Sprintf("no se pudo leer el catálogo %q: %v", path, err)
	}

	cacheMu.Lock()
	defer cacheMu.Unlock()

	if cache != nil && cache.path == path && cache.mtime.Equal(st.ModTime()) && cache.size == st.Size() {
		return cache.cat, ""
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return &Catalog{}, fmt.Sprintf("no se pudo leer el catálogo %q: %v", path, err)
	}
	cat, err := Parse(raw)
	if err != nil {
		return &Catalog{}, fmt.Sprintf("el catálogo %q está mal formado: %v", path, err)
	}

	cache = &cacheEntry{path: path, mtime: st.ModTime(), size: st.Size(), cat: cat}
	return cat, ""
}

// WarnOnce deja pasar el aviso del catálogo una única vez por arranque.
func WarnOnce() bool {
	fired := false
	avisoUna.Do(func() { fired = true })
	return fired
}

// ─── parseo ─────────────────────────────────────────────────────────────────

// Parse normaliza cualquiera de los formatos aceptados.
func Parse(raw []byte) (*Catalog, error) {
	cat := &Catalog{byID: map[string]Character{}}

	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return cat, err
	}
	switch body := v.(type) {
	case map[string]any:
		// {"serie": {...}} — el formato del JS. Si el objeto no tiene la
		// pinta de serie, se prueba como mapa id → personaje.
		if _, ok := body["characters"]; ok {
			cat.addSeries(body, "")
		} else {
			claves := make([]string, 0, len(body))
			for k := range body {
				claves = append(claves, k)
			}
			sort.Strings(claves)
			for _, k := range claves {
				if sub, ok := body[k].(map[string]any); ok {
					cat.addSeries(sub, k)
				}
			}
		}
	case []any:
		for _, item := range body {
			switch it := item.(type) {
			case map[string]any:
				if _, tieneHijos := it["characters"]; tieneHijos {
					cat.addSeries(it, "")
				} else {
					cat.addCharacter(it, "")
				}
			}
		}
	}

	cat.finish()
	if len(cat.Characters) == 0 {
		return cat, fmt.Errorf("no se encontró ningún personaje")
	}
	return cat, nil
}

func (c *Catalog) addSeries(s map[string]any, fallbackName string) {
	nombre := texto(s["name"], texto(s["nombre"], texto(s["serie"], fallbackName)))
	list, ok := s["characters"].([]any)
	if !ok {
		return
	}
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			c.addCharacter(m, nombre)
		}
	}
}

func (c *Catalog) addCharacter(m map[string]any, serie string) {
	nombre := texto(m["name"], texto(m["nombre"], ""))
	if nombre == "" {
		return
	}
	id := ident(m["id"], ident(m["charId"], ""))
	if id == "" {
		// Sin id no se puede reclamar ni distinguir dos personajes con el mismo
		// nombre: se usa el nombre, que es estable en el catálogo.
		id = strings.ToLower(nombre)
	}

	ch := Character{
		ID:     id,
		Name:   nombre,
		Series: texto(m["series"], texto(m["serie"], serie)),
		Gender: texto(m["gender"], texto(m["genero"], "Desconocido")),
		Value:  numero(m["value"], numero(m["valor"], 100)),
		Tags:   listaTexto(m["tags"]),
		Image:  texto(m["image"], texto(m["img"], texto(m["url"], ""))),
	}
	if ch.Value < 0 {
		ch.Value = 0
	}
	c.Characters = append(c.Characters, ch)
}

// finish ordena (para que el índice sea reproducible) y quita duplicados.
func (c *Catalog) finish() {
	vistos := make(map[string]bool, len(c.Characters))
	limpio := make([]Character, 0, len(c.Characters))
	for _, ch := range c.Characters {
		if vistos[ch.ID] {
			continue
		}
		vistos[ch.ID] = true
		limpio = append(limpio, ch)
	}
	sort.Slice(limpio, func(i, j int) bool { return limpio[i].ID < limpio[j].ID })
	c.Characters = limpio
	if c.byID == nil {
		c.byID = make(map[string]Character, len(limpio))
	}
	for _, ch := range limpio {
		c.byID[ch.ID] = ch
	}
}

// ─── ayudas de conversión ───────────────────────────────────────────────────

func texto(v any, def string) string {
	if s, ok := v.(string); ok {
		s = strings.TrimSpace(s)
		if s != "" {
			return s
		}
	}
	return def
}

// ident convierte un id a texto. En el catálogo vienen números (1), texto ("1")
// y a veces ids con nombre ("sakura"); todo se normaliza a texto para que
// comparar sea siempre igual.
func ident(v any, def string) string {
	switch n := v.(type) {
	case string:
		if s := strings.TrimSpace(n); s != "" {
			return s
		}
	case float64:
		// Los ids son enteros: 12 y no 12.000000.
		if n == float64(int64(n)) {
			return fmt.Sprintf("%d", int64(n))
		}
		return fmt.Sprintf("%g", n)
	case json.Number:
		return n.String()
	}
	return def
}

func numero(v any, def int64) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(n), "%g", &f); err == nil {
			return int64(f)
		}
	}
	return def
}

func listaTexto(v any) []string {
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}
