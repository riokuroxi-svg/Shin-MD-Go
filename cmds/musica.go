// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// musica.go — bloque B6.0: la busca y la baja de audio.
//
// El motor base de .play busca Audius + Deezer + iTunes y distingue audio
// libre de ficha comercial. El operador puede habilitar además el ejecutable
// spotDL: se invoca directamente, sin shell ni dependencia de Node, y sus
// resultados quedan marcados como externos en vez de fingir que son libres.
//
//	.play <nombre>           busca en los catálogos rápidos.
//	.play <enlace Spotify>   resuelve y descarga con spotDL si está habilitado.
//	.play spotdl:<consulta>  fuerza ese backend externo.
//	.escucha                 muestra el preview oficial de Deezer/iTunes.
//
// UX:
//
//	· La búsqueda muestra la carátula real y botones para descargar, escuchar
//	  el preview o abrir la fuente; «.play n» sigue funcionando como respaldo.
//	· Sin caché en disco: evita enlaces vencidos y archivos abandonados.
//	· El semáforo por chat se conserva (máx. 2 bajadas a la vez): la escucha
//	  no tumba el bot.
//
// `.play` no es Priority: baja una canción de MB y gasta su cola ahí.
package cmds

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"shin-go/internal/bot"
	"shin-go/internal/musica"
	"shin-go/internal/wa"
)

// maxDescargasParalelas es el semáforo por chat del play.js, conservado: dos
// canciones a la vez por chat, el tercero espera educadamente.
const maxDescargasParalelas = 2

// gMusica es el cliente real; en pruebas se cambia por otro apuntando a
// httptest (todo lo demás —semáforo, lista, respuestas— es igual).
var gMusica = musica.Nuevo(musica.Opciones{Timeout: 15 * time.Second})
var clienteMusica = func() *musica.Cliente { return gMusica }

// ConfigurarSpotDL habilita el ejecutable externo antes de arrancar el router.
// Si falta o no funciona, devuelve el motivo y conserva el motor tradicional.
func ConfigurarSpotDL(ctx context.Context, path, ffmpeg, cookieFile, audio string, timeout time.Duration) (string, error) {
	// Restablecer primero evita conservar por accidente un ejecutable o unas
	// cookies anteriores si una reconfiguración nueva falla.
	gMusica = musica.Nuevo(musica.Opciones{Timeout: 15 * time.Second})
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	nuevo := musica.Nuevo(musica.Opciones{Timeout: 15 * time.Second})
	if err := nuevo.UsarSpotDL(musica.OpcionesSpotDL{
		Path:       path,
		FFmpeg:     ffmpeg,
		CookieFile: cookieFile,
		Audio:      audio,
		Timeout:    timeout,
	}); err != nil {
		return "", err
	}
	version, err := nuevo.VersionSpotDL(ctx)
	if err != nil {
		return "", err
	}
	gMusica = nuevo
	return version, nil
}

// Semáforo y «lista guardada»: un map por chat con un mutex corto. Se usa un
// mutex y no canales porque dos goroutines del mismo chat compiten entre sí
// (y el segundo «.play» tiene que esperar, no pisar).
var (
	muMusica  sync.Mutex
	slotsMus  = map[wa.JID]int{}
	listasMus = map[wa.JID]listaPlay{}
)

// listaPlay guarda la última tabla de resultados por chat: «.play 2» la
// consulta. No se rebusca: la segunda consulta puede traer otras fuentes y el
// número cambiaría de pie.
type listaPlay struct {
	pistas   []musica.Pista
	consulta string
	cuando   time.Time
}

func init() {
	bot.Register(bot.Command{
		Name: "play", Aliases: []string{"playa"}, Category: bot.CatDownload,
		Desc: "Busca música y la envía; admite enlaces Spotify o «spotdl:consulta» cuando el servidor habilita spotDL.",
		Run:  cmdPlay,
	})
	bot.Register(bot.Command{
		Name: "escucha", Aliases: []string{"preview", "oy", "oi"}, Category: bot.CatDownload,
		Desc: "Escucha oficial: el fragmento de 30 s que publican Deezer/iTunes de una canción.",
		Run:  cmdEscucha,
	})
}

// ── .play ───────────────────────────────────────────────────────────────────

func cmdPlay(c *bot.Context) error {
	caja := strings.TrimSpace(c.Text)

	// «.play 2» sin texto nuevo: número de la última lista.
	if n, solo := esSoloNumero(caja); solo {
		return playPorNumero(c, n)
	}
	if caja == "" {
		return c.Reply("¿Qué busco? Ejemplo: *.play villanos del sana*. También acepta un enlace de Spotify o *.play spotdl:artista canción* si el servidor lo habilitó.")
	}

	consulta, forzarSpotDL := consultaParaSpotDL(caja)
	if consulta == "" {
		return c.Reply("Después de *spotdl:* escribe una canción o pega un enlace de Spotify.")
	}
	cli := clienteMusica()
	var b musica.Busqueda
	if forzarSpotDL {
		if !cli.TieneSpotDL() {
			return c.Reply("Este servidor no tiene spotDL habilitado. Configura *SPOTDL_PATH* y *SPOTDL_FFMPEG*, o busca por nombre para usar Audius/iTunes.")
		}
		b = buscarSoloSpotDL(c.Ctx, cli, consulta, 5)
	} else {
		b = cli.Buscar(c.Ctx, consulta, 5)
		// Solo se paga el proceso externo cuando los catálogos rápidos no
		// encontraron nada. Para forzarlo existe «spotdl:consulta».
		if len(b.Pistas) == 0 && cli.TieneSpotDL() {
			spot := buscarSoloSpotDL(c.Ctx, cli, consulta, 5)
			b.Pistas = append(b.Pistas, spot.Pistas...)
			for fuente, fallo := range spot.Fallos {
				b.Fallos[fuente] = fallo
			}
		}
	}
	guardarLista(c.Chat, b)

	if len(b.Pistas) == 0 {
		return c.Reply(respuestaSinPistas(b))
	}
	return mostrarTarjetaPlay(c, b, consulta)
}

func mostrarTarjetaPlay(c *bot.Context, b musica.Busqueda, consulta string) error {
	primera := b.Pistas[0]
	tope := min(5, len(b.Pistas))
	descargables, previews := cuentas(b.Pistas[:tope])

	var body strings.Builder
	fmt.Fprintf(&body, "*%s*\n👤 %s", bot.Esc(primera.Titulo), bot.Esc(primera.Artista))
	if primera.Album != "" {
		fmt.Fprintf(&body, "\n💿 %s", bot.Esc(primera.Album))
	}
	fmt.Fprintf(&body, "\n⏱ %s · %s", primera.Minutos(), bot.Esc(primera.Fuente))
	if tope > 1 {
		fmt.Fprintf(&body, "\n\n*%d resultados* · %d con audio · %d con preview\n", tope, descargables, previews)
		for i, pista := range b.Pistas[:tope] {
			fmt.Fprintf(&body, "\n*%d.* %s", i+1, lineaPista(pista))
		}
	}

	buttons := make([]bot.Button, 0, 4)
	if primera.Descargable() {
		buttons = append(buttons, bot.QuickReply("⬇️ Descargar audio", "play:1"))
	}
	if primera.Preview != "" {
		buttons = append(buttons, bot.QuickReply("▶️ Escuchar preview", "escucha:1"))
	}

	if tope > 1 {
		rows := make([]bot.ListRow, 0, tope*2)
		for i, pista := range b.Pistas[:tope] {
			title := bot.ClipText(fmt.Sprintf("%d. %s", i+1, pista.Titulo), 48)
			if pista.Descargable() {
				rows = append(rows, bot.ListRow{
					ID: fmt.Sprintf("play:%d", i+1), Title: title,
					Desc: bot.ClipText(pista.Artista+" · Descargar audio", 72),
				})
			}
			if pista.Preview != "" {
				rows = append(rows, bot.ListRow{
					ID: fmt.Sprintf("escucha:%d", i+1), Title: title,
					Desc: bot.ClipText(pista.Artista+" · Preview oficial", 72),
				})
			}
		}
		if len(rows) > 0 {
			buttons = append(buttons, bot.SingleSelect("🎶 Elegir canción",
				bot.ListSection{Title: "Resultados para " + bot.ClipText(consulta, 35), Rows: rows}))
		}
	}
	if origenMusicaSeguro(primera) {
		buttons = append(buttons, bot.CTAURL("🌐 Abrir canción", primera.Origen))
	}

	footer := "Shin-MD · botones o .play N / .escucha N"
	if primera.Caratula != "" {
		if artwork, err := clienteMusica().DescargarCaratula(c.Ctx, primera); err == nil {
			return c.ReplyImage(artwork, "🎵 Reproductor", body.String(), footer, buttons...)
		} else if c.Log != nil {
			c.Log.Warn("play: no se pudo cargar la carátula: " + err.Error())
		}
	}
	if len(buttons) > 0 {
		return c.ReplyButtons("🎵 Reproductor", body.String(), footer, buttons...)
	}
	return c.Reply(body.String() + "\n\nEscribe *.play N* para descargar o *.escucha N* para el preview.")
}

func origenMusicaSeguro(p musica.Pista) bool {
	raw := p.Origen
	if len(raw) == 0 || len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Port() != "" {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	allowed := map[string]bool{}
	switch {
	case p.Backend == musica.BackendSpotDL || p.Fuente == musica.FuenteSpotDL:
		allowed["open.spotify.com"] = true
		allowed["spotify.link"] = true
	case p.Fuente == musica.FuenteAudius:
		allowed["audius.co"] = true
		allowed["www.audius.co"] = true
	case p.Fuente == musica.FuenteDeezer:
		allowed["deezer.com"] = true
		allowed["www.deezer.com"] = true
	case p.Fuente == musica.FuenteITunes:
		allowed["music.apple.com"] = true
		allowed["itunes.apple.com"] = true
	case p.Fuente == musica.FuenteArchivo:
		allowed["archive.org"] = true
		allowed["www.archive.org"] = true
	}
	return allowed[host]
}

func consultaParaSpotDL(raw string) (string, bool) {
	query := strings.TrimSpace(raw)
	lower := strings.ToLower(query)
	if strings.HasPrefix(lower, "spotdl:") {
		return strings.TrimSpace(query[len("spotdl:"):]), true
	}
	if strings.HasPrefix(lower, "spotify:track:") ||
		strings.HasPrefix(lower, "https://open.spotify.com/") ||
		strings.HasPrefix(lower, "https://spotify.link/") {
		return query, true
	}
	return query, false
}

func buscarSoloSpotDL(ctx context.Context, cli *musica.Cliente, query string, limit int) musica.Busqueda {
	b := musica.Busqueda{Consulta: query, Fallos: map[string]string{}}
	pistas, err := cli.BuscarSpotDL(ctx, query, limit)
	if err != nil {
		b.Fallos[musica.FuenteSpotDL] = err.Error()
		return b
	}
	b.Pistas = pistas
	return b
}

// playPorNumero cumple «.play 3»: baja la 3 de la última lista del chat.
func playPorNumero(c *bot.Context, n int) error {
	muMusica.Lock()
	ult, ok := listasMus[c.Chat]
	muMusica.Unlock()
	if !ok || len(ult.pistas) == 0 {
		return c.Reply("No tengo ninguna lista guardada en este chat; primero busca con *.play <nombre>*.")
	}
	if time.Since(ult.cuando) > 10*time.Minute {
		return c.Reply("La lista es vieja; repite la búsqueda con *.play <nombre>* para que los enlaces sigan vivos.")
	}
	tope := min(5, len(ult.pistas))
	if n < 1 || n > tope {
		return c.Reply(fmt.Sprintf("Esa lista solo tiene %d; elige 1-%d.", tope, tope))
	}
	p := ult.pistas[n-1]
	if !p.Descargable() {
		if p.Preview != "" {
			return c.Reply(fmt.Sprintf("Esa (%s) solo publica preview; usa *.escucha %d*.",
				bot.Esc(p.Titulo), n))
		}
		return c.Reply("Esa canción no ofrece audio ni preview en el proveedor.")
	}
	if err := descargarYEnviar(c, p); err != nil {
		return c.Reply(motivoErrorMusica(err))
	}
	return nil
}

// descargarYEnviar: semáforo, descarga con los filtros del motor, manda.
func descargarYEnviar(c *bot.Context, p musica.Pista) error {
	return conSlotMusica(c, func() error {
		var audio []byte
		// Descargar aplica sus filtros (audio real y ≤ 24 MB); aquí solo se recoge
		// en memoria para el envío.
		if _, err := clienteMusica().Descargar(c.Ctx, p, &yardaMusica{&audio}); err != nil {
			return err
		}
		titulo := bot.ClipText(p.Titulo+" — "+p.Artista, 120)
		return c.Bot.SendAudioPista(c.Ctx, c.Chat, audio, titulo, p.Duracion, p.Origen, c.Priority())
	})
}

func conSlotMusica(c *bot.Context, trabajo func() error) error {
	muMusica.Lock()
	if slotsMus[c.Chat] >= maxDescargasParalelas {
		muMusica.Unlock()
		return c.Reply("Ya tengo dos audios cargándose para este chat; cuando salgan, repite el comando.")
	}
	slotsMus[c.Chat]++
	muMusica.Unlock()
	defer func() {
		muMusica.Lock()
		if slotsMus[c.Chat]--; slotsMus[c.Chat] <= 0 {
			delete(slotsMus, c.Chat)
		}
		muMusica.Unlock()
	}()
	return trabajo()
}

// ── .escucha ─────────────────────────────────────────────────────────────────

func cmdEscucha(c *bot.Context) error {
	caja := strings.TrimSpace(c.Text)
	if n, solo := esSoloNumero(caja); solo {
		return escuchaPorNumero(c, n)
	}
	if caja == "" {
		return c.Reply("¿Qué escucho? Ejemplo: *.escucha daft punk*. Solo sale el fragmento oficial de 30 s que publica la tienda.")
	}
	b := clienteMusica().Buscar(c.Ctx, caja, 5)
	guardarLista(c.Chat, b)
	for _, p := range b.Pistas {
		if p.Preview != "" {
			return descargarYEnviarPreview(c, p)
		}
	}
	if len(b.Fallos) > 0 {
		return c.Reply(respuestaSinPistas(b))
	}
	return c.Reply("Ni Deezer ni iTunes tienen preview de eso; puede que solo exista en el catálogo libre (prueba *.play*).")
}

func escuchaPorNumero(c *bot.Context, n int) error {
	muMusica.Lock()
	ult, ok := listasMus[c.Chat]
	muMusica.Unlock()
	if !ok || len(ult.pistas) == 0 {
		return c.Reply("No tengo una lista guardada; primero busca con *.play <nombre>*.")
	}
	if time.Since(ult.cuando) > 10*time.Minute {
		return c.Reply("La lista ya caducó; repite la búsqueda con *.play <nombre>*.")
	}
	tope := min(5, len(ult.pistas))
	if n < 1 || n > tope {
		return c.Reply(fmt.Sprintf("Esa lista solo tiene %d; elige 1-%d.", tope, tope))
	}
	p := ult.pistas[n-1]
	if p.Preview == "" {
		return c.Reply("Esa canción no publica preview; si tiene botón de descarga, usa *.play " + strconv.Itoa(n) + "*.")
	}
	return descargarYEnviarPreview(c, p)
}

func descargarYEnviarPreview(c *bot.Context, p musica.Pista) error {
	return conSlotMusica(c, func() error {
		var audio []byte
		if _, err := clienteMusica().DescargarPreview(c.Ctx, p, &yardaMusica{&audio}); err != nil {
			return c.Reply("No pude cargar el preview oficial esta vez (" + bot.Esc(cerca(err.Error(), 120)) + ").")
		}
		titulo := bot.ClipText(p.Titulo+" — "+p.Artista+" (preview)", 120)
		segundos := 30
		if p.Duracion > 0 && p.Duracion < segundos {
			segundos = p.Duracion
		}
		return c.Bot.SendAudioPista(c.Ctx, c.Chat, audio, titulo, segundos, p.Origen, c.Priority())
	})
}

// ── Presentación y pelo ─────────────────────────────────────────────────────

func guardarLista(jid wa.JID, b musica.Busqueda) {
	if len(b.Pistas) == 0 {
		return
	}
	muMusica.Lock()
	defer muMusica.Unlock()
	listasMus[jid] = listaPlay{pistas: b.Pistas, consulta: b.Consulta, cuando: time.Now()}
	if len(listasMus) > 40 {
		// Limpieza perezosa: con muchas listas viejas se barren las entradas
		// de más de 15 min. Sin cron para una tabla de 5.
		for k, l := range listasMus {
			if time.Since(l.cuando) > 15*time.Minute {
				delete(listasMus, k)
			}
		}
	}
}

func cuentas(pistas []musica.Pista) (descargables, conPreview int) {
	for _, p := range pistas {
		if p.Descargable() {
			descargables++
		}
		if p.Preview != "" {
			conPreview++
		}
	}
	return
}

func lineaPista(p musica.Pista) string {
	marca := "⚪ solo ficha"
	switch {
	case p.Libre:
		marca = "🟢 descarga libre"
	case p.Backend == musica.BackendSpotDL:
		marca = "🟣 spotDL"
	case p.Preview != "":
		marca = "🟡 preview 30 s"
	}
	dur := ""
	if p.Duracion > 0 {
		dur = " · " + p.Minutos()
	}
	return fmt.Sprintf("%s — %s (%s · _%s_%s)",
		bot.Esc(p.Titulo), bot.Esc(p.Artista), marca, bot.Esc(p.Fuente), dur)
}

func respuestaSinPistas(b musica.Busqueda) string {
	if len(b.Fallos) == 0 {
		return "No aparece así. Prueba con menos palabras o con el artista delante («.play daft punk luck»)."
	}
	var sb strings.Builder
	sb.WriteString("Hoy las fuentes no responden bien: ")
	fuentes := make([]string, 0, len(b.Fallos))
	for fuente := range b.Fallos {
		fuentes = append(fuentes, fuente)
	}
	sort.Strings(fuentes)
	for _, fuente := range fuentes {
		fmt.Fprintf(&sb, "_%s_ (%s) ", bot.Esc(fuente), bot.Esc(cerca(b.Fallos[fuente], 160)))
	}
	sb.WriteString("Reintenta en un minuto.")
	return sb.String()
}

// motivoErrorMusica traduce los fallos del motor a respuestas de usuario: el
// error dice por qué falló de verdad (no un «ytdlp ha muerto» críptico).
func motivoErrorMusica(err error) string {
	s := err.Error()
	switch {
	case errors.Is(err, musica.ErrNoLibre):
		return "Esa pista no tiene audio completo en esta fuente; prueba *.escucha* para el preview oficial."
	case errors.Is(err, musica.ErrSpotDLNoConfigurado):
		return "spotDL no está habilitado en este servidor; usa una pista 🟢 o configura SPOTDL_PATH."
	case strings.Contains(strings.ToLower(s), "audioprovidererror") ||
		strings.Contains(strings.ToLower(s), "yt-dlp download error"):
		return "spotDL encontró la canción, pero su proveedor de audio rechazó la descarga. Prueba SPOTDL_COOKIE_FILE u otro SPOTDL_AUDIO."
	case strings.Contains(strings.ToLower(s), "ffmpeg"):
		return "spotDL necesita ffmpeg y no pudo usarlo; revisa SPOTDL_FFMPEG."
	case strings.Contains(s, "24 MB") || strings.Contains(s, "el audio pesa"):
		return "El archivo supera el tope de seguridad de 24 MB; usaría otra fuente o un enlace más corto."
	case strings.Contains(s, "se esperaba audio") || strings.Contains(s, "no parece MP3"):
		return "La fuente devolvió algo que no era MP3; cancelo antes de mandar basura."
	default:
		return "No pude bajar el audio esta vez (" + bot.Esc(cerca(s, 120)) + ")."
	}
}

// esSoloNumero: true si el texto era solo un número (el «.play 2»).
func esSoloNumero(s string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	return n, err == nil && n > 0
}

func cerca(s string, n int) string {
	if len([]rune(s)) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// yardaMusica recoge la descarga en un []byte: el motor ya aplica su tope.
type yardaMusica struct{ b *[]byte }

func (y *yardaMusica) Write(p []byte) (int, error) {
	*y.b = append(*y.b, p...)
	return len(p), nil
}
