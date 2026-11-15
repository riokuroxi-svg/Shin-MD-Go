// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// utilidades2.go — Bloque B5.2: la segunda tanda de cmds/utils/*.js, las que
// sí tocan una API ajena (CADA UNA con la que comprobamos que sigue viva):
//
//	.traducir    Google gtx + MyMemory encadenados, detector propio (alias B4 hook)
//	.tts         texto → nota de voz (motor Google tw-ob, ver internal/apis/voz.go)
//	.acortar     TinyURL            .crypto   CoinGecko
//	.letra       lyrics.ovh         .ai       Gemini (necesita GEMINI_KEY)
//	.githubstalk ficha de usuario   .gitclone búsqueda o descarga de repo en .zip
//	.hd          mejora LOCAL 2× + enfoque, sin subir la foto a ninguna web
//	.carbon      código → imagen dibujada LOCALMENTE (sin carbonara)
//	.ptv         vídeo redondo      .tourl    archivo → enlace temporal
//
// `.benchdl` NO se porta: medía los proveedores del descargador, que aún no
// existe en Go (bloque B6). Medir lo que no hay sería mentir.
package cmds

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"

	"shin-go/internal/apis"
	"shin-go/internal/bot"
	"shin-go/internal/media"
	"shin-go/internal/ui"
	"shin-go/internal/wa"
)

func init() {
	registerTraducir()
	registerTTS()
	registerAcortar()
	registerCrypto()
	registerGithubStalk()
	registerGitClone()
	registerLetra()
	registerHD()
	registerCarbon()
	registerPTV()
	registerTourl()
	registerAI()
}

// (El texto citado lo saca textoCitado de utilidades.go: mismo paquete.)

// ─── .traducir / .tr ────────────────────────────────────────────────────────
// La misma gramática del JS: idioma opcional, texto o mensaje citado, y si el
// texto YA está en el idioma pedido se cambia el destino (es ↔ en), que es lo
// que la persona quería decir el 99 % de las veces.

func registerTraducir() {
	ayuda := func(p string) string {
		return "🌐 *Traducir*\n\nEscribe el idioma y el texto, o responde a un mensaje.\n\n" +
			"› " + p + "tr inglés hola qué tal\n" +
			"› " + p + "tr en   (respondiendo a un mensaje)\n" +
			"› " + p + "tr      (responde y lo pasa al español)\n\n" +
			"Idiomas: 🇪🇸 es · 🇬🇧 en · 🇧🇷 pt · 🇫🇷 fr · 🇮🇹 it · 🇩🇪 de · 🇷🇺 ru · 🇸🇦 ar · 🇯🇵 ja · 🇰🇷 ko · 🇨🇳 zh…"
	}
	bot.Register(bot.Command{
		Name:     "traducir",
		Aliases:  []string{"tr", "translate", "trad"},
		Category: bot.CatUtils,
		Desc:     "Traducir un texto o el mensaje al que respondas (28 idiomas)",
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			p := apis.ParsePeticion(c.Args, textoCitado(c), "es")
			if !p.OK {
				return c.Reply(ayuda(c.Bot.FirstPrefix()))
			}

			destino := p.Idioma
			if antes, confianza := apis.DetectarIdioma(p.Texto); antes == destino && confianza >= 0.3 {
				if destino == "en" {
					destino = "es"
				} else {
					destino = "en"
				}
			}

			ctx, cancel := context.WithTimeout(c.Ctx, 12*time.Second)
			defer cancel()
			r := apis.Traducir(ctx, p.Texto, destino)
			if !r.OK {
				motivos := map[string]string{
					"idioma-desconocido": "No conozco ese idioma. Prueba con el código de dos letras, por ejemplo *en* o *ja*.",
					"tiempo-agotado":     "El traductor tardó demasiado. Inténtalo otra vez en un momento.",
					"sin-texto":          "No hay nada que traducir.",
					"sin-red":            "Me quedé sin conexión para traducir.",
				}
				msg, ok := motivos[r.Motivo]
				if !ok {
					msg = "Los dos traductores fallaron (" + bot.Esc(r.Motivo) + "). Vuelve a intentarlo en un rato."
				}
				return c.Reply("《✧》 " + msg)
			}

			origen := "🌐 detectado"
			if r.De != "" {
				origen = apis.BanderaIdioma(r.De) + " " + apis.NombreIdioma(r.De)
			}
			cuerpo := origen + "  →  " + apis.BanderaIdioma(r.A) + " " + apis.NombreIdioma(r.A) +
				"\n\n" + bot.Esc(r.Texto)
			if p.Recortado {
				cuerpo += "\n\n_⚠ El texto era muy largo y se recortó a " +
					strconv.Itoa(apis.TraducirMaxTexto) + " caracteres._"
			}
			copiable := []rune(r.Texto)
			if len(copiable) > 1000 {
				copiable = copiable[:1000]
			}
			return c.ReplyButtons("🌐 TRADUCCIÓN · SHIN-MD", cuerpo,
				"❦ traducción automática vía "+r.Proveedor+": puede no ser exacta",
				bot.CTACopy("📋 Copiar traducción", string(copiable)))
		},
	})
}

// ─── .tts ───────────────────────────────────────────────────────────────────

func registerTTS() {
	bot.Register(bot.Command{
		Name:     "tts",
		Aliases:  []string{"voz", "decirvoz", "speak"},
		Category: bot.CatUtils,
		Desc:     "Convertir texto en una nota de voz",
		Cooldown: 10,
		Run: func(c *bot.Context) error {
			texto := strings.TrimSpace(c.Text)
			if texto == "" {
				texto = textoCitado(c)
			}
			if texto == "" {
				return c.Reply(fmt.Sprintf("🎙️ *Texto a nota de voz*\n\nEscribe: *%stts <texto>* o responde a un mensaje.\n\nEjemplo: *%stts Hola, soy Shin MD*",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}
			ctx, cancel := context.WithTimeout(c.Ctx, 40*time.Second)
			defer cancel()
			mp3, err := apis.Voz(ctx, texto, "")
			if err != nil {
				return c.Reply("《✧》 No pude generar el audio.\n> " + bot.Esc(err.Error()))
			}
			// El envío va con el contexto del comando, no con el plazo de la
			// API: si la síntesis usó casi todo el tiempo, la cola aún puede
			// mandar el audio tranquila.
			return c.Bot.SendAudioVoz(c.Ctx, c.Chat, mp3, c.Priority())
		},
	})
}

// ─── .acortar ───────────────────────────────────────────────────────────────

func registerAcortar() {
	bot.Register(bot.Command{
		Name:     "acortar",
		Aliases:  []string{"shorturl", "shorten", "acorta"},
		Category: bot.CatUtils,
		Desc:     "Acortar un enlace con TinyURL",
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			enlace := strings.TrimSpace(firstArg(c))
			if enlace == "" {
				return c.Reply(fmt.Sprintf("《✧》 Envía un *enlace* para acortar.\n> Ejemplo: %sacortar https://github.com/riokuroxi-svg/Shin-MD-Go",
					c.Bot.FirstPrefix()))
			}
			ctx, cancel := context.WithTimeout(c.Ctx, 15*time.Second)
			defer cancel()
			corto, err := apis.Acortar(ctx, enlace)
			if err != nil {
				if err.Error() == "no-es-enlace" {
					return c.Reply("《✧》 El texto no parece un enlace válido (debe empezar con http:// o https://).")
				}
				return c.Reply("《✧》 No se pudo acortar el enlace. Intenta más tarde.")
			}
			cuerpo := "🔗 *Enlace acortado:*\n\n> " + corto + "\n\n_Original:_ " + bot.Esc(recortar(enlace, 120))
			return c.ReplyButtons("🔗 ACORTADOR", cuerpo, "TinyURL · Shin-MD",
				bot.CTACopy("📋 Copiar el corto", corto),
				bot.CTAURL("🌐 Abrirlo", corto))
		},
	})
}

// ─── .crypto ────────────────────────────────────────────────────────────────

// nombreBonitoCrypto: igual que en el JS, con su símbolo y su ticker.
var nombreBonitoCrypto = map[string]string{
	"bitcoin": "₿ Bitcoin (BTC)", "ethereum": "Ξ Ethereum (ETH)",
	"solana": "◎ Solana (SOL)", "dogecoin": "Ð Dogecoin (DOGE)",
	"ripple": "✕ Ripple (XRP)", "cardano": "₳ Cardano (ADA)",
	"the-open-network": "💎 Toncoin (TON)", "tether": "₮ Tether (USDT)",
}

func registerCrypto() {
	bot.Register(bot.Command{
		Name:     "crypto",
		Aliases:  []string{"btc", "precio"},
		Category: bot.CatUtils,
		Desc:     "Precio de criptomonedas en USD y MXN (CoinGecko)",
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			var ids []string
			if crudo := strings.TrimSpace(c.Text); crudo != "" {
				ids = strings.FieldsFunc(crudo, func(r rune) bool {
					return r == ',' || r == ' ' || r == '\n'
				})
			}
			ctx, cancel := context.WithTimeout(c.Ctx, 15*time.Second)
			defer cancel()
			precios, err := apis.Precios(ctx, ids)
			if err != nil {
				if err.Error() == "no-encontrada" {
					return c.Reply("《✧》 No encontré esa(s) moneda(s).\n> Ejemplos válidos: bitcoin, ethereum, solana, doge, xrp, ada.")
				}
				return c.Reply("《✧》 No pude consultar los precios (" + bot.Esc(err.Error()) + ").")
			}
			var b strings.Builder
			b.WriteString("💹 *Criptomonedas*\n\n")
			for _, par := range precios {
				nombre := nombreBonitoCrypto[par.ID]
				if nombre == "" {
					nombre = "• " + par.ID
				}
				b.WriteString("*" + nombre + "*\n")
				b.WriteString("  🇺🇸 USD: " + formatoMoneda(par.P.USD, 6) + "\n")
				b.WriteString("  🇲🇽 MXN: " + formatoMoneda(par.P.MXN, 4) + " MXN\n\n")
			}
			b.WriteString("_Fuente: CoinGecko_")
			return c.Reply(b.String())
		},
	})
}

// formatoMoneda dibuja $1,234,567.89: Go no trae separador de miles, así que
// se hace a mano. Menos de $1 usa más decimales (shitcoins y ¢).
func formatoMoneda(v float64, decimalesPequeno int) string {
	if v <= 0 {
		return "—"
	}
	decimales := 2
	if v < 1 {
		decimales = decimalesPequeno
	}
	s := strconv.FormatFloat(v, 'f', decimales, 64)
	punto := strings.IndexByte(s, '.')
	entera, resto := s, ""
	if punto >= 0 {
		entera, resto = s[:punto], s[punto:]
	}
	var b strings.Builder
	for i, r := range entera {
		if i > 0 && (len(entera)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return "$" + b.String() + resto
}

// ─── .githubstalk ───────────────────────────────────────────────────────────

func registerGithubStalk() {
	bot.Register(bot.Command{
		Name:     "githubstalk",
		Aliases:  []string{"ghstalk", "gh"},
		Category: bot.CatUtils,
		Desc:     "Ver la ficha pública de un usuario de GitHub",
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			usuario := strings.TrimSpace(firstArg(c))
			if usuario == "" {
				return c.Reply(fmt.Sprintf("《✧》 Escribe el *nombre de usuario* de GitHub.\n> Ejemplo: %sgh riokuroxi-svg", c.Bot.FirstPrefix()))
			}
			ctx, cancel := context.WithTimeout(c.Ctx, 15*time.Second)
			defer cancel()
			u, err := apis.StalkGitHub(ctx, usuario)
			if err != nil {
				if err.Error() == apis.ErrNoEncontrado.Error() {
					return c.Reply("《✧》 Usuario *" + bot.Esc(usuario) + "* no encontrado en GitHub.")
				}
				return c.Reply("《✧》 Error al consultar GitHub (" + bot.Esc(err.Error()) + ").")
			}

			var lineas []string
			poner := func(s string) {
				if s != "" {
					lineas = append(lineas, s)
				}
			}
			poner("🐙 *GitHub:* " + bot.Esc(u.Login))
			if u.Nombre != "" {
				poner("👤 *Nombre:* " + bot.Esc(u.Nombre))
			}
			if u.Bio != "" {
				poner("📝 *Bio:* " + bot.Esc(recortar(u.Bio, 160)))
			}
			poner(fmt.Sprintf("📂 *Repos públicos:* %d", u.Repos))
			poner(fmt.Sprintf("⭐ *Seguidores:* %d", u.Seguidores))
			poner(fmt.Sprintf("👣 *Siguiendo:* %d", u.Siguiente))
			if u.Ubicacion != "" {
				poner("📍 *Ubicación:* " + bot.Esc(u.Ubicacion))
			}
			if u.Web != "" {
				poner("🔗 *Web:* " + bot.Esc(u.Web))
			}
			if u.Empresa != "" {
				poner("🏢 *Empresa:* " + bot.Esc(u.Empresa))
			}
			if u.Twitter != "" {
				poner("🐦 *Twitter:* @" + bot.Esc(u.Twitter))
			}
			if f := fechaBonita(u.Creado); f != "" {
				poner("📅 *En GitHub desde:* " + f)
			}
			poner("🔗 " + u.HTMLURL)
			caption := strings.Join(lineas, "\n")

			if u.AvatarURL != "" {
				if foto, err := apis.DescargarURL(ctx, u.AvatarURL, 5<<20); err == nil {
					return c.Bot.SendImage(c.Ctx, c.Chat, foto, caption, c.Priority())
				}
				// Sin avatar no pasa nada: la ficha sale en texto.
			}
			return c.Reply(caption)
		},
	})
}

// fechaBonita convierte «2019-08-10T14:23:41Z» en «10/8/2019».
func fechaBonita(iso string) string {
	if len(iso) < 10 {
		return ""
	}
	t, err := time.Parse("2006-01-02", iso[:10])
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d/%d/%d", t.Day(), int(t.Month()), t.Year())
}

// ─── .gitclone ──────────────────────────────────────────────────────────────

func registerGitClone() {
	bot.Register(bot.Command{
		Name:     "gitclone",
		Aliases:  []string{"git"},
		Category: bot.CatUtils,
		Desc:     "Buscar o descargar un repositorio de GitHub (.zip)",
		Cooldown: 15,
		Run: func(c *bot.Context) error {
			consulta := strings.TrimSpace(c.Text)
			if consulta == "" {
				return c.Reply(fmt.Sprintf("《✧》 Pásame un enlace o el nombre del repo.\n> Ejemplo: %sgitclone https://github.com/riokuroxi-svg/Shin-MD-Go", c.Bot.FirstPrefix()))
			}
			ctx, cancel := context.WithTimeout(c.Ctx, 60*time.Second)
			defer cancel()

			if dueno, repo := apis.ParseRepoURL(consulta); dueno != "" {
				return gitcloneDescarga(c, ctx, dueno, repo)
			}
			return gitcloneBusqueda(c, ctx, consulta)
		},
	})
}

// gitcloneBusqueda: texto libre → lista de 5 (y si solo hay uno, se descarga).
func gitcloneBusqueda(c *bot.Context, ctx context.Context, consulta string) error {
	repos, err := apis.BuscarRepos(ctx, consulta)
	if err != nil {
		if err.Error() == apis.ErrNoEncontrado.Error() {
			return c.Reply("《✧》 No se encontraron repositorios para *" + bot.Esc(recortar(consulta, 60)) + "*.")
		}
		return c.Reply("《✧》 Error al consultar GitHub (" + bot.Esc(err.Error()) + ").")
	}
	// El JS baja el .zip directo cuando hay un único resultado.
	if len(repos) == 1 {
		return gitcloneDescarga(c, ctx, repos[0].Dueno, repos[0].Nombre)
	}
	var lineas []string
	for i, r := range repos {
		lineas = append(lineas, fmt.Sprintf(
			"✩ *Resultado %d*\n✩ %s/%s\n✩ ⭐ %d · 🍴 %d\n✩ %s\n✩ %s",
			i+1, bot.Esc(r.Dueno), bot.Esc(r.Nombre), r.Estrellas, r.Forks,
			bot.Esc(recortar(r.Descripcion, 120)), r.HTMLURL))
	}
	cuerpo := "🔎 *GitHub:* resultados de «" + bot.Esc(recortar(consulta, 40)) + "»\n\n" +
		strings.Join(lineas, "\n────────────────────\n") +
		"\n\n> Pásame el enlace completo y te bajo el .zip."
	if avatar := repos[0].AvatarDueno; avatar != "" {
		if foto, err := apis.DescargarURL(ctx, avatar, 5<<20); err == nil {
			return c.Bot.SendImage(c.Ctx, c.Chat, foto, cuerpo, c.Priority())
		}
	}
	return c.Reply(cuerpo)
}

// gitcloneDescarga: enlace concreto → ficha + .zip como documento.
func gitcloneDescarga(c *bot.Context, ctx context.Context, dueno, repo string) error {
	info, err := apis.RepoInfo(ctx, dueno, repo)
	if err != nil {
		if err.Error() == apis.ErrNoEncontrado.Error() {
			return c.Reply("《✧》 Repositorio no encontrado: *" + bot.Esc(dueno+"/"+repo) + "*.")
		}
		return c.Reply("《✧》 Error al consultar GitHub (" + bot.Esc(err.Error()) + ").")
	}
	zip, nombre, err := apis.Zipball(ctx, dueno, repo)
	if err != nil {
		if err.Error() == "muy-grande" {
			return c.Reply(fmt.Sprintf("《✧》 *%s* pasa de %d MB comprimido: no lo voy a subir al chat.\n> %s",
				bot.Esc(info.Nombre), apis.MaxZip>>20, info.HTMLURL))
		}
		return c.Reply("《✧》 No pude bajar el .zip (" + bot.Esc(err.Error()) + ").")
	}
	caption := fmt.Sprintf("📦 *%s/%s*\n⭐ %d · 🍴 %d · 🐛 %d issues\n📝 %s\n🔗 %s",
		bot.Esc(info.Dueno), bot.Esc(info.Nombre), info.Estrellas, info.Forks, info.Issues,
		bot.Esc(recortar(info.Descripcion, 140)), info.HTMLURL)
	if info.Descripcion == "" {
		caption = strings.Replace(caption, "📝 \n", "", 1)
	}
	if err := c.Reply(caption); err != nil {
		return err
	}
	return c.Bot.SendDocument(c.Ctx, c.Chat, zip, nombre, "application/zip", c.Priority())
}

// ─── .letra ─────────────────────────────────────────────────────────────────

func registerLetra() {
	bot.Register(bot.Command{
		Name:     "letra",
		Aliases:  []string{"lyrics", "lyric"},
		Category: bot.CatUtils,
		Desc:     "Buscar la letra de una canción",
		Cooldown: 5,
		Run: func(c *bot.Context) error {
			texto := strings.TrimSpace(c.Text)
			if texto == "" {
				return c.Reply(fmt.Sprintf("《✧》 Escribe el *artista* y la *canción*.\n> Formato: %sletra Bad Bunny/Diles\n> O también: %sletra Bad Bunny Diles",
					c.Bot.FirstPrefix(), c.Bot.FirstPrefix()))
			}
			artista, cancion := apis.ParseLetraPeticion(texto)
			if artista == "" || cancion == "" {
				return c.Reply(fmt.Sprintf("《✧》 Usa el formato *artista/canción*.\n> Ej: %sletra Bad Bunny/Diles", c.Bot.FirstPrefix()))
			}
			ctx, cancel := context.WithTimeout(c.Ctx, 15*time.Second)
			defer cancel()
			letra, err := apis.Letra(ctx, artista, cancion)
			if err != nil {
				if err.Error() == "no-encontrada" {
					return c.Reply("《✧》 No encontré la letra de *" + bot.Esc(cancion) + "* de *" + bot.Esc(artista) + "*.")
				}
				return c.Reply("《✧》 Error al buscar la letra (" + bot.Esc(err.Error()) + ").")
			}
			cabecera := "🎵 *Letra: " + bot.Esc(cancion) + "*\n👤 " + bot.Esc(artista) + "\n\n"
			trozos := apis.TrocearLetra(letra, apis.LetraMensajeMax-len(cabecera))
			for i, trozo := range trozos {
				encabezado := cabecera
				if i > 0 {
					encabezado = "🎵 *" + bot.Esc(cancion) + "* (" +
						strconv.Itoa(i+1) + "/" + strconv.Itoa(len(trozos)) + ")\n\n"
				}
				if err := c.Reply(encabezado + bot.Esc(trozo)); err != nil {
					return err
				}
			}
			return nil
		},
	})
}

// ─── .hd ────────────────────────────────────────────────────────────────────

func registerHD() {
	bot.Register(bot.Command{
		Name:     "hd",
		Aliases:  []string{"remini", "enhance", "mejorar"},
		Category: bot.CatUtils,
		Desc:     "Mejorar la nitidez de una imagen (local, 2× + enfoque)",
		Cooldown: 15,
		Run: func(c *bot.Context) error {
			medio, ok := wa.MedioDe(c.Event.Message)
			if !ok || medio.Tipo != wa.MedioImagen {
				return c.Reply(fmt.Sprintf("《✧》 Responde a una *imagen* con:\n> %shd\n_La mejora es local: la foto no sale del servidor._", c.Bot.FirstPrefix()))
			}
			datos, err := c.Bot.DownloadMedia(c.Ctx, medio.Data)
			if err != nil || len(datos) == 0 {
				return c.Reply("《✧》 No se pudo descargar la imagen.")
			}
			img, err := media.Decode(datos)
			if err != nil {
				return c.Reply("《✧》 Ese formato de imagen no lo entiendo (uso JPEG, PNG o WebP).")
			}
			mejorada, err := media.Mejorar(img)
			if err != nil {
				if err.Error() == "demasiado-grande" {
					return c.Reply("《✧》 Esa imagen ya es enorme: no hay nada que mejorarle.")
				}
				return c.Reply("《✧》 No pude mejorarla (" + bot.Esc(err.Error()) + ").")
			}
			jpg, err := media.EncodeJPEG(mejorada, 92)
			if err != nil {
				return c.Reply("《✧》 No pude codificar el resultado.")
			}
			b := mejorada.Bounds()
			return c.Bot.SendImage(c.Ctx, c.Chat, jpg,
				fmt.Sprintf("🔍 *HD* · %d×%d → %d×%d · enfoque aplicado",
					img.Bounds().Dx(), img.Bounds().Dy(), b.Dx(), b.Dy()),
				c.Priority())
		},
	})
}

// ─── .carbon ────────────────────────────────────────────────────────────────

func registerCarbon() {
	bot.Register(bot.Command{
		Name:     "carbon",
		Aliases:  []string{"codigoimg", "codeimg"},
		Category: bot.CatUtils,
		Desc:     "Convertir código en una imagen bonita (local)",
		Cooldown: 8,
		Run: func(c *bot.Context) error {
			tema := ""
			codigo := strings.TrimSpace(c.Text)
			if len(c.Args) > 0 {
				if _, existe := ui.TemasCarbon[strings.ToLower(c.Args[0])]; existe {
					tema = strings.ToLower(c.Args[0])
					codigo = strings.TrimSpace(strings.TrimPrefix(codigo, c.Args[0]))
				}
			}
			if codigo == "" {
				codigo = textoCitado(c)
			}
			if codigo == "" {
				var nombres []string
				for n := range ui.TemasCarbon {
					nombres = append(nombres, n)
				}
				sort.Strings(nombres)
				return c.Reply(fmt.Sprintf("《✧》 Responde a un mensaje con código, o escribe el código a convertir.\n> Colores: %s\n> Ejemplo: %scarbon verde fmt.Println(\"hola\")",
					strings.Join(nombres, ", "), c.Bot.FirstPrefix()))
			}
			png, err := ui.RenderCarbon(codigo, tema)
			if err != nil {
				return c.Reply("《✧》 No pude generar la imagen (" + bot.Esc(err.Error()) + ").")
			}
			caption := "🎨 *Carbon*"
			if tema != "" {
				caption += " · tema " + tema
			}
			return c.Bot.SendImage(c.Ctx, c.Chat, png, caption, c.Priority())
		},
	})
}

// ─── .ptv ───────────────────────────────────────────────────────────────────

const (
	ptvMaxSegundos = 60
	ptvMaxBytes    = 16 << 20 // los límites reales de WhatsApp, como en el JS
)

// puedeSerPTV es la regla pura del JS (exportada allí como `puedeSerPtv` y
// probada sin sesión): ≤60 s, ≤16 MB y no GIF.
func puedeSerPTV(segundos, bytes uint64, esGIF bool) (bool, string) {
	if segundos > ptvMaxSegundos {
		return false, "largo"
	}
	if bytes > ptvMaxBytes {
		return false, "pesado"
	}
	if esGIF {
		return false, "gif"
	}
	return true, ""
}

func registerPTV() {
	bot.Register(bot.Command{
		Name:     "ptv",
		Aliases:  []string{"videoredondo", "redondo", "notavideo"},
		Category: bot.CatUtils,
		Desc:     "Convertir un vídeo (≤60 s) en vídeo redondo",
		Cooldown: 15,
		Run: func(c *bot.Context) error {
			medio, ok := wa.MedioDe(c.Event.Message)
			if !ok || medio.Tipo != wa.MedioVideo {
				return c.Reply("《✧》 Responde a un *vídeo* con " + c.Bot.FirstPrefix() +
					"ptv para convertirlo en redondo.\n> Tiene que durar menos de 60 segundos.")
			}
			video, esVideo := medio.Data.(*waE2E.VideoMessage)
			var segundos, bytes uint64
			if esVideo {
				segundos = uint64(video.GetSeconds())
				bytes = video.GetFileLength()
			}
			si, motivo := puedeSerPTV(segundos, bytes, esVideo && video.GetGifPlayback())
			if !si {
				switch motivo {
				case "largo":
					return c.Reply("《✧》 Ese vídeo dura más de *60 s*: WhatsApp no lo acepta como redondo.")
				case "pesado":
					return c.Reply("《✧》 Ese vídeo pasa de *16 MB*: píllalo más pequeño o recórtalo.")
				default:
					return c.Reply("《✧》 Los GIF no valen para vídeo redondo (usa " + c.Bot.FirstPrefix() + "sticker para eso).")
				}
			}
			datos, err := c.Bot.DownloadMedia(c.Ctx, medio.Data)
			if err != nil || len(datos) == 0 {
				return c.Reply("《✧》 No pude descargar el vídeo.")
			}
			return c.Bot.SendVideoRedondo(c.Ctx, c.Chat, datos, int(segundos), c.Priority())
		},
	})
}

// ─── .tourl ────────────────────────────────────────────────────────────────

func registerTourl() {
	bot.Register(bot.Command{
		Name:     "tourl",
		Aliases:  []string{"subirurl", "aurl"},
		Category: bot.CatUtils,
		Desc:     "Subir un archivo y devolver su enlace temporal",
		Cooldown: 10,
		Run: func(c *bot.Context) error {
			medio, ok := wa.MedioDe(c.Event.Message)
			if !ok {
				return c.Reply(fmt.Sprintf("《✧》 Responde a una *imagen, vídeo, audio o documento* con %stourl\n> El enlace caduca solo (24–72 h): no queda basura permanente.",
					c.Bot.FirstPrefix()))
			}
			ctx, cancel := context.WithTimeout(c.Ctx, 90*time.Second)
			defer cancel()
			datos, err := c.Bot.DownloadMedia(ctx, medio.Data)
			if err != nil || len(datos) == 0 {
				return c.Reply("《✧》 No pude descargar el archivo de WhatsApp.")
			}
			if len(datos) > apis.MaxSubida {
				return c.Reply(fmt.Sprintf("《✧》 Pasa de %d MB: no lo subo.", apis.MaxSubida>>20))
			}
			mime := medio.Mimetype
			if mime == "" {
				mime = "application/octet-stream"
			}
			nombre := fmt.Sprintf("shin_%d.%s", time.Now().Unix(), extensionDe(mime))
			enlace, caducidad, err := apis.SubirTemporal(ctx, datos, mime, nombre)
			if err != nil {
				return c.Reply("《✧》 No pude subir el archivo (" + bot.Esc(err.Error()) + ").")
			}
			cuerpo := "📤 *Archivo subido*\n\n> " + enlace +
				"\n\n_Caduca en " + caducidad + " · " + formatoBytes(len(datos)) + "_"
			return c.ReplyButtons("📤 ENLACE TEMPORAL", cuerpo, "litterbox · Shin-MD",
				bot.CTACopy("📋 Copiar enlace", enlace),
				bot.CTAURL("🌐 Abrirlo", enlace))
		},
	})
}

// extensionDe saca la extensión del mime («image/jpeg» → «jpg»).
func extensionDe(mime string) string {
	partes := strings.SplitN(mime, "/", 2)
	if len(partes) != 2 {
		return "bin"
	}
	ext := strings.SplitN(partes[1], ";", 2)[0]
	switch ext {
	case "", "octet-stream":
		return "bin"
	case "jpeg":
		return "jpg"
	}
	return ext
}

// formatoBytes: 1536 → «1.5 KB». Igual que la del JS.
func formatoBytes(bytes int) string {
	if bytes < 1024 {
		return strconv.Itoa(bytes) + " B"
	}
	unidades := []string{"KB", "MB", "GB"}
	v := float64(bytes)
	i := -1
	for v >= 1024 && i+1 < len(unidades) {
		v /= 1024
		i++
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".") + " " + unidades[i]
}

// ─── .ai ────────────────────────────────────────────────────────────────────

func registerAI() {
	bot.Register(bot.Command{
		Name:     "ai",
		Aliases:  []string{"ia", "chatgpt", "gemini", "chat"},
		Category: bot.CatUtils,
		Desc:     "Preguntar a la IA (necesita GEMINI_KEY en el .env)",
		Cooldown: 8,
		Run: func(c *bot.Context) error {
			cfg := c.Bot.Config()
			if cfg.GeminiKey == "" || cfg.GeminiKey == "tu_key_aqui" {
				return c.Reply("《✧》 No hay *key de Gemini* configurada.\n" +
					"> Consíguela gratis en aistudio.google.com/apikey y añade al .env:\n" +
					"> GEMINI_KEY=tu_key")
			}
			texto := strings.TrimSpace(c.Text)
			if texto == "" {
				texto = textoCitado(c)
			}

			// Como en el JS: una imagen citada entra como contexto visual.
			var imgB64, imgMime string
			if medio, ok := wa.MedioDe(c.Event.Message); ok && medio.Tipo == wa.MedioImagen {
				if datos, err := c.Bot.DownloadMedia(c.Ctx, medio.Data); err == nil && len(datos) < 4<<20 {
					imgB64 = base64.StdEncoding.EncodeToString(datos)
					imgMime = medio.Mimetype
				}
			}
			if texto == "" && imgB64 == "" {
				return c.Reply(fmt.Sprintf("《✧》 Escríbeme la pregunta.\n> Ejemplo: %sai ¿cuántos litros de agua hay en un vaso de 250 ml?", c.Bot.FirstPrefix()))
			}

			ctx, cancel := context.WithTimeout(c.Ctx, 40*time.Second)
			defer cancel()
			respuesta, err := apis.Gemini(ctx, cfg.GeminiKey, cfg.GeminiModel, texto, imgB64, imgMime)
			if err != nil {
				motivos := map[string]string{
					"key-invalida":   "La key de Gemini no vale (400/403). Revísala en el .env.",
					"cupo":           "Gemini está a tope ahora mismo (429). Prueba en un minuto.",
					"tiempo-agotado": "La IA tardó demasiado. Inténtalo otra vez.",
				}
				msg, ok := motivos[err.Error()]
				if !ok {
					msg = "No pude consultar a la IA (" + bot.Esc(err.Error()) + ")."
				}
				return c.Reply("《✧》 " + msg)
			}
			if runas := []rune(respuesta); len(runas) > 3500 {
				respuesta = string(runas[:3500]) + "…"
			}
			return c.Reply("💭 *Shin-MD · IA*\n\n" + respuesta)
		},
	})
}
