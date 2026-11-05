// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// juegos_test.go — Las reglas de los juegos, sin base de datos ni WhatsApp.
//
// Lo que se comprueba aquí son las probabilidades: que la ruleta tenga 18
// rojos y 18 negros (no 9 verdes pagando como en el JS), que la tragaperras
// devuelva menos de lo que cobra y que el piedra-papel-tijera no tenga una
// jugada imbatible.
package cmds

import (
	"math"
	"testing"

	"shin-go/internal/bot"
)

// ─── dados ──────────────────────────────────────────────────────────────────

func TestDadoRango(t *testing.T) {
	vistos := map[int64]bool{}
	for i := 0; i < 20000; i++ {
		v := dado(1, 6)
		if v < 1 || v > 6 {
			t.Fatalf("dado(1,6) = %d", v)
		}
		vistos[v] = true
	}
	if len(vistos) != 6 {
		t.Fatalf("solo salieron %d caras de 6", len(vistos))
	}
	// Con un solo valor posible no se rompe ni se cuelga.
	if v := dado(0, 0); v != 0 {
		t.Fatalf("dado(0,0) = %d", v)
	}
	if v := dado(5, 3); v != 5 {
		t.Fatalf("dado(5,3) = %d", v)
	}
}

// ─── ruleta ─────────────────────────────────────────────────────────────────

func TestRuletaColoresDeLaRueda(t *testing.T) {
	rojo, negro, verde := 0, 0, 0
	for n := int64(0); n <= 36; n++ {
		switch colorDe(n) {
		case "rojo":
			rojo++
		case "negro":
			negro++
		case "verde":
			verde++
		default:
			t.Fatalf("color desconocido para %d", n)
		}
	}
	if rojo != 18 || negro != 18 || verde != 1 {
		t.Fatalf("rueda con %d rojos, %d negros y %d verdes (se esperaban 18/18/1)",
			rojo, negro, verde)
	}
	// El 0 es verde y el 1 es rojo: los dos extremos que el JS pintaba mal.
	if colorDe(0) != "verde" || colorDe(1) != "rojo" || colorDe(2) != "negro" {
		t.Fatalf("colores mal: 0=%s 1=%s 2=%s", colorDe(0), colorDe(1), colorDe(2))
	}
}

func TestRuletaAciertos(t *testing.T) {
	casos := []struct {
		eleccion string
		n        int64
		gana     bool
	}{
		{"rojo", 1, true},
		{"rojo", 2, false},
		{"negro", 2, true},
		{"negro", 0, false},
		{"verde", 0, true},
		{"verde", 5, false},
		{"17", 17, true},
		{"17", 18, false},
		{"0", 0, true},
	}
	for _, c := range casos {
		if got := ganoRuleta(c.eleccion, c.n); got != c.gana {
			t.Fatalf("ganoRuleta(%q, %d) = %v, se esperaba %v", c.eleccion, c.n, got, c.gana)
		}
	}
}

// Devolución media de la ruleta: alrededor del 97 % (una ruleta europea de
// verdad). Si alguien toca los pagos, esto lo canta.
func TestRuletaDevolucion(t *testing.T) {
	const tiradas = 200000
	ganadas := 0
	for i := 0; i < tiradas; i++ {
		if ganoRuleta("rojo", dado(0, 36)) {
			ganadas++
		}
	}
	devolucion := float64(ganadas) * 2 / tiradas
	if devolucion < 0.93 || devolucion > 1.0 {
		t.Fatalf("la ruleta devuelve %.3f de lo apostado, se esperaba ~0.973", devolucion)
	}
}

// ─── tragaperras ────────────────────────────────────────────────────────────

func TestPremioSlot(t *testing.T) {
	casos := []struct {
		a, b, c string
		espera  float64
	}{
		{"🍒", "🍒", "🍒", 6},
		{"7️⃣", "7️⃣", "7️⃣", 120},
		{"✾", "✾", "✾", 300},
		{"🍒", "🍒", "🍋", 0.6},
		{"🍋", "🔔", "🍋", 0.7},
		{"🔔", "⭐", "🔔", 1.0},
		{"🍒", "🍋", "🔔", 0},
		{"⭐", "💎", "7️⃣", 0},
	}
	for _, c := range casos {
		if got := premioSlot(c.a, c.b, c.c); math.Abs(got-c.espera) > 1e-9 {
			t.Fatalf("premioSlot(%s,%s,%s) = %v, se esperaba %v", c.a, c.b, c.c, got, c.espera)
		}
	}
}

// La tragaperras tiene que devolver menos de lo que cobra (si no, se puede
// vivir de ella y la economía del bot se desborda). Se comprueba por cálculo y
// por simulación, que es lo que de verdad se va a encontrar el jugador.
func TestSlotDevolucion(t *testing.T) {
	ev := premioEsperadoSlot()
	if ev < 0.78 || ev > 0.95 {
		t.Fatalf("devolución teórica de la tragaperras = %.3f, se esperaba ~0.86", ev)
	}

	const giros = 200000
	total := 0.0
	for i := 0; i < giros; i++ {
		total += premioSlot(girarTambor(), girarTambor(), girarTambor())
	}
	simulado := total / giros
	if math.Abs(simulado-ev) > 0.05 {
		t.Fatalf("la simulación da %.3f y la cuenta teórica %.3f", simulado, ev)
	}
}

func TestTamborSoloDaSimbolosConocidos(t *testing.T) {
	validos := map[string]bool{}
	for _, s := range tragaperras {
		validos[s.carita] = true
	}
	for i := 0; i < 5000; i++ {
		if !validos[girarTambor()] {
			t.Fatalf("salió un símbolo que no está en la tabla")
		}
	}
}

// ─── piedra, papel o tijera ─────────────────────────────────────────────────

func TestGanaPPT(t *testing.T) {
	casos := []struct {
		a, b string
		gana bool
	}{
		{"piedra", "tijera", true},
		{"piedra", "papel", false},
		{"papel", "piedra", true},
		{"tijera", "papel", true},
		{"tijera", "piedra", false},
		{"lagarto", "papel", true},
		{"lagarto", "tijera", false},
		{"spock", "tijera", true},
		{"spock", "lagarto", false},
		// Los alias valen igual.
		{"roca", "tijera", true},
		{"tijeras", "papel", true},
		{"piedra", "piedra", false},
	}
	for _, c := range casos {
		if got := ganaPPT(c.a, c.b); got != c.gana {
			t.Fatalf("ganaPPT(%q, %q) = %v, se esperaba %v", c.a, c.b, got, c.gana)
		}
	}
}

// Ninguna jugada puede ganar a todas las demás (el juego tiene que ser cíclico
// y sin favoritos).
func TestPPTSinJugadaImbatible(t *testing.T) {
	jugadas := []string{"piedra", "papel", "tijera", "lagarto", "spock"}
	for _, a := range jugadas {
		victorias := 0
		for _, b := range jugadas {
			if a != b && ganaPPT(a, b) {
				victorias++
			}
		}
		if victorias != 2 {
			t.Fatalf("%q gana a %d jugadas, se esperaban 2", a, victorias)
		}
	}
}

// ─── registro ───────────────────────────────────────────────────────────────

func TestComandosDeJuegosRegistrados(t *testing.T) {
	esperados := map[string]string{
		"coinflip": "juegos", "slot": "juegos", "roulette": "juegos",
		"casino": "juegos", "ppt": "juegos",
		"pescar": "economia", "cazar": "economia", "minar": "economia",
		"mazmorra": "economia", "aventura": "economia", "ritual": "economia",
		"crime": "economia", "robar": "economia", "cofre": "economia",
		"heal": "economia", "pocion": "economia",
	}
	for nombre, categoria := range esperados {
		cmd, ok := bot.Lookup(nombre)
		if !ok || cmd == nil {
			t.Fatalf("el comando %q no está registrado", nombre)
		}
		if string(cmd.Category) != categoria {
			t.Fatalf("%q está en la categoría %q, se esperaba %q", nombre, cmd.Category, categoria)
		}
		if cmd.Desc == "" {
			t.Fatalf("%q no tiene descripción", nombre)
		}
	}
}

// Los alias de los juegos que venían del JS tienen que seguir respondiendo:
// quien escribía .rt o .flip en el bot viejo no debería tener que aprender otra
// palabra ahora.
func TestAliasDeJuegosHeredados(t *testing.T) {
	alias := map[string]string{
		"cf": "coinflip", "flip": "coinflip",
		"rt": "roulette", "ruleta": "roulette",
		"apostar": "casino",
		"fish":    "pescar", "hunt": "cazar", "mine": "minar", "excavar": "minar",
		"dungeon": "mazmorra", "adventure": "aventura",
		"crimen": "crime", "steal": "robar", "rob": "robar",
		"coffer": "cofre", "potion": "pocion", "curar": "heal",
		"invocar": "ritual",
	}
	for a, canonico := range alias {
		cmd, ok := bot.Lookup(a)
		if !ok || cmd == nil {
			t.Fatalf("el alias %q no existe", a)
		}
		if cmd.Name != canonico {
			t.Fatalf("el alias %q apunta a %q, se esperaba %q", a, cmd.Name, canonico)
		}
	}
}

// ─── eventos de la mina ─────────────────────────────────────────────────────

// Cada evento tiene que dar o premio o castigo, nunca los dos (si no, el
// mensaje diría que ganaste y perdiste a la vez).
func TestEventosDeMina(t *testing.T) {
	if len(eventosMina) == 0 {
		t.Fatal("no hay eventos de mina")
	}
	for _, e := range eventosMina {
		if e.prob <= 0 || e.prob >= 1 {
			t.Fatalf("el evento %q tiene probabilidad %v", e.titulo, e.prob)
		}
		if e.premio <= 0 {
			t.Fatalf("el evento %q no da premio", e.titulo)
		}
		premio, castigo, _, _ := resolverEvento(e)
		if (premio > 0) == (castigo > 0) {
			t.Fatalf("el evento %q devolvió premio=%d castigo=%d", e.titulo, premio, castigo)
		}
	}

	// La probabilidad de acertar tiene que corresponder de verdad a `prob`.
	e := eventosMina[2] // la roca, 45 %
	aciertos := 0
	for i := 0; i < 40000; i++ {
		if p, _, _, _ := resolverEvento(e); p > 0 {
			aciertos++
		}
	}
	real := float64(aciertos) / 40000
	if math.Abs(real-e.prob) > 0.02 {
		t.Fatalf("el evento %q acierta el %.2f %% y dice %.2f %%", e.titulo, real*100, e.prob*100)
	}
}

func TestNumeroCorto(t *testing.T) {
	casos := map[float64]string{6: "6", 0.6: "0.6", 120: "120", 1.5: "1.5", 300: "300"}
	for v, esperado := range casos {
		if got := numeroCorto(v); got != esperado {
			t.Fatalf("numeroCorto(%v) = %q, se esperaba %q", v, got, esperado)
		}
	}
}
