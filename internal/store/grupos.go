// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// grupos.go — Advertencias, actividad y ajustes del grupo.
//
// Lo que cambia respecto al JS:
//
//   - Las advertencias del JS eran un array dentro de `chat_users.warnings`
//     (a veces texto, a veces array). Aquí son filas: se puede borrar la #3 sin
//     reescribir la lista entera, contar sin traerlas todas y no hay forma de
//     que un JSON roto deje a alguien "sin advertencias" para siempre.
//   - El conteo de mensajes del JS vivía en `chat_users.stats`, un objeto
//     `{fecha: {msgs: n}}` que crecía cada día y que había que leer y reescribir
//     ENTERO en cada mensaje. Aquí es una fila por (chat, usuario, día) y se
//     suma con un UPSERT atómico: no hay lectura previa, así que dos mensajes a
//     la vez no pierden la cuenta.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Warn es una advertencia.
type Warn struct {
	ID     int64
	UserID string
	Reason string
	ByID   string
	At     int64 // ms
}

// Límites de advertencias: el JS acepta 1–10 y 0 para desactivar.
const (
	MaxWarnLimit = 10
	// MaxWarnReason: una razón puede ser larga, pero no un ensayo (evita que
	// alguien llene la base escribiendo un testamento en un .warn).
	MaxWarnReason = 300
	// MaxWarnsPerUser: tope de filas por persona y grupo. Con el límite en 10
	// y la expulsión automática, 100 es margen de sobra; sin tope, quien no
	// expulsa acumula advertencias para siempre.
	MaxWarnsPerUser = 100
)

// AddWarn apunta una advertencia y devuelve el total que queda.
func (s *Store) AddWarn(ctx context.Context, chatID, userID, reason, byID string) (int, error) {
	if chatID == "" || userID == "" {
		return 0, fmt.Errorf("chat o usuario vacío")
	}
	reason = recortar(reason, MaxWarnReason)
	if reason == "" {
		reason = "Sin razón."
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO group_warns(chat_id, user_id, reason, by_id, at) VALUES(?,?,?,?,?)`,
		chatID, userID, reason, byID, nowMS()); err != nil {
		return 0, err
	}

	// Si se pasa del tope, se tiran las más viejas (las nuevas importan más:
	// son las que describen lo que está pasando ahora).
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM group_warns
		 WHERE chat_id = ? AND user_id = ?
		   AND id NOT IN (
			SELECT id FROM group_warns WHERE chat_id = ? AND user_id = ?
			 ORDER BY id DESC LIMIT ?)`,
		chatID, userID, chatID, userID, MaxWarnsPerUser); err != nil {
		return 0, err
	}

	var total int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM group_warns WHERE chat_id = ? AND user_id = ?`,
		chatID, userID).Scan(&total); err != nil {
		return 0, err
	}
	return total, tx.Commit()
}

// Warns devuelve las advertencias, la más nueva primero (como el JS).
func (s *Store) Warns(ctx context.Context, chatID, userID string) ([]Warn, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, reason, by_id, at FROM group_warns
		 WHERE chat_id = ? AND user_id = ? ORDER BY id DESC`, chatID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Warn
	for rows.Next() {
		var w Warn
		var by sql.NullString
		if err := rows.Scan(&w.ID, &w.UserID, &w.Reason, &by, &w.At); err != nil {
			return nil, err
		}
		w.ByID = by.String
		out = append(out, w)
	}
	return out, rows.Err()
}

// WarnCount cuenta sin traer las filas.
func (s *Store) WarnCount(ctx context.Context, chatID, userID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM group_warns WHERE chat_id = ? AND user_id = ?`, chatID, userID).Scan(&n)
	return n, err
}

// DelWarn borra por índice visible (1 = la más nueva). Devuelve cuántas quedan.
func (s *Store) DelWarn(ctx context.Context, chatID, userID string, index int) (int, bool, error) {
	warns, err := s.Warns(ctx, chatID, userID)
	if err != nil {
		return 0, false, err
	}
	if index < 1 || index > len(warns) {
		return len(warns), false, nil
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM group_warns WHERE id = ?`, warns[index-1].ID); err != nil {
		return 0, false, err
	}
	return len(warns) - 1, true, nil
}

// ClearWarns borra todas las de una persona.
func (s *Store) ClearWarns(ctx context.Context, chatID, userID string) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM group_warns WHERE chat_id = ? AND user_id = ?`, chatID, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ─── actividad ──────────────────────────────────────────────────────────────

// AddActivity suma un mensaje al contador del día. Una sola sentencia: no hay
// lectura previa, así que no se pierde nada con mensajes simultáneos.
func (s *Store) AddActivity(ctx context.Context, chatID, userID string, when time.Time) error {
	if chatID == "" || userID == "" {
		return nil // sin datos no se cuenta, pero tampoco se falla
	}
	day := when.UTC().Format("2006-01-02")
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO chat_activity(chat_id, user_id, day, msgs) VALUES(?,?,?,1)
		ON CONFLICT(chat_id, user_id, day) DO UPDATE SET msgs = msgs + 1`,
		chatID, userID, day)
	return err
}

// Messages devuelve (total histórico, mensajes desde `since`).
func (s *Store) Messages(ctx context.Context, chatID, userID string, since time.Time) (total int64, recent int64, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(msgs),0),
		       COALESCE(SUM(CASE WHEN day >= ? THEN msgs ELSE 0 END),0)
		  FROM chat_activity WHERE chat_id = ? AND user_id = ?`,
		since.UTC().Format("2006-01-02"), chatID, userID).Scan(&total, &recent)
	return
}

// ActivityRow es una línea de los rankings de mensajes.
type ActivityRow struct {
	UserID string
	Msgs   int64
}

// TopActivity: quién más escribe en el grupo desde `since`.
func (s *Store) TopActivity(ctx context.Context, chatID string, since time.Time, limit int) ([]ActivityRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, SUM(msgs) AS total FROM chat_activity
		 WHERE chat_id = ? AND day >= ?
		 GROUP BY user_id ORDER BY total DESC, user_id ASC LIMIT ?`,
		chatID, since.UTC().Format("2006-01-02"), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ActivityRow
	for rows.Next() {
		var r ActivityRow
		if err := rows.Scan(&r.UserID, &r.Msgs); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ActivityOf devuelve el mapa usuario → mensajes desde `since`. Lo usa
// .topinactive para no hacer una consulta por participante.
func (s *Store) ActivityOf(ctx context.Context, chatID string, since time.Time) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, SUM(msgs) FROM chat_activity
		 WHERE chat_id = ? AND day >= ? GROUP BY user_id`,
		chatID, since.UTC().Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]int64{}
	for rows.Next() {
		var id string
		var n int64
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ─── ajustes del grupo ──────────────────────────────────────────────────────

// Claves de ajustes por chat. Son las mismas palabras que usa el JS para que
// una base migrada se lea sin traducción.
const (
	KeyWelcome    = "welcome"
	KeyGoodbye    = "goodbye"
	KeySetWelcome = "setwelcome"
	KeySetGoodbye = "setgoodbye"
	KeyWarnLimit  = "warnlimit"
	KeyExpulsar   = "expulsar"
	KeyAntiStatus = "antistatus"
	KeyBotOff     = "bot"
)

// Flag lee un ajuste sí/no del chat (por defecto, `def`).
func (s *Store) Flag(ctx context.Context, chatID, key string, def bool) (bool, error) {
	v, ok, err := s.GetSetting(ctx, chatID, key)
	if err != nil {
		return def, err
	}
	if !ok || v == "" {
		return def, nil
	}
	return v == "on" || v == "1" || v == "true", nil
}

// SetFlag guarda un ajuste sí/no.
func (s *Store) SetFlag(ctx context.Context, chatID, key string, on bool) error {
	val := "off"
	if on {
		val = "on"
	}
	return s.SetSetting(ctx, chatID, key, val)
}

// Text lee un ajuste de texto con valor por defecto.
func (s *Store) Text(ctx context.Context, chatID, key, def string) (string, error) {
	v, ok, err := s.GetSetting(ctx, chatID, key)
	if err != nil {
		return def, err
	}
	if !ok || v == "" {
		return def, nil
	}
	return v, nil
}

// WarnLimit: límite de advertencias del grupo y si al alcanzarlo se expulsa.
func (s *Store) WarnLimit(ctx context.Context, chatID string) (limit int, expulsar bool, err error) {
	v, ok, err := s.GetSetting(ctx, chatID, KeyWarnLimit)
	if err != nil {
		return 0, false, err
	}
	if !ok || v == "" {
		return 0, false, nil // 0 = desactivado, como el JS
	}
	n, err := parseInt64(v)
	if err != nil {
		return 0, false, nil
	}
	limit = int(n)
	exp, err := s.Flag(ctx, chatID, KeyExpulsar, false)
	if err != nil {
		return 0, false, err
	}
	return int(limit), exp, nil
}

// SetWarnLimit guarda límite y expulsión. Con 0 se desactiva todo.
func (s *Store) SetWarnLimit(ctx context.Context, chatID string, limit int, expulsar bool) error {
	if limit < 0 || limit > MaxWarnLimit {
		return fmt.Errorf("límite de advertencias fuera de rango: %d", limit)
	}
	if err := s.SetSetting(ctx, chatID, KeyWarnLimit, fmt.Sprint(limit)); err != nil {
		return err
	}
	return s.SetFlag(ctx, chatID, KeyExpulsar, expulsar && limit > 0)
}

// BotOff: ¿el bot está apagado en este chat? (el `.bot off` del JS)
func (s *Store) BotOff(ctx context.Context, chatID string) (bool, error) {
	v, ok, err := s.GetSetting(ctx, chatID, KeyBotOff)
	if err != nil {
		return false, err
	}
	if !ok || v == "" {
		return false, nil
	}
	return v == "off", nil
}

// ─── ayudas ─────────────────────────────────────────────────────────────────

// parseInt64 sin importar strconv en medio archivo.
func parseInt64(s string) (int64, error) {
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, err
	}
	return n, nil
}

// recortar deja el texto en `max` runas (no bytes: un corte a mitad de un emoji
// deja caracteres rotos en WhatsApp).
func recortar(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
