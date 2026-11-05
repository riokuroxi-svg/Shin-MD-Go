// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// juegos.go — Juegos de apuesta: coinflip, slot, ruleta, casino y piedra-papel-tijera.
//
// Los números salen de cmds/economy/*.js, pero la forma de cobrar es distinta a
// propósito. En el JS cada juego hacía:
//
//	user = db.getChatUser(...)        // 1. leer
//	newCoins = user.coins - apuesta   // 2. restar en JavaScript
//	db.setChatUser(..., newCoins)     // 3. escribir
//
// Con dos mensajes en el mismo segundo las dos lecturas veían el mismo saldo y
// las dos escribían el mismo resultado: la segunda apuesta se perdía (o, si
// ganaba, se cobraba dos veces). Aquí el cobro es un `UPDATE ... WHERE coins >=
// apuesta` —o pasa entero o no pasa— y el premio se suma con `coins = coins + ?`
// sobre el valor que haya en la base en ese momento.
//
// Tampoco se usa Math.random(): en JS no lleva semilla, y dos procesos que
// arrancan en el mismo instante reparten las mismas tiradas. Aquí se usa
// math/rand/v2, que se siembra solo.
package cmds

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"shin-go/internal/bot"
	"shin-go/internal/store"
)

// ─── azar y textos ──────────────────────────────────────────────────────────

// dado devuelve un entero en [a, b], los dos incluidos (randomInt del JS).
func dado(a, b int64) int64 {
	if b <= a {
		return a
	}
	return a + rand.Int64N(b-a+1)
}

// numeroCorto pinta un multiplicador sin decimales de adorno: 6, 0.6, 120.
func numeroCorto(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', 1, 64)
}

// azar devuelve true con probabilidad p (0..1).
func azar(p float64) bool { return rand.Float64() < p }

// elegir devuelve un texto al azar de la lista (pickRandom del JS).
func elegir(opciones []string) string {
	if len(opciones) == 0 {
		return ""
	}
	return opciones[rand.IntN(len(opciones))]
}

// ─── flujo común de una apuesta ─────────────────────────────────────────────

// cobrarApuesta hace lo mismo en todos los juegos con dinero: leer la cantidad,
// comprobar el mínimo, reservar el enfriamiento y descontar de la cartera.
//
// Devuelve (cantidad, true) si la apuesta quedó cobrada y el comando puede
// seguir. Si devuelve (0, false) ya contestó al usuario y el comando debe
// terminar con el error que traiga.
func cobrarApuesta(c *bot.Context, columna string, minimo int64, espera time.Duration) (int64, bool, error) {
	raw := firstArg(c)
	if raw == "" {
		return 0, false, c.Reply(fmt.Sprintf("ꕥ Indica cuánto quieres apostar. Mínimo *%s*.\n> Ejemplo: *%s%s %s*",
			money(c, minimo), c.Bot.FirstPrefix(), c.Command, miles(minimo)))
	}
	cantidad, all, ok := parseAmount(raw)
	if !ok {
		return 0, false, c.Reply(fmt.Sprintf("ꕥ *%s* no es una cantidad válida. Mínimo *%s*.",
			raw, money(c, minimo)))
	}

	chat, key := chatID(c), c.Key()
	acc, err := c.DB.ChatUser(c.Ctx, chat, key)
	if err != nil {
		return 0, false, err
	}
	if all {
		cantidad = acc.Coins
	}
	if cantidad < minimo {
		return 0, false, c.Reply(fmt.Sprintf("ꕥ La apuesta mínima es *%s*.\n> Tienes en la cartera: *%s*",
			money(c, minimo), money(c, acc.Coins)))
	}
	if cantidad > acc.Coins {
		return 0, false, c.Reply(fmt.Sprintf(
			"ꕥ No tienes suficiente en la cartera: quieres apostar *%s* y tienes *%s*.\n"+
				"> Retira del banco con *%swithdraw %s*", money(c, cantidad), money(c, acc.Coins),
			c.Bot.FirstPrefix(), miles(cantidad-acc.Coins)))
	}

	// El enfriamiento se reserva ANTES de cobrar: si se reservara después, dos
	// mensajes seguidos podrían pasar los dos la comprobación.
	listo, restante, err := c.DB.ClaimCooldown(c.Ctx, chat, key, columna, espera)
	if err != nil {
		return 0, false, err
	}
	if !listo {
		return 0, false, c.Reply(fmt.Sprintf("ꕥ Espera *%s* antes de volver a jugar.",
			formatWait(int64(restante.Seconds()))))
	}

	pagado, err := c.DB.Spend(c.Ctx, chat, key, cantidad)
	if err != nil {
		return 0, false, err
	}
	if !pagado {
		// Otra apuesta se llevó el saldo en el mismo instante: se devuelve el
		// turno para que la espera no se gaste en una partida que no existió.
		if e := c.DB.ReleaseCooldown(c.Ctx, chat, key, columna); e != nil {
			return 0, false, e
		}
		return 0, false, c.Reply("ꕥ Tu saldo cambió mientras apostabas y ya no alcanzaba. No se descontó nada.")
	}
	return cantidad, true, nil
}

// pagar devuelve al jugador lo que haya ganado. Se usa AddCoins (una suma
// sobre el saldo actual) y no "el saldo que leí antes + premio".
func pagar(c *bot.Context, cantidad int64) (int64, error) {
	return c.DB.AddCoins(c.Ctx, chatID(c), c.Key(), cantidad)
}

// ─── .coinflip ──────────────────────────────────────────────────────────────

func registerCoinflip() {
	bot.Register(bot.Command{
		Name:     "coinflip",
		Aliases:  []string{"cf", "flip", "caraocruz"},
		Category: bot.CatGames,
		Desc:     "Apostar a cara o cruz (paga el doble)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			apuesta, ok, err := cobrarApuesta(c, "last_coinflip", 100, 10*time.Second)
			if !ok {
				return err
			}

			// El jugador puede elegir lado; si no dice nada, la moneda decide.
			eleccion := ""
			if len(c.Args) > 1 {
				switch strings.ToLower(c.Args[1]) {
				case "cara", "sol", "aguila", "águila", "heads":
					eleccion = "cara"
				case "cruz", "sello", "tails":
					eleccion = "cruz"
				}
			}
			salio := "cara"
			if azar(0.5) {
				salio = "cruz"
			}
			moneda := "🪙"
			if salio == "cara" {
				moneda = "🌞"
			}

			gana := eleccion == "" || eleccion == salio
			if !gana {
				return c.Reply(fmt.Sprintf("%s La moneda salió *%s*.\n> Perdiste *%s*",
					moneda, salio, money(c, apuesta)))
			}

			coins, err := pagar(c, apuesta*2)
			if err != nil {
				return err
			}
			texto := fmt.Sprintf("%s La moneda salió *%s*. ¡Ganaste *%s*!", moneda, salio, money(c, apuesta))
			if eleccion != "" {
				texto = fmt.Sprintf("%s Elegiste *%s* y salió *%s*. ¡Ganaste *%s*!",
					moneda, eleccion, salio, money(c, apuesta))
			}
			return c.Reply(texto + fmt.Sprintf("\n> Cartera: *%s*", money(c, coins)))
		},
	})
}

// ─── .slot ──────────────────────────────────────────────────────────────────

// simbolo es una casilla de la tragaperras. `peso` es cuántas veces entra en el
// bombo (sobre 100) y `paga` lo que devuelve la apuesta si salen tres iguales.
type simbolo struct {
	carita string
	peso   int64
	paga   int64
	dos    float64 // multiplicador si salen exactamente dos iguales
}

// La tabla está pensada para devolver alrededor del 86 % de lo apostado a largo
// plazo (la "casa" se queda el resto). Con una tabla plana de "cualquier línea
// de tres paga ×3" el juego devolvía más de lo que cobraba y la economía se
// desbocaba; estos números están calculados a mano y comprobados en los tests.
var tragaperras = []simbolo{
	{"🍒", 30, 6, 0.6},
	{"🍋", 25, 8, 0.7},
	{"🔔", 18, 12, 1.0},
	{"⭐", 12, 25, 1.5},
	{"💎", 8, 60, 2.0},
	{"7️⃣", 5, 120, 3.0},
	{"✾", 2, 300, 5.0},
}

func girarTambor() string {
	total := int64(0)
	for _, s := range tragaperras {
		total += s.peso
	}
	n := rand.Int64N(total)
	for _, s := range tragaperras {
		if n < s.peso {
			return s.carita
		}
		n -= s.peso
	}
	return tragaperras[0].carita
}

// premioSlot calcula el multiplicador (0 = nada). Se saca a una función aparte
// para poder probarlo sin base de datos.
func premioSlot(a, b, d string) float64 {
	if a == b && b == d {
		for _, s := range tragaperras {
			if s.carita == a {
				return float64(s.paga)
			}
		}
		return 0
	}
	if a == b || b == d || a == d {
		par := a
		if b == d {
			par = b
		}
		for _, s := range tragaperras {
			if s.carita == par {
				return s.dos
			}
		}
	}
	return 0
}

func registerSlot() {
	bot.Register(bot.Command{
		Name:     "slot",
		Aliases:  []string{"tragaperras", "tragamonedas", "slots"},
		Category: bot.CatGames,
		Desc:     "Jugar a la tragaperras",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			apuesta, ok, err := cobrarApuesta(c, "last_slot", 100, 30*time.Second)
			if !ok {
				return err
			}

			a, b, d := girarTambor(), girarTambor(), girarTambor()
			premio := premioSlot(a, b, d)
			linea := fmt.Sprintf("[ %s │ %s │ %s ]", a, b, d)

			if premio == 0 {
				return c.Reply(fmt.Sprintf("🎰 %s\n> Nada. Perdiste *%s*", linea, money(c, apuesta)))
			}
			ganancia := int64(float64(apuesta) * premio)
			coins, err := pagar(c, ganancia)
			if err != nil {
				return err
			}
			etiqueta := "¡Dos iguales!"
			if a == b && b == d {
				etiqueta = "¡¡TRIPLE!!"
			}
			return c.Reply(fmt.Sprintf("🎰 %s\n> %s Pagó ×%s: *%s*\n> Cartera: *%s*",
				linea, etiqueta, numeroCorto(premio), money(c, ganancia), money(c, coins)))
		},
	})
}

// ─── .roulette ──────────────────────────────────────────────────────────────

// rojos: los números rojos de la ruleta europea. El resto (menos el 0) son
// negros. En el JS la "ruleta" tiraba un número del 0 al 36 y pintaba de verde
// los 9 primeros: 9 casillas verdes pagando ×5 sobre 37 daba una ventaja del
// 21 % a favor del jugador, y el juego se usaba para fabricar dinero. Aquí la
// rueda es la de verdad: 18 rojos, 18 negros y un verde.
var rojos = map[int64]bool{
	1: true, 3: true, 5: true, 7: true, 9: true, 12: true, 14: true, 16: true,
	18: true, 19: true, 21: true, 23: true, 25: true, 27: true, 30: true,
	32: true, 34: true, 36: true,
}

// colorDe: "verde" para el 0, "rojo" o "negro" para el resto.
func colorDe(n int64) string {
	switch {
	case n == 0:
		return "verde"
	case rojos[n]:
		return "rojo"
	default:
		return "negro"
	}
}

func registerRoulette() {
	bot.Register(bot.Command{
		Name:     "roulette",
		Aliases:  []string{"rt", "ruleta"},
		Category: bot.CatGames,
		Desc:     "Apostar a la ruleta: rojo, negro, verde o un número",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			// Se acepta la cantidad y la casilla en cualquier orden:
			//   .rt 500 rojo   ·   .rt rojo 500   ·   .rt 500 17
			apuesta, eleccion, err := leerApuestaRuleta(c)
			if err != nil {
				return err
			}
			if eleccion == "" {
				return c.Reply(fmt.Sprintf(
					"ꕥ Dime a qué apuestas.\n> *%sroulette 500 rojo* · *%sroulette 500 negro* · "+
						"*%sroulette 500 verde* · *%sroulette 500 17*",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix(), c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}
			if apuesta == 0 {
				return nil // leerApuestaRuleta ya contestó
			}

			chat, key := chatID(c), c.Key()
			listo, restante, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_roulette", 30*time.Second)
			if err != nil {
				return err
			}
			if !listo {
				return c.Reply(fmt.Sprintf("ꕥ Espera *%s* antes de volver a la ruleta.",
					formatWait(int64(restante.Seconds()))))
			}
			pagado, err := c.DB.Spend(c.Ctx, chat, key, apuesta)
			if err != nil {
				return err
			}
			if !pagado {
				if e := c.DB.ReleaseCooldown(c.Ctx, chat, key, "last_roulette"); e != nil {
					return e
				}
				return c.Reply("ꕥ Tu saldo cambió mientras apostabas y ya no alcanzaba.")
			}

			n := dado(0, 36)
			salio := colorDe(n)
			if ganoRuleta(eleccion, n) {
				mult := int64(2)
				switch {
				case salio == "verde":
					mult = 36 // 0 a secas: se paga como un número
				case esNumero(eleccion):
					mult = 36
				}
				ganancia := apuesta * mult
				coins, err := pagar(c, ganancia)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🎡 Salió *%d %s*.\n> ¡Ganaste *%s*!\n> Cartera: *%s*",
					n, salio, money(c, ganancia), money(c, coins)))
			}
			return c.Reply(fmt.Sprintf("🎡 Salió *%d %s*.\n> Perdiste *%s*", n, salio, money(c, apuesta)))
		},
	})
}

// esNumero: ¿la casilla apostada es un número del 0 al 36?
func esNumero(s string) bool {
	n, err := strconv.ParseInt(s, 10, 64)
	return err == nil && n >= 0 && n <= 36
}

// ganoRuleta decide si la casilla acertó.
func ganoRuleta(eleccion string, n int64) bool {
	if esNumero(eleccion) {
		v, _ := strconv.ParseInt(eleccion, 10, 64)
		return v == n
	}
	return eleccion == colorDe(n) && eleccion != "" && eleccion != "verde" ||
		(eleccion == "verde" && n == 0)
}

// leerApuestaRuleta separa cantidad y casilla. Devuelve cantidad=0 con
// eleccion="" y sin error cuando ya se contestó al usuario.
func leerApuestaRuleta(c *bot.Context) (int64, string, error) {
	var importe, casilla string
	for _, a := range c.Args {
		switch strings.ToLower(a) {
		case "rojo", "roja", "red", "negro", "negra", "black", "verde", "green", "cero":
			casilla = strings.ToLower(a)
		default:
			if n, err := strconv.Atoi(a); err == nil && n >= 0 && n <= 36 {
				casilla = a
				continue
			}
			if importe == "" {
				importe = a
			}
		}
	}
	switch casilla {
	case "roja", "red":
		casilla = "rojo"
	case "negra", "black":
		casilla = "negro"
	case "green":
		casilla = "verde"
	case "cero":
		casilla = "0"
	}
	if importe == "" {
		return 0, casilla, nil
	}

	cantidad, all, ok := parseAmount(importe)
	if !ok {
		return 0, casilla, c.Reply(fmt.Sprintf("ꕥ *%s* no es una cantidad válida.", importe))
	}
	chat, key := chatID(c), c.Key()
	acc, err := c.DB.ChatUser(c.Ctx, chat, key)
	if err != nil {
		return 0, casilla, err
	}
	if all {
		cantidad = acc.Coins
	}
	if cantidad < 200 {
		return 0, casilla, c.Reply(fmt.Sprintf("ꕥ La apuesta mínima es *%s*.", money(c, 200)))
	}
	if cantidad > acc.Coins {
		return 0, casilla, c.Reply(fmt.Sprintf("ꕥ Solo tienes *%s* en la cartera.", money(c, acc.Coins)))
	}
	return cantidad, casilla, nil
}

// ─── .casino / .apostar ─────────────────────────────────────────────────────

func registerCasino() {
	bot.Register(bot.Command{
		Name:     "casino",
		Aliases:  []string{"apostar", "dados", "dado"},
		Category: bot.CatGames,
		Desc:     "Tirar los dados contra el bot (paga ×1.9)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			apuesta, ok, err := cobrarApuesta(c, "last_casino", 100, 30*time.Second)
			if !ok {
				return err
			}

			// El JS tiraba "Aku" (el jugador) del 0 al 100 y "Kamu" (el bot) del
			// 0 al 54: el bot ganaba el 85 % de las veces y el jugador no lo
			// sabía. Aquí los dos tiran 1–100 y, si empatan, gana la casa (por
			// eso paga ×1.9 y no ×2).
			tuyo, mio := dado(1, 100), dado(1, 100)
			if tuyo > mio {
				ganancia := apuesta * 19 / 10
				coins, err := pagar(c, ganancia)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🎲 Tú *%d* — bot *%d*\n> ¡Ganaste *%s*!\n> Cartera: *%s*",
					tuyo, mio, money(c, ganancia), money(c, coins)))
			}
			nota := ""
			if tuyo == mio {
				nota = "\n> Empate: se queda la casa."
			}
			return c.Reply(fmt.Sprintf("🎲 Tú *%d* — bot *%d*%s\n> Perdiste *%s*",
				tuyo, mio, nota, money(c, apuesta)))
		},
	})
}

// ─── .ppt ───────────────────────────────────────────────────────────────────

var jugadasPPT = map[string]string{
	"piedra": "🪨", "roca": "🪨", "papel": "📄", "tijera": "✂️",
	"tijeras": "✂️", "lagarto": "🦎", "spock": "🖖",
}

// ganaPPT dice si `a` le gana a `b`. Además de las tres de siempre se aceptan
// lagarto y Spock, que hacen el juego más entretenido y no rompen nada.
func ganaPPT(a, b string) bool {
	norm := func(j string) string {
		switch j {
		case "roca":
			return "piedra"
		case "tijeras":
			return "tijera"
		}
		return j
	}
	a, b = norm(a), norm(b)
	switch a {
	case "piedra":
		return b == "tijera" || b == "lagarto"
	case "papel":
		return b == "piedra" || b == "spock"
	case "tijera":
		return b == "papel" || b == "lagarto"
	case "lagarto":
		return b == "papel" || b == "spock"
	case "spock":
		return b == "piedra" || b == "tijera"
	}
	return false
}

func registerPPT() {
	bot.Register(bot.Command{
		Name:     "ppt",
		Aliases:  []string{"piedrapapel", "cachipun"},
		Category: bot.CatGames,
		Desc:     "Piedra, papel o tijera contra el bot",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			jugada := ""
			if len(c.Args) > 0 {
				jugada = strings.ToLower(c.Args[0])
			}
			if _, ok := jugadasPPT[jugada]; !ok {
				return c.Reply(fmt.Sprintf("「✎」Elige una jugada: *%sppt piedra*, *papel*, *tijera*, *lagarto* o *spock*",
					c.Bot.FirstPrefix()))
			}

			chat, key := chatID(c), c.Key()
			listo, restante, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_ppt", time.Minute)
			if err != nil {
				return err
			}
			if !listo {
				return c.Reply(fmt.Sprintf("ꕥ Espera *%s* para volver a jugar.",
					formatWait(int64(restante.Seconds()))))
			}

			rival := elegir([]string{"piedra", "papel", "tijera", "lagarto", "spock"})
			mi := jugadasPPT[jugada] + " *" + jugada + "*"
			suyo := jugadasPPT[rival] + " *" + rival + "*"

			switch {
			case jugada == rival:
				premio := dado(800, 1500)
				coins, err := pagar(c, premio)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("✊ Tú %s · Bot %s\n> Empate. Consolación: *%s*\n> Cartera: *%s*",
					mi, suyo, money(c, premio), money(c, coins)))
			case ganaPPT(jugada, rival):
				premio := dado(3000, 5500)
				coins, err := pagar(c, premio)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("✊ Tú %s · Bot %s\n> ¡Ganaste *%s*!\n> Cartera: *%s*",
					mi, suyo, money(c, premio), money(c, coins)))
			default:
				perdida := dado(1000, 3000)
				pagado, err := c.DB.Charge(c.Ctx, chat, key, perdida)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("✊ Tú %s · Bot %s\n> Perdiste *%s*", mi, suyo, money(c, pagado)))
			}
		},
	})
}

// premioEsperadoSlot devuelve el multiplicador medio de la tragaperras. Vive
// aquí (y no en un test) porque es la comprobación de que la tabla sigue
// teniendo sentido si alguien toca los pesos o los premios.
func premioEsperadoSlot() float64 {
	total := 0.0
	for _, s := range tragaperras {
		total += float64(s.peso)
	}
	ev := 0.0
	for _, s := range tragaperras {
		p := float64(s.peso) / total
		ev += p * p * p * float64(s.paga) // tres iguales
		ev += 3 * p * p * (1 - p) * s.dos // exactamente dos iguales
	}
	return ev
}

// maxApuestaSlot: el techo de una sola tirada, para que un ×300 no meta más
// dinero del que MaxMoney admite.
const maxApuestaSlot = store.MaxMoney / 300

func init() {
	registerCoinflip()
	registerSlot()
	registerRoulette()
	registerCasino()
	registerPPT()
}
