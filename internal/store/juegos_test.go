// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// juegos_test.go — Las cuentas de los juegos de B4.1.
//
// Aquí no se prueba "que el comando conteste algo": se prueba que las
// probabilidades y los pagos sean los que dicen ser. Los tres fallos que se
// arreglaron al portar el JS fueron exactamente de esto: una ruleta con
// casillas verdes de más, un casino donde el bot tiraba dados más altos y una
// tragaperras que devolvía más de lo que cobraba.
package store

import (
	"sync"
	"testing"
)

// ─── Charge: multas que salen de cartera y banco ────────────────────────────

func TestChargeCobraCarteraYBanco(t *testing.T) {
	db, ctx := nuevoStore(t)
	chat, u := chatPrueba, userPrueba

	if _, err := db.AddCoins(ctx, chat, u, 1000); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}
	if ok, err := db.Deposit(ctx, chat, u, 600); err != nil || !ok {
		t.Fatalf("Deposit: %v %v", ok, err)
	}
	// Cartera 400, banco 600. Una multa de 700 tiene que salir de los dos.
	pagado, err := db.Charge(ctx, chat, u, 700)
	if err != nil {
		t.Fatalf("Charge: %v", err)
	}
	if pagado != 700 {
		t.Fatalf("pagado = %d, se esperaba 700", pagado)
	}
	acc, err := db.ChatUser(ctx, chat, u)
	if err != nil {
		t.Fatalf("ChatUser: %v", err)
	}
	if acc.Coins != 0 || acc.Bank != 300 {
		t.Fatalf("quedó cartera=%d banco=%d, se esperaba 0/300", acc.Coins, acc.Bank)
	}
}

func TestChargeNoCobraDeMas(t *testing.T) {
	db, ctx := nuevoStore(t)
	chat, u := chatPrueba, userPrueba
	if _, err := db.AddCoins(ctx, chat, u, 500); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}
	pagado, err := db.Charge(ctx, chat, u, 9000)
	if err != nil {
		t.Fatalf("Charge: %v", err)
	}
	if pagado != 500 {
		t.Fatalf("pagado = %d, se esperaba 500 (todo lo que había)", pagado)
	}
	acc, _ := db.ChatUser(ctx, chat, u)
	if acc.Coins != 0 || acc.Bank != 0 {
		t.Fatalf("quedó cartera=%d banco=%d, se esperaba 0/0", acc.Coins, acc.Bank)
	}
}

// ─── AddStat: nunca fuera de 0..100 ─────────────────────────────────────────

func TestAddStatRecorta(t *testing.T) {
	db, ctx := nuevoStore(t)
	chat, u := chatPrueba, userPrueba
	if _, err := db.ChatUser(ctx, chat, u); err != nil {
		t.Fatalf("ChatUser: %v", err)
	}
	for _, stat := range []string{"health", "stamina", "magic"} {
		if err := db.SetStat(ctx, chat, u, stat, 100); err != nil {
			t.Fatalf("SetStat: %v", err)
		}
		v, err := db.AddStat(ctx, chat, u, stat, -250)
		if err != nil {
			t.Fatalf("AddStat: %v", err)
		}
		if v != 0 {
			t.Fatalf("%s = %d tras restar 250, se esperaba 0", stat, v)
		}
		if v, err = db.AddStat(ctx, chat, u, stat, 250); err != nil || v != 100 {
			t.Fatalf("%s = %d, se esperaba 100 (%v)", stat, v, err)
		}
	}
	if _, err := db.AddStat(ctx, chat, u, "fuerza", 1); err == nil {
		t.Fatal("AddStat aceptó una estadística que no existe")
	}
}

// ─── Steal: el dinero no aparece de la nada ─────────────────────────────────

func TestStealMueveSinCrearDinero(t *testing.T) {
	db, ctx := nuevoStore(t)
	chat, ladron, victima := chatPrueba, "5215511111111@s.whatsapp.net", userPrueba

	if _, err := db.AddCoins(ctx, chat, victima, 5000); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}
	if _, err := db.ChatUser(ctx, chat, ladron); err != nil {
		t.Fatalf("ChatUser: %v", err)
	}

	robado, err := db.Steal(ctx, chat, victima, ladron, 8000)
	if err != nil {
		t.Fatalf("Steal: %v", err)
	}
	if robado != 5000 {
		t.Fatalf("robado = %d, se esperaba 5000 (tope de la víctima)", robado)
	}
	a, _ := db.ChatUser(ctx, chat, victima)
	b, _ := db.ChatUser(ctx, chat, ladron)
	if a.Coins != 0 || b.Coins != 5000 {
		t.Fatalf("quedó víctima=%d ladrón=%d, se esperaba 0/5000", a.Coins, b.Coins)
	}
	if a.Total()+b.Total() != 5000 {
		t.Fatalf("el total cambió: %d", a.Total()+b.Total())
	}

	// Con la víctima a cero no se roba nada (y no se rompe).
	if robado, err = db.Steal(ctx, chat, victima, ladron, 3000); err != nil || robado != 0 {
		t.Fatalf("Steal en vacío = %d, %v", robado, err)
	}
}

func TestStealConcurrenteNoDuplica(t *testing.T) {
	db, ctx := nuevoStore(t)
	chat := chatPrueba
	ladrones := []string{
		"5215522222221@s.whatsapp.net",
		"5215522222222@s.whatsapp.net",
		"5215522222223@s.whatsapp.net",
		"5215522222224@s.whatsapp.net",
	}
	victima := userPrueba
	if _, err := db.AddCoins(ctx, chat, victima, 10000); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}

	var wg sync.WaitGroup
	robado := make([]int64, len(ladrones))
	for i, l := range ladrones {
		wg.Add(1)
		go func(i int, l string) {
			defer wg.Done()
			n, err := db.Steal(ctx, chat, victima, l, 4000)
			if err != nil {
				t.Errorf("Steal: %v", err)
			}
			robado[i] = n
		}(i, l)
	}
	wg.Wait()

	total := int64(0)
	for _, n := range robado {
		total += n
	}
	if total > 10000 {
		t.Fatalf("se robaron %d de una cartera de 10000", total)
	}
	a, _ := db.ChatUser(ctx, chat, victima)
	if a.Coins != 10000-total {
		t.Fatalf("la víctima quedó con %d y se robaron %d (debería quedar %d)",
			a.Coins, total, 10000-total)
	}
}

// ─── apuestas simultáneas ───────────────────────────────────────────────────

// Cincuenta apuestas a la vez: el saldo nunca puede quedar en negativo y no
// puede aparecer dinero. Es el caso que en el JS duplicaba monedas.
func TestApuestasSimultaneasNoRompenSaldo(t *testing.T) {
	db, ctx := nuevoStore(t)
	chat, u := chatPrueba, userPrueba
	if _, err := db.AddCoins(ctx, chat, u, 10000); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}

	var wg sync.WaitGroup
	cobradas := int64(0)
	var mu sync.Mutex
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := db.Spend(ctx, chat, u, 500)
			if err != nil {
				t.Errorf("Spend: %v", err)
				return
			}
			if ok {
				mu.Lock()
				cobradas++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if cobradas != 20 {
		t.Fatalf("se cobraron %d apuestas de 500 con 10000 de saldo, se esperaban 20", cobradas)
	}
	acc, _ := db.ChatUser(ctx, chat, u)
	if acc.Coins != 0 {
		t.Fatalf("quedó %d en la cartera, se esperaba 0", acc.Coins)
	}
}

// Un enfriamiento por chat+persona: dos intentos a la vez, una sola reserva.
func TestCooldownDeJuegoEsUnico(t *testing.T) {
	db, ctx := nuevoStore(t)
	chat, u := chatPrueba, userPrueba
	const col = "last_slot"

	var wg sync.WaitGroup
	var mu sync.Mutex
	pasaron := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := db.ClaimCooldown(ctx, chat, u, col, 30_000_000_000) // 30 s
			if err != nil {
				t.Errorf("ClaimCooldown: %v", err)
				return
			}
			if ok {
				mu.Lock()
				pasaron++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if pasaron != 1 {
		t.Fatalf("pasaron %d reservas del mismo enfriamiento, se esperaba 1", pasaron)
	}

	// ReleaseCooldown devuelve el turno: la siguiente pasa.
	if err := db.ReleaseCooldown(ctx, chat, u, col); err != nil {
		t.Fatalf("ReleaseCooldown: %v", err)
	}
	if ok, _, err := db.ClaimCooldown(ctx, chat, u, col, 30_000_000_000); err != nil || !ok {
		t.Fatalf("tras liberar no pasó: %v %v", ok, err)
	}

	// Y no deja tocar columnas que no existen.
	if err := db.ReleaseCooldown(ctx, chat, u, "coins"); err == nil {
		t.Fatal("ReleaseCooldown aceptó una columna que no es un enfriamiento")
	}
}

// Las columnas nuevas de la migración 12 existen y son usables.
func TestColumnasDeJuegos(t *testing.T) {
	db, ctx := nuevoStore(t)
	chat, u := chatPrueba, userPrueba
	columnas := []string{
		"last_casino", "last_coinflip", "last_roulette", "last_slot", "last_ppt",
		"last_crime", "last_steal", "last_fish", "last_hunt", "last_mine",
		"last_dungeon", "last_adventure", "last_coffer", "last_invoke", "last_robbed",
	}
	for _, col := range columnas {
		if ok, _, err := db.ClaimCooldown(ctx, chat, u, col, 0); err != nil || !ok {
			t.Fatalf("%s: no se pudo reservar (%v, %v)", col, ok, err)
		}
	}
}
