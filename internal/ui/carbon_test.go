// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// carbon_test.go — el renderer local de `.carbon`: determinismo (mismo código,
// mismo PNG), topes y coloreado (un «//» dentro de una cadena NO es comentario).
package ui

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestRenderCarbonDeterminista(t *testing.T) {
	codigo := "package main\n\nfunc main() {\n\tprintln(\"hola\") // saludo\n}\n"
	a, err := RenderCarbon(codigo, "negro")
	if err != nil {
		t.Fatalf("RenderCarbon: %v", err)
	}
	b, err := RenderCarbon(codigo, "negro")
	if err != nil {
		t.Fatalf("RenderCarbon (2ª): %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("el mismo código debe producir el mismo PNG byte a byte")
	}
	// Y debe ser un PNG de verdad, con medidas razonables.
	img, err := png.Decode(bytes.NewReader(a))
	if err != nil {
		t.Fatalf("la salida no decodifica como PNG: %v", err)
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	if w < 200 || h < 100 || w > 4000 || h > 4000 {
		t.Fatalf("medidas fuera de rango: %dx%d", w, h)
	}
}

func TestRenderCarbonTemasYTecho(t *testing.T) {
	// Tema desconocido cae al verde (como hacía el JS con su color por defecto).
	verde, _ := RenderCarbon("x := 1", "verde")
	raro, _ := RenderCarbon("x := 1", "arcoiris-inventado")
	if !bytes.Equal(verde, raro) {
		t.Fatal("tema desconocido debe caer al predeterminado")
	}
	// Un muro de líneas se corta: la imagen no crece sin límite.
	muro := strings.Repeat("línea\n", 500)
	pngMuro, err := RenderCarbon(muro, "azul")
	if err != nil {
		t.Fatalf("RenderCarbon muro: %v", err)
	}
	img, _ := png.Decode(bytes.NewReader(pngMuro))
	altoMax := (carbonBarra + carbonMargenY*2 + carbonMaxLineas*carbonAltoLinea) * carbonEscala
	if img.Bounds().Dy() > altoMax {
		t.Fatalf("500 líneas superaron el techo de %d: %d", altoMax, img.Bounds().Dy())
	}
}

func TestRenderCarbonVacio(t *testing.T) {
	if _, err := RenderCarbon("   \n ", "verde"); err == nil {
		t.Fatal("código vacío no debe producir imagen")
	}
}

func TestColorearLineaCadenaVirgenComentario(t *testing.T) {
	// «//» dentro de una cadena NO es comentario. Es el bug clásico de los
	// resaltadores hechos a prisa.
	segs, _ := colorearLinea(`s := "https://ejemplo.com" // enlace`, false)
	var comentarios int
	for _, s := range segs {
		if strings.Contains(s.texto, "https://") && s.clase == carbonComentario {
			t.Errorf("la URL dentro de la cadena salió como comentario: %+v", segs)
		}
		if strings.HasPrefix(strings.TrimSpace(s.texto), "https://") && s.clase != carbonCadena {
			t.Errorf("la URL debía salir como cadena: %+v", segs)
		}
		if strings.Contains(s.texto, "enlace") && s.clase == carbonComentario {
			comentarios++
		}
	}
	if comentarios == 0 {
		t.Error("el comentario del final no se detectó")
	}
}

func TestColorearLineaKeywordsYNumeros(t *testing.T) {
	segs, _ := colorearLinea("func suma(a, b int) int { return a + b + 42 }", false)
	var keyword, numero bool
	for _, s := range segs {
		if (s.texto == "func" || s.texto == "int" || s.texto == "return") && s.clase == carbonKeyword {
			keyword = true
		}
		if s.texto == "42" && s.clase == carbonNumero {
			numero = true
		}
	}
	if !keyword || !numero {
		t.Errorf("keyword=%v numero=%v en %+v", keyword, numero, segs)
	}
}

func TestColorearBloqueComentarioEntreLineas(t *testing.T) {
	_, enBloque := colorearLinea("/* empieza", false)
	if !enBloque {
		t.Fatal("un /* sin cerrar deja el bloque abierto para la línea siguiente")
	}
	segs, enBloque := colorearLinea("sigue y cierra */ int x", enBloque)
	if enBloque {
		t.Fatal("tras */ el bloque debe cerrarse")
	}
	if len(segs) < 2 || segs[0].clase != carbonComentario {
		t.Fatalf("la parte anterior a */ es comentario: %+v", segs)
	}
}

func TestRecortarRunas(t *testing.T) {
	if got := recortarRunas("hola", 10); got != "hola" {
		t.Errorf("corta no debe tocar: %q", got)
	}
	largo := strings.Repeat("é", 100) // é son 2 bytes en UTF-8
	if got := recortarRunas(largo, 50); len([]rune(got)) != 50 {
		t.Errorf("recorte por runas: %d runas", len([]rune(got)))
	}
}
