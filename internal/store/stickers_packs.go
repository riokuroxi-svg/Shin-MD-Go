// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// stickers_packs.go — los paquetes de stickers que el JS guardaba en RAM
// (con pérdida al reiniciar). Aquí en SQLite: cada uno con nombre, dueño,
// descripción, público/privado y sus stickers (webp) en tabla aparte.
package store

import (
	"context"
	"database/sql"
	"time"
)

// Tope del JS: 30 stickers por paquete y ~512 KB por sticker (más es una
// foto, no un sticker).
const (
	MaxStickersPack = 30
	MaxStickerBytes = 512 << 10
)

func (s *Store) ensureStickerPacks(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS sticker_packs (
		  owner_id   TEXT NOT NULL,
		  name       TEXT NOT NULL,
		  descr      TEXT NOT NULL DEFAULT '',
		  public     INTEGER NOT NULL DEFAULT 1,
		  created_at INTEGER NOT NULL,
		  PRIMARY KEY (owner_id, name)
		);
		CREATE TABLE IF NOT EXISTS sticker_pack_items (
		  owner_id TEXT NOT NULL,
		  pack     TEXT NOT NULL,
		  idx      INTEGER NOT NULL,
		  data     BLOB NOT NULL,
		  PRIMARY KEY (owner_id, pack, idx)
		);
		CREATE INDEX IF NOT EXISTS ix_sticker_pack_pub ON sticker_packs(public);`)
	return err
}

// StickerPack es la ficha de un paquete (sin los webp).
type StickerPack struct {
	OwnerID string
	Name    string
	Descr   string
	Public  bool
	Count   int
}

// NewPack crea el paquete (devuelve created=false si ya existía).
func (s *Store) NewPack(ctx context.Context, owner, name string) (bool, error) {
	if err := s.ensureStickerPacks(ctx); err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO sticker_packs(owner_id, name, created_at) VALUES(?,?,?)`,
		owner, name, time.Now().Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// DelPack tira el paquete ENTERO del dueño.
func (s *Store) DelPack(ctx context.Context, owner, name string) (bool, error) {
	if err := s.ensureStickerPacks(ctx); err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM sticker_packs WHERE owner_id = ? AND name = ?`, owner, name)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	_, _ = s.db.ExecContext(ctx, `DELETE FROM sticker_pack_items WHERE owner_id = ? AND pack = ?`, owner, name)
	return n > 0, nil
}

// AddSticker guarda el webp en el pack y devuelve su posición (1..30).
// Devuelve Err lógico como 0 cuando el pack no existe o está lleno (el
// comando lo traduce).
func (s *Store) AddSticker(ctx context.Context, owner, name string, webp []byte) (int, error) {
	if err := s.ensureStickerPacks(ctx); err != nil {
		return 0, err
	}
	if len(webp) > MaxStickerBytes {
		return 0, nil // no es un sticker: es una foto. Lleno imposible.
	}
	var count int
	var packCount int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sticker_packs WHERE owner_id = ? AND name = ?`, owner, name).Scan(&count); err != nil {
		return 0, err
	}
	if count == 0 {
		return 0, nil
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sticker_pack_items WHERE owner_id = ? AND pack = ?`, owner, name).Scan(&packCount); err != nil {
		return 0, err
	}
	if packCount >= MaxStickersPack {
		return 0, nil
	}
	idx := packCount + 1
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO sticker_pack_items(owner_id, pack, idx, data) VALUES(?,?,?,?)`,
		owner, name, idx, webp); err != nil {
		return 0, err
	}
	return idx, nil
}

// Stickers devuelve los webp del pack en orden.
func (s *Store) Stickers(ctx context.Context, owner, name string) ([][]byte, error) {
	if err := s.ensureStickerPacks(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT data FROM sticker_pack_items WHERE owner_id = ? AND pack = ? ORDER BY idx`, owner, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var d []byte
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DelSticker borra el #idx del pack y recoloca la numeración.
func (s *Store) DelSticker(ctx context.Context, owner, name string, idx int) (bool, error) {
	if err := s.ensureStickerPacks(ctx); err != nil {
		return false, err
	}
	res, err := s.db.ExecContext(ctx, `
		DELETE FROM sticker_pack_items WHERE owner_id = ? AND pack = ? AND idx = ?`,
		owner, name, idx)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return false, nil
	}
	// Recolocar: más abajo que idx bajan uno (sin tocar los de arriba).
	_, _ = s.db.ExecContext(ctx, `
		UPDATE sticker_pack_items SET idx = idx - 1
		 WHERE owner_id = ? AND pack = ? AND idx > ?`, owner, name, idx)
	return true, nil
}

// Packs: los paquetes de una persona (o los públicos si owner == "").
func (s *Store) Packs(ctx context.Context, owner string, soloPublicos bool) ([]StickerPack, error) {
	if err := s.ensureStickerPacks(ctx); err != nil {
		return nil, err
	}
	var rows *sql.Rows
	var err error
	if owner == "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT owner_id, name, descr, public,
			       (SELECT COUNT(*) FROM sticker_pack_items i WHERE i.owner_id = p.owner_id AND i.pack = p.name)
			  FROM sticker_packs p ORDER BY created_at DESC`)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT owner_id, name, descr, public,
			       (SELECT COUNT(*) FROM sticker_pack_items i WHERE i.owner_id = p.owner_id AND i.pack = p.name)
			  FROM sticker_packs p WHERE owner_id = ? ORDER BY created_at DESC`, owner)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StickerPack
	for rows.Next() {
		var p StickerPack
		if err := rows.Scan(&p.OwnerID, &p.Name, &p.Descr, &p.Public, &p.Count); err != nil {
			return nil, err
		}
		if soloPublicos && !p.Public {
			continue
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// PackByName busca «nombre» o «nombre@dueño».
func (s *Store) PackByName(ctx context.Context, name, miPropietario string) (*StickerPack, error) {
	if err := s.ensureStickerPacks(ctx); err != nil {
		return nil, err
	}
	// nombre@dueño exacto
	var p StickerPack
	err := s.db.QueryRowContext(ctx, `
		SELECT p.owner_id, p.name, p.descr, p.public,
		       (SELECT COUNT(*) FROM sticker_pack_items i WHERE i.owner_id = p.owner_id AND i.pack = p.name)
		  FROM sticker_packs p WHERE p.owner_id = ? AND p.name = ?`,
		miPropietario, name).Scan(&p.OwnerID, &p.Name, &p.Descr, &p.Public, &p.Count)
	if err == sql.ErrNoRows {
		// buscar por nombre solo entre los públicos
		err = s.db.QueryRowContext(ctx, `
			SELECT p.owner_id, p.name, p.descr, p.public,
			       (SELECT COUNT(*) FROM sticker_pack_items i WHERE i.owner_id = p.owner_id AND i.pack = p.name)
			  FROM sticker_packs p WHERE p.name = ? AND p.public = 1 LIMIT 1`,
			name).Scan(&p.OwnerID, &p.Name, &p.Descr, &p.Public, &p.Count)
	}
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// SetPackMeta cambia descripción o público/privado.
func (s *Store) SetPackMeta(ctx context.Context, owner, name, descr string, public *bool) (bool, error) {
	if err := s.ensureStickerPacks(ctx); err != nil {
		return false, err
	}
	if descr != "" {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE sticker_packs SET descr = ? WHERE owner_id = ? AND name = ?`, descr, owner, name); err != nil {
			return false, err
		}
	}
	if public != nil {
		v := 0
		if *public {
			v = 1
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE sticker_packs SET public = ? WHERE owner_id = ? AND name = ?`, v, owner, name); err != nil {
			return false, err
		}
	}
	return true, nil
}
