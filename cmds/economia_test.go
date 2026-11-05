// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// economia_test.go — Formatos, cantidades y los números del juego.
package cmds

import (
	"math"
	"strings"
	"testing"

	"shin-go/internal/bot"
)

// ─── cantidades ─────────────────────────────────────────────────────────────

func TestParseAmount(t *testing.T) {
	bien := []struct {
		entrada string
		espera  int64
	}{
		{"1000", 1000},
		{"2,500", 2500},
		{"2.500", 2500},
		{"25k", 25000},
		{"1.5k", 1500},
		{"2m", 2000000},
		{"¥3000", 3000},
		{" 750 ", 750},
		{"1,000,000", 1000000},
	}
	for _, caso := range bien {
		v, all, ok := parseAmount(caso.entrada)
		if !ok || all || v != caso.espera {
			t.Fatalf("parseAmount(%q) = %d all=%v ok=%v, se esperaba %d",
				caso.entrada, v, all, ok, caso.espera)
		}
	}

	// "all"/"todo" se tratan aparte.
	for _, s := range []string{"all", "ALL", "todo"} {
		if _, all, ok := parseAmount(s); !ok || !all {
			t.Fatalf("parseAmount(%q) debía ser all", s)
		}
	}

	// Lo que el JS aceptaba por parseInt y aquí no debe pasar: basura,
	// negativos y ceros. En el JS "2500abc" valía 2500 y "-500" restaba.
	mal := []string{"", "  ", "abc", "2500abc", "-500", "0", "0k", "%%%", "1..2", "9e999", "¥"}
	for _, s := range mal {
		if v, _, ok := parseAmount(s); ok {
			t.Fatalf("parseAmount(%q) aceptó %d", s, v)
		}
	}
}

func TestMiles(t *testing.T) {
	casos := map[int64]string{
		0:          "0",
		7:          "7",
		999:        "999",
		1000:       "1,000",
		1500:       "1,500",
		1015000:    "1,015,000",
		1000000000: "1,000,000,000",
		-2500:      "-2,500",
	}
	for n, esperado := range casos {
		if got := miles(n); got != esperado {
			t.Fatalf("miles(%d) = %q, se esperaba %q", n, got, esperado)
		}
	}
}

func TestFormatWait(t *testing.T) {
	casos := map[int64]string{
		0:      "0 segundos",
		1:      "1 segundo",
		30:     "30 segundos",
		60:     "1 minuto",
		90:     "1 minuto",
		180:    "3 minutos",
		3600:   "1 hora",
		3661:   "1 hora 1 minuto",
		86400:  "1 día",
		90061:  "1 día 1 hora 1 minuto",
		172800: "2 días",
	}
	for s, esperado := range casos {
		if got := formatWait(s); got != esperado {
			t.Fatalf("formatWait(%d) = %q, se esperaba %q", s, got, esperado)
		}
	}
}

func TestCurrencySymbol(t *testing.T) {
	casos := map[string]string{
		"Yenes":   "¥",
		"yenes":   "¥",
		"Pesos":   "$",
		"Dólares": "$",
		"Euros":   "€",
		"Reales":  "R$",
		"Soles":   "S/",
		"":        "¥",
		"Zlotys":  "¥", // moneda desconocida: se cae al símbolo de serie
	}
	for nombre, simbolo := range casos {
		if got := currencySymbol(nombre); got != simbolo {
			t.Fatalf("currencySymbol(%q) = %q, se esperaba %q", nombre, got, simbolo)
		}
	}
}

// ─── premios: los números tienen que ser los del bot de JS ──────────────────

func TestDailyReward(t *testing.T) {
	// 20,000 el primer día y 5,000 más por día de racha.
	casos := map[int64]int64{
		0:   20000, // sin racha se paga como el día 1, nunca menos
		1:   20000,
		2:   25000,
		3:   30000,
		10:  65000,
		200: 1015000, // tope exacto con la racha máxima
		201: 1015000, // por encima del tope se queda igual
		999: 1015000,
	}
	for racha, esperado := range casos {
		if got := dailyReward(racha); got != esperado {
			t.Fatalf("dailyReward(%d) = %d, se esperaba %d", racha, got, esperado)
		}
	}
}

func TestWeeklyReward(t *testing.T) {
	casos := map[int64]int64{
		0:  40000,
		1:  40000,
		2:  45000,
		30: 185000,
		31: 185000,
	}
	for racha, esperado := range casos {
		if got := weeklyReward(racha); got != esperado {
			t.Fatalf("weeklyReward(%d) = %d, se esperaba %d", racha, got, esperado)
		}
	}
}

func TestMonthlyReward(t *testing.T) {
	casos := map[int64]int64{
		0:  60000,
		1:  60000,
		2:  65000,
		8:  95000,
		9:  95000,
		30: 95000,
	}
	for racha, esperado := range casos {
		if got := monthlyReward(racha); got != esperado {
			t.Fatalf("monthlyReward(%d) = %d, se esperaba %d", racha, got, esperado)
		}
	}
}

// work debe caer siempre dentro del rango del JS y tocar los extremos.
func TestWorkReward(t *testing.T) {
	minimo, maximo := int64(math.MaxInt64), int64(0)
	for i := 0; i < 5000; i++ {
		v := workReward()
		if v < 2000 || v > 4000 {
			t.Fatalf("workReward() = %d, fuera de 2,000–4,000", v)
		}
		if v < minimo {
			minimo = v
		}
		if v > maximo {
			maximo = v
		}
	}
	// Con 5,000 tiradas, no tocar los bordes significaría que el rango real es
	// más estrecho que el que dice el mensaje.
	if minimo > 2010 || maximo < 3990 {
		t.Fatalf("el rango real parece ser %d–%d", minimo, maximo)
	}
}

// ─── comandos registrados ───────────────────────────────────────────────────

// Los nombres y alias son los del JS: si alguien escribe .w, .chambear o
// .baltop tiene que pasar lo mismo que con .work y .economyboard.
func TestComandosDeEconomiaRegistrados(t *testing.T) {
	esperados := []string{
		"daily", "diario", "recompensa", "gratis",
		"weekly", "semanal",
		"monthly", "mensual",
		"work", "w", "chambear", "trabajar",
		"balance", "bal", "coins", "bank",
		"deposit", "dep", "depositar",
		"withdraw", "with", "retirar",
		"pay", "givecoins", "transfer",
		"economyboard", "eboard", "baltop", "ranking",
		"shop", "tienda", "buy", "comprar",
		"inventory", "inv", "inventario",
		"economy", "economia",
		"perfil", "profile",
	}
	for _, nombre := range esperados {
		cmd, ok := bot.Lookup(nombre)
		if !ok {
			t.Fatalf("falta el comando %q", nombre)
		}
		if cmd.Category == "" {
			t.Fatalf("%q no tiene categoría (no saldría en el menú)", nombre)
		}
		if cmd.Desc == "" {
			t.Fatalf("%q no tiene descripción", nombre)
		}
	}
}

// El que activa o desactiva la economía solo puede ser admin del grupo: si no,
// cualquiera apagaría la economía a los demás.
func TestEconomyToggleSoloAdmin(t *testing.T) {
	cmd, ok := bot.Lookup("economy")
	if !ok {
		t.Fatal("no está .economy")
	}
	if !cmd.AdminOnly || !cmd.GroupOnly {
		t.Fatalf(".economy debería ser de grupo y de admin: %+v", cmd)
	}
}

// Ningún comando de economía puede salirse de la categoría (si no, no aparece
// en el menú de economía y el usuario no lo encuentra).
func TestCategoriaEconomia(t *testing.T) {
	count := 0
	for _, nombre := range []string{"daily", "weekly", "monthly", "work", "balance", "deposit", "withdraw", "pay", "economyboard", "shop", "inventory"} {
		cmd, ok := bot.Lookup(nombre)
		if !ok {
			t.Fatalf("falta %q", nombre)
		}
		if string(cmd.Category) != "economia" {
			t.Fatalf("%q está en la categoría %q", nombre, cmd.Category)
		}
		count++
	}
	if count != 11 {
		t.Fatalf("se revisaron %d comandos", count)
	}
}

// Los alias no pueden repetirse entre comandos: si se repiten, gana el primero
// que se registró y el otro queda inalcanzable sin que nadie se entere.
func TestSinAliasRepetidos(t *testing.T) {
	vistos := map[string]string{}
	for _, cmd := range bot.All() {
		for _, nombre := range append([]string{cmd.Name}, cmd.Aliases...) {
			nombre = strings.ToLower(nombre)
			if otro, ok := vistos[nombre]; ok && otro != cmd.Name {
				t.Fatalf("el nombre %q lo usan %q y %q", nombre, otro, cmd.Name)
			}
			vistos[nombre] = cmd.Name
		}
	}
	if len(vistos) == 0 {
		t.Fatal("no hay comandos registrados")
	}
}

// Nada de nombres con espacios, mayúsculas raras ni prefijos: el parser tiene
// que poder reconocerlos tal cual.
func TestNombresLimpios(t *testing.T) {
	for _, cmd := range bot.All() {
		for _, nombre := range append([]string{cmd.Name}, cmd.Aliases...) {
			if nombre == "" || strings.TrimSpace(nombre) != nombre {
				t.Fatalf("nombre raro: %q", nombre)
			}
			if strings.ToLower(nombre) != nombre {
				t.Fatalf("el nombre %q no está en minúsculas", nombre)
			}
			if strings.ContainsAny(nombre, "./ !@#") {
				t.Fatalf("el nombre %q tiene caracteres que no van", nombre)
			}
		}
	}
}

// ─── gacha ──────────────────────────────────────────────────────────────────

// Los nombres del gacha son los del JS: .rw, .c, .waifus, .claims…
func TestComandosDeGachaRegistrados(t *testing.T) {
	esperados := []string{
		"rollwaifu", "roll", "rw",
		"claim", "c", "reclamar",
		"harem", "waifus", "claims",
		"setfav", "favorito",
		"waifusboard", "gachaboard",
		"gacha", "gachainfo",
	}
	for _, nombre := range esperados {
		cmd, ok := bot.Lookup(nombre)
		if !ok {
			t.Fatalf("falta el comando %q", nombre)
		}
		if cmd.Category != bot.CatGacha {
			t.Fatalf("%q está en la categoría %q, se esperaba gacha", nombre, cmd.Category)
		}
		if cmd.Desc == "" {
			t.Fatalf("%q no tiene descripción", nombre)
		}
	}

	// .gacha solo lo puede tocar un admin del grupo.
	if cmd, _ := bot.Lookup("gacha"); !cmd.AdminOnly || !cmd.GroupOnly {
		t.Fatalf(".gacha debería ser de grupo y de admin: %+v", cmd)
	}
	// Y reclamar requiere responder a un mensaje: no puede ser de owner ni de
	// admin, tiene que poder usarlo cualquiera del grupo.
	if cmd, _ := bot.Lookup("claim"); cmd.AdminOnly || cmd.OwnerOnly {
		t.Fatalf(".claim no puede tener restricciones: %+v", cmd)
	}
}
