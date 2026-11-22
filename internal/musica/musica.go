// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// musica.go — búsqueda y descarga de audio con proveedores diferenciados.
//
// El motor integrado mantiene tres niveles de permiso:
//
//	Audius            catálogo libre y descentralizado: audio completo
//	Internet Archive  dominio público y colecciones abiertas: audio completo
//	Deezer / iTunes   catálogo comercial: solo el fragmento de 30 s que ellos
//	                  publican para previsualización
//
// Opcionalmente, el operador puede configurar un ejecutable spotDL externo.
// Ese camino queda marcado con BackendSpotDL, separado de `Pista.Libre`: el bot
// no presenta como «libre» algo que spotDL resolvió mediante otro proveedor.
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

	"shin-go/internal/utilidades"
)

// Fuentes soportadas.
const (
	FuenteAudius  = "audius"
	FuenteDeezer  = "deezer"
	FuenteITunes  = "itunes"
	FuenteArchivo = "archive"
)

// urlDeezer / urlITunes: como var para que las pruebas las apunten a
// httptest igual que se hace en internal/apis (ver apis_test.go).
var urlDeezer = "https://api.deezer.com"
var urlITunes = "https://itunes.apple.com"

// SwapURLs mueve Deezer e iTunes a otro host (solo en pruebas) y devuelve los
// previos para ResetURLs. ResetURLs los restaura. Mismo patrón que apis.
func SwapURLs(dz, it string) (prevDz, prevIt string) {
	prevDz, prevIt = urlDeezer, urlITunes
	urlDeezer, urlITunes = dz, it
	return
}
func ResetURLs(dz, it string) { urlDeezer, urlITunes = dz, it }

// MaxAudio es el tope duro de una descarga. Un tema de 4 minutos en MP3 ronda
// los 4 MB; 24 MB deja sitio a directos y sesiones largas sin permitir que un
// enlace raro llene el disco de un teléfono.
const (
	MaxAudio   = 24 << 20
	MaxPreview = 8 << 20
	MaxArtwork = 8 << 20
)

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
	Stream   string `json:"stream,omitempty"`  // audio completo de una fuente libre
	Libre    bool   `json:"libre"`
	Origen   string `json:"origen,omitempty"` // página para atribución

	// Backend y Entrada seleccionan un proveedor externo configurado por el
	// operador. Entrada no se serializa y el adaptador valida su URL canónica.
	Backend string `json:"backend,omitempty"`
	Entrada string `json:"-"`
}

func (p Pista) Descargable() bool {
	return p.Libre || (p.Backend == BackendSpotDL && p.Entrada != "")
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
	spotdl    *spotDL
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

func (c *Cliente) get(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := c.validarURLRemota(rawURL)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.doSeguro(req)
	if err != nil {
		return nil, fmt.Errorf("musica: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musica: %s respondió %d", primerHost(rawURL), resp.StatusCode)
	}
	// 8 MB es de sobra para una respuesta de metadatos; si llega más, algo raro
	// pasa y no se lee.
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

func (c *Cliente) validarURLRemota(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Hostname() == "" || u.User != nil ||
		(u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("musica: URL remota inválida")
	}
	if c.esOrigenConfigurado(u) {
		// Los endpoints configurados por el operador pueden ser HTTP (útil para
		// un proxy local y para httptest). Los valores predeterminados son HTTPS.
		return u, nil
	}
	validada, err := utilidades.ValidarURL(u.String(), utilidades.ResolverDNSIPs)
	if err != nil {
		return nil, fmt.Errorf("musica: URL remota rechazada: %w", err)
	}
	if validada.Scheme != "https" {
		return nil, errors.New("musica: el medio remoto debe usar https")
	}
	return validada, nil
}

func (c *Cliente) esOrigenConfigurado(u *url.URL) bool {
	if u == nil {
		return false
	}
	for _, raw := range []string{c.Audius, urlDeezer, urlITunes} {
		base, err := url.Parse(raw)
		if err == nil && base.Host != "" && strings.EqualFold(base.Host, u.Host) &&
			strings.EqualFold(base.Scheme, u.Scheme) {
			return true
		}
	}
	return false
}

func (c *Cliente) doSeguro(req *http.Request) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, errors.New("musica: petición vacía")
	}
	if _, err := c.validarURLRemota(req.URL.String()); err != nil {
		return nil, err
	}
	base := c.HTTP
	if base == nil {
		base = http.DefaultClient
	}
	client := *base
	previousRedirect := client.CheckRedirect
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("musica: demasiadas redirecciones")
		}
		if _, err := c.validarURLRemota(next.URL.String()); err != nil {
			return err
		}
		if previousRedirect != nil {
			return previousRedirect(next, via)
		}
		return nil
	}
	return client.Do(req)
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
	u := urlDeezer + "/search?q=" + url.QueryEscape(consulta) +
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
	u := urlITunes + "/search?term=" + url.QueryEscape(consulta) +
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
	Pistas   []Pista
	Fallos   map[string]string
	Consulta string // la frase tal cual la pidió el usuario, para contestar
}

// Buscar consulta las tres fuentes en paralelo y mezcla por relevancia simple:
// primero lo que tiene audio completo, después por duración conocida.
func (c *Cliente) Buscar(ctx context.Context, consulta string, porFuente int) Busqueda {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		res = Busqueda{Fallos: map[string]string{}, Consulta: consulta}
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
		if a.Duracion != b.Duracion {
			return a.Duracion > b.Duracion
		}
		if a.Fuente != b.Fuente {
			return a.Fuente < b.Fuente
		}
		if a.Titulo != b.Titulo {
			return a.Titulo < b.Titulo
		}
		if a.Artista != b.Artista {
			return a.Artista < b.Artista
		}
		return a.ID < b.ID
	})
	return res
}

// ErrNoLibre se devuelve al intentar bajar audio completo de una pista que solo
// tiene preview. Es una negativa, no un fallo técnico.
var ErrNoLibre = errors.New("musica: esa pista no tiene audio libre; solo hay preview de 30 s")

// Descargar copia el audio a w desde la fuente libre o desde el backend spotDL
// habilitado por el operador. Ambos caminos respetan MaxAudio y validan formato.
func (c *Cliente) Descargar(ctx context.Context, p Pista, w io.Writer) (int64, error) {
	if p.Backend == BackendSpotDL {
		if !c.TieneSpotDL() {
			return 0, ErrSpotDLNoConfigurado
		}
		return c.spotdl.descargar(ctx, p, w)
	}
	if !p.Libre || p.Stream == "" {
		return 0, ErrNoLibre
	}
	streamURL, err := c.validarURLRemota(p.Stream)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.doSeguro(req)
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

// DescargarPreview copia únicamente el fragmento oficial publicado por la
// tienda. Se mantiene separado de Descargar para no presentar un preview como
// audio completo o libre.
func (c *Cliente) DescargarPreview(ctx context.Context, p Pista, w io.Writer) (int64, error) {
	if strings.TrimSpace(p.Preview) == "" {
		return 0, errors.New("musica: esa pista no tiene preview")
	}
	previewURL, err := c.validarURLRemota(p.Preview)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, previewURL.String(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.doSeguro(req)
	if err != nil {
		return 0, fmt.Errorf("musica: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, fmt.Errorf("musica: el preview respondió %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxPreview {
		return 0, fmt.Errorf("musica: el preview supera %d MB", MaxPreview>>20)
	}
	if tipo := resp.Header.Get("Content-Type"); tipo != "" && !strings.HasPrefix(tipo, "audio/") &&
		!strings.Contains(tipo, "octet-stream") {
		return 0, fmt.Errorf("musica: se esperaba audio y llegó %q", tipo)
	}
	n, err := io.Copy(w, io.LimitReader(resp.Body, MaxPreview+1))
	if err != nil {
		return n, fmt.Errorf("musica: %w", err)
	}
	if n > MaxPreview {
		return n, fmt.Errorf("musica: el preview superó %d MB", MaxPreview>>20)
	}
	return n, nil
}

// DescargarCaratula trae una portada indicada por un proveedor, con defensa
// SSRF, redirecciones revalidadas, MIME de imagen y límite de memoria.
func (c *Cliente) DescargarCaratula(ctx context.Context, p Pista) ([]byte, error) {
	u, err := c.validarURLRemota(p.Caratula)
	if err != nil {
		return nil, fmt.Errorf("musica: carátula rechazada: %w", err)
	}
	if u.Scheme != "https" {
		return nil, errors.New("musica: la carátula debe usar https")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.doSeguro(req)
	if err != nil {
		return nil, fmt.Errorf("musica: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musica: la carátula respondió %d", resp.StatusCode)
	}
	if resp.ContentLength > MaxArtwork {
		return nil, fmt.Errorf("musica: la carátula supera %d MB", MaxArtwork>>20)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxArtwork+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxArtwork {
		return nil, fmt.Errorf("musica: la carátula superó %d MB", MaxArtwork>>20)
	}
	tipo := strings.SplitN(http.DetectContentType(data), ";", 2)[0]
	switch tipo {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
		return data, nil
	default:
		return nil, fmt.Errorf("musica: la carátula no era una imagen (%s)", tipo)
	}
}
