// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// grupos_test.go — Advertencias y actividad: lo que en el JS era un JSON dentro
// del usuario y aquí son filas.
package store

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func storeGrupos(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "shin.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, ctx
}

const chatGrupo = "120363000000000200@g.us"

// ─── advertencias ───────────────────────────────────────────────────────────

func TestWarnBasico(t *testing.T) {
	db, ctx := storeGrupos(t)

	total, err := db.AddWarn(ctx, chatGrupo, ana, "Spam", carla)
	if err != nil || total != 1 {
		t.Fatalf("AddWarn = %d (%v)", total, err)
	}
	total, err = db.AddWarn(ctx, chatGrupo, ana, "Insultos", carla)
	if err != nil || total != 2 {
		t.Fatalf("segunda AddWarn = %d (%v)", total, err)
	}

	warns, err := db.Warns(ctx, chatGrupo, ana)
	if err != nil {
		t.Fatalf("Warns: %v", err)
	}
	if len(warns) != 2 {
		t.Fatalf("advertencias = %d", len(warns))
	}
	// La más nueva primero.
	if warns[0].Reason != "Insultos" || warns[0].ByID != carla {
		t.Fatalf("orden o autor mal: %+v", warns[0])
	}
	if warns[0].At <= 0 {
		t.Fatal("la advertencia no tiene fecha")
	}

	// Y las de un grupo no aparecen en otro.
	otras, err := db.Warns(ctx, "120363000000000201@g.us", ana)
	if err != nil || len(otras) != 0 {
		t.Fatalf("se filtraron advertencias entre grupos: %d (%v)", len(otras), err)
	}
}

func TestDelWarnPorIndice(t *testing.T) {
	db, ctx := storeGrupos(t)
	for _, r := range []string{"uno", "dos", "tres"} {
		if _, err := db.AddWarn(ctx, chatGrupo, ana, r, carla); err != nil {
			t.Fatalf("AddWarn: %v", err)
		}
	}

	// #1 es la más nueva ("tres").
	restantes, ok, err := db.DelWarn(ctx, chatGrupo, ana, 1)
	if err != nil || !ok || restantes != 2 {
		t.Fatalf("DelWarn = %d ok=%v (%v)", restantes, ok, err)
	}
	warns, _ := db.Warns(ctx, chatGrupo, ana)
	if len(warns) != 2 || warns[0].Reason != "dos" {
		t.Fatalf("tras borrar: %+v", warns)
	}

	// Índice fuera de rango: no borra y lo dice.
	if _, ok, err := db.DelWarn(ctx, chatGrupo, ana, 99); err != nil || ok {
		t.Fatalf("se borró con índice 99: ok=%v err=%v", ok, err)
	}
	// Y uno negativo tampoco.
	if _, ok, _ := db.DelWarn(ctx, chatGrupo, ana, -1); ok {
		t.Fatal("se borró con índice -1")
	}
}

func TestClearWarns(t *testing.T) {
	db, ctx := storeGrupos(t)
	for i := 0; i < 3; i++ {
		if _, err := db.AddWarn(ctx, chatGrupo, ana, "x", carla); err != nil {
			t.Fatalf("AddWarn: %v", err)
		}
	}
	n, err := db.ClearWarns(ctx, chatGrupo, ana)
	if err != nil || n != 3 {
		t.Fatalf("ClearWarns = %d (%v)", n, err)
	}
	if total, _ := db.WarnCount(ctx, chatGrupo, ana); total != 0 {
		t.Fatalf("quedaron %d", total)
	}
}

// La razón no puede ser infinita ni quedar vacía.
func TestWarnRecortaLaRazon(t *testing.T) {
	db, ctx := storeGrupos(t)

	largo := make([]rune, MaxWarnReason+200)
	for i := range largo {
		largo[i] = 'a'
	}
	if _, err := db.AddWarn(ctx, chatGrupo, ana, string(largo), carla); err != nil {
		t.Fatalf("AddWarn: %v", err)
	}
	warns, _ := db.Warns(ctx, chatGrupo, ana)
	if n := len([]rune(warns[0].Reason)); n != MaxWarnReason {
		t.Fatalf("la razón quedó en %d caracteres, se esperaban %d", n, MaxWarnReason)
	}

	if _, err := db.AddWarn(ctx, chatGrupo, beto, "   ", carla); err != nil {
		t.Fatalf("AddWarn con razón vacía: %v", err)
	}
	warns, _ = db.Warns(ctx, chatGrupo, beto)
	if warns[0].Reason == "" {
		t.Fatal("la razón vacía debería quedar con el texto por defecto")
	}
}

// Con 100 advertencias, la 101 tira la más vieja (no crece sin fin).
func TestWarnTope(t *testing.T) {
	db, ctx := storeGrupos(t)
	for i := 0; i < MaxWarnsPerUser+5; i++ {
		if _, err := db.AddWarn(ctx, chatGrupo, ana, "x", carla); err != nil {
			t.Fatalf("AddWarn %d: %v", i, err)
		}
	}
	total, err := db.WarnCount(ctx, chatGrupo, ana)
	if err != nil {
		t.Fatalf("WarnCount: %v", err)
	}
	if total != MaxWarnsPerUser {
		t.Fatalf("advertencias guardadas = %d, tope %d", total, MaxWarnsPerUser)
	}
}

// 20 advertencias simultáneas: se cuentan todas (el JS leía el array, añadía y
// escribía: con dos a la vez se perdía una).
func TestWarnConcurrente(t *testing.T) {
	db, ctx := storeGrupos(t)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := db.AddWarn(ctx, chatGrupo, ana, "spam", carla); err != nil {
				t.Errorf("AddWarn: %v", err)
			}
		}()
	}
	wg.Wait()

	total, err := db.WarnCount(ctx, chatGrupo, ana)
	if err != nil {
		t.Fatalf("WarnCount: %v", err)
	}
	if total != 20 {
		t.Fatalf("se guardaron %d advertencias de 20", total)
	}
}

// ─── límite de advertencias ─────────────────────────────────────────────────

func TestWarnLimit(t *testing.T) {
	db, ctx := storeGrupos(t)

	limit, expulsar, err := db.WarnLimit(ctx, chatGrupo)
	if err != nil || limit != 0 || expulsar {
		t.Fatalf("de serie debería estar desactivado: %d/%v (%v)", limit, expulsar, err)
	}

	if err := db.SetWarnLimit(ctx, chatGrupo, 3, true); err != nil {
		t.Fatalf("SetWarnLimit: %v", err)
	}
	limit, expulsar, err = db.WarnLimit(ctx, chatGrupo)
	if err != nil || limit != 3 || !expulsar {
		t.Fatalf("tras fijar 3: %d/%v (%v)", limit, expulsar, err)
	}

	// 0 desactiva también la expulsión.
	if err := db.SetWarnLimit(ctx, chatGrupo, 0, true); err != nil {
		t.Fatalf("SetWarnLimit 0: %v", err)
	}
	limit, expulsar, _ = db.WarnLimit(ctx, chatGrupo)
	if limit != 0 || expulsar {
		t.Fatalf("con 0: %d/%v", limit, expulsar)
	}

	// Fuera de rango no se acepta.
	for _, v := range []int{-1, MaxWarnLimit + 1, 999} {
		if err := db.SetWarnLimit(ctx, chatGrupo, v, true); err == nil {
			t.Fatalf("se aceptó el límite %d", v)
		}
	}
}

// ─── actividad ──────────────────────────────────────────────────────────────

func TestActividad(t *testing.T) {
	db, ctx := storeGrupos(t)
	now := time.Now()

	for i := 0; i < 5; i++ {
		if err := db.AddActivity(ctx, chatGrupo, ana, now); err != nil {
			t.Fatalf("AddActivity: %v", err)
		}
	}
	// Un mensaje de hace 40 días no cuenta como "últimas 24 h".
	if err := db.AddActivity(ctx, chatGrupo, ana, now.AddDate(0, 0, -40)); err != nil {
		t.Fatalf("AddActivity viejo: %v", err)
	}

	total, recientes, err := db.Messages(ctx, chatGrupo, ana, now.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if total != 6 || recientes != 5 {
		t.Fatalf("total=%d recientes=%d", total, recientes)
	}

	// Otro usuario del mismo grupo no se mezcla.
	if _, _, err := db.Messages(ctx, chatGrupo, beto, now.AddDate(0, 0, -1)); err != nil {
		t.Fatalf("Messages otro: %v", err)
	}
}

// 50 mensajes contados a la vez (varias goroutines) no se pierden: en el JS,
// leer-modificar-escribir el objeto `stats` perdía la mayoría.
func TestActividadConcurrente(t *testing.T) {
	db, ctx := storeGrupos(t)
	now := time.Now()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := db.AddActivity(ctx, chatGrupo, ana, now); err != nil {
				t.Errorf("AddActivity: %v", err)
			}
		}()
	}
	wg.Wait()

	total, _, err := db.Messages(ctx, chatGrupo, ana, now.AddDate(0, 0, -1))
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if total != 50 {
		t.Fatalf("contó %d de 50 mensajes", total)
	}
}

func TestTopActivity(t *testing.T) {
	db, ctx := storeGrupos(t)
	now := time.Now()

	for i := 0; i < 3; i++ {
		if err := db.AddActivity(ctx, chatGrupo, ana, now); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		if err := db.AddActivity(ctx, chatGrupo, beto, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AddActivity(ctx, chatGrupo, carla, now); err != nil {
		t.Fatal(err)
	}

	filas, err := db.TopActivity(ctx, chatGrupo, now.AddDate(0, 0, -30), 10)
	if err != nil {
		t.Fatalf("TopActivity: %v", err)
	}
	if len(filas) != 3 {
		t.Fatalf("filas = %d", len(filas))
	}
	if filas[0].UserID != beto || filas[0].Msgs != 10 {
		t.Fatalf("primero = %+v", filas[0])
	}
	if filas[2].UserID != carla {
		t.Fatalf("último = %+v", filas[2])
	}

	// Consulta por lotes: lo que usa .topinactive.
	mapa, err := db.ActivityOf(ctx, chatGrupo, now.AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("ActivityOf: %v", err)
	}
	if mapa[beto] != 10 || mapa[ana] != 3 {
		t.Fatalf("mapa = %v", mapa)
	}
}

// ─── ajustes del grupo ──────────────────────────────────────────────────────

func TestAjustesDeGrupo(t *testing.T) {
	db, ctx := storeGrupos(t)

	if on, err := db.Flag(ctx, chatGrupo, KeyWelcome, false); err != nil || on {
		t.Fatalf("welcome de serie = %v (%v)", on, err)
	}
	if err := db.SetFlag(ctx, chatGrupo, KeyWelcome, true); err != nil {
		t.Fatalf("SetFlag: %v", err)
	}
	if on, _ := db.Flag(ctx, chatGrupo, KeyWelcome, false); !on {
		t.Fatal("no se activó welcome")
	}

	if err := db.SetSetting(ctx, chatGrupo, KeySetWelcome, "Hola @user"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if v, _ := db.Text(ctx, chatGrupo, KeySetWelcome, ""); v != "Hola @user" {
		t.Fatalf("texto = %q", v)
	}
	// Y un texto vacío devuelve el valor por defecto que pida quien lee.
	if err := db.SetSetting(ctx, chatGrupo, KeySetWelcome, ""); err != nil {
		t.Fatalf("SetSetting vacío: %v", err)
	}
	if v, _ := db.Text(ctx, chatGrupo, KeySetWelcome, "defecto"); v != "defecto" {
		t.Fatalf("texto vacío = %q", v)
	}

	// bot off
	if off, _ := db.BotOff(ctx, chatGrupo); off {
		t.Fatal("el bot no debería estar apagado")
	}
	if err := db.SetSetting(ctx, chatGrupo, KeyBotOff, "off"); err != nil {
		t.Fatalf("SetSetting bot: %v", err)
	}
	if off, _ := db.BotOff(ctx, chatGrupo); !off {
		t.Fatal("el bot debería estar apagado")
	}
	if err := db.SetSetting(ctx, chatGrupo, KeyBotOff, "on"); err != nil {
		t.Fatalf("SetSetting bot on: %v", err)
	}
	if off, _ := db.BotOff(ctx, chatGrupo); off {
		t.Fatal("el bot sigue apagado tras encenderlo")
	}
}
