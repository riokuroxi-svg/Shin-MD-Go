// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package xp

import "testing"

// El primer nivel empieza en 0 de XP: quien acaba de llegar está en el nivel 1
// al 0 %, no en un «nivel 0» que no existe (era el fallo del bot viejo).
func TestPrimerNivelEmpiezaEnCero(t *testing.T) {
	r := De(1)
	if r.Level != 1 {
		t.Fatalf("De(1).Level = %d", r.Level)
	}
	if r.Min != 0 {
		t.Fatalf("Min del nivel 1 = %d, se esperaba 0", r.Min)
	}
	if r.Max <= 0 {
		t.Fatalf("Max del nivel 1 = %d, tiene que ser positivo", r.Max)
	}
	if r.Ancho() != r.Max {
		t.Fatalf("Ancho del nivel 1 = %d, se esperaba %d", r.Ancho(), r.Max)
	}
	// Pedir un nivel por debajo del mínimo devuelve el mínimo, sin explotar.
	if De(0) != De(1) {
		t.Fatalf("De(0) debería ser De(1)")
	}
}

// Los tramos tienen que ser contiguos: si hay un hueco entre el Max de un nivel
// y el Min del siguiente, existe una cantidad de XP que no pertenece a ningún
// nivel y NivelPara devolvería algo incoherente.
func TestTramosContiguos(t *testing.T) {
	for nivel := 1; nivel < 200; nivel++ {
		actual := De(nivel)
		siguiente := De(nivel + 1)
		if siguiente.Min != actual.Max+1 {
			t.Fatalf("hueco entre nivel %d (Max %d) y %d (Min %d)",
				nivel, actual.Max, nivel+1, siguiente.Min)
		}
		if actual.Ancho() <= 0 {
			t.Fatalf("el nivel %d no pide XP (%d)", nivel, actual.Ancho())
		}
	}
}

// La curva crece: un nivel alto nunca puede pedir menos que uno bajo.
func TestLaCurvaCrece(t *testing.T) {
	anterior := int64(0)
	for nivel := 1; nivel < 100; nivel++ {
		ancho := De(nivel).Ancho()
		if ancho < anterior {
			t.Fatalf("el nivel %d pide %d y el anterior pedía %d", nivel, ancho, anterior)
		}
		anterior = ancho
	}
}

// NivelPara tiene que ser la inversa de De en los bordes. Estos son los casos
// que se rompen cuando alguien "arregla" la fórmula sin tocar las dos.
func TestNivelParaEnLosBordes(t *testing.T) {
	casos := []struct {
		total  int64
		nivel  int
		motivo string
	}{
		{0, 1, "cero de XP es nivel 1"},
		{De(1).Min, 1, "justo en el mínimo del nivel 1"},
		{De(1).Max, 1, "en el tope del nivel 1"},
		{De(1).Max + 1, 2, "un punto por encima del nivel 1"},
		{De(10).Min, 10, "borde del nivel 10"},
	}
	for _, c := range casos {
		if got := NivelPara(c.total); got != c.nivel {
			t.Errorf("NivelPara(%d) = %d, se esperaba %d (%s)", c.total, got, c.nivel, c.motivo)
		}
	}
}

// Un dato corrupto no puede tumbar un comando: el JS lanzaba TypeError con un
// nivel negativo y eso se llevaba por delante el mensaje entero.
func TestDatosAbsurdosNoExplotan(t *testing.T) {
	if De(-5).Level != Minimo {
		t.Fatalf("De(-5) debería caer en el nivel mínimo")
	}
	if NivelPara(-100) != Minimo {
		t.Fatalf("NivelPara(-100) debería ser el mínimo")
	}
	p := Calcular(-1)
	if p.Level != Minimo || p.Dentro != 0 || p.Porcentaje != 0 {
		t.Fatalf("Calcular(-1) = %+v", p)
	}
	// Una XP enorme no debe colgar el cálculo.
	if n := NivelPara(1 << 40); n <= 0 {
		t.Fatalf("NivelPara(1<<40) = %d", n)
	}
}

func TestCalcularPorcentaje(t *testing.T) {
	r := De(3)
	// A mitad del tramo, el porcentaje tiene que ser ~50.
	medio := r.Min + r.Ancho()/2
	p := Calcular(medio)
	if p.Level != 3 {
		t.Fatalf("nivel = %d, se esperaba 3", p.Level)
	}
	if p.Porcentaje < 49 || p.Porcentaje > 51 {
		t.Fatalf("porcentaje = %d, se esperaba ~50", p.Porcentaje)
	}
	if p.Necesaria != r.Ancho() {
		t.Fatalf("necesaria = %d, se esperaba %d", p.Necesaria, r.Ancho())
	}
	// En el mínimo del nivel, el progreso es cero y no negativo.
	if pct := Calcular(r.Min).Porcentaje; pct != 0 {
		t.Fatalf("porcentaje en el mínimo = %d, se esperaba 0", pct)
	}
}

func TestBarra(t *testing.T) {
	if got := Barra(Progreso{Porcentaje: 0}, 10); got != "▱▱▱▱▱▱▱▱▱▱" {
		t.Fatalf("barra al 0 %% = %q", got)
	}
	if got := Barra(Progreso{Porcentaje: 100}, 10); got != "▰▰▰▰▰▰▰▰▰▰" {
		t.Fatalf("barra al 100 %% = %q", got)
	}
	if got := Barra(Progreso{Porcentaje: 50}, 10); got != "▰▰▰▰▰▱▱▱▱▱" {
		t.Fatalf("barra al 50 %% = %q", got)
	}
	// Un ancho inválido no debe producir una barra vacía.
	if got := Barra(Progreso{Porcentaje: 100}, 0); len([]rune(got)) != 10 {
		t.Fatalf("ancho 0 debería caer al 10 por defecto, salió %q", got)
	}
}
