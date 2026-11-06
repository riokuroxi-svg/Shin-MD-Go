// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// musica.go — búsqueda y descarga desde catálogos que sí permiten descargar.
//
// Tres fuentes, tres niveles de permiso:
//
//	Audius            catálogo libre y descentralizado: audio completo
//	Internet Archive  dominio público y colecciones abiertas: audio completo
//	Deezer / iTunes   catálogo comercial: solo el fragmento de 30 s que ellos
//	                  publican para previsualización
//
// Lo que este paquete NO hace: extraer audio de plataformas comerciales. El
// catálogo libre da archivo; el comercial da ficha y preview. La línea está en
// `Pista.Libre` y no es configurable a propósito.
package musica

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Fuentes soportadas.
const (
	FuenteAudius  = "audius"
	FuenteDeezer  = "deezer"
	FuenteITunes  = "itunes"
	FuenteArchivo = "archive"
)

// MaxAudio es el tope duro de una descarga. Un tema de 4 minutos en MP3 ronda
// los 4 MB; 24 MB deja sitio a directos y sesiones largas sin permitir que un
// enlace raro llene el disco de un teléfono.
const MaxAudio = 24 << 20

// Pista es el resultado unificado. Los campos vacíos no se inventan: si la
// fuente no los da, quedan en cero.
type Pista struct {
	Fuente   string `json:"fuente"`
	ID       string `json:"id"`
	Titulo   string `json:"titulo"`
	Artista  string `json:"artista"`
	Album    string `json:"album,omitempty"`
	Duracion int    `json:"duracion,omitempty"` // segundos
	Caratula string `json:"caratula,omitempty"`
	Preview  string `json:"preview,omitempty"` // fragmento oficial de 30 s
	Stream   string `json:"stream,omitempty"`  // audio completo (solo si Libre)
	Libre    bool   `json:"libre"`
	Origen   string `json:"origen,omitempty"` // página para atribución
}

// Minutos redondea la duración para mostrarla.
func (p Pista) Minutos() string {
	if p.Duracion <= 0 {
		return "—"
	}
	return fmt.Sprintf("%d:%02d", p.Duracion/60, p.Duracion%60)
}

// Cliente agrupa las tres fuentes con un http.Client con plazo, tope de tamaño
// y cabeceras honestas (el bot va identificado, no se hace pasar por un
// navegador para engañar a nadie).
type Cliente struct {
	HTTP      *http.Client
	Audius    string // base de la API
	UserAgent string
}

// Opciones de creación. Los ceros toman valores sanos.
type Opciones struct {
	Timeout   time.Duration
	AudiusURL string
	UserAgent string
}

// Nuevo crea el cliente.
func Nuevo(o Opciones) *Cliente {
	if o.Timeout <= 0 {
		o.Timeout = 15 * time.Second
	}
	if o.AudiusURL == "" {
		o.AudiusURL = "https://api.audius.co"
	}
	if o.UserAgent == "" {
		o.UserAgent = "Shin-MD/0.9 (+https://github.com/riokuroxi-svg/Shin-MD-Go)"
	}
	return &Cliente{
		HTTP: &http.Client{
			Timeout: o.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        16,
				IdleConnTimeout:     30 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
		Audius:    strings.TrimRight(o.AudiusURL, "/"),
		UserAgent: o.UserAgent,
	}
}

func (c *Cliente) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("musica: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musica: %s respondió %d", primerHost(u), resp.StatusCode)
	}
	// 8 MB es de sobra para una respuesta de metadatos; si llega más, algo raro
	// pasa y no se lee.
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func primerHost(u string) string {
	if p, err := url.Parse(u); err == nil {
		return p.Host
	}
	return "la fuente"
}

// ── Audius ──────────────────────────────────────────────────────────────

type audiusResp struct {
	Data []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Duration int    `json:"duration"`
		Genre    string `json:"genre"`
		User     struct {
			Name   string `json:"name"`
			Handle string `json:"handle"`
		} `json:"user"`
		// artwork trae tamaños como cadenas y "mirrors" como lista; se lee como
		// comodín y se filtra a mano para que una lista no tumbe el análisis.
		Artwork map[string]json.RawMessage `json:"artwork"`
		// is_streamable es la diferencia entre una pista que se puede bajar y
		// una que responde 404 al pedirle el flujo. Audius las devuelve igual
		// en la búsqueda, así que hay que filtrarlas aquí.
		IsStreamable bool   `json:"is_streamable"`
		Permalink    string `json:"permalink"`
	} `json:"data"`
}

// BuscarAudius consulta el catálogo libre. Devuelve pistas con audio completo.
func (c *Cliente) BuscarAudius(ctx context.Context, consulta string, limite int) ([]Pista, error) {
	if limite <= 0 || limite > 20 {
		limite = 5
	}
	u := fmt.Sprintf("%s/v1/tracks/search?query=%s&app_name=ShinMD&limit=%d",
		c.Audius, url.QueryEscape(consulta), limite)

	cuerpo, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var r audiusResp
	if err := json.Unmarshal(cuerpo, &r); err != nil {
		return nil, fmt.Errorf("musica: respuesta de Audius ilegible: %w", err)
	}

	pistas := make([]Pista, 0, len(r.Data))
	for _, t := range r.Data {
		if t.ID == "" || t.Title == "" || !t.IsStreamable {
			continue
		}
		p := Pista{
			Fuente:   FuenteAudius,
			ID:       t.ID,
			Titulo:   t.Title,
			Artista:  t.User.Name,
			Duracion: t.Duration,
			Libre:    true,
			Origen:   "https://audius.co" + t.Permalink,
			Stream: fmt.Sprintf("%s/v1/tracks/%s/stream?app_name=ShinMD",
				c.Audius, url.PathEscape(t.ID)),
		}
		for _, tam := range []string{"480x480", "1000x1000", "150x150"} {
			crudo, ok := t.Artwork[tam]
			if !ok {
				continue
			}
			var u string
			if err := json.Unmarshal([]byte(crudo), &u); err == nil && u != "" {
				p.Caratula = u
				break
			}
		}
		pistas = append(pistas, p)
	}
	return pistas, nil
}

// ── Deezer ──────────────────────────────────────────────────────────────

type deezerResp struct {
	Data []struct {
		ID       int    `json:"id"`
		Title    string `json:"title"`
		Duration int    `json:"duration"`
		Preview  string `json:"preview"`
		Link     string `json:"link"`
		Artist   struct {
			Name string `json:"name"`
		} `json:"artist"`
		Album struct {
			Title string `json:"title"`
			Cover string `json:"cover_medium"`
		} `json:"album"`
	} `json:"data"`
}

// BuscarDeezer devuelve fichas con el preview oficial de 30 segundos.
func (c *Cliente) BuscarDeezer(ctx context.Context, consulta string, limite int) ([]Pista, error) {
	if limite <= 0 || limite > 25 {
		limite = 5
	}
	u := "https://api.deezer.com/search?q=" + url.QueryEscape(consulta) +
		"&limit=" + strconv.Itoa(limite)

	cuerpo, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var r deezerResp
	if err := json.Unmarshal(cuerpo, &r); err != nil {
		return nil, fmt.Errorf("musica: respuesta de Deezer ilegible: %w", err)
	}

	pistas := make([]Pista, 0, len(r.Data))
	for _, t := range r.Data {
		pistas = append(pistas, Pista{
			Fuente:   FuenteDeezer,
			ID:       strconv.Itoa(t.ID),
			Titulo:   t.Title,
			Artista:  t.Artist.Name,
			Album:    t.Album.Title,
			Duracion: t.Duration,
			Caratula: t.Album.Cover,
			Preview:  t.Preview,
			Libre:    false,
			Origen:   t.Link,
		})
	}
	return pistas, nil
}

// ── iTunes ──────────────────────────────────────────────────────────────

type itunesResp struct {
	Results []struct {
		TrackName       string `json:"trackName"`
		ArtistName      string `json:"artistName"`
		CollectionName  string `json:"collectionName"`
		TrackTimeMillis int    `json:"trackTimeMillis"`
		ArtworkURL      string `json:"artworkUrl100"`
		PreviewURL      string `json:"previewUrl"`
		TrackViewURL    string `json:"trackViewUrl"`
		TrackID         int64  `json:"trackId"`
	} `json:"results"`
}

// BuscarITunes devuelve fichas con el preview oficial de Apple.
func (c *Cliente) BuscarITunes(ctx context.Context, consulta string, limite int) ([]Pista, error) {
	if limite <= 0 || limite > 50 {
		limite = 5
	}
	u := "https://itunes.apple.com/search?term=" + url.QueryEscape(consulta) +
		"&media=music&entity=song&limit=" + strconv.Itoa(limite)

	cuerpo, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	var r itunesResp
	if err := json.Unmarshal(cuerpo, &r); err != nil {
		return nil, fmt.Errorf("musica: respuesta de iTunes ilegible: %w", err)
	}

	pistas := make([]Pista, 0, len(r.Results))
	for _, t := range r.Results {
		if t.TrackName == "" || t.PreviewURL == "" {
			continue
		}
		pistas = append(pistas, Pista{
			Fuente:   FuenteITunes,
			ID:       strconv.FormatInt(t.TrackID, 10),
			Titulo:   t.TrackName,
			Artista:  t.ArtistName,
			Album:    t.CollectionName,
			Duracion: t.TrackTimeMillis / 1000,
			Caratula: strings.Replace(t.ArtworkURL, "100x100", "300x300", 1),
			Preview:  t.PreviewURL,
			Libre:    false,
			Origen:   t.TrackViewURL,
		})
	}
	return pistas, nil
}

// ── búsqueda combinada ──────────────────────────────────────────────────

// Resultado de una búsqueda combinada, con el error de cada fuente que falló.
// Una fuente caída no invalida las demás: es la diferencia entre "no hay
// resultados" y "hay resultados parciales y una fuente con problemas".
type Busqueda struct {
	Pistas []Pista
	Fallos map[string]string
}

// Buscar consulta las tres fuentes en paralelo y mezcla por relevancia simple:
// primero lo que tiene audio completo, después por duración conocida.
func (c *Cliente) Buscar(ctx context.Context, consulta string, porFuente int) Busqueda {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		res = Busqueda{Fallos: map[string]string{}}
	)
	agregar := func(fuente string, pistas []Pista, err error) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			res.Fallos[fuente] = err.Error()
			return
		}
		res.Pistas = append(res.Pistas, pistas...)
	}

	wg.Add(3)
	go func() {
		defer wg.Done()
		p, err := c.BuscarAudius(ctx, consulta, porFuente)
		agregar(FuenteAudius, p, err)
	}()
	go func() {
		defer wg.Done()
		p, err := c.BuscarDeezer(ctx, consulta, porFuente)
		agregar(FuenteDeezer, p, err)
	}()
	go func() {
		defer wg.Done()
		p, err := c.BuscarITunes(ctx, consulta, porFuente)
		agregar(FuenteITunes, p, err)
	}()
	wg.Wait()

	sort.SliceStable(res.Pistas, func(i, j int) bool {
		a, b := res.Pistas[i], res.Pistas[j]
		if a.Libre != b.Libre {
			return a.Libre
		}
		return a.Duracion > b.Duracion
	})
	return res
}

// ErrNoLibre se devuelve al intentar bajar audio completo de una pista que solo
// tiene preview. Es una negativa, no un fallo técnico.
var ErrNoLibre = errors.New("musica: esa pista no tiene audio libre; solo hay preview de 30 s")

// Descargar copia el audio de una pista libre a w y devuelve los bytes escritos.
// Respeta MaxAudio y no escribe nada si el contenido no parece audio.
func (c *Cliente) Descargar(ctx context.Context, p Pista, w io.Writer) (int64, error) {
	if !p.Libre || p.Stream == "" {
		return 0, ErrNoLibre
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Stream, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, fmt.Errorf("musica: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("musica: la descarga respondió %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxAudio {
		return 0, fmt.Errorf("musica: el audio pesa %d MB y el tope es %d MB",
			resp.ContentLength>>20, MaxAudio>>20)
	}
	if tipo := resp.Header.Get("Content-Type"); tipo != "" && !strings.HasPrefix(tipo, "audio/") &&
		!strings.Contains(tipo, "octet-stream") {
		return 0, fmt.Errorf("musica: se esperaba audio y llegó %q", tipo)
	}

	n, err := io.Copy(w, io.LimitReader(resp.Body, MaxAudio+1))
	if err != nil {
		return n, fmt.Errorf("musica: %w", err)
	}
	if n > MaxAudio {
		return n, fmt.Errorf("musica: el audio superó el tope de %d MB", MaxAudio>>20)
	}
	return n, nil
}
