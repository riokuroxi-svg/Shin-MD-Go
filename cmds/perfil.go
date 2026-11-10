// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// perfil.go — Puerto de cmds/profile/*.js: ficha, nivel, tabla de niveles,
// descripción, género, pasatiempo, cumpleaños, matrimonio y modo ausente.
//
// QUÉ CAMBIA RESPECTO AL JS
//
//   - **Una sola curva de XP.** El JS tenía dos (el perfil usaba una y `.level`
//     otra), así que el mismo usuario veía dos niveles según el comando. Aquí
//     todo sale de internal/xp.
//   - **Nada de tarjetas de canvas.** El JS dibujaba el perfil con
//     `cardGenerator`; en Go eso sería media librería gráfica para un adorno.
//     Se manda texto con botones (que sí funcionan sin imagen) y el avatar solo
//     cuando el comando lo necesita.
//   - **Los datos del usuario se escapan.** Un nombre con `*` subrayados podía
//     reescribir el formato del mensaje del bot: todo pasa por bot.Esc.
package cmds

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/bot"
	"shin-go/internal/store"
	"shin-go/internal/xp"
)

func init() {
	registerProfile()
	registerLevel()
	registerLeaderboard()
	registerProfileEdits()
	registerMarry()
}

// ─── utilidades ─────────────────────────────────────────────────────────────

// objetivo devuelve a quién se refiere el comando: la primera mención, o a
// quien se le está respondiendo, o a quien escribe. Es la regla del JS y no se
// cambia: `.profile` sin argumentos es «el mío».
func objetivo(c *bot.Context) string {
	for _, jid := range c.Mentioned() {
		if clave := claveDe(c, jid); clave != "" {
			return clave
		}
	}
	if key := c.Key(); key != "" {
		return key
	}
	return c.Sender.String()
}

// claveDe traduce un JID a la llave de la base (teléfono si se conoce, LID si
// no). Sin esto, mencionar a alguien por LID crearía una cuenta distinta de la
// suya de siempre.
func claveDe(c *bot.Context, jid types.JID) string {
	if jid.IsEmpty() {
		return ""
	}
	if jid.Server == types.DefaultUserServer {
		return jid.ToNonAD().String()
	}
	if pn, err := c.DB.PNForLID(c.Ctx, jid.String()); err == nil && pn != "" {
		return pn
	}
	return jid.ToNonAD().String()
}

// nombrePerfil es el nombre a mostrar: el que la persona puso en el bot o, si
// no hay, el de WhatsApp, recortado y escapado. (No se llama `nombreDe` porque
// ese ya existe en aventuras.go y hace otra cosa: resolver un JID.)
func nombrePerfil(c *bot.Context, p *store.Perfil) string {
	if p != nil && p.Nombre != "" {
		return bot.Esc(p.Nombre)
	}
	if n := bot.CleanName(c.PushName, 32); n != "" {
		return bot.Esc(n)
	}
	return "Usuario"
}

// ─── .profile ───────────────────────────────────────────────────────────────

func registerProfile() {
	bot.Register(bot.Command{
		Name:     "profile",
		Aliases:  []string{"perfil", "ficha", "yo"},
		Category: bot.CatPerfil,
		Desc:     "Tu ficha: nivel, progreso, monedas y pareja",
		Priority: true,
		Run: func(c *bot.Context) error {
			jid := objetivo(c)
			// A uno mismo se le crea la cuenta si es la primera vez; a otra
			// persona no: consultar a quien nunca usó el bot no puede ir
			// sembrando cuentas fantasma en la base.
			propio := jid == c.Key()
			p, err := c.DB.PerfilDe(c.Ctx, jid)
			if !propio {
				var ok bool
				p, ok, err = c.DB.PerfilExistente(c.Ctx, jid)
				if err == nil && !ok {
					return c.Reply("✕ Esa persona todavía no tiene perfil: nunca le ha escrito al bot.")
				}
			}
			if err != nil {
				return c.Reply("✕ No pude leer ese perfil de la base.")
			}
			pos, err := c.DB.Posicion(c.Ctx, jid)
			if err != nil {
				pos = 0
			}
			// Las monedas son POR CHAT, igual que en el JS: el saldo que se ve
			// aquí es el de este grupo, no el de todos.
			cuenta, err := c.DB.ChatUser(c.Ctx, c.Chat.String(), jid)
			if err != nil {
				return c.Reply("✕ No pude leer el saldo de este chat.")
			}
			prog := xp.Calcular(p.XP)
			// Rachas (globales, como en el JS) y estado del daily.
			dias, _ := c.DB.StreakValue(c.Ctx, jid, store.StreakDaily)
			semanas, _ := c.DB.StreakValue(c.Ctx, jid, store.StreakWeekly)
			daily := "disponible ✓"
			if cuenta.LastDaily > time.Now().UnixMilli() {
				daily = "vuelve en " + formatWait((cuenta.LastDaily-time.Now().UnixMilli())/1000)
			}
			nombre := nombrePerfil(c, p)
			if jid != c.Key() {
				nombre = "«" + nombre + "»"
			}

			body := textoPerfil(ficha{
				Nombre:      nombre,
				Nivel:       prog.Level,
				Puesto:      puesto(pos),
				Dentro:      prog.Dentro,
				Necesaria:   prog.Necesaria,
				Barra:       xp.Barra(prog, 10),
				Porcentaje:  prog.Porcentaje,
				Cartera:     cuenta.Coins,
				Banco:       cuenta.Bank,
				Moneda:      moneda(c),
				Cumple:      o(p.Cumple, "Sin especificar"),
				Pasatiempo:  o(p.Pasatiempo, "No definido"),
				Genero:      o(p.Genero, "Sin especificar"),
				Civil:       estadoCivil(p),
				Dias:        dias,
				Semanas:     semanas,
				Daily:       daily,
				Comandos:    p.Comandos,
				Descripcion: p.Descripcion,
				AvisoAFK:    motivoAFK(p),
			})

			return c.ReplyButtons(
				"👤 Perfil",
				body,
				"Perfil · Shin-MD",
				bot.QuickReply("📊 Mi nivel", "level"),
				bot.QuickReply("💰 Mi saldo", "balance"),
				bot.QuickReply("✏️ Editar descripción", "setdesc"),
			)
		},
	})
}

// ficha junta lo que se pinta en .profile. Va aparte del handler para poder
// probar el formato sin WhatsApp: es la parte que ve la gente y la que más se
// rompe con un cambio de sprintf.
type ficha struct {
	Nombre     string
	Nivel      int
	Puesto     string
	Dentro     int64
	Necesaria  int64
	Barra      string
	Porcentaje int
	Cartera    int64
	Banco      int64
	Moneda     string
	Cumple     string
	Pasatiempo string
	Genero     string
	Civil      string
	Dias       int64
	Semanas    int64
	Daily      string
	Comandos   int64
	// Texto libre del usuario: se escapa aquí dentro, no en el llamador.
	Descripcion string
	AvisoAFK    string
}

// textoPerfil arma la ficha completa de .profile.
func textoPerfil(f ficha) string {
	body := fmt.Sprintf(
		"╭┈┈⫹⫺ *PERFIL* ⫹⫺┈┈╮\n"+
			"│ ◈ *Nombre* : *%s*\n"+
			"│ ◈ *Nivel* : *%d*%s\n"+
			"│ ◈ *EXP* : *%s* / %s\n"+
			"│ ◈ *Progreso* : %s *(%d %%)*\n"+
			"│ ◈ *Cartera* : *%s* · *Banco* : *%s*\n"+
			"│ ◈ *Total* : *%s %s*\n"+
			"│ ◈ *Cumpleaños* : *%s*\n"+
			"│ ◈ *Pasatiempo* : *%s*\n"+
			"│ ◈ *Género* : *%s*\n"+
			"│ ◈ *Estado civil* : *%s*\n"+
			"│ ◈ *Racha* : *%d días · %d semanas*\n"+
			"│ ◈ *Daily* : %s\n"+
			"│ ◈ *Comandos* : *%s*\n"+
			"╰┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈╯",
		f.Nombre,
		f.Nivel, f.Puesto,
		miles(f.Dentro), miles(f.Necesaria),
		f.Barra, f.Porcentaje,
		miles(f.Cartera), miles(f.Banco),
		miles(f.Cartera+f.Banco), f.Moneda,
		bot.Esc(f.Cumple),
		bot.Esc(f.Pasatiempo),
		bot.Esc(f.Genero),
		f.Civil,
		f.Dias, f.Semanas, f.Daily,
		miles(f.Comandos),
	)
	if f.Descripcion != "" {
		body += "\n> " + bot.Esc(f.Descripcion)
	}
	if f.AvisoAFK != "" {
		body += "\n> 💤 Ausente: *" + bot.Esc(f.AvisoAFK) + "*"
	}
	return body
}

// motivoAFK es el motivo a enseñar en la ficha, o vacío si no está ausente.
func motivoAFK(p *store.Perfil) string {
	if p == nil || !p.AFK {
		return ""
	}
	return o(p.AFKRazon, "sin motivo")
}

// puesto: «(Top #3)» cuando hay puesto, vacío cuando no compite.
func puesto(pos int) string {
	if pos <= 0 {
		return ""
	}
	return fmt.Sprintf(" _(Top #%d)_", pos)
}

// estadoCivil arma la frase con la pareja. En el JS se elegía el participio por
// género («Casada con», «Casado con», «Casadx con»); se conserva.
func estadoCivil(p *store.Perfil) string {
	if p.Pareja == "" {
		return "Soltero/a"
	}
	pareja := p.ParejaNombre
	if pareja == "" {
		pareja = "alguien"
	}
	switch strings.ToLower(p.Genero) {
	case "mujer", "femenino":
		return "Casada con " + bot.Esc(pareja)
	case "hombre", "masculino":
		return "Casado con " + bot.Esc(pareja)
	default:
		return "Casadx con " + bot.Esc(pareja)
	}
}

// o devuelve el valor o el texto por defecto.
func o(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return bot.Esc(v)
}

// ─── .level ─────────────────────────────────────────────────────────────────

func registerLevel() {
	bot.Register(bot.Command{
		Name:     "level",
		Aliases:  []string{"lvl", "nivel", "xp"},
		Category: bot.CatPerfil,
		Desc:     "Tu nivel y lo que te falta para el siguiente",
		Priority: true,
		Run: func(c *bot.Context) error {
			jid := objetivo(c)
			p, err := c.DB.PerfilDe(c.Ctx, jid)
			if err != nil {
				return c.Reply("✕ No pude leer ese perfil de la base.")
			}
			pos, _ := c.DB.Posicion(c.Ctx, jid)
			prog := xp.Calcular(p.XP)
			nombre := nombrePerfil(c, p)
			if jid != c.Key() {
				nombre = "«" + nombre + "»"
			}

			body := fmt.Sprintf(
				"╭┈┈⫹⫺ *NIVEL Y EXPERIENCIA* ⫹⫺┈┈╮\n"+
					"│ ◈ *Usuario* : *%s*\n"+
					"│ ◈ *Nivel* : *%d*%s\n"+
					"│ ◈ *EXP total* : *%s*\n"+
					"│ ◈ *Este nivel* : *%s* / %s\n"+
					"│ ◈ *Progreso* : %s *(%d %%)*\n"+
					"│ ◈ *Comandos* : *%s*\n"+
					"╰┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈╯",
				nombre,
				prog.Level, puesto(pos),
				miles(p.XP),
				miles(prog.Dentro), miles(prog.Necesaria),
				xp.Barra(prog, 10), prog.Porcentaje,
				miles(p.Comandos),
			)

			return c.ReplyButtons(
				"📊 Nivel",
				body,
				"Nivel · Shin-MD",
				bot.QuickReply("🏆 Tabla de niveles", "lboard"),
				bot.QuickReply("👤 Mi perfil", "profile"),
			)
		},
	})
}

// ─── .lboard ────────────────────────────────────────────────────────────────

func registerLeaderboard() {
	bot.Register(bot.Command{
		Name:     "lboard",
		Aliases:  []string{"lb", "leaderboard", "topnivel"},
		Category: bot.CatPerfil,
		Desc:     "Tabla de niveles: quién tiene más experiencia",
		Priority: true,
		Run: func(c *bot.Context) error {
			tabla, err := c.DB.TopPorXP(c.Ctx, 10)
			if err != nil {
				return c.Reply("✕ No pude leer la tabla de niveles.")
			}
			if len(tabla) == 0 {
				return c.Reply("ꕥ Todavía nadie tiene experiencia. Habla un poco y vuelve a mirar.")
			}
			body := textoTabla(tabla, c.Key())
			return c.ReplyButtons(
				"🏆 Tabla de niveles",
				body,
				"Por experiencia · Shin-MD",
				bot.QuickReply("📊 Mi nivel", "level"),
				bot.QuickReply("👤 Mi perfil", "profile"),
			)
		},
	})
}

// textoTabla arma la lista. Se separa de la base para poder probarla sin
// WhatsApp: es donde está el formato que ve la gente.
func textoTabla(tabla []store.FilaRanking, yo string) string {
	var b strings.Builder
	b.WriteString("╭┈┈⫹⫺ *TABLA DE NIVELES* ⫹⫺┈┈╮\n")
	medallas := []string{"🥇", "🥈", "🥉"}
	for i, f := range tabla {
		marca := "│"
		if f.JID == yo {
			marca = "│ ▸"
		}
		puesto := fmt.Sprintf("%d)", i+1)
		if i < len(medallas) {
			puesto = medallas[i]
		}
		nombre := f.Nombre
		if nombre == "" {
			nombre = corto(f.JID)
		}
		b.WriteString(fmt.Sprintf("%s %s *%s* — nivel *%d* · *%s XP*\n",
			marca, puesto, bot.Esc(nombre), f.Nivel, miles(f.XP)))
	}
	b.WriteString("╰┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈╯")
	return b.String()
}

// corto: el número del JID, para no enseñar «52155…@s.whatsapp.net» entero.
// También quita el sufijo de dispositivo («52155…:12@…»), que no le dice nada a
// nadie y aparece cuando el mensaje viene de un LID o de un dispositivo viejo.
func corto(jid string) string {
	if i := strings.IndexByte(jid, '@'); i > 0 {
		jid = jid[:i]
	}
	if i := strings.IndexByte(jid, ':'); i > 0 {
		jid = jid[:i]
	}
	return jid
}

// ─── editar el perfil ───────────────────────────────────────────────────────

func registerProfileEdits() {
	type edit struct {
		name    string
		aliases []string
		campo   string
		label   string
		hint    string
		tope    int
	}
	edits := []edit{
		{"setdesc", []string{"desc", "setdescription", "midesc"}, store.CampoDescripcion, "descripción", "Una línea sobre ti", store.MaxDesc},
		{"setgenre", []string{"setgenero", "genre"}, store.CampoGenero, "género", "Hombre, Mujer u Oculto", store.MaxGenero},
		{"sethobby", []string{"setpasatiempo", "hobby"}, store.CampoPasatiempo, "pasatiempo", "Qué te gusta hacer", store.MaxPasatiempo},
		{"setbirth", []string{"setcumple", "birth", "cumple"}, store.CampoCumple, "cumpleaños", "14 de marzo, por ejemplo", store.MaxCumple},
	}
	for _, e := range edits {
		e := e
		bot.Register(bot.Command{
			Name:     e.name,
			Aliases:  e.aliases,
			Category: bot.CatPerfil,
			Desc:     "Cambiar tu " + e.label,
			Cooldown: 5,
			Run: func(c *bot.Context) error {
				texto := strings.TrimSpace(c.Text)
				if texto == "" {
					return c.Reply(fmt.Sprintf("《✧》 Escribe qué %s quieres poner.\n> Ejemplo: *%s%s %s*",
						e.label, c.Bot.FirstPrefix(), e.name, e.hint))
				}
				if len([]rune(texto)) > e.tope {
					return c.Reply(fmt.Sprintf("✕ Demasiado largo: el máximo son *%d* caracteres.", e.tope))
				}
				guardado, err := c.DB.SetPerfil(c.Ctx, c.Key(), e.campo, texto)
				if err != nil {
					return c.Reply("✕ No pude guardar eso en la base.")
				}
				return c.Reply(fmt.Sprintf("✓ Tu %s quedó así:\n> %s", e.label, bot.Esc(guardado)))
			},
		})
	}

	// Los borrados: mismos nombres que en el JS.
	borrados := []struct {
		name    string
		aliases []string
		campo   string
		label   string
	}{
		{"deldesc", []string{"deldescription"}, store.CampoDescripcion, "descripción"},
		{"delgenre", []string{"delgenero"}, store.CampoGenero, "género"},
		{"delhobby", []string{"delpasatiempo", "removehobby"}, store.CampoPasatiempo, "pasatiempo"},
		{"delbirth", []string{"delcumple"}, store.CampoCumple, "cumpleaños"},
	}
	for _, b := range borrados {
		b := b
		bot.Register(bot.Command{
			Name:     b.name,
			Aliases:  b.aliases,
			Category: bot.CatPerfil,
			Desc:     "Borrar tu " + b.label,
			Cooldown: 5,
			Run: func(c *bot.Context) error {
				if err := c.DB.LimpiarPerfil(c.Ctx, c.Key(), b.campo); err != nil {
					return c.Reply("✕ No pude borrarlo de la base.")
				}
				return c.Reply(fmt.Sprintf("✓ Borré tu %s.", b.label))
			},
		})
	}
}

// ─── .marry / .divorce ──────────────────────────────────────────────────────

func registerMarry() {
	bot.Register(bot.Command{
		Name:     "marry",
		Aliases:  []string{"casarse", "casar", "casarme"},
		Category: bot.CatPerfil,
		Desc:     "Proponer matrimonio a alguien a quien menciones o respondas",
		Cooldown: 10,
		Run: func(c *bot.Context) error {
			yo := c.Key()
			otro := ""
			for _, jid := range c.Mentioned() {
				if clave := claveDe(c, jid); clave != "" && clave != yo {
					otro = clave
					break
				}
			}
			if otro == "" {
				return c.Reply(fmt.Sprintf("《✧》 Menciona a alguien o responde a su mensaje.\n> *%smarry @persona*",
					c.Bot.FirstPrefix()))
			}
			if _, casado := parejaDe(c, yo); casado {
				return c.Reply("✕ Ya estás casado/a. Toca *" + c.Bot.FirstPrefix() + "divorce*.")
			}
			if otro == yo {
				return c.Reply("✕ No te puedes proponer matrimonio a ti.")
			}
			// Aceptar: en el JS se confirma volviendo a escribir .marry a la
			// misma persona. Se mantiene, además de .acepto, para que quien
			// venía del bot viejo no tenga que aprenderse otro comando.
			if de, ok := propuestaDe(yo); ok && de == otro {
				if _, ok := retirarPropuesta(yo); ok {
					if err := c.DB.Casarse(c.Ctx, de, yo); err != nil {
						if errors.Is(err, store.ErrYaCasado) {
							return c.Reply("✕ Una de las dos personas ya está casada. Hace falta un divorcio primero.")
						}
						return c.Reply("✕ No pude celebrar la boda: " + err.Error())
					}
					return c.Reply("💍 ¡Casados! Que dure.")
				}
			}
			if _, ocupado := parejaDe(c, otro); ocupado {
				return c.Reply(fmt.Sprintf("✕ *%s* ya está casado/a.", bot.Esc(corto(otro))))
			}
			// En el JS la boda se cerraba sin preguntar. Aquí la otra persona
			// tiene que aceptar: casar a alguien sin querer es una forma de
			// molestar, y deshacerlo exige un divorcio.
			proponer(otro, yo)
			return c.Reply(fmt.Sprintf(
				"💍 Le propusiste matrimonio a *%s*.\n> Tiene *%sacepto* para decir que sí (la propuesta caduca en 10 min).",
				bot.Esc(corto(otro)), c.Bot.FirstPrefix()))
		},
	})

	bot.Register(bot.Command{
		Name:     "acepto",
		Category: bot.CatPerfil,
		Desc:     "Aceptar una propuesta de matrimonio",
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			otro, ok := retirarPropuesta(c.Key())
			if !ok {
				return c.Reply("✕ No tienes ninguna propuesta pendiente (caducan a los 10 minutos).")
			}
			if err := c.DB.Casarse(c.Ctx, otro, c.Key()); err != nil {
				if errors.Is(err, store.ErrYaCasado) {
					return c.Reply("✕ Una de las dos personas ya está casada. Hace falta un divorcio primero.")
				}
				return c.Reply("✕ No pude celebrar la boda: " + err.Error())
			}
			return c.Reply("💍 ¡Casados! Que dure.")
		},
	})

	bot.Register(bot.Command{
		Name:     "divorce",
		Aliases:  []string{"divorcio", "divorciarme"},
		Category: bot.CatPerfil,
		Desc:     "Deshacer tu matrimonio",
		Cooldown: 10,
		Run: func(c *bot.Context) error {
			ex, err := c.DB.Divorciarse(c.Ctx, c.Key())
			if err != nil {
				if errors.Is(err, store.ErrNoCasado) {
					return c.Reply("✕ No estás casado/a.")
				}
				return c.Reply("✕ No pude tramitar el divorcio.")
			}
			return c.Reply(fmt.Sprintf("💔 Divorcio hecho. Ya no estás casado/a con *%s*.", bot.Esc(corto(ex))))
		},
	})
}

// parejaDe dice si la persona tiene pareja (consulta barata, sin traer la ficha
// entera: .marry se usa mucho y no necesita nombre ni XP).
func parejaDe(c *bot.Context, jid string) (string, bool) {
	p, err := c.DB.PerfilDe(c.Ctx, jid)
	if err != nil || p.Pareja == "" {
		return "", false
	}
	return p.Pareja, true
}

// Propuestas de matrimonio pendientes.
//
// Van en memoria y no en la base a propósito: una propuesta de hace tres días
// ya no significa nada, y si el bot se reinicia es razonable que se pierdan
// (como las partidas pendientes del JS).
//
// El mutex NO es decorativo: los handlers corren en goroutines distintas, y un
// map escribiéndose desde dos a la vez en Go no da un error — da un pánico que
// tumba el proceso entero.
var propuestas = struct {
	sync.Mutex
	m map[string]propuestaMatrimonio
}{m: map[string]propuestaMatrimonio{}}

type propuestaMatrimonio struct {
	de     string // quién propone
	cuando time.Time
}

const caducidadPropuesta = 10 * time.Minute

func proponer(para, de string) {
	propuestas.Lock()
	defer propuestas.Unlock()
	// Se limpia lo caducado al pasar: sin esto el mapa crece sin fin en un
	// grupo donde la gente se propone matrimonio todo el día.
	for k, p := range propuestas.m {
		if time.Since(p.cuando) > caducidadPropuesta {
			delete(propuestas.m, k)
		}
	}
	propuestas.m[para] = propuestaMatrimonio{de: de, cuando: time.Now()}
}

// propuestaDe dice quién le propuso matrimonio, sin gastar la propuesta.
func propuestaDe(para string) (string, bool) {
	propuestas.Lock()
	defer propuestas.Unlock()
	p, ok := propuestas.m[para]
	if !ok || time.Since(p.cuando) > caducidadPropuesta {
		return "", false
	}
	return p.de, true
}

// retirarPropuesta devuelve quién propuso y borra la propuesta (de un solo uso).
func retirarPropuesta(para string) (string, bool) {
	propuestas.Lock()
	defer propuestas.Unlock()
	p, ok := propuestas.m[para]
	if !ok || time.Since(p.cuando) > caducidadPropuesta {
		delete(propuestas.m, para)
		return "", false
	}
	delete(propuestas.m, para)
	return p.de, true
}
