// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEstadisticasCuenta(t *testing.T) {
	ctx := context.Background()
	ruta := filepath.Join(t.TempDir(), "shin.db")
	db, err := Open(ruta)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	e, err := db.Estadisticas(ctx, ruta)
	if err != nil {
		t.Fatalf("Estadisticas: %v", err)
	}
	if e.Usuarios != 0 || e.Chats != 0 {
		t.Fatalf("base nueva con datos: %+v", e)
	}
	if e.Bytes <= 0 {
		t.Fatalf("no se midió el archivo: %+v", e)
	}

	if _, err := db.PerfilDe(ctx, perfilAna); err != nil {
		t.Fatalf("PerfilDe: %v", err)
	}
	if _, err := db.AddWarn(ctx, "grupo@g.us", perfilAna, "spam", perfilBeto); err != nil {
		t.Fatalf("AddWarn: %v", err)
	}
	if _, err := db.ChatUser(ctx, "grupo@g.us", perfilAna); err != nil {
		t.Fatalf("ChatUser: %v", err)
	}

	e, err = db.Estadisticas(ctx, ruta)
	if err != nil {
		t.Fatalf("Estadisticas: %v", err)
	}
	if e.Usuarios != 1 {
		t.Fatalf("usuarios = %d", e.Usuarios)
	}
	if e.Chats != 1 {
		t.Fatalf("chats = %d", e.Chats)
	}
	if e.Advertencias != 1 {
		t.Fatalf("advertencias = %d", e.Advertencias)
	}

	// Sin ruta no se inventa un tamaño ni se cae.
	if v := tamanoDB(""); v != 0 {
		t.Fatalf("tamanoDB(\"\") = %d", v)
	}
	if v := tamanoDB(filepath.Join(t.TempDir(), "no-existe.db")); v != 0 {
		t.Fatalf("tamanoDB de algo que no existe = %d", v)
	}
}
