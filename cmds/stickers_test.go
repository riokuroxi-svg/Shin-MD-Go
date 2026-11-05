// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// stickers_test.go — Cómo se leen las opciones del `.sticker` y qué comandos
// de stickers existen. Lo que se prueba aquí es la parte que el usuario escribe:
// `-c` tiene que ser el círculo y `-sepia` el efecto, no al revés.
package cmds

import (
	"testing"

	"shin-go/internal/bot"
	"shin-go/internal/media"
)

func TestOpcionesStickerFormas(t *testing.T) {
	casos := map[string]string{
		"-c":         "circulo",
		"-t":         "triangulo",
		"-s":         "estrella",
		"-v":         "corazon",
		"-o":         "octogono",
		"--corazon":  "corazon",
		"-hexagono":  "hexagono",
		"-PENTAGONO": "pentagono",
	}
	for arg, esperado := range casos {
		op, err := opcionesSticker([]string{arg})
		if err != nil {
			t.Fatalf("opcionesSticker(%q): %v", arg, err)
		}
		if op.Forma != esperado {
			t.Fatalf("opcionesSticker(%q).Forma = %q, se esperaba %q", arg, op.Forma, esperado)
		}
	}
}

func TestOpcionesStickerEfectos(t *testing.T) {
	casos := map[string]string{
		"-sepia":   "sepia",
		"-blur":    "blur",
		"-blur2":   "", // no existe: tiene que fallar
		"--invert": "invert",
		"-flip":    "flip",
	}
	for arg, esperado := range casos {
		op, err := opcionesSticker([]string{arg})
		if esperado == "" {
			if err == nil {
				t.Fatalf("opcionesSticker(%q) aceptó algo inventado", arg)
			}
			continue
		}
		if err != nil {
			t.Fatalf("opcionesSticker(%q): %v", arg, err)
		}
		if op.Efecto != esperado {
			t.Fatalf("opcionesSticker(%q).Efecto = %q, se esperaba %q", arg, op.Efecto, esperado)
		}
	}
}

func TestOpcionesStickerCombinadas(t *testing.T) {
	op, err := opcionesSticker([]string{"-corazon", "-sepia", "-llenar"})
	if err != nil {
		t.Fatalf("opcionesSticker: %v", err)
	}
	if op.Forma != "corazon" || op.Efecto != "sepia" || op.FormaAuto {
		t.Fatalf("opciones = %+v", op)
	}
	if op.Lado != media.Lado {
		t.Fatalf("el lado del sticker es %d, se esperaba %d", op.Lado, media.Lado)
	}

	// Sin opciones: encaja entera y sin forma, que es lo que espera la gente
	// que escribe `.s` a secas.
	op, err = opcionesSticker(nil)
	if err != nil || op.Forma != "" || op.Efecto != "" || !op.FormaAuto {
		t.Fatalf("opciones por defecto = %+v (%v)", op, err)
	}

	// Los textos sueltos se ignoran (no son opciones).
	if _, err := opcionesSticker([]string{"hola", "que", "tal"}); err != nil {
		t.Fatalf("opcionesSticker con texto suelto: %v", err)
	}
}

func TestComandosDeStickersRegistrados(t *testing.T) {
	esperados := map[string]string{
		"sticker": "stickers", "toimg": "stickers", "stickerinfo": "stickers",
	}
	for nombre, categoria := range esperados {
		cmd, ok := bot.Lookup(nombre)
		if !ok || cmd == nil {
			t.Fatalf("el comando %q no está registrado", nombre)
		}
		if string(cmd.Category) != categoria {
			t.Fatalf("%q está en %q, se esperaba %q", nombre, cmd.Category, categoria)
		}
		if cmd.Desc == "" {
			t.Fatalf("%q no tiene descripción", nombre)
		}
	}
	// Los alias del JS tienen que seguir funcionando.
	alias := map[string]string{
		"s": "sticker", "figu": "sticker", "figura": "sticker",
		"toimage": "toimg", "afoto": "toimg",
	}
	for a, canonico := range alias {
		cmd, ok := bot.Lookup(a)
		if !ok || cmd.Name != canonico {
			t.Fatalf("el alias %q no apunta a %q", a, canonico)
		}
	}
}

func TestLimitarTexto(t *testing.T) {
	if got := limitarTexto("  Shin-MD  ", 64); got != "Shin-MD" {
		t.Fatalf("limitarTexto recortó de más: %q", got)
	}
	if got := limitarTexto("abcdefghij", 5); got != "abcd…" {
		t.Fatalf("limitarTexto = %q", got)
	}
}
