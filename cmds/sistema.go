// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// sistema.go — Puerto de cmds/main (health, runtime, warmup, infobot, compat,
// allmenu).
//
// POR QUÉ ESTO IMPORTA MÁS EN GO QUE EN EL JS
// El bot de Node se caía y lo reiniciabas desde Termux; aquí el binario casi
// nunca se cae, así que la información de estado deja de ser un adorno: es la
// única forma de saber si el proceso está sano sin entrar por SSH. Estos
// comandos se leen desde el móvil, en un grupo contigo mismo.
//
// Los números que se enseñan son los del proceso real: uptime del cliente, RAM
// del sistema y del bot, tope diario de la cola anti-ban y el anillo de
// errores de internal/xlog.
package cmds

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"shin-go/internal/bot"
	"shin-go/internal/xlog"
)

func init() {
	registerHealth()
	registerRuntime()
	registerWarmup()
	registerInfobot()
	registerCompat()
	registerAllMenu()
}

// ─── .health ────────────────────────────────────────────────────────────────

func registerHealth() {
	bot.Register(bot.Command{
		Name:     "health",
		Aliases:  []string{"salud", "statsbot", "sistema"},
		Category: bot.CatMain,
		Desc:     "Salud del bot: RAM, uptime, herramientas y errores recientes",
		Priority: true,
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			sub := strings.ToLower(firstArg(c))

			// ── .health clear ────────────────────────────────────────────
			if sub == "clear" || sub == "limpiar" {
				if !c.IsOwner {
					return c.Reply("> *health clear* solo lo puede usar el dueño del bot.")
				}
				xlog.LimpiarErrores()
				return c.Reply("> ✅ Historial de errores recientes vaciado.")
			}

			snap := c.Bot.SendQueue().Snapshot()
			est, err := c.DB.Estadisticas(c.Ctx, c.Bot.Config().DBFile)
			if err != nil {
				c.Log.Warn("no se pudieron contar las filas de la base: " + err.Error())
			}

			lineas := []string{
				"《⊛》 Salud de *" + nombreBot(c) + "*",
				"",
				"◇ Conexión › " + conectado(c),
				"◇ Uptime › " + formatWait(int64(c.Bot.Uptime().Seconds())),
				"◇ Sistema › " + runtime.GOOS + "/" + runtime.GOARCH + " · Go " + runtime.Version(),
				"◇ CPU › " + strconv.Itoa(runtime.NumCPU()) + " núcleos",
				"",
				"✦ RAM sistema › " + memoriaSistema(),
				"✦ RAM del bot (RSS) › " + rssPropio(),
				"",
				"✪ ffmpeg › " + siNo(hayBinario("ffmpeg")),
				"✪ yt-dlp › " + siNo(hayBinario("yt-dlp")),
				"",
				"𓍯 Personas › " + miles(est.Usuarios),
				"𓍯 Chats con economía › " + miles(est.Chats),
				"𓍯 Advertencias › " + miles(est.Advertencias),
				"𓍯 Base de datos › " + bytesLegibles(est.Bytes),
				"",
				// El tope diario del calentamiento es invisible cuando se
				// alcanza: el bot deja de contestar los comandos normales y
				// el único rastro está en el log.
				fmt.Sprintf("⏳ Cola de hoy › *%d* / *%d* mensajes",
					snap.SentToday, snap.Budget),
			}
			if snap.Slowed {
				lineas = append(lineas, "> ⚠ Hoy el número va en modo lento (por rebotes del servidor).")
			}

			total := xlog.ContarErrores()
			if c.IsOwner {
				lineas = append(lineas, "", "❖ Errores recientes (*"+miles(int64(total))+"* en total):")
				ultimos := xlog.UltimosErrores(5)
				if len(ultimos) == 0 {
					lineas = append(lineas, "▸ (sin errores desde que arrancó)")
				}
				for _, e := range ultimos {
					lineas = append(lineas, fmt.Sprintf("▸ %s · [%s] %s",
						e.Cuando.Format("15:04"), e.Nivel, bot.ClipText(e.Mensaje, 110)))
				}
			} else {
				lineas = append(lineas, "", "❖ Errores recientes › *"+miles(int64(total))+"*")
			}

			if c.IsOwner {
				lineas = append(lineas, "",
					"> Vaciar el historial: *"+c.Bot.FirstPrefix()+"health clear*")
			}
			return c.Reply(strings.Join(lineas, "\n"))
		},
	})
}

// ─── .runtime ───────────────────────────────────────────────────────────────

func registerRuntime() {
	bot.Register(bot.Command{
		Name:     "runtime",
		Aliases:  []string{"uptime"},
		Category: bot.CatMain,
		Desc:     "Tiempo activo, memoria y cola de envío",
		Cooldown: 10,
		Run: func(c *bot.Context) error {
			snap := c.Bot.SendQueue().Snapshot()
			return c.Reply(fmt.Sprintf(
				"📊 *Estado · Shin-MD*\n"+
					"╭───────────────────\n"+
					"│ ⏱️ *Activo:* %s\n"+
					"│ 🧠 *RAM del bot:* %s\n"+
					"│ 🖥️ *Go:* %s · %s\n"+
					"│ ⏳ *Cola de hoy:* %d / %d\n"+
					"│ 🏷️ *Conexión:* %s\n"+
					"╰────「 反魂 」────",
				formatWait(int64(c.Bot.Uptime().Seconds())),
				rssPropio(),
				runtime.Version(), runtime.GOARCH,
				snap.SentToday, snap.Budget,
				conectado(c)))
		},
	})
}

// ─── .warmup ────────────────────────────────────────────────────────────────

func registerWarmup() {
	bot.Register(bot.Command{
		Name:      "warmup",
		Aliases:   []string{"tope", "limite", "limiteanti"},
		Category:  bot.CatOwner,
		Desc:      "Tope diario de mensajes del calentamiento anti-ban",
		OwnerOnly: true,
		Priority:  true,
		Cooldown:  5,
		Run: func(c *bot.Context) error {
			snap := c.Bot.SendQueue().Snapshot()
			lineas := []string{
				"《⊛》 Calentamiento del número",
				"",
				fmt.Sprintf("◇ Hoy › *%d* / *%d* mensajes", snap.SentToday, snap.Budget),
				"◇ Barra › " + barraTope(snap.SentToday, snap.Budget, 12),
				"◇ Reinicio del contador › en " + faltaMedianocheUTC(),
				"",
				"> Los topes se ajustan con WARMUP, WARMUP_START_MSGS, WARMUP_MAX_MSGS",
				"> y WARMUP_DAYS en el .env; no desde el chat.",
			}
			if snap.Slowed {
				lineas = append(lineas, "> ⚠ El número está en modo lento: el servidor nos pidió bajar el ritmo.")
			}
			if snap.SentToday >= snap.Budget {
				lineas = append(lineas, "> ⚠ Tope alcanzado: los comandos normales no responden hasta mañana.")
			}
			return c.Reply(strings.Join(lineas, "\n"))
		},
	})
}

// barraTope pinta el uso del día. Se separa para poder probarla sin el reloj.
func barraTope(usados, tope, ancho int) string {
	if tope <= 0 || ancho <= 0 {
		return ""
	}
	llenos := int(float64(usados) / float64(tope) * float64(ancho))
	if llenos < 0 {
		llenos = 0
	}
	if llenos > ancho {
		llenos = ancho
	}
	return strings.Repeat("▰", llenos) + strings.Repeat("▱", ancho-llenos)
}

// faltaMedianocheUTC: el contador se reinicia a las 00:00 UTC (igual que en el
// JS, que usaba toISOString). Se dice tal cual para que nadie espere a la hora
// equivocada de su país.
func faltaMedianocheUTC() string {
	ahora := time.Now().UTC()
	proximo := time.Date(ahora.Year(), ahora.Month(), ahora.Day(), 0, 0, 0, 0, time.UTC).Add(24 * time.Hour)
	d := proximo.Sub(ahora)
	return fmt.Sprintf("%dh %02dm", int(d.Hours()), int(d.Minutes())%60)
}

// ─── .infobot ───────────────────────────────────────────────────────────────

func registerInfobot() {
	bot.Register(bot.Command{
		Name:     "infobot",
		Aliases:  []string{"botinfo"},
		Category: bot.CatMain,
		Desc:     "Ficha técnica del bot",
		Priority: true,
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			cfg := c.Bot.Config()
			iniciativa := "sin dato"
			if !c.Bot.StartedAt.IsZero() {
				iniciativa = c.Bot.StartedAt.Local().Format("02/01/2006 15:04")
			}
			return c.Reply(fmt.Sprintf(
				"《⊛》 *Ficha del bot*\n"+
					"│ ◈ Nombre › *%s*\n"+
					"│ ◈ Versión › *%s*\n"+
					"│ ◈ Prefijos › %s\n"+
					"│ ◈ Dueños › %d\n"+
					"│ ◈ Comandos › %d\n"+
					"│ ◈ Encendido › %s\n"+
					"│ ◈ Motor › Go %s · whatsmeow\n"+
					"╰─「 Shin-MD 」",
				nombreBot(c), version,
				strings.Join(cfg.Prefixes, " "),
				len(cfg.Owners),
				len(bot.All()),
				iniciativa,
				runtime.Version()))
		},
	})
}

// ─── .compat ────────────────────────────────────────────────────────────────

// pixelPNG es la imagen de 1×1 que usa la prueba de álbum: sin red, sin
// archivos, sin depender de que hoy responda ninguna web.
var pixelPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0d, 0x49, 0x44, 0x41, 0x54, 0x78, 0xda, 0x63, 0xfc, 0xcf, 0xc0, 0xf0,
	0x1f, 0x00, 0x05, 0x00, 0x01, 0xff, 0x89, 0x99, 0x3d, 0x1d, 0x00, 0x00,
	0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func registerCompat() {
	bot.Register(bot.Command{
		Name:      "compat",
		Aliases:   []string{"diagnostico"},
		Category:  bot.CatOwner,
		Desc:      "Manda cuatro mensajes de prueba para ver cuál no lee tu WhatsApp",
		OwnerOnly: true,
		Priority:  true,
		Cooldown:  30,
		Run: func(c *bot.Context) error {
			if c.IsGroup {
				return c.Reply("「✧」 Esto va por privado.\n> Escríbeme *.compat* en un chat contigo para no llenar el grupo de pruebas.")
			}

			var resultados []string
			anotar := func(n int, tipo string, err error) {
				marca := "✅"
				if err != nil {
					marca = "❌ " + bot.ClipText(err.Error(), 90)
				}
				resultados = append(resultados, fmt.Sprintf("%s %d. %s", marca, n, tipo))
			}

			// 1. Texto plano: si esto falla, el problema no son los botones.
			anotar(1, "texto plano", c.Reply("「PRUEBA 1/4」 texto plano"))

			// 2. Tarjeta con botones (native_flow).
			anotar(2, "tarjeta con botones", c.ReplyButtons(
				"「PRUEBA 2/4」 botones",
				"Si esto sale raro, el culpable es native_flow.",
				"diagnóstico",
				bot.QuickReply("Botón A", "compat_a"),
				bot.QuickReply("Botón B", "compat_b")))

			// 3. Imagen: comprueba el envío de medios, que en el JS iba por
			//    otra ruta (y a veces por un servicio de terceros).
			anotar(3, "imagen", c.ReplyImage(pixelPNG,
				"「PRUEBA 3/4」 imagen",
				"Si SOLO falla esta, el problema es el envío de medios.",
				"diagnóstico"))

			// 4. Texto con formato: negritas, monoespaciado y cursivas.
			anotar(4, "formato", c.Reply(
				"「PRUEBA 4/4」 formato\n> *negrita* _cursiva_ ~tachado~ ```mono```"))

			return c.Reply("「✧」 *Resultado del envío*\n\n" + strings.Join(resultados, "\n") +
				"\n\n*Lo que buscamos*\n" +
				"> Mira los cuatro mensajes de arriba. El que salga como\n" +
				"> «no compatible con tu versión» es el que tu WhatsApp no lee,\n" +
				"> aunque aquí haya salido ✅ (enviar y poder leerlo son cosas\n" +
				"> distintas).\n\n" +
				"*Qué significa cada número*\n" +
				"> 1 → texto: si falla, no es la librería.\n" +
				"> 2 → botones: native_flow.\n" +
				"> 3 → imagen: subida de medios.\n" +
				"> 4 → formato: marcado de WhatsApp.")
		},
	})
}

// ─── .allmenu ───────────────────────────────────────────────────────────────

func registerAllMenu() {
	bot.Register(bot.Command{
		Name:     "allmenu",
		Aliases:  []string{"allcmds", "listatotal"},
		Category: bot.CatMain,
		Desc:     "Todos los comandos en una lista, sin categorías",
		Priority: true,
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			texto := listaTodosLosComandos(c.Bot.FirstPrefix())
			if len(texto) > 3500 {
				// WhatsApp corta los mensajes larguísimos: se avisa en vez de
				// mandar algo que llegue a medias.
				return c.Reply(texto[:3500] + "\n…\n> La lista sigue: usa *" +
					c.Bot.FirstPrefix() + "menu <categoría>* para verla por partes.")
			}
			return c.Reply(texto)
		},
	})
}

// listaTodosLosComandos arma la lista plana. Se ordena por categoría y nombre
// (no por orden de registro) para que dos ejecuciones den lo mismo.
func listaTodosLosComandos(prefijo string) string {
	todos := bot.All()
	sort.Slice(todos, func(i, j int) bool {
		if todos[i].Category != todos[j].Category {
			return todos[i].Category < todos[j].Category
		}
		return todos[i].Name < todos[j].Name
	})

	var b strings.Builder
	fmt.Fprintf(&b, "《⊛》 *Todos los comandos* (%d)\n", len(todos))
	cat := ""
	for _, cmd := range todos {
		if cmd.Category != bot.Category(cat) {
			cat = string(cmd.Category)
			fmt.Fprintf(&b, "\n*── %s ──*\n", strings.ToUpper(cat))
		}
		fmt.Fprintf(&b, "│ %s%s", prefijo, cmd.Name)
		if len(cmd.Aliases) > 0 {
			fmt.Fprintf(&b, " (%s)", strings.Join(cmd.Aliases, ", "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ─── ayudas de sistema ──────────────────────────────────────────────────────

func conectado(c *bot.Context) string {
	if c.Bot.LoggedIn() {
		return "🟢 Conectado"
	}
	return "🟡 Conectando / reconectando"
}

// hayBinario dice si una herramienta externa está en el PATH. Es lo que en el
// JS hacía `isYtdlpAvailable()` con un spawn: aquí basta con no tener que
// ejecutarla para saberlo.
func hayBinario(nombre string) bool {
	_, err := exec.LookPath(nombre)
	return err == nil
}

// rssPropio lee la memoria residente del proceso. En Linux sale de
// /proc/self/status; si no está (o el SO es otro), se da la memoria que Go
// tiene reservada, que es una cota parecida y nunca miente hacia abajo.
func rssPropio() string {
	if f, err := os.Open("/proc/self/status"); err == nil {
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			linea := sc.Text()
			if !strings.HasPrefix(linea, "VmRSS:") {
				continue
			}
			campos := strings.Fields(linea)
			if len(campos) >= 2 {
				if kb, err := strconv.ParseInt(campos[1], 10, 64); err == nil {
					return bytesLegibles(kb * 1024)
				}
			}
		}
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return bytesLegibles(int64(m.Sys))
}

// memoriaSistema lee /proc/meminfo. Si no se puede, devuelve «sin dato» en vez
// de un cero que parecería una alarma.
func memoriaSistema() string {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return "sin dato"
	}
	defer func() { _ = f.Close() }()

	var total, disponible int64
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		campos := strings.Fields(sc.Text())
		if len(campos) < 2 {
			continue
		}
		kb, err := strconv.ParseInt(campos[1], 10, 64)
		if err != nil {
			continue
		}
		switch campos[0] {
		case "MemTotal:":
			total = kb * 1024
		case "MemAvailable:":
			disponible = kb * 1024
		}
	}
	if total <= 0 {
		return "sin dato"
	}
	return bytesLegibles(total-disponible) + " / " + bytesLegibles(total)
}

// bytesLegibles pinta tamaños como los enseñaba el JS (MB, GB) sin depender de
// human-readable.
func bytesLegibles(n int64) string {
	if n < 0 {
		n = 0
	}
	const unidad = 1024
	if n < unidad {
		return strconv.FormatInt(n, 10) + " B"
	}
	valor := float64(n)
	for _, sufijo := range []string{"KB", "MB", "GB", "TB"} {
		valor /= unidad
		if valor < unidad {
			return strconv.FormatFloat(valor, 'f', 1, 64) + " " + sufijo
		}
	}
	return strconv.FormatFloat(valor, 'f', 1, 64) + " PB"
}

// nombreBot: el nombre configurado, o el de siempre si no hay.
func nombreBot(c *bot.Context) string {
	if n := strings.TrimSpace(c.Bot.Config().BotName); n != "" {
		return bot.Esc(n)
	}
	return "SHIN-MD"
}
