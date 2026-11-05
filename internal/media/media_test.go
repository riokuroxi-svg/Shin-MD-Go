// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// media_test.go — El sticker de principio a fin, sin WhatsApp de por medio.
//
// Aquí se comprueba lo que de verdad puede romperse en silencio: que el WebP
// final se pueda volver a leer, que el bloque EXIF quede en el sitio donde
// WhatsApp lo busca, que las formas recorten de verdad y que los filtros no
// inventen colores. Un sticker mal montado no da error: se manda y WhatsApp lo
// muestra como "archivo no compatible".
package media

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"os/exec"
	"testing"

	"github.com/HugoSmits86/nativewebp"
)

// lienzo crea una imagen de prueba con un cuadro de color en el centro.
func lienzo(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}
	return img
}

func pngDe(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}
	return buf.Bytes()
}

// ─── detección de formato ───────────────────────────────────────────────────

func TestDetect(t *testing.T) {
	casos := map[string][]byte{
		"png":   pngDe(t, lienzo(4, 4)),
		"webp":  nil, // se rellena abajo
		"jpeg":  nil,
		"gif":   nil,
		"video": append([]byte{0, 0, 0, 0x20}, []byte("ftypisom")...),
	}
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, lienzo(4, 4), nil); err != nil {
		t.Fatalf("jpeg: %v", err)
	}
	casos["jpeg"] = jpg.Bytes()

	var w bytes.Buffer
	if err := nativewebp.Encode(&w, lienzo(4, 4), nil); err != nil {
		t.Fatalf("webp: %v", err)
	}
	casos["webp"] = w.Bytes()

	var g bytes.Buffer
	if err := gif.Encode(&g, lienzo(4, 4), nil); err != nil {
		t.Fatalf("gif: %v", err)
	}
	casos["gif"] = g.Bytes()

	for esperado, datos := range casos {
		if got := Detect(datos); got != esperado {
			t.Fatalf("Detect = %q, se esperaba %q", got, esperado)
		}
	}
	if got := Detect([]byte("hola")); got != "" {
		t.Fatalf("Detect de basura = %q", got)
	}
	// Y todos los formatos de imagen se tienen que poder leer.
	for _, nombre := range []string{"png", "jpeg", "webp", "gif"} {
		if _, err := Decode(casos[nombre]); err != nil {
			t.Fatalf("Decode(%s): %v", nombre, err)
		}
	}
	if _, err := Decode([]byte("no soy una imagen, ni de lejos")); err == nil {
		t.Fatal("Decode aceptó basura")
	}
}

// ─── escalado ───────────────────────────────────────────────────────────────

func TestFitContenerRellenaConTransparencia(t *testing.T) {
	// Una imagen panorámica (400×100) tiene que quedar centrada y dejar
	// transparentes arriba y abajo; si no, el sticker sale deformado.
	out := Fit(lienzo(400, 100), 512, Contener)
	if out.Bounds().Dx() != 512 || out.Bounds().Dy() != 512 {
		t.Fatalf("tamaño = %v, se esperaba 512×512", out.Bounds())
	}
	if a := out.NRGBAAt(0, 0).A; a != 0 {
		t.Fatalf("la esquina tiene alfa %d, se esperaba 0 (transparente)", a)
	}
	if a := out.NRGBAAt(256, 256).A; a != 255 {
		t.Fatalf("el centro tiene alfa %d, se esperaba 255", a)
	}
	// La banda pintada ocupa 512×128 y está centrada: entre 192 y 320.
	if a := out.NRGBAAt(256, 200).A; a != 255 {
		t.Fatalf("la banda de la imagen tiene alfa %d", a)
	}
	if a := out.NRGBAAt(256, 150).A; a != 0 {
		t.Fatalf("fuera de la banda hay alfa %d, se esperaba 0", a)
	}
}

func TestFitCubrirNoDejaHuecos(t *testing.T) {
	out := Fit(lienzo(400, 100), 512, Cubrir)
	for _, p := range []image.Point{{0, 0}, {511, 0}, {0, 511}, {511, 511}, {256, 256}} {
		if a := out.NRGBAAt(p.X, p.Y).A; a != 255 {
			t.Fatalf("el punto %v tiene alfa %d con Cubrir", p, a)
		}
	}
}

func TestFitDeUnaSolaLinea(t *testing.T) {
	// Una imagen de 1×1 no puede dividir por cero ni salir vacía.
	out := Fit(lienzo(1, 1), 512, Contener)
	if out.Bounds().Dx() != 512 {
		t.Fatalf("tamaño = %v", out.Bounds())
	}
	if a := out.NRGBAAt(256, 256).A; a != 255 {
		t.Fatalf("el centro de un 1×1 escalado tiene alfa %d", a)
	}
}

// ─── formas ─────────────────────────────────────────────────────────────────

func TestMascaraCirculo(t *testing.T) {
	img := lienzo(512, 512)
	m, ok := Mascara("circulo")
	if !ok {
		t.Fatal("no existe la forma circulo")
	}
	m.Aplicar(img)
	if a := img.NRGBAAt(2, 2).A; a != 0 {
		t.Fatalf("la esquina tiene alfa %d, tiene que quedar fuera del círculo", a)
	}
	if a := img.NRGBAAt(256, 256).A; a != 255 {
		t.Fatalf("el centro tiene alfa %d", a)
	}
	// El borde del círculo es suave (antialias), no un corte a cuchillo: por
	// la imagen tiene que haber píxeles a medio transparentar.
	suaves := 0
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			if a := img.NRGBAAt(x, y).A; a > 0 && a < 255 {
				suaves++
			}
		}
	}
	if suaves < 100 {
		t.Fatalf("el borde del círculo no está suavizado (%d píxeles de borde)", suaves)
	}
}

func TestTodasLasMascarasRecortanAlgo(t *testing.T) {
	for _, nombre := range NombresMascaras() {
		m, ok := Mascara(nombre)
		if !ok || m.dentro == nil {
			t.Fatalf("la forma %q no tiene función", nombre)
		}
		img := lienzo(64, 64)
		m.Aplicar(img)
		// Alguna esquina tiene que quedar transparente: si no, la forma no hace
		// nada (es el error clásico al cambiar de máscara).
		if img.NRGBAAt(0, 0).A == 255 && img.NRGBAAt(63, 0).A == 255 &&
			img.NRGBAAt(0, 63).A == 255 && img.NRGBAAt(63, 63).A == 255 {
			t.Fatalf("la forma %q no recortó ninguna esquina", nombre)
		}
	}
}

func TestAliasDeFormasDelJS(t *testing.T) {
	// El JS usaba una letra: -c, -t, -s, -v…
	casos := map[string]string{
		"c": "circulo", "t": "triangulo", "s": "estrella", "v": "corazon",
		"o": "octogono", "y": "pentagono", "e": "elipse", "z": "cruz",
	}
	for alias, esperado := range casos {
		m, ok := Mascara(alias)
		if !ok || m.Nombre != esperado {
			t.Fatalf("Mascara(%q) = %q, se esperaba %q", alias, m.Nombre, esperado)
		}
	}
	if _, ok := Mascara("noexiste"); ok {
		t.Fatal("Mascara aceptó una forma inventada")
	}
}

// ─── filtros ────────────────────────────────────────────────────────────────

func TestFiltrosDeColor(t *testing.T) {
	rojo := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(rojo.Pix); i += 4 {
		rojo.Pix[i], rojo.Pix[i+1], rojo.Pix[i+2], rojo.Pix[i+3] = 255, 0, 0, 255
	}

	gris := clonar(rojo)
	mustFiltro(t, "grayscale").Aplicar(gris)
	r, g, b := gris.Pix[0], gris.Pix[1], gris.Pix[2]
	if r != g || g != b {
		t.Fatalf("grayscale dejó %d,%d,%d", r, g, b)
	}

	inv := clonar(rojo)
	mustFiltro(t, "invert").Aplicar(inv)
	if inv.Pix[0] != 0 || inv.Pix[1] != 255 || inv.Pix[2] != 255 {
		t.Fatalf("invert dejó %d,%d,%d", inv.Pix[0], inv.Pix[1], inv.Pix[2])
	}

	oscuro := clonar(rojo)
	mustFiltro(t, "darken").Aplicar(oscuro)
	if oscuro.Pix[0] >= 255 {
		t.Fatalf("darken no oscureció: %d", oscuro.Pix[0])
	}

	sep := clonar(rojo)
	mustFiltro(t, "sepia").Aplicar(sep)
	if !(sep.Pix[0] > sep.Pix[2]) {
		t.Fatalf("sepia no dio el tono cálido: %d,%d,%d", sep.Pix[0], sep.Pix[1], sep.Pix[2])
	}
}

func TestFiltrosDeGeometria(t *testing.T) {
	// Imagen 3×2 con un píxel marcado arriba a la izquierda.
	img := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})

	gira := clonar(img)
	mustFiltro(t, "rotate90").Aplicar(gira)
	if gira.Bounds().Dx() != 2 || gira.Bounds().Dy() != 3 {
		t.Fatalf("rotate90 dejó %v", gira.Bounds())
	}
	if gira.NRGBAAt(1, 0).R != 255 {
		t.Fatal("rotate90 no movió el píxel a la esquina superior derecha")
	}

	espejo := clonar(img)
	mustFiltro(t, "flop").Aplicar(espejo)
	if espejo.NRGBAAt(2, 0).R != 255 {
		t.Fatal("flop no llevó el píxel a la derecha")
	}

	volteo := clonar(img)
	mustFiltro(t, "flip").Aplicar(volteo)
	if volteo.NRGBAAt(0, 1).R != 255 {
		t.Fatal("flip no llevó el píxel abajo")
	}
}

func TestFiltrosDelCatalogoTransforman(t *testing.T) {
	for _, nombre := range NombresFiltros() {
		f, ok := Filtro(nombre)
		if !ok || f.Aplicar == nil {
			t.Fatalf("el filtro %q no tiene función", nombre)
		}
		img := tablero(32, 32)
		antes := append([]byte(nil), img.Pix...)
		f.Aplicar(img)
		// Todo filtro tiene que cambiar algo, salvo que la imagen ya estuviera
		// en ese estado; aquí el lienzo es un degradado, así que cambia seguro.
		if bytes.Equal(antes, img.Pix) && nombre != "normalice" {
			t.Fatalf("el filtro %q no cambió ningún píxel", nombre)
		}
	}
	if _, ok := Filtro("noexiste"); ok {
		t.Fatal("Filtro aceptó un efecto inventado")
	}
}

// tablero: imagen con detalle de sobra para que cualquier filtro la cambie
// (un degradado lineal deja igual al enfoque, porque su segunda derivada es 0).
func tablero(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if (x/3+y/3)%2 == 0 {
				img.SetNRGBA(x, y, color.NRGBA{R: 240, G: 30, B: 90, A: 255})
			} else {
				img.SetNRGBA(x, y, color.NRGBA{R: 20, G: 200, B: 40, A: 255})
			}
		}
	}
	return img
}

func clonar(src *image.NRGBA) *image.NRGBA {
	out := image.NewNRGBA(src.Bounds())
	copy(out.Pix, src.Pix)
	return out
}

func mustFiltro(t *testing.T, nombre string) Filtro_ {
	t.Helper()
	f, ok := Filtro(nombre)
	if !ok {
		t.Fatalf("no existe el filtro %q", nombre)
	}
	return f
}

// ─── EXIF del paquete ───────────────────────────────────────────────────────

func TestEXIFIdaYVuelta(t *testing.T) {
	webp, err := EncodeWebP(Fit(lienzo(400, 400), Lado, Contener), nativewebp.DefaultCompression)
	if err != nil {
		t.Fatalf("EncodeWebP: %v", err)
	}
	if !bytes.HasPrefix(webp, []byte("RIFF")) || string(webp[8:12]) != "WEBP" {
		t.Fatal("el WebP no empieza por RIFF…WEBP")
	}

	paquete := NuevoPaquete("Mi paquete", "riokuroxi-svg", "✨", "🔥")
	bloque, err := paquete.EXIF()
	if err != nil {
		t.Fatalf("EXIF: %v", err)
	}
	if len(bloque) < len(cabeceraEXIF) {
		t.Fatal("el bloque EXIF es más corto que su cabecera")
	}
	// La cabecera TIFF tiene que ser la del JS, salvo los 4 bytes del offset 14,
	// que son justo donde va la longitud del JSON.
	if !bytes.Equal(bloque[:14], cabeceraEXIF[:14]) || !bytes.Equal(bloque[18:len(cabeceraEXIF)], cabeceraEXIF[18:]) {
		t.Fatalf("la cabecera EXIF no coincide con la que usa WhatsApp: % x", bloque[:len(cabeceraEXIF)])
	}
	if int(bloque[14])|int(bloque[15])<<8|int(bloque[16])<<16|int(bloque[17])<<24 != len(bloque)-len(cabeceraEXIF) {
		t.Fatalf("la longitud del JSON no está escrita en el offset 14")
	}

	final, err := AplicarEXIF(webp, bloque)
	if err != nil {
		t.Fatalf("AplicarEXIF: %v", err)
	}

	// 1) El tamaño del RIFF tiene que cuadrar con el archivo real.
	if int(uint32(final[4])|uint32(final[5])<<8|uint32(final[6])<<16|uint32(final[7])<<24) != len(final)-8 {
		t.Fatalf("el tamaño del RIFF no cuadra: %d vs %d", final[4:8], len(final)-8)
	}

	// 2) Se tiene que poder volver a leer el paquete.
	leido, ok, err := LeerEXIF(final)
	if err != nil || !ok {
		t.Fatalf("LeerEXIF: %v (ok=%v)", err, ok)
	}
	if leido.Nombre != "Mi paquete" || leido.Autor != "riokuroxi-svg" {
		t.Fatalf("se leyó %+v", leido)
	}
	if len(leido.Emojis) != 2 || leido.Emojis[0] != "✨" {
		t.Fatalf("emojis = %v", leido.Emojis)
	}

	// 3) Y, sobre todo, la imagen sigue siendo válida.
	if _, err := Decode(final); err != nil {
		t.Fatalf("el WebP con EXIF ya no se puede leer: %v", err)
	}
}

func TestEXIFReemplazaAlAnterior(t *testing.T) {
	webp, _ := EncodeWebP(Fit(lienzo(64, 64), Lado, Contener), nativewebp.DefaultCompression)
	uno, _ := NuevoPaquete("primero", "autor").EXIF()
	dos, _ := NuevoPaquete("segundo", "autor").EXIF()

	conUno, err := AplicarEXIF(webp, uno)
	if err != nil {
		t.Fatalf("AplicarEXIF: %v", err)
	}
	conDos, err := AplicarEXIF(conUno, dos)
	if err != nil {
		t.Fatalf("AplicarEXIF (2): %v", err)
	}
	leido, _, err := LeerEXIF(conDos)
	if err != nil || leido.Nombre != "segundo" {
		t.Fatalf("se quedó el paquete viejo: %+v (%v)", leido, err)
	}

	// Y no se acumulan bloques EXIF.
	chunks, err := trocear(cuerpoRIFF(conDos))
	if err != nil {
		t.Fatalf("trocear: %v", err)
	}
	cuantos := 0
	for _, c := range chunks {
		if string(c.cabecera[0:4]) == "EXIF" {
			cuantos++
		}
	}
	if cuantos != 1 {
		t.Fatalf("hay %d bloques EXIF, se esperaba 1", cuantos)
	}
}

func TestAplicarEXIFRechazaBasura(t *testing.T) {
	bloque, _ := NuevoPaquete("", "").EXIF()
	if _, err := AplicarEXIF([]byte("no soy un webp"), bloque); err == nil {
		t.Fatal("AplicarEXIF aceptó algo que no es WebP")
	}
}

func TestPaquetePorDefecto(t *testing.T) {
	p := NuevoPaquete("", "")
	if p.Nombre != PaqueteNombrePorDefecto || p.Autor != PaqueteAutorPorDefecto {
		t.Fatalf("paquete por defecto = %+v", p)
	}
	if p.ID != PaqueteIDPorDefecto {
		t.Fatalf("id = %q", p.ID)
	}
	if len(p.Emojis) != 1 || p.Emojis[0] != "✨" {
		t.Fatalf("emojis por defecto = %v", p.Emojis)
	}
}

// Un paquete enorme no puede reventar el EXIF (el tamaño va en 2 bytes).
func TestPaqueteDemasiadoGrande(t *testing.T) {
	nombre := string(bytes.Repeat([]byte("a"), 70000))
	if _, err := NuevoPaquete(nombre, "x").EXIF(); err == nil {
		t.Fatal("se aceptó un paquete de más de 65535 bytes")
	}
}

// ─── la tubería completa ────────────────────────────────────────────────────

func TestStickerCompleto(t *testing.T) {
	foto := pngDe(t, lienzo(800, 600))
	casos := []Opciones{
		{Lado: Lado, FormaAuto: true},
		{Lado: Lado, Forma: "circulo"},
		{Lado: Lado, Efecto: "sepia"},
		{Lado: Lado, Forma: "corazon", Efecto: "blur"},
	}
	for i, op := range casos {
		webp, err := Sticker(foto, op)
		if err != nil {
			t.Fatalf("caso %d (%+v): %v", i, op, err)
		}
		img, err := Decode(webp)
		if err != nil {
			t.Fatalf("caso %d: el WebP no se puede leer: %v", i, err)
		}
		if img.Bounds().Dx() != Lado || img.Bounds().Dy() != Lado {
			t.Fatalf("caso %d: tamaño %v", i, img.Bounds())
		}
		if len(webp) > 1<<20 {
			t.Fatalf("caso %d: el sticker pesa %d KB (el máximo de WhatsApp es 1 MB)", i, len(webp)/1024)
		}
	}
}

func TestStickerRechazaFormaYFiltroInventados(t *testing.T) {
	foto := pngDe(t, lienzo(64, 64))
	if _, err := Sticker(foto, Opciones{Lado: Lado, Forma: "hexagrama"}); err == nil {
		t.Fatal("se aceptó una forma que no existe")
	}
	if _, err := Sticker(foto, Opciones{Lado: Lado, Efecto: "holograma"}); err == nil {
		t.Fatal("se aceptó un efecto que no existe")
	}
	if _, err := Sticker([]byte("basura"), Opciones{Lado: Lado}); err == nil {
		t.Fatal("se aceptó un archivo que no es una imagen")
	}
}

func TestEsAnimado(t *testing.T) {
	fijo, _ := EncodeWebP(Fit(lienzo(64, 64), Lado, Contener), nativewebp.DefaultCompression)
	if EsAnimado(fijo) {
		t.Fatal("un WebP fijo se detectó como animado")
	}
	if EsAnimado([]byte("basura")) {
		t.Fatal("EsAnimado aceptó basura")
	}
}

// ─── ffmpeg (solo si está instalado) ────────────────────────────────────────

func TestSinFFmpegAvisaBien(t *testing.T) {
	if HayFFmpeg() {
		t.Skip("ffmpeg está instalado: se prueba la conversión de verdad")
	}
	if _, err := VideoAWebP(t.Context(), []byte("x"), 3); err != ErrSinFFmpeg {
		t.Fatalf("VideoAWebP sin ffmpeg = %v, se esperaba ErrSinFFmpeg", err)
	}
	if _, err := WebPAVideo(t.Context(), []byte("x"), 3); err != ErrSinFFmpeg {
		t.Fatalf("WebPAVideo sin ffmpeg = %v, se esperaba ErrSinFFmpeg", err)
	}
}

// ─── conversión real con ffmpeg (se salta si no está instalado) ─────────────

// videoDePrueba genera un MP4 de dos segundos con el propio ffmpeg (pantalla de
// colores de test). Así la prueba no depende de tener un archivo en el repo.
func videoDePrueba(t *testing.T) []byte {
	t.Helper()
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc=size=320x240:rate=10:duration=2",
		"-pix_fmt", "yuv420p", "-f", "mp4", "pipe:1")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Skipf("no se pudo generar el vídeo de prueba: %v (%s)", err, errb.String())
	}
	return out.Bytes()
}

func TestVideoAWebPConFFmpeg(t *testing.T) {
	if !HayFFmpeg() {
		t.Skip("ffmpeg no está instalado")
	}
	video := videoDePrueba(t)
	webp, err := VideoAWebP(t.Context(), video, 2)
	if err != nil {
		t.Fatalf("VideoAWebP: %v", err)
	}
	if !EsAnimado(webp) {
		t.Fatal("el WebP convertido no tiene chunk ANIM: WhatsApp lo vería como imagen rota")
	}
	if len(webp) > 1<<20 {
		t.Fatalf("el sticker animado pesa %d KB (máximo de WhatsApp: 1 MB)", len(webp)/1024)
	}

	// Con EXIF encima tiene que seguir siendo válido y legible.
	bloque, err := NuevoPaquete("Prueba", "riokuroxi-svg").EXIF()
	if err != nil {
		t.Fatalf("EXIF: %v", err)
	}
	final, err := AplicarEXIF(webp, bloque)
	if err != nil {
		t.Fatalf("AplicarEXIF: %v", err)
	}
	if leido, ok, err := LeerEXIF(final); err != nil || !ok || leido.Nombre != "Prueba" {
		t.Fatalf("LeerEXIF = %+v %v %v", leido, ok, err)
	}

	// Y siempre se puede sacar el primer fotograma, haya o no ffmpeg que lea
	// WebP animado.
	img, err := PrimerFotograma(final)
	if err != nil {
		t.Fatalf("PrimerFotograma: %v", err)
	}
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatalf("fotograma vacío: %v", img.Bounds())
	}
}

func TestPrimerFotogramaDeUnStickerFijo(t *testing.T) {
	fijo, err := EncodeWebP(Fit(lienzo(300, 200), Lado, Contener), nativewebp.DefaultCompression)
	if err != nil {
		t.Fatalf("EncodeWebP: %v", err)
	}
	img, err := PrimerFotograma(fijo)
	if err != nil {
		t.Fatalf("PrimerFotograma: %v", err)
	}
	if img.Bounds().Dx() != Lado || img.Bounds().Dy() != Lado {
		t.Fatalf("tamaño = %v, se esperaba 512×512", img.Bounds())
	}
	if _, err := PrimerFotograma([]byte("basura")); err == nil {
		t.Fatal("PrimerFotograma aceptó basura")
	}
}
