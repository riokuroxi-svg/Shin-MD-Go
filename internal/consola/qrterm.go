// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// qrterm.go — un QR dibujado con caracteres de bloque, para terminales.
//
// Sirve para el QR de vinculación de WhatsApp (un VPS con navegador de
// terminal, por ejemplo). En el móvil donde corre el bot no tiene sentido —
// no hay cámara que lo escanee— y por eso el flujo por defecto es el código
// de 8 letras; esto es la alternativa, no el camino principal.
//
// Se usan medios bloques (▀ ▄ █): cada carácter pinta DOS módulos en
// vertical, así el QR sale a la mitad de alto y cabe en una pantalla de
// teléfono o en una ventana de ssh de 24 filas.
package consola

import (
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"rsc.io/qr"
)

// AnchoQR: cuántas columnas necesita el QR dibujado en texto (con su zona de
// silencio). Un ref de WhatsApp son ~370 caracteres → 81 columnas: en Termux
// en vertical (50) NO cabe ni cabe mal. Saber el número exacto es lo que
// permite decir «ensancha a 82» en vez de escupir un QR partido que no escanea
// nadie.
func AnchoQR(contenido string) int {
	codigo, err := qr.Encode(contenido, qr.M)
	if err != nil {
		return 0
	}
	return codigo.Size + 4 // 2 módulos de silencio por lado
}

// QRTexto devuelve el QR de `contenido` como líneas de caracteres de bloque.
// `escala` repite cada módulo en horizontal (1 o 2; 2 se lee mejor en
// pantallas de alta densidad). La zona de silencio de 2 módulos ya va
// incluida: sin ella muchos lectores no enganchan el código.
func QRTexto(contenido string, escala int) ([]string, error) {
	if escala < 1 {
		escala = 1
	}
	codigo, err := qr.Encode(contenido, qr.M)
	if err != nil {
		return nil, err
	}
	const silencio = 2
	lado := codigo.Size + 2*silencio

	negro := func(x, y int) bool {
		x -= silencio
		y -= silencio
		if x < 0 || y < 0 || x >= codigo.Size || y >= codigo.Size {
			return false
		}
		return codigo.Black(x, y)
	}

	var lineas []string
	for y := 0; y < lado; y += 2 {
		var b strings.Builder
		for x := 0; x < lado; x++ {
			arriba := negro(x, y)
			abajo := y+1 < lado && negro(x, y+1)
			var c string
			switch {
			case arriba && abajo:
				c = "█"
			case arriba:
				c = "▀"
			case abajo:
				c = "▄"
			default:
				c = " "
			}
			b.WriteString(strings.Repeat(c, escala))
		}
		lineas = append(lineas, b.String())
	}
	return lineas, nil
}

// DibujarQR pinta el QR centrado, con margen y un pie de ayuda. Fuera de una
// terminal con color se imprime igual (los bloques no necesitan ANSI), pero
// sin las instrucciones en sakura.
func DibujarQR(w io.Writer, contenido, pie string) bool {
	escala := 1
	lineas, err := QRTexto(contenido, escala)
	if err != nil {
		Ojo("no se pudo dibujar el QR: " + err.Error())
		return false
	}
	ancho := 0
	for _, l := range lineas {
		if n := len([]rune(l)); n > ancho {
			ancho = n
		}
	}
	if ancho+2 > Ancho() {
		// No entra: mejor decirlo que pintar un QR partido en dos líneas.
		return false
	}
	margen := (Ancho() - ancho) / 2
	if margen < 0 {
		margen = 0
	}
	sangria := strings.Repeat(" ", margen)
	sep := stTenue.Sprint(sangria + strings.Repeat("▀", ancho))
	io.WriteString(w, "\n"+sep+"\n")
	for _, l := range lineas {
		io.WriteString(w, sangria+l+"\n")
	}
	io.WriteString(w, sep+"\n")
	if pie != "" {
		for _, l := range envolver(pie, Ancho()-8) {
			io.WriteString(w, sangria+"  "+stTenue.Sprint(l)+"\n")
		}
	}
	io.WriteString(w, "\n")
	return true
}

// qrEscala / qrSilencio: 8 píxeles por módulo y 4 módulos de margen blanco.
// Con menos margen muchos lectores de cámara fallan; con menos de 6 px por
// módulo, una pantalla de móvil mal centrada también.
const (
	qrEscala   = 8
	qrSilencio = 4
)

// GuardarQRPNG escribe el QR como imagen. Es el camino cuando el terminal es
// estrecho (Termux en vertical) o cuando se escanea desde otra pantalla: en
// Termux `termux-open <archivo>`; en un VPS, `scp` del archivo o el panel web.
func GuardarQRPNG(ruta, contenido string) error {
	codigo, err := qr.Encode(contenido, qr.M)
	if err != nil {
		return err
	}
	lado := (codigo.Size + 2*qrSilencio) * qrEscala
	img := image.NewRGBA(image.Rect(0, 0, lado, lado))
	for y := 0; y < lado; y++ {
		for x := 0; x < lado; x++ {
			img.Set(x, y, color.White)
		}
	}
	negro := color.RGBA{0, 0, 0, 255}
	for my := 0; my < codigo.Size; my++ {
		for mx := 0; mx < codigo.Size; mx++ {
			if !codigo.Black(mx, my) {
				continue
			}
			for dy := 0; dy < qrEscala; dy++ {
				for dx := 0; dx < qrEscala; dx++ {
					img.Set((mx+qrSilencio)*qrEscala+dx, (my+qrSilencio)*qrEscala+dy, negro)
				}
			}
		}
	}
	if dir := filepath.Dir(ruta); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	f, err := os.Create(ruta)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return png.Encode(f, img)
}
