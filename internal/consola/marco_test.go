// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// marco_test.go — la aritmética del ancho de la casa. Se mide en columnas
// reales, con color puesto y con 反魂 de por medio, porque es exactamente ahí
// donde los cuadros se descuadraban en Termux.
package consola

import (
	"fmt"
	"strings"
	"testing"
)

func TestAnchoFijoSegunVentana(t *testing.T) {
	casos := []struct{ ventana, quiere int }{
		{20, 40}, // más angosta que el mínimo: se deja en 40 y se trunca luego
		{40, 40}, // Termux con el teclado abierto
		{58, 58},
		{80, 58}, // una ventana normal NO ensancha el cuadro: 58 es la casa
		{120, 58},
		{300, 58},
	}
	for _, c := range casos {
		if got := encajar(c.ventana, 40, AnchoCasa); got != c.quiere {
			t.Errorf("encajar(%d) = %d, quería %d", c.ventana, got, c.quiere)
		}
	}
}

func TestMarcoCuadraEnCualquierAncho(t *testing.T) {
	relleno := []string{
		"",
		"una línea normal",
		"  sangría que hay que respetar al envolver",
		Papel("texto con color que no debe contar para el ancho"),
		"反魂 con kanji de dos columnas y una frase bastante más larga que el cuadro para ver si envuelve bien",
		"",
	}
	for _, ancho := range []int{40, 50, 58, 80, 120} {
		filas := Marco("título", relleno, ancho)
		if len(filas) < 3 {
			t.Fatalf("ancho %d: el marco salió de %d filas", ancho, len(filas))
		}
		fijo := encajar(ancho, 40, 100) // por encima de 100 la caja no se estira
		for i, l := range filas {
			if n := anchoVisible(l); n != fijo {
				t.Errorf("ancho %d, fila %d mide %d (quería %d): %q", ancho, i, n, fijo, SinColor(l))
			}
		}
		// Arriba y abajo cerrados, y nada de bloques sólidos.
		if !strings.HasPrefix(filas[0], "╭") || !strings.HasSuffix(filas[0], "╮") {
			t.Errorf("ancho %d: la esquina de arriba no cierra: %q", ancho, SinColor(filas[0]))
		}
		if !strings.HasPrefix(filas[len(filas)-1], "╰") || !strings.HasSuffix(filas[len(filas)-1], "╯") {
			t.Errorf("ancho %d: la esquina de abajo no cierra: %q", ancho, SinColor(filas[len(filas)-1]))
		}
		for _, l := range filas {
			if strings.ContainsAny(l, "█▓▒░┌┐└┘") {
				t.Errorf("usó un glifo que no es de la casa: %q", SinColor(l))
			}
			if l[0] != '\x1b' && !strings.HasPrefix(l, "│") && !strings.HasPrefix(l, "╭") && !strings.HasPrefix(l, "╰") {
				t.Errorf("fila sin borde: %q", SinColor(l))
			}
		}
	}
}

func TestMarcoRespetaLaSangriaIzquierda(t *testing.T) {
	// El bug del que avisaba el JS: envolver se comía los espacios de la
	// izquierda y el código quedaba pegado al borde. Aquí la sangría de dos
	// espacios tiene que seguir estando, aunque la línea se parta.
	filas := Marco("caja", []string{"  dos espacios al principio y luego mucho texto para que se vea obligado a partir la línea en dos"}, 40)
	encontrado := false
	for _, l := range filas {
		limpio := SinColor(l)
		if strings.HasPrefix(limpio, "│   ") {
			encontrado = true
		}
	}
	if !encontrado {
		t.Errorf("se comió la sangría al envolver:\n%s", strings.Join(mapFilas(filas, SinColor), "\n"))
	}
}

func mapFilas(en []string, f func(string) string) []string {
	out := make([]string, len(en))
	for i, s := range en {
		out[i] = f(s)
	}
	return out
}

func TestFilaAlineaElValorADerecha(t *testing.T) {
	for _, ancho := range []int{40, 58} {
		a := SinColor(Fila("conexión", "conectado", ancho))
		b := SinColor(Fila("memoria", "1.2G/1.9G", ancho))
		if len([]rune(a)) != len([]rune(b)) {
			t.Errorf("ancho %d: las filas no miden lo mismo: %q / %q", ancho, a, b)
		}
		if !strings.HasSuffix(b, "1.2G/1.9G") {
			t.Errorf("el valor tiene que quedar pegado a la derecha: %q", b)
		}
		if strings.Contains(a, "...") || strings.Contains(a, "···") {
			t.Logf("fila con puntos de guía: %q", a)
		}
	}
}

func TestEspaciarYCentrar(t *testing.T) {
	if got := Espaciar("R3RY-7KA2"); got != "R 3 R Y - 7 K A 2" {
		t.Errorf("Espaciar = %q", got)
	}
	if got := anchoVisible(Centrar("hola", 20)); got != 20 {
		t.Errorf("Centrar no rellenó: %d columnas", got)
	}
	if got := SinColor(Centrar("hola", 20)); got != "        hola        " {
		t.Errorf("Centrar mal centrado: %q", got)
	}
	// Lo que ya no cabe, no se centra a lo tonto: se trunca.
	if got := Truncar("una frase larga de más para este ancho", 10); anchoVisible(got) != 10 {
		t.Errorf("Truncar dio %d columnas: %q", anchoVisible(got), got)
	}
	if got := Truncar("corta", 40); got != "corta" {
		t.Errorf("Truncar no debe tocar lo que cabe: %q", got)
	}
}

func TestHistorialReleyendoNoSalta(t *testing.T) {
	h := &Historial{Max: 20}
	for i := 0; i < 20; i++ {
		h.Anadir(fmt.Sprintf("aviso %02d", i))
	}
	if h.Releyendo() {
		t.Error("sin retroceder no está releyendo")
	}
	ventana := h.Ventana(5)
	if len(ventana) != 5 || !strings.HasSuffix(ventana[4], "aviso 19") {
		t.Fatalf("Ventana(5) = %q", ventana)
	}
	h.Atras(4)
	if !h.Releyendo() {
		t.Error("tras Atras debería estar releyendo")
	}
	atras := h.Ventana(5)
	// Lo que se ve tras retroceder son las líneas anteriores, no las nuevas.
	if !strings.HasSuffix(atras[0], "aviso 11") {
		t.Error("retroceder no movió la ventana")
	}
	h.Abajo()
	if h.Releyendo() {
		t.Error("Abajo no volvió al final")
	}
	// Adelante de más no se sale por abajo.
	h.Adelante(99)
	if got := h.Ventana(5); !strings.HasSuffix(got[len(got)-1], "aviso 19") {
		t.Errorf("Adelante se salió del final: %q", got)
	}
}

func TestBannerCabeEnTermuxYNoUsaBloques(t *testing.T) {
	b := Banner("0.22.0")
	for _, l := range strings.Split(b, "\n") {
		if n := anchoVisible(l); n > AnchoCasa {
			t.Errorf("el banner saca una línea de %d columnas: %q", n, SinColor(l))
		}
	}
	for _, trozo := range []string{"S H I N - M D", "反魂", "0.22.0", "riokuroxi-svg", "AGPL-3.0-only", "go1."} {
		if !strings.Contains(b, trozo) {
			t.Errorf("al banner le falta %q", trozo)
		}
	}
	if strings.ContainsAny(b, "█▓▒░") {
		t.Error("el banner usa glifos de bloque")
	}
}

func TestPieNoDiceCtrlC_enServicio(t *testing.T) {
	// El pie es la promesa de cómo se para. Si no hay teclado, no puede
	// prometer Ctrl+C.
	pie := strings.Join(Pie(AnchoCasa), "\n")
	if EsServicio() {
		if strings.Contains(pie, "Ctrl+C") {
			t.Errorf("el pie promete Ctrl+C sin haber teclado:\n%s", pie)
		}
		if !strings.Contains(pie, "shin stop") {
			t.Errorf("el pie debe decir cómo se para desde fuera:\n%s", pie)
		}
	}
	// Y en ningún ancho puede pasarse de la pantalla: se mide del 32 al 100.
	for ancho := 32; ancho <= 100; ancho++ {
		for _, l := range Pie(ancho) {
			if n := anchoVisible(l); n > ancho {
				t.Errorf("el pie no cabe en %d: %d columnas (%q)", ancho, n, SinColor(l))
			}
		}
	}
}

// TestEnvolverCuentaElAnchoConColor: una línea de log empieza con el icono y la
// hora pintados. Antes el envoltorio medía esas palabras sin contarlas (o las
// partía, rompiendo el escape por medio) y el trozo salía más ancho que el
// límite: el terminal lo partía otra vez a media palabra. Que cada pieza quepa,
// salvo una palabra que por sí sola ya sea más ancha que el cuadro.
func TestEnvolverCuentaElAnchoConColor(t *testing.T) {
	base := Sakura("◐") + " " + Tenue("22:43:24") + "  " +
		strings.Repeat("palabra ", 14) + Menta("y-una-final-muy-larga-para-ver-que-no-se-parte")
	for _, limite := range []int{24, 40, 58} {
		for _, l := range Envolver(base, limite) {
			n := anchoVisible(l)
			if n <= limite {
				continue
			}
			if strings.Contains(l, " ") {
				t.Errorf("límite %d: trozo de %d columnas con espacios dentro: %q",
					limite, n, SinColor(l))
			}
		}
	}
}

// TestPrefijoUnaLineaYConHora: el registro del panel se compone con las líneas
// que pinta Prefijo; si alguna tuviera un salto dentro, el cuadro se descuadra,
// y si no trajera la hora, el historial saldría sin ella (o con dos).
func TestPrefijoUnaLineaYConHora(t *testing.T) {
	var b strings.Builder
	Prefijo(&b, "22:43:24", "AVISO", "el panel web no arrancó: puerto ocupado")
	s := b.String()
	if n := strings.Count(s, "\n"); n != 1 {
		t.Errorf("un aviso tiene que ser UNA línea (salió de %d): %q", n, SinColor(s))
	}
	if !strings.Contains(s, "22:43:24") {
		t.Errorf("la hora debe ir en la línea, no añadirla después: %q", SinColor(s))
	}
	if strings.Contains(s, "!") == false {
		t.Error("un AVISO tiene que llevar su símbolo")
	}
}

// TestBannerCabeEnCualquierVentana: la portada es lo primero que se ve y lo
// primero que se partía mal (la regla de guiones de 41 en 40 columnas, la firma
// cortada en «AGPL-3.0-o / nly»). Ninguna línea puede pasar del ancho, del
// terminal más tonto al más ancho.
func TestBannerCabeEnCualquierVentana(t *testing.T) {
	antes := anchoDePrueba
	defer func() { anchoDePrueba = antes }()
	for _, ancho := range []int{24, 30, 38, 40, 50, 58, 80, 120} {
		anchoDePrueba = ancho
		for i, l := range strings.Split(Banner("0.23.0"), "\n") {
			if n := anchoVisible(l); n > ancho {
				t.Errorf("ventana de %d: la línea %d del banner mide %d: %q", ancho, i, n, SinColor(l))
			}
		}
	}
	anchoDePrueba = 0
}

// TestBannerReglasCuadradas: las dos líneas de guiones del banner tienen que
// terminar en la misma columna. Es el detalle que hace que una pantalla se vea
// «cuidada» o «a medias», y sólo se comprueba midiendo.
func TestBannerReglasCuadradas(t *testing.T) {
	antes := anchoDePrueba
	defer func() { anchoDePrueba = antes }()
	for _, ancho := range []int{24, 30, 40, 50, 58, 80} {
		anchoDePrueba = ancho
		var reglas []int
		for _, l := range strings.Split(Banner("0.23.0"), "\n") {
			if strings.Contains(l, "─") {
				reglas = append(reglas, anchoVisible(SinColor(strings.TrimRight(l, "─")))+
					strings.Count(SinColor(l), "─"))
			}
		}
		if len(reglas) < 2 {
			t.Fatalf("ancho %d: el banner no trae sus dos reglas (%v)", ancho, reglas)
		}
		if reglas[0] != reglas[1] {
			t.Errorf("ancho %d: las reglas terminan en columnas distintas: %v", ancho, reglas)
		}
	}
	anchoDePrueba = 0
}
