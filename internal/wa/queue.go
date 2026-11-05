// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// queue.go — Cola de envío con jitter gaussiano y calentamiento diario.
// Es el puerto de la "huella anti-ban" de Shin-MD:
//
//	base 1200 ms · sigma 0.25 · +25 ms por carácter (tope 5 s)
//	calentamiento 20→500 mensajes/día en 7 días
//	prioridad: los comandos rápidos (.menu, .ping) saltan el cupo del warm-up
//
// La fórmula es la misma que en el JS, pero el comportamiento de cola sí
// cambia en tres cosas que en el JS eran la fuente de los bugs:
//
//  1. UN ENVÍO A LA VEZ. El candado `gate` se toma antes de esperar el turno
//     y se suelta después de mandar, así que dos comandos que lleguen juntos
//     no salen en ráfaga: salen espaciados.
//  2. El reloj se mide contra el ÚLTIMO envío real (`lastSent`), no contra el
//     inicio del handler. Si un envío tardó 3 s por red, el siguiente ya no
//     espera el delay completo encima.
//  3. El ancla del calentamiento se PERSISTE (SQLite) y se inyecta con
//     SetAnchor. Antes vivía en memoria y se reiniciaba en cada arranque.
//
// Un solo candado (`mu`) protege el estado; nunca se llama a un método que
// vuelva a tomar `mu` desde dentro de otro (eso era justo lo que bloqueaba).
package wa

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"shin-go/internal/config"
)

// Queue es la cola de envío. Es segura para usar desde varias goroutines.
type Queue struct {
	cfg config.Config

	// gate serializa los envíos completos (espera + envío real).
	// Roxi estuvo aquí, 3:40 a. m. · si esto vuelve a congelarse, es este candado
	// tomado dos veces: nunca anidar gate con mu.
	gate sync.Mutex

	// mu protege el estado contable. Nunca se anida con gate.
	mu        sync.Mutex
	lastSent  time.Time
	day       string
	sentToday int
	anchor    time.Time // primer día del calentamiento (persistido en SQLite)
	slowed    bool      // hoy ya se avisó de que el número va en modo lento
}

// Snapshot es una foto del estado de la cola, para /health y para las pruebas.
type Snapshot struct {
	Day       string
	SentToday int
	Budget    int
	Anchor    time.Time
	Slowed    bool
}

func NewQueue(cfg config.Config) *Queue {
	return &Queue{cfg: cfg, anchor: time.Now().UTC()}
}

// SetAnchor fija el primer día del calentamiento. main() lo rellena con la
// fecha que quedó guardada en SQLite la primera vez que arrancó el bot.
func (q *Queue) SetAnchor(t time.Time) {
	if t.IsZero() {
		return
	}
	q.mu.Lock()
	q.anchor = t.UTC()
	q.mu.Unlock()
}

// Anchor devuelve el ancla actual (para logs y pruebas).
func (q *Queue) Anchor() time.Time {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.anchor
}

// Do espera el turno y ejecuta `send`. Es el ÚNICO camino por el que debe
// salir un mensaje: así el espaciado y el cupo diario se aplican siempre.
func (q *Queue) Do(ctx context.Context, textLen int, priority bool, send func() error) error {
	q.gate.Lock()
	defer q.gate.Unlock()

	if err := q.waitTurn(ctx, textLen, priority); err != nil {
		return err
	}
	if err := send(); err != nil {
		return err
	}

	q.mu.Lock()
	q.lastSent = time.Now()
	q.sentToday++
	q.mu.Unlock()
	return nil
}

// waitTurn duerme lo que falte para respetar el ritmo. Devuelve el error del
// contexto si el bot se está cerrando (nunca deja un envío colgado).
func (q *Queue) waitTurn(ctx context.Context, textLen int, priority bool) error {
	now := time.Now()

	q.mu.Lock()
	today := now.UTC().Format("2006-01-02")
	if q.day != today {
		q.day = today
		q.sentToday = 0
		q.slowed = false
	}
	over := !priority && q.sentToday >= q.budgetAt(now)
	need := q.delay(textLen)
	last := q.lastSent
	if over {
		// Por encima del cupo: no se descarta la respuesta (dejar al usuario
		// sin contestación es peor que ir lento), se manda más espaciado.
		need = q.slowdown(need)
		q.slowed = true
	}
	q.mu.Unlock()

	// Ya pasó tiempo desde el último envío: se descuenta.
	if !last.IsZero() {
		if since := now.Sub(last); since < need {
			need -= since
		} else {
			need = 0
		}
	}
	if need <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(need)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// delay calcula la espera base: jitter gaussiano sobre la base, más tiempo por
// longitud del texto, con recorte a [400 ms, MaxDelay].
func (q *Queue) delay(textLen int) time.Duration {
	base := float64(q.cfg.BaseDelay.Milliseconds())
	jitter := rand.NormFloat64() * q.cfg.JitterSigma // sigma 0.25 → ±25 %
	d := base * (1 + jitter)

	d += float64(q.cfg.PerCharMS * textLen)
	if d < 400 {
		d = 400
	}
	if max := float64(q.cfg.MaxDelay.Milliseconds()); max > 0 && d > max {
		d = max
	}
	return time.Duration(d) * time.Millisecond
}

// slowdown: el "modo lento" del calentamiento. Duplica la espera y le suma
// medio segundo, con techo en el doble del máximo (nunca un minuto de espera).
func (q *Queue) slowdown(d time.Duration) time.Duration {
	slow := d*2 + 500*time.Millisecond
	if max := q.cfg.MaxDelay * 2; max > 0 && slow > max {
		slow = max
	}
	return slow
}

// budgetAt devuelve el cupo de mensajes no prioritarios para el día `now`.
// 20 el primer día → 500 a partir del séptimo. No toma el candado: quien
// llama debe tener `mu` o ser un método de prueba.
func (q *Queue) budgetAt(now time.Time) int {
	start, end := q.cfg.WarmupStart, q.cfg.WarmupEnd
	if end <= start {
		if end <= 0 {
			return 0
		}
		return end
	}

	// Se cuentan DÍAS DE CALENDARIO, no horas transcurridas: con horas, a las
	// 15:00 del primer día ya llevabas 0,66 días y el cupo salía en 66 en vez
	// de 20 (el número recién vinculado empezaba "caliente" sin motivo).
	anchor := q.anchor
	if anchor.IsZero() {
		anchor = now.UTC()
	}
	days := float64(dayNumber(now) - dayNumber(anchor))
	if days < 0 {
		days = 0
	}
	if days >= 7 {
		return end
	}
	step := float64(end-start) / 7
	return start + int(math.Round(days*step))
}

// dayNumber convierte un instante en un número de día UTC (para restar días
// de calendario sin que las horas del día ensucien la cuenta).
func dayNumber(t time.Time) int64 {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix() / 86400
}

// Budget es el cupo de hoy (lo usa .ping y el panel de salud).
func (q *Queue) Budget() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.budgetAt(time.Now())
}

// Snapshot devuelve una foto del estado de la cola.
func (q *Queue) Snapshot() Snapshot {
	now := time.Now()
	q.mu.Lock()
	defer q.mu.Unlock()

	today := now.UTC().Format("2006-01-02")
	sent := q.sentToday
	if q.day != today { // cambió el día y todavía no se ha enviado nada
		sent = 0
	}
	return Snapshot{
		Day:       today,
		SentToday: sent,
		Budget:    q.budgetAt(now),
		Anchor:    q.anchor,
		Slowed:    q.slowed,
	}
}
