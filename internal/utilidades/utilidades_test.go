// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// utilidades_test.go — Morse, QR, encuestas y el filtro de enlaces de `.get`.
package utilidades

import (
	"bytes"
	"image/png"
	"net"
	"strings"
	"testing"
)

// ─── morse ──────────────────────────────────────────────────────────────────

func TestMorseIdaYVuelta(t *testing.T) {
	casos := []string{
		"hola mundo",
		"sos",
		"shin md 2026",
		"a b",
		"0123456789",
		"¿", // carácter que no existe en la tabla: se cuenta, no se inventa
	}
	for _, texto := range casos {
		codigo, fuera := ASimbolo(texto)
		if texto == "¿" {
			if fuera != 1 || codigo != "" {
				t.Fatalf("%q: fuera=%d codigo=%q", texto, fuera, codigo)
			}
			continue
		}
		vuelta, _ := ATexto(codigo)
		if vuelta != strings.ToLower(texto) {
			t.Fatalf("%q → %q → %q", texto, codigo, vuelta)
		}
	}
}

func TestMorseSeparaPalabrasConBarra(t *testing.T) {
	codigo, _ := ASimbolo("a b")
	if codigo != ".- / -..." {
		t.Fatalf("código = %q", codigo)
	}
	// Un espacio de más no deja «/ /».
	codigo, _ = ASimbolo("a   b")
	if strings.Contains(codigo, "/ /") {
		t.Fatalf("barras dobles: %q", codigo)
	}
}

func TestMorseCuentaLoQueNoSabe(t *testing.T) {
	// «ñ» no está en la tabla: se cuenta aparte en vez de meterse tal cual.
	codigo, fuera := ASimbolo("año")
	if fuera != 1 {
		t.Fatalf("fuera = %d (código %q)", fuera, codigo)
	}
	if strings.Contains(codigo, "Ñ") {
		t.Fatalf("se coló un carácter sin traducir: %q", codigo)
	}
}

func TestDemorseConBasura(t *testing.T) {
	// Los caracteres se pegan (así lo hacía el JS): el espacio entre palabras
	// es la barra, no el hueco entre grupos morse.
	texto, fuera := ATexto(".... --- .-.. .- xxx")
	if texto != "hola?" {
		t.Fatalf("texto = %q", texto)
	}
	if fuera != 1 {
		t.Fatalf("fuera = %d", fuera)
	}
	// Con barra sí hay espacio entre palabras.
	texto, _ = ATexto(".... --- .-.. .- / -- ..- -. -.. ---")
	if texto != "hola mundo" {
		t.Fatalf("con barra: %q", texto)
	}
}

func TestMorseVacio(t *testing.T) {
	if codigo, fuera := ASimbolo("   "); codigo != "" || fuera != 0 {
		t.Fatalf("vacío: %q, %d", codigo, fuera)
	}
	if texto, fuera := ATexto("  "); texto != "" || fuera != 0 {
		t.Fatalf("vacío: %q, %d", texto, fuera)
	}
}

// ─── QR ─────────────────────────────────────────────────────────────────────

func TestQRGeneraPNGValido(t *testing.T) {
	datos, err := ImagenQR("https://github.com/riokuroxi-svg/Shin-MD-Go")
	if err != nil {
		t.Fatalf("ImagenQR: %v", err)
	}
	img, err := png.Decode(bytes.NewReader(datos))
	if err != nil {
		t.Fatalf("el PNG no se puede decodificar: %v", err)
	}
	limites := img.Bounds()
	if limites.Dx() != limites.Dy() {
		t.Fatalf("el QR debería ser cuadrado: %dx%d", limites.Dx(), limites.Dy())
	}
	if limites.Dx() < 200 {
		t.Fatalf("demasiado pequeño para escanear: %d px", limites.Dx())
	}
	// Tiene que haber negro y blanco: una imagen de un solo color no es un QR.
	var negro, blanco int
	for y := limites.Min.Y; y < limites.Max.Y; y++ {
		for x := limites.Min.X; x < limites.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			if r == 0 && g == 0 && b == 0 {
				negro++
			} else {
				blanco++
			}
		}
	}
	if negro == 0 || blanco == 0 {
		t.Fatalf("imagen de un solo color: %d negros, %d blancos", negro, blanco)
	}
	// Zona de silencio: la esquina superior izquierda tiene que ser blanca o
	// muchos lectores no enganchan el código.
	if r, _, _, _ := img.At(1, 1).RGBA(); r != 0xffff {
		t.Fatalf("la esquina debería ser blanca (zona de silencio)")
	}
}

func TestQRRechazaVacioYDemasiadoLargo(t *testing.T) {
	if _, err := ImagenQR(""); err == nil {
		t.Fatal("un QR vacío no debería generarse")
	}
	if _, err := ImagenQR(strings.Repeat("a", MaxQR+1)); err == nil {
		t.Fatal("un contenido demasiado largo debería rechazarse")
	}
}

func TestQRContenidoLargoPeroValido(t *testing.T) {
	// Justo en el límite sí tiene que salir.
	if _, err := ImagenQR(strings.Repeat("a", MaxQR)); err != nil {
		t.Fatalf("con %d bytes debería funcionar: %v", MaxQR, err)
	}
}

// ─── encuestas ──────────────────────────────────────────────────────────────

func TestEncuestaNormal(t *testing.T) {
	e, err := ParsearEncuesta("¿Qué pizza?|Pepperoni|Hawaiana|4 Quesos")
	if err != nil {
		t.Fatalf("debería valer: %v", err)
	}
	if e.Tipo != TipoNormal || e.Pregunta != "¿Qué pizza?" || len(e.Opciones) != 3 {
		t.Fatalf("encuesta = %+v", e)
	}
	if e.Seleccionables != 1 || e.Correcta != -1 {
		t.Fatalf("una encuesta normal es de una respuesta: %+v", e)
	}
}

func TestEncuestaMultiple(t *testing.T) {
	e, err := ParsearEncuesta("multi ¿Qué días?|Lunes|Martes|Viernes")
	if err != nil {
		t.Fatalf("debería valer: %v", err)
	}
	if e.Seleccionables != 3 {
		t.Fatalf("seleccionables = %d", e.Seleccionables)
	}
	if e.Pregunta != "¿Qué días?" {
		t.Fatalf("la palabra «multi» se quedó en la pregunta: %q", e.Pregunta)
	}
}

func TestEncuestaQuiz(t *testing.T) {
	e, err := ParsearEncuesta("¿Capital de Japón?|Kioto|*Tokio|Osaka")
	if err != nil {
		t.Fatalf("debería valer: %v", err)
	}
	if e.Tipo != TipoQuiz || e.Correcta != 1 {
		t.Fatalf("quiz = %+v", e)
	}
	// El asterisco no se queda en el texto de la opción.
	if e.Opciones[1] != "Tokio" {
		t.Fatalf("opción con asterisco: %q", e.Opciones[1])
	}
	if e.Seleccionables != 1 {
		t.Fatalf("un quiz es de una sola respuesta: %d", e.Seleccionables)
	}

	// El asterisco al final también vale.
	e, err = ParsearEncuesta("¿2+2?|4*|5")
	if err != nil || e.Correcta != 0 || e.Opciones[0] != "4" {
		t.Fatalf("asterisco al final: %+v, %v", e, err)
	}
}

func TestEncuestaErrores(t *testing.T) {
	casos := map[string]string{
		"":                    "vacío",
		"sin plecas":          "sin-plecas",
		"¿Pregunta?|Una sola": "pocas",
		"|a|b":                "sin pregunta",
		"¿Pregunta?|*A|*B":    "dos correctas",
		"¿Pregunta?||":        "opciones vacías",
	}
	for entrada := range casos {
		if _, err := ParsearEncuesta(entrada); err == nil {
			t.Fatalf("%q debería fallar", entrada)
		}
	}

	// Justo en el límite, vale.
	justas := "¿Sí o no?" + strings.Repeat("|op", MaxOpciones)
	if _, err := ParsearEncuesta(justas); err != nil {
		t.Fatalf("con %d opciones debería valer: %v", MaxOpciones, err)
	}
	// Una más, no.
	muchas := "¿Sí o no?" + strings.Repeat("|op", MaxOpciones+1)
	if _, err := ParsearEncuesta(muchas); err == nil {
		t.Fatalf("con %d opciones debería fallar", MaxOpciones+1)
	}
}

func TestEncuestaMensajesDeError(t *testing.T) {
	_, err := ParsearEncuesta("sin plecas")
	if err == nil {
		t.Fatal("se esperaba error")
	}
	// El mensaje tiene que explicar el formato, no solo decir «error».
	if !strings.Contains(err.Error(), "|") {
		t.Fatalf("el error no explica el formato: %q", err.Error())
	}
}

// ─── .get: filtro de enlaces ───────────────────────────────────────────────

// resolverFalso devuelve las IPs que se le digan, para probar sin red.
func resolverFalso(ips ...string) func(string) ([]net.IP, error) {
	return func(string) ([]net.IP, error) {
		var out []net.IP
		for _, s := range ips {
			out = append(out, net.ParseIP(s))
		}
		return out, nil
	}
}

func TestValidarURLAceptaPublicas(t *testing.T) {
	r := resolverFalso("93.184.216.34")
	bien := []string{
		"https://example.com",
		"http://example.com/ruta?x=1",
		"https://sub.example.com/a/b/c",
	}
	for _, cruda := range bien {
		if _, err := ValidarURL(cruda, r); err != nil {
			t.Fatalf("%q debería aceptarse: %v", cruda, err)
		}
	}
}

func TestValidarURLRechazaInternas(t *testing.T) {
	casos := []string{
		"http://127.0.0.1/",
		"http://localhost:8080/admin", // resuelve a 127.0.0.1 (abajo)
		"http://192.168.1.1/",
		"http://10.0.0.5/",
		"http://172.16.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]/",
		"http://0.0.0.0/",
		"http://100.64.0.1/",
	}
	for _, cruda := range casos {
		// Dominios que resuelven a loopback / privadas.
		r := resolverFalso("127.0.0.1")
		if strings.Contains(cruda, "192.168") {
			r = resolverFalso("192.168.1.1")
		}
		if _, err := ValidarURL(cruda, r); err == nil {
			t.Fatalf("%q debería rechazarse", cruda)
		}
	}
}

func TestValidarURLRechazaFormasRaras(t *testing.T) {
	r := resolverFalso("93.184.216.34")
	males := []string{
		"",
		"ftp://example.com/archivo",
		"file:///etc/passwd",
		"example.com",                       // sin esquema
		"https://",                          // sin dominio
		"https://usuario:clave@example.com", // credenciales dentro
	}
	for _, cruda := range males {
		if _, err := ValidarURL(cruda, r); err == nil {
			t.Fatalf("%q debería rechazarse", cruda)
		}
	}
}

func TestValidarURLDominioQueApuntaAdentro(t *testing.T) {
	// El truco clásico: un dominio público que resuelve a la red interna.
	_, err := ValidarURL("https://mi-dominio.com/", resolverFalso("10.1.2.3"))
	if err == nil {
		t.Fatal("un dominio que resuelve a una IP privada debería rechazarse")
	}
	// Y si resuelve a una pública Y una privada, tampoco pasa.
	if _, err := ValidarURL("https://mixto.com/", resolverFalso("93.184.216.34", "127.0.0.1")); err == nil {
		t.Fatal("con una IP privada entre las respuestas debería rechazarse")
	}
}

func TestRecortarParaChat(t *testing.T) {
	// Los caracteres de control que rompen el envío se van.
	got := RecortarParaChat("hola\x00mundo\r\nfin")
	if strings.ContainsRune(got, 0) || strings.Contains(got, "\r") {
		t.Fatalf("quedaron controles: %q", got)
	}
	if !strings.Contains(got, "hola") || !strings.Contains(got, "fin") {
		t.Fatalf("se perdió texto: %q", got)
	}
	// Lo largo se recorta y se dice cuánto había.
	largo := RecortarParaChat(strings.Repeat("a", MaxTexto+500))
	if !strings.Contains(largo, "recortado") {
		t.Fatalf("no avisa del recorte")
	}
	if len([]rune(largo)) > MaxTexto+100 {
		t.Fatalf("el recorte no recortó: %d caracteres", len([]rune(largo)))
	}
}
