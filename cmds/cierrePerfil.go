// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// cierrePerfil.go — el perfil del JS que quedaba (set*/del* sobre 4 campos),
// la diversión offline (diversion.js) y tres cabos sueltos de main/economía.
// Nada toca red: todo es store o texto local, así que cada comando aquí es
// barato y testeable.
package cmds

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"shin-go/internal/bot"
	"shin-go/internal/store"
)

func init() {
	// ── perfil set*/del* ──
	for _, p := range []struct{ nombre, alias, campo, nombreCampo string }{
		{"setdescription", "setdesc", store.CampoDescripcion, "descripción"},
		{"setgenre", "setdesc2", store.CampoGenero, "género"},
		{"sethobby", "setpasatiempo", store.CampoPasatiempo, "pasatiempo"},
		{"setbirth", "", store.CampoCumple, "cumpleaños"},
	} {
		nombre, alias, campo, nCampo := p.nombre, p.alias, p.campo, p.nombreCampo
		var aliases []string
		if alias != "" {
			aliases = []string{alias}
		}
		bot.Register(bot.Command{
			Name: nombre, Aliases: aliases, Category: bot.CatPerfil,
			Desc: "Pon tu " + nCampo + " de perfil. Vacío = borra (igual que del*).",
			Run: func(c *bot.Context) error {
				v := strings.TrimSpace(c.Text)
				if v == "" {
					if err := c.DB.LimpiarPerfil(c.Ctx, c.SenderKey, campo); err != nil {
						return err
					}
					return c.Reply("✔ Quité tu " + nCampo + ".")
				}
				if campo == store.CampoCumple {
					if _, err := time.Parse("02/01/2006", v); err != nil {
						return c.Reply("Formato: *DD/MM/AAAA* (ej: 25/12/1998).")
					}
				}
				if _, err := c.DB.SetPerfil(c.Ctx, c.SenderKey, campo, v); err != nil {
					return err
				}
				return c.Reply("✔ Guardé tu " + nCampo + ": " + v)
			},
		})
	}
	for _, p := range []struct{ nombre, alias, campo, nombreCampo string }{
		{"deldescription", "deldesc", store.CampoDescripcion, "descripción"},
		{"delgenre", "", store.CampoGenero, "género"},
		{"delpasatiempo", "removehobby", store.CampoPasatiempo, "pasatiempo"},
		{"delbirth", "", store.CampoCumple, "cumpleaños"},
	} {
		nombre, alias, campo, nCampo := p.nombre, p.alias, p.campo, p.nombreCampo
		var aliases []string
		if alias != "" {
			aliases = []string{alias}
		}
		bot.Register(bot.Command{
			Name: nombre, Aliases: aliases, Category: bot.CatPerfil,
			Desc: "Borra tu " + nCampo + " de perfil.",
			Run: func(c *bot.Context) error {
				if err := c.DB.LimpiarPerfil(c.Ctx, c.SenderKey, campo); err != nil {
					return err
				}
				return c.Reply("✔ Borré tu " + nCampo + ".")
			},
		})
	}

	// ── diversión (diversion.js, offline) ──
	bot.Register(bot.Command{
		Name: "chiste", Aliases: []string{"dato", "datointeresante", "consejo", "piropo"},
		Category: bot.CatGames,
		Desc:     "Chiste / dato curioso / consejo / piropo (offline).",
		Run:      cmdDiversion,
	})
	bot.Register(bot.Command{
		Name: "8ball", Aliases: []string{"8bola"}, Category: bot.CatGames,
		Desc: "La bola 8: hazle una pregunta de sí/no.",
		Run:  cmdBola8,
	})
	bot.Register(bot.Command{
		Name: "ship", Aliases: []string{"pareja"}, Category: bot.CatGames,
		Desc: "Calculadora de amor: .ship @alguien.",
		Run:  cmdShip,
	})
	bot.Register(bot.Command{
		Name: "dado", Category: bot.CatGames,
		Desc: "Lanza un dado: .dado [caras].",
		Run:  cmdDado,
	})
	bot.Register(bot.Command{
		Name: "moneda", Aliases: []string{"coin"}, Category: bot.CatGames,
		Desc: "Cara o cruz.",
		Run:  cmdMoneda,
	})

	// ── main (invite, miid, suggest) ──
	bot.Register(bot.Command{
		Name: "invite", Aliases: []string{"invitar"}, Category: bot.CatMain,
		Desc: "Enlace para añadir el bot a un grupo.",
		Run: func(c *bot.Context) error {
			jid := "tu número registrado en WhatsApp"
			if c.Bot.WA != nil && c.Bot.WA.Store != nil && c.Bot.WA.Store.ID != nil {
				jid = c.Bot.WA.Store.ID.User
			}
			return c.Reply("📲 Para meterme en un grupo: añade *@" + jid +
				"* como contacto y luego agrégalo al grupo. Si el grupo tiene aprobación, que un admin lo acepte.")
		},
	})
	bot.Register(bot.Command{
		Name: "miid", Aliases: []string{"mylid"}, Category: bot.CatMain,
		Desc: "Tu identidad ante el bot (teléfono y LID si lo tienes).",
		Run: func(c *bot.Context) error {
			alt := c.SenderAlt
			otro := "—"
			if !alt.IsEmpty() {
				otro = alt.String()
			}
			return c.Reply(fmt.Sprintf("🪪 *Tu identidad*\n· JID: %s\n· Llave en la base: %s\n· Alternativo (LID): %s",
				c.Sender.String(), c.SenderKey, otro))
		},
	})
	bot.Register(bot.Command{
		Name: "suggest", Aliases: []string{"sug"}, Category: bot.CatMain,
		Desc: "Deja una sugerencia para el dueño del bot.",
		Run: func(c *bot.Context) error {
			v := strings.TrimSpace(c.Text)
			if v == "" {
				return c.Reply("¿Cuál es la sugerencia? Escríbela tras el comando.")
			}
			if err := c.DB.SetSetting(c.Ctx, store.SystemJID,
				fmt.Sprintf("sug_%d", time.Now().Unix()),
				fmt.Sprintf("%s (%s): %s", c.SenderKey, c.Sender.String(), cerca(v, 300))); err != nil {
				return err
			}
			return c.Reply("📬 Guardada. El dueño la verá al revisar las sugerencias.")
		},
	})

	// ── economía: einfo / math / slut / coinsgive ──
	bot.Register(bot.Command{
		Name: "einfo", Aliases: []string{"cooldowns", "economyinfo", "infoeconomy"}, Category: bot.CatEconomy,
		Desc: "Los enfriamientos y reglas de la economía en este chat.",
		Run:  cmdEInfo,
	})
	bot.Register(bot.Command{
		Name: "math", Aliases: []string{"mates"}, Category: bot.CatEconomy,
		Desc: "Cuenta sencilla sobre tu monedero: .math 1100*2.",
		Run:  cmdMath,
	})
	bot.Register(bot.Command{
		Name: "coinsgive", Category: bot.CatEconomy,
		Desc: "Regala monedas a alguien: .coinsgive @user 100.",
		Run:  cmdCoinsGive,
	})
}

// ─── diversión offline ─────────────────────────────────────────────────────

var chistes = []string{
	"¿Qué le dice un jaguar a otro? ¡Jaguar you! 🐆",
	"¿Por qué el libro de mates está triste? Porque tiene demasiados problemas. 📚",
	"¿Qué hace una abeja en el gimnasio? ¡Zum-ba! 🐝",
	"—Me quiero comprar un专车. —No digas cosas por dictado.",
	"¿Qué le dice un techo al otro? Te echo de menos. 🏠",
	"¿Por qué el ordenador fue al doctor? Porque tenía un virus. 💻",
}
var datos = []string{
	"Los pulpos tienen tres corazones. 🐙",
	"La miel no caduca: se han encontrado tarros comestibles de hace 3000 años. 🍯",
	"Los flamencos no nacen rosados: lo hacen por la dieta. 🦩",
	"Un rayo es 5 veces más caliente que la superficie del sol. ⚡",
	"Los nutrias se dan la mano para no separarse mientras duermen. 🦦",
}
var consejos = []string{
	"Si empiezas por lo difícil, el resto del día se siente llanito.",
	"Guarda 10 minutos de descanso por cada hora de pantalla.",
	"Antes de responder enfadado, espera 10 segundos.",
	"Lo que no revisas antes de publicar, te revisa después.",
	"El silencio de las notificaciones también es un ajuste.",
}
var piropos = []string{
	"Si la belleza pagara impuestos, tu familia sería millonaria.",
	"No eres café, pero me despiertas igual.",
	"Contigo ni el modo oscuro cansa.",
	"Eres como el WiFi de la esquina: no se te ve y sin embargo no se siente lo mismo sin ti.",
	"Si fueras error del compilador, sería el único que celebraría.",
}
var respuestas8 = []string{
	"Sí, definitivamente", "Sin duda alguna", "Pregunta de nuevo cuando esté más claro",
	"No cuentes con ello", "Mejor no te lo digo ahora", "Sí", "No", "Quizás",
	"Muy probablemente", "Mis fuentes dicen que no", "Los astros se alinean", "Concentrado: no",
}

func pick[T any](xs []T) T { return xs[rand.Intn(len(xs))] }

func cmdDiversion(c *bot.Context) error {
	switch c.Command {
	case "chiste":
		return c.Reply("😂 " + pick(chistes))
	case "dato", "datointeresante":
		return c.Reply("🤓 " + pick(datos))
	case "consejo":
		return c.Reply("💡 " + pick(consejos))
	case "piropo":
		return c.Reply("💘 " + pick(piropos))
	}
	return nil
}

func cmdBola8(c *bot.Context) error {
	if strings.TrimSpace(c.Text) == "" {
		return c.Reply("🎱 Hazme una pregunta de sí/no: *.8ball ¿saldrá el sol mañana?*")
	}
	return c.Reply("🎱 *Bola 8*\n\n» " + pick(respuestas8))
}

func cmdShip(c *bot.Context) error {
	m := c.Mentioned()
	if len(m) == 0 {
		return c.Reply("Etiqueta a alguien: *.ship @amigo*")
	}
	porcentaje := rand.Intn(101)
	comentario := "😬 Mejor quedan como amigos…"
	switch {
	case porcentaje >= 85:
		comentario = "💞 ¡Almas gemelas! Boda a la vista."
	case porcentaje >= 60:
		comentario = "😍 Hay mucha química entre ustedes."
	case porcentaje >= 35:
		comentario = "😊 Hay potencial, inténtenlo."
	}
	return c.Reply(fmt.Sprintf("💕 *Test de amor*\n\n@%s 💘 @%s\nCompatibilidad: *%d%%*\n%s",
		c.Sender.User, m[0].User, porcentaje, comentario))
}

func cmdDado(c *bot.Context) error {
	caras := 6
	if len(c.Args) > 0 {
		if n, err := strconv.Atoi(c.Args[0]); err == nil && n >= 2 && n <= 1000 {
			caras = n
		}
	}
	return c.Reply(fmt.Sprintf("🎲 @%s lanza un d%d: *%d*", c.Sender.User, caras, 1+rand.Intn(caras)))
}

func cmdMoneda(c *bot.Context) error {
	cara := "🌞 Cara"
	if rand.Intn(2) == 0 {
		cara = "🌙 Cruz"
	}
	return c.Reply("🪙 " + cara)
}

// ─── economía extra ─────────────────────────────────────────────────────────

// cmdEInfo: las reglas visibles (enfriamientos y puerta del chat).
func cmdEInfo(c *bot.Context) error {
	on, err := c.DB.EconomyOn(c.Ctx, chatID(c))
	if err != nil {
		return err
	}
	estado := "on"
	if !on {
		estado = "off (un admin la enciende con .economy on)"
	}
	return c.Reply("💰 *Economía de este chat*\n\n" +
		"Estado: " + estado + "\n" +
		"· daily/weekly/monthly: 1 vez cada uno\n" +
		"· work: cada 10 min\n· rob: 50% de éxito, cada 30 min\n" +
		"· robwaifu/vote gacha: 10 min / 24 h\n" +
		"El saldo es por chat: en privado es tuyo solo.")
}

// cmdMath: la cuenta del JS era sobre el saldo; aquí sobre cualquier texto
// "a+b", "a*b" con dos operandos (lo que el JS usaba de verdad: cuánto
// habrá apostando X con multiplicador Y).
func cmdMath(c *bot.Context) error {
	v := strings.TrimSpace(strings.Join(c.Args, ""))
	if v == "" {
		return c.Reply("*.math 30*1.5* → cuánto sale de esa apuesta.")
	}
	i := strings.IndexAny(v, "+-*/")
	if i <= 0 || i >= len(v)-1 {
		return c.Reply("Formato: *numero op numero* (ej: 30*1.5).")
	}
	a, err1 := strconv.ParseFloat(v[:i], 64)
	b, err2 := strconv.ParseFloat(v[i+1:], 64)
	if err1 != nil || err2 != nil {
		return c.Reply("No entendí los números (ej: 30*1.5).")
	}
	var r float64
	switch v[i] {
	case '+':
		r = a + b
	case '-':
		r = a - b
	case '*':
		r = a * b
	case '/':
		if b == 0 {
			return c.Reply("No divido entre cero: el infinito no cabe en el chat.")
		}
		r = a / b
	}
	return c.Reply(fmt.Sprintf("🧮 %s = *%s*", v, strconv.FormatFloat(r, 'f', 2, 64)))
}

// cmdCoinsGive: da monedas de TU wallet a otra persona (el JS lo hacía con
// addCoins limpio; aquí lo hacemos honesto: la transferencia las descuenta).
func cmdCoinsGive(c *bot.Context) error {
	if ok, err := ecoAllowed(c); !ok {
		return err
	}
	m := c.Mentioned()
	if len(m) == 0 || len(c.Args) < 1 {
		return c.Reply("*.coinsgive @user 100* — le pasas 100 de tu bolsillo.")
	}
	cant, err := strconv.ParseInt(strings.TrimPrefix(c.Args[0], "@"), 10, 64)
	if err != nil || cant <= 0 {
		return c.Reply("La cantidad tiene que ser un número entero positivo (ej: 100).")
	}
	chat := chatID(c)
	if _, err := c.DB.Charge(c.Ctx, chat, c.SenderKey, cant); err != nil {
		return c.Reply("No tienes tanto.")
	}
	destino := userKey(c, m[0])
	if _, err := c.DB.AddCoins(c.Ctx, chat, destino, cant); err != nil {
		return err
	}
	return c.Reply(fmt.Sprintf("🎁 Le regalaste *%d* a @%s.", cant, m[0].User))
}
