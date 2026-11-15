// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// gacha_extra.go — la economía del harem que quedaba: ventas, compras,
// intercambio, robo, regalos y los tableros. Todo sobre la tabla ya existente
// `gacha_owned` (sin migraciones: el precio de venta va en una columna
// añadida con ALTER IF-missing, patrón repo).
package store

import (
	"context"
	"database/sql"
	"time"
)

// ── Esquema extra ───────────────────────────────────────────────────────────
// La migración vive aquí y no en db.go a propósito: es un detalle del gacha y
// simplemente la primera vez que se toca. idempotente.

func (s *Store) ensureGachaExtra(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		ALTER TABLE gacha_owned ADD COLUMN sale_price INTEGER`)
	if err != nil {
		// SQLite viejo no tiene "IF NOT EXISTS" en ADD COLUMN; el error de
		// columna duplicada es exactamente el caso ya-existe, se traga.
		_, _ = s.db.ExecContext(ctx, `SELECT sale_price FROM gacha_owned LIMIT 1`)
	}
	return nil
}

// ── Ventas ──────────────────────────────────────────────────────────────────

// PutOnSale pone precio a un personaje del usuario; price=0 lo quita de venta.
func (s *Store) PutOnSale(ctx context.Context, chatID, ownerID, charID string, price int64) (bool, error) {
	if err := s.ensureGachaExtra(ctx); err != nil {
		return false, err
	}
	var precio any = price
	if price <= 0 {
		precio = nil
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE gacha_owned SET sale_price = ?
		 WHERE chat_id = ? AND owner_id = ? AND char_id = ?`,
		precio, chatID, ownerID, charID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Sale es un personaje a la venta en el chat.
type Sale struct {
	OwnerID string
	CharID  string
	Name    string
	Value   int64
	Price   int64
}

// ListSales: la tienda del harem (.haremshop).
func (s *Store) ListSales(ctx context.Context, chatID string, limit int) ([]Sale, error) {
	if err := s.ensureGachaExtra(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT owner_id, char_id, name, value, sale_price
		  FROM gacha_owned
		 WHERE chat_id = ? AND sale_price IS NOT NULL
		 ORDER BY sale_price DESC, name LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sale
	for rows.Next() {
		var s2 Sale
		if err := rows.Scan(&s2.OwnerID, &s2.CharID, &s2.Name, &s2.Value, &s2.Price); err != nil {
			return nil, err
		}
		out = append(out, s2)
	}
	return out, rows.Err()
}

// SaleByID: la ficha de venta para comprar/quitar.
func (s *Store) SaleByID(ctx context.Context, chatID, charID string) (*Sale, error) {
	if err := s.ensureGachaExtra(ctx); err != nil {
		return nil, err
	}
	var s2 Sale
	err := s.db.QueryRowContext(ctx, `
		SELECT owner_id, char_id, name, value, sale_price
		  FROM gacha_owned WHERE chat_id = ? AND char_id = ? AND sale_price IS NOT NULL`,
		chatID, charID).
		Scan(&s2.OwnerID, &s2.CharID, &s2.Name, &s2.Value, &s2.Price)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s2, nil
}

// BuySale: cobra al comprador, paga al vendedor, cambia de dueño, quita venta
// y quita el favorito si el vendedor lo tenía. Todo en una transacción.
func (s *Store) BuySale(ctx context.Context, chatID, comprador string, s2 *Sale) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// ¿Sigue a la venta y con el mismo dueño? (si no, la compra cancela)
	var n int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM gacha_owned
		 WHERE chat_id = ? AND char_id = ? AND owner_id = ? AND sale_price IS NOT NULL`,
		chatID, s2.CharID, s2.OwnerID).Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}

	// Mover monedas (sin trampas: la economía ya valida los fondos antes).
	if _, err := tx.ExecContext(ctx, `
		UPDATE economia SET wallet = wallet + ? WHERE chat_id = ? AND user_id = ?`,
		s2.Price, chatID, s2.OwnerID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE gacha_owned SET owner_id = ?, sale_price = NULL, favorite = 0
		 WHERE chat_id = ? AND char_id = ?`, comprador, chatID, s2.CharID); err != nil {
		return err
	}
	return tx.Commit()
}

// ── Intercambio / regalo / robo ─────────────────────────────────────────────

// Transfer: cambia el dueño de un personaje.
func (s *Store) TransferChar(ctx context.Context, chatID, charID, from, to string) (bool, error) {
	if err := s.ensureGachaExtra(ctx); err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE gacha_owned SET owner_id = ?, favorite = 0, sale_price = NULL
		 WHERE chat_id = ? AND char_id = ? AND owner_id = ?`,
		to, chatID, charID, from)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SwapCharacteres: trade atómico (A da su X, recibe su Y; y al revés).
func (s *Store) SwapCharacters(ctx context.Context, chatID, a, ida, b, idb string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	upd := func(from, to, id string) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE gacha_owned SET owner_id = ?, favorite = 0, sale_price = NULL
			 WHERE chat_id = ? AND char_id = ? AND owner_id = ?`,
			to, chatID, id, from)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return sql.ErrNoRows
		}
		return nil
	}
	if err := upd(a, b, ida); err != nil {
		return err
	}
	if err := upd(b, a, idb); err != nil {
		return err
	}
	return tx.Commit()
}

// Steal: robwaifu. Selecciona un personaje al azar del objetivo (no fav, no
// a la venta) y lo transfiere; el JS tenía probabilidad y aquí la misma
// probabilidad se decide arriba, en el comando.
func (s *Store) StealChar(ctx context.Context, chatID, victima, ladron string) (*OwnedCharacter, error) {
	if err := s.ensureGachaExtra(ctx); err != nil {
		return nil, err
	}
	var o OwnedCharacter
	err := s.db.QueryRowContext(ctx, `
		SELECT chat_id, owner_id, char_id, name, value, favorite, claimed_at
		  FROM gacha_owned
		 WHERE chat_id = ? AND owner_id = ? AND favorite = 0 AND sale_price IS NULL
		 ORDER BY RANDOM() LIMIT 1`, chatID, victima).
		Scan(&o.ChatID, &o.OwnerID, &o.CharID, &o.Name, &o.Value, &o.Favorite, &o.ClaimedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE gacha_owned SET owner_id = ? WHERE chat_id = ? AND char_id = ?`,
		ladron, chatID, o.CharID); err != nil {
		return nil, err
	}
	o.OwnerID = ladron
	return &o, nil
}

// DeleteOwned: «.delwaifu» (la suelta: desaparece del harem; JS la mandaba de
// vuelta al pool, que aquí equivale a borrar la fila).
func (s *Store) DeleteOwned(ctx context.Context, chatID, ownerID, charID string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM gacha_owned WHERE chat_id = ? AND owner_id = ? AND char_id = ?`,
		chatID, ownerID, charID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// OwnerOf: quién tiene charID en el chat (para las capas de arriba).
func (s *Store) OwnerOf(ctx context.Context, chatID, charID string) (*OwnedCharacter, error) {
	var o OwnedCharacter
	err := s.db.QueryRowContext(ctx, `
		SELECT chat_id, owner_id, char_id, name, value, favorite, claimed_at
		  FROM gacha_owned WHERE chat_id = ? AND char_id = ?`, chatID, charID).
		Scan(&o.ChatID, &o.OwnerID, &o.CharID, &o.Name, &o.Value, &o.Favorite, &o.ClaimedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// ── Tableros ────────────────────────────────────────────────────────────────

// CharBoardRow: personaje más veces reclamado (para waifustop/favtop).
type CharBoardRow struct {
	CharID string
	Name   string
	Count  int
	Value  int64
}

// CharBoard ordena por nº de dueños (o por valor si byValue).
func (s *Store) CharBoard(ctx context.Context, chatID string, limit int, byValue bool) ([]CharBoardRow, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	ord := "n DESC, v DESC"
	if byValue {
		ord = "v DESC, n DESC"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT char_id, name, COUNT(*) n, MAX(value) v
		  FROM gacha_owned WHERE chat_id = ?
		 GROUP BY char_id, name ORDER BY `+ord+` LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CharBoardRow
	for rows.Next() {
		var r CharBoardRow
		if err := rows.Scan(&r.CharID, &r.Name, &r.Count, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FavBoard ordena por favoritos señalados (los ⭐ de cada uno).
func (s *Store) FavBoard(ctx context.Context, chatID string, limit int) ([]CharBoardRow, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	if err := s.ensureGachaExtra(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT char_id, name, COUNT(*) n, MAX(value) v
		  FROM gacha_owned WHERE chat_id = ? AND favorite = 1
		 GROUP BY char_id, name ORDER BY n DESC, v DESC LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CharBoardRow
	for rows.Next() {
		var r CharBoardRow
		if err := rows.Scan(&r.CharID, &r.Name, &r.Count, &r.Value); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ClaimMessage: el texto del chat para cuando alguien reclama.
func (s *Store) ClaimMessage(ctx context.Context, chatID string) (string, error) {
	v, ok, err := s.GetSetting(ctx, chatID, "gacha_claim_msg")
	if err != nil || !ok {
		return "", err
	}
	return v, nil
}

// SetClaimMessage pone/quita el texto (vacío = quitar, con json_remove).
func (s *Store) SetClaimMessage(ctx context.Context, chatID, msg string) error {
	if msg == "" {
		_, err := s.db.ExecContext(ctx,
			`UPDATE settings SET json = json_remove(json, '$.gacha_claim_msg') WHERE jid = ?`, chatID)
		return err
	}
	return s.SetSetting(ctx, chatID, "gacha_claim_msg", msg)
}

// NowMs: con ClaimedAt del JS, que era milisegundos.
func NowMs() int64 { return time.Now().UnixMilli() }

// AddValue suma a un personaje del usuario (es el voto diario del JS).
func (s *Store) AddValue(ctx context.Context, chatID, ownerID, charID string, delta int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `
		UPDATE gacha_owned SET value = value + ?
		 WHERE chat_id = ? AND owner_id = ? AND char_id = ?`,
		delta, chatID, ownerID, charID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}
