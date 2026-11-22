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
//	.play/.escucha → motor de música con fallbacks; el acceso directo a Deezer
//	               se retiró porque su API pública responde 403 en esta red.
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
	"shin-go/internal/utilidades"
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
	base, err := url.Parse(urlOpenverse)
	if err != nil || base.Hostname() == "" {
		return c.Reply("El buscador de imágenes está mal configurado.")
	}
	u := urlOpenverse + "?q=" + url.QueryEscape(q) + "&page_size=5"
	datos, err := getCortoLimite(c.Ctx, u, 12<<20, base.Hostname())
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
	if !mediafireURLSegura(enlace) {
		return c.Reply("Esperaba un enlace HTTPS de MediaFire: https://www.mediafire.com/file/…")
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

func mediafireURLSegura(raw string) bool {
	if len(raw) == 0 || len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	return host == "mediafire.com" || strings.HasSuffix(host, ".mediafire.com")
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
func descargarDirecto(c *bot.Context, rawURL, etiqueta string) error {
	u, err := utilidades.ValidarURL(rawURL, utilidades.ResolverDNSIPs)
	if err != nil || u.Scheme != "https" {
		return c.Reply("el enlace de descarga no es HTTPS público o está mal formado.")
	}
	ctx, cancel := context.WithTimeout(c.Ctx, 90*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return c.Reply("el enlace está mal formado.")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Shin-MD-Go)")
	client := *http.DefaultClient
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("demasiadas redirecciones")
		}
		validada, err := utilidades.ValidarURL(next.URL.String(), utilidades.ResolverDNSIPs)
		if err != nil || validada.Scheme != "https" {
			return fmt.Errorf("redirección de descarga rechazada")
		}
		return nil
	}
	resp, err := client.Do(req)
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

var urlAptoide = "https://ws75.aptoide.com/api/7/apps/search"

type aptoideResp struct {
	Info struct {
		Status string `json:"status"`
	} `json:"info"`
	Datalist struct {
		List []struct {
			Name    string `json:"name"`
			Package string `json:"package"`
			Size    int64  `json:"size"`
			Icon    string `json:"icon"`
			File    struct {
				Path     string `json:"path"`
				Version  string `json:"vername"`
				FileSize int64  `json:"filesize"`
				Malware  struct {
					Rank string `json:"rank"`
				} `json:"malware"`
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
	base, err := url.Parse(urlAptoide)
	if err != nil || base.Hostname() == "" {
		return c.Reply("Aptoide está mal configurado.")
	}
	u := urlAptoide + "?query=" + url.QueryEscape(q) + "&limit=5"
	datos, err := getCortoLimite(c.Ctx, u, 12<<20, base.Hostname())
	if err != nil {
		return c.Reply("Aptoide no responde ahora.")
	}
	var r aptoideResp
	if json.Unmarshal(datos, &r) != nil || (r.Info.Status != "" && r.Info.Status != "OK") {
		return c.Reply("Aptoide devolvió una respuesta que no pude validar.")
	}
	if len(r.Datalist.List) == 0 {
		return c.Reply("no aparece esa app en Aptoide.")
	}
	ap := r.Datalist.List[0]
	size := ap.File.FileSize
	if size <= 0 {
		size = ap.Size
	}
	body := fmt.Sprintf("📦 *%s*\n🧩 %s · versión %s\n🏪 %s · %.1f MB\n🛡 Verificación Aptoide: %s",
		bot.Esc(ap.Name), bot.Esc(ap.Package), bot.Esc(orDefault2(ap.File.Version, "—")),
		bot.Esc(ap.Store.Name), float64(size)/1048576.0,
		bot.Esc(orDefault2(ap.File.Malware.Rank, "sin dato")))
	buttons := make([]bot.Button, 0, 1)
	if aptoideURLSegura(ap.File.Path, "pool.apk.aptoide.com") {
		buttons = append(buttons, bot.CTAURL("⬇️ Descargar APK", ap.File.Path))
	}
	if len(buttons) == 0 {
		return c.Reply(body + "\n\nAptoide no devolvió un enlace de descarga seguro.")
	}
	if aptoideURLSegura(ap.Icon, "pool.img.aptoide.com") {
		if icono, err := getCortoLimite(c.Ctx, ap.Icon, 4<<20, "pool.img.aptoide.com"); err == nil {
			return c.ReplyImage(icono, "Aptoide", body, "Instala APK únicamente si confías en la tienda y la firma.", buttons...)
		}
	}
	return c.ReplyButtons("Aptoide", body, "Instala APK únicamente si confías en la tienda y la firma.", buttons...)
}

func aptoideURLSegura(raw, host string) bool {
	if len(raw) == 0 || len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Hostname(), host) && u.User == nil && u.Port() == ""
}

// ─── ytsearch (Piped, honesto) ──────────────────────────────────────────────

// Piped no tiene una instancia pública permanente. Estas dos respondieron en
// la auditoría de noviembre de 2026; se prueba la segunda solo si la primera falla.
var urlPiped = "https://pipedapi.ducks.party"
var urlPipedFallbacks = []string{"https://api.piped.private.coffee"}

type pipedVideo struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Uploader  string `json:"uploaderName"`
	Duration  int64  `json:"duration"`
	Thumbnail string `json:"thumbnail"`
}

type pipedSearchResp struct {
	Items []pipedVideo `json:"items"`
}

func cmdYtSearch(c *bot.Context) error {
	q := strings.TrimSpace(c.Text)
	if q == "" {
		return c.Reply("¿Qué busco en YouTube? Ej: *.ytsearch lofi*")
	}
	videos, err := buscarPiped(c.Ctx, q)
	if err != nil {
		return c.Reply("Las instancias verificadas de Piped no responden ahora; con *.yt <url>* puedes bajar uno que ya tengas.")
	}
	if len(videos) == 0 {
		return c.Reply("no salió ningún video con esa búsqueda.")
	}
	var sb strings.Builder
	sb.WriteString("📺 *YouTube — " + bot.Esc(q) + "*\n\n")
	shown := 0
	for _, v := range videos {
		watchURL, ok := youtubeURLPiped(v.URL)
		if !ok || (v.Type != "" && v.Type != "stream" && v.Type != "video") {
			continue
		}
		shown++
		duracion := "en vivo / duración desconocida"
		if v.Duration > 0 {
			duracion = fmt.Sprintf("%dm%02ds", v.Duration/60, v.Duration%60)
		}
		fmt.Fprintf(&sb, "%d. *%s* (%s)\n   👤 %s\n   %s\n\n",
			shown, bot.Esc(v.Title), duracion, bot.Esc(v.Uploader), watchURL)
		if shown == 5 {
			break
		}
	}
	if shown == 0 {
		return c.Reply("Piped respondió, pero no devolvió enlaces de video válidos.")
	}
	sb.WriteString("_Para bajar: .yt <enlace> · para música: .play <nombre>_")
	return c.Reply(strings.TrimRight(sb.String(), "\n"))
}

func buscarPiped(ctx context.Context, query string) ([]pipedVideo, error) {
	hosts := append([]string{urlPiped}, urlPipedFallbacks...)
	vistos := make(map[string]bool, len(hosts))
	var ultimo error
	for _, host := range hosts {
		host = strings.TrimRight(strings.TrimSpace(host), "/")
		if host == "" || vistos[host] {
			continue
		}
		vistos[host] = true
		base, parseErr := url.Parse(host)
		if parseErr != nil || base.Hostname() == "" ||
			(base.Scheme != "https" && base.Scheme != "http") || base.User != nil {
			ultimo = fmt.Errorf("instancia Piped inválida")
			continue
		}
		u := host + "/search?q=" + url.QueryEscape(query) + "&filter=videos"
		datos, err := getCortoLimite(ctx, u, 4<<20, base.Hostname())
		if err != nil {
			ultimo = err
			continue
		}
		var env pipedSearchResp
		if err := json.Unmarshal(datos, &env); err == nil && env.Items != nil {
			return env.Items, nil
		}
		// Compatibilidad con instancias antiguas que devolvían el arreglo en
		// la raíz. No se usa para el contrato actual, pero evita una regresión.
		var legacy []pipedVideo
		if err := json.Unmarshal(datos, &legacy); err == nil {
			return legacy, nil
		}
		ultimo = fmt.Errorf("Piped devolvió un esquema desconocido")
	}
	if ultimo == nil {
		ultimo = fmt.Errorf("sin instancias Piped configuradas")
	}
	return nil, ultimo
}

var youtubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

func youtubeURLPiped(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Path != "/watch" {
		return "", false
	}
	id := u.Query().Get("v")
	if !youtubeID.MatchString(id) {
		return "", false
	}
	return "https://youtu.be/" + id, true
}
