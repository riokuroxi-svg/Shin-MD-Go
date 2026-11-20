// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// vinculacion_test.go — la decisión de a quién vincular, a prueba de
// terminals de mentira y números escritos de mil maneras.
package wa

import (
	"slices"
	"strings"
	"testing"
)

func TestNormalizarTelefono(t *testing.T) {
	casos := []struct {
		in, quiere string
	}{
		{"+52 1 55 1234 5678", "5215512345678"}, // MX con el 1 móvil ya puesto
		{"52 55 1234 5678", "5215512345678"},    // MX sin el 1: lo añade
		{"5215512345678", "5215512345678"},      // ya bien
		{"+54 9 11 2345 6789", "5491123456789"}, // AR con el 9
		{"54 11 2345 6789", "5491123456789"},    // AR sin el 9: lo añade
		{"0155 123 4567", "1551234567"},         // ceros delante fuera
		{"abc", ""},                             // sin dígitos: vacío
		{"", ""},
		{"  +1 (555) 000-1234  ", "15550001234"}, // basura de formato fuera
	}
	for _, c := range casos {
		if got := NormalizarTelefono(c.in); got != c.quiere {
			t.Errorf("NormalizarTelefono(%q) = %q, quería %q", c.in, got, c.quiere)
		}
	}
}

func TestPlanDeVinculacion(t *testing.T) {
	casos := []struct {
		nombre           string
		registrada       bool
		forzada          bool
		numero           string
		hayTerminal      bool
		quiereAccion     Accion
		quiereNumeroVivo bool
	}{
		{"sesión buena y nada forzado: no hace nada", true, false, "", true, AccionNada, false},
		{"sesión buena pero --code pasado: manda el número escrito", true, true, "5215512345678", true, AccionUsar, true},
		{"número escrito manda siempre, sin preguntar", false, false, "5215512345678", false, AccionUsar, true},
		{"sin sesión y con terminal: pregunta", false, false, "", true, AccionPreguntar, false},
		{"sin sesión y sin terminal: salir, no QR", false, false, "", false, AccionSalir, false},
		{"--code vacío sin sesión: con terminal pregunta igual", false, true, "", true, AccionPreguntar, false},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			p := PlanDeVinculacion(c.registrada, c.forzada, c.numero, c.hayTerminal, "no hay sesión")
			if p.Accion != c.quiereAccion {
				t.Fatalf("acción = %v, quería %v", p.Accion, c.quiereAccion)
			}
			if (p.Numero != "") != c.quiereNumeroVivo {
				t.Fatalf("número = %q; esperaba vacío=%v", p.Numero, !c.quiereNumeroVivo)
			}
		})
	}
}

func TestFormatearCodigo(t *testing.T) {
	if got := FormatearCodigo("xbbm4pbt"); got != "XBBM-4PBT" {
		t.Fatalf("sin guion ni mayúsculas → %q, quería «XBBM-4PBT»", got)
	}
	if got := FormatearCodigo("ABCD-1234"); got != "ABCD-1234" {
		t.Fatalf("ya formateado no se toca → %q", got)
	}
	if got := Espaciar("XBBM-4PBT"); got != "X B B M - 4 P B T" {
		t.Fatalf("espaciar → %q", got)
	}
}

func TestPanelesNoRompiyContienenLoImportante(t *testing.T) {
	var b strings.Builder
	PanelVinculacion(&b)
	if !strings.Contains(b.String(), "código de 8 letras") {
		t.Fatal("el panel de vinculación debe explicar el código de 8 letras")
	}
	b.Reset()
	PanelCodigo(&b, "5215544443333", "XBBM4PBT")
	s := b.String()
	for _, trozo := range []string{"tu código", "X B B M - 4 P B T", "5215544443333", "Vincular un dispositivo › con número"} {
		if !strings.Contains(s, trozo) {
			t.Fatalf("el panel del código debe contener %q\n%s", trozo, s)
		}
	}
	// Con espacios se puede leer y seleccionar; sin quitarlos, nadie lo copia
	// mal. Y si alguien vuelve a pintarlo con letras de bloque, esto falla.
	if strings.ContainsAny(s, "█▓▒░") {
		t.Error("el código no se dibuja con arte de bloques (se ve mal en Termux)")
	}
	if !strings.Contains(strings.ReplaceAll(s, " ", ""), "XBBM-4PBT") {
		t.Error("quitando los espacios tiene que quedar el código tal cual")
	}
	b.Reset()
	ExplicarSinTerminal(&b, "no hay sesión")
	s = b.String()
	for _, trozo := range []string{"--code 521XXXXXXXXXX", "PAIRING_NUMBER=", "no hay sesión"} {
		if !strings.Contains(s, trozo) {
			t.Fatalf("la explicación sin terminal debe contener %q\n%s", trozo, s)
		}
	}
}

// TestPairDisplayNameEsNavegadorYSO: WhatsApp valida el nombre visible del
// compañero al pedir el código (PairPhone) y contesta 400 bad-request si no
// cumple «Navegador (SO)». Aquí ponía «Shin-MD (Go)»: ningún número podía
// vincularse. La prueba falla si alguien vuelve a poner un nombre libre.
func TestPairDisplayNameEsNavegadorYSO(t *testing.T) {
	navegadores := []string{"Chrome", "Firefox", "Edge", "IE", "Opera", "Safari", "Electron", "Chrome Mobile"}
	sistemas := []string{"Linux", "Windows", "Mac OS", "Android", "iOS", "Ubuntu"}

	navegador, sistema, cumple := strings.Cut(PairDisplayName, " (")
	sistema = strings.TrimSuffix(sistema, ")")
	if !cumple || !strings.HasSuffix(PairDisplayName, ")") {
		t.Fatalf("PairDisplayName = %q, el formato tiene que ser «Navegador (SO)»", PairDisplayName)
	}
	if !slices.Contains(navegadores, navegador) {
		t.Errorf("navegador %q no es de los que WhatsApp acepta (%v)", navegador, navegadores)
	}
	if !slices.Contains(sistemas, sistema) {
		t.Errorf("sistema %q no es de los que WhatsApp acepta (%v)", sistema, sistemas)
	}
}
