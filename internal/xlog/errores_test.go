// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// xlog_test.go — el anillo de errores que lee `.health`.
package xlog

import (
	"io"
	"strings"
	"testing"
)

func limpiar() { LimpiarErrores() }

func TestAnilloGuardaYCuenta(t *testing.T) {
	limpiar()
	l := New("error", io.Discard)

	l.Warn("algo raro")
	l.Err("se rompió")
	l.Info("esto no se guarda") // Info no entra en el anillo

	got := UltimosErrores(0)
	if len(got) != 2 {
		t.Fatalf("se guardaron %d registros, se esperaban 2", len(got))
	}
	if got[0].Nivel != "aviso" || got[0].Mensaje != "algo raro" {
		t.Fatalf("primer registro: %+v", got[0])
	}
	if got[1].Nivel != "error" || got[1].Mensaje != "se rompió" {
		t.Fatalf("segundo registro: %+v", got[1])
	}
	if ContarErrores() != 2 {
		t.Fatalf("contador = %d", ContarErrores())
	}
	// Del más viejo al más nuevo: así se leen en el chat.
	if UltimosErrores(1)[0].Mensaje != "se rompió" {
		t.Fatalf("UltimosErrores(1) = %+v", UltimosErrores(1))
	}
}

func TestAnilloNoCreceSinFin(t *testing.T) {
	limpiar()
	l := New("error", io.Discard)
	for i := 0; i < maxRegistros*3; i++ {
		l.Err("mensaje " + strings.Repeat("x", 1))
	}
	if n := len(UltimosErrores(0)); n != maxRegistros {
		t.Fatalf("el anillo guardó %d, el tope es %d", n, maxRegistros)
	}
	// El contador sí sigue contando todo: es el total desde el arranque.
	if ContarErrores() != maxRegistros*3 {
		t.Fatalf("contador = %d", ContarErrores())
	}
}

func TestAnilloRecortaMensajesLargos(t *testing.T) {
	limpiar()
	l := New("error", io.Discard)
	l.Err(strings.Repeat("a", 900))
	got := UltimosErrores(1)[0].Mensaje
	if len([]rune(got)) > 301 {
		t.Fatalf("no se recortó: %d caracteres", len([]rune(got)))
	}
}

func TestAnilloIgnoraVacios(t *testing.T) {
	limpiar()
	l := New("error", io.Discard)
	l.Err("")
	if ContarErrores() != 0 {
		t.Fatalf("un mensaje vacío no debería contar")
	}
}

func TestLimpiar(t *testing.T) {
	limpiar()
	l := New("error", io.Discard)
	l.Err("uno")
	LimpiarErrores()
	if len(UltimosErrores(0)) != 0 || ContarErrores() != 0 {
		t.Fatalf("después de limpiar quedan datos")
	}
}
