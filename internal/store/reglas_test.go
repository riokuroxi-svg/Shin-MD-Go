// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTemp(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "shin.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

// Roxi estuvo aquí — probé .daily como 200 veces para creerme que ya no
// pagaba doble xD
//
// El .daily pagaba cada vez que se ejecutaba: se podían farmear monedas sin
// límite. Ahora la primera llamada del día reclama y la segunda no.
func TestClaimDailyOncePerDay(t *testing.T) {
	ctx := context.Background()
	db, _ := openTemp(t)

	jid := "5215555555555@s.whatsapp.net"

	claimed, err := db.ClaimDaily(ctx, jid, "2026-11-05")
	if err != nil || !claimed {
		t.Fatalf("primer daily: claimed=%v err=%v", claimed, err)
	}

	claimed, err = db.ClaimDaily(ctx, jid, "2026-11-05")
	if err != nil {
		t.Fatalf("segundo daily: %v", err)
	}
	if claimed {
		t.Fatal("el segundo .daily del mismo día no debería dar premio")
	}

	claimed, err = db.ClaimDaily(ctx, jid, "2026-11-06")
	if err != nil || !claimed {
		t.Fatalf("daily del día siguiente: claimed=%v err=%v", claimed, err)
	}

	if last, err := db.LastDaily(ctx, jid); err != nil || last != "2026-11-06" {
		t.Fatalf("LastDaily = %q (%v)", last, err)
	}
	if last, err := db.LastDaily(ctx, "otro@s.whatsapp.net"); err != nil || last != "" {
		t.Fatalf("usuario sin daily: %q (%v)", last, err)
	}
}

// La clave entra en una ruta JSON de SQLite: una clave con comillas o puntos
// podía reescribir el JSON entero del usuario.
func TestSettingsRejectBadKeys(t *testing.T) {
	ctx := context.Background()
	db, _ := openTemp(t)
	jid := "5215555555555@s.whatsapp.net"

	malas := []string{"", "a.b", `a'"`, "clave con espacios", "x'; DROP TABLE settings; --"}
	for _, k := range malas {
		if err := db.SetSetting(ctx, jid, k, "1"); err == nil {
			t.Errorf("SetSetting aceptó la clave %q", k)
		}
		if _, _, err := db.GetSetting(ctx, jid, k); err == nil {
			t.Errorf("GetSetting aceptó la clave %q", k)
		}
	}

	if err := db.SetSetting(ctx, jid, "antilink", "on"); err != nil {
		t.Fatalf("clave válida rechazada: %v", err)
	}
	if v, ok, err := db.GetSetting(ctx, jid, "antilink"); err != nil || !ok || v != "on" {
		t.Fatalf("GetSetting = %q ok=%v err=%v", v, ok, err)
	}

	// Escribir una clave no debe borrar las demás.
	if err := db.SetSetting(ctx, jid, "selfmode", "off"); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := db.GetSetting(ctx, jid, "antilink"); !ok || v != "on" {
		t.Fatalf("antilink se perdió al escribir otra clave: %q", v)
	}
	if _, ok, _ := db.GetSetting(ctx, jid, "noexiste"); ok {
		t.Fatal("una clave inexistente devolvió ok=true")
	}
}

// El ancla del calentamiento tiene que sobrevivir a un reinicio: si se pierde,
// el cupo vuelve a 20/día y el número nunca calienta.
func TestWarmupAnchorPersists(t *testing.T) {
	ctx := context.Background()
	db, path := openTemp(t)

	first := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)
	anchor, err := db.WarmupAnchor(ctx, first)
	if err != nil {
		t.Fatalf("WarmupAnchor: %v", err)
	}
	if want := "2026-11-01"; anchor.Format("2006-01-02") != want {
		t.Fatalf("ancla = %s, se esperaba %s", anchor.Format("2006-01-02"), want)
	}

	// Simula un reinicio tres días después: la fecha guardada no cambia.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reabrir: %v", err)
	}
	defer db2.Close()

	anchor, err = db2.WarmupAnchor(ctx, first.AddDate(0, 0, 3))
	if err != nil {
		t.Fatalf("WarmupAnchor tras reinicio: %v", err)
	}
	if got := anchor.Format("2006-01-02"); got != "2026-11-01" {
		t.Fatalf("el ancla se reinició: %s", got)
	}
}

// Un coste mayor que la XP acumulada no debe dejar XP ni nivel negativos.
func TestAddXPNeverNegative(t *testing.T) {
	ctx := context.Background()
	db, _ := openTemp(t)
	jid := "5215555555555@s.whatsapp.net"

	if _, err := db.AddXP(ctx, jid, 50); err != nil {
		t.Fatal(err)
	}
	acc, err := db.AddXP(ctx, jid, -5000)
	if err != nil {
		t.Fatal(err)
	}
	if acc.XP != 0 || acc.Level < 1 {
		t.Fatalf("XP/nivel quedaron mal: %+v", acc)
	}
}

// Las parejas LID ↔ teléfono son lo que evita que un usuario tenga dos cuentas
// de monedas. Tienen que sobrevivir al reinicio (como todo lo demás).
func TestIdentitiesPersist(t *testing.T) {
	ctx := context.Background()
	db, path := openTemp(t)

	const (
		lid = "123456789012345@lid"
		pn  = "5215512345678@s.whatsapp.net"
	)

	if err := db.PutIdentity(ctx, lid, pn); err != nil {
		t.Fatalf("PutIdentity: %v", err)
	}

	if got, err := db.PNForLID(ctx, lid); err != nil || got != pn {
		t.Fatalf("PNForLID = %q (%v)", got, err)
	}
	if got, err := db.LIDForPN(ctx, pn); err != nil || got != lid {
		t.Fatalf("LIDForPN = %q (%v)", got, err)
	}

	// Datos incompletos no se guardan y no revientan.
	if err := db.PutIdentity(ctx, "", pn); err != nil {
		t.Fatalf("PutIdentity vacío: %v", err)
	}
	if err := db.PutIdentity(ctx, lid, lid); err != nil {
		t.Fatalf("PutIdentity iguales: %v", err)
	}
	if n, err := db.CountIdentities(ctx); err != nil || n != 1 {
		t.Fatalf("CountIdentities = %d (%v)", n, err)
	}

	// Un LID desconocido devuelve vacío, no un error.
	if got, err := db.PNForLID(ctx, "999999999999@lid"); err != nil || got != "" {
		t.Fatalf("LID desconocido = %q (%v)", got, err)
	}

	// Tras reiniciar sigue sabiendo la pareja.
	_ = db.Close()
	db2, err := Open(path)
	if err != nil {
		t.Fatalf("reabrir: %v", err)
	}
	defer db2.Close()
	if got, err := db2.PNForLID(ctx, lid); err != nil || got != pn {
		t.Fatalf("tras reiniciar = %q (%v)", got, err)
	}
}

// Si WhatsApp cambia el LID de alguien, el teléfono debe apuntar al más nuevo.
func TestIdentityUpdatedLID(t *testing.T) {
	ctx := context.Background()
	db, _ := openTemp(t)

	const pn = "5215599999999@s.whatsapp.net"
	if err := db.PutIdentity(ctx, "111@lid", pn); err != nil {
		t.Fatal(err)
	}
	if err := db.PutIdentity(ctx, "222@lid", pn); err != nil { // LID nuevo
		t.Fatal(err)
	}

	if got, err := db.LIDForPN(ctx, pn); err != nil || got != "222@lid" {
		t.Fatalf("LIDForPN = %q, se esperaba el más reciente (%v)", got, err)
	}
	if got, err := db.PNForLID(ctx, "111@lid"); err != nil || got != pn {
		t.Fatalf("el LID viejo debería seguir resolviendo: %q (%v)", got, err)
	}
}

// Un usuario que escribió cuando solo se conocía su LID no puede perder las
// monedas cuando el bot aprende su teléfono.
func TestIdentityMergesAccounts(t *testing.T) {
	ctx := context.Background()
	db, _ := openTemp(t)

	const (
		lid = "444444444444@lid"
		pn  = "5215566666666@s.whatsapp.net"
	)

	// 1) Primero solo se conocía el LID: ahí cayó el .daily.
	if _, err := db.AddCoins(ctx, lid, 500); err != nil {
		t.Fatal(err)
	}
	if claimed, err := db.ClaimDaily(ctx, lid, "2026-11-05"); err != nil || !claimed {
		t.Fatalf("daily por LID: %v %v", claimed, err)
	}

	// 2) El bot aprende la pareja (llega un mensaje con teléfono).
	if err := db.PutIdentity(ctx, lid, pn); err != nil {
		t.Fatalf("PutIdentity: %v", err)
	}

	acc, err := db.Get(ctx, pn)
	if err != nil {
		t.Fatalf("Get por teléfono: %v", err)
	}
	if acc.Coins != 500 {
		t.Fatalf("monedas = %d, se esperaban 500 (se perdieron en la fusión)", acc.Coins)
	}
	if last, _ := db.LastDaily(ctx, pn); last != "2026-11-05" {
		t.Fatalf("el daily no se movió: %q", last)
	}
	if n, _ := db.CountUsers(ctx); n != 1 {
		t.Fatalf("quedaron %d cuentas, debería haber 1", n)
	}
}

// Si ya existían las dos cuentas, se suman en la del teléfono.
func TestIdentitySumsBothAccounts(t *testing.T) {
	ctx := context.Background()
	db, _ := openTemp(t)

	const (
		lid = "555555555555@lid"
		pn  = "5215577777777@s.whatsapp.net"
	)

	if _, err := db.AddCoins(ctx, lid, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddCoins(ctx, pn, 250); err != nil {
		t.Fatal(err)
	}
	if err := db.PutIdentity(ctx, lid, pn); err != nil {
		t.Fatal(err)
	}

	acc, err := db.Get(ctx, pn)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Coins != 350 {
		t.Fatalf("monedas = %d, se esperaban 350 (100 + 250)", acc.Coins)
	}
	if got, _ := db.Get(ctx, lid); got.Coins != 0 {
		t.Fatalf("la cuenta del LID debería haber quedado vacía, tiene %d", got.Coins)
	}
}
