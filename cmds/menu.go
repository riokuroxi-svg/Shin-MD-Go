// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// menu.go — Puerto de cmds/main/menu.js + categorías en ListMessage paginado.
// Es el comando que mejor demuestra las dos cosas que te importan:
// botones nativos (con nodos <biz>) y arranque rápido (.menu con prioridad).
package cmds

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"shin-go/internal/bot"
)

func init() {
	bot.Register(bot.Command{
		Name:     "menu",
		Aliases:  []string{"ayuda", "help", "h"},
		Category: bot.CatMain,
		Desc:     "Muestra el menú con botones",
		Priority: true, // no espera el calentamiento: es el comando que más se usa
		Run: func(c *bot.Context) error {
			unique, aliases := bot.Count()

			// .menu <categoría> → imagen de esa categoría (si MENU_IMAGE=1) o texto
			if len(c.Args) > 0 {
				cat := strings.ToLower(c.Args[0])
				if c.Bot.Config().MenuImage {
					return sendFax(c, cat)
				}
				return menuCategory(c, cat)
			}

			// MENU_IMAGE=1: el menú sale como imagen dibujada (más vistoso y
			// no se puede "copiar" el formato). Por defecto va con botones,
			// que es más rápido y funciona en cualquier teléfono.
			if c.Bot.Config().MenuImage {
				return sendFax(c, "")
			}

			// El estilo elegido con .setmenu. La imagen manda sobre la
			// variante: si alguien pidió el menú dibujado, se respeta.
			switch varianteMenuActual(c) {
			case 2:
				return c.ReplyPriority(menuDeTexto(c, unique, aliases))
			case 4:
				return c.ReplyPriority(menuCompacto(c, unique, aliases))
			}

			body := menuNubeBody(
				c.Bot.Config().BotName,
				nombre(c),
				unique,
				aliases,
				bot.HookCount(),
				c.Bot.Prefixes(),
			)

			return c.ReplyMenu(
				"☁️ "+bot.CleanName(c.Bot.Config().BotName, 24),
				body,
				"✧ Elige una ruta · también puedes escribir "+c.Bot.FirstPrefix()+"menu <categoría>",
				menuNubeButtons(c.Bot.FirstPrefix())...,
			)
		},
	})

	bot.Register(bot.Command{
		Name:     "menucat",
		Aliases:  []string{"categorias", "menucat"},
		Category: bot.CatMain,
		Desc:     "Lista de categorías con sus comandos",
		Priority: true,
		Run: func(c *bot.Context) error {
			pagina := 1
			if len(c.Args) > 0 {
				arg := strings.ToLower(c.Args[0])
				if n, ok := menuNubePageArg(arg); ok {
					pagina = n
				} else {
					return menuCategory(c, arg)
				}
			}

			paginas := menuNubePages(bot.ByCategory())
			if len(paginas) == 0 {
				return c.ReplyPriority("No hay categorías registradas todavía.")
			}
			if pagina < 1 || pagina > len(paginas) {
				pagina = 1
			}

			titulo := "☁️ Categorías · " + bot.CleanName(c.Bot.Config().BotName, 24)
			if len(paginas) > 1 {
				titulo = fmt.Sprintf("☁️ Categorías %d/%d · %s", pagina, len(paginas), bot.CleanName(c.Bot.Config().BotName, 24))
			}
			return c.ReplyList(
				titulo,
				"╭── ⋅ ──  ☁️  ✧  🪽  ── ⋅ ──╮\n"+
					"Elige una ruta para abrir sus comandos.\n"+
					"╰── ⋅ ──  ★  ── ⋅ ──╯",
				"Explorar categorías",
				"También puedes escribir "+c.Bot.FirstPrefix()+"menu <categoría>",
				paginas[pagina-1]...,
			)
		},
	})
}

// menuNubeBody es la portada de la variante v1. Solo usa texto y formato que
// WhatsApp conserva en Android, iOS y Web: los glifos decorativos no dependen
// de una fuente externa y el nombre del usuario siempre llega saneado.
func menuNubeBody(botName, persona string, unicos, alias, hooks int, prefijos []string) string {
	if strings.TrimSpace(persona) == "" {
		persona = "*viajero*"
	}
	listaPrefijos := strings.Join(prefijos, " · ")
	if listaPrefijos == "" {
		listaPrefijos = "."
	}

	var b strings.Builder
	b.WriteString("╭── ⋅ ──  ☁️  ✧  🪽  ── ⋅ ──╮\n\n")
	fmt.Fprintf(&b, "       *%s*\n", menuNubeNombre(botName))
	b.WriteString("     反魂 · *NÚCLEO EN GO*\n\n")
	b.WriteString("╰── ⋅ ──  ★  ── ⋅ ──╯\n\n")
	fmt.Fprintf(&b, "✦ Hola, %s.\n", persona)
	b.WriteString("Tu espacio está listo. ¿A dónde vamos?\n\n")
	b.WriteString("╭─「 *RESUMEN* 」\n")
	fmt.Fprintf(&b, "│ %d comandos · %d nombres\n", unicos, alias)
	fmt.Fprintf(&b, "│ %d hooks activos\n", hooks)
	fmt.Fprintf(&b, "│ Prefijos · %s\n", listaPrefijos)
	b.WriteString("╰──────────────\n\n")
	b.WriteString("_Elige una ruta en los botones de abajo._")
	return b.String()
}

// menuNubeNombre da presencia al título sin prometer una tipografía que
// WhatsApp no permite elegir. Los caracteres fullwidth sí forman parte del
// mensaje y se ven igual aunque el teléfono use otra fuente del sistema.
func menuNubeNombre(name string) string {
	name = bot.CleanName(name, 24)
	if name == "" {
		name = "SHIN-MD"
	}
	name = strings.ToUpper(name)

	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r + 0xFEE0)
		case r == '-':
			b.WriteRune('–')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// menuNubeButtons mantiene dos accesos principales a la vista. Al haber más
// de tres acciones, internal/interactivos añade la hoja inferior «Ver todo»;
// los botones restantes siguen disponibles sin alargar la tarjeta del chat.
func menuNubeButtons(prefijo string) []bot.Button {
	if prefijo == "" {
		prefijo = "."
	}
	return []bot.Button{
		bot.QuickReply("☁️ Categorías", "menucat"),
		bot.QuickReply("🪽 Mi perfil", "profile"),
		bot.QuickReply("🏓 Ping", "ping"),
		bot.QuickReply("🖼️ Fax", "fax"),
		bot.CTACopy("📮 Copiar prefijo", prefijo),
		bot.CTAURL("🌐 Repositorio", repoURL),
	}
}

type menuNubeGrupo struct {
	titulo     string
	categorias []bot.Category
}

type menuNubeCategoria struct {
	icono  string
	titulo string
	desc   string
}

var menuNubeGrupos = []menuNubeGrupo{
	{
		titulo:     "Tu espacio",
		categorias: []bot.Category{bot.CatMain, bot.CatPerfil, bot.CatEconomy, bot.CatGacha},
	},
	{
		titulo: "Entretenimiento",
		categorias: []bot.Category{
			bot.CatAnime, bot.CatGames, bot.CatDownload, bot.CatMedia, bot.CatStickers, bot.CatNSFW,
		},
	},
	{
		titulo:     "Comunidad",
		categorias: []bot.Category{bot.CatGroup, bot.CatEventsCode, bot.CatSocket},
	},
	{
		titulo:     "Herramientas",
		categorias: []bot.Category{bot.CatUtils, bot.CatOwner},
	},
}

// El primer desplegable prioriza las rutas de uso diario. Las demás quedan en
// la página siguiente; cada página reserva sitio para navegar y nunca supera
// el límite oficial de diez filas de un ListMessage.
var menuNubeOrden = []bot.Category{
	bot.CatMain,
	bot.CatPerfil,
	bot.CatEconomy,
	bot.CatGames,
	bot.CatDownload,
	bot.CatStickers,
	bot.CatGroup,
	bot.CatUtils,
	bot.CatGacha,
	bot.CatAnime,
	bot.CatMedia,
	bot.CatNSFW,
	bot.CatEventsCode,
	bot.CatSocket,
	bot.CatOwner,
}

var menuNubeCategorias = map[bot.Category]menuNubeCategoria{
	bot.CatMain:       {"✦", "Principal", "Ayuda, estado y opciones esenciales"},
	bot.CatPerfil:     {"🪽", "Perfil", "Cuenta, nivel y preferencias"},
	bot.CatEconomy:    {"💰", "Economía", "Saldo, banco, tienda y rankings"},
	bot.CatGacha:      {"🎴", "Gacha", "Colección, personajes y sorteos"},
	bot.CatAnime:      {"🌸", "Anime", "Búsquedas y contenido de anime"},
	bot.CatGames:      {"🎮", "Juegos", "Retos, azar y aventuras"},
	bot.CatDownload:   {"🎧", "Descargas", "Música, video y redes sociales"},
	bot.CatMedia:      {"🎞️", "Multimedia", "Audio, imagen y conversión"},
	bot.CatStickers:   {"✨", "Stickers", "Crear y transformar stickers"},
	bot.CatNSFW:       {"🔞", "NSFW", "Contenido para adultos"},
	bot.CatGroup:      {"🛡️", "Grupos", "Moderación y administración"},
	bot.CatEventsCode: {"📅", "Eventos", "Automatizaciones del chat"},
	bot.CatSocket:     {"🔌", "Conexiones", "Sesiones y conexiones externas"},
	bot.CatUtils:      {"🧰", "Utilidades", "Herramientas para el día a día"},
	bot.CatOwner:      {"⚙️", "Owner", "Control y mantenimiento del bot"},
}

const menuNubeCategoriasPorPagina = 8

func menuNubePageArg(arg string) (int, bool) {
	const prefix = "pagina-"
	if !strings.HasPrefix(arg, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(arg, prefix))
	return n, err == nil && n > 0
}

// menuNubePages convierte el registro real en desplegables de diez filas como
// máximo: ocho categorías y hasta dos filas para anterior/siguiente. Cada
// categoría conserva el ID menu:<categoría>, que el parser enruta a
// `.menu <categoría>`. Las categorías futuras se agregan al final sin perderse.
func menuNubePages(cats map[bot.Category][]*bot.Command) [][]bot.ListSection {
	ordered := menuNubeCategoryOrder(cats)
	if len(ordered) == 0 {
		return nil
	}

	total := (len(ordered) + menuNubeCategoriasPorPagina - 1) / menuNubeCategoriasPorPagina
	pages := make([][]bot.ListSection, 0, total)
	for page := 0; page < total; page++ {
		start := page * menuNubeCategoriasPorPagina
		end := min(start+menuNubeCategoriasPorPagina, len(ordered))
		sections := menuNubeSectionsFor(cats, ordered[start:end])

		nav := bot.ListSection{Title: "Navegación"}
		if page > 0 {
			nav.Rows = append(nav.Rows, bot.ListRow{
				ID:    fmt.Sprintf("menucat:pagina-%d", page),
				Title: "← Página anterior",
				Desc:  "Volver a las categorías anteriores",
			})
		}
		if page+1 < total {
			nav.Rows = append(nav.Rows, bot.ListRow{
				ID:    fmt.Sprintf("menucat:pagina-%d", page+2),
				Title: "Más categorías →",
				Desc:  "Abrir las rutas restantes",
			})
		}
		if len(nav.Rows) > 0 {
			sections = append(sections, nav)
		}
		pages = append(pages, sections)
	}
	return pages
}

func menuNubeCategoryOrder(cats map[bot.Category][]*bot.Command) []bot.Category {
	ordered := make([]bot.Category, 0, len(cats))
	seen := make(map[bot.Category]bool, len(cats))
	for _, cat := range menuNubeOrden {
		if len(cats[cat]) == 0 {
			continue
		}
		seen[cat] = true
		ordered = append(ordered, cat)
	}

	extras := make([]string, 0)
	for cat, cmds := range cats {
		if !seen[cat] && len(cmds) > 0 {
			extras = append(extras, string(cat))
		}
	}
	sort.Strings(extras)
	for _, raw := range extras {
		ordered = append(ordered, bot.Category(raw))
	}
	return ordered
}

func menuNubeSectionsFor(cats map[bot.Category][]*bot.Command, selected []bot.Category) []bot.ListSection {
	chosen := make(map[bot.Category]bool, len(selected))
	for _, cat := range selected {
		chosen[cat] = true
	}

	added := make(map[bot.Category]bool, len(selected))
	sections := make([]bot.ListSection, 0, len(menuNubeGrupos)+1)
	for _, grupo := range menuNubeGrupos {
		sec := bot.ListSection{Title: grupo.titulo}
		for _, cat := range grupo.categorias {
			if !chosen[cat] || len(cats[cat]) == 0 {
				continue
			}
			added[cat] = true
			sec.Rows = append(sec.Rows, menuNubeRow(cat, len(cats[cat])))
		}
		if len(sec.Rows) > 0 {
			sections = append(sections, sec)
		}
	}

	extras := make([]string, 0)
	for _, cat := range selected {
		if !added[cat] && len(cats[cat]) > 0 {
			extras = append(extras, string(cat))
		}
	}
	sort.Strings(extras)
	if len(extras) > 0 {
		sec := bot.ListSection{Title: "Otras rutas"}
		for _, raw := range extras {
			cat := bot.Category(raw)
			sec.Rows = append(sec.Rows, menuNubeRow(cat, len(cats[cat])))
		}
		sections = append(sections, sec)
	}
	return sections
}

func menuNubeRow(cat bot.Category, total int) bot.ListRow {
	meta, ok := menuNubeCategorias[cat]
	if !ok {
		meta = menuNubeCategoria{"📁", bot.CleanName(string(cat), 18), "Comandos de esta categoría"}
	}
	return bot.ListRow{
		ID:    "menu:" + string(cat),
		Title: meta.icono + " " + meta.titulo,
		Desc:  fmt.Sprintf("%d comandos · %s", total, meta.desc),
	}
}

func menuCategory(c *bot.Context, cat string) error {
	cats := bot.ByCategory()
	cmds, ok := cats[bot.Category(cat)]
	if !ok {
		// El nombre de categoría viene del usuario y se repite en la
		// respuesta: se limpia y se recorta antes de mostrarlo.
		return c.Reply("✕ No existe la categoría *" + bot.CleanName(cat, 24) + "*.\nUsa .menucat")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "╭─「 *%s* 」\n", strings.ToUpper(cat))
	for _, cmd := range cmds {
		fmt.Fprintf(&sb, "│ %s%s — %s\n",
			c.Bot.FirstPrefix(), cmd.Name, cmd.Desc)
	}
	sb.WriteString("╰─")

	return c.ReplyPriority(sb.String())
}

// menuDeTexto es la variante v2 (.setmenu v2): todo en un mensaje, sin botones.
// Es la que funciona igual en un cliente viejo, en WhatsApp Web y en un
// teléfono sin soporte de botones.
func menuDeTexto(c *bot.Context, unicos, alias int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "╭─「 *%s* 」\n", bot.CleanName(c.Bot.Config().BotName, 24))
	fmt.Fprintf(&b, "│ %d comandos · %d nombres\n", unicos, alias)
	fmt.Fprintf(&b, "╰─ prefijos: %s\n", strings.Join(c.Bot.Prefixes(), " "))

	cats := bot.ByCategory()
	names := make([]string, 0, len(cats))
	for cat := range cats {
		names = append(names, string(cat))
	}
	sort.Strings(names)

	for _, nombre := range names {
		cmds := ordenDeComandos(cats[bot.Category(nombre)])
		fmt.Fprintf(&b, "\n*── %s (%d) ──*\n", strings.ToUpper(nombre), len(cmds))
		for _, cmd := range cmds {
			fmt.Fprintf(&b, "│ %s%s — %s\n", c.Bot.FirstPrefix(), cmd.Name, cmd.Desc)
		}
	}
	return b.String()
}

// menuCompacto es la variante v4 (.setmenu v4): cabecera corta, categorías con
// su número y el estado del bot en una línea. Pensada para mirar de un vistazo.
func menuCompacto(c *bot.Context, unicos, alias int) string {
	snap := c.Bot.SendQueue().Snapshot()
	cats := bot.ByCategory()

	names := make([]string, 0, len(cats))
	for cat := range cats {
		names = append(names, string(cat))
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprintf(&b, "《⊛》 *%s*\n", bot.CleanName(c.Bot.Config().BotName, 24))
	fmt.Fprintf(&b, "◇ %d comandos · %d nombres · %d hooks\n", unicos, alias, bot.HookCount())
	fmt.Fprintf(&b, "◇ Cola hoy › %d/%d · uptime %s\n", snap.SentToday, snap.Budget,
		formatWait(int64(c.Bot.Uptime().Seconds())))
	b.WriteString("╭───────────────────\n")
	for _, nombre := range names {
		fmt.Fprintf(&b, "│ ▸ *%s* (%d)\n", nombre, len(cats[bot.Category(nombre)]))
	}
	b.WriteString("╰─ escribe .menu <categoría>")
	return b.String()
}
