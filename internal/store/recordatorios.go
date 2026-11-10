// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// recordatorios.go — los `.recordar` guardados en la base.
//
// POR QUÉ EN LA BASE Y NO EN MEMORIA
// En el JS los recordatorios eran `setTimeout` en un array global: si el bot se
// reiniciaba (y se reiniciaba), el recordatorio desaparecía sin avisar a nadie.
// Aquí se guardan como filas y un vigilante los revisa cada 20 s, así que
// sobreviven a un reinicio y a una caída.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Recordatorio es un aviso programado para un chat.
type Recordatorio struct {
	ID     int64
	ChatID string
	UserID string
	Texto  string
	Cuando time.Time
	Creado time.Time
}

// PonRecordatorio guarda uno nuevo y devuelve su id.
//
// El tope de 24 h es el mismo del JS: un recordatorio para dentro de un mes no
// aporta nada y se queda en la base para siempre.
func (s *Store) PonRecordatorio(ctx context.Context, chatID, userID, texto string, cuando time.Time) (int64, error) {
	if chatID == "" || userID == "" {
		return 0, fmt.Errorf("recordatorio: chat o usuario vacío")
	}
	if texto == "" {
		return 0, fmt.Errorf("recordatorio: texto vacío")
	}
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO recordatorios (chat_id, user_id, texto, cuando_ms, creado_ms)
		VALUES (?, ?, ?, ?, ?)`,
		chatID, userID, texto, cuando.UnixMilli(), time.Now().UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// RecordatoriosVencidos devuelve los que ya toca avisar (y los borra de la
// base en la misma transacción: si se avisa y no se borra, se avisa dos veces).
func (s *Store) RecordatoriosVencidos(ctx context.Context, ahora time.Time) ([]Recordatorio, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	filas, err := tx.QueryContext(ctx, `
		SELECT id, chat_id, user_id, texto, cuando_ms, creado_ms
		  FROM recordatorios WHERE cuando_ms <= ? ORDER BY cuando_ms LIMIT 50`,
		ahora.UnixMilli())
	if err != nil {
		return nil, err
	}
	var salida []Recordatorio
	var ids []int64
	for filas.Next() {
		var r Recordatorio
		var cuando, creado int64
		if err := filas.Scan(&r.ID, &r.ChatID, &r.UserID, &r.Texto, &cuando, &creado); err != nil {
			_ = filas.Close()
			return nil, err
		}
		r.Cuando = time.UnixMilli(cuando)
		r.Creado = time.UnixMilli(creado)
		salida = append(salida, r)
		ids = append(ids, r.ID)
	}
	if err := filas.Close(); err != nil {
		return nil, err
	}
	if err := filas.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `DELETE FROM recordatorios WHERE id = ?`, id); err != nil {
			return nil, err
		}
	}
	return salida, tx.Commit()
}

// RecordatoriosDe lista los pendientes de una persona en un chat.
func (s *Store) RecordatoriosDe(ctx context.Context, chatID, userID string) ([]Recordatorio, error) {
	filas, err := s.db.QueryContext(ctx, `
		SELECT id, chat_id, user_id, texto, cuando_ms, creado_ms
		  FROM recordatorios WHERE chat_id = ? AND user_id = ? ORDER BY cuando_ms`,
		chatID, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = filas.Close() }()

	var salida []Recordatorio
	for filas.Next() {
		var r Recordatorio
		var cuando, creado int64
		if err := filas.Scan(&r.ID, &r.ChatID, &r.UserID, &r.Texto, &cuando, &creado); err != nil {
			return nil, err
		}
		r.Cuando = time.UnixMilli(cuando)
		r.Creado = time.UnixMilli(creado)
		salida = append(salida, r)
	}
	return salida, filas.Err()
}

// BorrarRecordatorio quita uno por id, solo si es de esa persona (para que
// nadie pueda borrar el recordatorio de otro cambiando el número).
func (s *Store) BorrarRecordatorio(ctx context.Context, id int64, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM recordatorios WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// LimpiarRecordatoriosViejos borra los que llevan más de una semana vencidos
// (por si el bot estuvo apagado tanto tiempo): avisar de algo de hace siete
// días ya no ayuda a nadie.
func (s *Store) LimpiarRecordatoriosViejos(ctx context.Context, ahora time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM recordatorios WHERE cuando_ms < ?`,
		ahora.Add(-7*24*time.Hour).UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ErrRecordatorioNoExiste lo devuelve BorrarRecordatorio cuando el id no es de
// esa persona.
var ErrRecordatorioNoExiste = errors.New("ese recordatorio no existe")
