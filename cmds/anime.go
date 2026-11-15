// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// anime.go — las reacciones anime del JS (cmds/anime/shares.js: 141 nombres)
// más .anime/.waifu/.neko/.ppcouple de la misma carpeta.
//
// El JS las servía desde su API propia (Ginko, con clave: no exportable).
// Aquí el proveedor es **waifu.pics** (público, sin clave) para las acciones
// que tienen GIF oficial; las que no tienen endpoint mandan solo la frase en
// texto — el JS ya hacía algo parecido cuando la API no tenía la acción
// (caía a caption sin GIF). La tabla es declarativa a propósito: si waifu.pics
// añade una acción, basta poner su slug en la columna `gif`.
//
// Cada acción lleva dos frases: sola (sin mención) y hacia alguien, como el
// JS ("está abrazando a …" / "se abraza a sí misma").
package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"shin-go/internal/bot"

	"net/url"
)

// urlWaifuPics es variable para que las pruebas lo apunten a httptest.
var urlWaifuPics = "https://api.waifu.pics/sfw"

// accion describe una reacción: slugs de waifu.pics (o "" = sin gif) y las
// frases sola / hacia otra persona. El segundo campo es el alias o nombre
// principal en español cuando existe.
type accion struct {
	slug  string // endpoint en waifu.pics (slug); vacío si waifu.pics no la sirve
	sola  string // c.Caption cuando no hay mención
	hacia string // cuando hay mención (se añade "@…")
}

// acciones: nombre → acción. Los alias se registran con el mismo nombre que
// la clave (mismo comportamiento), así "hug" y "abrazar" son la misma fila.
var acciones = map[string]accion{
	"hug":          {"hug", "se abraza a sí misma", "está abrazando a"},
	"abrazar":      {"hug", "se abraza a sí misma", "está abrazando a"},
	"kiss":         {"kiss", "se besa a sí misma", "está besando a"},
	"muak":         {"kiss", "se besa a sí misma", "está besando a"},
	"beso":         {"kiss", "se besa a sí misma", "está besando a"},
	"besar":        {"kiss", "se besa a sí misma", "está besando a"},
	"kisscheek":    {"kiss", "se besa la mejilla", "le besa la mejilla a"},
	"pat":          {"pat", "se acaricia la cabeza", "le acaricia la cabeza a"},
	"acariciar":    {"pat", "se acaricia la cabeza", "le acaricia la cabeza a"},
	"slap":         {"slap", "se da una bofetada", "le da una bofetada a"},
	"bofetada":     {"slap", "se da una bofetada", "le da una bofetada a"},
	"punch":        {"kick", "se golpea solo", "le pega un puñetazo a"},
	"golpear":      {"kick", "se golpea solo", "le pega un puñetazo a"},
	"bonk":         {"bonk", "se da con el bat en la cabeza", "le da un bonk a"},
	"golpe":        {"bonk", "se da con el bat en la cabeza", "le da un bonk a"},
	"bite":         {"bite", "se muerde el brazo", "le muerde a"},
	"morder":       {"bite", "se muerde el brazo", "le muerde a"},
	"lick":         {"lick", "se lame la mano", "le lame a"},
	"lamer":        {"lick", "se lame la mano", "le lame a"},
	"hump":         {"hug", "anda amoroso", "está muy cariñoso con"},
	"cuddle":       {"cuddle", "se acurruca con una almohada", "se acurruca con"},
	"acurrucar":    {"cuddle", "se acurruca con una almohada", "se acurruca con"},
	"snuggle":      {"cuddle", "se acurruca dulce", "se acurruca dulce con"},
	"acurrucarse":  {"cuddle", "se acurruca dulce", "se acurruca dulce con"},
	"highfive":     {"highfive", "se choca los cinco solo", "choca los cinco con"},
	"chocar":       {"highfive", "se choca los cinco solo", "choca los cinco con"},
	"handhold":     {"handhold", "se toma su propia mano", "le toma la mano a"},
	"tomar":        {"handhold", "se toma su propia mano", "le toma la mano a"},
	"wave":         {"wave", "saluda al aire", "saluda a"},
	"saludar":      {"wave", "saluda al aire", "saluda a"},
	"wink":         {"wink", "se guiña el ojo", "le guiña el ojo a"},
	"guiñar":       {"wink", "se guiña el ojo", "le guiña el ojo a"},
	"smile":        {"smile", "sonríe para sí", "le sonríe a"},
	"sonreir":      {"smile", "sonríe para sí", "le sonríe a"},
	"happy":        {"happy", "está feliz", "está feliz por"},
	"feliz":        {"happy", "está feliz", "está feliz por"},
	"clap":         {"clap", "aplaude solo", "aplaude a"},
	"aplaudir":     {"clap", "aplaude solo", "aplaude a"},
	"dance":        {"dance", "está bailando", "baila con"},
	"bailar":       {"dance", "está bailando", "baila con"},
	"sing":         {"dance", "está cantando", "le canta a"},
	"cantar":       {"dance", "está cantando", "le canta a"},
	"cry":          {"cry", "está llorando", "llora por"},
	"llorar":       {"cry", "está llorando", "llora por"},
	"sad":          {"cry", "está triste", "está triste por"},
	"triste":       {"cry", "está triste", "está triste por"},
	"blush":        {"blush", "se sonroja", "se sonroja por"},
	"sonrojarse":   {"blush", "se sonroja", "se sonroja por"},
	"shy":          {"blush", "está tímido", "se pone tímido con"},
	"timido":       {"blush", "está tímido", "se pone tímido con"},
	"timida":       {"blush", "está tímida", "se pone tímida con"},
	"smug":         {"smug", "presume solo", "presume delante de"},
	"presumir":     {"smug", "presume solo", "presume delante de"},
	"smirk":        {"smug", "sonríe con suficiencia", "le sonríe con suficiencia a"},
	"cringe":       {"cringe", "siente vergüenza ajena", "le da vergüenza ajena"},
	"avergonzarse": {"cringe", "siente vergüenza ajena", "le da vergüenza ajena"},
	"asco":         {"cringe", "siente asco", "le da asco"},
	"bully":        {"bully", "se molesta a sí mismo", "está molestando a"},
	"molestar":     {"bully", "se molesta a sí mismo", "está molestando a"},
	"kill":         {"kill", "se autodestruye", "mata a"},
	"matar":        {"kill", "se autodestruye", "mata a"},
	"patear":       {"kick", "se patea solo", "patea a"},
	"nom":          {"nom", "está comiendo", "se come a"},
	"comer":        {"nom", "está comiendo", "se come a"},
	"eat":          {"nom", "está comiendo", "se come a"},
	"angry":        {"waifu", "está enojado", "está enojado con"},
	"enojado":      {"waifu", "está enojado", "está enojado con"},
	"enojada":      {"waifu", "está enojada", "está enojada con"},
	"bleh":         {"", "saca la lengua", "le saca la lengua a"},
	"meh":          {"", "está indiferente", "le da igual"},
	"bored":        {"", "está aburrido", "se aburre con"},
	"aburrido":     {"", "está aburrido", "se aburre con"},
	"aburrida":     {"", "está aburrida", "se aburre con"},
	"coffee":       {"", "se toma un café", "le ofrece café a"},
	"cafe":         {"", "se toma un café", "le ofrece café a"},
	"dramatic":     {"", "está dramático", "le monta un drama a"},
	"drama":        {"", "está dramático", "le monta un drama a"},
	"drunk":        {"", "anda ebrio", "brinda con"},
	"ebrio":        {"", "anda ebrio", "brinda con"},
	"ebria":        {"", "anda ebria", "brinda con"},
	"cold":         {"", "tiene frío", "le da frío estar con"},
	"impregnate":   {"", "?!", "embaraza a"},
	"preg":         {"", "?!", "embaraza a"},
	"preñar":       {"", "?!", "embaraza a"},
	"embarazar":    {"", "?!", "embaraza a"},
	"laugh":        {"happy", "se ríe solo", "se burla de"},
	"reir":         {"happy", "se ríe solo", "se burla de"},
	"love":         {"kiss", "se quiere a sí misma", "está enamorado de"},
	"amor":         {"kiss", "se quiere a sí misma", "está enamorado de"},
	"pout":         {"", "hace una mueca", "le hace una mueca a"},
	"mueca":        {"", "hace una mueca", "le hace una mueca a"},
	"run":          {"", "sale corriendo", "corre tras de"},
	"correr":       {"", "sale corriendo", "corre tras de"},
	"scared":       {"", "está asustado", "le tiene miedo a"},
	"asustado":     {"", "está asustado", "le tiene miedo a"},
	"asustada":     {"", "está asustada", "le tiene miedo a"},
	"seduce":       {"", "se seduce a sí misma", "seduce a"},
	"seducir":      {"", "se seduce a sí misma", "seduce a"},
	"sleep":        {"", "se quedó dormido", "se duerme al lado de"},
	"dormir":       {"", "se quedó dormido", "se duerme al lado de"},
	"smoke":        {"", "está fumando", "fuma al lado de"},
	"fumar":        {"", "está fumando", "fuma al lado de"},
	"spit":         {"", "escupe al suelo", "escupe a"},
	"escupir":      {"", "escupe al suelo", "escupe a"},
	"step":         {"", "se pisa el pie", "pisa a"},
	"pisar":        {"", "se pisa el pie", "pisa a"},
	"think":        {"", "está pensando", "piensa en"},
	"pensar":       {"", "está pensando", "piensa en"},
	"thinkhard":    {"", "piensa intensamente", "piensa profundamente en"},
	"walk":         {"", "camina solo", "camina con"},
	"caminar":      {"", "camina solo", "camina con"},
	"bath":         {"", "se está bañando", "se baña con"},
	"bañarse":      {"", "se está bañando", "se baña con"},
	"high5":        {"highfive", "choca los cinco", "choca los cinco con"},
	"jump":         {"", "salta de alegría", "salta con"},
	"saltar":       {"", "salta de alegría", "salta con"},
	"heat":         {"", "tiene calor", "le da calor"},
	"calor":        {"", "tiene calor", "le da calor"},
	"gaming":       {"", "está jugando", "juega con"},
	"jugar":        {"", "está jugando", "juega con"},
	"draw":         {"", "está dibujando", "dibujuja a"},
	"dibujar":      {"", "está dibujando", "dibuja a"},
	"call":         {"", "se llama a sí mismo", "llama a"},
	"llamar":       {"", "se llama a sí mismo", "llama a"},
	"blowkiss":     {"kiss", "se manda un beso", "le lanza un beso a"},
	"besito":       {"kiss", "se manda un beso", "le lanza un beso a"},
	"trip":         {"", "se tropieza solo", "tropieza con"},
	"tropezar":     {"", "se tropieza solo", "tropieza con"},
	"stare":        {"", "mira al techo", "mira fijamente a"},
	"mirar":        {"", "mira al techo", "mira fijamente a"},
	"sniff":        {"", "se olfatea", "huele a"},
	"oler":         {"", "se olfatea", "huele a"},
	"curious":      {"", "está curioso", "está curioso por"},
	"curioso":      {"", "está curioso", "está curioso por"},
	"curiosa":      {"", "está curiosa", "está curiosa por"},
	"comfort":      {"cuddle", "se consuela", "consuela a"},
	"consolar":     {"cuddle", "se consuela", "consuela a"},
	"peek":         {"", "espía desde la puerta", "espía a"},
	"nope":         {"", "opone resistencia", "dice que nop a"},
	"nop":          {"", "opone resistencia", "dice que nop a"},
	"tickle":       {"", "se hace cosquillas", "le hace cosquillas a"},
	"cosquillas":   {"", "se hace cosquillas", "le hace cosquillas a"},
	"scream":       {"", "grita al aire", "le grita a"},
	"gritar":       {"", "grita al aire", "le grita a"},
	"push":         {"", "se empuja solo", "empuja a"},
	"empujar":      {"", "se empuja solo", "empuja a"},
}

// simbolosAccion del JS (kaomoji).
var simbolosAccion = []string{
	"(◠‿◕)", "˃͈◡˂͈", "૮(˶ᵔᵕᵔ˶)ა", "(づ｡◕‿‿◕｡)づ", "(✿◡‿◡)", "(✧ω◕)", "ʕ•́ᴥ•̀ʔっ",
}

func init() {
	bot.Register(bot.Command{
		Name: "hug", Aliases: clavesAcciones(), Category: bot.CatAnime,
		Desc: "Reacciones anime (hug, kiss, slap, pat, dance… 140 en total; algunos con GIF).",
		Run:  cmdAccion,
	})
	bot.Register(bot.Command{
		Name: "waifu", Aliases: []string{"neko"}, Category: bot.CatAnime,
		Desc: "Imagen aleatoria de waifu / neko (waifu.pics).",
		Run:  cmdWaifuNeko,
	})
	bot.Register(bot.Command{
		Name: "ppcouple", Aliases: []string{"ppcp"}, Category: bot.CatAnime,
		Desc: "Par de fotos de perfil a juego (dos waifus de waifu.pics).",
		Run:  cmdPareja,
	})
	bot.Register(bot.Command{
		Name: "anime", Aliases: []string{"anibusc"}, Category: bot.CatAnime,
		Desc: "Busca un anime y da su ficha (Jikan, la API pública de MyAnimeList).",
		Run:  cmdAnime,
	})
}

// clavesAcciones: registrar "hug" hace visibles las demás 140 como aliases;
// el router rompe la vinculación por nombre de tabla.
func clavesAcciones() []string {
	ks := make([]string, 0, len(acciones)-1)
	for k := range acciones {
		if k != "hug" {
			ks = append(ks, k)
		}
	}
	return ks
}

// cmdAccion: frase + (si hay slug) GIF de waifu.pics.
func cmdAccion(c *bot.Context) error {
	a, ok := acciones[c.Command]
	if !ok {
		return c.Reply("esa acción no la tengo.")
	}
	quien := c.PushName
	if quien == "" {
		quien = "@" + c.Sender.User
	}
	mencionados := c.Mentioned()
	frase := a.sola
	if len(mencionados) > 0 {
		frase = a.hacia + " @" + mencionados[0].User
	}
	caption := fmt.Sprintf("%s %s %s", kaomoji(), quien, frase)

	men := c.Mentioned()
	if a.slug == "" {
		return c.Bot.SendMentions(c.Ctx, c.Chat, caption, men, c.Priority()) // versión texto (como el JS cuando la API no la tenía)
	}
	gif, err := gifDe(c.Ctx, a.slug)
	if err != nil {
		// Si el proveedor falla, la frase no se pierde: se manda en texto.
		return c.Bot.SendMentions(c.Ctx, c.Chat, caption, men, c.Priority())
	}
	// El caption lleva el @ como texto visible; el JS solo notificaba si era
	// mención real, y aquí SendMentions lo cubre en el fallback de texto.
	return c.Bot.SendVideo(c.Ctx, c.Chat, gif, caption, c.Priority())
}

// gifDe pide el GIF de la acción a waifu.pics ({"url":…} → descarga).
func gifDe(ctx context.Context, slug string) ([]byte, error) {
	url := urlWaifuPics + "/" + slug
	body, err := getCorto(ctx, url)
	if err != nil {
		return nil, err
	}
	var r struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(body, &r) != nil || r.URL == "" {
		return nil, fmt.Errorf("waifu.pics no devolvió url")
	}
	return getCorto(ctx, r.URL)
}

func getCorto(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s → %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 12<<20))
}

// cmdWaifuNeko / cmdPareja: el .waifu .neko .ppcouple del JS con waifu.pics.
func cmdWaifuNeko(c *bot.Context) error {
	slug := "waifu"
	if c.Command == "neko" {
		slug = "neko"
	}
	img, err := gifDe(c.Ctx, slug)
	if err != nil {
		return c.Reply("El proveedor de imágenes no responde ahora; prueba en un rato.")
	}
	return c.Bot.SendImage(c.Ctx, c.Chat, img, "✧ "+slug, c.Priority())
}

func cmdPareja(c *bot.Context) error {
	a, err := gifDe(c.Ctx, "waifu")
	if err != nil {
		return c.Reply("No pude traer las fotos ahora.")
	}
	b, _ := gifDe(c.Ctx, "waifu") // si falla la segunda, se manda la una
	_ = c.Bot.SendImage(c.Ctx, c.Chat, a, "💞 Pareja (1/2)", c.Priority())
	if len(b) > 0 {
		return c.Bot.SendImage(c.Ctx, c.Chat, b, "💞 Pareja (2/2)", c.Priority())
	}
	return nil
}

// cmdAnime: ficha de Jikan (sustituto público de la API propia del JS).
func cmdAnime(c *bot.Context) error {
	q := strings.TrimSpace(c.Text)
	if q == "" {
		return c.Reply("¿Qué anime busco? Ej: *.anime frieren*")
	}
	bus, err := getCorto(c.Ctx, "https://api.jikan.moe/v4/anime?limit=3&q="+url.QueryEscape(q))
	if err != nil {
		return c.Reply("Jikan no responde ahora; prueba en un rato.")
	}
	var r struct {
		Data []struct {
			Title  string  `json:"title"`
			TitleJ string  `json:"title_japanese"`
			Score  float64 `json:"score"`
			Eps    int     `json:"episodes"`
			Year   int     `json:"year"`
			Synop  string  `json:"synopsis"`
			URL    string  `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal(bus, &r) != nil || len(r.Data) == 0 {
		return c.Reply("No aparece así en MyAnimeList; prueba otro nombre.")
	}
	a := r.Data[0]
	sin := a.Synop
	if len([]rune(sin)) > 260 {
		sin = string([]rune(sin)[:260]) + "…"
	}
	return c.Reply(fmt.Sprintf("📺 *%s* (%s)\n⭐ %.2f · %d eps · %d\n%s\n%s",
		a.Title, a.TitleJ, a.Score, a.Eps, a.Year, sin, a.URL))
}

func kaomoji() string { return simbolosAccion[int(time.Now().UnixNano())%len(simbolosAccion)] }
