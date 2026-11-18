// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// musica.go — bloque B6.0: la busca y la baja de audio.
//
// El .play del JS sacaba audio de YouTube con un patio trasero de ytdl-core,
// pipelines rotos y yt-dlp: rápido cuando funcionaba, roto cada dos semanas
// y encima contra las condiciones de YouTube. Aquí el trato es otro, ya
// tomado en internal/musica: **solo catálogos que sí permiten bajar**.
//
//	.play <nombre>  busca en Audius + Deezer + iTunes en paralelo y baja el
//	                audio completo cuando la pista es libre; si solo hay
//	                ficha comercial, avisa en vez de piratearla.
//	.escucha         el preview oficial de 30 s que publican Deezer/iTunes;
//	                se manda de dónde sale para que no parezca magia.
//
// Cambios de UX contra el JS y por qué:
//
//	· Los botones nativos de WhatsApp (el JS mandaba quick-replies) no llegan
//	  por whatsmeow hoy: la lista queda numerada, «.play n» baja la n-ésima.
//	· Sin caché en RAM ni en disco: era el punto más frágil del play.js
//	  (entradas con valores clave defer dentro de defer, disco atascado) y en
//	  Go bajar una canción libre tarda segundos, no los minutos de yt-dlp.
//	· El semáforo por chat se conserva (máx. 2 bajadas a la vez): la escucha
//	  no tumba el bot.
//
// `.play` no es Priority: baja una canción de MB y gasta su cola ahí.
package cmds

import (
	"errors"
	"fmt"
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
		Desc: "Busca una canción en catálogos con permiso de descarga y la manda completa. «.play n» baja la n-ésima de la lista.",
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
		return c.Reply("¿Qué busco? Ejemplo: *.play villanos del sana*. Si ya hay una lista en pantalla, *.play 2* baja la 2.")
	}

	b := clienteMusica().Buscar(c.Ctx, caja, 5)
	guardarLista(c.Chat, b)

	libres, _ := cuentas(b.Pistas)
	switch {
	case len(b.Pistas) == 0:
		return c.Reply(respuestaSinPistas(b))
	case libres == 1 && len(b.Pistas) == 1:
		// Único resultado con audio completo: se baja de una, sin menú, como
		// el JS cuando yt daba una única URL.
		if err := descargarYEnviar(c, b.Pistas[0]); err != nil {
			return c.Reply(motivoErrorMusica(err))
		}
		return nil
	default:
		var sb strings.Builder
		fmt.Fprintf(&sb, "🎧 *%d resultados* para «%s» — %d con audio completo\n\n",
			len(b.Pistas), caja, libres)
		for i, p := range b.Pistas[:min(5, len(b.Pistas))] {
			fmt.Fprintf(&sb, "*%d.* %s\n", i+1, lineaPista(p))
		}
		if libres > 0 {
			sb.WriteString("\nResponde *.play N* para bajar una (solo las marcadas 🟢).")
		} else {
			sb.WriteString("\nNinguna tiene licencia de descarga directa; *.escucha " + caja + "* escucha el fragmento oficial.")
		}
		return c.Reply(sb.String())
	}
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
	if !p.Libre {
		return c.Reply(fmt.Sprintf("Esa (%s) no tiene descarga libre, solo ficha; *.escucha %s* oye 30 s.", p.Titulo, ult.consulta))
	}
	if err := descargarYEnviar(c, p); err != nil {
		return c.Reply(motivoErrorMusica(err))
	}
	return nil
}

// descargarYEnviar: semáforo, descarga con los filtros del motor, manda.
func descargarYEnviar(c *bot.Context, p musica.Pista) error {
	muMusica.Lock()
	if slotsMus[c.Chat] >= maxDescargasParalelas {
		muMusica.Unlock()
		return c.Reply("Ya tengo dos canciones bajando para este chat; cuando salgan, repite el comando.")
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

	var audio []byte
	// Descargar aplica sus filtros (audio real y ≤ 24 MB); aquí solo se recoge
	// en memoria para el envío.
	if _, err := clienteMusica().Descargar(c.Ctx, p, &yardaMusica{&audio}); err != nil {
		return err
	}
	return c.Bot.SendAudioPista(c.Ctx, c.Chat, audio, p.Titulo+" — "+p.Artista, p.Duracion, p.Origen, c.Priority())
}

// ── .escucha ─────────────────────────────────────────────────────────────────

func cmdEscucha(c *bot.Context) error {
	caja := strings.TrimSpace(c.Text)
	if caja == "" {
		return c.Reply("¿Qué escucho? Ejemplo: *.escucha daft punk*. Solo sale el fragmento de 30 s que publica la tienda.")
	}
	b := clienteMusica().Buscar(c.Ctx, caja, 5)
	for _, p := range b.Pistas {
		if p.Preview != "" {
			return c.Reply(fmt.Sprintf("🎵 *%s* — %s\n⏱ 30 s (preview oficial de %s)\n\n_Audio completo: en su página (%s)_", p.Titulo, p.Artista, p.Fuente, p.Origen))
		}
	}
	if len(b.Fallos) > 0 {
		return c.Reply(respuestaSinPistas(b))
	}
	return c.Reply("Ni Deezer ni iTunes tienen preview de eso; puede que solo exista en el catálogo libre (prueba *.play*).")
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

func cuentas(pistas []musica.Pista) (libres, conPreview int) {
	for _, p := range pistas {
		if p.Libre {
			libres++
		} else if p.Preview != "" {
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
	case p.Preview != "":
		marca = "🟡 preview 30 s"
	}
	dur := ""
	if p.Duracion > 0 {
		dur = " · " + p.Minutos()
	}
	return fmt.Sprintf("%s — %s (%s · _%s_%s)", p.Titulo, p.Artista, marca, p.Fuente, dur)
}

func respuestaSinPistas(b musica.Busqueda) string {
	if len(b.Fallos) == 0 {
		return "No aparece así. Prueba con menos palabras o con el artista delante («.play daft punk luck»)."
	}
	var sb strings.Builder
	sb.WriteString("Hoy las fuentes no responden bien: ")
	for f, why := range b.Fallos {
		fmt.Fprintf(&sb, "_%s_ (%s) ", f, why)
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
		return "Esa pista no tiene descarga libre, solo ficha y preview de 30 s. Prueba *.escucha*."
	case strings.Contains(s, "24 MB") || strings.Contains(s, "el audio pesa"):
		return "El archivo supera el tope de seguridad de 24 MB; usaría otra fuente o un enlace más corto."
	case strings.Contains(s, "se esperaba audio"):
		return "La fuente devolvió algo que no era audio; cancelo antes de mandar basura."
	default:
		return "No pude bajar el audio esta vez (" + cerca(s, 120) + ")."
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
