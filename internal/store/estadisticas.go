// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// estadisticas.go — números de la base para `.health`: cuánta gente hay,
// cuántos chats, cuánto ocupa el archivo.
//
// En el JS esto se sacaba cargando el JSON entero en memoria y contando
// (`db.getUser().length`). Aquí son tres `COUNT(*)` y un `stat`: el bot puede
// tardar años en tener suficientes filas para que esto importe, pero contar
// cargando todo es la clase de cosa que un día se cae sola.
package store

import (
	"context"
	"os"
)

// Estadisticas es la foto que enseña `.health`.
type Estadisticas struct {
	Usuarios     int64 // filas en users (gente que le escribió al bot)
	Chats        int64 // chats distintos con economía (chat_users)
	Advertencias int64
	Bytes        int64 // tamaño del archivo de la base, incluido el WAL
}

// Estadisticas cuenta las filas y mide el archivo. Un error al medir el
// archivo no es un error de la base: se devuelve 0 y se sigue.
func (s *Store) Estadisticas(ctx context.Context, ruta string) (Estadisticas, error) {
	var e Estadisticas
	consultas := []struct {
		sql string
		dst *int64
	}{
		{`SELECT COUNT(*) FROM users`, &e.Usuarios},
		{`SELECT COUNT(DISTINCT chat_id) FROM chat_users`, &e.Chats},
		{`SELECT COUNT(*) FROM group_warns`, &e.Advertencias},
	}
	for _, q := range consultas {
		if err := s.db.QueryRowContext(ctx, q.sql).Scan(q.dst); err != nil {
			return e, err
		}
	}
	e.Bytes = tamanoDB(ruta)
	return e, nil
}

// tamanoDB suma el archivo y sus acompañantes (-wal, -shm): el WAL puede ser
// más grande que la base y es justo lo que se olvida al mirar el disco.
func tamanoDB(ruta string) int64 {
	if ruta == "" {
		return 0
	}
	var total int64
	for _, sufijo := range []string{"", "-wal", "-shm"} {
		if info, err := os.Stat(ruta + sufijo); err == nil {
			total += info.Size()
		}
	}
	return total
}
