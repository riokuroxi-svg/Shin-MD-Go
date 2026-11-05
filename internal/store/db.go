// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// db.go — Migraciones versionadas simples (mismo espíritu que
// src/storage/migrations.js) y consultas de la economía.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite" // driver "sqlite" en Go puro, sin cgo
)

type Store struct{ db *sql.DB }

// migraciones: cada una es un escalón y puede llevar VARIAS sentencias (una
// por elemento). Nunca se edita una ya publicada, se añade otra al final
// (igual que en Shin-MD JS). El número de escalón es la posición + 1, así que
// añadir al final nunca cambia lo que ya está aplicado.
var migrations = [][]string{
	{`CREATE TABLE IF NOT EXISTS users (
		jid        TEXT PRIMARY KEY,
		coins      INTEGER NOT NULL DEFAULT 0,
		bank       INTEGER NOT NULL DEFAULT 0,
		xp         INTEGER NOT NULL DEFAULT 0,
		level      INTEGER NOT NULL DEFAULT 1,
		created_at TEXT NOT NULL
	);`},
	{`CREATE TABLE IF NOT EXISTS settings (
		jid  TEXT PRIMARY KEY,
		json TEXT NOT NULL DEFAULT '{}'
	);`},
	{`CREATE TABLE IF NOT EXISTS warnings (
		jid TEXT NOT NULL,
		group_jid TEXT NOT NULL,
		count INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (jid, group_jid)
	);`},
	// 4) .daily tiene que ser "una vez al día" de verdad. Se guarda el día en
	// una tabla propia (antes se intentaba con una clave por fecha en
	// settings, que solo crecía y no impedía reclamar dos veces).
	{`CREATE TABLE IF NOT EXISTS daily_claims (
		jid TEXT PRIMARY KEY,
		day TEXT NOT NULL
	);`},
	// 5) Identidades: LID (el identificador interno de WhatsApp) ↔ teléfono.
	// WhatsApp puede mandar al mismo usuario como 521…@s.whatsapp.net o como
	// 123…@lid. Sin esta tabla, la misma persona tendría dos cuentas de monedas
	// según por dónde llegue el mensaje.
	{`CREATE TABLE IF NOT EXISTS identities (
		lid TEXT PRIMARY KEY,
		pn  TEXT NOT NULL
	);`,
		`CREATE INDEX IF NOT EXISTS identities_pn ON identities(pn);`},
	// 6) Economía POR CHAT, igual que el bot de JS (tabla g_chat_users): los
	// mismos usuarios pueden tener saldos distintos en cada grupo, y así la
	// migración de datos es 1:1 en vez de un invento.
	{`CREATE TABLE IF NOT EXISTS chat_users (
		chat_id      TEXT    NOT NULL,
		user_id      TEXT    NOT NULL,
		coins        INTEGER NOT NULL DEFAULT 0,
		bank         INTEGER NOT NULL DEFAULT 0,
		health       INTEGER NOT NULL DEFAULT 100,
		stamina      INTEGER NOT NULL DEFAULT 100,
		magic        INTEGER NOT NULL DEFAULT 100,
		inventory    TEXT    NOT NULL DEFAULT '{}',
		last_work    INTEGER NOT NULL DEFAULT 0,
		last_daily   INTEGER NOT NULL DEFAULT 0,
		last_weekly  INTEGER NOT NULL DEFAULT 0,
		last_monthly INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (chat_id, user_id)
	);`,
		`CREATE INDEX IF NOT EXISTS chat_users_board ON chat_users(chat_id, coins);`},
	// 7) Rachas globales (una por persona, no por chat) y campos de perfil
	// que en el JS viven en g_users.
	{`ALTER TABLE users ADD COLUMN streak INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE users ADD COLUMN last_daily_global INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE users ADD COLUMN weekly_streak INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE users ADD COLUMN last_weekly_global INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE users ADD COLUMN monthly_streak INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE users ADD COLUMN last_monthly_global INTEGER NOT NULL DEFAULT 0;`},
	// 8) Marca de "ya se sembró la economía global en chat_users": evita
	// duplicar monedas cuando alguien escribe por primera vez en un grupo.
	{`ALTER TABLE users ADD COLUMN economy_seeded INTEGER NOT NULL DEFAULT 0;`},
	// 9) Gacha: personajes reclamados y rolls pendientes.
	//
	// El JS guarda las pertenencias en tres sitios a la vez (tabla
	// `characters`, tabla `user_characters` y un JSON dentro de
	// `g_chat_users.characters` que a veces es array y a veces texto) y los
	// rolls pendientes en el JSON del chat, sin caducidad ni limpieza. Aquí:
	// una tabla con clave (chat, personaje) — un personaje solo puede tener un
	// dueño por chat, así que dos reclamos a la vez no pueden ganar los dos — y
	// otra para los rolls, con su mensaje, su reserva y su caducidad.
	{`CREATE TABLE IF NOT EXISTS gacha_owned (
		chat_id    TEXT    NOT NULL,
		char_id    TEXT    NOT NULL,
		owner_id   TEXT    NOT NULL,
		name       TEXT    NOT NULL DEFAULT '',
		value      INTEGER NOT NULL DEFAULT 0,
		favorite   INTEGER NOT NULL DEFAULT 0,
		claimed_at INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (chat_id, char_id)
	);`,
		`CREATE INDEX IF NOT EXISTS gacha_owned_harem ON gacha_owned(chat_id, owner_id, value);`,
		`CREATE TABLE IF NOT EXISTS gacha_rolls (
		chat_id     TEXT    NOT NULL,
		message_id  TEXT    NOT NULL,
		char_id     TEXT    NOT NULL,
		name        TEXT    NOT NULL DEFAULT '',
		value       INTEGER NOT NULL DEFAULT 0,
		roller_id   TEXT    NOT NULL DEFAULT '',
		reserved_to INTEGER NOT NULL DEFAULT 0,
		expires_at  INTEGER NOT NULL DEFAULT 0,
		claimed_by  TEXT,
		claimed_at  INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (chat_id, message_id)
	);`,
		`CREATE INDEX IF NOT EXISTS gacha_rolls_vigencia ON gacha_rolls(expires_at);`},
	// 10) Contador de rolls por persona y chat: el JS usaba el enfriamiento
	// `lastRoll`, guardado como número absoluto dentro del JSON del usuario.
	{`ALTER TABLE chat_users ADD COLUMN last_roll INTEGER NOT NULL DEFAULT 0;`,
		`ALTER TABLE chat_users ADD COLUMN last_claim INTEGER NOT NULL DEFAULT 0;`},
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite escribe de uno en uno: evita "database is locked"
	if err := db.Ping(); err != nil {
		return nil, err
	}

	s := &Store{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	// La DB guarda economía de usuarios: permisos 600, como en B5.1 del JS.
	_ = os.Chmod(path, 0o600)
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_version (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	var current int
	_ = s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0) FROM schema_version`).Scan(&current)

	for i := current; i < len(migrations); i++ {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for n, sentencia := range migrations[i] {
			if _, err = tx.ExecContext(ctx, sentencia); err != nil {
				// Un ALTER sobre una base a la que ya se le añadió la columna
				// (por ejemplo al restaurar una copia) no debe romper el
				// arranque: se ignora y se sigue con la sentencia siguiente,
				// sin deshacer la transacción.
				if isDuplicateColumn(err) {
					continue
				}
				_ = tx.Rollback()
				return fmt.Errorf("migración %d, sentencia %d: %w", i+1, n+1, err)
			}
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_version(version, applied_at) VALUES(?,?)`, i+1, time.Now().UTC().Format(time.RFC3339)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// isDuplicateColumn detecta el error de SQLite al añadir una columna que ya
// existe ("duplicate column name: x").
func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "duplicate column name")
}

// Get devuelve la cuenta, creándola si no existe (como el upsert perezoso del JS).
func (s *Store) Get(ctx context.Context, jid string) (*Account, error) {
	acc := &Account{JID: jid}
	err := s.db.QueryRowContext(ctx, `SELECT coins, bank, xp, level, created_at FROM users WHERE jid = ?`, jid).
		Scan(&acc.Coins, &acc.Bank, &acc.XP, &acc.Level, &acc.CreatedAt)
	if err == nil {
		return acc, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}

	// Bug que cazó la prueba: en JS el default era level 1, en Go el valor
	// cero del struct es 0. Se fija a mano al crear la cuenta.
	if acc.Level == 0 {
		acc.Level = 1
	}
	acc.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO users(jid, coins, bank, xp, level, created_at) VALUES(?,?,?,?,?,?)`,
		jid, acc.Coins, acc.Bank, acc.XP, acc.Level, acc.CreatedAt)
	return acc, err
}

// AddGlobalCoins suma (o resta, con delta negativo) en una sola sentencia.
// Es la cuenta global heredada de la 0.3.0: la economía de verdad vive ahora
// por chat (ver economia.go) y esto solo se usa para lo que no es dinero de
// juego (permisos, y la siembra del saldo antiguo).
func (s *Store) AddGlobalCoins(ctx context.Context, jid string, delta int64) (int64, error) {
	if _, err := s.Get(ctx, jid); err != nil {
		return 0, err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE users SET coins = coins + ? WHERE jid = ?`, delta, jid); err != nil {
		return 0, err
	}
	var coins int64
	err := s.db.QueryRowContext(ctx, `SELECT coins FROM users WHERE jid = ?`, jid).Scan(&coins)
	return coins, err
}

// AddXP sube experiencia y calcula nivel (curva sencilla: nivel*100 por tramo).
func (s *Store) AddXP(ctx context.Context, jid string, delta int64) (*Account, error) {
	acc, err := s.Get(ctx, jid)
	if err != nil {
		return nil, err
	}
	acc.XP += delta
	if acc.XP < 0 { // un coste mayor que la XP acumulada no debe dejar el nivel en negativo
		acc.XP = 0
	}
	level := 1
	for xp := acc.XP; xp >= int64(level*100); xp -= int64(level * 100) {
		level++
	}
	if level != acc.Level {
		acc.Level = level
	}
	_, err = s.db.ExecContext(ctx, `UPDATE users SET xp = ?, level = ? WHERE jid = ?`, acc.XP, acc.Level, jid)
	return acc, err
}

// validKey acepta solo claves simples (letras, números, guion y guion bajo).
// La clave entra en la ruta JSON de SQLite: si se dejara pasar una comilla o
// un punto, se podría reescribir el JSON entero (o romper la consulta).
func validKey(key string) bool {
	if key == "" || len(key) > 48 {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}

// SetSetting guarda un valor de texto para un JID (self mode, antilink, etc.),
// igual que el JS. El valor se guarda como string JSON.
func (s *Store) SetSetting(ctx context.Context, jid, key, value string) error {
	if !validKey(key) {
		return fmt.Errorf("clave de setting inválida: %q", key)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO settings(jid, json) VALUES(?, json_set('{}', ?, ?))
		ON CONFLICT(jid) DO UPDATE SET json = json_set(settings.json, ?, ?)`,
		jid, "$."+key, value, "$."+key, value)
	return err
}

// GetSetting lee un valor de texto. ok=false si no existe.
func (s *Store) GetSetting(ctx context.Context, jid, key string) (value string, ok bool, err error) {
	if !validKey(key) {
		return "", false, fmt.Errorf("clave de setting inválida: %q", key)
	}
	var raw sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT json_extract(json, ?) FROM settings WHERE jid = ?`, "$."+key, jid).Scan(&raw)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if !raw.Valid || raw.String == "" {
		return "", false, nil
	}
	return raw.String, true, nil
}

// SystemJID identifica los ajustes del propio bot (no de un usuario).
const SystemJID = "__shin__"

const keyWarmupAnchor = "warmup_anchor"

// WarmupAnchor devuelve el primer día del calentamiento y lo crea la primera
// vez. Antes el ancla vivía en memoria y se reiniciaba en cada arranque, así
// que el número nunca pasaba de 20 mensajes/día: el warm-up no avanzaba jamás.
func (s *Store) WarmupAnchor(ctx context.Context, now time.Time) (time.Time, error) {
	if v, ok, err := s.GetSetting(ctx, SystemJID, keyWarmupAnchor); err != nil {
		return now.UTC(), err
	} else if ok {
		if t, err := time.Parse("2006-01-02", v); err == nil {
			return t.UTC(), nil
		}
	}
	day := now.UTC().Format("2006-01-02")
	if err := s.SetSetting(ctx, SystemJID, keyWarmupAnchor, day); err != nil {
		return now.UTC(), err
	}
	t, err := time.Parse("2006-01-02", day)
	return t.UTC(), err
}

// ClaimDaily marca el día como reclamado y dice si ESTA llamada fue la que lo
// reclamó. La comprobación y la escritura son una sola sentencia, así que dos
// mensajes .daily seguidos no pueden dar premio dos veces (antes se sumaban
// monedas sin límite: no había ninguna comprobación, solo un comentario).
func (s *Store) ClaimDaily(ctx context.Context, jid, day string) (claimed bool, err error) {
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO daily_claims(jid, day) VALUES(?, ?)
		ON CONFLICT(jid) DO UPDATE SET day = excluded.day
		WHERE daily_claims.day <> excluded.day`, jid, day)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// LastDaily devuelve el último día reclamado ("" si nunca).
func (s *Store) LastDaily(ctx context.Context, jid string) (string, error) {
	var day string
	err := s.db.QueryRowContext(ctx, `SELECT day FROM daily_claims WHERE jid = ?`, jid).Scan(&day)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return day, err
}

// ─── Identidades LID ↔ teléfono ──────────────────────────────────────────────
//
// En WhatsApp moderno un mismo usuario puede aparecer como dos JID distintos:
//
//	5215512345678@s.whatsapp.net   (PN, el teléfono)
//	123456789012345@lid            (LID, identificador interno, sin teléfono)
//
// El bot tiene que tratarlos como la MISMA persona. La red solo resuelve
// PN → LID (nunca al revés para un LID desconocido), así que las parejas se
// guardan aquí en cuanto aparecen en un mensaje o en el grupo.

// PutIdentity guarda una pareja LID → teléfono. Cadena vacía en cualquiera de
// los dos no se guarda (no hay nada que aprender).
//
// Además FUSIONA las cuentas: si alguien escribió mientras solo se conocía su
// LID, sus monedas quedaron en una fila con llave @lid. Al aprender el
// teléfono se mueven a la llave buena (o se suman si ya existían las dos), así
// nadie pierde economía por culpa del cambio de identificadores de WhatsApp.
func (s *Store) PutIdentity(ctx context.Context, lid, pn string) error {
	if lid == "" || pn == "" || lid == pn {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO identities(lid, pn) VALUES(?, ?)
		ON CONFLICT(lid) DO UPDATE SET pn = excluded.pn`, lid, pn); err != nil {
		return err
	}
	return s.mergeAccounts(ctx, lid, pn)
}

// mergeAccounts mueve (o suma) la cuenta del LID a la del teléfono, y con
// ella la marca del daily (para que no pueda reclamarlo otra vez).
func (s *Store) mergeAccounts(ctx context.Context, lid, pn string) error {
	var lidExists, pnExists bool
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE jid = ?)`, lid).Scan(&lidExists); err != nil {
		return err
	}
	if !lidExists {
		return nil
	}
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE jid = ?)`, pn).Scan(&pnExists); err != nil {
		return err
	}

	if pnExists {
		// Existen las dos: se suman en la del teléfono y se borra la del LID.
		if _, err := s.db.ExecContext(ctx, `
			UPDATE users
			   SET coins = coins + (SELECT coins FROM users WHERE jid = ?),
			       bank  = bank  + (SELECT bank  FROM users WHERE jid = ?),
			       xp    = xp    + (SELECT xp    FROM users WHERE jid = ?)
			 WHERE jid = ?`, lid, lid, lid, pn); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE jid = ?`, lid); err != nil {
			return err
		}
	} else if _, err := s.db.ExecContext(ctx, `UPDATE users SET jid = ? WHERE jid = ?`, pn, lid); err != nil {
		return err
	}

	return s.mergeDaily(ctx, lid, pn)
}

// mergeDaily traspasa la marca del daily (y la borra del LID).
func (s *Store) mergeDaily(ctx context.Context, lid, pn string) error {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO daily_claims(jid, day) SELECT ?, day FROM daily_claims WHERE jid = ?
		ON CONFLICT(jid) DO UPDATE SET day = MAX(daily_claims.day, excluded.day)`, pn, lid); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM daily_claims WHERE jid = ?`, lid)
	return err
}

// PNForLID devuelve el teléfono de un LID ("" si no se conoce todavía).
func (s *Store) PNForLID(ctx context.Context, lid string) (string, error) {
	var pn string
	err := s.db.QueryRowContext(ctx, `SELECT pn FROM identities WHERE lid = ?`, lid).Scan(&pn)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return pn, err
}

// LIDForPN es la dirección contraria: puede haber varias filas con el mismo
// teléfono si WhatsApp cambió el LID, así que se devuelve la más reciente.
func (s *Store) LIDForPN(ctx context.Context, pn string) (string, error) {
	var lid string
	err := s.db.QueryRowContext(ctx, `SELECT lid FROM identities WHERE pn = ? ORDER BY rowid DESC LIMIT 1`, pn).Scan(&lid)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return lid, err
}

// CountIdentities: cuántas parejas se conocen (para /health).
func (s *Store) CountIdentities(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM identities`).Scan(&n)
	return n, err
}

// CountUsers: útil para /health del panel web.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}
