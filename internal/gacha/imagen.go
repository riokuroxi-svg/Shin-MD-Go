// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// imagen.go — Buscar la ilustración de un personaje.
//
// El JS consultaba tres tableros a la vez (safebooru, danbooru, gelbooru) con
// una clave de API de gelbooru escrita a mano dentro del código. Aquí:
//
//   - **una sola fuente**, safebooru, que sirve contenido etiquetado como
//     seguro y no pide clave;
//   - **sin claves** de ningún tipo dentro del código (una clave en un repo
//     público es una clave quemada);
//   - topes de tiempo y de tamaño, porque la URL la acaba eligiendo un
//     servicio de fuera y el comando lo dispara cualquiera: 8 s de espera y
//     4 MB de imagen como máximo.
//
// Si no hay imagen, el global sigue: el personaje se anuncia igual y se puede
// reclamar. En el JS, sin imagen el rol entero fallaba.
package gacha

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// MaxImageBytes: tope de la imagen descargada (4 MB).
	MaxImageBytes int64 = 4 << 20
	// imageTimeout: lo que se espera por la búsqueda y por la descarga.
	imageTimeout = 8 * time.Second
	// maxResultados: cuántas URL se piden al tablero.
	maxResultados = 40
)

// ErrSinImagen: no se encontró ilustración. No es un fallo del comando.
var ErrSinImagen = errors.New("sin imagen para ese personaje")

// safebooruEndpoint se puede cambiar desde las pruebas para no salir a la red.
var safebooruEndpoint = "https://safebooru.org/index.php"

// HTTPClient es el cliente que usa el buscador. main() no lo toca; las pruebas
// lo sustituyen por uno falso.
var HTTPClient = &http.Client{Timeout: imageTimeout}

// BuscarImagen devuelve los bytes de una imagen del personaje.
//
// `tag`: la etiqueta a buscar (normalmente el nombre del personaje o el primer
// tag del catálogo). `rnd`: para elegir entre los resultados; se pasa desde el
// comando para no compartir estado global de azar.
func BuscarImagen(ctx context.Context, tag string, elegir func(n int) int) ([]byte, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil, ErrSinImagen
	}

	urls, err := buscarURLs(ctx, tag)
	if err != nil || len(urls) == 0 {
		return nil, ErrSinImagen
	}
	if elegir != nil {
		urls = []string{urls[elegir(len(urls))]}
	}

	var ultimo error
	for _, u := range urls {
		img, err := descargar(ctx, u)
		if err == nil {
			return img, nil
		}
		ultimo = err
	}
	if ultimo == nil {
		ultimo = ErrSinImagen
	}
	return nil, ultimo
}

// BuscarImagenURL descarga una imagen de una URL concreta (la que traiga el
// catálogo), con los mismos topes que la búsqueda.
func BuscarImagenURL(ctx context.Context, u string) ([]byte, error) {
	u = strings.TrimSpace(u)
	if u == "" {
		return nil, ErrSinImagen
	}
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return nil, fmt.Errorf("url no permitida")
	}
	if !esImagenSegura(u) {
		return nil, ErrSinImagen
	}
	return descargar(ctx, u)
}

// buscarURLs pregunta a safebooru por imágenes del tag.
func buscarURLs(ctx context.Context, tag string) ([]string, error) {
	q := url.Values{}
	q.Set("page", "dapi")
	q.Set("s", "post")
	q.Set("q", "index")
	q.Set("json", "1")
	q.Set("limit", fmt.Sprint(maxResultados))
	q.Set("tags", normalizarTag(tag))

	endpoint := safebooruEndpoint + "?" + q.Encode()

	ctx, cancel := context.WithTimeout(ctx, imageTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Shin-MD-Go/1.0")
	req.Header.Set("Accept", "application/json")

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("safebooru respondió %d", resp.StatusCode)
	}
	// La respuesta es JSON, pero con 4 MB de tope por si acaso.
	cuerpo, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}

	var posts []map[string]any
	if err := json.Unmarshal(cuerpo, &posts); err != nil {
		return nil, err
	}

	base := baseImagenes()
	urls := make([]string, 0, len(posts))
	for _, p := range posts {
		directorio, _ := p["directory"].(string)
		imagen, _ := p["image"].(string)
		// Los nombres llegan de un servicio de fuera: se limpian antes de
		// pegarlos a una URL (si no, un "directory": "../../x" se sale de sitio).
		directorio = limpiarComponente(directorio)
		imagen = limpiarComponente(imagen)
		if directorio == "" || imagen == "" || !esImagenSegura(imagen) {
			continue
		}
		urls = append(urls, base+directorio+"/"+imagen)
	}
	return urls, nil
}

// descargar baja los bytes de la imagen con topes.
func descargar(ctx context.Context, u string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, imageTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Shin-MD-Go/1.0")
	req.Header.Set("Referer", "https://safebooru.org/")

	resp, err := HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("imagen: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "image/") {
		return nil, fmt.Errorf("eso no era una imagen (%s)", ct)
	}
	if resp.ContentLength > MaxImageBytes {
		return nil, fmt.Errorf("imagen demasiado grande (%d bytes)", resp.ContentLength)
	}

	// Se lee un byte de más para poder detectar que se pasó del tope.
	datos, err := io.ReadAll(io.LimitReader(resp.Body, MaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(datos)) > MaxImageBytes {
		return nil, fmt.Errorf("imagen demasiado grande")
	}
	if len(datos) == 0 {
		return nil, fmt.Errorf("imagen vacía")
	}
	return datos, nil
}

// baseImagenes: la carpeta de imágenes del mismo servidor que se consultó.
// Así las pruebas pueden apuntar a un servidor local.
func baseImagenes() string {
	u, err := url.Parse(safebooruEndpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "https://safebooru.org/images/"
	}
	return u.Scheme + "://" + u.Host + "/images/"
}

// limpiarComponente deja solo letras, números, guion y guion bajo, y exige que
// quede algo. Un componente vacío o con ".." se rechaza.
func limpiarComponente(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" || strings.Contains(out, "..") || strings.HasPrefix(out, ".") {
		return ""
	}
	return out
}

// esImagenSegura acepta solo jpg/png (lo que WhatsApp pinta sin recodificar).
func esImagenSegura(nombre string) bool {
	n := strings.ToLower(nombre)
	return strings.HasSuffix(n, ".jpg") || strings.HasSuffix(n, ".jpeg") || strings.HasSuffix(n, ".png")
}

// normalizarTag deja el tag como lo espera el tablero: minúsculas y guiones
// bajos en vez de espacios.
func normalizarTag(tag string) string {
	t := strings.ToLower(strings.TrimSpace(tag))
	t = strings.ReplaceAll(t, " ", "_")
	t = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			return r
		// Los tableros usan ASCII: "ñandú" se busca como "nandu".
		case r == 'á' || r == 'à' || r == 'ä' || r == 'â' || r == 'ã':
			return 'a'
		case r == 'é' || r == 'è' || r == 'ë' || r == 'ê':
			return 'e'
		case r == 'í' || r == 'ì' || r == 'ï' || r == 'î':
			return 'i'
		case r == 'ó' || r == 'ò' || r == 'ö' || r == 'ô' || r == 'õ':
			return 'o'
		case r == 'ú' || r == 'ù' || r == 'ü' || r == 'û':
			return 'u'
		case r == 'ñ':
			return 'n'
		case r == 'ç':
			return 'c'
		default:
			return -1
		}
	}, t)
	if len(t) > 64 {
		t = t[:64]
	}
	return t
}
