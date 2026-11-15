// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// gacha_extra.go — la economía del harem que quedaba del JS: venta, compra,
// tienda, intercambio, regalo, robo, tableros y fichas (.charinfo, .serielist…).
//
// Las monedas son LAS de la economía (.wallet/.bank): no hay una caja aparte
// para gacha, igual que en el JS, donde sell y buychar movían `addCoins` del
// mismo bolsillo que .daily.
package cmds

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"shin-go/internal/bot"
	"shin-go/internal/gacha"
	"shin-go/internal/store"
)

func init() {
	bot.Register(bot.Command{
		Name: "sell", Aliases: []string{"vender"}, Category: bot.CatGacha,
		Desc: "Pone uno de tus personajes a la venta: .sell <id> <precio> (0 para retirar).",
		Run:  cmdSell,
	})
	bot.Register(bot.Command{
		Name: "buychar", Aliases: []string{"buyc", "buycharacter"}, Category: bot.CatGacha,
		Desc: "Compra un personaje en venta: .buychar <id>.",
		Run:  cmdBuyChar,
	})
	bot.Register(bot.Command{
		Name: "haremshop", Aliases: []string{"wshop", "tiendawaifus"}, Category: bot.CatGacha,
		Desc: "Tienda del chat: personajes a la venta.",
		Run:  cmdHaremShop,
	})
	bot.Register(bot.Command{
		Name: "removesale", Aliases: []string{"removerventa"}, Category: bot.CatGacha,
		Desc: "Quita a tu personaje de la venta: .removesale <id>.",
		Run:  cmdRemoveSale,
	})
	bot.Register(bot.Command{
		Name: "trade", Aliases: []string{"intercambiar"}, Category: bot.CatGacha,
		Desc: "Intercambio con alguien: .trade @user <tu_id> <su_id>. El otro confirma con el mismo comando.",
		Run:  cmdTrade,
	})
	bot.Register(bot.Command{
		Name: "robwaifu", Aliases: []string{"robarwaifu"}, Category: bot.CatGacha,
		Desc: "Intenta robar un personaje de @user (50%, uno cada 10 min; los ⭐ y los a la venta no se pueden robar).",
		Run:  cmdRobWaifu,
	})
	bot.Register(bot.Command{
		Name: "givechar", Aliases: []string{"givewaifu", "regalar"}, Category: bot.CatGacha,
		Desc: "Regala un personaje: .givechar @user <id>.",
		Run:  cmdGiveChar,
	})
	bot.Register(bot.Command{
		Name: "delwaifu", Aliases: []string{"delchar", "deletewaifu"}, Category: bot.CatGacha,
		Desc: "Suelta un personaje tuyo (vuelve al pool): .delwaifu <id>.",
		Run:  cmdDelWaifu,
	})
	bot.Register(bot.Command{
		Name: "deletefav", Aliases: []string{"delfav"}, Category: bot.CatGacha,
		Desc: "Quita tu personaje favorito.",
		Run:  cmdDelFav,
	})
	bot.Register(bot.Command{
		Name: "favtop", Aliases: []string{"favoritetop", "favboard"}, Category: bot.CatGacha,
		Desc: "Personajes más marcados como favorito ⭐ en el chat.",
		Run:  func(c *bot.Context) error { return cmdTableroGacha(c, true) },
	})
	bot.Register(bot.Command{
		Name: "wtop", Aliases: []string{"waifustop"}, Category: bot.CatGacha,
		Desc: "Personajes más reclamados del chat.",
		Run:  func(c *bot.Context) error { return cmdTableroGacha(c, false) },
	})
	bot.Register(bot.Command{
		Name: "charinfo", Aliases: []string{"winfo", "waifuinfo"}, Category: bot.CatGacha,
		Desc: "Ficha del personaje: .charinfo <id>.",
		Run:  cmdCharInfo,
	})
	bot.Register(bot.Command{
		Name: "serielist", Aliases: []string{"slist", "animelist"}, Category: bot.CatGacha,
		Desc: "Series del catálogo.",
		Run:  cmdSerieList,
	})
	bot.Register(bot.Command{
		Name: "serieinfo", Aliases: []string{"ainfo", "animeinfo"}, Category: bot.CatGacha,
		Desc: "Personajes de una serie: .serieinfo <nombre>.",
		Run:  cmdSerieInfo,
	})
	bot.Register(bot.Command{
		Name: "charimage", Aliases: []string{"waifuimage", "cimage", "wimage"}, Category: bot.CatGacha,
		Desc: "Busca una imagen del personaje o tag: .charimage <nombre>.",
		Run:  cmdCharImage,
	})
	bot.Register(bot.Command{
		Name: "setclaimmsg", Aliases: []string{"setclaim"}, Category: bot.CatGacha,
		Desc: "Texto del chat al reclamar un personaje: .setclaimmsg <texto>.",
		Run:  cmdSetClaimMsg,
	})
	bot.Register(bot.Command{
		Name: "delclaimmsg", Aliases: []string{"resetclaimmsg"}, Category: bot.CatGacha,
		Desc: "Quita el texto de reclamo del chat.",
		Run:  cmdDelClaimMsg,
	})
	bot.Register(bot.Command{
		Name: "setvote", Aliases: []string{"vote", "votar"}, Category: bot.CatGacha,
		Desc: "Suma valor a tu personaje: .setvote <id> (una vez al día).",
		Run:  cmdVote,
	})
}

// ── ventas ──────────────────────────────────────────────────────────────────

func cmdSell(c *bot.Context) error {
	if ok, err := gachaAllowed(c); !ok {
		return err
	}
	if len(c.Args) < 2 {
		return c.Reply("Uso: *.sell <id> <precio>* (precio 0 = quitar de la venta).")
	}
	precio, err := strconv.ParseInt(c.Args[1], 10, 64)
	if err != nil || precio < 0 {
		return c.Reply("El precio tiene que ser un número sin letras.")
	}
	chat := c.Chat.String()
	dueno, _ := c.DB.OwnerOf(c.Ctx, chat, c.Args[0])
	if dueno == nil || dueno.OwnerID != c.SenderKey {
		return c.Reply("ese personaje no es tuyo (mira *.harem*).")
	}
	if _, err := c.DB.PutOnSale(c.Ctx, chat, c.SenderKey, c.Args[0], precio); err != nil {
		return err
	}
	if precio == 0 {
		return c.Reply("✔ Fuera de venta.")
	}
	return c.Reply(fmt.Sprintf("🏷 «%s» a la venta por *%d*. Quien quiera: *.buychar %s*", dueno.Name, precio, c.Args[0]))
}

func cmdBuyChar(c *bot.Context) error {
	if ok, err := gachaAllowed(c); !ok {
		return err
	}
	if len(c.Args) < 1 {
		return c.Reply("¿Cuál compro? Mira la tienda con *.haremshop* y dame el id.")
	}
	chat := c.Chat.String()
	sale, err := c.DB.SaleByID(c.Ctx, chat, c.Args[0])
	if err != nil {
		return err
	}
	if sale == nil {
		return c.Reply("Ese no está a la venta (o ya se fue).")
	}
	if sale.OwnerID == c.SenderKey {
		return c.Reply("es tuyo; si lo quieres de vuelta, retíralo con *.removesale*.")
	}
	// Cobra antes de mover: sin fondos, no hay trato.
	if _, err := c.DB.Charge(c.Ctx, chat, c.SenderKey, sale.Price); err != nil {
		return c.Reply(fmt.Sprintf("No tienes suficiente: cuesta *%d*.", sale.Price))
	}
	if err := c.DB.BuySale(c.Ctx, chat, c.SenderKey, sale); err != nil {
		// Devolución si el truco se caía a medias (el cargo ya salió).
		_, _ = c.DB.AddCoins(c.Ctx, chat, c.SenderKey, sale.Price)
		return err
	}
	return c.Reply(fmt.Sprintf("💸 Compraste a *%s* por %d. Ya es tuyo.", sale.Name, sale.Price))
}

func cmdHaremShop(c *bot.Context) error {
	if ok, err := gachaAllowed(c); !ok {
		return err
	}
	ventas, err := c.DB.ListSales(c.Ctx, c.Chat.String(), 10)
	if err != nil {
		return err
	}
	if len(ventas) == 0 {
		return c.Reply("La tienda del chat está vacía; vende algo con *.sell <id> <precio>*.")
	}
	var sb strings.Builder
	sb.WriteString("🛒 *Tienda del harem*\n\n")
	for _, v := range ventas {
		fmt.Fprintf(&sb, "· *%s* (%s) — %d 💰 · dueño: @%s\n", v.Name, v.CharID, v.Price, v.OwnerID)
	}
	return c.Reply(strings.TrimRight(sb.String(), "\n"))
}

func cmdRemoveSale(c *bot.Context) error {
	if len(c.Args) < 1 {
		return c.Reply("¿Cuál retiro? *.removesale <id>*")
	}
	ok, err := c.DB.PutOnSale(c.Ctx, c.Chat.String(), c.SenderKey, c.Args[0], 0)
	if err != nil {
		return err
	}
	if !ok {
		return c.Reply("no tenías ese a la venta (mira *.harem*).")
	}
	return c.Reply("✔ Quitado de la tienda.")
}

// ── trade, regalo, robo, soltar ─────────────────────────────────────────────

// tradePendiente: el ofrecido confirma con el MISMO comando. Como el JS, no
// hay cola de confirmación persistente: se guarda 2 minutos en memoria.
var tradePend = struct {
	when  map[string]time.Time
	lista map[string][4]string // chat → [a, ida, b, idb]
}{map[string]time.Time{}, map[string][4]string{}}

func cmdTrade(c *bot.Context) error {
	if ok, err := gachaAllowed(c); !ok {
		return err
	}
	m := c.Mentioned()
	if len(m) == 0 || len(c.Args) < 2 {
		return c.Reply("*.trade @user <tu_id> <su_id>* — la otra persona lo repite para confirmar.")
	}
	chat := c.Chat.String()
	key := chat + "|" + c.SenderKey
	_ = key
	// la oferta se guarda bajo la llave del OTRO: él confirma
	miID, suID := c.Args[1], ""
	if len(c.Args) > 2 {
		suID = c.Args[2]
	}

	// ¿Estoy confirmando una oferta previa del otro?
	if pend, ok := tradePend.lista[key]; ok && time.Since(tradePend.when[key]) < 2*time.Minute {
		delete(tradePend.lista, key)
		delete(tradePend.when, key)
		a, ida, _, _ := pend[0], pend[1], pend[2], pend[3]
		// a ofrece ida a mí; yo confirmo entregando pend[3]
		if err := c.DB.SwapCharacters(c.Ctx, chat, a, ida, c.SenderKey, miID); err != nil {
			return c.Reply("el intercambio se cayó: uno de los dos ya no tiene su personaje.")
		}
		return c.Reply("🔁 Intercambio hecho.")
	}

	if suID == "" {
		return c.Reply("me falta el id del personaje del otro: *.trade @user <tu_id> <su_id>*")
	}
	// Valido que los dos personajes existen y son de quien dice ser.
	ahora, _ := c.DB.OwnerOf(c.Ctx, chat, miID)
	suya, _ := c.DB.OwnerOf(c.Ctx, chat, suID)
	if ahora == nil || ahora.OwnerID != c.SenderKey {
		return c.Reply("el primer id no es tuyo.")
	}
	if suya == nil || suya.OwnerID != userKey(c, m[0]) {
		return c.Reply("el segundo id no es de quien mencionas.")
	}
	tradePend.lista[chat+"|"+userKey(c, m[0])] = [4]string{c.SenderKey, miID, c.SenderKey, suID}
	tradePend.when[chat+"|"+userKey(c, m[0])] = time.Now()
	return c.Reply(fmt.Sprintf("🔁 Oferta lista: tú das *%s* (%s) y recibes *%s* (%s).\n@%s confirma con: *.trade @%s %s %s*",
		ahora.Name, miID, suya.Name, suID, userKey(c, m[0]), c.SenderKey, suID, miID))
}

func cmdGiveChar(c *bot.Context) error {
	if ok, err := gachaAllowed(c); !ok {
		return err
	}
	m := c.Mentioned()
	if len(m) == 0 || len(c.Args) < 1 {
		return c.Reply("*.givechar @user <id>*")
	}
	ok, err := c.DB.TransferChar(c.Ctx, c.Chat.String(), c.Args[0], c.SenderKey, userKey(c, m[0]))
	if err != nil {
		return err
	}
	if !ok {
		return c.Reply("ese id no es tuyo.")
	}
	return c.Reply("🎁 Regalado.")
}

func cmdRobWaifu(c *bot.Context) error {
	if ok, err := gachaAllowed(c); !ok {
		return err
	}
	m := c.Mentioned()
	if len(m) == 0 {
		return c.Reply("¿A quién intento robarle? *.robwaifu @user* (50%, un intento cada 10 min)")
	}
	if userKey(c, m[0]) == c.SenderKey {
		return c.Reply("no te robes a ti mismo, que la economía no perdona.")
	}
	ok, espera, err := c.DB.ClaimCooldown(c.Ctx, c.Chat.String(), c.SenderKey, "steal", 10*time.Minute)
	if err != nil {
		return err
	}
	if !ok {
		return c.Reply(fmt.Sprintf("Te quedan %v de enfriamiento desde el último intento.", espera.Round(time.Second)))
	}
	if rand.Intn(2) == 0 {
		return c.Reply("👮 Te pillaron antes de entrar. (El 50% de las veces sale mal.)")
	}
	o, err := c.DB.StealChar(c.Ctx, c.Chat.String(), userKey(c, m[0]), c.SenderKey)
	if err != nil {
		return err
	}
	if o == nil {
		return c.Reply("No tenía nada robable (los ⭐ y los a la venta se salvan).")
	}
	return c.Reply(fmt.Sprintf("🥷 Robaste a *%s* (valor %d).", o.Name, o.Value))
}

func cmdDelWaifu(c *bot.Context) error {
	if len(c.Args) < 1 {
		return c.Reply("*.delwaifu <id>*  (vuelve al pool, no hay vuelta atrás)")
	}
	ok, err := c.DB.DeleteOwned(c.Ctx, c.Chat.String(), c.SenderKey, c.Args[0])
	if err != nil {
		return err
	}
	if !ok {
		return c.Reply("no tienes ese id.")
	}
	return c.Reply("🗑 Suelta. Ya puede salir en un nuevo roll.")
}

func cmdDelFav(c *bot.Context) error {
	if err := c.DB.SetFavorite(c.Ctx, c.Chat.String(), c.SenderKey, ""); err != nil {
		return err
	}
	return c.Reply("☆ Sin favorito.")
}

// ── tableros y fichas ───────────────────────────────────────────────────────

func cmdTableroGacha(c *bot.Context, favs bool) error {
	if ok, err := gachaAllowed(c); !ok {
		return err
	}
	var filas []store.CharBoardRow
	var err error
	if favs {
		filas, err = c.DB.FavBoard(c.Ctx, c.Chat.String(), 10)
	} else {
		filas, err = c.DB.CharBoard(c.Ctx, c.Chat.String(), 10, false)
	}
	if err != nil {
		return err
	}
	if len(filas) == 0 {
		return c.Reply("todavía no hay datos en este chat.")
	}
	titulo := "🎌 *Más reclamadas*"
	if favs {
		titulo = "⭐ *Favoritas*"
	}
	var sb strings.Builder
	sb.WriteString(titulo + "\n\n")
	for i, f := range filas {
		fmt.Fprintf(&sb, "%d. *%s* — ×%d · valor %d\n", i+1, f.Name, f.Count, f.Value)
	}
	return c.Reply(strings.TrimRight(sb.String(), "\n"))
}

func cmdCharInfo(c *bot.Context) error {
	if len(c.Args) < 1 {
		return c.Reply("*.charinfo <id>*")
	}
	cat := catalogo(c)
	ch, ok := cat.ByID(c.Args[0])
	if !ok {
		return c.Reply("No conozco ese id; mira *.harem* o *.serielist*.")
	}
	dueno, _ := c.DB.OwnerOf(c.Ctx, c.Chat.String(), ch.ID)
	duenoTxt := "libre en el pool"
	if dueno != nil {
		duenoTxt = "de @" + dueno.OwnerID
	}
	enVenta, _ := c.DB.SaleByID(c.Ctx, c.Chat.String(), ch.ID)
	venta := "no"
	if enVenta != nil {
		venta = fmt.Sprintf("sí, por %d 💰", enVenta.Price)
	}
	return c.Reply(fmt.Sprintf("📇 *%s* (%s)\n🏷 %s · valor %d\n👤 %s\n🛒 ¿a la venta? %s",
		ch.Name, ch.ID, ch.Series, ch.Value, duenoTxt, venta))
}

func cmdSerieList(c *bot.Context) error {
	cat := catalogo(c)
	if cat.Empty() {
		return c.Reply("No hay catálogo cargado (falta el archivo gacha).")
	}
	var sb strings.Builder
	sb.WriteString("📚 *Series del catálogo*\n\n")
	visto := map[string]bool{}
	for _, ch := range cat.Characters {
		if !visto[ch.Series] {
			visto[ch.Series] = true
			sb.WriteString("· " + ch.Series + "\n")
		}
	}
	return c.Reply(strings.TrimRight(sb.String(), "\n"))
}

func cmdSerieInfo(c *bot.Context) error {
	q := strings.TrimSpace(strings.Join(c.Args, " "))
	if q == "" {
		return c.Reply("*.serieinfo <serie>*")
	}
	cat := catalogo(c)
	var sb strings.Builder
	n := 0
	for _, ch := range cat.Characters {
		if strings.Contains(strings.ToLower(ch.Series), strings.ToLower(q)) {
			fmt.Fprintf(&sb, "· *%s* (%s) — %d\n", ch.Name, ch.ID, ch.Value)
			if n++; n >= 25 {
				sb.WriteString("…\n")
				break
			}
		}
	}
	if n == 0 {
		return c.Reply("No tengo esa serie en el catálogo.")
	}
	return c.Reply("📺 *" + q + "*\n\n" + strings.TrimRight(sb.String(), "\n"))
}

func cmdCharImage(c *bot.Context) error {
	q := strings.TrimSpace(strings.Join(c.Args, " "))
	if q == "" {
		return c.Reply("*.charimage <nombre o tag>*")
	}
	img, err := gacha.BuscarImagen(c.Ctx, q, nil)
	if err != nil {
		return c.Reply("No encontré imagen de eso ahora (" + err.Error() + ").")
	}
	return c.Bot.SendImage(c.Ctx, c.Chat, img, "🖼 "+q, c.Priority())
}

// ── mensaje de reclamo y voto ───────────────────────────────────────────────

func cmdSetClaimMsg(c *bot.Context) error {
	if c.IsGroup && !c.Admin() {
		return c.Reply("Eso lo decide un administrador del grupo.")
	}
	texto := strings.TrimSpace(c.Text)
	if texto == "" {
		return c.Reply("*.setclaimmsg <texto>* — lo que digo cuando alguien reclama en este chat.")
	}
	if err := c.DB.SetClaimMessage(c.Ctx, c.Chat.String(), texto); err != nil {
		return err
	}
	return c.Reply("✔ Mensaje de reclamo guardado.")
}

func cmdDelClaimMsg(c *bot.Context) error {
	if c.IsGroup && !c.Admin() {
		return c.Reply("Eso lo decide un administrador del grupo.")
	}
	if err := c.DB.SetClaimMessage(c.Ctx, c.Chat.String(), ""); err != nil {
		return err
	}
	return c.Reply("✔ Vuelvo al mensaje de siempre.")
}

func cmdVote(c *bot.Context) error {
	if ok, err := gachaAllowed(c); !ok {
		return err
	}
	if len(c.Args) < 1 {
		return c.Reply("*.vote <id>* — suma valor a uno de tus personajes (una vez al día).")
	}
	ok, espera, err := c.DB.ClaimCooldown(c.Ctx, c.Chat.String(), c.SenderKey, "vote", 24*time.Hour)
	if err != nil {
		return err
	}
	if !ok {
		return c.Reply(fmt.Sprintf("Ya votaste hoy; te quedan %v.", espera.Round(time.Second)))
	}
	suma := int64(50 + rand.Intn(300))
	aplicado, err := c.DB.AddValue(c.Ctx, c.Chat.String(), c.SenderKey, c.Args[0], suma)
	if err != nil {
		return err
	}
	if !aplicado {
		return c.Reply("ese id no es tuyo.")
	}
	return c.Reply(fmt.Sprintf("🗳 +%d de valor para %s.", suma, c.Args[0]))
}
