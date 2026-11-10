// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// perfil.go — Los datos de la persona: nombre, descripción, género, pasatiempo,
// cumpleaños, pareja y modo ausente.
//
// POR QUÉ NO VA EN chat_users
// En el bot de JS esto vive en g_users, la tabla global: alguien casado en un
// grupo está casado en todos, y su cumpleaños no cambia según dónde escriba.
// Eso se mantiene igual aquí a propósito — cambiar dónde vive un dato es la
// forma más rápida de que una base migrada empiece a mentir.
//
// LO QUE SÍ CAMBIA: cada campo tiene una lista blanca de nombres. En el JS se
// construía la sentencia con el nombre del campo que llegaba del comando
// (`db.setUser(id, campo, valor)`); aquí un campo desconocido se rechaza con
// error en vez de acabar dentro del SQL.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Campos que se pueden escribir con SetPerfil. Cualquier otro nombre se
// rechaza: la lista es también la documentación de qué es editable.
const (
	CampoNombre      = "name"
	CampoDescripcion = "description"
	CampoGenero      = "genre"
	CampoPasatiempo  = "hobby"
	CampoCumple      = "birth"
	CampoAFK         = "afk"
	CampoAFKRazon    = "afk_reason"
)

var camposPerfil = map[string]bool{
	CampoNombre:      true,
	CampoDescripcion: true,
	CampoGenero:      true,
	CampoPasatiempo:  true,
	CampoCumple:      true,
	CampoAFK:         true,
	CampoAFKRazon:    true,
}

// Topes de longitud. No son un capricho de estilo: 1000 caracteres de
// descripción caben en un mensaje sin acercarse al límite de WhatsApp, y un
// cumpleaños de 40 letras ya no es un cumpleaños.
const (
	MaxNombre     = 64
	MaxDesc       = 200
	MaxGenero     = 24
	MaxPasatiempo = 40
	MaxCumple     = 40
	MaxPareja     = 64
)

// Perfil es la ficha de una persona.
type Perfil struct {
	JID         string
	Nombre      string
	Descripcion string
	Genero      string
	Pasatiempo  string
	Cumple      string
	Pareja      string // JID de la pareja; vacío = soltero/a
	AFK         bool
	AFKRazon    string

	XP           int64
	Nivel        int
	Comandos     int64
	CreadoEn     string
	ParejaNombre string // relleno por PerfilDe cuando la pareja tiene nombre
}

// ErrCampoInvalido: el nombre del campo no está en la lista blanca.
var ErrCampoInvalido = errors.New("campo de perfil no válido")

// ErrNoCasado: se pide divorcio de alguien que no está casado.
var ErrNoCasado = errors.New("no está casado")

// ErrYaCasado: la persona o su pareja ya tienen pareja.
var ErrYaCasado = errors.New("ya está casado")

// limpiarCampo aplana el texto y lo recorta. Reutiliza `recortar` (el mismo que
// usan los avisos de grupo) para no tener dos formas de cortar texto.
func limpiarCampo(s string, max int) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	return recortar(s, max)
}

// PerfilDe devuelve la ficha, creando la cuenta si es la primera vez (igual
// que el upsert perezoso del resto del almacén).
func (s *Store) PerfilDe(ctx context.Context, jid string) (*Perfil, error) {
	if jid == "" {
		return nil, fmt.Errorf("perfil: jid vacío")
	}
	if _, err := s.Get(ctx, jid); err != nil { // crea la fila si no existe
		return nil, err
	}
	p := &Perfil{JID: jid}
	err := s.db.QueryRowContext(ctx, `
		SELECT name, description, genre, hobby, birth, marry, afk, afk_reason,
		       xp, level, used_commands, created_at
		  FROM users WHERE jid = ?`, jid).
		Scan(&p.Nombre, &p.Descripcion, &p.Genero, &p.Pasatiempo, &p.Cumple,
			&p.Pareja, &p.AFK, &p.AFKRazon, &p.XP, &p.Nivel, &p.Comandos, &p.CreadoEn)
	if err != nil {
		return nil, err
	}
	if p.Pareja != "" {
		var nombre sql.NullString
		_ = s.db.QueryRowContext(ctx, `SELECT name FROM users WHERE jid = ?`, p.Pareja).Scan(&nombre)
		if nombre.Valid {
			p.ParejaNombre = nombre.String
		}
	}
	return p, nil
}

// PerfilExistente es como PerfilDe pero NO crea la fila: devuelve ok=false si
// esa persona nunca le ha escrito al bot.
//
// Existe por una razón concreta: consultar el perfil de alguien a quien se
// menciona no puede ir sembrando filas en `users`. Un grupo donde se escribe
// «.profile @» con JIDs que nunca usaron el bot acabaría con cuentas fantasma
// en la base (y en `.info`, que cuenta usuarios).
func (s *Store) PerfilExistente(ctx context.Context, jid string) (*Perfil, bool, error) {
	if jid == "" {
		return nil, false, fmt.Errorf("perfil: jid vacío")
	}
	p := &Perfil{JID: jid}
	err := s.db.QueryRowContext(ctx, `
		SELECT name, description, genre, hobby, birth, marry, afk, afk_reason,
		       xp, level, used_commands, created_at
		  FROM users WHERE jid = ?`, jid).
		Scan(&p.Nombre, &p.Descripcion, &p.Genero, &p.Pasatiempo, &p.Cumple,
			&p.Pareja, &p.AFK, &p.AFKRazon, &p.XP, &p.Nivel, &p.Comandos, &p.CreadoEn)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if p.Pareja != "" {
		var nombre sql.NullString
		_ = s.db.QueryRowContext(ctx, `SELECT name FROM users WHERE jid = ?`, p.Pareja).Scan(&nombre)
		if nombre.Valid {
			p.ParejaNombre = nombre.String
		}
	}
	return p, true, nil
}

// SetPerfil escribe un campo editable. Devuelve lo que quedó guardado (ya
// recortado), porque el mensaje de confirmación del bot debe enseñar lo mismo
// que la base y no la versión de 4000 caracteres que llegó.
func (s *Store) SetPerfil(ctx context.Context, jid, campo, valor string) (string, error) {
	if !camposPerfil[campo] {
		return "", fmt.Errorf("%w: %q", ErrCampoInvalido, campo)
	}
	if _, err := s.Get(ctx, jid); err != nil {
		return "", err
	}
	if campo == CampoAFK {
		// El AFK se guarda como número (0/1) para que la ficha no dependa de
		// cómo se escribió el texto.
		v := 0
		if valor == "1" || strings.EqualFold(valor, "true") {
			v = 1
		}
		_, err := s.db.ExecContext(ctx, `UPDATE users SET afk = ? WHERE jid = ?`, v, jid)
		return fmt.Sprint(v), err
	}
	valor = limpiarCampo(valor, topeDe(campo))
	if _, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`UPDATE users SET %s = ? WHERE jid = ?`, campo), valor, jid); err != nil {
		return "", err
	}
	return valor, nil
}

// topeDe: longitud máxima por campo. Se centraliza para que el comando no
// tenga que repetir los números.
func topeDe(campo string) int {
	switch campo {
	case CampoNombre:
		return MaxNombre
	case CampoDescripcion:
		return MaxDesc
	case CampoGenero:
		return MaxGenero
	case CampoPasatiempo:
		return MaxPasatiempo
	case CampoCumple:
		return MaxCumple
	default:
		return MaxCumple
	}
}

// LimpiarPerfil vacía un campo editable (los .deldesc, .delgenre… del bot).
func (s *Store) LimpiarPerfil(ctx context.Context, jid, campo string) error {
	if !camposPerfil[campo] {
		return fmt.Errorf("%w: %q", ErrCampoInvalido, campo)
	}
	if _, err := s.Get(ctx, jid); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`UPDATE users SET %s = '' WHERE jid = ?`, campo), jid)
	return err
}

// SumarComando cuenta un comando usado (el `.profile` del JS enseña el total).
func (s *Store) SumarComando(ctx context.Context, jid string) error {
	if _, err := s.Get(ctx, jid); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET used_commands = used_commands + 1 WHERE jid = ?`, jid)
	return err
}

// Casarse empareja a dos personas. Se hace en una transacción: sin ella, un
// fallo a medias dejaría a una casada con la otra y a la otra soltera, y eso no
// se arregla sin tocar la base a mano.
func (s *Store) Casarse(ctx context.Context, a, b string) error {
	if a == b {
		return errors.New("no puedes casarte contigo")
	}
	for _, jid := range []string{a, b} {
		if _, err := s.Get(ctx, jid); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, jid := range []string{a, b} {
		var pareja string
		if err := tx.QueryRowContext(ctx, `SELECT marry FROM users WHERE jid = ?`, jid).Scan(&pareja); err != nil {
			return err
		}
		if pareja != "" {
			return ErrYaCasado
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET marry = CASE jid WHEN ? THEN ? WHEN ? THEN ? END WHERE jid IN (?, ?)`,
		a, b, b, a, a, b); err != nil {
		return err
	}
	return tx.Commit()
}

// Divorciarse deshace el matrimonio de los dos lados y devuelve el ex.
func (s *Store) Divorciarse(ctx context.Context, jid string) (string, error) {
	p, err := s.PerfilDe(ctx, jid)
	if err != nil {
		return "", err
	}
	if p.Pareja == "" {
		return "", ErrNoCasado
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `UPDATE users SET marry = '' WHERE jid IN (?, ?)`, jid, p.Pareja); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return p.Pareja, nil
}

// FilaRanking es una línea de la tabla de niveles.
type FilaRanking struct {
	JID    string
	Nombre string
	XP     int64
	Nivel  int
}

// TopPorXP devuelve los que más experiencia tienen. Solo cuenta a quien tiene
// algo de XP: una base recién creada tiene a todo el mundo con 0 y la tabla
// saldría con veinte ceros.
func (s *Store) TopPorXP(ctx context.Context, limite int) ([]FilaRanking, error) {
	if limite <= 0 || limite > 50 {
		limite = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT jid, name, xp, level FROM users
		 WHERE xp > 0
		 ORDER BY xp DESC, jid ASC
		 LIMIT ?`, limite)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	out := make([]FilaRanking, 0, limite)
	for rows.Next() {
		var f FilaRanking
		if err := rows.Scan(&f.JID, &f.Nombre, &f.XP, &f.Nivel); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// Posicion devuelve el puesto por experiencia (1 = el que más tiene). Si la
// persona no tiene XP, devuelve 0: no está en la tabla, y decir «puesto 47» a
// quien tiene cero es mentir con un número.
func (s *Store) Posicion(ctx context.Context, jid string) (int, error) {
	var xp int64
	if err := s.db.QueryRowContext(ctx, `SELECT xp FROM users WHERE jid = ?`, jid).Scan(&xp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	if xp <= 0 {
		return 0, nil
	}
	var puesto int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) + 1 FROM users WHERE xp > ?`, xp).Scan(&puesto)
	return puesto, err
}

// AFKDe: si la persona está ausente y por qué. Lo consulta el hook que avisa
// cuando alguien menciona a un ausente.
func (s *Store) AFKDe(ctx context.Context, jid string) (ausente bool, razon string, desde time.Time, err error) {
	var afk int
	var ms int64
	err = s.db.QueryRowContext(ctx,
		`SELECT afk, afk_reason, COALESCE((SELECT 0), 0) FROM users WHERE jid = ?`, jid).
		Scan(&afk, &razon, &ms)
	if errors.Is(err, sql.ErrNoRows) {
		return false, "", time.Time{}, nil
	}
	if err != nil {
		return false, "", time.Time{}, err
	}
	return afk == 1, razon, time.Time{}, nil
}
