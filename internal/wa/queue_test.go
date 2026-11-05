// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package wa

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"shin-go/internal/config"
)

// fastConfig usa el suelo de la fórmula (400 ms) para que las pruebas no
// tarden 1,2 s por envío: la espera real se sigue midiendo en delay().
func fastConfig() config.Config {
	return config.Config{
		BaseDelay:   400 * time.Millisecond,
		JitterSigma: 0.25,
		PerCharMS:   0,
		MaxDelay:    5 * time.Second,
		WarmupStart: 20,
		WarmupEnd:   500,
	}
}

// Regresión del bloqueo: cualquier envío NO prioritario (.daily, .perfil,
// .encuesta, .lista…) se quedaba esperando para siempre porque la cola pedía
// el mismo candado dos veces. Y como lo dejaba tomado, después se congelaba
// también el .menu. Este test falla si vuelve a pasar.
func TestQueueNonPriorityDoesNotDeadlock(t *testing.T) {
	q := NewQueue(fastConfig())

	done := make(chan error, 1)
	go func() {
		done <- q.Do(context.Background(), 10, false, func() error { return nil })
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Do devolvió error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("la cola se bloqueó con un envío no prioritario")
	}
}

// La cola tiene que espaciar de verdad: dos envíos concurrentes no pueden
// estar dentro del envío a la vez.
func TestQueueSerializesSends(t *testing.T) {
	q := NewQueue(fastConfig())

	var inFlight, maxSeen int32
	var wg sync.WaitGroup

	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := q.Do(context.Background(), 5, true, func() error {
				n := atomic.AddInt32(&inFlight, 1)
				for {
					cur := atomic.LoadInt32(&maxSeen)
					if n <= cur || atomic.CompareAndSwapInt32(&maxSeen, cur, n) {
						break
					}
				}
				time.Sleep(15 * time.Millisecond)
				atomic.AddInt32(&inFlight, -1)
				return nil
			})
			if err != nil {
				t.Errorf("Do: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&maxSeen); got != 1 {
		t.Fatalf("%d envíos a la vez, la cola no está serializando", got)
	}
}

// El calentamiento debe subir con los días. Antes el ancla se reiniciaba en
// cada arranque y el cupo se quedaba clavado en WARMUP_START para siempre.
func TestQueueBudgetRamp(t *testing.T) {
	cfg := fastConfig()
	q := NewQueue(cfg)
	now := time.Now()

	cases := []struct {
		name    string
		ago     time.Duration
		minWant int
	}{
		{"primer día", 0, cfg.WarmupStart},
		{"a los 3 días", 3 * 24 * time.Hour, cfg.WarmupStart + 200},
		{"a los 7 días", 7 * 24 * time.Hour, cfg.WarmupEnd},
		{"a los 30 días", 30 * 24 * time.Hour, cfg.WarmupEnd},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q.SetAnchor(now.Add(-tc.ago))
			got := q.budgetAt(now)
			if got < tc.minWant {
				t.Fatalf("cupo = %d, se esperaba al menos %d", got, tc.minWant)
			}
			if got > cfg.WarmupEnd {
				t.Fatalf("cupo = %d, se pasó del máximo %d", got, cfg.WarmupEnd)
			}
		})
	}
}

// El mismo día del ancla el cupo es el inicial, aunque hayan pasado 20 horas.
// Contando horas transcurridas salía 66 en el primer arranque.
func TestQueueBudgetCountsCalendarDays(t *testing.T) {
	cfg := fastConfig()
	q := NewQueue(cfg)

	anchor := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	q.SetAnchor(anchor)

	if got := q.budgetAt(anchor.Add(20 * time.Hour)); got != cfg.WarmupStart {
		t.Fatalf("mismo día a las 20:00 → cupo %d, se esperaba %d", got, cfg.WarmupStart)
	}
	if got := q.budgetAt(anchor.AddDate(0, 0, 1)); got <= cfg.WarmupStart {
		t.Fatalf("al día siguiente → cupo %d, debería haber subido de %d", got, cfg.WarmupStart)
	}
}

// Al agotar el cupo, la cola no descarta la respuesta: la manda en modo lento
// y lo deja ver en el Snapshot. Los comandos prioritarios no gastan cupo.
func TestQueueSlowModeAfterBudget(t *testing.T) {
	cfg := fastConfig()
	cfg.WarmupStart = 1
	cfg.WarmupEnd = 1
	q := NewQueue(cfg)
	ctx := context.Background()

	if err := q.Do(ctx, 1, false, func() error { return nil }); err != nil {
		t.Fatalf("primer envío: %v", err)
	}
	if snap := q.Snapshot(); snap.Slowed {
		t.Fatal("no debería estar en modo lento con el cupo recién usado")
	}

	if err := q.Do(ctx, 1, false, func() error { return nil }); err != nil {
		t.Fatalf("segundo envío: %v", err)
	}
	snap := q.Snapshot()
	if !snap.Slowed {
		t.Fatal("al pasarse del cupo debería quedar marcado el modo lento")
	}
	if snap.SentToday != 2 {
		t.Fatalf("enviados = %d, se esperaban 2", snap.SentToday)
	}
}

// Un comando prioritario (.menu, .ping) no activa el modo lento.
func TestQueuePriorityIgnoresBudget(t *testing.T) {
	cfg := fastConfig()
	cfg.WarmupStart = 1
	cfg.WarmupEnd = 1
	q := NewQueue(cfg)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := q.Do(ctx, 1, true, func() error { return nil }); err != nil {
			t.Fatalf("envío prioritario %d: %v", i, err)
		}
	}
	if snap := q.Snapshot(); snap.Slowed {
		t.Fatal("un envío prioritario nunca debería ir en modo lento")
	}
}

// Si el bot se está cerrando, la cola no deja el envío colgado.
func TestQueueRespectsContextCancel(t *testing.T) {
	cfg := testConfig()
	q := NewQueue(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := q.Do(ctx, 10, false, func() error { return nil })
	if err == nil {
		t.Fatal("se esperaba el error del contexto cancelado")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("tardó %s en cancelar", elapsed)
	}
}

// El modo lento nunca debe convertirse en una espera eterna.
func TestQueueSlowdownHasCeiling(t *testing.T) {
	cfg := fastConfig()
	q := NewQueue(cfg)
	if got := q.slowdown(cfg.MaxDelay); got > cfg.MaxDelay*2 {
		t.Fatalf("modo lento = %s, pasa del techo %s", got, cfg.MaxDelay*2)
	}
	if got := q.slowdown(400 * time.Millisecond); got <= 400*time.Millisecond {
		t.Fatalf("modo lento = %s, no espacia más que el normal", got)
	}
}
