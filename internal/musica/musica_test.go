// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package musica

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Respuestas recortadas de lo que devolvieron las APIs de verdad (nov-2026).
const fixtureAudius = `{"data":[
 {"id":"AK3w5q0","title":"iLLPeTiLL - 140 Mix","duration":584,"genre":"Dubstep","is_streamable":true,
  "user":{"name":"@iLLPeTiLL","handle":"illpetill"},
  "artwork":{"150x150":"https://img/150.jpg","480x480":"https://img/480.jpg"},
  "permalink":"/illpetill/mix-140"},
 {"id":"","title":"","duration":0},
 {"id":"95wro","title":"no se puede reproducir","duration":71,"is_streamable":false}
]}`

const fixtureDeezer = `{"data":[
 {"id":3135556,"title":"Instant Crush","duration":337,
  "preview":"https://cdnt-preview.dzcdn.net/api/1/1/d/6/b/0/abc.mp3",
  "link":"https://www.deezer.com/track/3135556",
  "artist":{"name":"Daft Punk"},
  "album":{"title":"Random Access Memories","cover_medium":"https://cdn/cover.jpg"}}
]}`

const fixtureITunes = `{"results":[
 {"trackId":617154241,"trackName":"Get Lucky","artistName":"Daft Punk",
  "collectionName":"Random Access Memories","trackTimeMillis":248000,
  "artworkUrl100":"https://is1/100x100bb.jpg",
  "previewUrl":"https://audio-ssl.itunes.apple.com/preview.m4a",
  "trackViewUrl":"https://music.apple.com/track/617154241"},
 {"trackId":1,"trackName":"sin preview","artistName":"X","previewUrl":""}
]}`

func clienteDePrueba(audius, deezer, itunes string) *Cliente {
	c := Nuevo(Opciones{Timeout: 3 * time.Second})
	c.Audius = audius
	return c
}

func servidor(t *testing.T, cuerpo string, codigo int) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("la petición salió sin User-Agent identificable")
		}
		w.WriteHeader(codigo)
		w.Write([]byte(cuerpo))
	}))
	t.Cleanup(s.Close)
	return s
}

func TestBuscarAudius(t *testing.T) {
	s := servidor(t, fixtureAudius, 200)
	c := Nuevo(Opciones{})
	c.Audius = s.URL

	pistas, err := c.BuscarAudius(context.Background(), "lofi", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(pistas) != 1 {
		t.Fatalf("devolvió %d pistas, quiero 1 (la vacía y la no reproducible se descartan)", len(pistas))
	}
	p := pistas[0]
	if p.Fuente != FuenteAudius || !p.Libre {
		t.Fatalf("la pista de Audius debe ser libre: %+v", p)
	}
	if p.Caratula != "https://img/480.jpg" {
		t.Fatalf("carátula = %q, quiero la de 480", p.Caratula)
	}
	if !strings.Contains(p.Stream, "/v1/tracks/AK3w5q0/stream") {
		t.Fatalf("stream inesperado: %s", p.Stream)
	}
	if p.Minutos() != "9:44" {
		t.Fatalf("Minutos() = %q, quiero 9:44", p.Minutos())
	}
}

// Caso real: Audius devuelve en la búsqueda pistas con is_streamable=false y su
// endpoint de flujo responde 404. Si no se filtran, el bot ofrece una canción
// que después no suena.
func TestAudiusDescartaNoReproducibles(t *testing.T) {
	s := servidor(t, fixtureAudius, 200)
	c := Nuevo(Opciones{})
	c.Audius = s.URL

	pistas, err := c.BuscarAudius(context.Background(), "lofi", 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range pistas {
		if p.ID == "95wro" {
			t.Fatal("se coló una pista con is_streamable=false")
		}
	}
}

func TestBuscarDeezerNoEsLibre(t *testing.T) {
	s := servidor(t, fixtureDeezer, 200)
	c := Nuevo(Opciones{})
	// Deezer va por URL fija; se sustituye el transporte del cliente para
	// dirigirlo al servidor de prueba.
	c.HTTP = s.Client()
	c.HTTP.Transport = redirigir(s.URL, "api.deezer.com")

	pistas, err := c.BuscarDeezer(context.Background(), "daft punk", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(pistas) != 1 {
		t.Fatalf("devolvió %d pistas", len(pistas))
	}
	if pistas[0].Libre {
		t.Fatal("Deezer nunca es libre: solo preview")
	}
	if pistas[0].Preview == "" {
		t.Fatal("falta el preview")
	}
}

func TestBuscarITunesDescartaSinPreview(t *testing.T) {
	s := servidor(t, fixtureITunes, 200)
	c := Nuevo(Opciones{})
	c.HTTP = s.Client()
	c.HTTP.Transport = redirigir(s.URL, "itunes.apple.com")

	pistas, err := c.BuscarITunes(context.Background(), "daft punk", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(pistas) != 1 {
		t.Fatalf("devolvió %d pistas, quiero 1", len(pistas))
	}
	if pistas[0].Duracion != 248 {
		t.Fatalf("duración = %d, quiero 248", pistas[0].Duracion)
	}
	if !strings.Contains(pistas[0].Caratula, "300x300") {
		t.Fatalf("la carátula debe pedirse a 300x300: %s", pistas[0].Caratula)
	}
}

func TestBuscarUnaFuenteCaidaNoTumbaLaBusqueda(t *testing.T) {
	caido := servidor(t, "boom", 503)
	vivo := servidor(t, fixtureITunes, 200)

	c := Nuevo(Opciones{Timeout: 2 * time.Second})
	c.Audius = caido.URL
	c.HTTP = vivo.Client()
	c.HTTP.Transport = redirigir(vivo.URL, "api.deezer.com", "itunes.apple.com")

	res := c.Buscar(context.Background(), "daft punk", 5)
	if len(res.Pistas) == 0 {
		t.Fatal("las fuentes vivas deben devolver resultados aunque otra falle")
	}
	if len(res.Fallos) == 0 {
		t.Fatal("debe quedar registrado el fallo de la fuente caída")
	}
}

func TestDescargarSeNiegaConPreview(t *testing.T) {
	c := Nuevo(Opciones{})
	p := Pista{Fuente: FuenteDeezer, Preview: "https://x/y.mp3", Libre: false}
	if _, err := c.Descargar(context.Background(), p, &bytes.Buffer{}); !errors.Is(err, ErrNoLibre) {
		t.Fatalf("err = %v, quiero ErrNoLibre", err)
	}
}

func TestDescargarCopiaAudio(t *testing.T) {
	audio := bytes.Repeat([]byte("ID3"), 1000)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write(audio)
	}))
	defer s.Close()

	c := Nuevo(Opciones{})
	var salida bytes.Buffer
	n, err := c.Descargar(context.Background(), Pista{Libre: true, Stream: s.URL}, &salida)
	if err != nil {
		t.Fatal(err)
	}
	if int(n) != len(audio) || salida.Len() != len(audio) {
		t.Fatalf("copió %d bytes, quiero %d", n, len(audio))
	}
	if !bytes.HasPrefix(salida.Bytes(), []byte("ID3")) {
		t.Fatal("el contenido no empieza por ID3")
	}
}

func TestDescargarRechazaLoQueNoEsAudio(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>no soy audio</html>"))
	}))
	defer s.Close()

	c := Nuevo(Opciones{})
	if _, err := c.Descargar(context.Background(), Pista{Libre: true, Stream: s.URL}, &bytes.Buffer{}); err == nil {
		t.Fatal("debía rechazar un contenido que no es audio")
	}
}

func TestDescargarCortaPorTope(t *testing.T) {
	// Anuncia un tamaño por encima del tope en la cabecera.
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Content-Length", "999999999")
		w.WriteHeader(200)
	}))
	defer s.Close()

	c := Nuevo(Opciones{})
	_, err := c.Descargar(context.Background(), Pista{Libre: true, Stream: s.URL}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "tope") {
		t.Fatalf("err = %v, quiero un rechazo por tope", err)
	}
}

func TestDescargarRespetaContexto(t *testing.T) {
	var pedidos atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedidos.Add(1)
		time.Sleep(300 * time.Millisecond)
	}))
	defer s.Close()

	c := Nuevo(Opciones{Timeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	inicio := time.Now()
	if _, err := c.Descargar(ctx, Pista{Libre: true, Stream: s.URL}, &bytes.Buffer{}); err == nil {
		t.Fatal("debía fallar por plazo")
	}
	if time.Since(inicio) > time.Second {
		t.Fatal("el plazo no se respetó: la descarga siguió demasiado tiempo")
	}
}

// redirigir manda al servidor de prueba solo las peticiones de los hosts
// indicados; el resto sigue su camino normal. Sin la lista se desviaría todo,
// incluida la fuente que la prueba quiere ver fallar.
func redirigir(base string, hosts ...string) http.RoundTripper {
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if len(hosts) > 0 {
			coincide := false
			for _, h := range hosts {
				if r.URL.Host == h {
					coincide = true
					break
				}
			}
			if !coincide {
				return http.DefaultTransport.RoundTrip(r)
			}
		}
		u := base + r.URL.Path
		if r.URL.RawQuery != "" {
			u += "?" + r.URL.RawQuery
		}
		nueva, err := http.NewRequestWithContext(r.Context(), r.Method, u, nil)
		if err != nil {
			return nil, err
		}
		nueva.Header = r.Header
		return http.DefaultTransport.RoundTrip(nueva)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
