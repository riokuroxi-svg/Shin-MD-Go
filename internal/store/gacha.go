// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// gacha.go — Personajes del gacha: quién tiene qué, en qué chat.
//
// En el JS esto vive en tres tablas (`characters`, `user_characters` y un JSON
// `g_chat_users.characters` que a veces es array y a veces string) y los
// "rolls" pendientes se guardan dentro de `g_chats.rolls`, un JSON del chat
// que crece para siempre y del que nunca se borra nada.
//
// Aquí hay UNA tabla de pertenencias (`gacha_owned`) y los rolls pendientes van
// a `gacha_rolls` con su mensaje, su dueño y su caducidad, de modo que un
// "claim" es un UPDATE con condición (nadie puede reclamar dos veces lo mismo)
// y los rolls viejos se pueden borrar sin tocar nada más.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Errores del gacha: los comandos los traducen a mensajes, así que se nombran
// por lo que le pasa al usuario y no por lo que pasa en la base.
var (
	ErrAlreadyClaimed = errors.New("personaje ya reclamado")
	ErrReserved       = errors.New("personaje reservado por otra persona")
	ErrRollExpired    = errors.New("roll caducado")
	ErrRollNotFound   = errors.New("roll no encontrado")
)

// OwnedCharacter es un personaje en el harem de alguien (en un chat).
type OwnedCharacter struct {
	ChatID    string
	OwnerID   string
	CharID    string
	Name      string
	Value     int64
	Favorite  bool
	ClaimedAt int64 // ms
}

// Roll es un personaje que salió en un `.roll` y aún se puede reclamar.
type Roll struct {
	ChatID     string
	MessageID  string
	CharID     string
	Name       string
	Value      int64
	RollerID   string
	ReservedBy string
	ReservedTo int64 // ms
	ExpiresAt  int64 // ms
	ClaimedBy  string
	ClaimedAt  int64
}

// RollTTL y ReserveTTL: los mismos tiempos del JS (60 s para reclamar, 20 s de
// reserva para quien lo sacó).
const (
	RollTTL    = 60 * time.Second
	ReserveTTL = 20 * time.Second
)

// SaveRoll apunta un personaje recién sacado, ligado al id del mensaje que lo
// anunció. Es lo que después permite reclamar respondiendo a ese mensaje sin
// tener que adivinar nada del texto.
func (s *Store) SaveRoll(ctx context.Context, r Roll) error {
	if r.ChatID == "" || r.MessageID == "" || r.CharID == "" {
		return fmt.Errorf("roll incompleto")
	}
	now := nowMS()
	if r.ExpiresAt == 0 {
		r.ExpiresAt = now + RollTTL.Milliseconds()
	}
	if r.ReservedTo == 0 {
		r.ReservedTo = now + ReserveTTL.Milliseconds()
	}
	// claimed_by va NULL (no cadena vacía): la condición del reclamo es
	// "claimed_by IS NULL" y con "" dejaría el roll imposible de reclamar.
	var reclamado any
	if r.ClaimedBy != "" {
		reclamado = r.ClaimedBy
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO gacha_rolls(chat_id, message_id, char_id, name, value, roller_id,
		                        reserved_to, expires_at, claimed_by, claimed_at)
		VALUES(?,?,?,?,?,?,?,?,?,0)
		ON CONFLICT(chat_id, message_id) DO UPDATE SET
			char_id = excluded.char_id, name = excluded.name, value = excluded.value,
			roller_id = excluded.roller_id, reserved_to = excluded.reserved_to,
			expires_at = excluded.expires_at`,
		r.ChatID, r.MessageID, r.CharID, r.Name, clampMoney(r.Value), r.RollerID,
		r.ReservedTo, r.ExpiresAt, reclamado)
	return err
}

// RollByMessage devuelve el roll anunciado en ese mensaje.
func (s *Store) RollByMessage(ctx context.Context, chatID, messageID string) (*Roll, error) {
	var r Roll
	var claimedBy sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT chat_id, message_id, char_id, name, value, roller_id, reserved_to,
		       expires_at, claimed_by, claimed_at
		  FROM gacha_rolls WHERE chat_id = ? AND message_id = ?`, chatID, messageID).
		Scan(&r.ChatID, &r.MessageID, &r.CharID, &r.Name, &r.Value, &r.RollerID,
			&r.ReservedTo, &r.ExpiresAt, &claimedBy, &r.ClaimedAt)
	if err == sql.ErrNoRows {
		return nil, ErrRollNotFound
	}
	if err != nil {
		return nil, err
	}
	if claimedBy.Valid {
		r.ClaimedBy = claimedBy.String
	}
	return &r, nil
}

// ClaimRoll reclama el personaje de un roll.
//
// Todo ocurre en una transacción y la condición decisiva va DENTRO del UPDATE:
//
//	UPDATE gacha_rolls SET claimed_by = ?
//	 WHERE chat_id = ? AND message_id = ? AND claimed_by IS NULL AND expires_at > ?
//	   AND (reserved_to <= ? OR roller_id = ?)
//
// Si alguien llegó antes (o el roll caducó, o está reservado para otra
// persona), no se actualiza ninguna fila y no se regala nada. Después se crea
// la pertenencia con INSERT OR IGNORE: si ya la tenía, no se duplica.
func (s *Store) ClaimRoll(ctx context.Context, chatID, messageID, ownerID string, now time.Time) (*OwnedCharacter, error) {
	if chatID == "" || messageID == "" || ownerID == "" {
		return nil, fmt.Errorf("claim incompleto")
	}
	nowM := ToMS(now)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var roll Roll
	var claimedBy sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT char_id, name, value, roller_id, reserved_to, expires_at, claimed_by
		  FROM gacha_rolls WHERE chat_id = ? AND message_id = ?`, chatID, messageID).
		Scan(&roll.CharID, &roll.Name, &roll.Value, &roll.RollerID, &roll.ReservedTo,
			&roll.ExpiresAt, &claimedBy)
	if err == sql.ErrNoRows {
		return nil, ErrRollNotFound
	}
	if err != nil {
		return nil, err
	}
	if claimedBy.Valid && claimedBy.String != "" {
		return nil, ErrAlreadyClaimed
	}
	if roll.ExpiresAt <= nowM {
		return nil, ErrRollExpired
	}
	if roll.ReservedTo > nowM && roll.RollerID != ownerID {
		return nil, ErrReserved
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE gacha_rolls SET claimed_by = ?, claimed_at = ?
		 WHERE chat_id = ? AND message_id = ? AND claimed_by IS NULL AND expires_at > ?`,
		ownerID, nowM, chatID, messageID, nowM)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrAlreadyClaimed
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO gacha_owned(chat_id, owner_id, char_id, name, value, favorite, claimed_at)
		VALUES(?,?,?,?,?,0,?)
		ON CONFLICT(chat_id, char_id) DO NOTHING`,
		chatID, ownerID, roll.CharID, roll.Name, clampMoney(roll.Value), nowM); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &OwnedCharacter{
		ChatID: chatID, OwnerID: ownerID, CharID: roll.CharID,
		Name: roll.Name, Value: roll.Value, ClaimedAt: nowM,
	}, nil
}

// Harem devuelve los personajes de una persona en un chat, ordenados por valor
// (los caros primero, como el JS).
func (s *Store) Harem(ctx context.Context, chatID, ownerID string) ([]OwnedCharacter, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT char_id, name, value, favorite, claimed_at
		  FROM gacha_owned WHERE chat_id = ? AND owner_id = ?
		 ORDER BY value DESC, claimed_at ASC`, chatID, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []OwnedCharacter
	for rows.Next() {
		c := OwnedCharacter{ChatID: chatID, OwnerID: ownerID}
		var fav int64
		if err := rows.Scan(&c.CharID, &c.Name, &c.Value, &fav, &c.ClaimedAt); err != nil {
			return nil, err
		}
		c.Favorite = fav != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

// OwnedCount cuenta los personajes de alguien en un chat.
func (s *Store) OwnedCount(ctx context.Context, chatID, ownerID string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM gacha_owned WHERE chat_id = ? AND owner_id = ?`, chatID, ownerID).Scan(&n)
	return n, err
}

// SetFavorite marca el personaje favorito (uno solo por persona y chat). El
// favorito no se puede robar ni vender, igual que en el JS.
func (s *Store) SetFavorite(ctx context.Context, chatID, ownerID, charID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var name string
	if err := tx.QueryRowContext(ctx,
		`SELECT name FROM gacha_owned WHERE chat_id = ? AND owner_id = ? AND char_id = ?`,
		chatID, ownerID, charID).Scan(&name); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("no tienes ese personaje")
		}
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE gacha_owned SET favorite = 0 WHERE chat_id = ? AND owner_id = ?`, chatID, ownerID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE gacha_owned SET favorite = 1 WHERE chat_id = ? AND owner_id = ? AND char_id = ?`,
		chatID, ownerID, charID); err != nil {
		return err
	}
	return tx.Commit()
}

// Favorite devuelve el personaje favorito ("" si no tiene).
func (s *Store) Favorite(ctx context.Context, chatID, ownerID string) (string, error) {
	var charID string
	err := s.db.QueryRowContext(ctx,
		`SELECT char_id FROM gacha_owned WHERE chat_id = ? AND owner_id = ? AND favorite = 1`,
		chatID, ownerID).Scan(&charID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return charID, err
}

// GachaTop devuelve quién tiene más personajes en el chat.
type GachaRow struct {
	OwnerID string
	Owned   int
	Value   int64
}

// GachaBoard: ranking por valor total del harem.
func (s *Store) GachaBoard(ctx context.Context, chatID string, limit int) ([]GachaRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT owner_id, COUNT(*), SUM(value)
		  FROM gacha_owned WHERE chat_id = ?
		 GROUP BY owner_id
		 ORDER BY SUM(value) DESC, COUNT(*) DESC, owner_id ASC
		 LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []GachaRow
	for rows.Next() {
		var r GachaRow
		if err := rows.Scan(&r.OwnerID, &r.Owned, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ─── ajustes del chat (gachaGate) ───────────────────────────────────────────

// GachaOn: ¿está el gacha activo en este chat? Por defecto sí.
func (s *Store) GachaOn(ctx context.Context, chatID string) (bool, error) {
	v, ok, err := s.GetSetting(ctx, chatID, "gacha")
	if err != nil {
		return false, err
	}
	if !ok || v == "" {
		return true, nil
	}
	return v != "off" && v != "0" && v != "false", nil
}

// SetGacha activa o desactiva el gacha en un chat.
func (s *Store) SetGacha(ctx context.Context, chatID string, on bool) error {
	val := "off"
	if on {
		val = "on"
	}
	return s.SetSetting(ctx, chatID, "gacha", val)
}

// PurgeRolls borra los rolls caducados hace más de un día. Sin esto la tabla
// crece para siempre (el JS guardaba cada roll dentro del JSON del chat y nunca
// borraba ninguno).
func (s *Store) PurgeRolls(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM gacha_rolls WHERE expires_at < ?`, ToMS(now.Add(-24*time.Hour)))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
