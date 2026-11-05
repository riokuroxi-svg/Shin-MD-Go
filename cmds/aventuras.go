// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// aventuras.go — Las salidas que dan dinero a cambio de salud, stamina y
// objetos: pescar, cazar, minar, mazmorra, aventura, ritual, crimen, robo,
// cofre, además de la curación.
//
// Estas eran las que en el JS más fácil rompían la base: cada una leía el
// usuario entero, restaba salud/stamina/dinero en JavaScript y volvía a
// escribir todas las columnas. Si dos comandos coincidían (o si el proceso
// moría entre dos escrituras) quedaban números imposibles: salud en −7, banco
// en −3,000 o la misma stamina descontada dos veces. Aquí cada cambio es una
// sentencia con su límite dentro (MIN/MAX, `WHERE coins >= ?`) y el premio se
// suma sobre el saldo que hay en ese momento.
//
// Dos cosas del JS que NO se copiaron:
//
//   - `.mine` abría un mensaje con botones y dejaba la partida "pendiente"
//     3 minutos en memoria. Si el bot se reiniciaba, la partida quedaba
//     colgada con el dinero ya descontado. Aquí el evento de la mina se
//     resuelve en el mismo mensaje.
//   - `.steal` solo dejaba robar a quien llevaba "1 hora inactivo", pero esa
//     hora se calculaba con `lastCmd`, un campo que no existía: en la práctica
//     nunca se cumplía. Aquí el límite es por víctima: nadie puede ser robado
//     dos veces en la misma hora.
package cmds

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/bot"
	"shin-go/internal/store"
)

// ─── ayudas de las salidas ──────────────────────────────────────────────────

// tieneObjeto cuenta las unidades de un objeto en el inventario del chat.
func tieneObjeto(c *bot.Context, id string) int64 {
	inv, err := c.DB.Inventory(c.Ctx, chatID(c), c.Key())
	if err != nil {
		return 0
	}
	return inv[id]
}

// gastarHerramienta resta una unidad del objeto. Devuelve false si no había.
func gastarHerramienta(c *bot.Context, id string) (bool, error) {
	return c.DB.TakeItem(c.Ctx, chatID(c), c.Key(), id, 1)
}

// romperHerramienta tira la herramienta si sale el número malo. El JS lo hacía
// con "durabilidad −1..15 y se rompe a 10 o menos", que en la práctica la
// rompía casi siempre en el primer uso (y a veces la borraba sin decir nada).
// Aquí la probabilidad es explícita y se avisa cuando pasa.
func romperHerramienta(c *bot.Context, id string, prob float64) (bool, error) {
	if !azar(prob) {
		return false, nil
	}
	ok, err := gastarHerramienta(c, id)
	if err != nil || !ok {
		return false, err
	}
	return true, nil
}

// salida comprueba lo común de toda actividad: enfriamiento, salud y stamina.
// Devuelve la stamina que se gastó y si se puede seguir.
func salida(c *bot.Context, columna string, espera time.Duration, saludMin, saludMax, staminaMax int64) (int64, bool, error) {
	chat, key := chatID(c), c.Key()
	acc, err := c.DB.ChatUser(c.Ctx, chat, key)
	if err != nil {
		return 0, false, err
	}
	gasto := dado(1, staminaMax)
	if acc.Stamina < gasto {
		return 0, false, c.Reply(fmt.Sprintf(
			"ꕥ No tienes stamina suficiente (necesitas *%d* y tienes *%d*).\n> Recupérate con *%sheal*",
			gasto, acc.Stamina, c.Bot.FirstPrefix()))
	}
	if acc.Health < saludMin {
		return 0, false, c.Reply(fmt.Sprintf(
			"ꕥ No tienes salud suficiente (tienes *%d/%d*).\n> Recupérate con *%sheal*",
			acc.Health, store.MaxStat, c.Bot.FirstPrefix()))
	}

	listo, restante, err := c.DB.ClaimCooldown(c.Ctx, chat, key, columna, espera)
	if err != nil {
		return 0, false, err
	}
	if !listo {
		return 0, false, c.Reply(fmt.Sprintf("ꕥ Debes esperar *%s* antes de volver.",
			formatWait(int64(restante.Seconds()))))
	}

	if _, err := c.DB.AddStat(c.Ctx, chat, key, "stamina", -gasto); err != nil {
		return 0, false, err
	}
	if _, err := c.DB.AddStat(c.Ctx, chat, key, "health", -dado(0, saludMax)); err != nil {
		return 0, false, err
	}
	return gasto, true, nil
}

// salidaSimple: la versión sin salud mínima (pesca, caza, mina).
func salidaSimple(c *bot.Context, columna string, espera time.Duration) (bool, error) {
	_, ok, err := salida(c, columna, espera, 10, 4, 5)
	return ok, err
}

// ─── .pescar ────────────────────────────────────────────────────────────────

func registerFish() {
	const espera = 10 * time.Minute
	bot.Register(bot.Command{
		Name:     "pescar",
		Aliases:  []string{"fish", "pesca"},
		Category: bot.CatEconomy,
		Desc:     "Pescar para ganar dinero (necesitas caña)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			if tieneObjeto(c, "cana") <= 0 {
				return c.Reply(fmt.Sprintf("ꕥ Necesitas una *Caña de pescar*.\n> Cómprala con *%sshop*",
					c.Bot.FirstPrefix()))
			}
			ok, err := salidaSimple(c, "last_fish", espera)
			if err != nil || !ok {
				return err
			}
			rota, err := romperHerramienta(c, "cana", 0.08)
			if err != nil {
				return err
			}
			aviso := ""
			if rota {
				aviso = "\n> 💔 Tu caña se partió. Necesitarás otra."
			}

			if azar(0.6) {
				ganancia := dado(6000, 8000)
				coins, err := pagar(c, ganancia)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🎣 %s\n> Ganaste *%s*\n> Cartera: *%s*%s",
					elegir(pescaBuena), money(c, ganancia), money(c, coins), aviso))
			}
			perdida := dado(2000, 4000)
			pagado, err := c.DB.Charge(c.Ctx, chatID(c), c.Key(), perdida)
			if err != nil {
				return err
			}
			return c.Reply(fmt.Sprintf("🎣 %s\n> Perdiste *%s* (se te cayó el anzuelo)%s",
				elegir(pescaMala), money(c, pagado), aviso))
		},
	})
}

var pescaBuena = []string{
	"¡Pescaste una trucha enorme y la vendiste en el mercado!",
	"Un pez de plata mordió el anzuelo. Un coleccionista te lo compró caro.",
	"Encontraste una red olvidada llena de peces.",
	"Pescaste una anguila eléctrica… y la vendiste al mejor postor.",
	"Tu caña tembló y salió una cartera con dinero de un ahogado.",
}

var pescaMala = []string{
	"Solo sacaste una bota vieja y un resfriado.",
	"El pez se llevó tu anzuelo y tu paciencia.",
	"Un cangrejo te rompió la carnada.",
	"La corriente te arrastró la red.",
	"Un pez gigante rompió el sedal y se llevó parte de tu equipo.",
}

// ─── .cazar ─────────────────────────────────────────────────────────────────

func registerHunt() {
	const espera = 12 * time.Minute
	bot.Register(bot.Command{
		Name:     "cazar",
		Aliases:  []string{"hunt", "caceria"},
		Category: bot.CatEconomy,
		Desc:     "Cazar para ganar dinero (necesitas arco)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			if tieneObjeto(c, "arco") <= 0 {
				return c.Reply(fmt.Sprintf("ꕥ Necesitas un *Arco*.\n> Cómpralo con *%sshop*",
					c.Bot.FirstPrefix()))
			}
			ok, err := salidaSimple(c, "last_hunt", espera)
			if err != nil || !ok {
				return err
			}
			rota, err := romperHerramienta(c, "arco", 0.10)
			if err != nil {
				return err
			}
			aviso := ""
			if rota {
				aviso = "\n> 💔 El arco se quebró de tanto tensarlo."
			}

			if azar(0.6) {
				ganancia := dado(10000, 13000)
				coins, err := pagar(c, ganancia)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🏹 %s\n> Ganaste *%s*\n> Cartera: *%s*%s",
					elegir(cazaBuena), money(c, ganancia), money(c, coins), aviso))
			}
			perdida := dado(3000, 5000)
			pagado, err := c.DB.Charge(c.Ctx, chatID(c), c.Key(), perdida)
			if err != nil {
				return err
			}
			return c.Reply(fmt.Sprintf("🏹 %s\n> Perdiste *%s*%s",
				elegir(cazaMala), money(c, pagado), aviso))
		},
	})
}

var cazaBuena = []string{
	"Volviste con un jabalí y lo vendiste entero.",
	"Un ciervo enorme cayó de un solo flechazo.",
	"Cazaste dos conejos gordos y los vendiste en la feria.",
	"El lobo que te acechaba ahora es una alfombra carísima.",
	"Un comerciante te pagó bien por una piel impecable.",
}

var cazaMala = []string{
	"El ciervo te vio primero y desapareció.",
	"Tu flecha se perdió en el bosque y la presa escapó.",
	"Un oso te persiguió hasta el río.",
	"Ventisca en la montaña: volviste con las manos vacías y el arco mojado.",
	"Te esquivaron todos los animales del bosque.",
}

// ─── .minar ─────────────────────────────────────────────────────────────────

// eventoMina es un suceso que puede salir al minar. En el JS se preguntaba por
// botones (y la partida quedaba abierta 3 minutos); aquí ocurre de golpe, con
// la misma probabilidad de éxito, y se cuenta cómo acabó.
type eventoMina struct {
	emoji   string
	titulo  string
	prob    float64 // de que el minero acierte
	premio  int64
	castigo int64 // dinero que se pierde si falla
	stamina int64 // stamina extra que se pierde si falla
	bien    string
	mal     string
}

var eventosMina = []eventoMina{
	{"🎁", "cofre misterioso", 0.65, 3500, 800, 5,
		"Abriste el cofre: monedas brillantes por todas partes.",
		"¡Era una trampa! Un resorte te golpea la mano."},
	{"🧌", "duende de la mina", 0.50, 1000, 500, 0,
		"¡Ganaste la apuesta! El duende gruñe pero paga.",
		"El duende se ríe y te saca 500 monedas. Tramposo."},
	{"🪨", "roca brillante", 0.45, 2500, 200, 10,
		"Partiste la roca: había un puñado de gemas.",
		"Solo era pirita. Te cansaste para nada."},
	{"🐈", "gato negro", 0.85, 2000, 200, 0,
		"El gato te trae una bolsa de monedas como pago.",
		"El gato te mira mal y se lleva 200 monedas. Ingrato."},
}

// resolverEvento devuelve (premio, castigo, stamina, texto). Se separa de la
// base de datos para poder probarlo con una semilla fija.
func resolverEvento(e eventoMina) (int64, int64, int64, string) {
	if azar(e.prob) {
		return e.premio, 0, 0, e.bien
	}
	return 0, e.castigo, e.stamina, e.mal
}

func registerMine() {
	const espera = 10 * time.Minute
	bot.Register(bot.Command{
		Name:     "minar",
		Aliases:  []string{"mine", "mineria", "excavar"},
		Category: bot.CatEconomy,
		Desc:     "Minar para ganar dinero (necesitas pico)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			if tieneObjeto(c, "pico") <= 0 {
				return c.Reply(fmt.Sprintf("ꕥ Necesitas un *Pico*.\n> Cómpralo con *%sshop*",
					c.Bot.FirstPrefix()))
			}
			ok, err := salidaSimple(c, "last_mine", espera)
			if err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()

			// Hallazgo base y, la mitad de las veces, un evento.
			base := dado(3000, 7000)
			coins, err := pagar(c, base)
			if err != nil {
				return err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "⛏️ %s *%s*\n", elegir(frasesMina), money(c, base))

			if azar(0.50) {
				e := eventosMina[rand.IntN(len(eventosMina))]
				premio, castigo, stamina, texto := resolverEvento(e)
				if premio > 0 {
					if coins, err = pagar(c, premio); err != nil {
						return err
					}
				}
				if castigo > 0 {
					if _, err := c.DB.Charge(c.Ctx, chat, key, castigo); err != nil {
						return err
					}
				}
				if stamina > 0 {
					if _, err := c.DB.AddStat(c.Ctx, chat, key, "stamina", -stamina); err != nil {
						return err
					}
				}
				fmt.Fprintf(&b, "%s *%s*: %s\n", e.emoji, e.titulo, texto)
				if premio > 0 {
					fmt.Fprintf(&b, "> +%s\n", money(c, premio))
				}
				if castigo > 0 {
					fmt.Fprintf(&b, "> -%s\n", money(c, castigo))
				}
				if stamina > 0 {
					fmt.Fprintf(&b, "> -%d stamina\n", stamina)
				}
			}
			if rota, err := romperHerramienta(c, "pico", 0.10); err != nil {
				return err
			} else if rota {
				b.WriteString("> 💔 El pico se partió contra la roca.\n")
			}
			fmt.Fprintf(&b, "> Cartera: *%s*", money(c, coins))
			return c.Reply(b.String())
		},
	})
}

var frasesMina = []string{
	"Encontraste un viejo cofre con",
	"Hallaste una bolsa llena de",
	"Descubriste un saco de",
	"Rompiste una roca y adentro estaba",
	"Cavando profundo, hallaste",
	"Entre las raíces encontraste",
}

// ─── .mazmorra ──────────────────────────────────────────────────────────────

func registerDungeon() {
	const espera = 17 * time.Minute
	bot.Register(bot.Command{
		Name:     "mazmorra",
		Aliases:  []string{"dungeon"},
		Category: bot.CatEconomy,
		Desc:     "Explorar la mazmorra (hacha o magia)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			if err := armaOMagia(c, "hacha"); err != nil {
				return err
			}
			ok, err := salidaSimple(c, "last_dungeon", espera)
			if err != nil || !ok {
				return err
			}

			switch {
			case azar(0.4):
				ganancia := dado(12000, 15000)
				coins, err := pagar(c, ganancia)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🏰 %s\n> Ganaste *%s*\n> Cartera: *%s*",
					elegir(mazmorraBuena), money(c, ganancia), money(c, coins)))
			case azar(0.5): // 0.6 del total acumulado: el 30 % de las salidas
				perdida := dado(7500, 9000)
				pagado, err := c.DB.Charge(c.Ctx, chatID(c), c.Key(), perdida)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🏰 %s\n> Perdiste *%s*",
					elegir(mazmorraMala), money(c, pagado)))
			default:
				return c.Reply(fmt.Sprintf("🏰 %s\n> No ganaste ni perdiste nada.",
					elegir(mazmorraNeutra)))
			}
		},
	})
}

var mazmorraBuena = []string{
	"Derrotaste al guardián de las ruinas y reclamaste el tesoro.",
	"Descifraste los símbolos rúnicos y obtuviste recompensas ocultas.",
	"El espíritu de la reina ancestral te bendijo con una gema de poder.",
	"Derrotaste a un gólem de obsidiana y abriste un acceso secreto.",
	"Purificaste el altar corrompido y recibiste una bendición.",
}

var mazmorraMala = []string{
	"Un espectro te drenó energía antes de que pudieras escapar.",
	"Un basilisco te sorprendió en la cámara oculta y huiste herido.",
	"Fracasaste al invocar un portal y quedaste atrapado un rato.",
	"Un grupo de espectros te rodeó y te obligó a soltar el tesoro.",
}

var mazmorraNeutra = []string{
	"Activaste una trampa, pero evitaste el daño y aprendiste algo nuevo.",
	"La sala cambió de forma y perdiste tiempo explorando en círculos.",
	"Encontraste un mural antiguo que revela secretos de la mazmorra.",
}

// ─── .aventura ──────────────────────────────────────────────────────────────

func registerAdventure() {
	const espera = 20 * time.Minute
	bot.Register(bot.Command{
		Name:     "aventura",
		Aliases:  []string{"adventure", "aventurarse"},
		Category: bot.CatEconomy,
		Desc:     "Salir de aventura (espada o magia)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			if err := armaOMagia(c, "espada"); err != nil {
				return err
			}
			ok, err := salidaSimple(c, "last_adventure", espera)
			if err != nil || !ok {
				return err
			}

			switch {
			case azar(0.4):
				ganancia := dado(14000, 18000)
				coins, err := pagar(c, ganancia)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🗺️ %s\n> Ganaste *%s*\n> Cartera: *%s*",
					elegir(aventuraBuena), money(c, ganancia), money(c, coins)))
			case azar(0.5):
				perdida := dado(9000, 11000)
				pagado, err := c.DB.Charge(c.Ctx, chatID(c), c.Key(), perdida)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🗺️ %s\n> Perdiste *%s*",
					elegir(aventuraMala), money(c, pagado)))
			default:
				return c.Reply(fmt.Sprintf("🗺️ %s\n> No ganaste ni perdiste nada.",
					elegir(aventuraNeutra)))
			}
		},
	})
}

var aventuraBuena = []string{
	"Derrotaste a un ogro emboscado entre los árboles de Drakonia.",
	"Te convertiste en campeón del torneo de gladiadores de Valoria.",
	"Rescataste el libro mágico del altar de los Susurros.",
	"Derrotaste a un dragón joven en los acantilados de Flamear.",
	"Resolviste el acertijo de la cripta eterna y obtuviste un tesoro.",
}

var aventuraMala = []string{
	"El hechicero oscuro te lanzó una maldición y huiste perdiendo dinero.",
	"Te extraviaste en la jungla de Zarkelia y unos bandidos te asaltaron.",
	"Un basilisco te embistió y escapaste herido sin botín.",
	"Un grupo de trolls te emboscó y te quitó las pertenencias.",
}

var aventuraNeutra = []string{
	"Exploraste ruinas antiguas y aprendiste secretos ocultos.",
	"Seguiste la pista de un espectro, pero desapareció entre la niebla.",
	"Recorriste un bosque encantado y descubriste nuevas rutas.",
}

// armaOMagia: si no tienes el arma, la salida se paga con magia (como el JS).
// Devuelve error solo cuando ya se contestó al usuario.
func armaOMagia(c *bot.Context, arma string) error {
	if tieneObjeto(c, arma) > 0 {
		return nil
	}
	chat, key := chatID(c), c.Key()
	acc, err := c.DB.ChatUser(c.Ctx, chat, key)
	if err != nil {
		return err
	}
	coste := dado(1, 12)
	if acc.Magic < coste {
		item := store.Items[arma]
		return c.Reply(fmt.Sprintf(
			"ꕥ No tienes *%s* y tu magia está agotada (necesitas *%d* y tienes *%d*).\n"+
				"> Compra una con *%sshop* o recupera magia con *%spocion*",
			item.Name, coste, acc.Magic, c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
	}
	_, err = c.DB.AddStat(c.Ctx, chat, key, "magic", -coste)
	return err
}

// ─── .ritual ────────────────────────────────────────────────────────────────

func registerRitual() {
	const espera = 12 * time.Minute
	bot.Register(bot.Command{
		Name:     "ritual",
		Aliases:  []string{"invocar", "invoke"},
		Category: bot.CatEconomy,
		Desc:     "Ritual de invocación (necesitas tótem)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()
			acc, err := c.DB.ChatUser(c.Ctx, chat, key)
			if err != nil {
				return err
			}
			if acc.Inventory["totem"] <= 0 {
				return c.Reply(fmt.Sprintf("ꕥ Necesitas un *Tótem*.\n> Cómpralo con *%sshop*",
					c.Bot.FirstPrefix()))
			}
			if acc.Stamina < 10 || acc.Magic < 10 || acc.Health < 10 {
				return c.Reply(fmt.Sprintf(
					"ꕥ El ritual exige al menos *10* de salud, stamina y magia.\n"+
						"> Ahora tienes: ❤️ %d · ⚡ %d · ✨ %d",
					acc.Health, acc.Stamina, acc.Magic))
			}
			listo, restante, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_invoke", espera)
			if err != nil {
				return err
			}
			if !listo {
				return c.Reply(fmt.Sprintf("ꕥ Espera *%s* para invocar otro ritual.",
					formatWait(int64(restante.Seconds()))))
			}
			if ok, err := gastarHerramienta(c, "totem"); err != nil || !ok {
				if err != nil {
					return err
				}
				if e := c.DB.ReleaseCooldown(c.Ctx, chat, key, "last_invoke"); e != nil {
					return e
				}
				return c.Reply("ꕥ Te quedaste sin tótem justo antes del ritual.")
			}
			for stat, delta := range map[string]int64{
				"stamina": -dado(1, 5), "magic": -dado(1, 12), "health": -dado(1, 15),
			} {
				if _, err := c.DB.AddStat(c.Ctx, chat, key, stat, delta); err != nil {
					return err
				}
			}

			premio := dado(8000, 11000)
			texto := elegir(invocacionNormal)
			if azar(0.05) {
				premio = dado(11000, 13000)
				texto = elegir(invocacionLegendaria)
			} else if azar(0.15) {
				premio += dado(2500, 4500)
			}
			coins, err := pagar(c, premio)
			if err != nil {
				return err
			}
			return c.Reply(fmt.Sprintf("🕯️ %s\n> Ganaste *%s*\n> Cartera: *%s*",
				texto, money(c, premio), money(c, coins)))
		},
	})
}

var invocacionNormal = []string{
	"Tu ritual abre un portal y caen riquezas ardientes del vacío.",
	"Las velas se consumen y revelan un cofre lleno de monedas antiguas.",
	"El círculo de invocación brilla y aparecen gemas relucientes.",
	"Un espíritu menor te entrega un saco de oro como ofrenda.",
	"Los cánticos atraen un espectro que deja riquezas a tus pies.",
}

var invocacionLegendaria = []string{
	"¡Invocaste un espíritu ancestral que te entrega un tesoro legendario!",
	"Un dragón cósmico emerge del ritual y te concede riquezas.",
	"Los dioses antiguos responden y derraman oro celestial sobre ti.",
	"El portal dimensional se abre y un tesoro prohibido cae ante ti.",
}

// ─── .crime ─────────────────────────────────────────────────────────────────

func registerCrime() {
	const espera = 7 * time.Minute
	bot.Register(bot.Command{
		Name:     "crime",
		Aliases:  []string{"crimen", "delito"},
		Category: bot.CatEconomy,
		Desc:     "Delinque por dinero rápido (40 % de éxito)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()
			listo, restante, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_crime", espera)
			if err != nil {
				return err
			}
			if !listo {
				return c.Reply(fmt.Sprintf("ꕥ Debes esperar *%s* antes de intentarlo otra vez.",
					formatWait(int64(restante.Seconds()))))
			}

			if azar(0.4) {
				ganancia := dado(5500, 7500)
				coins, err := pagar(c, ganancia)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🕶️ %s\n> Ganaste *%s*\n> Cartera: *%s*",
					elegir(crimenBueno), money(c, ganancia), money(c, coins)))
			}
			multa := dado(4000, 6000)
			pagado, err := c.DB.Charge(c.Ctx, chat, key, multa)
			if err != nil {
				return err
			}
			return c.Reply(fmt.Sprintf("🚔 %s\n> Perdiste *%s*", elegir(crimenMalo), money(c, pagado)))
		},
	})
}

var crimenBueno = []string{
	"Hackeaste un cajero y sacaste el efectivo sin que saltara la alarma.",
	"Te infiltraste como técnico en una mansión y saliste con las joyas.",
	"Simulaste una transferencia falsa y cobraste antes de que la cancelaran.",
	"Vaciaste una cartera olvidada en un restaurante sin que nadie lo notara.",
	"Falsificaste entradas VIP y saliste con objetos exclusivos.",
}

var crimenMalo = []string{
	"Intentaste vender un reloj falso y el comprador llamó a la policía.",
	"Hackeaste una cuenta, pero olvidaste ocultar tu IP.",
	"El guardia nocturno de la joyería te descubrió.",
	"Trataste de escapar con mercancía robada y te atraparon.",
	"El banco bloqueó la transacción y te cobró la comisión.",
}

// ─── .rob ───────────────────────────────────────────────────────────────────

func registerSteal() {
	const espera = time.Hour
	bot.Register(bot.Command{
		Name:     "robar",
		Aliases:  []string{"steal", "rob"},
		Category: bot.CatEconomy,
		Desc:     "Robarle dinero a alguien (menciona o cita su mensaje)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			j, ok := target(c)
			if !ok {
				return c.Reply(fmt.Sprintf(
					"❀ Tienes que mencionar a alguien o citar su mensaje.\n> *%srobar @alguien*",
					c.Bot.FirstPrefix()))
			}
			chat, key := chatID(c), c.Key()
			vitima := userKey(c, j)
			if vitima == key {
				return c.Reply("ꕥ No puedes robarte a ti mismo.")
			}
			if c.Bot != nil && j.ToNonAD() == c.Bot.OwnID() {
				return c.Reply("ꕥ No puedo dejarte robarme a mí.")
			}

			cuenta, err := c.DB.ChatUser(c.Ctx, chat, vitima)
			if err != nil {
				return err
			}
			if cuenta.Coins < 3000 {
				return c.Reply("ꕥ Esa persona no tiene suficiente en la cartera como para que valga la pena.")
			}

			// Un robo por hora a la misma persona (el límite del JS, que en la
			// práctica nunca saltaba, aquí sí se guarda).
			puede, _, err := c.DB.ClaimCooldown(c.Ctx, chat, vitima, "last_robbed", espera)
			if err != nil {
				return err
			}
			if !puede {
				return c.Reply("ꕥ A esa persona ya le robaron hace poco. Búscate a otra.")
			}
			listo, restante, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_steal", espera)
			if err != nil {
				return err
			}
			if !listo {
				return c.Reply(fmt.Sprintf("ꕥ Debes esperar *%s* para intentar otro robo.",
					formatWait(int64(restante.Seconds()))))
			}

			if azar(0.3) {
				multa := dado(2000, 5000)
				pagado, err := c.DB.Charge(c.Ctx, chat, key, multa)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf(
					"🚔 Te pillaron con las manos en la masa.\n> Pagaste una multa de *%s*",
					money(c, pagado)))
			}

			// Se intenta robar 3000–9000; la transacción nunca deja a la víctima
			// en negativo, así que lo robado puede ser menos si se gastó el
			// dinero en el mismo segundo.
			querido := dado(3000, 9000)
			robado, err := c.DB.Steal(c.Ctx, chat, vitima, key, querido)
			if err != nil {
				return err
			}
			if robado <= 0 {
				return c.Reply("ꕥ No se pudo robar nada: el dinero había desaparecido de la cartera.")
			}
			return c.Reply(fmt.Sprintf("❀ Le robaste *%s* a *%s* sin que se diera cuenta.",
				money(c, robado), nombreDe(c, j)))
		},
	})
}

// nombreDe: nombre visible de una persona (push name si es quien escribe).
func nombreDe(c *bot.Context, j types.JID) string {
	if c.Sender.ToNonAD() == j.ToNonAD() && c.PushName != "" {
		return c.PushName
	}
	return strings.SplitN(j.ToNonAD().User, "@", 2)[0]
}

// ─── .cofre ─────────────────────────────────────────────────────────────────

func registerCoffer() {
	const espera = 24 * time.Hour
	bot.Register(bot.Command{
		Name:     "cofre",
		Aliases:  []string{"coffer", "abrircofre"},
		Category: bot.CatEconomy,
		Desc:     "Abrir tu cofre diario",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()
			listo, restante, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_coffer", espera)
			if err != nil {
				return err
			}
			if !listo {
				return c.Reply(fmt.Sprintf("ꕥ Debes esperar *%s* para abrir otro cofre.",
					formatWait(int64(restante.Seconds()))))
			}

			switch {
			case azar(0.5):
				coins, err := pagar(c, 25000)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🎁 Has abierto un cofre normal: *%s*\n> Cartera: *%s*",
					money(c, 25000), money(c, coins)))
			case azar(0.6): // 0.5 × 0.6 = 30 % de cofre legendario
				coins, err := pagar(c, 40000)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("🎁 ¡Cofre *legendario*! Recibiste *%s*\n> Cartera: *%s*",
					money(c, 40000), money(c, coins)))
			default:
				return c.Reply("🎁 Abriste el cofre con emoción… pero estaba vacío.")
			}
		},
	})
}

// ─── .heal / .pocion ────────────────────────────────────────────────────────

func registerHeal() {
	bot.Register(bot.Command{
		Name:     "heal",
		Aliases:  []string{"curar", "medico"},
		Category: bot.CatEconomy,
		Desc:     "Curar salud y stamina (cuesta dinero)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()

			quien, nombre := key, "ti"
			if j, ok := target(c); ok {
				quien = userKey(c, j)
				nombre = "*" + nombreDe(c, j) + "*"
			}
			paciente, err := c.DB.ChatUser(c.Ctx, chat, quien)
			if err != nil {
				return err
			}
			faltaSalud := store.MaxStat - paciente.Health
			faltaStamina := store.MaxStat - paciente.Stamina
			if faltaSalud <= 0 && faltaStamina <= 0 {
				return c.Reply(fmt.Sprintf("ꕥ La salud y la stamina de %s ya están al máximo (%d/%d).",
					nombre, paciente.Health, paciente.Stamina))
			}

			// Se cobra por tramos de 10 puntos, como el JS: 500 por tramo de
			// salud y 300 por tramo de stamina.
			tramosSalud := (faltaSalud + 9) / 10
			tramosStamina := (faltaStamina + 9) / 10
			coste := tramosSalud*500 + tramosStamina*300

			// Quien paga es siempre quien escribe, aunque cure a otro.
			pagado, err := c.DB.Charge(c.Ctx, chat, key, coste)
			if err != nil {
				return err
			}
			if pagado < coste {
				if pagado > 0 { // devolver lo cobrado a medias
					if _, e := c.DB.AddCoins(c.Ctx, chat, key, pagado); e != nil {
						return e
					}
				}
				return c.Reply(fmt.Sprintf(
					"ꕥ No te alcanza para curar a %s.\n> Cuesta *%s* y tienes *%s*",
					nombre, money(c, coste), money(c, pagado)))
			}

			if faltaSalud > 0 {
				if err := c.DB.SetStat(c.Ctx, chat, quien, "health", store.MaxStat); err != nil {
					return err
				}
			}
			if faltaStamina > 0 {
				if err := c.DB.SetStat(c.Ctx, chat, quien, "stamina", store.MaxStat); err != nil {
					return err
				}
			}
			return c.Reply(fmt.Sprintf(
				"ꕥ Curaste a %s al máximo.\n> Coste: *%s*\n> ❤️ %d/%d · ⚡ %d/%d",
				nombre, money(c, coste),
				store.MaxStat, store.MaxStat, store.MaxStat, store.MaxStat))
		},
	})

	bot.Register(bot.Command{
		Name:     "pocion",
		Aliases:  []string{"potion", "magia"},
		Category: bot.CatEconomy,
		Desc:     "Usar una poción para recuperar magia",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()

			quien, nombre := key, "ti"
			if j, ok := target(c); ok {
				quien = userKey(c, j)
				nombre = "*" + nombreDe(c, j) + "*"
			}
			if tieneObjeto(c, "pocion") <= 0 {
				return c.Reply(fmt.Sprintf("ꕥ No tienes pociones.\n> Cómpralas con *%sshop pocion*",
					c.Bot.FirstPrefix()))
			}
			paciente, err := c.DB.ChatUser(c.Ctx, chat, quien)
			if err != nil {
				return err
			}
			if paciente.Magic >= store.MaxStat {
				return c.Reply(fmt.Sprintf("ꕥ La magia de %s ya está al máximo (%d/%d).",
					nombre, paciente.Magic, store.MaxStat))
			}

			ok, err := gastarHerramienta(c, "pocion")
			if err != nil {
				return err
			}
			if !ok {
				return c.Reply("ꕥ Te quedaste sin pociones.")
			}
			if err := c.DB.SetStat(c.Ctx, chat, quien, "magic", store.MaxStat); err != nil {
				return err
			}
			return c.Reply(fmt.Sprintf("🧪 Usaste una poción en %s.\n> ✨ Magia: *%d/%d*",
				nombre, store.MaxStat, store.MaxStat))
		},
	})
}

func init() {
	registerFish()
	registerHunt()
	registerMine()
	registerDungeon()
	registerAdventure()
	registerRitual()
	registerCrime()
	registerSteal()
	registerCoffer()
	registerHeal()
}
