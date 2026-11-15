// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// gacha_extra_test.go — la economía del harem: vender, comprar cobrando la
// economía, tradear atómico, robar (solo ni fav ni a la venta), borrar, y
// los dos tableros. Todo contra SQLite en memoria, sin red.
package store

import (
	"context"
	"testing"
	"time"
)

func tienda(t *testing.T) (*Store, context.Context) {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, context.Background()
}

func dar(t *testing.T, s *Store, ctx context.Context, chat, owner, id, nombre string, valor int64) {
	t.Helper()
	if err := s.SaveRoll(ctx, Roll{ChatID: chat, MessageID: "m" + owner + id, CharID: id, Name: nombre, Value: valor, RollerID: owner, ExpiresAt: NowMs() + 60000}); err != nil {
		t.Fatalf("SaveRoll: %v", err)
	}
	r, err := s.RollByMessage(ctx, chat, "m"+owner+id)
	if err != nil || r == nil {
		t.Fatalf("RollByMessage: %v %v", r, err)
	}
	if _, err := s.ClaimRoll(ctx, chat, "m"+owner+id, owner, time.Now()); err != nil {
		t.Fatalf("ClaimRoll: %v", err)
	}
}

func TestVentaCompraCompleta(t *testing.T) {
	s, ctx := tienda(t)
	dar(t, s, ctx, "g", "ana", "c1", "Asuna", 1000)

	if _, err := s.PutOnSale(ctx, "g", "ana", "c1", 500); err != nil {
		t.Fatalf("PutOnSale: %v", err)
	}
	ventas, err := s.ListSales(ctx, "g", 10)
	if err != nil || len(ventas) != 1 || ventas[0].Price != 500 {
		t.Fatalf("ListSales: %v %v", err, ventas)
	}

	// Comprador con fondos: paga y recibe.
	if _, err := s.AddCoins(ctx, "g", "beto", 600); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}
	sale, err := s.SaleByID(ctx, "g", "c1")
	if err != nil || sale == nil {
		t.Fatalf("SaleByID: %v", err)
	}
	if err := s.BuySale(ctx, "g", "beto", sale); err != nil {
		t.Fatalf("BuySale: %v", err)
	}
	dueno, err := s.OwnerOf(ctx, "g", "c1")
	if err != nil || dueno == nil || dueno.OwnerID != "beto" {
		t.Fatalf("tras BuySale el dueño no cambió: %+v", dueno)
	}
	ventas, _ = s.ListSales(ctx, "g", 10)
	if len(ventas) != 0 {
		t.Fatalf("la venta no se cerró: %v", ventas)
	}
	// Ana recibió el dinero.
	acct, _ := s.ChatUser(ctx, "g", "ana")
	if acct == nil || acct.Coins < 500 {
		t.Fatalf("ana no cobró: %+v", acct)
	}
}

func TestVentaSinFondosNoMueve(t *testing.T) {
	s, ctx := tienda(t)
	dar(t, s, ctx, "g", "ana", "c1", "Asuna", 100)
	if ok, err := s.PutOnSale(ctx, "g", "ana", "c1", 400); err != nil || !ok {
		t.Fatalf("PutOnSale: %v %v", ok, err)
	}
	if ok, err := s.Spend(ctx, "g", "beto", 400); err != nil || ok {
		t.Fatal("sin fondos Spend debía devolver false")
	}
}

func TestTradeYRobo(t *testing.T) {
	s, ctx := tienda(t)
	dar(t, s, ctx, "g", "ana", "a1", "A1", 10)
	dar(t, s, ctx, "g", "beto", "b1", "B1", 20)

	if err := s.SwapCharacters(ctx, "g", "ana", "a1", "beto", "b1"); err != nil {
		t.Fatalf("Swap: %v", err)
	}
	o1, _ := s.OwnerOf(ctx, "g", "a1")
	o2, _ := s.OwnerOf(ctx, "g", "b1")
	if o1.OwnerID != "beto" || o2.OwnerID != "ana" {
		t.Fatalf("swap: %v %v", o1, o2)
	}

	// El robo respeta fav y venta.
	if err := s.SetFavorite(ctx, "g", "ana", "b1"); err != nil {
		t.Fatalf("SetFavorite: %v", err)
	}
	if _, err := s.PutOnSale(ctx, "g", "ana", "b1", 50); err != nil {
		t.Fatalf("PutOnSale: %v", err)
	}
	robado, err := s.StealChar(ctx, "g", "ana", "eve")
	if err != nil || robado != nil {
		t.Fatalf("robable no debía haber nada: %v %+v", err, robado)
	}
}

func TestTableros(t *testing.T) {
	s, ctx := tienda(t)
	dar(t, s, ctx, "g", "ana", "c1", "A", 10)
	dar(t, s, ctx, "g", "beto", "c2", "B", 200)

	porValor, err := s.CharBoard(ctx, "g", 10, true)
	if err != nil || len(porValor) != 2 || porValor[0].CharID != "c2" {
		t.Fatalf("CharBoard byValue: %v %v", err, porValor)
	}
	porCount, _ := s.CharBoard(ctx, "g", 10, false)
	if len(porCount) != 2 || porCount[0].Count != 1 {
		t.Fatalf("CharBoard byCount: %v", porCount)
	}
	if err := s.SetFavorite(ctx, "g", "beto", "c2"); err != nil {
		t.Fatalf("fav: %v", err)
	}
	favs, err := s.FavBoard(ctx, "g", 10)
	if err != nil || len(favs) != 1 || favs[0].CharID != "c2" {
		t.Fatalf("FavBoard: %v %v", err, favs)
	}
}

func TestBorraSuelta(t *testing.T) {
	s, ctx := tienda(t)
	dar(t, s, ctx, "g", "ana", "c1", "Asuna", 10)
	ok, err := s.DeleteOwned(ctx, "g", "ana", "c1")
	if err != nil || !ok {
		t.Fatalf("DeleteOwned: %v %v", ok, err)
	}
	if o, _ := s.OwnerOf(ctx, "g", "c1"); o != nil {
		t.Fatalf("sigue vivo: %+v", o)
	}
}
