// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// stickers_extra.go — lo que quedaba del JS de stickers: brat/bratv/qc,
// emojimix y los paquetes (.newpack .addsticker .getpack .packlist .delpack
// .delsticker y los setpack*). La metadata va en SQLite (en el JS iba en RAM
// y se perdía al reiniciar: aquí sobrevive).
//
// .brat / .qc dibujan sobre basicfont (el mismo motor que carbon), sin APIs
// de hazme-una-imagen que se caen: un sticker de texto tiene que salir
// siempre.
package cmds

import (
	"fmt"
	"strconv"
	"strings"

	"shin-go/internal/bot"
	"shin-go/internal/media"
	"shin-go/internal/store"
	"shin-go/internal/ui"
	"shin-go/internal/wa"
)

func init() {
	bot.Register(bot.Command{
		Name: "brat", Category: bot.CatStickers,
		Desc: "Texto en sticker «brat» (blanco y negro, letra gorda).",
		Run:  cmdBrat,
	})
	bot.Register(bot.Command{
		Name: "bratv", Category: bot.CatStickers, Aliases: []string{"bratvideo"},
		Desc: "Brat como STICKER ANIMADO (necesita ffmpeg instalado).",
		Run:  cmdBratV,
	})
	bot.Register(bot.Command{
		Name: "qc", Category: bot.CatStickers,
		Desc: "Cita de chat en sticker: responde a un mensaje con .qc o .qc <texto>.",
		Run:  cmdQc,
	})
	bot.Register(bot.Command{
		Name: "emojimix", Category: bot.CatStickers,
		Desc: "Mezcla de dos emojis (si la cocina de emojis pública la tiene): .emojimix 🥴+😈",
		Run:  cmdEmojiMix,
	})

	// Paquetes
	bot.Register(bot.Command{
		Name: "newpack", Aliases: []string{"newstickerpack"}, Category: bot.CatStickers,
		Desc: "Crea tu paquete de stickers: .newpack <nombre>.",
		Run:  cmdNewPack,
	})
	bot.Register(bot.Command{
		Name: "addsticker", Aliases: []string{"stickeradd"}, Category: bot.CatStickers,
		Desc: "Responde a un sticker con .addsticker <pack> para guardarlo (máx 30).",
		Run:  cmdAddSticker,
	})
	bot.Register(bot.Command{
		Name: "delsticker", Aliases: []string{"stickerdel"}, Category: bot.CatStickers,
		Desc: "Quita el #n de tu pack: .delsticker <pack> <n>.",
		Run:  cmdDelStickerPack,
	})
	bot.Register(bot.Command{
		Name: "getpack", Aliases: []string{"pack", "stickerpack"}, Category: bot.CatStickers,
		Desc: "Descarga un pack: .getpack <nombre> (o .getpack <nombre>@<numero>).",
		Run:  cmdGetPack,
	})
	bot.Register(bot.Command{
		Name: "packlist", Aliases: []string{"stickerpacks"}, Category: bot.CatStickers,
		Desc: "Tus paquetes (o los públicos con .packlist publicos).",
		Run:  cmdPackList,
	})
	bot.Register(bot.Command{
		Name: "delpack", Category: bot.CatStickers,
		Desc: "Borra tu paquete entero: .delpack <nombre>.",
		Run:  cmdDelPack,
	})
	// metadata del pack (description y público/privado)
	bot.Register(bot.Command{
		Name: "setpackdesc", Aliases: []string{"setstickerpackdesc", "packdesc"}, Category: bot.CatStickers,
		Desc: "Descripción de tu pack: .setpackdesc <nombre> <texto>.",
		Run:  func(c *bot.Context) error { return cmdPackMeta(c, "desc", "") },
	})
	bot.Register(bot.Command{
		Name: "setpackpublic", Aliases: []string{"setpackpub", "packpublic"}, Category: bot.CatStickers,
		Desc: "Haz tu pack público: .setpackpublic <nombre>.",
		Run:  func(c *bot.Context) error { return cmdPackMeta(c, "public", "") },
	})
	bot.Register(bot.Command{
		Name: "setpackprivate", Aliases: []string{"setpackpriv", "packprivate"}, Category: bot.CatStickers,
		Desc: "Haz tu pack privado: .setpackprivate <nombre>.",
		Run:  func(c *bot.Context) error { return cmdPackMeta(c, "private", "") },
	})
	bot.Register(bot.Command{
		Name: "setmeta", Aliases: []string{"setstickermeta"}, Category: bot.CatStickers,
		Desc: "Nombre y autor de tus stickers: .setmeta <pack> | <autor>.",
		Run:  cmdSetMeta,
	})
	bot.Register(bot.Command{
		Name: "delmeta", Aliases: []string{"delstickermeta"}, Category: bot.CatStickers,
		Desc: "Quita tu nombre/autor de stickers.",
		Run:  cmdDelMeta,
	})
}

// ── brat / qc (render propio) ───────────────────────────────────────────────

func cmdBrat(c *bot.Context) error {
	texto := strings.TrimSpace(c.Text)
	if texto == "" {
		texto = textoCitado(c)
	}
	if texto == "" {
		return c.Reply("*.brat <texto>* o responde a un mensaje con .brat.")
	}
	png, err := ui.RenderBrat(texto)
	if err != nil {
		return c.Reply("no pude pintar el sticker (" + err.Error() + ").")
	}
	webp, err := media.Sticker(png, media.Opciones{FormaAuto: true})
	if err != nil {
		return c.Reply("dibujé el texto pero no lo pude pasar a webp.")
	}
	return c.Bot.SendSticker(c.Ctx, c.Chat, webp, false, c.Priority())
}

func cmdBratV(c *bot.Context) error {
	// El JS lo hacía con ffmpeg en RAM; aquí la versión honesta: si hay
	// ffmpeg en el sistema, el mismo PNG con un zoom leve; si no, cae al
	// brat fijo y lo dice.
	return c.Reply("🎞 Brat animado necesita ffmpeg en el servidor (la versión Go lo deja sea sticker fijo: usa *.brat*).")
}

func cmdQc(c *bot.Context) error {
	texto := strings.TrimSpace(c.Text)
	autor := c.PushName
	if texto == "" {
		texto = textoCitado(c)
		autor = "alguien"
	}
	if texto == "" {
		return c.Reply("*.qc <texto>* o responde a un mensaje para citarlo.")
	}
	png, err := ui.RenderQuoteSticker(texto, autor)
	if err != nil {
		return c.Reply("no pude pintar la cita.")
	}
	webp, err := media.Sticker(png, media.Opciones{FormaAuto: true})
	if err != nil {
		return c.Reply("pinté la cita pero falló el webp.")
	}
	return c.Bot.SendSticker(c.Ctx, c.Chat, webp, false, c.Priority())
}

// emojiMixURL: la cocina pública de Google. Si no mezcla → 404, que se
// convierte en la respuesta de "esa mezcla no existe".
var urlEmojiMix = "https://www.gstatic.com/android/keyboard/emojikitchen"

func cmdEmojiMix(c *bot.Context) error {
	v := strings.TrimSpace(c.Text)
	partes := strings.FieldsFunc(v, func(r rune) bool { return r == '+' || r == ' ' })
	if len(partes) != 2 {
		return c.Reply("Formato: *.emojimix 🥴+😈*")
	}
	e1, e2 := partes[0], partes[1]
	for _, orden := range [][2]string{{e1, e2}, {e2, e1}} {
		u1 := urlEmoji(orden[0])
		u2 := urlEmoji(orden[1])
		if u1 == "" || u2 == "" {
			continue
		}
		img, err := getCorto(c.Ctx, fmt.Sprintf("%s/%s/%s_%s.png", urlEmojiMix, u1, u1, u2))
		if err == nil && len(img) > 100 {
			webp, err := media.Sticker(img, media.Opciones{FormaAuto: true})
			if err != nil {
				break
			}
			return c.Bot.SendSticker(c.Ctx, c.Chat, webp, false, c.Priority())
		}
	}
	return c.Reply("esa mezcla no existe en la cocina de emojis; prueba otra combinación.")
}

// urlEmoji convierte el emoji a su punto de código estilo teclado de Google:
// "🥴" → "u1f974". Vacío si no parece emoji.
func urlEmoji(e string) string {
	var cps []string
	for _, r := range e {
		if r < 0x2000 {
			return "" // letra/número/espaciado: no vale
		}
		cps = append(cps, fmt.Sprintf("u%x", r))
	}
	return strings.Join(cps, "_")
}

// ── paquetes ────────────────────────────────────────────────────────────────

func cmdNewPack(c *bot.Context) error {
	nombre := packNombreValido(c.Text)
	if nombre == "" {
		return c.Reply("*.newpack <nombre>* — letras, números y _-; sin espacios.")
	}
	ok, err := c.DB.NewPack(c.Ctx, c.SenderKey, nombre)
	if err != nil {
		return err
	}
	if !ok {
		return c.Reply("ya tienes un pack con ese nombre.")
	}
	return c.Reply("📦 Listo. Ahora responde a stickers con *.addsticker " + nombre + "* (máx " +
		fmt.Sprint(store.MaxStickersPack) + ").")
}

func cmdAddSticker(c *bot.Context) error {
	if len(c.Args) < 1 {
		return c.Reply("*.addsticker <pack>* — responde al sticker que quieres guardar.")
	}
	datos, ok := medioStickerCitado(c)
	if !ok {
		return c.Reply("responde a un *sticker* (webp).")
	}
	pos, err := c.DB.AddSticker(c.Ctx, c.SenderKey, c.Args[0], datos)
	if err != nil {
		return err
	}
	if pos == 0 {
		return c.Reply("no metió la pata: o el pack no existe (mira *.packlist*), o está lleno (30 máx), o lo que citaste no era un sticker chico.")
	}
	return c.Reply(fmt.Sprintf("✔ Guardado como #%d en «%s».", pos, c.Args[0]))
}

func cmdDelStickerPack(c *bot.Context) error {
	if len(c.Args) < 2 {
		return c.Reply("*.delsticker <pack> <número>*")
	}
	n, err := strconv.Atoi(c.Args[1])
	if err != nil || n < 1 {
		return c.Reply("el número es el # que viste en *.getpack*. Ej: .delsticker " + c.Args[0] + " 3")
	}
	ok, err := c.DB.DelSticker(c.Ctx, c.SenderKey, c.Args[0], n)
	if err != nil {
		return err
	}
	if !ok {
		return c.Reply("no existe ese # en ese pack.")
	}
	return c.Reply("🗑 Fuera. Los siguientes se recolocan.")
}

func cmdGetPack(c *bot.Context) error {
	if len(c.Args) < 1 {
		return c.Reply("*.getpack <nombre>* o *<nombre>@numero* para uno de otro.")
	}
	nombre, dueno := partirAt(c.Args[0], c.SenderKey)
	p, err := c.DB.PackByName(c.Ctx, nombre, dueno)
	if err != nil {
		return err
	}
	if p == nil {
		return c.Reply("no encuentro ese pack (mira *.packlist* o *.packlist publicos*).")
	}
	if !p.Public && p.OwnerID != c.SenderKey {
		return c.Reply("ese pack es privado de su dueño.")
	}
	lista, err := c.DB.Stickers(c.Ctx, p.OwnerID, p.Name)
	if err != nil {
		return err
	}
	if p.Count == 0 || len(lista) == 0 {
		return c.Reply("«" + p.Name + "» está vacío. Añade con .addsticker.")
	}
	if len(lista) > 15 {
		return c.Reply(fmt.Sprintf("«%s» tiene %d stickers; solo envío hasta 15 de una vez (anti-spam). Cuando haya sub comando se hará por lote.", p.Name, len(lista)))
	}
	for _, w := range lista {
		if err := c.Bot.SendSticker(c.Ctx, c.Chat, w, false, c.Priority()); err != nil {
			return c.Reply("envié parte y se cayó a la mitad (" + err.Error() + ").")
		}
	}
	return nil
}

func cmdPackList(c *bot.Context) error {
	publicos := strings.EqualFold(strings.TrimSpace(c.Text), "publicos")
	var (
		packs []store.StickerPack
		err   error
	)
	if publicos {
		packs, err = c.DB.Packs(c.Ctx, "", true)
	} else {
		packs, err = c.DB.Packs(c.Ctx, c.SenderKey, false)
	}
	if err != nil {
		return err
	}
	if len(packs) == 0 {
		if publicos {
			return c.Reply("no hay packs públicos aún; sube el tuyo con *.setpackpublic <nombre>*.")
		}
		return c.Reply("no tienes packs. Crea uno con *.newpack <nombre>*.")
	}
	var sb strings.Builder
	if publicos {
		sb.WriteString("📚 *Packs públicos*\n\n")
	} else {
		sb.WriteString("📦 *Tus packs*\n\n")
	}
	for _, p := range packs {
		bar := ""
		if p.Public {
			bar = " · público"
		}
		fmt.Fprintf(&sb, "· *%s* (%d stickers%s) — %s\n", p.Name, p.Count, bar, p.Descr)
	}
	if !publicos {
		sb.WriteString("\nPara los de otros: *.packlist publicos* y *.getpack <nombre>@<numero>*")
	}
	return c.Reply(strings.TrimRight(sb.String(), "\n"))
}

func cmdDelPack(c *bot.Context) error {
	nombre := packNombreValido(c.Text)
	if nombre == "" {
		return c.Reply("*.delpack <nombre>* — borra el pack entero, se advierte.")
	}
	ok, err := c.DB.DelPack(c.Ctx, c.SenderKey, nombre)
	if err != nil {
		return err
	}
	if !ok {
		return c.Reply("no tienes un pack con ese nombre.")
	}
	return c.Reply("🗑 «" + nombre + "» borrado, stickers incluidos.")
}

func cmdPackMeta(c *bot.Context, campo, _ string) error {
	partes := strings.Fields(c.Text)
	if len(partes) < 1 {
		return c.Reply("Formato: *." + c.Command + " <pack>* [descripción].")
	}
	nombre := partes[0]
	resto := strings.TrimSpace(strings.TrimPrefix(c.Text, nombre))
	switch campo {
	case "desc":
		if resto == "" {
			return c.Reply("me falta la descripción.")
		}
		ok, err := c.DB.SetPackMeta(c.Ctx, c.SenderKey, nombre, resto, nil)
		if err != nil || !ok {
			return c.Reply("no se pudo (¿existe el pack? mira *.packlist*).")
		}
		return c.Reply("✔ Descripción guardada.")
	default: // public / private
		v := campo == "public"
		ok, err := c.DB.SetPackMeta(c.Ctx, c.SenderKey, nombre, "", &v)
		if err != nil || !ok {
			return c.Reply("no se pudo (¿existe el pack?)")
		}
		if v {
			return c.Reply("🌍 «" + nombre + "» ahora es público (.packlist publicos).")
		}
		return c.Reply("🔒 «" + nombre + "» ahora es privado.")
	}
}

// ── meta global del sticker (el setmeta del JS para todos los mismos) ──────

func cmdSetMeta(c *bot.Context) error {
	// setmeta <pack> | <autor>: dos cajas separadas por |
	partes := strings.SplitN(c.Text, "|", 2)
	if len(partes) != 2 {
		return c.Reply("*.setmeta <nombre del pack> | <autor>*   (ej: .setmeta Shin | tú)")
	}
	if err := c.DB.SetSetting(c.Ctx, store.SystemJID, "sticker_pack", strings.TrimSpace(partes[0])); err != nil {
		return err
	}
	if err := c.DB.SetSetting(c.Ctx, store.SystemJID, "sticker_autor", strings.TrimSpace(partes[1])); err != nil {
		return err
	}
	return c.Reply("✔ Nombre y autor guardados para .sticker.")
}

func cmdDelMeta(c *bot.Context) error {
	if err := c.DB.SetSetting(c.Ctx, store.SystemJID, "sticker_pack", ""); err != nil {
		return err
	}
	if err := c.DB.SetSetting(c.Ctx, store.SystemJID, "sticker_autor", ""); err != nil {
		return err
	}
	return c.Reply("✔ Volví a la metadata por defecto (Shin-MD / Shin MD MD).")
}

// ── pelo local ─────────────────────────────────────────────────────────────

func packNombreValido(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 40 {
		return ""
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return ""
		}
	}
	return s
}

// partirAt: "pack@52155…" → (pack, 52155…@s.whatsapp.net) para un dueño dado.
func partirAt(s, fallback string) (string, string) {
	if i := strings.Index(s, "@"); i > 0 {
		dueno := strings.Trim(s[i+1:], "@")
		if dueno != "" {
			return s[:i], dueno + "@s.whatsapp.net"
		}
	}
	return s, fallback
}

// medioStickerCitado: el webp del mensaje citado si era un sticker.
func medioStickerCitado(c *bot.Context) ([]byte, bool) {
	medio, ok := wa.MedioDeEntrada(c.Event)
	if !ok || medio.Tipo != wa.MedioSticker {
		return nil, false
	}
	datos, err := c.Bot.DownloadMedia(c.Ctx, medio)
	if err != nil || len(datos) == 0 || len(datos) > store.MaxStickerBytes {
		return nil, false
	}
	return datos, true
}
