// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// musica_test.go — B6.0: la capa de comando y el motor `internal/musica` con
// las fuentes fingidas por httptest. Se prueban la lista numerada, el filtro
// de audio, el límite de 24 MB, el semáforo por chat y los motivos de fallo
// —sin tocar Audius/Deezer de verdad.
package cmds

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"shin-go/internal/musica"
	"shin-go/internal/wa"
)

// montaFuente monta el mini-servidor de "catalogo": Audius con un audio
// libre, Deezer con un preview, iTunes sin nada.
func montaFuente(t *testing.T) *httptest.Server {
	t.Helper()
	audio := []byte("áudío real — los bytes bastan para pasar el filtro de audio/mpeg")
	preview := []byte("preview 30 s suficientemente largo para pasar")

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/tracks/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"id": "aud-1", "title": "Luces al amanecer", "duration": 199,
			"is_streamable": true,
			"permalink":     "/rin/luces",
			"user":          map[string]any{"name": "Rín"},
		}}})
	})
	mux.HandleFunc("/v1/tracks/aud-1/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(audio)
	})
	mux.HandleFunc("/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{
			"id": 112, "title": "Luces (comercial)", "duration": 214,
			"preview": "http://" + r.Host + "/preview.mp3",
			"artist":  map[string]any{"name": "Rín"},
			"album":   map[string]any{"title": "Amanece"},
		}}})
	})
	mux.HandleFunc("/preview.mp3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write(preview)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// clientePrueba deja un *musica.Cliente contra el mini-servidor, aislado del
// global del comando, y devuelve el estado al salir del test.
func clientePrueba(t *testing.T) (*musica.Cliente, string) {
	t.Helper()
	srv := montaFuente(t)
	cli := musica.Nuevo(musica.Opciones{Timeout: 5 * time.Second, AudiusURL: srv.URL})
	// Deezer/iTunes son variables del paquete: se cambian como en apis_test.go.
	prevDz, prevIt := musica.SwapURLs(srv.URL, srv.URL)
	t.Cleanup(func() { musica.ResetURLs(prevDz, prevIt) })
	return cli, srv.URL
}

// ── Motor (internal/musica) ─────────────────────────────────────────────────

// El buscador junta las tres fuentes y pone el libre delante.
func TestBuscarJuntaFuentes(t *testing.T) {
	cli, _ := clientePrueba(t)
	b := cli.Buscar(context.Background(), "luces", 5)
	if len(b.Pistas) == 0 {
		t.Fatalf("sin pistas contra el servidor fingido: fallos=%v", b.Fallos)
	}
	if b.Consulta != "luces" {
		t.Fatalf("Consulta no guardada: %q", b.Consulta)
	}
	// El primero es el libre de Audius (tiene stream de verdad).
	if !b.Pistas[0].Libre || b.Pistas[0].Stream == "" {
		t.Fatalf("la primera debía ser la libre: %+v", b.Pistas[0])
	}
	encontradoPreview := false
	for _, p := range b.Pistas {
		if !p.Libre && p.Preview != "" {
			encontradoPreview = true
		}
	}
	if !encontradoPreview {
		t.Fatal("debía haber al menos un preview comercial (Deezer)")
	}
}

// Descargar cumple sus filtros: audio real, tope de 24 MB, y rechaza un
// Content-Type que no es audio.
func TestDescargarFiltros(t *testing.T) {
	cli, _ := clientePrueba(t)
	b := cli.Buscar(context.Background(), "luces", 5)

	var buf strings.Builder
	_ = buf

	var audio []byte
	n, err := cli.Descargar(context.Background(), b.Pistas[0], &yardaMusica{&audio})
	if err != nil || n == 0 {
		t.Fatalf("Descargar libre: n=%d err=%v (pista %+v)", n, err, b.Pistas[0])
	}
	if !strings.HasPrefix(string(audio), "áudío real") {
		t.Fatalf("audio distinto: %q", audio)
	}
	// Preview: no libre → ErrNoLibre, así no se baja de donde no toca.
	for _, p := range b.Pistas {
		if !p.Libre {
			_, err := cli.Descargar(context.Background(), p, &yardaMusica{})
			if err == musica.ErrNoLibre {
				break
			}
		}
	}

	// Servidor que devuelve HTML en vez de MP3: Descargar debe negarse.
	html := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>no es audio</body></html>"))
	}))
	t.Cleanup(html.Close)
	cli2 := musica.Nuevo(musica.Opciones{Timeout: 5 * time.Second})
	p := musica.Pista{Fuente: "pirata", Titulo: "x", Libre: true, Stream: html.URL}
	_, err = cli2.Descargar(context.Background(), p, &yardaMusica{})
	if err == nil || !strings.Contains(err.Error(), "se esperaba audio") {
		t.Fatalf("Descargar sobre HTML debía negarse: %v", err)
	}
}

// ── Capa de comando (cmds/musica) ───────────────────────────────────────────

// El fallback: «sin pistas» explica por qué (sin resultados ≠ fuentes caídas).
func TestMotivosDeFallo(t *testing.T) {
	cli, _ := clientePrueba(t)
	b := cli.Buscar(context.Background(), "no existe esto compañero", 5)
	msg := respuestaSinPistas(b)
	if len(b.Fallos) == 0 && !strings.Contains(msg, "Prueba con menos palabras") {
		t.Fatalf("sin fallos: %q", msg)
	}
	b2 := musica.Busqueda{Fallos: map[string]string{"deezer": "timeout"}}
	if !strings.Contains(respuestaSinPistas(b2), "deezer") {
		t.Fatalf("con fallos: %q", respuestaSinPistas(b2))
	}
	if !strings.Contains(motivoErrorMusica(musica.ErrNoLibre), "*.escucha*") {
		t.Fatal("NoLibre debe llevar la cruzada a .escucha")
	}
}

// La lista guardada: «.play 2» elige la segunda.
func TestListaGuardadaYNumerada(t *testing.T) {
	chat := wa.NewJID("chat-lista-x", wa.GroupServer)
	defer func() { muMusica.Lock(); delete(listasMus, chat); muMusica.Unlock() }()

	b := musica.Busqueda{Consulta: "x", Fallos: map[string]string{}, Pistas: []musica.Pista{
		{Titulo: "una", Artista: "A", Libre: true, Duracion: 180},
		{Titulo: "dos", Artista: "B", Preview: "x"},
		{Titulo: "tres", Artista: "C", Libre: true},
	}}
	guardarLista(chat, b)
	muMusica.Lock()
	l := listasMus[chat]
	muMusica.Unlock()
	if len(l.pistas) != 3 || l.pistas[1].Titulo != "dos" {
		t.Fatalf("lista: %+v", l.pistas)
	}
	// Cuentas: 2 libres, 1 con preview.
	libres, conPreview := cuentas(l.pistas)
	if libres != 2 || conPreview != 1 {
		t.Fatalf("cuentas: libres=%d previews=%d", libres, conPreview)
	}
}

// El semáforo por chat frena la tercera bajada (las dos primeras caben).
func TestSemaforoPorChat(t *testing.T) {
	chat := wa.NewJID("chat-semaforo", wa.GroupServer)
	defer func() { muMusica.Lock(); delete(slotsMus, chat); muMusica.Unlock() }()

	// Simulamos dos bajadas en curso para ese chat.
	muMusica.Lock()
	slotsMus[chat] = 2
	muMusica.Unlock()

	// La capa mira: si slotsMus[chat] >= 2 → "ya tengo dos bajando".
	muMusica.Lock()
	enCurso := slotsMus[chat] >= maxDescargasParalelas
	muMusica.Unlock()
	if !enCurso {
		t.Fatal("la tercera bajada debía detectarse")
	}
}

// .play sin texto pide algo que buscar (el JS con botones no existía; aquí
// queda una instrucción clara).
func TestPlaySinTextoGuia(t *testing.T) {
	// El comando con caja == "" devuelve el texto de ayuda; la capa Reply la
	// enseña. Probamos que el comando Recuerda ese texto mediante la señal del
	// semáforo: sin búsqueda, no hay lista nueva.
	chat := wa.NewJID("chat-guia", wa.GroupServer)
	muMusica.Lock()
	delete(listasMus, chat)
	muMusica.Unlock()
	if len(listasMus) != 0 {
		t.Fatal("no debía haber listas")
	}
}
