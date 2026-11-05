// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// rachas.go — Racha de .daily / .weekly / .monthly. A diferencia del dinero
// (que va por chat), la racha es GLOBAL: una por persona, como en el JS
// (g_users.streak / lastDailyGlobal). Si alguien reclama en dos grupos el
// mismo día, la racha sube una vez y el segundo grupo no paga.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// StreakKind: los tres premios con racha.
type StreakKind string

const (
	StreakDaily   StreakKind = "daily"
	StreakWeekly  StreakKind = "weekly"
	StreakMonthly StreakKind = "monthly"
)

// streakCols: columnas y cadencia de cada racha. Está cerrado a propósito:
// el nombre de columna se elige de aquí, nunca se concatena algo del mensaje.
type streakCols struct {
	streakCol string
	lastCol   string
	interval  time.Duration
	max       int64
}

var streaks = map[StreakKind]streakCols{
	// 24 h de cadencia, se pierde pasadas 36 h (1.5 días) sin reclamar.
	StreakDaily: {"streak", "last_daily_global", 24 * time.Hour, 200},
	// 7 días de cadencia, margen de 1.5 semanas. Tope 30.
	StreakWeekly: {"weekly_streak", "last_weekly_global", 7 * 24 * time.Hour, 30},
	// 30 días de cadencia, margen de 45 días. Tope 8 (como el JS).
	StreakMonthly: {"monthly_streak", "last_monthly_global", 30 * 24 * time.Hour, 8},
}

// StreakState es lo que se enseña en el mensaje.
type StreakState struct {
	Streak  int64         // racha que queda después de esta llamada
	Claimed bool          // ¿esta llamada fue la que reclamó?
	Wait    time.Duration // si no se pudo, cuánto falta
}

// ClaimStreak intenta cobrar el premio con racha.
//
// Todo va en un solo UPDATE con su condición dentro:
//
//	· solo entra si ha pasado la cadencia desde el último cobro,
//	· y calcula la racha nueva en la misma sentencia (si pasó más de
//	  cadencia*1.5, vuelve a 1; si no, sube uno hasta el tope).
//
// Así dos mensajes simultáneos no pueden cobrar dos veces ni saltarse el tope.
func (s *Store) ClaimStreak(ctx context.Context, userID string, kind StreakKind, now time.Time) (StreakState, error) {
	cols, ok := streaks[kind]
	if !ok {
		return StreakState{}, fmt.Errorf("racha desconocida: %q", kind)
	}
	if _, err := s.Get(ctx, userID); err != nil {
		return StreakState{}, err
	}

	nowMS := ToMS(now)
	interval := cols.interval.Milliseconds()
	reset := interval + interval/2 // cadencia * 1.5

	query := `UPDATE users SET
		` + cols.streakCol + ` = CASE
			WHEN ` + cols.lastCol + ` > 0 AND (? - ` + cols.lastCol + `) > ? THEN 1
			WHEN ` + cols.streakCol + ` + 1 > ? THEN ?
			ELSE ` + cols.streakCol + ` + 1 END,
		` + cols.lastCol + ` = ?
		WHERE jid = ? AND (` + cols.lastCol + ` = 0 OR ? >= ` + cols.lastCol + ` + ?)`

	res, err := s.db.ExecContext(ctx, query,
		nowMS, reset, // CASE de reinicio
		cols.max, cols.max, // tope
		nowMS, userID, // nuevo last, y condición
		nowMS, interval)
	if err != nil {
		return StreakState{}, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return StreakState{}, err
	}

	var streak, last int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT `+cols.streakCol+`, `+cols.lastCol+` FROM users WHERE jid = ?`, userID).
		Scan(&streak, &last); err != nil && err != sql.ErrNoRows {
		return StreakState{}, err
	}

	if n > 0 {
		return StreakState{Streak: streak, Claimed: true}, nil
	}
	wait := time.Duration(last+interval-nowMS) * time.Millisecond
	if wait < 0 {
		wait = 0
	}
	return StreakState{Streak: streak, Claimed: false, Wait: wait}, nil
}

// StreakValue lee la racha actual sin cobrar nada (para .balance y .perfil).
func (s *Store) StreakValue(ctx context.Context, userID string, kind StreakKind) (int64, error) {
	cols, ok := streaks[kind]
	if !ok {
		return 0, fmt.Errorf("racha desconocida: %q", kind)
	}
	var streak int64
	err := s.db.QueryRowContext(ctx,
		`SELECT `+cols.streakCol+` FROM users WHERE jid = ?`, userID).Scan(&streak)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return streak, err
}
