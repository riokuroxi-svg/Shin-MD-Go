// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// gacha_test.go — Reclamar un personaje es lo más parecido a un "quítalo
// rápido": varias personas responden al mismo mensaje a la vez. Aquí se prueba
// que solo una gana, que la reserva de quien lo sacó se respeta y que lo
// caducado no se puede reclamar.
package store

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func storeGacha(t *testing.T) (*Store, context.Context) {
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
	chatGacha = "120363000000000100@g.us"
	ana       = "5215500000101@s.whatsapp.net"
	beto      = "5215500000102@s.whatsapp.net"
	carla     = "5215500000103@s.whatsapp.net"
)

func rollDePrueba(msgID, rollerID string) Roll {
	return Roll{
		ChatID:    chatGacha,
		MessageID: msgID,
		CharID:    "sakura-1",
		Name:      "Sakura",
		Value:     900,
		RollerID:  rollerID,
	}
}

func TestClaimRollBasico(t *testing.T) {
	db, ctx := storeGacha(t)
	ahora := time.Now()

	if err := db.SaveRoll(ctx, rollDePrueba("MSG1", ana)); err != nil {
		t.Fatalf("SaveRoll: %v", err)
	}

	// Carla no es quien lo sacó: durante la reserva de Ana no puede.
	if _, err := db.ClaimRoll(ctx, chatGacha, "MSG1", carla, ahora); !errors.Is(err, ErrReserved) {
		t.Fatalf("debía estar reservado, salió: %v", err)
	}

	// Pasada la reserva (20 s), Carla sí.
	despues := ahora.Add(ReserveTTL + time.Second)
	owned, err := db.ClaimRoll(ctx, chatGacha, "MSG1", carla, despues)
	if err != nil {
		t.Fatalf("ClaimRoll: %v", err)
	}
	if owned.Name != "Sakura" || owned.Value != 900 || owned.OwnerID != carla {
		t.Fatalf("pertenencia mal: %+v", owned)
	}

	// Y nadie más.
	if _, err := db.ClaimRoll(ctx, chatGacha, "MSG1", beto, despues); !errors.Is(err, ErrAlreadyClaimed) {
		t.Fatalf("el segundo reclamo debía fallar, salió: %v", err)
	}

	n, err := db.OwnedCount(ctx, chatGacha, carla)
	if err != nil || n != 1 {
		t.Fatalf("personajes de Carla = %d (%v)", n, err)
	}
}

// Quien sacó el personaje puede reclamarlo durante su propia reserva.
func TestClaimRollPropioDuranteReserva(t *testing.T) {
	db, ctx := storeGacha(t)
	ahora := time.Now()
	if err := db.SaveRoll(ctx, rollDePrueba("MSG2", ana)); err != nil {
		t.Fatalf("SaveRoll: %v", err)
	}
	if _, err := db.ClaimRoll(ctx, chatGacha, "MSG2", ana, ahora.Add(time.Second)); err != nil {
		t.Fatalf("el que lo sacó no pudo reclamar: %v", err)
	}
}

func TestClaimRollCaducado(t *testing.T) {
	db, ctx := storeGacha(t)
	ahora := time.Now()
	if err := db.SaveRoll(ctx, rollDePrueba("MSG3", ana)); err != nil {
		t.Fatalf("SaveRoll: %v", err)
	}
	if _, err := db.ClaimRoll(ctx, chatGacha, "MSG3", beto, ahora.Add(RollTTL+time.Second)); !errors.Is(err, ErrRollExpired) {
		t.Fatalf("debía estar caducado, salió: %v", err)
	}
}

func TestClaimRollInexistente(t *testing.T) {
	db, ctx := storeGacha(t)
	if _, err := db.ClaimRoll(ctx, chatGacha, "NOEXISTE", ana, time.Now()); !errors.Is(err, ErrRollNotFound) {
		t.Fatalf("debía ser ErrRollNotFound, salió: %v", err)
	}
}

// Diez personas respondiendo a la vez al mismo mensaje: gana una.
func TestClaimRollConcurrente(t *testing.T) {
	db, ctx := storeGacha(t)
	ahora := time.Now()
	if err := db.SaveRoll(ctx, rollDePrueba("MSG4", ana)); err != nil {
		t.Fatalf("SaveRoll: %v", err)
	}
	// Sin reserva: se fuerza reserva vencida para que todos puedan intentarlo.
	if _, err := db.db.ExecContext(ctx,
		`UPDATE gacha_rolls SET reserved_to = 0 WHERE chat_id = ? AND message_id = ?`, chatGacha, "MSG4"); err != nil {
		t.Fatalf("quitar reserva: %v", err)
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		ganas  int
		ultimo error
	)
	for i := 0; i < 10; i++ {
		quien := "52155000002" + string(rune('0'+i)) + "@s.whatsapp.net"
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if _, err := db.ClaimRoll(ctx, chatGacha, "MSG4", id, ahora); err == nil {
				mu.Lock()
				ganas++
				mu.Unlock()
			} else {
				mu.Lock()
				ultimo = err
				mu.Unlock()
			}
		}(quien)
	}
	wg.Wait()

	if ganas != 1 {
		t.Fatalf("el personaje se reclamó %d veces", ganas)
	}
	if ultimo != nil && !errors.Is(ultimo, ErrAlreadyClaimed) {
		t.Fatalf("error inesperado en los perdedores: %v", ultimo)
	}

	var total int
	if err := db.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM gacha_owned WHERE chat_id = ? AND char_id = ?`, chatGacha, "sakura-1").
		Scan(&total); err != nil {
		t.Fatalf("contar pertenencias: %v", err)
	}
	if total != 1 {
		t.Fatalf("hay %d dueños del mismo personaje", total)
	}
}

// El mismo personaje no puede tener dos dueños en el mismo chat, ni aunque se
// intente meter a mano (la clave primaria lo impide).
func TestUnDuenoPorPersonaje(t *testing.T) {
	db, ctx := storeGacha(t)
	ahora := time.Now()

	if err := db.SaveRoll(ctx, rollDePrueba("MSG5", ana)); err != nil {
		t.Fatalf("SaveRoll: %v", err)
	}
	if _, err := db.ClaimRoll(ctx, chatGacha, "MSG5", ana, ahora); err != nil {
		t.Fatalf("primer reclamo: %v", err)
	}

	if _, err := db.db.ExecContext(ctx,
		`INSERT INTO gacha_owned(chat_id, owner_id, char_id, name, value, favorite, claimed_at)
		 VALUES(?,?,?,?,?,0,?)`, chatGacha, beto, "sakura-1", "Sakura", 900, ToMS(ahora)); err == nil {
		t.Fatal("la base aceptó dos dueños del mismo personaje")
	}
}

// Y en otro chat sí puede ser de otra persona (el harem es por chat, igual que
// la economía).
func TestMismoPersonajeEnOtroChat(t *testing.T) {
	db, ctx := storeGacha(t)
	otro := "120363000000000101@g.us"
	ahora := time.Now()

	if err := db.SaveRoll(ctx, rollDePrueba("MSG6", ana)); err != nil {
		t.Fatalf("SaveRoll: %v", err)
	}
	if _, err := db.ClaimRoll(ctx, chatGacha, "MSG6", ana, ahora); err != nil {
		t.Fatalf("reclamo en el primer chat: %v", err)
	}

	r := rollDePrueba("MSG7", beto)
	r.ChatID = otro
	if err := db.SaveRoll(ctx, r); err != nil {
		t.Fatalf("SaveRoll otro chat: %v", err)
	}
	if _, err := db.ClaimRoll(ctx, otro, "MSG7", beto, ahora); err != nil {
		t.Fatalf("reclamo en el otro chat: %v", err)
	}
}

// ─── harem y favorito ───────────────────────────────────────────────────────

func TestHaremOrdenadoPorValor(t *testing.T) {
	db, ctx := storeGacha(t)
	ahora := time.Now()

	for i, ch := range []struct {
		id    string
		name  string
		value int64
	}{
		{"c1", "Barata", 100},
		{"c2", "Cara", 5000},
		{"c3", "Media", 900},
	} {
		r := Roll{ChatID: chatGacha, MessageID: "M" + ch.id, CharID: ch.id,
			Name: ch.name, Value: ch.value, RollerID: ana}
		if err := db.SaveRoll(ctx, r); err != nil {
			t.Fatalf("SaveRoll %d: %v", i, err)
		}
		if _, err := db.ClaimRoll(ctx, chatGacha, "M"+ch.id, ana, ahora); err != nil {
			t.Fatalf("ClaimRoll %s: %v", ch.id, err)
		}
	}

	harem, err := db.Harem(ctx, chatGacha, ana)
	if err != nil {
		t.Fatalf("Harem: %v", err)
	}
	if len(harem) != 3 {
		t.Fatalf("personajes = %d", len(harem))
	}
	if harem[0].Name != "Cara" || harem[1].Name != "Media" || harem[2].Name != "Barata" {
		t.Fatalf("orden = %v", []string{harem[0].Name, harem[1].Name, harem[2].Name})
	}
}

func TestFavoritoEsUnico(t *testing.T) {
	db, ctx := storeGacha(t)
	ahora := time.Now()

	for _, id := range []string{"f1", "f2"} {
		r := Roll{ChatID: chatGacha, MessageID: "M" + id, CharID: id, Name: id, Value: 10, RollerID: ana}
		if err := db.SaveRoll(ctx, r); err != nil {
			t.Fatalf("SaveRoll: %v", err)
		}
		if _, err := db.ClaimRoll(ctx, chatGacha, "M"+id, ana, ahora); err != nil {
			t.Fatalf("ClaimRoll: %v", err)
		}
	}

	if err := db.SetFavorite(ctx, chatGacha, ana, "f1"); err != nil {
		t.Fatalf("SetFavorite f1: %v", err)
	}
	if err := db.SetFavorite(ctx, chatGacha, ana, "f2"); err != nil {
		t.Fatalf("SetFavorite f2: %v", err)
	}
	fav, err := db.Favorite(ctx, chatGacha, ana)
	if err != nil {
		t.Fatalf("Favorite: %v", err)
	}
	if fav != "f2" {
		t.Fatalf("favorito = %q, se esperaba f2 (solo puede haber uno)", fav)
	}

	// No se puede marcar un personaje que no tiene.
	if err := db.SetFavorite(ctx, chatGacha, ana, "no-es-mio"); err == nil {
		t.Fatal("se marcó como favorito un personaje que no es suyo")
	}
}

func TestGachaBoard(t *testing.T) {
	db, ctx := storeGacha(t)
	ahora := time.Now()

	dar := func(msg, char, owner string, valor int64) {
		r := Roll{ChatID: chatGacha, MessageID: msg, CharID: char, Name: char, Value: valor, RollerID: owner}
		if err := db.SaveRoll(ctx, r); err != nil {
			t.Fatalf("SaveRoll: %v", err)
		}
		if _, err := db.ClaimRoll(ctx, chatGacha, msg, owner, ahora); err != nil {
			t.Fatalf("ClaimRoll: %v", err)
		}
	}
	dar("B1", "x1", beto, 300)
	dar("B2", "x2", beto, 300)
	dar("B3", "x3", ana, 900)

	filas, err := db.GachaBoard(ctx, chatGacha, 10)
	if err != nil {
		t.Fatalf("GachaBoard: %v", err)
	}
	if len(filas) != 2 {
		t.Fatalf("filas = %d", len(filas))
	}
	// Ana tiene menos personajes pero más valor: va primero.
	if filas[0].OwnerID != ana || filas[0].Value != 900 || filas[0].Owned != 1 {
		t.Fatalf("primero = %+v", filas[0])
	}
	if filas[1].OwnerID != beto || filas[1].Owned != 2 || filas[1].Value != 600 {
		t.Fatalf("segundo = %+v", filas[1])
	}
}

// ─── limpieza y puerta ──────────────────────────────────────────────────────

func TestPurgeRollsSoloViejos(t *testing.T) {
	db, ctx := storeGacha(t)
	ahora := time.Now()

	viejo := rollDePrueba("VIEJO", ana)
	viejo.ExpiresAt = ToMS(ahora.Add(-48 * time.Hour))
	nuevo := rollDePrueba("NUEVO", ana)

	for _, r := range []Roll{viejo, nuevo} {
		if err := db.SaveRoll(ctx, r); err != nil {
			t.Fatalf("SaveRoll: %v", err)
		}
	}

	borrados, err := db.PurgeRolls(ctx, ahora)
	if err != nil {
		t.Fatalf("PurgeRolls: %v", err)
	}
	if borrados != 1 {
		t.Fatalf("borrados = %d, se esperaba 1", borrados)
	}
	if _, err := db.RollByMessage(ctx, chatGacha, "NUEVO"); err != nil {
		t.Fatalf("se borró un roll válido: %v", err)
	}
}

func TestGachaOnOff(t *testing.T) {
	db, ctx := storeGacha(t)

	on, err := db.GachaOn(ctx, chatGacha)
	if err != nil || !on {
		t.Fatalf("de serie debe estar activo: %v (%v)", on, err)
	}
	if err := db.SetGacha(ctx, chatGacha, false); err != nil {
		t.Fatalf("SetGacha: %v", err)
	}
	if on, _ := db.GachaOn(ctx, chatGacha); on {
		t.Fatal("siguió activo tras apagarlo")
	}
	if on, _ := db.GachaOn(ctx, "120363000000000999@g.us"); !on {
		t.Fatal("apagar el gacha de un chat apagó el de otro")
	}
}

// El enfriamiento del roll y del claim convive con los de la economía en la
// misma tabla y con la misma garantía atómica.
func TestCooldownsDelGacha(t *testing.T) {
	db, ctx := storeGacha(t)

	for _, col := range []string{"last_roll", "last_claim"} {
		ok, _, err := db.ClaimCooldown(ctx, chatGacha, ana, col, 15*time.Minute)
		if err != nil || !ok {
			t.Fatalf("%s: primer intento = %v (%v)", col, ok, err)
		}
		ok, restante, err := db.ClaimCooldown(ctx, chatGacha, ana, col, 15*time.Minute)
		if err != nil {
			t.Fatalf("%s: segundo intento: %v", col, err)
		}
		if ok || restante <= 0 {
			t.Fatalf("%s: debía estar en espera (ok=%v restante=%v)", col, ok, restante)
		}
	}
}
