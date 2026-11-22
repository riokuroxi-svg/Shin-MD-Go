// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// nsfw.go — la parte del JS explícita CON su candado: los boorus
// (Danbooru y Rule34 con JSON público) detrás de «nsfw off» por
// defecto y solo en grupos donde un admin lo enciende.
//
// Qué NO se porta y por qué (deliberado):
//   - Las 44 reacciones NSFW del JS (incluida una «violación»). Contenido de
//     agresión no se porta; el texto de ayuda lo dice.
//   - xvideos/xnxx: scraping porno de por sí; sin una API pública fiable no
//     hay versión que no mienta.
package cmds

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"shin-go/internal/bot"
)

func init() {
	bot.Register(bot.Command{
		Name: "nsfw", Category: bot.CatGroup, GroupOnly: true,
		Desc: "Activa o apaga el contenido +18 del grupo: .nsfw on|off (admin).",
		Run: func(c *bot.Context) error {
			return cmdToggleGrupo(c, "nsfw", "contenido +18")
		},
	})
	bot.Register(bot.Command{
		Name: "danbooru", Aliases: []string{"dbooru"}, Category: bot.CatNSFW,
		Desc: "Busca en Danbooru (requiere .nsfw on): .danbooru <tags>.",
		Run:  func(c *bot.Context) error { return cmdBooru(c, "danbooru") },
	})
	bot.Register(bot.Command{
		Name: "rule34", Aliases: []string{"r34", "rule"}, Category: bot.CatNSFW,
		Desc: "Busca en Rule34 (requiere .nsfw on): .rule34 <tags>.",
		Run:  func(c *bot.Context) error { return cmdBooru(c, "rule34") },
	})
}

// urlBooru: variables para pruebas.
var (
	urlDanbooru = "https://danbooru.donmai.us"
	urlRule34   = "https://api.rule34.xxx"
)

// nsfwPermitido: el candado. Fuera de grupo o sin «nsfw on», no se sirve.
func nsfwPermitido(c *bot.Context) (bool, error) {
	if !c.IsGroup {
		return false, nil
	}
	return c.DB.Flag(c.Ctx, c.Chat.String(), "nsfw", false)
}

// cmdBooru consulta el JSON del booru con tags saneados; límite 1 resultado
// con nota de origen (así no finge ser un catálogo propio).
func cmdBooru(c *bot.Context, cual string) error {
	ok, err := nsfwPermitido(c)
	if err != nil {
		return err
	}
	if !ok {
		return c.Reply("🔞 Este comando solo funciona en grupos con *nsfw on* (lo enciende un admin).")
	}
	tags := strings.TrimSpace(c.Text)
	if tags == "" {
		return c.Reply("¿Con qué etiqueta? Ej: *." + c.Command + " 1girl*")
	}
	seguro, etiqueta := limpiaTagsBooru(tags)
	if !seguro {
		return c.Reply("Etiquetas de verdad, sin espacios raros ni ceros delante.")
	}

	var imgURL, origen string
	switch cual {
	case "danbooru":
		base, parseErr := url.Parse(urlDanbooru)
		if parseErr != nil || base.Hostname() == "" {
			return c.Reply("Danbooru está mal configurado.")
		}
		datos, err := getCortoLimite(c.Ctx, urlDanbooru+"/posts.json?limit=3&tags="+url.QueryEscape(etiqueta), 12<<20, base.Hostname())
		if err != nil {
			return c.Reply("Danbooru no responde ahora.")
		}
		var r []struct {
			FileURL string `json:"file_url"`
			Preview string `json:"large_file_url"`
		}
		if json.Unmarshal(datos, &r) != nil || len(r) == 0 {
			return c.Reply("Sin resultados en Danbooru con eso.")
		}
		imgURL = r[0].FileURL
		if imgURL == "" {
			imgURL = r[0].Preview
		}
		origen = "danbooru.donmai.us"
	case "rule34":
		origen = "api.rule34.xxx"
		base, parseErr := url.Parse(urlRule34)
		if parseErr != nil || base.Hostname() == "" {
			return c.Reply("Rule34 está mal configurado.")
		}
		datos, err := getCortoLimite(c.Ctx, urlRule34+"/index.php?page=dapi&s=post&q=index&json=1&limit=3&tags="+url.QueryEscape(etiqueta), 12<<20, base.Hostname())
		if err != nil {
			return c.Reply(origen + " no responde ahora.")
		}
		var r []struct {
			FileURL   string `json:"file_url"`
			Preview   string `json:"preview_url"`
			SampleURL string `json:"sample_url"`
		}
		if json.Unmarshal(datos, &r) != nil || len(r) == 0 {
			return c.Reply("Sin resultados en " + origen + " con eso.")
		}
		imgURL = r[0].FileURL
		if imgURL == "" {
			imgURL = r[0].SampleURL
		}
		origen = "https://" + origen
	}
	if imgURL == "" {
		return c.Reply("encontrado, pero sin archivo servible.")
	}
	if !strings.HasPrefix(imgURL, "http") {
		imgURL = "https:" + imgURL
	}
	img, err := getCorto(c.Ctx, imgURL)
	if err != nil || len(img) < 100 {
		return c.Reply("no pude traer la imagen (" + etiqueta + " en " + cual + ").")
	}
	return c.Bot.SendImage(c.Ctx, c.Chat, img, fmt.Sprintf("🔞 *%s* — %s\norigen: %s", c.Command, etiqueta, origen), c.Priority())
}

// limpiaTagsBooru: letras, números, _-() y espacios → _. Nada de «?» ni «&»
// (nunca se construye una URL con ellos).
func limpiaTagsBooru(s string) (bool, string) {
	s = strings.Join(strings.Fields(s), "_")
	if s == "" || len(s) > 120 {
		return false, ""
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '-', r == '(', r == ')', r == '.':
		default:
			return false, ""
		}
	}
	return true, s
}
