// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// economia_test.go — Lo que se prueba aquí no es "que las sumas sumen": es que
// la economía no se pueda romper. Cada caso corresponde a un fallo real del bot
// de JS, donde el saldo se leía, se calculaba en JavaScript y se escribía
// detrás: con dos mensajes a la vez salían monedas de la nada.
package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func nuevoStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "shin.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, ctx
}

const (
	chatPrueba = "120363000000000000@g.us"
	userPrueba = "5215500000001@s.whatsapp.net"
)

// ─── migraciones ────────────────────────────────────────────────────────────

// La base nueva tiene que llegar al último escalón, y abrirla otra vez no debe
// volver a aplicarlos (si los repitiera, los ALTER fallarían en el arranque).
func TestMigracionesIdempotentes(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "shin.db")

	for i := 0; i < 3; i++ {
		db, err := Open(path)
		if err != nil {
			t.Fatalf("Open #%d: %v", i+1, err)
		}
		var version int
		if err := db.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
			t.Fatalf("schema_version: %v", err)
		}
		if version != len(migrations) {
			t.Fatalf("versión %d, se esperaban %d", version, len(migrations))
		}
		_ = db.Close()
	}
}

// ─── economía por chat ──────────────────────────────────────────────────────

// El mismo usuario tiene saldos distintos en cada grupo, como en g_chat_users.
func TestEconomiaPorChat(t *testing.T) {
	db, ctx := nuevoStore(t)
	const otroChat = "120363000000000001@g.us"

	if _, err := db.AddCoins(ctx, chatPrueba, userPrueba, 1000); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}
	uno, err := db.ChatUser(ctx, chatPrueba, userPrueba)
	if err != nil {
		t.Fatalf("ChatUser: %v", err)
	}
	otro, err := db.ChatUser(ctx, otroChat, userPrueba)
	if err != nil {
		t.Fatalf("ChatUser otro: %v", err)
	}
	if uno.Coins != 1000 {
		t.Fatalf("cartera en el primer chat = %d", uno.Coins)
	}
	if otro.Coins != 0 {
		t.Fatalf("el saldo se filtró entre chats: %d", otro.Coins)
	}
}

// 40 mensajes a la vez pidiendo gastar 100 de un saldo de 1000: solo pueden
// pasar 10. En el JS entraban todos y el saldo acababa en negativo.
func TestSpendConcurrenteNoDuplica(t *testing.T) {
	db, ctx := nuevoStore(t)
	if _, err := db.AddCoins(ctx, chatPrueba, userPrueba, 1000); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		cobrados int
	)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := db.Spend(ctx, chatPrueba, userPrueba, 100)
			if err != nil {
				t.Errorf("Spend: %v", err)
				return
			}
			if ok {
				mu.Lock()
				cobrados++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if cobrados != 10 {
		t.Fatalf("se cobraron %d veces, se esperaban 10", cobrados)
	}
	acc, err := db.ChatUser(ctx, chatPrueba, userPrueba)
	if err != nil {
		t.Fatalf("ChatUser: %v", err)
	}
	if acc.Coins != 0 {
		t.Fatalf("cartera final = %d, se esperaba 0", acc.Coins)
	}
}

func TestAddCoinsNoBajaDeCero(t *testing.T) {
	db, ctx := nuevoStore(t)
	if _, err := db.AddCoins(ctx, chatPrueba, userPrueba, 500); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}
	coins, err := db.AddCoins(ctx, chatPrueba, userPrueba, -5000)
	if err != nil {
		t.Fatalf("AddCoins negativo: %v", err)
	}
	if coins != 0 {
		t.Fatalf("cartera = %d, se esperaba 0 (nunca negativa)", coins)
	}
}

func TestDepositWithdraw(t *testing.T) {
	db, ctx := nuevoStore(t)
	if _, err := db.AddCoins(ctx, chatPrueba, userPrueba, 3000); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}

	if ok, err := db.Deposit(ctx, chatPrueba, userPrueba, 2000); err != nil || !ok {
		t.Fatalf("Deposit = %v (%v)", ok, err)
	}
	acc, _ := db.ChatUser(ctx, chatPrueba, userPrueba)
	if acc.Coins != 1000 || acc.Bank != 2000 {
		t.Fatalf("tras depositar: cartera %d banco %d", acc.Coins, acc.Bank)
	}

	// No se puede depositar más de lo que hay en la cartera.
	if ok, _ := db.Deposit(ctx, chatPrueba, userPrueba, 5000); ok {
		t.Fatal("se depositó más dinero del que había")
	}

	if ok, err := db.Withdraw(ctx, chatPrueba, userPrueba, 1500); err != nil || !ok {
		t.Fatalf("Withdraw = %v (%v)", ok, err)
	}
	acc, _ = db.ChatUser(ctx, chatPrueba, userPrueba)
	if acc.Coins != 2500 || acc.Bank != 500 {
		t.Fatalf("tras retirar: cartera %d banco %d", acc.Coins, acc.Bank)
	}

	// Retirar más que el banco no deja el saldo en negativo.
	if ok, _ := db.Withdraw(ctx, chatPrueba, userPrueba, 999999); ok {
		t.Fatal("se retiró más de lo que había en el banco")
	}
}

// ─── transferencias ─────────────────────────────────────────────────────────

func TestTransferVaDelBanco(t *testing.T) {
	db, ctx := nuevoStore(t)
	const otro = "5215500000002@s.whatsapp.net"

	if _, err := db.AddCoins(ctx, chatPrueba, userPrueba, 5000); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}
	if ok, err := db.Deposit(ctx, chatPrueba, userPrueba, 5000); err != nil || !ok {
		t.Fatalf("Deposit: %v (%v)", ok, err)
	}

	ok, err := db.Transfer(ctx, chatPrueba, userPrueba, otro, 1200)
	if err != nil || !ok {
		t.Fatalf("Transfer = %v (%v)", ok, err)
	}
	origen, _ := db.ChatUser(ctx, chatPrueba, userPrueba)
	destino, _ := db.ChatUser(ctx, chatPrueba, otro)
	if origen.Bank != 3800 || origen.Coins != 0 {
		t.Fatalf("origen: banco %d cartera %d", origen.Bank, origen.Coins)
	}
	if destino.Bank != 1200 {
		t.Fatalf("destino: banco %d", destino.Bank)
	}
}

func TestTransferRechazaBasura(t *testing.T) {
	db, ctx := nuevoStore(t)
	if _, err := db.AddCoins(ctx, chatPrueba, userPrueba, 1000); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}
	casos := []struct {
		nombre   string
		destino  string
		cantidad int64
	}{
		{"a uno mismo", userPrueba, 100},
		{"cantidad cero", "5215500000002@s.whatsapp.net", 0},
		{"cantidad negativa", "5215500000002@s.whatsapp.net", -50},
		{"destino vacío", "", 100},
		{"por encima del techo", "5215500000002@s.whatsapp.net", MaxTransfer + 1},
	}
	for _, caso := range casos {
		ok, err := db.Transfer(ctx, chatPrueba, userPrueba, caso.destino, caso.cantidad)
		if err != nil {
			t.Fatalf("%s: %v", caso.nombre, err)
		}
		if ok {
			t.Fatalf("%s: la transferencia no debió pasar", caso.nombre)
		}
	}

	// Y con dinero en la cartera (no en el banco) tampoco: el .pay del JS
	// mueve desde el banco.
	ok, _ := db.Transfer(ctx, chatPrueba, userPrueba, "5215500000002@s.whatsapp.net", 100)
	if ok {
		t.Fatal("se transfirió dinero que estaba en la cartera")
	}
}

// Si dos transferencias salen a la vez con el banco justo para una, solo una
// debe completarse (en el JS se abonaban las dos y el saldo quedaba en rojo).
func TestTransferConcurrente(t *testing.T) {
	db, ctx := nuevoStore(t)
	const otro = "5215500000002@s.whatsapp.net"

	if _, err := db.AddCoins(ctx, chatPrueba, userPrueba, 1000); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}
	if _, err := db.Deposit(ctx, chatPrueba, userPrueba, 1000); err != nil {
		t.Fatalf("Deposit: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := db.Transfer(ctx, chatPrueba, userPrueba, otro, 1000); err != nil {
				t.Errorf("Transfer: %v", err)
			}
		}()
	}
	wg.Wait()

	origen, _ := db.ChatUser(ctx, chatPrueba, userPrueba)
	destino, _ := db.ChatUser(ctx, chatPrueba, otro)
	if origen.Bank != 0 || destino.Bank != 1000 {
		t.Fatalf("origen %d / destino %d: se duplicó o se perdió dinero", origen.Bank, destino.Bank)
	}
}

// ─── enfriamientos ──────────────────────────────────────────────────────────

func TestClaimCooldownEsAtomico(t *testing.T) {
	db, ctx := nuevoStore(t)

	ok, _, err := db.ClaimCooldown(ctx, chatPrueba, userPrueba, "last_work", 3*time.Minute)
	if err != nil || !ok {
		t.Fatalf("primer claim = %v (%v)", ok, err)
	}
	ok, restante, err := db.ClaimCooldown(ctx, chatPrueba, userPrueba, "last_work", 3*time.Minute)
	if err != nil {
		t.Fatalf("segundo claim: %v", err)
	}
	if ok {
		t.Fatal("el enfriamiento dejó pasar dos veces seguidas")
	}
	if restante <= 0 || restante > 3*time.Minute {
		t.Fatalf("restante raro: %v", restante)
	}
}

func TestClaimCooldownConcurrente(t *testing.T) {
	db, ctx := nuevoStore(t)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		pasaron int
	)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, err := db.ClaimCooldown(ctx, chatPrueba, userPrueba, "last_daily", 24*time.Hour)
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
		t.Fatalf("el daily se reclamó %d veces", pasaron)
	}
}

// Columna inventada: tiene que rechazarse, no acabar dentro del SQL.
func TestClaimCooldownRechazaColumna(t *testing.T) {
	db, ctx := nuevoStore(t)
	if _, _, err := db.ClaimCooldown(ctx, chatPrueba, userPrueba, "coins = 999, last_work", time.Minute); err == nil {
		t.Fatal("se aceptó un nombre de columna inventado")
	}
}

// ─── rachas ─────────────────────────────────────────────────────────────────

func TestRachaDiariaSubeYBaja(t *testing.T) {
	db, ctx := nuevoStore(t)
	base := time.Now().UTC()

	// Día 1.
	st, err := db.ClaimStreak(ctx, userPrueba, StreakDaily, base)
	if err != nil {
		t.Fatalf("ClaimStreak: %v", err)
	}
	if !st.Claimed || st.Streak != 1 {
		t.Fatalf("día 1: claimed=%v streak=%d", st.Claimed, st.Streak)
	}

	// El mismo día otra vez: no cobra y no sube.
	st, err = db.ClaimStreak(ctx, userPrueba, StreakDaily, base.Add(time.Hour))
	if err != nil {
		t.Fatalf("ClaimStreak mismo día: %v", err)
	}
	if st.Claimed || st.Streak != 1 {
		t.Fatalf("mismo día: claimed=%v streak=%d", st.Claimed, st.Streak)
	}

	// Día 2.
	st, _ = db.ClaimStreak(ctx, userPrueba, StreakDaily, base.Add(25*time.Hour))
	if !st.Claimed || st.Streak != 2 {
		t.Fatalf("día 2: claimed=%v streak=%d", st.Claimed, st.Streak)
	}

	// Se le pasa el margen (más de 1.5 días): la racha vuelve a 1.
	st, _ = db.ClaimStreak(ctx, userPrueba, StreakDaily, base.Add(25*time.Hour+40*time.Hour))
	if !st.Claimed || st.Streak != 1 {
		t.Fatalf("tras perder la racha: claimed=%v streak=%d", st.Claimed, st.Streak)
	}
}

// La racha es global: reclamar en un segundo grupo el mismo día no la sube.
func TestRachaEsGlobalNoPorChat(t *testing.T) {
	db, ctx := nuevoStore(t)
	now := time.Now().UTC()

	if _, err := db.ClaimStreak(ctx, userPrueba, StreakDaily, now); err != nil {
		t.Fatalf("ClaimStreak: %v", err)
	}
	st, _ := db.ClaimStreak(ctx, userPrueba, StreakDaily, now.Add(time.Minute))
	if st.Streak != 1 {
		t.Fatalf("la racha se contó dos veces: %d", st.Streak)
	}
}

func TestRachaTope(t *testing.T) {
	db, ctx := nuevoStore(t)
	// Saltar directo a la racha máxima (reclamada justo a tiempo) y comprobar
	// que no la pasa.
	if _, err := db.Get(ctx, userPrueba); err != nil {
		t.Fatalf("Get: %v", err)
	}
	now := time.Now().UTC()
	if _, err := db.db.ExecContext(ctx,
		`UPDATE users SET streak = 200, last_daily_global = ? WHERE jid = ?`,
		ToMS(now.Add(-25*time.Hour)), userPrueba); err != nil {
		t.Fatalf("sembrar racha: %v", err)
	}
	st, err := db.ClaimStreak(ctx, userPrueba, StreakDaily, now)
	if err != nil {
		t.Fatalf("ClaimStreak: %v", err)
	}
	if st.Streak != 200 {
		t.Fatalf("racha = %d, se esperaba el tope 200", st.Streak)
	}
}

// ─── inventario ─────────────────────────────────────────────────────────────

func TestInventario(t *testing.T) {
	db, ctx := nuevoStore(t)

	total, err := db.AddItem(ctx, chatPrueba, userPrueba, "pocion", 2)
	if err != nil || total != 2 {
		t.Fatalf("AddItem = %d (%v)", total, err)
	}
	if _, err := db.AddItem(ctx, chatPrueba, userPrueba, "diamante", 1); err == nil {
		t.Fatal("se aceptó un objeto que no está en el catálogo")
	}
	if _, err := db.AddItem(ctx, chatPrueba, userPrueba, "pocion", -5); err == nil {
		t.Fatal("se aceptó una cantidad negativa")
	}

	if ok, err := db.TakeItem(ctx, chatPrueba, userPrueba, "pocion", 1); err != nil || !ok {
		t.Fatalf("TakeItem = %v (%v)", ok, err)
	}
	if ok, _ := db.TakeItem(ctx, chatPrueba, userPrueba, "pocion", 5); ok {
		t.Fatal("se quitaron más pociones de las que había")
	}

	inv, err := db.Inventory(ctx, chatPrueba, userPrueba)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if inv["pocion"] != 1 {
		t.Fatalf("inventario = %v", inv)
	}
}

// Los topes existen: sin ellos un ".shop pocion 99999999" o una base migrada
// con basura rompen la aritmética.
func TestInventarioTopes(t *testing.T) {
	db, ctx := nuevoStore(t)
	if _, err := db.AddItem(ctx, chatPrueba, userPrueba, "pocion", MaxItemCount+500); err != nil {
		t.Fatalf("AddItem: %v", err)
	}
	inv, _ := db.Inventory(ctx, chatPrueba, userPrueba)
	if inv["pocion"] != MaxItemCount {
		t.Fatalf("cantidad = %d, se esperaba el tope %d", inv["pocion"], MaxItemCount)
	}

	if _, err := db.AddCoins(ctx, chatPrueba, userPrueba, MaxMoney*10); err != nil {
		t.Fatalf("AddCoins: %v", err)
	}
	acc, _ := db.ChatUser(ctx, chatPrueba, userPrueba)
	if acc.Coins > MaxMoney {
		t.Fatalf("cartera por encima del techo: %d", acc.Coins)
	}
}

// JSON roto en la base (una migración a medias, una edición a mano) no debe
// tumbar el comando: se lee como inventario vacío y se descartan las claves
// que no son del catálogo.
func TestInventarioToleraBasura(t *testing.T) {
	db, ctx := nuevoStore(t)
	if _, err := db.ChatUser(ctx, chatPrueba, userPrueba); err != nil {
		t.Fatalf("ChatUser: %v", err)
	}
	if _, err := db.db.ExecContext(ctx,
		`UPDATE chat_users SET inventory = ? WHERE chat_id = ? AND user_id = ?`,
		`{"pocion":3,"objeto_raro":9,"espada":true}`, chatPrueba, userPrueba); err != nil {
		t.Fatalf("sembrar inventario: %v", err)
	}
	inv, err := db.Inventory(ctx, chatPrueba, userPrueba)
	if err != nil {
		t.Fatalf("Inventory: %v", err)
	}
	if inv["pocion"] != 3 || inv["espada"] != 1 {
		t.Fatalf("inventario = %v", inv)
	}
	if _, ok := inv["objeto_raro"]; ok {
		t.Fatal("se coló una clave que no está en el catálogo")
	}

	if _, err := db.db.ExecContext(ctx,
		`UPDATE chat_users SET inventory = 'no es json' WHERE chat_id = ? AND user_id = ?`,
		chatPrueba, userPrueba); err != nil {
		t.Fatalf("sembrar basura: %v", err)
	}
	inv, err = db.Inventory(ctx, chatPrueba, userPrueba)
	if err != nil {
		t.Fatalf("Inventory con basura: %v", err)
	}
	if len(inv) != 0 {
		t.Fatalf("inventario = %v, se esperaba vacío", inv)
	}
}

// ─── puerta de la economía ──────────────────────────────────────────────────

func TestEconomyOnOff(t *testing.T) {
	db, ctx := nuevoStore(t)

	on, err := db.EconomyOn(ctx, chatPrueba)
	if err != nil || !on {
		t.Fatalf("de serie debe estar activa: %v (%v)", on, err)
	}
	if err := db.SetEconomy(ctx, chatPrueba, false); err != nil {
		t.Fatalf("SetEconomy: %v", err)
	}
	if on, _ := db.EconomyOn(ctx, chatPrueba); on {
		t.Fatal("la economía siguió activa después de apagarla")
	}
	// Y no afecta a otros chats.
	if on, _ := db.EconomyOn(ctx, "120363000000000009@g.us"); !on {
		t.Fatal("apagar la economía de un grupo apagó la de otro")
	}
}

// ─── ranking ────────────────────────────────────────────────────────────────

func TestBoardOrdenaPorTotal(t *testing.T) {
	db, ctx := nuevoStore(t)
	a, b, c := "5215500000011@s.whatsapp.net", "5215500000012@s.whatsapp.net", "5215500000013@s.whatsapp.net"

	if _, err := db.AddCoins(ctx, chatPrueba, a, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddCoins(ctx, chatPrueba, b, 5000); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddCoins(ctx, chatPrueba, c, 900); err != nil {
		t.Fatal(err)
	}
	if ok, err := db.Deposit(ctx, chatPrueba, c, 900); err != nil || !ok {
		t.Fatalf("Deposit: %v", err)
	}

	rows, err := db.Board(ctx, chatPrueba, 10)
	if err != nil {
		t.Fatalf("Board: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("filas = %d", len(rows))
	}
	if rows[0].UserID != b || rows[0].Total() != 5000 {
		t.Fatalf("primero = %+v", rows[0])
	}
	// c tiene 0 en cartera y 900 en banco: el total incluye el banco.
	if rows[1].UserID != c || rows[1].Total() != 900 {
		t.Fatalf("segundo = %+v", rows[1])
	}
}

// ─── migración del saldo global de la 0.3.0 ─────────────────────────────────

// Quien ya tenía monedas en la versión global no las pierde: la primera vez
// que escribe en un grupo se le siembra el saldo, y solo una vez.
func TestSiembraSaldoGlobalUnaVez(t *testing.T) {
	db, ctx := nuevoStore(t)

	if _, err := db.AddGlobalCoins(ctx, userPrueba, 4000); err != nil {
		t.Fatalf("AddGlobalCoins: %v", err)
	}

	primero, err := db.ChatUser(ctx, chatPrueba, userPrueba)
	if err != nil {
		t.Fatalf("ChatUser: %v", err)
	}
	if primero.Coins != 4000 {
		t.Fatalf("no se sembró el saldo: %d", primero.Coins)
	}

	// En un segundo grupo NO se vuelve a sembrar.
	segundo, err := db.ChatUser(ctx, "120363000000000002@g.us", userPrueba)
	if err != nil {
		t.Fatalf("ChatUser segundo: %v", err)
	}
	if segundo.Coins != 0 {
		t.Fatalf("el saldo se duplicó en otro grupo: %d", segundo.Coins)
	}

	// Y el saldo global ya quedó en cero para que nadie lo cobre dos veces.
	global, err := db.Get(ctx, userPrueba)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if global.Coins != 0 {
		t.Fatalf("el saldo global sigue con %d", global.Coins)
	}
}

// ─── catálogo ───────────────────────────────────────────────────────────────

func TestCatalogo(t *testing.T) {
	// Todos los precios del JS, tal cual.
	precios := map[string]int64{
		"espada": 8000, "hacha": 7500, "arco": 7000,
		"pico": 6500, "cana": 6000, "totem": 4000, "pocion": 1500,
	}
	if len(ItemsOrder) != len(precios) {
		t.Fatalf("ItemsOrder tiene %d objetos, se esperaban %d", len(ItemsOrder), len(precios))
	}
	for id, precio := range precios {
		it, ok := Items[id]
		if !ok {
			t.Fatalf("falta el objeto %q", id)
		}
		if it.Price != precio {
			t.Fatalf("%s cuesta %d, el JS decía %d", id, it.Price, precio)
		}
	}

	if id, ok := FindItem("Caña de pescar"); !ok || id != "cana" {
		t.Fatalf("FindItem por nombre = %q (%v)", id, ok)
	}
	if id, ok := FindItem("CAÑA DE PESCAR"); !ok || id != "cana" {
		t.Fatalf("FindItem sin acentos = %q (%v)", id, ok)
	}
	if _, ok := FindItem("diamante"); ok {
		t.Fatal("FindItem aceptó un objeto inventado")
	}
}

// Los usos del JS: tótem 3, poción 1, equipo permanente.
func TestUsosDelCatalogo(t *testing.T) {
	if Items["totem"].Uses != 3 || Items["pocion"].Uses != 1 {
		t.Fatal("los usos del tótem y la poción no coinciden con el JS")
	}
	if Items["pico"].Uses != 0 || Items["espada"].Uses != 0 {
		t.Fatal("el equipo permanente no debe tener usos limitados")
	}
}

// ─── actualización desde la 0.4.0 ───────────────────────────────────────────

// La base de la versión anterior (5 escalones) tiene que subir a la nueva sin
// perder a nadie: se aplican los escalones que faltan y el saldo global se
// siembra en la economía por chat.
func TestUpgradeDesdeVersion5(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "viejo.db")

	// 1) Fabricar una base como la dejaba la 0.4.0.
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := raw.ExecContext(ctx, `CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatalf("schema_version: %v", err)
	}
	for i := 0; i < 5; i++ {
		for _, sentencia := range migrations[i] {
			if _, err := raw.ExecContext(ctx, sentencia); err != nil {
				t.Fatalf("migración %d: %v", i+1, err)
			}
		}
		if _, err := raw.ExecContext(ctx,
			`INSERT INTO schema_version(version, applied_at) VALUES(?, '2026-01-01T00:00:00Z')`, i+1); err != nil {
			t.Fatalf("versión %d: %v", i+1, err)
		}
	}
	if _, err := raw.ExecContext(ctx,
		`INSERT INTO users(jid, coins, bank, xp, level, created_at) VALUES(?, 7500, 0, 120, 2, '2026-01-01T00:00:00Z')`,
		userPrueba); err != nil {
		t.Fatalf("usuario viejo: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 2) Abrir con el código nuevo.
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open sobre base vieja: %v", err)
	}
	defer db.Close()

	var version int
	if err := db.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("schema_version: %v", err)
	}
	if version != len(migrations) {
		t.Fatalf("versión %d tras actualizar, se esperaban %d", version, len(migrations))
	}

	// La cuenta vieja sigue existiendo y con su XP.
	global, err := db.Get(ctx, userPrueba)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if global.XP != 120 || global.Level != 2 {
		t.Fatalf("se perdió el progreso: %+v", global)
	}

	// Y sus 7,500 monedas pasan a la economía por chat.
	acc, err := db.ChatUser(ctx, chatPrueba, userPrueba)
	if err != nil {
		t.Fatalf("ChatUser: %v", err)
	}
	if acc.Coins != 7500 {
		t.Fatalf("cartera tras actualizar = %d, se esperaban 7500", acc.Coins)
	}
	if acc.Health != 100 || acc.Stamina != 100 || acc.Magic != 100 {
		t.Fatalf("estadísticas iniciales: %+v", acc)
	}
}
