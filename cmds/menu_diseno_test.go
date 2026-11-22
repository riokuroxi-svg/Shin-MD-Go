// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package cmds

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"shin-go/internal/bot"
	"shin-go/internal/wa"
)

func TestMenuPrincipalConservaElTextoDelBotJS(t *testing.T) {
	got := menuNubeBody(menuNubeData{
		Greeting: "¡Buenas Noches! 🌙",
		BotName:  "SHIN-MD",
		Version:  "0.26.0-rc1",
		Engine:   "whatsmeow Multi-Device",
		Uptime:   "1h 2m 3s",
		Weather:  "⛅ Parcialmente nublado | 🌡️ 18°C",
		Commands: 137,
		UserName: "Rin",
		Role:     "👑 Creador / Owner",
		Number:   "+5210000000000",
		Prefix:   ".",
	})

	want := "¡Buenas Noches! 🌙\n\n" +
		"      *Rin*\n\n" +
		"╭┈┈⫹⫺ *INFORMACIÓN SHIN-MD* ⫹⫺┈┈╮\n" +
		"│ ◈ *Nombre* : *SHIN-MD*\n" +
		"│ ◈ *Versión* : *0.26.0-rc1*\n" +
		"│ ◈ *Motor* : `whatsmeow Multi-Device`\n" +
		"│ ◈ *Uptime* : *1h 2m 3s*\n" +
		"│ ◈ *Clima* : ⛅ Parcialmente nublado | 🌡️ 18°C\n" +
		"│ ◈ *Comandos* : *137 únicos*\n" +
		"╰┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈╯\n\n" +
		"╭┈┈⫹⫺ *INFORMACIÓN USUARIO* ⫹⫺┈┈╮\n" +
		"│ ◈ *Nombre* : *Rin*\n" +
		"│ ◈ *Rango* : *👑 Creador / Owner*\n" +
		"│ ◈ *Número* : +5210000000000\n" +
		"│ ◈ *Prefijo* : `.`\n" +
		"╰┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈╯\n\n" +
		"╭──〔 📌 *GUÍA RÁPIDA* 〕──⬣\n" +
		"│ 🅞 Dueño • 🅟 Premium • 🅐 Admin\n" +
		"│ 🅖 Grupos • 🅛 Límite\n" +
		"╰─────────────────────────⬣\n\n" +
		"_Selecciona una categoría en el botón desplegable de abajo para ver sus comandos._\n\n" +
		"📢 *Canal:* " + menuChannelURL

	if got != want {
		t.Fatalf("texto del menú cambió:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if n := utf8.RuneCountInString(got); n > 1024 {
		t.Fatalf("cuerpo demasiado largo para una tarjeta: %d runas", n)
	}
	if menuFooter != "Shin-MD • Bot de WhatsApp Profesional\ngithub.com/riokuroxi-svg/Shin-MD" {
		t.Fatalf("tag final inesperado: %q", menuFooter)
	}
}

func TestMenuNombreYGreetingSonSeguros(t *testing.T) {
	if got, want := menuUserName("  Rin\n\x00  "), "Rin"; got != want {
		t.Fatalf("nombre saneado = %q; want %q", got, want)
	}
	if got, want := menuUserName(""), "Usuario"; got != want {
		t.Fatalf("nombre por defecto = %q; want %q", got, want)
	}
	for _, tc := range []struct {
		hour int
		want string
	}{
		{5, "¡Buenos Días! 🌅"},
		{12, "¡Buenas Tardes! ☀️"},
		{19, "¡Buenas Noches! 🌙"},
	} {
		got := menuGreeting(time.Date(2026, 11, 20, tc.hour, 0, 0, 0, time.Local))
		if got != tc.want {
			t.Errorf("saludo de las %d = %q; want %q", tc.hour, got, tc.want)
		}
	}
}

func TestMenuDatosRealesDelUsuario(t *testing.T) {
	owner := &bot.Context{
		IsOwner:  true,
		Identity: wa.Identity{PN: wa.NewJID("5210000000000", wa.DefaultUserServer)},
	}
	if got, want := menuUserRole(owner), "👑 Creador / Owner"; got != want {
		t.Fatalf("rango owner = %q; want %q", got, want)
	}
	if got, want := menuUserNumber(owner), "+5210000000000"; got != want {
		t.Fatalf("número = %q; want %q", got, want)
	}
	if got := menuUserNumber(&bot.Context{Sender: wa.NewJID("12345", wa.HiddenUserServer)}); got != "No disponible" {
		t.Fatalf("un LID desconocido no debe mostrarse como teléfono: %q", got)
	}
}

func TestMenuUptimeCompacto(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{7 * time.Second, "7s"},
		{3*time.Minute + 12*time.Second, "3m 12s"},
		{25*time.Hour + 4*time.Minute + 9*time.Second, "1d 1h 4m 9s"},
	}
	for _, tc := range cases {
		if got := menuUptime(tc.d); got != tc.want {
			t.Errorf("menuUptime(%s) = %q; want %q", tc.d, got, tc.want)
		}
	}
}

func TestMenuClimaTraduceCodigos(t *testing.T) {
	for code, want := range map[int]string{
		0:  "☀️ Despejado",
		3:  "☁️ Nublado",
		45: "🌫️ Niebla",
		63: "🌧️ Lluvia moderada",
		95: "⛈️ Tormenta eléctrica",
	} {
		if got := menuWeatherCondition(code); got != want {
			t.Errorf("código %d = %q; want %q", code, got, want)
		}
	}
}

func TestMenuClimaFallaSinBloquearElMenu(t *testing.T) {
	menuWeatherState.Lock()
	menuWeatherState.city = ""
	menuWeatherState.value = ""
	menuWeatherState.until = time.Time{}
	menuWeatherState.Unlock()
	t.Cleanup(func() {
		menuWeatherState.Lock()
		menuWeatherState.city = ""
		menuWeatherState.value = ""
		menuWeatherState.until = time.Time{}
		menuWeatherState.Unlock()
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := menuWeather(ctx, "Tula de Allende"); got != menuWeatherFallback {
		t.Fatalf("fallback del clima = %q; want %q", got, menuWeatherFallback)
	}

	menuWeatherState.Lock()
	defer menuWeatherState.Unlock()
	if menuWeatherState.city != "Tula de Allende" || menuWeatherState.value != menuWeatherFallback {
		t.Fatalf("caché del fallback = ciudad %q, valor %q", menuWeatherState.city, menuWeatherState.value)
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
