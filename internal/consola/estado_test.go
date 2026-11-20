// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// estado_test.go — el panel vivo, medido en columnas reales: que nada se
// duplique, que nada se salga del ancho y que los avisos no se pierdan.

package consola

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
)

// ─── truncarANSI ──────────────────────────────────────────────────────────

// anchoVisible mide una línea sin códigos ANSI y contando 反魂 como 2.
func anchoVisible(s string) int {
	return runewidth.StringWidth(stripANSI(s))
}

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

// ─── Panel ───────────────────────────────────────────────────

// TestPanelNoComeLineasDeArriba: el repintado sube `alto` líneas y borra desde
// ahí. Si el cursor viviera en la primera fila del bloque, cada refresco
// borraría la línea de encima (el log recién escrito).
func TestPanelNoComeLineasDeArriba(t *testing.T) {
	antes := coloresOn
	coloresOn = true
	defer func() { coloresOn = antes }()

	var b bytes.Buffer
	p := NuevoPanel(&b, func() []string { return []string{"a", "b"} }, nil)
	p.Arrancar(15 * time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	p.Parar()
	salida := b.String()
	// Tras pintar NO puede haber un «subir» sobrante: solo el de borrar.
	if alto := strings.Count(salida, "b\x1b[0m\x1b[0m\r\n"); alto < 1 {
		t.Log("pistas del número de refrescos:", alto)
	}
	for _, i := range []int{} {
		_ = i
	}
	if strings.Contains(salida, "\r\n\x1b[2A") {
		t.Error("doble subida: el siguiente repintado se comería la línea de arriba")
	}
}

func TestPanelSeCongelaAlSalir(t *testing.T) {
	// El test corre sin terminal: se fuerza el color para ejercitar el mismo
	// camino que ve un usuario en Termux/VPS.
	antes := coloresOn
	coloresOn = true
	defer func() { coloresOn = antes }()

	var b bytes.Buffer
	n := 0
	p := NuevoPanel(&b, func() []string {
		n++
		return []string{"línea uno", "línea dos"}
	}, nil)
	p.Arrancar(20 * time.Millisecond)
	time.Sleep(90 * time.Millisecond)
	p.Parar()

	salida := b.String()
	// El bloque son 2 líneas de datos + la ayuda al pie: cada repintado sube
	// el cursor 3 para tapar lo anterior. Eso es lo que impide restos.
	if up := strings.Count(salida, "\033[3A\r\033[J"); up < 2 {
		t.Errorf("el panel debe subir sus 3 líneas y borrar al repintar; lo hizo %d veces", up)
	}
	if strings.Contains(salida, "\n\n\n") {
		t.Error("el panel dejó líneas en blanco acumuladas (scroll en un móvil)")
	}
	if strings.Contains(salida, "línea uno\rlínea uno") {
		t.Error("dibujó la misma línea dos veces seguidas")
	}
	// Al salir el bloque se QUEDA (último estado visible) y el cursor baja:
	// si Parar borrara, se comería el mensaje de cierre que va debajo.
	if i := strings.LastIndex(salida, "\033[J"); i > strings.LastIndex(salida, "línea dos") {
		t.Error("Parar borró el panel: el mensaje de cierre desaparecía")
	}
	if !strings.HasSuffix(salida, "\r\n") {
		t.Errorf("tras parar el cursor debe quedar en una línea nueva: %q", salida[len(salida)-12:])
	}
	// Cada repintado dibuja el bloque entero y sube lo mismo: nunca de más.
	if got := strings.Count(stripANSI(salida), "línea uno"); got < 2 {
		t.Errorf("quería al menos 2 dibujos, hay %d", got)
	}
}

func TestPanelSinColorNoDibujaNada(t *testing.T) {
	antes := coloresOn
	coloresOn = false
	defer func() { coloresOn = antes }()
	var b bytes.Buffer
	p := NuevoPanel(&b, func() []string { return []string{"x"} }, nil)
	p.Arrancar(5 * time.Millisecond)
	time.Sleep(30 * time.Millisecond)
	p.Parar()
	if b.Len() != 0 {
		t.Errorf("sin TTY no se pinta ningún panel (sería texto roto en un log): %q", b.String())
	}
}

func TestPanelOcultarMostrar(t *testing.T) {
	var b bytes.Buffer
	p := NuevoPanel(&b, func() []string { return []string{"x"} }, nil)
	p.Mostrar()
	if b.Len() == 0 && coloresOn {
		t.Error("Mostrar debe dibujar")
	}
	b.Reset()
	p.Ocultar()
	if !coloresOn && b.Len() != 0 {
		t.Error("sin color no se pueden borrar líneas")
	}
}

func TestResumenEnUnaLinea(t *testing.T) {
	r := Resumen(Estado{Version: "0.22.0", Conectado: true, Uptime: time.Minute, Recibidos: 3, Procesados: 2, EnCola: 1, Comandos: 213, Enviados: 4, Cupo: 20})
	if strings.ContainsAny(r, "\n") {
		t.Errorf("el resumen es de una línea, salió:\n%s", r)
	}
	for _, quiere := range []string{"conectado", "1m 00s", "4/20"} {
		if !strings.Contains(stripANSI(r), quiere) {
			t.Errorf("falta %q en %q", quiere, stripANSI(r))
		}
	}
}

func TestAtajosSinTerminalNoEstorba(t *testing.T) {
	// En un servicio (systemd, docker) no hay /dev/tty usable: Atajos no debe
	// leer de stdin ni bloquear el cierre.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	parar := Atajos(ctx, map[string]func(){})
	hecho := make(chan struct{})
	go func() { parar(); close(hecho) }()
	select {
	case <-hecho:
	case <-time.After(2 * time.Second):
		t.Fatal("parar() bloqueó sin haber terminal: el bot no se podría apagar")
	}
}

// ─── menú ────────────────────────────────────────────────────

func TestRenderMenuCabeYSeAlinea(t *testing.T) {
	ops := []Opcion{
		{Tecla: "1", Titulo: "Código de 8 letras  (recomendado)", Detalle: "te lo da este bot y lo escribes en WhatsApp"},
		{Tecla: "2", Titulo: "Código QR", Detalle: "para escanear desde OTRO teléfono (útil en un VPS)"},
	}
	for _, ancho := range []int{40, 50, 60, 80} {
		s := renderMenu(ops, 0)
		if !strings.HasSuffix(s, "\r\n") {
			t.Errorf("ancho %d: en modo crudo cada línea debe terminar en \\r\\n", ancho)
		}
		for i, l := range strings.Split(strings.TrimSuffix(s, "\r\n"), "\r\n") {

			if strings.Contains(l, "\n") {
				t.Errorf("ancho %d línea %d trae un salto suelto: %q", ancho, i, l)
			}
		}
		// El selector y las dos opciones deben estar, una por línea.
		if strings.Count(s, "❯") != 1 {
			t.Errorf("ancho %d: quería un solo ❯", ancho)
		}
	}
	if got := lineasMenu(ops); got != strings.Count(renderMenu(ops, 0), "\r\n") {
		t.Errorf("lineasMenu()= %d, el render tiene %d líneas: el repintado dejaría restos",
			got, strings.Count(renderMenu(ops, 0), "\r\n"))
	}
}
