// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// buscadores.go — lo que quedaba de downloads/ del JS: buscar bajar y traer
// ficheros por enlace. Reglas honestas del bloque:
//
//	.imagen/.img → Openverse (API pública CC, sin clave) — NO Google Images,
//	               que hoy exige JS y cookies y mentiría.
//	.pinterest   → mismo motor, se te dice que no es Pinterest de verdad.
//	.mediafire   → página de descarga: raspa el botón «Download» (el JS lo
//	               hacía igual) y baja el archivo con tope.
//	.gdrive      → reescribe el enlace a descarga directa de Drive (uc?id=).
//	.apk         → la banca de Aptoide, que tiene API JSON pública.
//	.deezer/.itunes/.spotify → buscan con el motor de música (B6) — solo el
//	               catálogo que sí deja bajar.
//	.ytsearch    → Piped (instancia configurable). Si esa instancia cae o
//	               cambia, el error lo dice.
package cmds

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"shin-go/internal/bot"
	"shin-go/internal/musica"
)

func init() {
	bot.Register(bot.Command{
		Name: "imagen", Aliases: []string{"img", "image"}, Category: bot.CatDownload,
		Desc: "Busca una imagen en Openverse (API pública Creative Commons): .imagen <qué>.",
		Run:  cmdImagen,
	})
	bot.Register(bot.Command{
		Name: "pinterest", Category: bot.CatDownload,
		Desc: "Busca imágenes por estética (motor Openverse, no el Pinterest cerrado).",
		Run:  cmdImagen,
	})
	bot.Register(bot.Command{
		Name: "mediafire", Aliases: []string{"mf"}, Category: bot.CatDownload,
		Desc: "Baja de un enlace de MediaFire: .mediafire <url>.",
		Run:  cmdMediafire,
	})
	bot.Register(bot.Command{
		Name: "gdrive", Aliases: []string{"drive"}, Category: bot.CatDownload,
		Desc: "Baja de un enlace de Google Drive: .gdrive <url>.",
		Run:  cmdGdrive,
	})
	bot.Register(bot.Command{
		Name: "apk", Aliases: []string{"aptoide", "apkdl"}, Category: bot.CatDownload,
		Desc: "Busca la APK de una app en Aptoide: .apk <nombre>.",
		Run:  cmdApk,
	})
	bot.Register(bot.Command{
		Name: "deezer", Aliases: []string{"dzr", "deezermusic"}, Category: bot.CatDownload,
		Desc: "Busca en Deezer: la ficha y el preview de 30 s.",
		Run:  func(c *bot.Context) error { return cmdMusicaFuente(c, (*musica.Cliente).BuscarDeezer) },
	})
	bot.Register(bot.Command{
		Name: "ytsearch", Aliases: []string{"yts", "search"}, Category: bot.CatDownload,
		Desc: "Busca en YouTube los títulos (Piped; para bajar, .yt <url>).",
		Run:  cmdYtSearch,
	})
}

// ─── imágenes (Openverse) ──────────────────────────────────────────────────

// urlOpenverse es variable para las pruebas (httptest).
var urlOpenverse = "https://api.openverse.org/v1/images/"

type openverseResp struct {
	Results []struct {
		URL     string `json:"url"`
		Title   string `json:"title"`
		Foreign string `json:"foreign_landing_url"`
		License string `json:"license"`
		Creator string `json:"creator"`
	} `json:"results"`
}

func cmdImagen(c *bot.Context) error {
	q := strings.TrimSpace(c.Text)
	if q == "" {
		return c.Reply("¿Qué busco? Ej: *.imagen gatitos*")
	}
	u := urlOpenverse + "?q=" + url.QueryEscape(q) + "&page_size=5"
	datos, err := getCorto(c.Ctx, u)
	if err != nil {
		return c.Reply("El buscador de imágenes no responde; prueba en un rato.")
	}
	var r openverseResp
	if json.Unmarshal(datos, &r) != nil || len(r.Results) == 0 {
		return c.Reply("No encuentro nada con esa palabra; prueba más corta o en inglés.")
	}
	img := r.Results[0]
	pie := fmt.Sprintf("🖼 %s\n© %s (%s)\n%s", cerca(img.Title, 80), orDefault2(img.Creator, "·"), img.License, img.Foreign)
	bytes2, err := getCorto(c.Ctx, img.URL)
	if err == nil && len(bytes2) > 200 && len(bytes2) < 20<<20 {
		return c.Bot.SendImage(c.Ctx, c.Chat, bytes2, pie, c.Priority())
	}
	return c.Reply(pie) // si no se pudo bajar, al menos la ficha
}

func orDefault2(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

// ─── mediafire / gdrive: bajar el enlace de verdad ─────────────────────────

// rxBotonMediafire: el botón de descarga de la página. El JS lo raspaba igual
// porque no hay API pública; esto mientras dure el <a id="downloadButton">.
var rxBotonMediafire = regexp.MustCompile(`<a[^>]+id="downloadButton"[^>]+href="([^"]+)"`)

const maxDescargaDirecta = 100 << 20 // mismo tope que los vídeos

func cmdMediafire(c *bot.Context) error {
	enlace := strings.TrimSpace(c.Text)
	if !strings.Contains(enlace, "mediafire.com") {
		return c.Reply("Esperaba un enlace de MediaFire: https://www.mediafire.com/file/…")
	}
	pagina, err := getCorto(c.Ctx, enlace)
	if err != nil {
		return c.Reply("no pude abrir la página de MediaFire.")
	}
	m := rxBotonMediafire.FindSubmatch(pagina)
	if len(m) < 2 {
		return c.Reply("la página no enseñaba el botón de descarga (¿será carpeta o está caído?).")
	}
	return descargarDirecto(c, html.UnescapeString(string(m[1])), "archivo de MediaFire")
}

// idDrive extrae el ID de cualquiera de los formatos de Drive.
var rxDrive = regexp.MustCompile(`(?:/d/|id=|/file/d/)([a-zA-Z0-9_-]{20,})`)

func cmdGdrive(c *bot.Context) error {
	enlace := strings.TrimSpace(c.Text)
	m := rxDrive.FindStringSubmatch(enlace)
	if len(m) < 2 {
		return c.Reply("Esperaba el enlace de un archivo de Drive (…/file/d/ID o …?id=ID).")
	}
	return descargarDirecto(c,
		"https://drive.usercontent.google.com/download?id="+m[1]+"&export=download&confirm=t",
		"archivo de Google Drive")
}

// descargarDirecto: GET con tope y mandar como documento, como el JS.
func descargarDirecto(c *bot.Context, u, etiqueta string) error {
	ctx, cancel := context.WithTimeout(c.Ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return c.Reply("el enlace está mal formado.")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Shin-MD-Go)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return c.Reply("no pude bajar " + etiqueta + ".")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.Reply(fmt.Sprintf("la descarga respondió %d; si pide confirmación de virus o es demasiado grande, Drive/MediaFire lo cortan ellos.", resp.StatusCode))
	}
	if resp.ContentLength > maxDescargaDirecta {
		return c.Reply(fmt.Sprintf("pesa %d MB y el tope de WhatsApp son %d MB; no lo gasto.", resp.ContentLength>>20, maxDescargaDirecta>>20))
	}
	datos, err := io.ReadAll(io.LimitReader(resp.Body, maxDescargaDirecta+1))
	if err != nil || len(datos) == 0 {
		return c.Reply("la descarga llegó vacía.")
	}
	if len(datos) > maxDescargaDirecta {
		return c.Reply(fmt.Sprintf("supera los %d MB de tope; no lo gasto.", maxDescargaDirecta>>20))
	}
	nombre := etiqueta
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		if a := strings.Split(cd, "filename="); len(a) > 1 {
			nombre = strings.Trim(strings.TrimSpace(a[1]), `"`)
		}
	}
	return c.Bot.SendDocument(c.Ctx, c.Chat, datos, nombre, "", c.Priority())
}

// ─── apk (Aptoide tiene API JSON pública) ─────────────────────────────────

var urlAptoide = "https://webservices.aptoide.com/apt/7/apksearch"

type aptoideResp struct {
	Datalist struct {
		List []struct {
			Name string `json:"name"`
			Size int64  `json:"size"`
			File struct {
				Path string `json:"path"`
			} `json:"file"`
			Store struct {
				Name string `json:"name"`
			} `json:"store"`
		} `json:"list"`
	} `json:"datalist"`
}

func cmdApk(c *bot.Context) error {
	q := strings.TrimSpace(c.Text)
	if q == "" {
		return c.Reply("¿Qué app busco? Ej: *.apk whatsapp business*")
	}
	u := urlAptoide + "?query=" + url.QueryEscape(q) + "&limit=5"
	datos, err := getCorto(c.Ctx, u)
	if err != nil {
		return c.Reply("Aptoide no responde ahora.")
	}
	var r aptoideResp
	if json.Unmarshal(datos, &r) != nil || len(r.Datalist.List) == 0 {
		return c.Reply("no aparece esa app en Aptoide.")
	}
	ap := r.Datalist.List[0]
	return c.Reply(fmt.Sprintf("📦 *%s*\n🏪 %s · %.1f MB\n⬇ %s\n\n_Si la quieres por el bot, dime y te paso el archivo (tope 100 MB)._",
		ap.Name, ap.Store.Name, float64(ap.Size)/1048576.0, ap.File.Path))
}

// ─── música por buscador (B6 reutilizado) ───────────────────────────────────

// cmdMusicaFuente: .deezer y hermanos usan el mismo motor de música.
// f es un *método* del motor ((*musica.Cliente).BuscarDeezer …) como función-método.
func cmdMusicaFuente(c *bot.Context, f func(*musica.Cliente, context.Context, string, int) ([]musica.Pista, error)) error {
	q := strings.TrimSpace(c.Text)
	if q == "" {
		return c.Reply("¿Qué busco ahí? Ej: *." + c.Command + " daft punk*")
	}
	pistas, err := f(clienteMusica(), c.Ctx, q, 5)
	if err != nil {
		return c.Reply("esa fuente no responde ahora (" + cerca(err.Error(), 80) + ").")
	}
	if len(pistas) == 0 {
		return c.Reply("no aparece ahí; prueba *.play*, que busca en más sitios.")
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎵 *%d en %s*\n\n", len(pistas), c.Command)
	for i, p := range pistas {
		lnk := "solo ficha"
		if p.Preview != "" {
			lnk = "preview 30 s: " + p.Preview
		}
		fmt.Fprintf(&sb, "%d. *%s* — %s\n   %s\n", i+1, p.Titulo, p.Artista, lnk)
	}
	return c.Reply(sb.String())
}

// ─── ytsearch (Piped, honesto) ──────────────────────────────────────────────

// urlPiped es la instancia Piped para .ytsearch. Las instancias públicas se
// caen y se rotan; quedará en config si hace falta moverla sin recompilar.
var urlPiped = "https://pipedapi.kavin.rocks"

func cmdYtSearch(c *bot.Context) error {
	q := strings.TrimSpace(c.Text)
	if q == "" {
		return c.Reply("¿Qué busco en YouTube? Ej: *.ytsearch lofi*")
	}
	u := urlPiped + "/search?q=" + url.QueryEscape(q) + "&filter=videos"
	datos, err := getCorto(c.Ctx, u)
	if err != nil {
		return c.Reply("El buscador de YouTube (Piped) no responde ahora; con *.yt <url>* puedes bajar uno que ya tengas.")
	}
	var r []struct {
		Title    string `json:"title"`
		Url      string `json:"url"`
		Uploader string `json:"uploaderName"`
		Duration int64  `json:"duration"`
	}
	if json.Unmarshal(datos, &r) != nil || len(r) == 0 {
		return c.Reply("no salió nada; puede que la instancia Piped esté caída hoy.")
	}
	var sb strings.Builder
	sb.WriteString("📺 *YouTube — " + q + "*\n\n")
	for i, v := range r[:min(5, len(r))] {
		if v.Duration > 0 {
			fmt.Fprintf(&sb, "%d. *%s* (%dm%02ds)\n   👤 %s\n   https://youtu.be%s\n\n", i+1, v.Title, v.Duration/60, v.Duration%60, v.Uploader, v.Url)
		}
	}
	sb.WriteString("_Para bajar: .yt <enlace> · para música libre: .play <nombre>_")
	return c.Reply(strings.TrimRight(sb.String(), "\n"))
}
