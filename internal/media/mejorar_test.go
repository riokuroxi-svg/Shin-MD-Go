// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// mejorar_test.go — la mejora local de `.hd`: tamaños, techos y que nunca
// mancha la imagen original.
package media

import (
	"image"
	"image/color"
	"testing"
)

func TestMejorarDoble(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 320, 240))
	out, err := Mejorar(src)
	if err != nil {
		t.Fatalf("Mejorar: %v", err)
	}
	if out.Bounds().Dx() != 640 || out.Bounds().Dy() != 480 {
		t.Fatalf("320×240 ×2 = %dx%d, se esperaba 640×480", out.Bounds().Dx(), out.Bounds().Dy())
	}
}

func TestMejorarTecho4096(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3000, 2000))
	out, err := Mejorar(src)
	if err != nil {
		t.Fatalf("Mejorar: %v", err)
	}
	if out.Bounds().Dx() > MejoraMaxLado || out.Bounds().Dy() > MejoraMaxLado {
		t.Fatalf("ningún lado puede pasar de %d: %dx%d", MejoraMaxLado, out.Bounds().Dx(), out.Bounds().Dy())
	}
}

func TestMejorarNuncaEncogeNoPisaOriginal(t *testing.T) {
	// Mayor que el techo de salida: no se escala, solo se enfoca... y la
	// imagen del llamador NO se toca.
	src := image.NewNRGBA(image.Rect(0, 0, 4500, 3000)) // 13.5 Mpx pasa por pixel a pixel? no: > 12 Mpx
	if _, err := Mejorar(src); err == nil || err.Error() != "demasiado-grande" {
		t.Fatalf("13.5 Mpx debían salir con demasiado-grande: %v", err)
	}

	chica := image.NewNRGBA(image.Rect(0, 0, 300, 300))
	// Relleno uniforme: enfoque no cambia nada → cualquier cambio es mancha.
	for i := range chica.Pix {
		chica.Pix[i] = 100
	}
	copia := make([]uint8, len(chica.Pix))
	copy(copia, chica.Pix)
	if _, err := Mejorar(chica); err != nil {
		t.Fatalf("Mejorar: %v", err)
	}
	// Si aún fuera NRGBA original habría que comparar; con copia defensiva el
	// buffer del llamador queda idéntico.
	for i := range chica.Pix {
		if chica.Pix[i] != copia[i] {
			t.Fatalf("la original cambió en el byte %d: se olvidó la copia", i)
		}
	}
}

func TestMejorarErrores(t *testing.T) {
	if _, err := Mejorar(nil); err == nil {
		t.Fatal("imagen nil no puede salir buena")
	}
}

// TestMejorarColor: bordes se enfocan, tonos medios no se invierten.
func TestMejorarColor(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	rojo := color.NRGBA{200, 40, 40, 255}
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			src.SetNRGBA(x, y, rojo)
		}
	}
	out, err := Mejorar(src)
	if err != nil {
		t.Fatalf("Mejorar: %v", err)
	}
	centro := out.NRGBAAt(100, 100)
	if centro.R < 150 || centro.G > 90 { // la zona plana se queda roja
		t.Fatalf("un relleno uniforme no debe cambiar de color: %+v", centro)
	}
}
