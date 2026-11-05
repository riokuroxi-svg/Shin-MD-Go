// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// meta.go — Bloque B2: la cara del bot.
//
//	.fax      menú dibujado como imagen (sin dependencias, ver ui/fax.go)
//	.info     estado real: versión, uptime, comandos, hooks, base, cola
//	.owner    contacto del dueño con botones (llamar, escribir, repo)
//	.report   el usuario avisa al dueño de algo (con límite anti-spam)
//
// Todo lo que viene del usuario (su nombre, su motivo, su reporte) se limpia y
// se escapa antes de entrar en un mensaje: ver bot/output.go.
package cmds

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"shin-go/internal/bot"
	"shin-go/internal/ui"
)

func init() {
	// ── .fax ─────────────────────────────────────────────────────────────
	bot.Register(bot.Command{
		Name:     "fax",
		Aliases:  []string{"menuimg", "menuimagen", "cartel"},
		Category: bot.CatMain,
		Desc:     "Menú en imagen (se dibuja en Go, sin dependencias)",
		Priority: true, // es un menú: no espera el calentamiento
		Cooldown: 10,
		Run: func(c *bot.Context) error {
			return sendFax(c, "")
		},
	})

	// ── .info ────────────────────────────────────────────────────────────
	bot.Register(bot.Command{
		Name:     "info",
		Aliases:  []string{"estado", "status", "stats"},
		Category: bot.CatMain,
		Desc:     "Estado del bot: versión, uptime, comandos y base",
		Priority: true,
		Run: func(c *bot.Context) error {
			unique, aliases := bot.Count()

			snap := c.Bot.Queue.Snapshot()
			cola := fmt.Sprintf("%d/%d hoy", snap.SentToday, snap.Budget)
			if snap.Slowed {
				cola += " (lento)"
			}

			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)

			conexion := "reconectando…"
			if c.Bot.WA.IsConnected() {
				conexion = "activa ✓"
			}

			lineas := []string{
				"╭─「 *INFO* 」",
				fmt.Sprintf("│ Versión: %s", version),
				fmt.Sprintf("│ Uptime: %s", duracion(c.Bot.Uptime())),
				fmt.Sprintf("│ Conexión: %s", conexion),
				fmt.Sprintf("│ Comandos: %d (%d nombres) · %d hooks", unique, aliases, bot.HookCount()),
				fmt.Sprintf("│ Cola: %s", cola),
				fmt.Sprintf("│ Calentamiento: desde %s", snap.Anchor.Format("02/01/2006")),
				fmt.Sprintf("│ Memoria: %s", sizeMB(mem.Alloc)),
				fmt.Sprintf("│ Go: %s · %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH),
				fmt.Sprintf("│ Chat: %s", chatKind(c)),
				"╰─",
			}

			// Los datos internos (cuántos usuarios, cuántas identidades, desde
			// cuándo calienta el número) solo los ve el owner: a un usuario
			// cualquiera le sirven de poco y a un curioso le sirven de mucho.
			if c.IsOwner {
				usuarios, _ := c.DB.CountUsers(c.Ctx)
				identidades, _ := c.DB.CountIdentities(c.Ctx)
				lineas = insertBeforeLast(lineas,
					fmt.Sprintf("│ Base: %d usuarios · %d identidades", usuarios, identidades))
			}

			return c.ReplyButtons(
				"📊 Shin-MD",
				strings.Join(lineas, "\n"),
				"datos reales del proceso, no estimados",
				bot.QuickReply("📋 Menú", "menu"),
				bot.QuickReply("🏓 Ping", "ping"),
			)
		},
	})

	// ── .owner ───────────────────────────────────────────────────────────
	bot.Register(bot.Command{
		Name:     "owner",
		Aliases:  []string{"dueño", "dueno", "creador", "contacto"},
		Category: bot.CatMain,
		Desc:     "Contacto del dueño del bot",
		Priority: true,
		Run: func(c *bot.Context) error {
			cfg := c.Bot.Config()
			nombre := bot.CleanName(cfg.OwnerName, 32)
			if nombre == "" {
				nombre = "el dueño"
			}

			cuerpo := fmt.Sprintf(
				"╭─「 *OWNER* 」\n│ 👤 %s\n│ 🏷 %s\n│ 🤖 núcleo en Go sobre whatsmeow\n╰─ escribe .report para avisarle",
				nombre, cfg.BotName)

			botones := []bot.Button{bot.CTAURL("🌐 Repositorio", repoURL)}
			if num := cfg.OwnerNumber; num != "" {
				// El botón de llamada y el de WhatsApp solo se muestran si hay
				// número configurado: un enlace roto se ve peor que no tenerlo.
				tel := "+" + bot.NormalizeNumber(num)
				botones = append([]bot.Button{
					bot.CTACall("📞 Llamar", tel),
					bot.CTAURL("💬 WhatsApp", "https://wa.me/"+bot.NormalizeNumber(num)),
				}, botones...)
			}
			botones = append(botones, bot.QuickReply("📋 Menú", "menu"))

			return c.ReplyButtons("👤 Owner", cuerpo, cfg.BotName, botones...)
		},
	})

	// ── .report ──────────────────────────────────────────────────────────
	reportGuard := bot.NewRateGuard(2, 10*time.Minute)

	bot.Register(bot.Command{
		Name:     "report",
		Aliases:  []string{"reportar", "reporte", "avisar"},
		Category: bot.CatUtils,
		Desc:     "Avisa al dueño de un problema (.report texto)",
		Cooldown: 15,
		Run: func(c *bot.Context) error {
			if c.IsOwner {
				return c.Reply("✕ Eres el owner: no hace falta que te reportes a ti mismo.")
			}

			texto := strings.TrimSpace(strings.Join(c.Args, " "))
			if texto == "" {
				return c.Reply("Uso: .report lo que quieras avisar\nMáximo 500 caracteres.")
			}

			// Límite: dos reportes cada 10 minutos por usuario. Evita que
			// alguien use el bot para inundar al owner.
			if allow, warn := reportGuard.Allow(c.Key()); !allow {
				if warn {
					return c.Reply("✕ Ya enviaste varios reportes seguidos. Espera un rato.")
				}
				return nil
			}

			texto = bot.ClipText(texto, 500)

			// Al owner se le manda con el remitente para que pueda responder.
			for _, num := range c.Bot.Config().Owners {
				jid := ownerJID(num)
				if jid.IsEmpty() {
					continue
				}
				aviso := fmt.Sprintf(
					"📮 *Reporte* de %s\n│ Chat: %s\n│ %s\n╰─ responde a este mensaje para contestarle",
					nombre(c), chatKind(c), texto)
				if err := c.Bot.SendText(c.Ctx, jid, aviso, false); err != nil {
					c.Log.Warn("no se pudo entregar el reporte: " + err.Error())
				}
			}

			return c.ReplyButtons(
				"📮 Reporte enviado",
				"Gracias, el aviso llegó al owner.\nSu reporte incluye este chat para poder responder.",
				"Shin-MD",
				bot.QuickReply("📋 Menú", "menu"),
			)
		},
	})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// repoURL: un solo sitio donde vive el enlace del repositorio.
const repoURL = "https://github.com/riokuroxi-svg/Shin-MD-Go"

// version la rellena main con -ldflags; aquí vale la de por defecto.
var version = "0.8.0"

// SetVersion permite que main.go publique la versión real en .info.
func SetVersion(v string) { version = v }

// sendFax dibuja el menú y lo manda como imagen. Si algo falla en el dibujo o
// en la subida, cae a texto: nunca se queda sin responder.
func sendFax(c *bot.Context, categoria string) error {
	lineas, titulo := menuLines(c, categoria)

	png, err := ui.RenderFax(ui.FaxDoc{
		Title:    titulo,
		Subtitle: fmt.Sprintf("%d COMANDOS · %s", countCommands(categoria), strings.ToUpper(c.Bot.FirstPrefix())),
		Lines:    lineas,
		Footer:   "ESCRIBE " + strings.ToUpper(c.Bot.FirstPrefix()) + "MENU <CATEGORIA>",
	})
	if err != nil {
		// Dibujo imposible (no debería pasar con los topes): texto plano.
		c.Log.Warn("no se pudo dibujar el fax: " + err.Error())
		return c.Reply("╭─「 " + titulo + " 」\n│ " + strings.Join(lineas, "\n│ ") + "\n╰─")
	}

	caption := fmt.Sprintf("🖼 *%s* — escribe %smenu <categoría>", titulo, c.Bot.FirstPrefix())

	// Con imagen de cabecera y botones queda como una tarjeta; si WhatsApp no
	// pinta la tarjeta, la imagen suelta ya lleva el menú dentro.
	if err := c.ReplyImage(png, titulo, caption, "Shin-MD · imagen generada en Go",
		bot.QuickReply("📋 Categorías", "menucat"),
		bot.QuickReply("🏓 Ping", "ping"),
	); err != nil {
		if imgErr := c.Bot.SendImage(c.Ctx, c.Chat, png, caption, true); imgErr != nil {
			c.Log.Warn("no se pudo mandar el fax: " + imgErr.Error())
			return c.Reply("╭─「 " + titulo + " 」\n│ " + strings.Join(lineas, "\n│ ") + "\n╰─")
		}
	}
	return nil
}

// menuLines arma las líneas del dibujo: por categoría, o todas si no se pide
// una en concreto.
func menuLines(c *bot.Context, categoria string) ([]string, string) {
	pref := c.Bot.FirstPrefix()
	cats := bot.ByCategory()

	var nombres []string
	for cat := range cats {
		nombres = append(nombres, string(cat))
	}
	sort.Strings(nombres)

	if categoria != "" {
		cmds, ok := cats[bot.Category(categoria)]
		if !ok {
			return []string{"NO EXISTE ESA CATEGORIA", "USA " + strings.ToUpper(pref) + "MENUCAT"}, "SHIN-MD"
		}
		var lineas []string
		for _, cmd := range cmds {
			lineas = append(lineas, "> "+strings.ToUpper(pref+cmd.Name))
			if cmd.Desc != "" {
				lineas = append(lineas, "  "+strings.ToUpper(cmd.Desc))
			}
		}
		return lineas, strings.ToUpper(categoria)
	}

	var lineas []string
	for _, cat := range nombres {
		lineas = append(lineas, strings.ToUpper(cat))
		for _, cmd := range cats[bot.Category(cat)] {
			lineas = append(lineas, "> "+strings.ToUpper(pref+cmd.Name)+"  "+strings.ToUpper(shortDesc(cmd.Desc)))
		}
		lineas = append(lineas, "")
	}
	return lineas, "SHIN-MD · MENU"
}

// shortDesc recorta la descripción para que quepa en una línea del dibujo.
func shortDesc(d string) string {
	if r := []rune(d); len(r) > 22 {
		return string(r[:21]) + "…"
	}
	return d
}

func countCommands(categoria string) int {
	cats := bot.ByCategory()
	if categoria != "" {
		return len(cats[bot.Category(categoria)])
	}
	total := 0
	for _, cmds := range cats {
		total += len(cmds)
	}
	return total
}

// chatKind describe dónde está pasando esto (va en .info y .report).
func chatKind(c *bot.Context) string {
	if c.IsGroup {
		return "grupo " + c.Chat.User
	}
	return "privado"
}

// insertBeforeLast mete una línea antes del cierre "╰─" del recuadro.
func insertBeforeLast(lineas []string, nueva string) []string {
	if len(lineas) == 0 {
		return []string{nueva}
	}
	out := make([]string, 0, len(lineas)+1)
	out = append(out, lineas[:len(lineas)-1]...)
	out = append(out, nueva, lineas[len(lineas)-1])
	return out
}

// ownerJID convierte un número normalizado en JID de WhatsApp.
func ownerJID(num string) types.JID {
	n := bot.NormalizeNumber(num)
	if n == "" {
		return types.EmptyJID
	}
	return types.JID{User: n, Server: types.DefaultUserServer}
}

func sizeMB(b uint64) string {
	const mb = 1024 * 1024
	if b < mb {
		return fmt.Sprintf("%.0f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/mb)
}
