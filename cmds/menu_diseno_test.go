// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package cmds

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"shin-go/internal/bot"
)

func TestMenuNubeConservaElDisenoAprobado(t *testing.T) {
	got := menuNubeBody(
		"Shin-MD",
		"*Ginko*",
		213,
		748,
		5,
		[]string{".", "/", "#", "!"},
	)

	for _, want := range []string{
		"╭── ⋅ ──  ☁️  ✧  🪽  ── ⋅ ──╮",
		"*ＳＨＩＮ–ＭＤ*",
		"反魂 · *NÚCLEO EN GO*",
		"╰── ⋅ ──  ★  ── ⋅ ──╯",
		"Hola, *Ginko*",
		"213 comandos · 748 nombres",
		"5 hooks activos",
		"Prefijos · . · / · # · !",
		"Elige una ruta en los botones de abajo",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("el menú no contiene %q:\n%s", want, got)
		}
	}
	if n := utf8.RuneCountInString(got); n > 1024 {
		t.Fatalf("cuerpo demasiado largo para una tarjeta: %d runas", n)
	}
}

func TestMenuNubeNombreUsaUnicodeVisible(t *testing.T) {
	if got, want := menuNubeNombre("Shin-MD"), "ＳＨＩＮ–ＭＤ"; got != want {
		t.Fatalf("nombre decorado = %q; want %q", got, want)
	}
	if got, want := menuNubeNombre(""), "ＳＨＩＮ–ＭＤ"; got != want {
		t.Fatalf("nombre por defecto = %q; want %q", got, want)
	}
}

func TestMenuNubeBotonesDejaAccesosPrincipalesPrimero(t *testing.T) {
	buttons := menuNubeButtons(".")
	if len(buttons) != 6 {
		t.Fatalf("botones = %d; want 6 para activar Ver todo", len(buttons))
	}

	type params struct {
		DisplayText string `json:"display_text"`
		ID          string `json:"id"`
		CopyCode    string `json:"copy_code"`
		URL         string `json:"url"`
	}
	read := func(index int) params {
		t.Helper()
		var got params
		if err := json.Unmarshal([]byte(buttons[index].Params), &got); err != nil {
			t.Fatalf("params del botón %d: %v", index, err)
		}
		return got
	}

	if buttons[0].Name != "quick_reply" || read(0).ID != "menucat" || read(0).DisplayText != "☁️ Categorías" {
		t.Fatalf("primer botón no abre categorías: %#v / %s", buttons[0], buttons[0].Params)
	}
	if buttons[1].Name != "quick_reply" || read(1).ID != "profile" || read(1).DisplayText != "🪽 Mi perfil" {
		t.Fatalf("segundo botón no abre el perfil: %#v / %s", buttons[1], buttons[1].Params)
	}
	if buttons[4].Name != "cta_copy" || read(4).CopyCode != "." {
		t.Fatalf("botón copiar inválido: %#v / %s", buttons[4], buttons[4].Params)
	}
	if buttons[5].Name != "cta_url" || read(5).URL != repoURL {
		t.Fatalf("botón del repositorio inválido: %#v / %s", buttons[5], buttons[5].Params)
	}
}

func TestMenuNubeListaIncluyeTodasLasCategoriasSinSuperarDiezFilas(t *testing.T) {
	cats := bot.ByCategory()
	pages := menuNubePages(cats)
	seen := make(map[bot.Category]int, len(cats))

	if len(pages) < 2 {
		t.Fatalf("se esperaban varias páginas para %d categorías; got %d", len(cats), len(pages))
	}
	for pageIndex, sections := range pages {
		rows := 0
		for _, section := range sections {
			if len(section.Rows) == 0 {
				t.Fatalf("página %d tiene una sección vacía: %q", pageIndex+1, section.Title)
			}
			if utf8.RuneCountInString(section.Title) > 24 {
				t.Errorf("título de sección demasiado largo: %q", section.Title)
			}
			for _, row := range section.Rows {
				rows++
				if utf8.RuneCountInString(row.Title) > 24 {
					t.Errorf("título de fila demasiado largo (%d): %q", utf8.RuneCountInString(row.Title), row.Title)
				}
				if utf8.RuneCountInString(row.Desc) > 72 {
					t.Errorf("descripción demasiado larga (%d): %q", utf8.RuneCountInString(row.Desc), row.Desc)
				}
				if strings.HasPrefix(row.ID, "menucat:pagina-") {
					continue
				}
				if !strings.HasPrefix(row.ID, "menu:") {
					t.Errorf("fila no enrutable: %q", row.ID)
					continue
				}
				cat := bot.Category(strings.TrimPrefix(row.ID, "menu:"))
				seen[cat]++
			}
		}
		if rows > 10 {
			t.Errorf("página %d tiene %d filas; WhatsApp admite 10", pageIndex+1, rows)
		}
	}

	if len(seen) != len(cats) {
		t.Fatalf("la lista cubre %d categorías; el registro tiene %d", len(seen), len(cats))
	}
	for cat := range cats {
		if seen[cat] != 1 {
			t.Errorf("categoría %q aparece %d veces", cat, seen[cat])
		}
	}
}

func TestMenuNubeNoPierdeCategoriasFuturas(t *testing.T) {
	cats := map[bot.Category][]*bot.Command{
		bot.Category("nueva"): {{Name: "prueba"}},
	}
	pages := menuNubePages(cats)
	if len(pages) != 1 || len(pages[0]) != 1 || pages[0][0].Title != "Otras rutas" || len(pages[0][0].Rows) != 1 {
		t.Fatalf("categoría futura no quedó en Otras rutas: %#v", pages)
	}
	if got, want := pages[0][0].Rows[0].ID, "menu:nueva"; got != want {
		t.Fatalf("id futuro = %q; want %q", got, want)
	}
}

func TestMenuNubePaginaSePuedeAbrirDesdeUnTap(t *testing.T) {
	if got, ok := menuNubePageArg("pagina-2"); !ok || got != 2 {
		t.Fatalf("pagina-2 = %d, %v; want 2, true", got, ok)
	}
	for _, invalid := range []string{"2", "pagina-0", "pagina-x"} {
		if _, ok := menuNubePageArg(invalid); ok {
			t.Errorf("%q no debería ser una página válida", invalid)
		}
	}
}
