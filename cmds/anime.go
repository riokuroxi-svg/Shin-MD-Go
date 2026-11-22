// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// anime.go — las reacciones anime del JS (cmds/anime/shares.js: 141 nombres)
// más .anime/.waifu/.neko/.ppcouple de la misma carpeta.
//
// El JS las servía desde su API propia (Ginko, con clave: no exportable).
// waifu.pics dejó de responder en 2026, por lo que las reacciones usan ahora
// **nekos.best** (público, sin clave). Sus GIF se convierten a MP4 local y se
// envían con gifPlayback=true: subir image/gif fingiendo video/mp4 era la causa
// de que `.pat` terminara mostrando solo el texto.
//
// Cada acción lleva dos frases: sola (sin mención) y hacia alguien, como el
// JS ("está abrazando a …" / "se abraza a sí misma").
package cmds

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"shin-go/internal/bot"
	"shin-go/internal/media"
	"shin-go/internal/utilidades"
)

// urlNekosBest es variable para que las pruebas lo apunten a httptest.
var urlNekosBest = "https://nekos.best/api/v2"

// accion describe una reacción: slugs de nekos.best (o "" = sin GIF) y las
// frases sola / hacia otra persona. El segundo campo es el alias o nombre
// principal en español cuando existe.
type accion struct {
	slug  string // endpoint en nekos.best; vacío si el catálogo no la sirve
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
	"punch":        {"punch", "se golpea solo", "le pega un puñetazo a"},
	"golpear":      {"punch", "se golpea solo", "le pega un puñetazo a"},
	"bonk":         {"bonk", "se da con el bat en la cabeza", "le da un bonk a"},
	"golpe":        {"bonk", "se da con el bat en la cabeza", "le da un bonk a"},
	"bite":         {"bite", "se muerde el brazo", "le muerde a"},
	"morder":       {"bite", "se muerde el brazo", "le muerde a"},
	"lick":         {"", "se lame la mano", "le lame a"},
	"lamer":        {"", "se lame la mano", "le lame a"},
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
	"cringe":       {"facepalm", "siente vergüenza ajena", "le da vergüenza ajena"},
	"avergonzarse": {"facepalm", "siente vergüenza ajena", "le da vergüenza ajena"},
	"asco":         {"facepalm", "siente asco", "le da asco"},
	"bully":        {"punch", "se molesta a sí mismo", "está molestando a"},
	"molestar":     {"punch", "se molesta a sí mismo", "está molestando a"},
	"kill":         {"shoot", "se autodestruye", "mata a"},
	"matar":        {"shoot", "se autodestruye", "mata a"},
	"patear":       {"kick", "se patea solo", "patea a"},
	"nom":          {"nom", "está comiendo", "se come a"},
	"comer":        {"nom", "está comiendo", "se come a"},
	"eat":          {"nom", "está comiendo", "se come a"},
	"angry":        {"angry", "está enojado", "está enojado con"},
	"enojado":      {"angry", "está enojado", "está enojado con"},
	"enojada":      {"angry", "está enojada", "está enojada con"},
	"bleh":         {"bleh", "saca la lengua", "le saca la lengua a"},
	"meh":          {"shrug", "está indiferente", "le da igual"},
	"bored":        {"bored", "está aburrido", "se aburre con"},
	"aburrido":     {"bored", "está aburrido", "se aburre con"},
	"aburrida":     {"bored", "está aburrida", "se aburre con"},
	"coffee":       {"sip", "se toma un café", "le ofrece café a"},
	"cafe":         {"sip", "se toma un café", "le ofrece café a"},
	"dramatic":     {"", "está dramático", "le monta un drama a"},
	"drama":        {"", "está dramático", "le monta un drama a"},
	"drunk":        {"", "anda ebrio", "brinda con"},
	"ebrio":        {"", "anda ebrio", "brinda con"},
	"ebria":        {"", "anda ebria", "brinda con"},
	"cold":         {"shake", "tiene frío", "le da frío estar con"},
	"impregnate":   {"", "?!", "embaraza a"},
	"preg":         {"", "?!", "embaraza a"},
	"preñar":       {"", "?!", "embaraza a"},
	"embarazar":    {"", "?!", "embaraza a"},
	"laugh":        {"laugh", "se ríe solo", "se burla de"},
	"reir":         {"laugh", "se ríe solo", "se burla de"},
	"love":         {"kiss", "se quiere a sí misma", "está enamorado de"},
	"amor":         {"kiss", "se quiere a sí misma", "está enamorado de"},
	"pout":         {"pout", "hace una mueca", "le hace una mueca a"},
	"mueca":        {"pout", "hace una mueca", "le hace una mueca a"},
	"run":          {"run", "sale corriendo", "corre tras de"},
	"correr":       {"run", "sale corriendo", "corre tras de"},
	"scared":       {"shocked", "está asustado", "le tiene miedo a"},
	"asustado":     {"shocked", "está asustado", "le tiene miedo a"},
	"asustada":     {"shocked", "está asustada", "le tiene miedo a"},
	"seduce":       {"", "se seduce a sí misma", "seduce a"},
	"seducir":      {"", "se seduce a sí misma", "seduce a"},
	"sleep":        {"sleep", "se quedó dormido", "se duerme al lado de"},
	"dormir":       {"sleep", "se quedó dormido", "se duerme al lado de"},
	"smoke":        {"", "está fumando", "fuma al lado de"},
	"fumar":        {"", "está fumando", "fuma al lado de"},
	"spit":         {"", "escupe al suelo", "escupe a"},
	"escupir":      {"", "escupe al suelo", "escupe a"},
	"step":         {"", "se pisa el pie", "pisa a"},
	"pisar":        {"", "se pisa el pie", "pisa a"},
	"think":        {"think", "está pensando", "piensa en"},
	"pensar":       {"think", "está pensando", "piensa en"},
	"thinkhard":    {"think", "piensa intensamente", "piensa profundamente en"},
	"walk":         {"run", "camina solo", "camina con"},
	"caminar":      {"run", "camina solo", "camina con"},
	"bath":         {"", "se está bañando", "se baña con"},
	"bañarse":      {"", "se está bañando", "se baña con"},
	"high5":        {"highfive", "choca los cinco", "choca los cinco con"},
	"jump":         {"happy", "salta de alegría", "salta con"},
	"saltar":       {"happy", "salta de alegría", "salta con"},
	"heat":         {"shake", "tiene calor", "le da calor"},
	"calor":        {"shake", "tiene calor", "le da calor"},
	"gaming":       {"", "está jugando", "juega con"},
	"jugar":        {"", "está jugando", "juega con"},
	"draw":         {"", "está dibujando", "dibujuja a"},
	"dibujar":      {"", "está dibujando", "dibuja a"},
	"call":         {"", "se llama a sí mismo", "llama a"},
	"llamar":       {"", "se llama a sí mismo", "llama a"},
	"blowkiss":     {"blowkiss", "se manda un beso", "le lanza un beso a"},
	"besito":       {"blowkiss", "se manda un beso", "le lanza un beso a"},
	"trip":         {"facepalm", "se tropieza solo", "tropieza con"},
	"tropezar":     {"facepalm", "se tropieza solo", "tropieza con"},
	"stare":        {"stare", "mira al techo", "mira fijamente a"},
	"mirar":        {"stare", "mira al techo", "mira fijamente a"},
	"sniff":        {"confused", "se olfatea", "huele a"},
	"oler":         {"confused", "se olfatea", "huele a"},
	"curious":      {"confused", "está curioso", "está curioso por"},
	"curioso":      {"confused", "está curioso", "está curioso por"},
	"curiosa":      {"confused", "está curiosa", "está curiosa por"},
	"comfort":      {"cuddle", "se consuela", "consuela a"},
	"consolar":     {"cuddle", "se consuela", "consuela a"},
	"peek":         {"lurk", "espía desde la puerta", "espía a"},
	"nope":         {"nope", "opone resistencia", "dice que nop a"},
	"nop":          {"nope", "opone resistencia", "dice que nop a"},
	"tickle":       {"tickle", "se hace cosquillas", "le hace cosquillas a"},
	"cosquillas":   {"tickle", "se hace cosquillas", "le hace cosquillas a"},
	"scream":       {"shocked", "grita al aire", "le grita a"},
	"gritar":       {"shocked", "grita al aire", "le grita a"},
	"push":         {"yeet", "se empuja solo", "empuja a"},
	"empujar":      {"yeet", "se empuja solo", "empuja a"},
}

// simbolosAccion del JS (kaomoji).
var simbolosAccion = []string{
	"(◠‿◕)", "˃͈◡˂͈", "૮(˶ᵔᵕᵔ˶)ა", "(づ｡◕‿‿◕｡)づ", "(✿◡‿◡)", "(✧ω◕)", "ʕ•́ᴥ•̀ʔっ",
}

func init() {
	bot.Register(bot.Command{
		Name: "hug", Aliases: clavesAcciones(), Category: bot.CatAnime,
		Desc: "Reacciones anime (hug, kiss, slap, pat, dance…); GIF cuando nekos.best tiene la acción.",
		Run:  cmdAccion,
	})
	bot.Register(bot.Command{
		Name: "waifu", Aliases: []string{"neko"}, Category: bot.CatAnime,
		Desc: "Imagen aleatoria de waifu / neko (nekos.best).",
		Run:  cmdWaifuNeko,
	})
	bot.Register(bot.Command{
		Name: "ppcouple", Aliases: []string{"ppcp"}, Category: bot.CatAnime,
		Desc: "Par de fotos de perfil a juego (dos imágenes de nekos.best).",
		Run:  cmdPareja,
	})
	bot.Register(bot.Command{
		Name: "anime", Aliases: []string{"anibusc"}, Category: bot.CatAnime,
		Desc: "Busca un anime y muestra ficha, portada y enlace de AniList.",
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
	sort.Strings(ks)
	return ks
}

// cmdAccion: frase + (si hay slug) GIF de nekos.best.
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
		return c.Bot.SendMentions(c.Ctx, c.Chat, caption, men, c.Priority())
	}
	gif, err := gifDe(c.Ctx, a.slug)
	if err != nil {
		// El fallo ya no se disfraza como comportamiento normal: la frase se
		// conserva, pero explica por qué no llegó la animación.
		return c.Bot.SendMentions(c.Ctx, c.Chat,
			caption+"\n\n_(No pude cargar el GIF del proveedor; inténtalo de nuevo.)_", men, c.Priority())
	}
	mp4, err := media.GIFAVideo(c.Ctx, gif)
	if err == nil && len(mp4) <= 16<<20 {
		return c.Bot.SendGIF(c.Ctx, c.Chat, mp4, caption, c.Priority())
	}
	// Sin ffmpeg se conserva el GIF como imagen en vez de mentir con MIME
	// video/mp4. Algunos clientes lo animan y otros muestran el primer cuadro.
	if err != nil && !errors.Is(err, media.ErrSinFFmpeg) && c.Log != nil {
		c.Log.Debug("reacción GIF: " + err.Error())
	}
	return c.Bot.SendImage(c.Ctx, c.Chat, gif, caption, c.Priority())
}

// gifDe pide una imagen o GIF a nekos.best y descarga el archivo devuelto.
func gifDe(ctx context.Context, slug string) ([]byte, error) {
	base, err := url.Parse(urlNekosBest)
	if err != nil || base.Hostname() == "" || base.User != nil ||
		(base.Scheme != "https" && base.Scheme != "http") {
		return nil, fmt.Errorf("URL base de nekos.best inválida")
	}
	metaURL := strings.TrimRight(urlNekosBest, "/") + "/" + url.PathEscape(slug)
	body, err := getCortoLimite(ctx, metaURL, 512<<10, base.Hostname())
	if err != nil {
		return nil, err
	}
	var r struct {
		Results []struct {
			URL string `json:"url"`
		} `json:"results"`
	}
	if json.Unmarshal(body, &r) != nil || len(r.Results) == 0 || r.Results[0].URL == "" {
		return nil, fmt.Errorf("nekos.best no devolvió url")
	}
	mediaURL, mediaErr := url.Parse(r.Results[0].URL)
	if mediaErr != nil || mediaURL.User != nil || !strings.EqualFold(mediaURL.Host, base.Host) ||
		!strings.EqualFold(mediaURL.Scheme, base.Scheme) {
		return nil, fmt.Errorf("nekos.best devolvió una URL de medio no confiable")
	}
	data, err := getCortoLimite(ctx, mediaURL.String(), 12<<20, base.Hostname())
	if err != nil {
		return nil, err
	}
	mime := strings.SplitN(http.DetectContentType(data), ";", 2)[0]
	switch mime {
	case "image/gif", "image/jpeg", "image/png", "image/webp":
		return data, nil
	default:
		return nil, fmt.Errorf("nekos.best devolvió un medio inválido: %s", mime)
	}
}

func getCorto(ctx context.Context, rawURL string) ([]byte, error) {
	return getCortoLimite(ctx, rawURL, 12<<20)
}

func getCortoLimite(ctx context.Context, rawURL string, max int64, sameHost ...string) ([]byte, error) {
	if max <= 0 {
		return nil, fmt.Errorf("límite de descarga inválido")
	}
	hostFijo := ""
	if len(sameHost) > 0 {
		hostFijo = strings.TrimSpace(sameHost[0])
	}
	autoridadFija := ""
	validar := func(raw string) (*url.URL, error) {
		if hostFijo == "" {
			return utilidades.ValidarURL(raw, utilidades.ResolverDNSIPs)
		}
		u, err := url.Parse(raw)
		if err != nil || u.User != nil || !strings.EqualFold(u.Hostname(), hostFijo) ||
			(autoridadFija != "" && !strings.EqualFold(u.Host, autoridadFija)) ||
			(u.Scheme != "https" && u.Scheme != "http") {
			return nil, fmt.Errorf("URL fuera del proveedor")
		}
		return u, nil
	}
	u, err := validar(rawURL)
	if hostFijo != "" && err == nil {
		autoridadFija = u.Host
	}
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Shin-MD-Go/0.27")
	client := *http.DefaultClient
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("demasiadas redirecciones")
		}
		if _, err := validar(next.URL.String()); err != nil {
			return fmt.Errorf("redirección rechazada: %w", err)
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s → %d", req.URL.Hostname(), resp.StatusCode)
	}
	if resp.ContentLength > max {
		return nil, fmt.Errorf("%s devolvió más de %d MB", req.URL.Hostname(), max>>20)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%s devolvió más de %d MB", req.URL.Hostname(), max>>20)
	}
	return data, nil
}

func postJSONCorto(ctx context.Context, rawURL string, payload any, max int64) ([]byte, error) {
	endpoint, err := utilidades.ValidarURL(rawURL, utilidades.ResolverDNSIPs)
	if err != nil || endpoint.Scheme != "https" {
		return nil, fmt.Errorf("endpoint JSON no confiable")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Shin-MD-Go/0.27")
	client := *http.DefaultClient
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 || next.URL.Scheme != "https" ||
			!strings.EqualFold(next.URL.Hostname(), endpoint.Hostname()) {
			return fmt.Errorf("redirección JSON rechazada")
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s → %d", req.URL.Hostname(), resp.StatusCode)
	}
	if resp.ContentLength > max {
		return nil, fmt.Errorf("%s devolvió una respuesta demasiado grande", req.URL.Hostname())
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%s devolvió una respuesta demasiado grande", req.URL.Hostname())
	}
	return data, nil
}

// cmdWaifuNeko / cmdPareja: el .waifu .neko .ppcouple del JS con nekos.best.
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

var etiquetasHTMLAnime = regexp.MustCompile(`<[^>]+>`)

func urlHTTPSDe(raw string, hosts ...string) (*url.URL, bool) {
	if len(raw) == 0 || len(raw) > 2048 {
		return nil, false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || u.Port() != "" {
		return nil, false
	}
	for _, host := range hosts {
		if strings.EqualFold(u.Hostname(), host) {
			return u, true
		}
	}
	return nil, false
}

// cmdAnime usa AniList GraphQL. Jikan quedó inaccesible desde varias redes en
// la auditoría de noviembre de 2026; AniList ofrece la misma ficha y portada
// sin clave, con un contrato estable.
func cmdAnime(c *bot.Context) error {
	q := strings.TrimSpace(c.Text)
	if q == "" {
		return c.Reply("¿Qué anime busco? Ej: *.anime frieren*")
	}
	payload := map[string]any{
		"query": `query ($search: String) {
			Media(search: $search, type: ANIME) {
				title { romaji native }
				episodes startDate { year } averageScore description siteUrl
				coverImage { large }
			}
		}`,
		"variables": map[string]string{"search": q},
	}
	bus, err := postJSONCorto(c.Ctx, "https://graphql.anilist.co", payload, 2<<20)
	if err != nil {
		return c.Reply("AniList no responde ahora; prueba en un rato.")
	}
	var r struct {
		Data struct {
			Media *struct {
				Title struct {
					Romaji string `json:"romaji"`
					Native string `json:"native"`
				} `json:"title"`
				Episodes     int    `json:"episodes"`
				AverageScore int    `json:"averageScore"`
				Description  string `json:"description"`
				SiteURL      string `json:"siteUrl"`
				StartDate    struct {
					Year int `json:"year"`
				} `json:"startDate"`
				CoverImage struct {
					Large string `json:"large"`
				} `json:"coverImage"`
			} `json:"Media"`
		} `json:"data"`
	}
	if json.Unmarshal(bus, &r) != nil || r.Data.Media == nil || r.Data.Media.Title.Romaji == "" {
		return c.Reply("No aparece así en AniList; prueba otro nombre.")
	}
	a := r.Data.Media
	synopsis := html.UnescapeString(etiquetasHTMLAnime.ReplaceAllString(a.Description, " "))
	synopsis = strings.Join(strings.Fields(synopsis), " ")
	if len([]rune(synopsis)) > 300 {
		synopsis = string([]rune(synopsis)[:299]) + "…"
	}
	body := fmt.Sprintf("📺 *%s* (%s)\n⭐ %.1f/10 · %d eps · %d\n\n%s",
		bot.Esc(a.Title.Romaji), bot.Esc(a.Title.Native), float64(a.AverageScore)/10,
		a.Episodes, a.StartDate.Year, bot.Esc(synopsis))
	buttons := make([]bot.Button, 0, 1)
	if site, ok := urlHTTPSDe(a.SiteURL, "anilist.co", "www.anilist.co"); ok {
		buttons = append(buttons, bot.CTAURL("🌐 Abrir en AniList", site.String()))
	}
	if coverURL, ok := urlHTTPSDe(a.CoverImage.Large, "s4.anilist.co"); ok {
		if cover, err := getCortoLimite(c.Ctx, coverURL.String(), 8<<20, coverURL.Hostname()); err == nil {
			return c.ReplyImage(cover, "Anime · AniList", body, "Datos y portada: AniList", buttons...)
		}
	}
	if len(buttons) > 0 {
		return c.ReplyButtons("Anime · AniList", body, "Datos: AniList", buttons...)
	}
	return c.Reply(body)
}

func kaomoji() string { return simbolosAccion[int(time.Now().UnixNano())%len(simbolosAccion)] }
