// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

const chatRecordatorio = "120363000000000001@g.us"

func TestRecordatorioVenceUnaSolaVez(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "shin.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ahora := time.Now()
	if _, err := db.PonRecordatorio(ctx, chatRecordatorio, perfilAna, "tomar agua", ahora.Add(-time.Minute)); err != nil {
		t.Fatalf("PonRecordatorio: %v", err)
	}

	vencidos, err := db.RecordatoriosVencidos(ctx, ahora)
	if err != nil {
		t.Fatalf("RecordatoriosVencidos: %v", err)
	}
	if len(vencidos) != 1 || vencidos[0].Texto != "tomar agua" || vencidos[0].ChatID != chatRecordatorio {
		t.Fatalf("vencidos = %+v", vencidos)
	}

	// La segunda pasada no puede repetir el aviso: ya se borró al entregarlo.
	otra, err := db.RecordatoriosVencidos(ctx, ahora)
	if err != nil {
		t.Fatalf("segunda pasada: %v", err)
	}
	if len(otra) != 0 {
		t.Fatalf("se avisó dos veces: %+v", otra)
	}
}

func TestRecordatorioFuturoNoVence(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "shin.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ahora := time.Now()
	if _, err := db.PonRecordatorio(ctx, chatRecordatorio, perfilAna, "más tarde", ahora.Add(10*time.Minute)); err != nil {
		t.Fatalf("PonRecordatorio: %v", err)
	}
	vencidos, err := db.RecordatoriosVencidos(ctx, ahora)
	if err != nil {
		t.Fatalf("RecordatoriosVencidos: %v", err)
	}
	if len(vencidos) != 0 {
		t.Fatalf("un recordatorio futuro no debe avisarse: %+v", vencidos)
	}

	// Y sigue ahí para cuando llegue su hora.
	pendientes, err := db.RecordatoriosDe(ctx, chatRecordatorio, perfilAna)
	if err != nil || len(pendientes) != 1 {
		t.Fatalf("pendientes = %+v, err = %v", pendientes, err)
	}
}

func TestRecordatorioEsDeSuDueño(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "shin.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	id, err := db.PonRecordatorio(ctx, chatRecordatorio, perfilAna, "mío", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("PonRecordatorio: %v", err)
	}
	// Beto no puede borrar el recordatorio de Ana.
	if err := db.BorrarRecordatorio(ctx, id, perfilBeto); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("se dejó borrar el de otra persona: %v", err)
	}
	if err := db.BorrarRecordatorio(ctx, id, perfilAna); err != nil {
		t.Fatalf("el dueño no pudo borrarlo: %v", err)
	}
	if restantes, _ := db.RecordatoriosDe(ctx, chatRecordatorio, perfilAna); len(restantes) != 0 {
		t.Fatalf("quedaron %d", len(restantes))
	}
}

func TestRecordatorioListaPorChat(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "shin.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	base := time.Now()
	if _, err := db.PonRecordatorio(ctx, chatRecordatorio, perfilAna, "segundo", base.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PonRecordatorio(ctx, chatRecordatorio, perfilAna, "primero", base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PonRecordatorio(ctx, "otro@g.us", perfilAna, "de otro chat", base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	lista, err := db.RecordatoriosDe(ctx, chatRecordatorio, perfilAna)
	if err != nil {
		t.Fatalf("RecordatoriosDe: %v", err)
	}
	if len(lista) != 2 {
		t.Fatalf("se esperaban 2 del chat, salieron %d", len(lista))
	}
	// Ordenados por cuándo, no por cuándo se crearon.
	if lista[0].Texto != "primero" || lista[1].Texto != "segundo" {
		t.Fatalf("orden inesperado: %q, %q", lista[0].Texto, lista[1].Texto)
	}
}

func TestRecordatorioRechazaVacios(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "shin.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	casos := []struct{ chat, user, texto string }{
		{"", perfilAna, "x"},
		{chatRecordatorio, "", "x"},
		{chatRecordatorio, perfilAna, ""},
	}
	for _, c := range casos {
		if _, err := db.PonRecordatorio(ctx, c.chat, c.user, c.texto, time.Now()); err == nil {
			t.Fatalf("se aceptó un recordatorio inválido: %+v", c)
		}
	}
}

func TestLimpiarRecordatoriosViejos(t *testing.T) {
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "shin.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	ahora := time.Now()
	if _, err := db.PonRecordatorio(ctx, chatRecordatorio, perfilAna, "viejo", ahora.Add(-10*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.PonRecordatorio(ctx, chatRecordatorio, perfilAna, "reciente", ahora.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	borrados, err := db.LimpiarRecordatoriosViejos(ctx, ahora)
	if err != nil {
		t.Fatalf("Limpiar: %v", err)
	}
	if borrados != 1 {
		t.Fatalf("borró %d, se esperaba 1", borrados)
	}
	if quedan, _ := db.RecordatoriosDe(ctx, chatRecordatorio, perfilAna); len(quedan) != 1 {
		t.Fatalf("quedaron %d", len(quedan))
	}
}
