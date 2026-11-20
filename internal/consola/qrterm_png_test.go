// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// qrterm_png_test.go — el QR en imagen: que la matriz sea la correcta, no un
// dibujo que se parece.

package consola

import (
	"bytes"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"rsc.io/qr"
)

// contenido de prueba: con este largo un ref de WhatsApp (~370 caracteres).
const contenido = "1@ABCDEF0123456789abcdef0123456789ABCDEF0123456789abcdef" +
	"0123456789ABCDEF0123456789abcdef0123456789ABCDEF0123456789abcdef" +
	"0123456789ABCDEF0123456789abcdef0123456789ABCDEF0123456789abcdef" +
	"0123456789ABCDEF0123456789abcdef0123456789ABCDEF0123456789ab"

func TestGuardarQRPNG(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "qr.png")
	if err := GuardarQRPNG(ruta, contenido); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(ruta)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 500 {
		t.Errorf("PNG sospechosamente pequeño: %d bytes", len(b))
	}

	// Y el contenido tiene que ser el QR EXACTO, con su zona de silencio:
	// un PNG bonito pero con la matriz mal no escanea (y nadie se entera
	// hasta que lleva dos minutos delante de la pantalla).
	img, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	codigo, err := qr.Encode(contenido, qr.M)
	if err != nil {
		t.Fatal(err)
	}
	lado := codigo.Size + 2*qrSilencio
	if bb := img.Bounds(); bb.Dx() != lado*qrEscala || bb.Dy() != lado*qrEscala {
		t.Fatalf("tamaño %dx%d px, quería %d px de lado", bb.Dx(), bb.Dy(), lado*qrEscala)
	}
	malo := 0
	for my := 0; my < codigo.Size; my++ {
		for mx := 0; mx < codigo.Size; mx++ {
			r, g, l, _ := img.At((mx+qrSilencio)*qrEscala+qrEscala/2, (my+qrSilencio)*qrEscala+qrEscala/2).RGBA()
			negro := r < 0x8000 && g < 0x8000 && l < 0x8000
			if negro != codigo.Black(mx, my) {
				malo++
			}
		}
	}
	if malo != 0 {
		t.Errorf("%d módulos no cuadran con el QR esperado", malo)
	}
	negras := 0
	for y := 0; y < qrSilencio*qrEscala; y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			if r < 0x8000 {
				negras++
			}
		}
	}
	if negras != 0 {
		t.Errorf("la zona de silencio tiene %d píxeles negros", negras)
	}
}

func TestGuardarQRPNCreaLaCarpeta(t *testing.T) {
	ruta := filepath.Join(t.TempDir(), "sin-crear", "qr.png")
	if err := GuardarQRPNG(ruta, contenido); err != nil {
		t.Fatalf("no debería dar error con la carpeta ausente: %v", err)
	}
	if _, err := os.Stat(ruta); err != nil {
		t.Error(err)
	}
}

// Un ref de WhatsApp real necesita muchísimas más columnas de las que da
// Termux en vertical: por eso existe el plan B en PNG.
func TestAnchoQRDeUnRefReal(t *testing.T) {
	ref := "1@" + strings.Repeat("aZ9!", 93)
	if len(ref) < 350 {
		t.Fatalf("el ejemplo debería parecerse a un ref real (%d)", len(ref))
	}
	got := AnchoQR(ref)
	if got < 70 {
		t.Errorf("AnchoQR(%d chars) = %d, se esperaba > 70", len(ref), got)
	}
	if got := AnchoQR(""); got != 0 {
		t.Logf("AnchoQR vacío = %d", got)
	}
}

// En una terminal de 50 columnas no se dibuja NADA: mejor un aviso claro que
// un QR partido en dos líneas que no escanea ni su abuela.
func TestDibujarQRNoCabeEnEstrecho(t *testing.T) {
	var b bytes.Buffer
	if DibujarQR(&b, contenido, "pie") && Ancho() < AnchoQR(contenido)+2 {
		t.Error("cabe en un terminal de 80: revisa el test con cols=50")
	}
	for _, l := range strings.Split(b.String(), "\n") {
		if anchoVisible(l) > Ancho() {
			t.Errorf("se salió del ancho (%d > %d): %q", anchoVisible(l), Ancho(), stripANSI(l))
		}
	}
}
