// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// catalogo_test.go — El catálogo tiene que aceptar lo que hay ahí fuera.
//
// Estas pruebas existen porque en el JS el formato nunca se documentó: 18
// copias de `loadCharacters` leían un archivo que ni siquiera estaba en el
// repo. Aquí se fija por escrito qué formas se aceptan y qué pasa cuando falta
// el archivo.
package gacha

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestParseFormatoDelJS(t *testing.T) {
	// {"serie": {"name": ..., "characters": [...]}} es el formato real.
	raw := []byte(`{
		"Naruto": {
			"name": "Naruto",
			"characters": [
				{"id": 1, "name": "Sakura", "value": 500, "gender": "Femenino", "tags": ["sakura_haruno"]},
				{"id": 2, "name": "Hinata", "value": 900}
			]
		},
		"Bleach": {
			"name": "Bleach",
			"characters": [
				{"id": "3", "name": "Rukia", "value": 300}
			]
		}
	}`)

	cat, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cat.Len() != 3 {
		t.Fatalf("personajes = %d, se esperaban 3", cat.Len())
	}

	sakura, ok := cat.ByID("1")
	if !ok {
		t.Fatal("no encontró el id 1")
	}
	if sakura.Name != "Sakura" || sakura.Series != "Naruto" || sakura.Value != 500 {
		t.Fatalf("Sakura salió mal: %+v", sakura)
	}
	if sakura.Gender != "Femenino" || len(sakura.Tags) != 1 || sakura.Tags[0] != "sakura_haruno" {
		t.Fatalf("género/tags mal: %+v", sakura)
	}
	// La serie se hereda del bloque padre cuando el personaje no la trae.
	if h, _ := cat.ByID("2"); h.Series != "Naruto" {
		t.Fatalf("Hinata debería ser de Naruto: %+v", h)
	}
	// Y sin género se pone "Desconocido", como el JS.
	if h, _ := cat.ByID("2"); h.Gender != "Desconocido" {
		t.Fatalf("género por defecto = %q", h.Gender)
	}
	// Los ids se comparan como texto: 3 y "3" son el mismo.
	if _, ok := cat.ByID("3"); !ok {
		t.Fatal("no encontró el id 3")
	}
}

func TestParseListaSueltaYListaDeSeries(t *testing.T) {
	suelta := []byte(`[{"id": 10, "name": "Asuka"}, {"id": 11, "name": "Rei"}]`)
	cat, err := Parse(suelta)
	if err != nil {
		t.Fatalf("lista suelta: %v", err)
	}
	if cat.Len() != 2 {
		t.Fatalf("personajes = %d", cat.Len())
	}

	series := []byte(`[{"name": "Evangelion", "characters": [{"id": 12, "name": "Misato"}]}]`)
	cat, err = Parse(series)
	if err != nil {
		t.Fatalf("lista de series: %v", err)
	}
	if m, ok := cat.ByID("12"); !ok || m.Series != "Evangelion" {
		t.Fatalf("Misato salió mal: %+v", m)
	}
}

func TestParseSinIDUsaElNombre(t *testing.T) {
	cat, err := Parse([]byte(`[{"name": "Saber", "value": 1000}]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, ok := cat.ByID("saber"); !ok {
		t.Fatal("sin id debía quedar el nombre en minúsculas como id")
	}
}

// Los valores vienen a veces como texto ("500") y a veces como número (500).
func TestParseValoresMixtos(t *testing.T) {
	cat, err := Parse([]byte(`[{"id":"a","name":"A","value":"1500"},{"id":"b","name":"B","value":2500}]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if a, _ := cat.ByID("a"); a.Value != 1500 {
		t.Fatalf("valor de texto = %d", a.Value)
	}
	if b, _ := cat.ByID("b"); b.Value != 2500 {
		t.Fatalf("valor numérico = %d", b.Value)
	}
}

func TestParseQuitaDuplicados(t *testing.T) {
	cat, err := Parse([]byte(`[{"id": 1, "name": "Uno"}, {"id": 1, "name": "Otra vez"}]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cat.Len() != 1 {
		t.Fatalf("personajes = %d, se esperaba 1", cat.Len())
	}
}

func TestParseRechazaBasura(t *testing.T) {
	for _, raw := range []string{
		`no es json`,
		`{}`,
		`[]`,
		`{"serie": {"name": "X"}}`,      // serie sin personajes
		`[{"id": 1}]`,                   // personaje sin nombre
		`[{"characters": [{"id": 1}]}]`, // sin nombre, tampoco sirve
	} {
		cat, err := Parse([]byte(raw))
		if err == nil {
			t.Fatalf("se aceptó %q", raw)
		}
		if cat == nil {
			t.Fatalf("con %q se devolvió nil en vez de un catálogo vacío", raw)
		}
	}
}

func TestLoadCatalogoInexistente(t *testing.T) {
	cat, aviso := Load(filepath.Join(t.TempDir(), "no-existe.json"))
	if cat == nil || cat.Len() != 0 {
		t.Fatal("sin archivo se debe devolver un catálogo vacío usable")
	}
	if aviso == "" {
		t.Fatal("sin archivo tiene que haber un aviso")
	}
	if _, ok := cat.Random(rand.New(rand.NewSource(1))); ok {
		t.Fatal("Random sobre un catálogo vacío no debe devolver nada")
	}
}

func TestLoadConCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "characters.json")
	if err := os.WriteFile(path, []byte(`[{"id":1,"name":"Uno"}]`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cat, aviso := Load(path)
	if aviso != "" {
		t.Fatalf("aviso inesperado: %s", aviso)
	}
	if cat.Len() != 1 {
		t.Fatalf("personajes = %d", cat.Len())
	}

	// La segunda lectura usa la caché y no debe dar aviso ni cambiar nada.
	cat2, aviso := Load(path)
	if aviso != "" || cat2.Len() != 1 {
		t.Fatalf("segunda lectura: len=%d aviso=%q", cat2.Len(), aviso)
	}

	// Si el archivo cambia, se vuelve a leer.
	if err := os.WriteFile(path, []byte(`[{"id":1,"name":"Uno"},{"id":2,"name":"Dos"}]`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cat3, _ := Load(path)
	if cat3.Len() != 2 {
		t.Fatalf("tras cambiar el archivo: %d personajes (la caché no se refrescó)", cat3.Len())
	}
}

func TestRandomRecorreTodoElCatalogo(t *testing.T) {
	cat, err := Parse([]byte(`[{"id":"a","name":"A"},{"id":"b","name":"B"},{"id":"c","name":"C"}]`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rnd := rand.New(rand.NewSource(7))
	vistos := map[string]bool{}
	for i := 0; i < 500; i++ {
		ch, ok := cat.Random(rnd)
		if !ok {
			t.Fatal("Random falló con catálogo lleno")
		}
		vistos[ch.ID] = true
	}
	if len(vistos) != 3 {
		t.Fatalf("solo salieron %d personajes distintos", len(vistos))
	}
}
