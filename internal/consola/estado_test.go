// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// estado_test.go — el panel vivo, medido en columnas reales: que nada se
// duplique, que nada se salga del ancho y que los avisos no se pierdan.

package consola

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// ─── truncarANSI ──────────────────────────────────────────────────────────

func TestTruncarANSICortaAlAnchoVisible(t *testing.T) {
	casos := []struct {
		nombre string
		in     string
		ancho  int
		qUIere string // texto plano esperado tras el corte
	}{
		{"cabe entero", "hola", 80, "hola"},
		{"corta ascii", "abcdefghij", 4, "abc…"},
		{"corta kanji", "反魂反魂", 4, "反…"},
		{"cabe justo", "\x1b[95mrojo\x1b[0m negro", 10, "rojo negro"},
		{"respeta ansi y avisa", "\x1b[95mrojo\x1b[0m negro", 4, "roj…"},
		{"ancho cero no corta", "\x1b[1mlx", 0, "lx"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got := truncarANSI(c.in, c.ancho)
			if anchoVisible(got) > c.ancho && c.ancho > 0 {
				t.Errorf("ancho %d > %d en %q", anchoVisible(got), c.ancho, stripANSI(got))
			}
			if !strings.HasPrefix(stripANSI(got), c.qUIere) {
				t.Errorf("obtuv %q, quería prefijo %q", stripANSI(got), c.qUIere)
			}
		})
	}
}

func TestTruncarANSINuncaDejaEscapeIncompleto(t *testing.T) {
	// Un corte a mitad de secuencia dejaría color «pegado» al resto del
	// terminal (texto rosa para siempre). Siempre hay un reset al final.
	for _, ancho := range []int{1, 2, 3, 5, 7, 11, 13} {
		s := truncarANSI("\x1b[38;5;213mfucsia\x1b[0m y más", ancho)
		if i := strings.Index(s, "\x1b["); i >= 0 && !strings.HasSuffix(s, "\x1b[0m") {
			t.Errorf("ancho %d: %q no cierra el estilo", ancho, s)
		}
	}
}

func TestStripANSIQuitaTodo(t *testing.T) {
	got := stripANSI("\x1b[90m19:04\x1b[0m  \x1b[1m◑ listo\x1b[0m")
	if strings.Contains(got, "\x1b") {
		t.Errorf("quedan escapes: %q", got)
	}
	if !strings.Contains(got, "listo") {
		t.Errorf("se perdió el texto: %q", got)
	}
}

// ─── LineasEstado ────────────────────────────────────────────

func TestLineasEstadoNoDesbordaElAncho(t *testing.T) {
	e := Estado{
		Version: "0.22.0", Cuenta: "+52 1 55 1234 5678", Conectado: true,
		Prefijos: []string{".", "/", "#", "!"}, Comandos: 213,
		Uptime: 3 * time.Hour, Enviados: 7, Cupo: 20, EnCola: 2,
		Procesados: 41, Recibidos: 44, Web: "http://127.0.0.1:3000",
	}
	for _, ancho := range []int{40, 50, 60, 80, 120} {
		lineas := LineasEstado(e, ancho-2)
		if len(lineas) < 5 {
			t.Fatalf("ancho %d: solo %d líneas", ancho, len(lineas))
		}
		for i, l := range lineas {
			if w := anchoVisible(l); w > ancho-2 {
				t.Errorf("ancho %d, línea %d mide %d: %q", ancho, i, w, stripANSI(l))
			}
			if strings.ContainsAny(l, "\n") {
				t.Errorf("línea %d contiene un salto: rompe el repintado", i)
			}
		}
	}
}

func TestLineasEstadoOmiteLoQueNoHay(t *testing.T) {
	corto := LineasEstado(Estado{Version: "1"}, 78)
	largo := LineasEstado(Estado{Version: "1", Cuenta: "x", Cupo: 20, Comandos: 5, Web: "y"}, 78)
	if len(corto) >= len(largo) {
		t.Errorf("sin datos el panel no debería ser más largo: %d vs %d", len(corto), len(largo))
	}
	for _, l := range corto {
		if strings.Contains(stripANSI(l), "cupo") || strings.Contains(stripANSI(l), "cuenta") {
			t.Errorf("apareció una fila vacía: %q", stripANSI(l))
		}
	}
	if !strings.Contains(stripANSI(corto[0]), "desconectado") {
		t.Errorf("sin conexión debe decirlo: %q", stripANSI(corto[0]))
	}
}

func TestDuracionLegible(t *testing.T) {
	casos := map[time.Duration]string{
		7 * time.Second:               "0m 07s",
		2*time.Minute + 5*time.Second: "2m 05s",
		3*time.Hour + 40*time.Minute:  "3h 40m",
	}
	for d, quiere := range casos {
		if got := duracion(d); got != quiere {
			t.Errorf("%v → %q, quería %q", d, got, quiere)
		}
	}
}

// ─── Historial / Teletipo ────────────────────────────────────

func TestHistorialGuardaLasUltimasYNoCrece(t *testing.T) {
	h := &Historial{Max: 5}
	for i := 0; i < 50; i++ {
		h.Anadir("aviso")
	}
	if got := h.Ultimos(0); len(got) != 5 {
		t.Errorf("Max 5 → %d entradas", len(got))
	}
	if got := h.Ultimos(2); len(got) != 2 {
		t.Errorf("Ultimos(2) → %d", len(got))
	}
	// Copia: modificar lo devuelto no puede tocar el historial.
	hh := h.Ultimos(5)
	hh[0] = "manoseado"
	if h.Ultimos(5)[0] == "manoseado" {
		t.Error("Ultimos devuelve la lista interna, no una copia")
	}
}

func TestTeletipoAgorraMientrasElPanelVivo(t *testing.T) {
	var pantalla bytes.Buffer
	h := &Historial{Max: 10}
	tt := NuevoTeletipo(&pantalla, h)

	tt.Write([]byte("fuera, suelto\n"))
	if !strings.Contains(pantalla.String(), "fuera") {
		t.Error("sin panel activo debe escribir en la terminal")
	}
	if len(h.Ultimos(0)) != 0 {
		t.Error("sin panel activo no debe guardar nada")
	}

	repintados := 0
	tt.Activar(true, func() { repintados++ })
	tt.Write([]byte("\x1b[96m◑ dentro\x1b[0m\n\n"))
	if strings.Contains(pantalla.String(), "dentro") {
		t.Error("con el panel activo NO puede soltar líneas sueltas (duplica el dibujo)")
	}
	if len(h.Ultimos(0)) != 1 {
		t.Errorf("quería 1 aviso guardado, hay %d", len(h.Ultimos(0)))
	}
	if repintados != 1 {
		t.Errorf("cada aviso debe pedir un repintado; hubo %d", repintados)
	}
	if strings.Contains(stripANSI(h.Ultimos(1)[0]), "\n") {
		t.Error("el aviso debe guardarse en una sola línea")
	}

	tt.Activar(false, nil)
	tt.Write([]byte("otra vez suelto\n"))
	if !strings.Contains(pantalla.String(), "otra vez suelto") {
		t.Error("apagado el panel, los logs vuelven a la terminal")
	}
}

// ─── la pantalla (búfer alternativo) ─────────────────────────────────────

// TestPantallaEntraYSaleDelBufferAlternativo: entrar con ?1049h y salir con
// ?1049l, siempre por pares. Si se queda a medias, al usuario le queda la
// terminal «rota» (sin cursor, con el prompt encima del último cuadro).
func TestPantallaEntraYSaleDelBufferAlternativo(t *testing.T) {
	var b bytes.Buffer
	p := NuevaPantalla(&b, func() []string { return []string{"a", "b"} }, nil, nil)
	p.arrancar(15 * time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	p.Parar()

	salida := b.String()
	if !strings.Contains(salida, "\x1b[?1049h\x1b[?25l") {
		t.Error("no entró en el búfer alternativo / no escondió el cursor")
	}
	if !strings.HasSuffix(salida, "\x1b[?25h\x1b[?1049l") {
		t.Errorf("no salió limpio del búfer alternativo: %q", salida[len(salida)-24:])
	}
	if n := strings.Count(salida, "\x1b[?1049h"); n != 1 {
		t.Errorf("entró %d veces; una sola, o parpadea", n)
	}
	if n := strings.Count(salida, "\x1b[?1049l"); n != 1 {
		t.Errorf("salió %d veces", n)
	}
}

// TestPantallaLimpiaCadaLinea: cada cuadro va de la esquina, con \x1b[K al final
// de cada línea y \e[J al final del cuadro. Eso es lo que sustituye al «subir N
// líneas» que en Termux fallaba según el ancho.
func TestPantallaLimpiaCadaLinea(t *testing.T) {
	var b bytes.Buffer
	p := NuevaPantalla(&b, func() []string { return []string{"línea uno", "línea dos"} },
		func() []string { return []string{"q salir"} }, nil)
	p.arrancar(15 * time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	p.Parar()

	salida := b.String()
	if !strings.Contains(salida, "\x1b[H") {
		t.Error("no llevó el cursor a la esquina: se pintarían encima de otro sitio")
	}
	if strings.Count(salida, "\x1b[K") < 3 {
		t.Error("alguna línea no se limpió con \x1b[K (quedarían restos del cuadro anterior)")
	}
	if !strings.Contains(salida, "\x1b[J") {
		t.Error("no se borró el resto de la pantalla (quedaría basura debajo)")
	}
	// Salida sincronizada: abierta y cerrada las mismas veces, o la terminal
	// puede mostrar el barrido a medias (el «flash» que se veía antes).
	if a, c := strings.Count(salida, "\x1b[?2026h"), strings.Count(salida, "\x1b[?2026l"); a != c || a == 0 {
		t.Errorf("marcas de salida sincronizada despareadas: %d abiertas, %d cerradas", a, c)
	}
	// Cada refresco pinta el estado entero: no debe repetirse dos veces en la
	// misma pasada ni dejar líneas en blanco acumuladas.
	if strings.Contains(salida, "\n\n\n") {
		t.Error("acumuló líneas en blanco (en un móvil eso es scroll infinito)")
	}
	if got := strings.Count(stripANSI(salida), "línea uno"); got < 2 {
		t.Errorf("quería al menos 2 refrescos, hay %d", got)
	}
}

// TestPantallaNoSacaNadaPorFueraDeAncho: con ventana de 40, nada puede medir
// 41 — una línea de más hace scroll y descuadra el cuadro entero.
func TestPantallaRespetaElAnchoDeLaVentana(t *testing.T) {
	var b bytes.Buffer
	p := NuevaPantalla(&b, func() []string {
		return LineasEstado(Estado{
			Version: "0.22.0", Cuenta: "Anti", Conectado: true,
			Prefijos: []string{">", "=>"}, Comandos: 213, Uptime: 4231 * time.Second,
			Enviados: 3, Cupo: 157, EnCola: 2, Procesados: 9, Recibidos: 41,
			Web: "http://127.0.0.1:5560",
		}, 40)
	}, func() []string { return Pie(40) }, nil)
	p.medida = func() (int, int) { return 40, 24 } // como Termux en vertical
	p.arrancar(20 * time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	p.Parar()

	for _, l := range strings.Split(stripANSI(b.String()), "\r\n") {
		l = strings.TrimRight(l, "\x1b[J\x1b[0m")
		l = strings.TrimSuffix(strings.TrimSuffix(l, "\x1b[K"), "\r")
		if w := anchoVisible(l); w > 40 {
			t.Errorf("línea de %d columnas (máximo 40): %q", w, l)
		}
	}
}

// TestPantallaSinTerminalNoPintaNada: en systemd/pm2/nohup tiene que salir
// texto plano, ni un escape. Es lo que hace que sirva en un VPS.
func TestPantallaSinTerminalNoPintaNada(t *testing.T) {
	var b bytes.Buffer
	p := NuevaPantalla(&b, func() []string { return []string{"x"} }, nil, nil)
	if p.Arrancar(5 * time.Millisecond) {
		t.Error("Arrancar dijo que sí sin haber terminal")
	}
	time.Sleep(30 * time.Millisecond)
	p.Parar()
	if b.Len() != 0 {
		t.Errorf("sin TTY no se pinta nada (sería texto roto en el log): %q", b.String())
	}
}

// TestPantallaOcultarMostrar: `e` baja al registro y `v` vuelve; cada uno
// emite su secuencia una sola vez y no se pisa si se repite la tecla.
func TestPantallaOcultarMostrar(t *testing.T) {
	var b bytes.Buffer
	p := NuevaPantalla(&b, func() []string { return []string{"x"} }, nil, nil)
	p.arrancar(30 * time.Millisecond)
	b.Reset()
	p.Ocultar()
	if !strings.Contains(b.String(), "\x1b[?1049l") {
		t.Error("Ocultar no devolvió la terminal al usuario")
	}
	b.Reset()
	p.Ocultar() // segunda vez: no debe escribir nada más
	if b.Len() != 0 {
		t.Errorf("Ocultar dos veces escribió de más: %q", b.String())
	}
	b.Reset()
	p.Mostrar()
	if !strings.Contains(b.String(), "\x1b[?1049h") {
		t.Error("Mostrar no volvió a entrar en el búfer alternativo")
	}
	p.Parar()
}
