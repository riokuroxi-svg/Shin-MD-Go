// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package store

import (
	"context"
	"path/filepath"
	"testing"
)

// Prueba de punta a punta SIN WhatsApp: abre SQLite en un temporal, corre
// las migraciones, da monedas y sube XP. En el bot de JS esto obligaba a
// arrancar medio bot para probar lo mismo.
func TestStoreEndToEnd(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shin.db")

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	jid := "5215555555555@s.whatsapp.net"

	acc, err := db.Get(ctx, jid)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if acc.Coins != 0 || acc.Level != 1 {
		t.Fatalf("cuenta nueva inesperada: %+v", acc)
	}

	coins, err := db.AddGlobalCoins(ctx, jid, 250)
	if err != nil || coins != 250 {
		t.Fatalf("AddCoins = %d (%v)", coins, err)
	}

	acc, err = db.AddXP(ctx, jid, 350)
	if err != nil {
		t.Fatalf("AddXP: %v", err)
	}
	if acc.Level < 2 {
		t.Fatalf("el nivel no subió: %+v", acc)
	}

	if n, err := db.CountUsers(ctx); err != nil || n != 1 {
		t.Fatalf("CountUsers = %d (%v)", n, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reabrir no debe repetir migraciones ni perder datos.
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reabrir: %v", err)
	}
	defer db2.Close()
	acc, err = db2.Get(ctx, jid)
	if err != nil || acc.Coins != 250 {
		t.Fatalf("tras reabrir: %+v (%v)", acc, err)
	}
}
