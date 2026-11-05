// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// economia.go — Puerto de cmds/economy/*.js.
//
// Reglas copiadas tal cual del bot de JS para que los números cuadren con lo
// que la gente ya conoce (y para que la base migrada valga):
//
//	.work     3 min de espera, paga 2,000–4,000
//	.daily    24 h, 20,000 + 5,000 por día de racha (tope 1,015,000), racha 200
//	.weekly   7 días, 40,000 + 5,000 por semana (tope 185,000), racha 30
//	.monthly  30 días, 60,000 + 5,000 por mes (tope 95,000), racha 8
//
// La diferencia con el JS no está en los números, está en cómo se guardan: allí
// se lee el saldo, se suma en JavaScript y se escribe detrás (dos mensajes a la
// vez = dinero duplicado). Aquí cada movimiento es una sola sentencia con su
// condición dentro y, si no se cumple, no pasa nada.
package cmds

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/bot"
	"shin-go/internal/store"
)

// ─── puerta de la economía (economyGate del JS) ─────────────────────────────

// ecoAllowed comprueba si la economía está activa en este chat. Si no lo está
// contesta el aviso y devuelve false, para que el comando se corte ahí.
//
// Igual que el JS: con `adminonly` activo o con la economía apagada, todos los
// comandos de dinero quedan cerrados.
func ecoAllowed(c *bot.Context) (bool, error) {
	if !c.IsGroup {
		// En privado no hay ajustes por chat: la economía siempre está abierta.
		return true, nil
	}
	on, err := c.DB.EconomyOn(c.Ctx, c.Chat.String())
	if err != nil {
		return false, err
	}
	admin, err := c.DB.AdminOnly(c.Ctx, c.Chat.String())
	if err != nil {
		return false, err
	}
	if on && !admin {
		return true, nil
	}
	prefix := c.Bot.FirstPrefix()
	return false, c.Reply(fmt.Sprintf(
		"ꕥ Los comandos de *Economía* están desactivados en este grupo.\n\n"+
			"Un *administrador* puede activarlos con el comando:\n» *%seconomy on*", prefix))
}

// chatID: la economía va por chat. En privado el "chat" es la persona, así que
// cada quien tiene su propio saldo en el privado del bot.
func chatID(c *bot.Context) string { return c.Chat.String() }

// userKey traduce un JID a la llave de la base (teléfono si se conoce).
func userKey(c *bot.Context, j types.JID) string {
	j = j.ToNonAD()
	if j.Server == types.HiddenUserServer || j.Server == "lid" {
		if pn, err := c.DB.PNForLID(c.Ctx, j.String()); err == nil && pn != "" {
			return pn
		}
	}
	return j.String()
}

// target: a quién van los comandos que aceptan mención o cita.
func target(c *bot.Context) (types.JID, bool) {
	if m := c.Mentioned(); len(m) > 0 {
		return m[0], true
	}
	return types.JID{}, false
}

func init() {
	registerDaily()
	registerWeekly()
	registerMonthly()
	registerWork()
	registerBalance()
	registerBank()
	registerTransfer()
	registerBoard()
	registerShop()
	registerEconomyToggle()
	registerPerfil()
}

// ─── premios (números del JS, en un solo sitio) ─────────────────────────────

// dailyReward: 20,000 + 5,000 por día de racha, con tope de 1,015,000
// (racha máxima 200). Es la fórmula de cmds/economy/daily.js.
func dailyReward(streak int64) int64 {
	if streak < 1 {
		streak = 1
	}
	return min64i(20000+(streak-1)*5000, 1015000)
}

// weeklyReward: 40,000 + 5,000 por semana, tope 185,000 (racha máxima 30).
func weeklyReward(streak int64) int64 {
	if streak < 1 {
		streak = 1
	}
	return min64i(40000+(streak-1)*5000, 185000)
}

// monthlyReward: 60,000 + 5,000 por mes, tope 95,000 (racha máxima 8).
func monthlyReward(streak int64) int64 {
	if streak < 1 {
		streak = 1
	}
	return min64i(60000+(streak-1)*5000, 95000)
}

// workReward: 2,000–4,000 al azar, como randomInt(2000, 4000) del JS.
func workReward() int64 { return int64(2000 + rand.Intn(2001)) }

// ─── .daily ─────────────────────────────────────────────────────────────────

func registerDaily() {
	bot.Register(bot.Command{
		Name:     "daily",
		Aliases:  []string{"diario", "recompensa", "gratis"},
		Category: bot.CatEconomy,
		Desc:     "Reclamar tu recompensa diaria",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			now := time.Now().UTC()
			chat, key := chatID(c), c.Key()

			// 1) Reserva del día EN ESTE CHAT (atómica).
			ok, wait, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_daily", 24*time.Hour)
			if err != nil {
				return err
			}
			if !ok {
				return c.Reply(fmt.Sprintf(
					"ꕥ Ya has reclamado hoy.\n> Vuelve en *%s*", formatWait(int64(wait.Seconds()))))
			}

			// 2) Racha global (una por persona, no una por grupo).
			st, err := c.DB.ClaimStreak(c.Ctx, key, store.StreakDaily, now)
			if err != nil {
				return err
			}
			streak := st.Streak
			if streak < 1 {
				streak = 1
			}

			premio := dailyReward(streak)

			// 3) Paquete de bienvenida: solo la primera vez que reclaman
			//    (el JS lo detecta por "no tiene pico").
			acc, err := c.DB.ChatUser(c.Ctx, chat, key)
			if err != nil {
				return err
			}
			bienvenida := acc.Inventory["pico"] == 0
			if bienvenida {
				premio += 15000
			}

			coins, err := c.DB.AddCoins(c.Ctx, chat, key, premio)
			if err != nil {
				return err
			}
			if bienvenida {
				if _, err := c.DB.AddItem(c.Ctx, chat, key, "pico", 1); err != nil {
					return err
				}
				if _, err := c.DB.AddItem(c.Ctx, chat, key, "pocion", 2); err != nil {
					return err
				}
				for _, stat := range []string{"health", "stamina", "magic"} {
					if err := c.DB.SetStat(c.Ctx, chat, key, stat, store.MaxStat); err != nil {
						return err
					}
				}
			}
			if _, err := c.DB.AddXP(c.Ctx, key, 50); err != nil {
				return err
			}

			msg := fmt.Sprintf("「✿」Has reclamado *%s*! (Día *%d*)\nDía *%d* » *+%s*",
				money(c, premio), streak, streak+1,
				moneyShort(c, dailyReward(streak+1)))
			if st.Streak == 0 || premio == 0 {
				msg += "\n> ☆ ¡Perdiste tu racha de días!"
			}
			msg += fmt.Sprintf("\n> Cartera: *%s*", money(c, coins))
			if bienvenida {
				msg += "\n\n🎁 *¡Paquete de bienvenida!*\n" +
					"▪ +15,000 extra\n" +
					"▪ ⛏️ *Pico* (100/100)\n" +
					"▪ 🧪 *2 Pociones*\n" +
					"▪ ❤️ Salud, ⚡ Stamina y ✨ Magia al 100%\n" +
					"Ya puedes usar *.work* para empezar."
			}
			return c.ReplyButtons("🎁 Recompensa diaria", msg, "❦ Shin-MD · economía",
				bot.QuickReply("💰 Mi saldo", "balance"),
				bot.QuickReply("🏦 Banco", "deposit"))
		},
	})
}

// ─── .weekly ────────────────────────────────────────────────────────────────

func registerWeekly() {
	bot.Register(bot.Command{
		Name:     "weekly",
		Aliases:  []string{"semanal"},
		Category: bot.CatEconomy,
		Desc:     "Reclamar tu recompensa semanal",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()
			ok, wait, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_weekly", 7*24*time.Hour)
			if err != nil {
				return err
			}
			if !ok {
				return c.Reply(fmt.Sprintf(
					"ꕥ Ya has reclamado tu recompensa semanal.\n> Puedes reclamarla de nuevo en *%s*",
					formatWait(int64(wait.Seconds()))))
			}

			st, err := c.DB.ClaimStreak(c.Ctx, key, store.StreakWeekly, time.Now().UTC())
			if err != nil {
				return err
			}
			semana := st.Streak
			if semana < 1 {
				semana = 1
			}
			coins := weeklyReward(semana)
			total, err := c.DB.AddCoins(c.Ctx, chat, key, coins)
			if err != nil {
				return err
			}

			msg := fmt.Sprintf("「❁」 Has reclamado tu recompensa semanal de *%s* (Semana *%d*)\n"+
				"> Semana *%d* » *+%s*\n> Cartera: *%s*",
				money(c, coins), semana,
				semana+1, moneyShort(c, weeklyReward(semana+1)),
				money(c, total))
			if st.Streak == 0 {
				msg += "\n> ☆ ¡Has perdido tu racha de semanas!"
			}
			return c.Reply(msg)
		},
	})
}

// ─── .monthly ───────────────────────────────────────────────────────────────

func registerMonthly() {
	bot.Register(bot.Command{
		Name:     "monthly",
		Aliases:  []string{"mensual"},
		Category: bot.CatEconomy,
		Desc:     "Reclamar tu recompensa mensual",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()
			ok, wait, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_monthly", 30*24*time.Hour)
			if err != nil {
				return err
			}
			if !ok {
				return c.Reply(fmt.Sprintf(
					"ꕥ Ya has reclamado tu recompensa mensual.\n> Puedes reclamarla de nuevo en *%s*",
					formatWait(int64(wait.Seconds()))))
			}

			st, err := c.DB.ClaimStreak(c.Ctx, key, store.StreakMonthly, time.Now().UTC())
			if err != nil {
				return err
			}
			mes := st.Streak
			if mes < 1 {
				mes = 1
			}
			coins := monthlyReward(mes)
			total, err := c.DB.AddCoins(c.Ctx, chat, key, coins)
			if err != nil {
				return err
			}

			msg := fmt.Sprintf("「❁」 Has reclamado tu recompensa mensual de *%s* (Mes *%d*)\n"+
				"> Mes *%d* » *+%s*\n> Cartera: *%s*",
				money(c, coins), mes,
				mes+1, moneyShort(c, monthlyReward(mes+1)),
				money(c, total))
			if st.Streak == 0 {
				msg += "\n> ☆ ¡Has perdido tu racha de meses!"
			}
			return c.Reply(msg)
		},
	})
}

// ─── .work ──────────────────────────────────────────────────────────────────

// trabajos: los mismos mensajes del JS (elegidos al azar).
var trabajos = []string{
	"Trabajas como recolector de fresas y ganas",
	"Eres asistente en un taller de cerámica y obtienes",
	"Diseñas páginas web y ganas",
	"Eres fotógrafo de bodas y recibes",
	"Atiendes una cafetería y ganas",
	"Repartes paquetes en bicicleta y ganas",
	"Traduces documentos y ganas",
	"Limpias una oficina y ganas",
	"Cuidas plantas en un invernadero y ganas",
	"Reparas celulares y ganas",
	"Das clases de matemáticas y ganas",
	"Cocinas en un puesto de tacos y ganas",
}

func registerWork() {
	bot.Register(bot.Command{
		Name:     "work",
		Aliases:  []string{"w", "chambear", "trabajar"},
		Category: bot.CatEconomy,
		Desc:     "Ganar dinero trabajando",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()

			ok, wait, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_work", 3*time.Minute)
			if err != nil {
				return err
			}
			if !ok {
				return c.Reply(fmt.Sprintf(
					"ꕥ Debes esperar *%s* para usar *%swork* de nuevo.",
					formatWait(int64(wait.Seconds())), c.Bot.FirstPrefix()))
			}

			ganancia := workReward()
			coins, err := c.DB.AddCoins(c.Ctx, chat, key, ganancia)
			if err != nil {
				return err
			}
			if _, err := c.DB.AddXP(c.Ctx, key, 10); err != nil {
				return err
			}

			return c.Reply(fmt.Sprintf("❀ %s *%s*.\n> Cartera: *%s*",
				trabajos[rand.Intn(len(trabajos))], money(c, ganancia), money(c, coins)))
		},
	})
}

// ─── .balance ───────────────────────────────────────────────────────────────

func registerBalance() {
	bot.Register(bot.Command{
		Name:     "balance",
		Aliases:  []string{"bal", "coins", "bank", "saldo", "cartera"},
		Category: bot.CatEconomy,
		Desc:     "Ver cuánto dinero tienes",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			key := c.Key()
			if j, ok := target(c); ok {
				key = userKey(c, j)
			}
			acc, err := c.DB.ChatUser(c.Ctx, chatID(c), key)
			if err != nil {
				return err
			}
			quien := "Tú"
			if key != c.Key() {
				quien = strings.SplitN(key, "@", 2)[0]
			}
			return c.ReplyButtons("💰 Cartera", fmt.Sprintf(
				"╭─「 *BALANCE* 」\n│ 👤 %s\n│ 💰 Cartera: %s\n│ 🏦 Banco: %s\n│ 🧮 Total: %s\n╰─\n"+
					"> El dinero de la cartera te lo pueden robar; el del banco no.",
				quien, money(c, acc.Coins), money(c, acc.Bank), money(c, acc.Total())),
				"❦ Shin-MD · economía",
				bot.QuickReply("🎁 Diaria", "daily"),
				bot.QuickReply("🏦 Depositar", "deposit"),
				bot.QuickReply("🏆 Ranking", "economyboard"))
		},
	})
}

// ─── .deposit / .withdraw ───────────────────────────────────────────────────

func registerBank() {
	bot.Register(bot.Command{
		Name:     "deposit",
		Aliases:  []string{"dep", "depositar", "d"},
		Category: bot.CatEconomy,
		Desc:     "Guardar dinero en el banco (no te lo pueden robar)",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()
			acc, err := c.DB.ChatUser(c.Ctx, chat, key)
			if err != nil {
				return err
			}

			raw := firstArg(c)
			if raw == "" {
				return c.Reply(fmt.Sprintf("《✧》 Ingresa la cantidad de *%s* que quieras *depositar*.", moneda(c)))
			}
			amount, all, ok := parseAmount(raw)
			if !ok {
				return c.Reply("✎ Ingresa una cantidad *válida* para depositar")
			}
			if all {
				amount = acc.Coins
			}
			if amount <= 0 || acc.Coins <= 0 {
				return c.Reply(fmt.Sprintf("✎ No tienes *%s* para depositar en tu *banco*", moneda(c)))
			}

			ok, err = c.DB.Deposit(c.Ctx, chat, key, amount)
			if err != nil {
				return err
			}
			if !ok {
				return c.Reply(fmt.Sprintf("❀ No tienes suficientes *%s* para depositar", moneda(c)))
			}
			return c.Reply(fmt.Sprintf("ꕥ Has depositado *%s* en tu Banco", money(c, amount)))
		},
	})

	bot.Register(bot.Command{
		Name:     "withdraw",
		Aliases:  []string{"with", "retirar"},
		Category: bot.CatEconomy,
		Desc:     "Sacar dinero del banco",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()
			acc, err := c.DB.ChatUser(c.Ctx, chat, key)
			if err != nil {
				return err
			}

			raw := firstArg(c)
			if raw == "" {
				return c.Reply(fmt.Sprintf("《✧》 Ingresa la cantidad de *%s* que quieras retirar.", moneda(c)))
			}
			amount, all, ok := parseAmount(raw)
			if !ok {
				return c.Reply(fmt.Sprintf("《✧》 Debes retirar una cantidad válida.\n"+
					"> Ejemplo 1 » *%swithdraw 25000*\n> Ejemplo 2 » *%swithdraw all*",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}
			if all {
				amount = acc.Bank
			}
			if amount <= 0 || acc.Bank < amount {
				return c.Reply(fmt.Sprintf("《✧》 No tienes suficientes *%s* en tu banco.\n> Solo tienes *%s*.",
					moneda(c), money(c, acc.Bank)))
			}

			ok, err = c.DB.Withdraw(c.Ctx, chat, key, amount)
			if err != nil {
				return err
			}
			if !ok {
				return c.Reply(fmt.Sprintf("《✧》 No tienes suficientes *%s* en tu banco.", moneda(c)))
			}
			return c.Reply(fmt.Sprintf("✎ Has retirado *%s* del banco, ahora podrás usarlo pero también podrán robártelo.",
				money(c, amount)))
		},
	})
}

// ─── .pay / .givecoins / .transfer ──────────────────────────────────────────

func registerTransfer() {
	bot.Register(bot.Command{
		Name:     "pay",
		Aliases:  []string{"givecoins", "transfer", "dar", "pagar"},
		Category: bot.CatEconomy,
		Desc:     "Pasar dinero a otra persona (desde tu banco)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()

			dst, ok := target(c)
			if !ok {
				return c.Reply(fmt.Sprintf("《✧》 Menciona o responde a quien quieras darle *%s*.\n» *%spay @persona 1000*",
					moneda(c), c.Bot.FirstPrefix()))
			}
			to := userKey(c, dst)
			if to == key {
				return c.Reply("✎ No puedes darte dinero a ti mismo.")
			}

			// La cantidad puede venir antes o después de la mención.
			var (
				amount int64
				all    bool
				parsed bool
			)
			for _, a := range c.Args {
				if strings.HasPrefix(a, "@") {
					continue
				}
				if v, isAll, ok := parseAmount(a); ok {
					amount, all, parsed = v, isAll, true
					break
				}
			}
			if !parsed {
				return c.Reply(fmt.Sprintf("《✧》 Ingresa la cantidad de *%s* que quieras dar.", moneda(c)))
			}

			acc, err := c.DB.ChatUser(c.Ctx, chat, key)
			if err != nil {
				return err
			}
			if all {
				amount = acc.Bank
			}
			if amount <= 0 {
				return c.Reply(fmt.Sprintf("✎ No tienes *%s* en el banco para dar.", moneda(c)))
			}
			if amount > acc.Bank {
				return c.Reply(fmt.Sprintf("《✧》 No tienes suficientes *%s* en el banco.\n> Tienes *%s*.",
					moneda(c), money(c, acc.Bank)))
			}

			if ok, err := c.DB.Transfer(c.Ctx, chat, key, to, amount); err != nil {
				return err
			} else if !ok {
				return c.Reply(fmt.Sprintf("《✧》 No se pudo hacer la transferencia. Revisa tu saldo en el banco."))
			}
			return c.Reply(fmt.Sprintf("✎ Le has dado *%s* a *%s*\n> Sale de tu banco, no de la cartera.",
				money(c, amount), strings.SplitN(to, "@", 2)[0]))
		},
	})
}

// ─── .economyboard / .ranking ───────────────────────────────────────────────

func registerBoard() {
	bot.Register(bot.Command{
		Name:     "economyboard",
		Aliases:  []string{"eboard", "baltop", "ranking", "top"},
		Category: bot.CatEconomy,
		Desc:     "Ranking de dinero del grupo",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			const minTotal = 1000
			const perPage = 10

			pagina := 1
			if len(c.Args) > 0 {
				if n, err := strconv.Atoi(c.Args[0]); err == nil && n > 0 {
					pagina = n
				}
			}

			// Se pide de más para poder filtrar el mínimo y paginar sin traer
			// la tabla entera a memoria (el JS hacía limit 1000 y filtraba en JS).
			rows, err := c.DB.Board(c.Ctx, chatID(c), 500)
			if err != nil {
				return err
			}
			visibles := make([]store.BoardRow, 0, len(rows))
			for _, r := range rows {
				if r.Total() >= minTotal {
					visibles = append(visibles, r)
				}
			}
			if len(visibles) == 0 {
				return c.Reply(fmt.Sprintf("ꕥ No hay usuarios en el grupo con más de 1,000 %s.", moneda(c)))
			}

			totalPaginas := (len(visibles) + perPage - 1) / perPage
			if pagina > totalPaginas {
				return c.Reply(fmt.Sprintf("《✧》 La página *%d* no existe. Hay *%d* páginas.", pagina, totalPaginas))
			}
			ini := (pagina - 1) * perPage
			fin := min64i(int64(ini+perPage), int64(len(visibles)))

			var b strings.Builder
			b.WriteString("*✩ EconomyBoard (✿◡‿◡)*\n\n")
			for i := ini; i < int(fin); i++ {
				r := visibles[i]
				nombre := strings.SplitN(r.UserID, "@", 2)[0]
				fmt.Fprintf(&b, "✩ %d › *%s*\n     Total → *%s*\n", i+1, nombre, money(c, r.Total()))
			}
			fmt.Fprintf(&b, "\n> ⌦ Página *%d* de *%d*", pagina, totalPaginas)
			botones := []bot.Button{bot.QuickReply("💰 Mi saldo", "balance")}
			if pagina < totalPaginas {
				fmt.Fprintf(&b, "\n> Para ver la siguiente página › *%seconomyboard %d*", c.Bot.FirstPrefix(), pagina+1)
				botones = append([]bot.Button{bot.QuickReply("▶️ Siguiente página",
					"economyboard "+strconv.Itoa(pagina+1))}, botones...)
			}
			return c.ReplyButtons(fmt.Sprintf("🏆 Ranking · página %d de %d", pagina, totalPaginas),
				b.String(), "❦ Shin-MD · economía", botones...)
		},
	})
}

// ─── .shop / .inv ───────────────────────────────────────────────────────────

func registerShop() {
	bot.Register(bot.Command{
		Name:     "shop",
		Aliases:  []string{"tienda", "buy", "comprar"},
		Category: bot.CatEconomy,
		Desc:     "Ver la tienda o comprar un objeto",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()

			// Sin argumentos: lista de precios.
			if len(c.Args) == 0 {
				var b strings.Builder
				b.WriteString("*「✿」Tienda* ◢ Shin-MD ◤\n")
				for _, id := range store.ItemsOrder {
					it := store.Items[id]
					extra := ""
					if it.Uses > 0 {
						extra = fmt.Sprintf(" (%d usos)", it.Uses)
					}
					fmt.Fprintf(&b, "%s *%s* › *%s*%s\n    %s\n", it.Emoji, it.Name, money(c, it.Price), extra, it.Desc)
				}
				fmt.Fprintf(&b, "\n> Para comprar: *%sshop <objeto>*\n> Ejemplo: *%sshop espada*",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix())
				return c.Reply(b.String())
			}

			// Con argumentos: comprar. Se admite "caña de pescar" con espacios.
			q := strings.Join(c.Args, " ")
			id, okFind := store.FindItem(q)
			if !okFind {
				return c.Reply(fmt.Sprintf("《✧》 Ese objeto no existe en la tienda.\n> Mira la lista con *%sshop*",
					c.Bot.FirstPrefix()))
			}
			it := store.Items[id]

			// Se cobra primero y se entrega después, pero ambas cosas en el
			// mismo sentido: si el cobro no entra, no se entrega nada.
			paid, err := c.DB.Spend(c.Ctx, chat, key, it.Price)
			if err != nil {
				return err
			}
			if !paid {
				acc, err := c.DB.ChatUser(c.Ctx, chat, key)
				if err != nil {
					return err
				}
				return c.Reply(fmt.Sprintf("《✧》 Te faltan *%s* para comprar *%s*.\n> Tienes *%s* en la cartera.",
					money(c, it.Price-acc.Coins), it.Name, money(c, acc.Coins)))
			}
			total, err := c.DB.AddItem(c.Ctx, chat, key, id, 1)
			if err != nil {
				// Devolver el dinero: la compra no se completó.
				if _, e := c.DB.AddCoins(c.Ctx, chat, key, it.Price); e != nil {
					return e
				}
				return err
			}
			return c.Reply(fmt.Sprintf("ꕥ Compraste %s *%s* por *%s*\n> Ahora tienes *%d*.",
				it.Emoji, it.Name, money(c, it.Price), total))
		},
	})

	bot.Register(bot.Command{
		Name:     "inventory",
		Aliases:  []string{"inv", "inventario", "mochila"},
		Category: bot.CatEconomy,
		Desc:     "Ver tu inventario",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			if ok, err := ecoAllowed(c); err != nil || !ok {
				return err
			}
			key := c.Key()
			if j, ok := target(c); ok {
				key = userKey(c, j)
			}
			acc, err := c.DB.ChatUser(c.Ctx, chatID(c), key)
			if err != nil {
				return err
			}

			nombre := c.PushName
			if nombre == "" {
				nombre = strings.SplitN(key, "@", 2)[0]
			}
			var b strings.Builder
			fmt.Fprintf(&b, "*「✿」Inventario* ◢ %s ◤\n", nombre)
			fmt.Fprintf(&b, "⛁ Dinero total › *%s*\n", money(c, acc.Total()))
			fmt.Fprintf(&b, "♡ Salud › *%d/%d*\n", acc.Health, store.MaxStat)
			fmt.Fprintf(&b, "♛ Stamina › *%d/%d*\n", acc.Stamina, store.MaxStat)
			fmt.Fprintf(&b, "⸙ Magia › *%d/%d*\n", acc.Magic, store.MaxStat)

			if len(acc.Inventory) == 0 {
				b.WriteString("\n> No tienes objetos todavía. Compra en *" + c.Bot.FirstPrefix() + "shop*")
			} else {
				b.WriteString("\n")
				for _, id := range store.ItemsOrder {
					n := acc.Inventory[id]
					if n <= 0 {
						continue
					}
					it := store.Items[id]
					fmt.Fprintf(&b, "%s %s › *%d*\n", it.Emoji, it.Name, n)
				}
			}
			return c.Reply(b.String())
		},
	})
}

// ─── .economy on|off ────────────────────────────────────────────────────────

func registerEconomyToggle() {
	bot.Register(bot.Command{
		Name:      "economy",
		Aliases:   []string{"economia", "eco"},
		Category:  bot.CatEconomy,
		Desc:      "Activar o desactivar la economía en el grupo",
		GroupOnly: true,
		AdminOnly: true,
		Cooldown:  2,
		Run: func(c *bot.Context) error {
			opcion := strings.ToLower(strings.TrimSpace(firstArg(c)))
			switch opcion {
			case "":
				on, err := c.DB.EconomyOn(c.Ctx, c.Chat.String())
				if err != nil {
					return err
				}
				estado := "activada ✅"
				if !on {
					estado = "desactivada ⛔"
				}
				return c.Reply(fmt.Sprintf("「✿」La economía está *%s* en este grupo.\n"+
					"> *%seconomy on* / *%seconomy off*", estado, c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			case "on", "si", "sí", "1", "true":
				if err := c.DB.SetEconomy(c.Ctx, c.Chat.String(), true); err != nil {
					return err
				}
				return c.Reply("ꕥ Economía *activada* en este grupo.")
			case "off", "no", "0", "false":
				if err := c.DB.SetEconomy(c.Ctx, c.Chat.String(), false); err != nil {
					return err
				}
				return c.Reply("ꕥ Economía *desactivada* en este grupo.")
			default:
				return c.Reply(fmt.Sprintf("《✧》 Usa *%seconomy on* o *%seconomy off*",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}
		},
	})
}

// ─── .perfil ────────────────────────────────────────────────────────────────

func registerPerfil() {
	bot.Register(bot.Command{
		Name:     "perfil",
		Aliases:  []string{"profile", "yo"},
		Category: bot.CatPerfil,
		Desc:     "Tu perfil y economía",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			key := c.Key()
			acc, err := c.DB.ChatUser(c.Ctx, chatID(c), key)
			if err != nil {
				return err
			}
			global, err := c.DB.Get(c.Ctx, key)
			if err != nil {
				return err
			}
			dias, err := c.DB.StreakValue(c.Ctx, key, store.StreakDaily)
			if err != nil {
				return err
			}
			semanas, err := c.DB.StreakValue(c.Ctx, key, store.StreakWeekly)
			if err != nil {
				return err
			}

			nombre := c.PushName
			if nombre == "" {
				nombre = strings.SplitN(key, "@", 2)[0]
			}
			daily := "disponible ✓"
			if time.Now().UnixMilli() < acc.LastDaily {
				daily = "ya reclamado · " + formatWait((acc.LastDaily-time.Now().UnixMilli())/1000)
			}
			return c.Reply(fmt.Sprintf(
				"╭─「 *PERFIL* 」\n│ 👤 %s\n│ 💰 Cartera: %s\n│ 🏦 Banco: %s\n"+
					"│ 🧮 Total: %s\n│ ⭐ Nivel: %d (%d XP)\n│ 🔥 Racha: %d días · %d semanas\n"+
					"│ 🎁 Daily: %s\n╰─",
				nombre, money(c, acc.Coins), money(c, acc.Bank), money(c, acc.Total()),
				global.Level, global.XP, dias, semanas, daily))
		},
	})
}

// ─── ayudas ─────────────────────────────────────────────────────────────────

// firstArg: el primer argumento ya limpio.
func firstArg(c *bot.Context) string {
	if len(c.Args) == 0 {
		return ""
	}
	return c.Args[0]
}

func min64i(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
