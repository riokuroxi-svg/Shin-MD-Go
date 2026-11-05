// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// economia.go — Economía POR CHAT, igual que el bot de JS (g_chat_users).
//
// ¿Por qué por chat y no global? Porque así lo hace el bot de producción: los
// mismos dos usuarios pueden tener saldos distintos en cada grupo, y la
// migración de datos es 1:1 en vez de un invento. La racha diaria/semanal/
// mensual sí es global (una por persona), también como en el JS (g_users).
//
// Todas las operaciones que mueven dinero son UNA sola sentencia con su
// condición dentro. Esa es la diferencia con el JS, donde se leía el saldo,
// se calculaba en JavaScript y se hacía un UPDATE detrás: dos mensajes a la
// vez podían gastar el mismo dinero dos veces. Aquí, si la condición no se
// cumple, `RowsAffected()` es 0 y la operación no ocurrió.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Límites del dinero. Sin esto, un `.transfer 999999999999999999` o una
// importación con basura deja saldos que rompen la aritmética (en JS, además,
// por encima de 2^53 los enteros pierden precisión y el saldo "tiembla").
const (
	MaxMoney     int64 = 1_000_000_000_000 // 1 billón: techo por saldo
	MaxTransfer  int64 = 1_000_000_000     // 1000 millones por transferencia
	MaxStat      int64 = 100               // salud/stamina/magia sobre 100
	MaxItemCount int64 = 9_999             // unidades de un objeto
)

// ChatAccount es el saldo de una persona EN UN CHAT (equivale a g_chat_users).
type ChatAccount struct {
	ChatID  string
	UserID  string
	Coins   int64
	Bank    int64
	Health  int64
	Stamina int64
	Magic   int64

	LastWork    int64 // milisegundos UTC (0 = nunca)
	LastDaily   int64
	LastWeekly  int64
	LastMonthly int64

	Inventory map[string]int64
}

// Total: lo que tiene entre cartera y banco.
func (a *ChatAccount) Total() int64 { return a.Coins + a.Bank }

// nowMS: el JS guarda los enfriamientos en milisegundos desde 1970. Se usa lo
// mismo para que una base migrada siga cuadrando.
func nowMS() int64 { return time.Now().UnixMilli() }

// FromMS convierte milisegundos del JS a time.Time.
func FromMS(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}

// ToMS convierte time.Time a milisegundos (0 si es cero).
func ToMS(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// ─── lectura ────────────────────────────────────────────────────────────────

// ChatUser devuelve la cuenta de (chat, usuario), creándola si no existe.
//
// Al crear la primera cuenta de alguien en un chat se revisa si venía de la
// versión anterior del bot, donde el dinero era global (users.coins): en ese
// caso se siembra una sola vez y se marca `economy_seeded`, para que entrar a
// un grupo nuevo no regale el saldo otra vez.
func (s *Store) ChatUser(ctx context.Context, chatID, userID string) (*ChatAccount, error) {
	if chatID == "" || userID == "" {
		return nil, fmt.Errorf("chat o usuario vacío")
	}

	acc, err := s.chatUserRow(ctx, chatID, userID)
	if err == nil {
		return acc, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	if err := s.createChatUser(ctx, chatID, userID); err != nil {
		return nil, err
	}
	return s.chatUserRow(ctx, chatID, userID)
}

// createChatUser crea la fila (o no hace nada si ya existe) y siembra el saldo
// heredado de la versión global, una única vez por usuario.
func (s *Store) createChatUser(ctx context.Context, chatID, userID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`INSERT OR IGNORE INTO chat_users(chat_id, user_id) VALUES(?, ?)`, chatID, userID); err != nil {
		return err
	}

	// ¿hay saldo global sin sembrar?
	var coins, bank, seeded int64
	err = tx.QueryRowContext(ctx, `SELECT coins, bank, economy_seeded FROM users WHERE jid = ?`, userID).
		Scan(&coins, &bank, &seeded)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && seeded == 0 && (coins > 0 || bank > 0) {
		if _, err := tx.ExecContext(ctx,
			`UPDATE chat_users SET coins = ?, bank = ? WHERE chat_id = ? AND user_id = ?`,
			clampMoney(coins), clampMoney(bank), chatID, userID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE users SET economy_seeded = 1, coins = 0, bank = 0 WHERE jid = ?`, userID); err != nil {
			return err
		}
	} else if err == nil && seeded == 0 {
		// Sin saldo heredado: se marca igual para no volver a mirar.
		if _, err := tx.ExecContext(ctx, `UPDATE users SET economy_seeded = 1 WHERE jid = ?`, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) chatUserRow(ctx context.Context, chatID, userID string) (*ChatAccount, error) {
	var (
		acc       ChatAccount
		inventory string
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT chat_id, user_id, coins, bank, health, stamina, magic, inventory,
		       last_work, last_daily, last_weekly, last_monthly
		  FROM chat_users WHERE chat_id = ? AND user_id = ?`, chatID, userID).
		Scan(&acc.ChatID, &acc.UserID, &acc.Coins, &acc.Bank, &acc.Health, &acc.Stamina,
			&acc.Magic, &inventory, &acc.LastWork, &acc.LastDaily, &acc.LastWeekly, &acc.LastMonthly)
	if err != nil {
		return nil, err
	}
	acc.Inventory = parseInventory(inventory)
	return &acc, nil
}

// ─── dinero (atómico) ───────────────────────────────────────────────────────

// AddCoins suma (o resta con delta negativo) a la cartera del chat. El saldo
// nunca baja de 0 ni pasa de MaxMoney.
func (s *Store) AddCoins(ctx context.Context, chatID, userID string, delta int64) (int64, error) {
	if _, err := s.ChatUser(ctx, chatID, userID); err != nil {
		return 0, err
	}
	if delta > 0 {
		delta = clampMoney(delta)
	} else {
		delta = -clampMoney(-delta)
	}

	// Una sola sentencia: el recorte se hace con MIN/MAX en SQL, así dos
	// sumas concurrentes no se pisan (el UPDATE siempre parte del valor actual).
	if _, err := s.db.ExecContext(ctx, `
		UPDATE chat_users
		   SET coins = MIN(MAX(coins + ?, 0), ?)
		 WHERE chat_id = ? AND user_id = ?`, delta, MaxMoney, chatID, userID); err != nil {
		return 0, err
	}

	var coins int64
	err := s.db.QueryRowContext(ctx,
		`SELECT coins FROM chat_users WHERE chat_id = ? AND user_id = ?`, chatID, userID).Scan(&coins)
	return coins, err
}

// Spend cobra `amount` de la cartera. Devuelve false si no alcanza. Es
// atómico: el descuento solo ocurre si el saldo era suficiente en ese instante.
func (s *Store) Spend(ctx context.Context, chatID, userID string, amount int64) (bool, error) {
	if amount <= 0 || amount > MaxMoney {
		return false, nil
	}
	if _, err := s.ChatUser(ctx, chatID, userID); err != nil {
		return false, err
	}

	res, err := s.db.ExecContext(ctx, `
		UPDATE chat_users SET coins = coins - ?
		 WHERE chat_id = ? AND user_id = ? AND coins >= ?`,
		amount, chatID, userID, amount)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Deposit mueve de la cartera al banco. Atómico.
func (s *Store) Deposit(ctx context.Context, chatID, userID string, amount int64) (bool, error) {
	return s.moveCoins(ctx, chatID, userID, amount, true)
}

// Withdraw mueve del banco a la cartera. Atómico.
func (s *Store) Withdraw(ctx context.Context, chatID, userID string, amount int64) (bool, error) {
	return s.moveCoins(ctx, chatID, userID, amount, false)
}

func (s *Store) moveCoins(ctx context.Context, chatID, userID string, amount int64, toBank bool) (bool, error) {
	if amount <= 0 || amount > MaxMoney {
		return false, nil
	}
	if _, err := s.ChatUser(ctx, chatID, userID); err != nil {
		return false, err
	}

	// `from` es la columna que se descuenta; se elige entre dos literales
	// fijos, nunca con una cadena que venga de fuera.
	var query string
	if toBank {
		query = `UPDATE chat_users
		            SET coins = coins - ?, bank = MIN(bank + ?, ?)
		          WHERE chat_id = ? AND user_id = ? AND coins >= ?`
	} else {
		query = `UPDATE chat_users
		            SET bank = bank - ?, coins = MIN(coins + ?, ?)
		          WHERE chat_id = ? AND user_id = ? AND bank >= ?`
	}

	res, err := s.db.ExecContext(ctx, query, amount, amount, MaxMoney, chatID, userID, amount)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// Transfer mueve dinero ENTRE DOS PERSONAS del mismo chat, en una transacción.
//
// Va desde el banco, como el `.givecoins` del JS. Rechaza: cantidad inválida,
// a uno mismo y al bot. La resta y la suma ocurren juntas o no ocurren: si el
// que envía se queda sin saldo en medio, no hay abono.
func (s *Store) Transfer(ctx context.Context, chatID, fromID, toID string, amount int64) (bool, error) {
	if amount <= 0 || amount > MaxTransfer {
		return false, nil
	}
	if fromID == "" || toID == "" || fromID == toID {
		return false, nil
	}

	if _, err := s.ChatUser(ctx, chatID, fromID); err != nil {
		return false, err
	}
	if _, err := s.ChatUser(ctx, chatID, toID); err != nil {
		return false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		UPDATE chat_users SET bank = bank - ?
		 WHERE chat_id = ? AND user_id = ? AND bank >= ?`,
		amount, chatID, fromID, amount)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, nil // no tenía saldo: no se abona nada
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE chat_users SET bank = MIN(bank + ?, ?)
		 WHERE chat_id = ? AND user_id = ?`, amount, MaxMoney, chatID, toID); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ─── enfriamientos (atómico) ────────────────────────────────────────────────

// cooldownColumns: las únicas columnas que se pueden tocar. Se usa un mapa y
// no concatenación, para que un nombre de fuera no acabe dentro del SQL.
var cooldownColumns = map[string]bool{
	"last_work": true, "last_daily": true, "last_weekly": true, "last_monthly": true,
	"last_roll": true, "last_claim": true,
}

// ClaimCooldown intenta reservar el turno de un comando con enfriamiento.
//
// Devuelve (true, restante=0) si se puede usar ahora, y (false, restante>0)
// si todavía falta. La condición va DENTRO del UPDATE: si dos mensajes llegan
// a la vez, solo uno consigue la fila y el otro ve RowsAffected=0. En el JS
// esto era "leer, comparar, escribir" y con dos mensajes seguidos se podía
// reclamar dos veces.
func (s *Store) ClaimCooldown(ctx context.Context, chatID, userID, column string, cooldown time.Duration) (bool, time.Duration, error) {
	if !cooldownColumns[column] {
		return false, 0, fmt.Errorf("columna de enfriamiento inválida: %q", column)
	}
	if _, err := s.ChatUser(ctx, chatID, userID); err != nil {
		return false, 0, err
	}

	now := nowMS()
	next := now + cooldown.Milliseconds()

	// El nombre de columna ya está validado contra el mapa: los valores siguen
	// yendo por parámetros.
	query := `UPDATE chat_users SET ` + column + ` = ?
	           WHERE chat_id = ? AND user_id = ? AND ` + column + ` <= ?`
	res, err := s.db.ExecContext(ctx, query, next, chatID, userID, now)
	if err != nil {
		return false, 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, 0, err
	}
	if n > 0 {
		return true, 0, nil
	}

	// No se pudo: se informa cuánto falta (para el mensaje al usuario).
	var last int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT `+column+` FROM chat_users WHERE chat_id = ? AND user_id = ?`,
		chatID, userID).Scan(&last); err != nil {
		return false, 0, err
	}
	restante := time.Duration(last-now) * time.Millisecond
	if restante < 0 {
		restante = 0
	}
	return false, restante, nil
}

// ─── ranking ────────────────────────────────────────────────────────────────

// BoardRow es una línea del ranking.
type BoardRow struct {
	UserID string
	Coins  int64
	Bank   int64
}

func (r BoardRow) Total() int64 { return r.Coins + r.Bank }

// Board devuelve los mejores del chat por dinero total (cartera + banco).
func (s *Store) Board(ctx context.Context, chatID string, limit int) ([]BoardRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, coins, bank FROM chat_users
		 WHERE chat_id = ?
		 ORDER BY (coins + bank) DESC, user_id ASC
		 LIMIT ?`, chatID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BoardRow
	for rows.Next() {
		var r BoardRow
		if err := rows.Scan(&r.UserID, &r.Coins, &r.Bank); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ─── inventario ─────────────────────────────────────────────────────────────

// Inventory devuelve el inventario del usuario en el chat.
func (s *Store) Inventory(ctx context.Context, chatID, userID string) (map[string]int64, error) {
	acc, err := s.ChatUser(ctx, chatID, userID)
	if err != nil {
		return nil, err
	}
	return acc.Inventory, nil
}

// AddItem suma unidades de un objeto al inventario. El id del objeto se valida
// contra el catálogo del juego (mapa cerrado): si no, el JSON del inventario
// se podría llenar de claves arbitrarias y crecer sin control.
func (s *Store) AddItem(ctx context.Context, chatID, userID, item string, qty int64) (int64, error) {
	if !ValidItem(item) {
		return 0, fmt.Errorf("objeto desconocido: %q", item)
	}
	if qty <= 0 {
		return 0, fmt.Errorf("cantidad inválida: %d", qty)
	}

	inv, err := s.Inventory(ctx, chatID, userID)
	if err != nil {
		return 0, err
	}
	total := inv[item] + qty
	if total > MaxItemCount {
		total = MaxItemCount
	}
	inv[item] = total

	if err := s.saveInventory(ctx, chatID, userID, inv); err != nil {
		return 0, err
	}
	return total, nil
}

// TakeItem quita unidades; devuelve false si no había suficientes.
func (s *Store) TakeItem(ctx context.Context, chatID, userID, item string, qty int64) (bool, error) {
	if !ValidItem(item) || qty <= 0 {
		return false, nil
	}
	inv, err := s.Inventory(ctx, chatID, userID)
	if err != nil {
		return false, err
	}
	if inv[item] < qty {
		return false, nil
	}
	inv[item] -= qty
	if inv[item] <= 0 {
		delete(inv, item)
	}
	return true, s.saveInventory(ctx, chatID, userID, inv)
}

func (s *Store) saveInventory(ctx context.Context, chatID, userID string, inv map[string]int64) error {
	raw, err := json.Marshal(inv)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE chat_users SET inventory = ? WHERE chat_id = ? AND user_id = ?`,
		string(raw), chatID, userID)
	return err
}

// SetStat pone salud/stamina/magia entre 0 y MaxStat.
func (s *Store) SetStat(ctx context.Context, chatID, userID, stat string, value int64) error {
	var col string
	switch stat {
	case "health", "stamina", "magic":
		col = stat
	default:
		return fmt.Errorf("estadística inválida: %q", stat)
	}
	if _, err := s.ChatUser(ctx, chatID, userID); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE chat_users SET `+col+` = ? WHERE chat_id = ? AND user_id = ?`,
		clampStat(value), chatID, userID)
	return err
}

// ─── ajustes por chat (economía on/off, como el economyGate del JS) ─────────

const (
	keyChatEconomy = "economy"
	keyChatAdmin   = "adminonly"
)

// EconomyOn: ¿está la economía activa en este chat? Por defecto sí (igual que
// el JS: g_chats.economy DEFAULT 1).
func (s *Store) EconomyOn(ctx context.Context, chatID string) (bool, error) {
	v, ok, err := s.GetSetting(ctx, chatID, keyChatEconomy)
	if err != nil {
		return false, err
	}
	if !ok || v == "" {
		return true, nil
	}
	return v != "off" && v != "0" && v != "false", nil
}

// SetEconomy activa o desactiva la economía en un chat.
func (s *Store) SetEconomy(ctx context.Context, chatID string, on bool) error {
	val := "off"
	if on {
		val = "on"
	}
	return s.SetSetting(ctx, chatID, keyChatEconomy, val)
}

// AdminOnly: si el chat está en modo "solo admins" la economía se bloquea
// también (es lo que hace el gate del JS).
func (s *Store) AdminOnly(ctx context.Context, chatID string) (bool, error) {
	v, ok, err := s.GetSetting(ctx, chatID, keyChatAdmin)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return v == "on" || v == "1" || v == "true", nil
}

// ─── utilidades ─────────────────────────────────────────────────────────────

// clampMoney deja el valor entre 0 y MaxMoney.
func clampMoney(v int64) int64 {
	if v < 0 {
		return 0
	}
	if v > MaxMoney {
		return MaxMoney
	}
	return v
}

func clampStat(v int64) int64 {
	if v < 0 {
		return 0
	}
	if v > MaxStat {
		return MaxStat
	}
	return v
}

// parseInventory lee el JSON del inventario sin romperse con basura.
func parseInventory(raw string) map[string]int64 {
	inv := map[string]int64{}
	if raw == "" {
		return inv
	}
	var tmp map[string]any
	if err := json.Unmarshal([]byte(raw), &tmp); err != nil {
		return inv
	}
	for k, v := range tmp {
		if !ValidItem(k) {
			continue // claves que no son del catálogo se descartan
		}
		switch n := v.(type) {
		case float64:
			if n > 0 {
				inv[k] = int64(min64(int64(n), MaxItemCount))
			}
		case bool:
			if n {
				inv[k] = 1 // el JS guarda a veces "true" en vez de cantidad
			}
		}
	}
	return inv
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
