// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// gacha.go — Puerto de cmds/gacha/*.js (roll, claim, harem, favorito, ranking).
//
// Diferencias con el JS, todas por el mismo motivo (que no se pueda hacer
// trampa ni romper):
//
//   - **Reclamar es responder al mensaje del personaje.** En el JS, `.claim`
//     buscaba el id del mensaje citado en un JSON del chat que crecía sin
//     límite; aquí el roll se guarda con ese id en su tabla, con caducidad, y
//     el reclamo es un UPDATE condicional: dos personas que respondan a la vez
//     no pueden ganar las dos.
//   - **Un solo dueño por personaje y chat** (clave de la tabla). En el JS la
//     pertenencia se apuntaba en tres sitios y podía quedar descuadrada.
//   - **Sin imagen el rol no falla**: se anuncia igual y se puede reclamar.
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
	"shin-go/internal/wa"
)

// Tiempos del gacha, iguales que el JS.
const (
	rollCooldown  = 15 * time.Minute // lastRoll
	claimCooldown = 30 * time.Minute // lastClaim
)

func init() {
	registerRoll()
	registerClaim()
	registerHarem()
	registerSetFav()
	registerGachaBoard()
	registerGachaToggle()
	registerGachaInfo()
}

// gachaAllowed: puerta del gacha (el `gachaGate` del JS). Si está cerrada,
// contesta el aviso y devuelve false.
func gachaAllowed(c *bot.Context) (bool, error) {
	if !c.IsGroup {
		return true, nil
	}
	on, err := c.DB.GachaOn(c.Ctx, c.Chat.String())
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
		"ꕥ Los comandos de *Gacha* están desactivados en este grupo.\n\n"+
			"Un *administrador* puede activarlos con:\n» *%sgacha on*", prefix))
}

// catalogo carga el catálogo. Si falta el archivo avisa una sola vez y sigue
// con un catálogo vacío (el comando contesta "no hay personajes").
func catalogo(c *bot.Context) *gacha.Catalog {
	cat, aviso := gacha.Load(c.Bot.Config().GachaFile)
	if aviso != "" {
		if gacha.WarnOnce() {
			c.Log.Warn("gacha: " + aviso)
		}
		return &gacha.Catalog{}
	}
	return cat
}

// ─── .roll ──────────────────────────────────────────────────────────────────

func registerRoll() {
	bot.Register(bot.Command{
		Name:     "rollwaifu",
		Aliases:  []string{"roll", "rw", "rolar"},
		Category: bot.CatGacha,
		Desc:     "Sacar un personaje al azar",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			if ok, err := gachaAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()

			ok, wait, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_roll", rollCooldown)
			if err != nil {
				return err
			}
			if !ok {
				return c.Reply(fmt.Sprintf("ꕥ Debes esperar *%s* para usar *%sroll* de nuevo.",
					formatWait(int64(wait.Seconds())), c.Bot.FirstPrefix()))
			}

			cat := catalogo(c)
			if cat.Empty() {
				return c.Reply("《✧》 El catálogo de personajes está vacío o no está configurado.\n" +
					"> El dueño tiene que poner el archivo y la ruta en *CHARACTERS_FILE*.")
			}

			rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
			personaje, _ := cat.Random(rnd)

			// Imagen: primero la del catálogo (si la trae) y si no, el tablero.
			var imagen []byte
			if personaje.Image != "" {
				if img, err := gacha.BuscarImagenURL(c.Ctx, personaje.Image); err == nil {
					imagen = img
				}
			}
			if imagen == nil {
				tag := personaje.Name
				if len(personaje.Tags) > 0 {
					tag = personaje.Tags[0]
				}
				if img, err := gacha.BuscarImagen(c.Ctx, tag, rnd.Intn); err == nil {
					imagen = img
				}
			}

			caption := fmt.Sprintf("❀ Nombre » *%s*\n⚥ Género » *%s*\n✰ Valor » *%s*\n"+
				"♡ Estado » *Libre*\n❖ Fuente » *%s*\n\n"+
				"> Responde a este mensaje con *%sclaim* para reclamarlo.\n> Tienes 60 segundos.",
				personaje.Name, personaje.Gender, miles(personaje.Value),
				orDefault(personaje.Series, "Desconocida"), c.Bot.FirstPrefix())

			// Se manda y se guarda con el id del mensaje: así el reclamo no
			// depende de adivinar nada del texto.
			var msgID string
			if imagen != nil {
				msgID, err = c.Bot.SendImageID(c.Ctx, c.Chat, imagen, caption, c.CmdPriority)
			} else {
				msgID, err = c.Bot.SendTextID(c.Ctx, c.Chat, caption+"\n\n_(sin ilustración disponible)_", c.CmdPriority)
			}
			if err != nil {
				return err
			}
			if msgID != "" {
				if err := c.DB.SaveRoll(c.Ctx, store.Roll{
					ChatID:    chat,
					MessageID: msgID,
					CharID:    personaje.ID,
					Name:      personaje.Name,
					Value:     personaje.Value,
					RollerID:  key,
				}); err != nil {
					return err
				}
			}
			return nil
		},
	})
}

// ─── .claim ─────────────────────────────────────────────────────────────────

func registerClaim() {
	bot.Register(bot.Command{
		Name:     "claim",
		Aliases:  []string{"c", "reclamar"},
		Category: bot.CatGacha,
		Desc:     "Reclamar el personaje del mensaje al que respondes",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			if ok, err := gachaAllowed(c); err != nil || !ok {
				return err
			}
			chat, key := chatID(c), c.Key()

			// El id del mensaje citado: es la llave del roll.
			ci := wa.ContextInfo(c.Event.Message)
			if ci == nil || ci.GetStanzaID() == "" {
				return c.Reply(fmt.Sprintf(
					"❀ Debes *responder* al mensaje del personaje para reclamarlo.\n"+
						"> Saca uno con *%sroll*", c.Bot.FirstPrefix()))
			}

			ok, wait, err := c.DB.ClaimCooldown(c.Ctx, chat, key, "last_claim", claimCooldown)
			if err != nil {
				return err
			}
			if !ok {
				return c.Reply(fmt.Sprintf("ꕥ Debes esperar *%s* para reclamar otro personaje.",
					formatWait(int64(wait.Seconds()))))
			}

			owned, err := c.DB.ClaimRoll(c.Ctx, chat, ci.GetStanzaID(), key, time.Now())
			switch err {
			case nil:
			case store.ErrAlreadyClaimed:
				return c.Reply("ꕥ Ese personaje ya lo reclamó alguien.")
			case store.ErrRollExpired:
				return c.Reply("ꕥ Ese personaje ya caducó (son 60 segundos).")
			case store.ErrReserved:
				return c.Reply("ꕥ Ese personaje es de quien lo sacó: espera a que pasen sus 20 segundos.")
			case store.ErrRollNotFound:
				return c.Reply("❀ Ese mensaje no es de un personaje que se pueda reclamar.")
			default:
				return err
			}

			total, err := c.DB.OwnedCount(c.Ctx, chat, key)
			if err != nil {
				return err
			}
			return c.Reply(fmt.Sprintf("「✿」 ¡Reclamo exitoso!\n❀ Personaje » *%s*\n"+
				"✰ Valor » *%s*\n♡ Ahora tienes » *%d* personajes",
				owned.Name, money(c, owned.Value), total))
		},
	})
}

// ─── .harem ─────────────────────────────────────────────────────────────────

func registerHarem() {
	bot.Register(bot.Command{
		Name:     "harem",
		Aliases:  []string{"waifus", "claims", "harenes"},
		Category: bot.CatGacha,
		Desc:     "Ver tus personajes (o los de otra persona)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := gachaAllowed(c); err != nil || !ok {
				return err
			}
			key := c.Key()
			nombre := c.PushName
			if j, ok := target(c); ok {
				key = userKey(c, j)
				nombre = strings.SplitN(key, "@", 2)[0]
			}
			if nombre == "" {
				nombre = strings.SplitN(key, "@", 2)[0]
			}

			personajes, err := c.DB.Harem(c.Ctx, chatID(c), key)
			if err != nil {
				return err
			}
			if len(personajes) == 0 {
				if key == c.Key() {
					return c.Reply("ꕥ No tienes personajes reclamados.")
				}
				return c.Reply(fmt.Sprintf("ꕥ *%s* no tiene personajes reclamados.", nombre))
			}

			const porPagina = 20
			pagina := 1
			for _, a := range c.Args {
				if n, err := strconv.Atoi(a); err == nil && n > 0 {
					pagina = n
					break
				}
			}
			totalPaginas := (len(personajes) + porPagina - 1) / porPagina
			if pagina > totalPaginas {
				return c.Reply(fmt.Sprintf("❀ Página no válida. Hay *%d* páginas.", totalPaginas))
			}
			ini := (pagina - 1) * porPagina
			fin := ini + porPagina
			if fin > len(personajes) {
				fin = len(personajes)
			}

			var b strings.Builder
			b.WriteString("✿ Personajes reclamados ✿\n")
			fmt.Fprintf(&b, "⌦ Usuario: *%s*\n♡ Personajes: *(%d)*\n\n", nombre, len(personajes))
			var total int64
			for _, p := range personajes {
				total += p.Value
			}
			for i := ini; i < fin; i++ {
				p := personajes[i]
				marca := ""
				if p.Favorite {
					marca = " ⭐"
				}
				fmt.Fprintf(&b, "» *%s* (*%s*)%s\n", p.Name, miles(p.Value), marca)
			}
			fmt.Fprintf(&b, "\n⌦ _Página *%d* de *%d*_\n⌦ Valor total » *%s*",
				pagina, totalPaginas, money(c, total))
			if pagina < totalPaginas {
				fmt.Fprintf(&b, "\n> Siguiente página » *%sharem %d*", c.Bot.FirstPrefix(), pagina+1)
			}
			return c.Reply(b.String())
		},
	})
}

// ─── .setfav ────────────────────────────────────────────────────────────────

func registerSetFav() {
	bot.Register(bot.Command{
		Name:     "setfav",
		Aliases:  []string{"favorito", "setfavorito"},
		Category: bot.CatGacha,
		Desc:     "Marcar tu personaje favorito (no te lo pueden robar)",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			if ok, err := gachaAllowed(c); err != nil || !ok {
				return err
			}
			if len(c.Args) == 0 {
				fav, err := c.DB.Favorite(c.Ctx, chatID(c), c.Key())
				if err != nil {
					return err
				}
				if fav == "" {
					return c.Reply(fmt.Sprintf("ꕥ No tienes favorito.\n> Usa *%ssetfav <nombre>*",
						c.Bot.FirstPrefix()))
				}
				return c.Reply(fmt.Sprintf("⭐ Tu favorito es *%s*", fav))
			}

			pedido := strings.ToLower(strings.Join(c.Args, " "))
			personajes, err := c.DB.Harem(c.Ctx, chatID(c), c.Key())
			if err != nil {
				return err
			}
			for _, p := range personajes {
				if strings.ToLower(p.Name) == pedido || strings.ToLower(p.CharID) == pedido {
					if err := c.DB.SetFavorite(c.Ctx, chatID(c), c.Key(), p.CharID); err != nil {
						return err
					}
					return c.Reply(fmt.Sprintf("⭐ *%s* es ahora tu favorito.", p.Name))
				}
			}
			return c.Reply("《✧》 No tienes ese personaje.")
		},
	})
}

// ─── .waifusboard ───────────────────────────────────────────────────────────

func registerGachaBoard() {
	bot.Register(bot.Command{
		Name:     "waifusboard",
		Aliases:  []string{"haremsboard", "topwaifus", "gachaboard"},
		Category: bot.CatGacha,
		Desc:     "Ranking de personajes del grupo",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			if ok, err := gachaAllowed(c); err != nil || !ok {
				return err
			}
			filas, err := c.DB.GachaBoard(c.Ctx, chatID(c), 10)
			if err != nil {
				return err
			}
			if len(filas) == 0 {
				return c.Reply("ꕥ Todavía nadie ha reclamado personajes en este grupo.")
			}
			var b strings.Builder
			b.WriteString("✿ Ranking del gacha ✿\n\n")
			for i, f := range filas {
				fmt.Fprintf(&b, "✩ %d › *%s*\n     Personajes → *%d* · Valor → *%s*\n",
					i+1, strings.SplitN(f.OwnerID, "@", 2)[0], f.Owned, miles(f.Value))
			}
			return c.Reply(b.String())
		},
	})
}

// ─── .gacha on|off ──────────────────────────────────────────────────────────

func registerGachaToggle() {
	bot.Register(bot.Command{
		Name:      "gacha",
		Aliases:   []string{"gachapon"},
		Category:  bot.CatGacha,
		Desc:      "Activar o desactivar el gacha en el grupo",
		GroupOnly: true,
		AdminOnly: true,
		Cooldown:  2,
		Run: func(c *bot.Context) error {
			switch strings.ToLower(firstArg(c)) {
			case "":
				on, err := c.DB.GachaOn(c.Ctx, c.Chat.String())
				if err != nil {
					return err
				}
				estado := "activado ✅"
				if !on {
					estado = "desactivado ⛔"
				}
				return c.Reply(fmt.Sprintf("「✿」El gacha está *%s* en este grupo.\n> *%sgacha on* / *%sgacha off*",
					estado, c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			case "on", "si", "sí", "1", "true":
				if err := c.DB.SetGacha(c.Ctx, c.Chat.String(), true); err != nil {
					return err
				}
				return c.Reply("ꕥ Gacha *activado* en este grupo.")
			case "off", "no", "0", "false":
				if err := c.DB.SetGacha(c.Ctx, c.Chat.String(), false); err != nil {
					return err
				}
				return c.Reply("ꕥ Gacha *desactivado* en este grupo.")
			default:
				return c.Reply(fmt.Sprintf("《✧》 Usa *%sgacha on* o *%sgacha off*",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}
		},
	})
}

// ─── .gachainfo ─────────────────────────────────────────────────────────────

func registerGachaInfo() {
	bot.Register(bot.Command{
		Name:     "gachainfo",
		Aliases:  []string{"ginfo", "personajes"},
		Category: bot.CatGacha,
		Desc:     "Cuántos personajes hay en el catálogo",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			cat := catalogo(c)
			if cat.Empty() {
				return c.Reply("《✧》 El catálogo está vacío o no está configurado.\n" +
					"> Ruta actual: *" + orDefault(c.Bot.Config().GachaFile, "(sin configurar)") + "*")
			}
			series := map[string]bool{}
			var valorMax, valorMin int64 = 0, -1
			for _, p := range cat.Characters {
				if p.Series != "" {
					series[p.Series] = true
				}
				if p.Value > valorMax {
					valorMax = p.Value
				}
				if valorMin < 0 || p.Value < valorMin {
					valorMin = p.Value
				}
			}
			if valorMin < 0 {
				valorMin = 0
			}
			return c.Reply(fmt.Sprintf(
				"╭─「 *GACHA* 」\n│ ✿ Personajes: *%d*\n│ ⌦ Series: *%d*\n"+
					"│ ✰ Valor mínimo: *%s*\n│ ✰ Valor máximo: *%s*\n│ ⴵ Rolles libres cada: *15 min*\n"+
					"╰─ ¿Quién saldrá hoy?",
				cat.Len(), len(series), miles(valorMin), miles(valorMax)))
		},
	})
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
