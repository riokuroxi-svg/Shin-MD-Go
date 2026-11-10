// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package store

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func abrir(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	db, err := Open(filepath.Join(t.TempDir(), "shin.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, ctx
}

const (
	perfilAna   = "5215500000001@s.whatsapp.net"
	perfilBeto  = "5215500000002@s.whatsapp.net"
	perfilCarla = "5215500000003@s.whatsapp.net"
)

// El perfil empieza vacío y con nivel 1: nadie nace en «nivel 0».
func TestPerfilNuevo(t *testing.T) {
	db, ctx := abrir(t)
	p, err := db.PerfilDe(ctx, perfilAna)
	if err != nil {
		t.Fatalf("PerfilDe: %v", err)
	}
	if p.Nivel < 1 {
		t.Fatalf("un perfil nuevo debería estar en el nivel 1, salió %d", p.Nivel)
	}
	if p.Nombre != "" || p.Descripcion != "" || p.Pareja != "" || p.AFK {
		t.Fatalf("perfil nuevo con datos: %+v", p)
	}
}

// Cada campo editable se guarda y se lee de vuelta. Es el camino que usan
// .setdesc, .setgenre, .sethobby y .setbirth.
func TestCamposDelPerfil(t *testing.T) {
	db, ctx := abrir(t)
	campos := map[string]string{
		CampoNombre:      "Ana",
		CampoDescripcion: "la de los memes",
		CampoGenero:      "Mujer",
		CampoPasatiempo:  "dibujar",
		CampoCumple:      "14 de marzo",
	}
	for campo, valor := range campos {
		guardado, err := db.SetPerfil(ctx, perfilAna, campo, valor)
		if err != nil {
			t.Fatalf("SetPerfil(%s): %v", campo, err)
		}
		if guardado != valor {
			t.Fatalf("SetPerfil(%s) devolvió %q, se esperaba %q", campo, guardado, valor)
		}
	}
	p, err := db.PerfilDe(ctx, perfilAna)
	if err != nil {
		t.Fatal(err)
	}
	if p.Nombre != "Ana" || p.Descripcion != "la de los memes" || p.Genero != "Mujer" ||
		p.Pasatiempo != "dibujar" || p.Cumple != "14 de marzo" {
		t.Fatalf("perfil incompleto: %+v", p)
	}
}

// La lista blanca es lo que impide que un nombre de campo acabe dentro del SQL.
func TestCampoNoValidoSeRechaza(t *testing.T) {
	db, ctx := abrir(t)
	if _, err := db.SetPerfil(ctx, perfilAna, "coins", "999999"); !errors.Is(err, ErrCampoInvalido) {
		t.Fatalf("se esperaba ErrCampoInvalido, salió %v", err)
	}
	if _, err := db.SetPerfil(ctx, perfilAna, "marry = 'x' WHERE 1=1 --", ""); !errors.Is(err, ErrCampoInvalido) {
		t.Fatalf("inyección aceptada: %v", err)
	}
	if err := db.LimpiarPerfil(ctx, perfilAna, "xp"); !errors.Is(err, ErrCampoInvalido) {
		t.Fatalf("LimpiarPerfil con campo inválido: %v", err)
	}
	// Y el saldo sigue intacto.
	acc, err := db.Get(ctx, perfilAna)
	if err != nil {
		t.Fatal(err)
	}
	if acc.Coins != 0 {
		t.Fatalf("las monedas cambiaron: %d", acc.Coins)
	}
}

// Un texto larguísimo se recorta en vez de guardarse entero: la respuesta del
// bot enseña lo guardado, así que no puede diferir de la base.
func TestTextoLargoSeRecorta(t *testing.T) {
	db, ctx := abrir(t)
	largo := strings.Repeat("a", 1000)
	guardado, err := db.SetPerfil(ctx, perfilAna, CampoDescripcion, largo)
	if err != nil {
		t.Fatal(err)
	}
	if len([]rune(guardado)) != MaxDesc {
		t.Fatalf("descripción de %d caracteres, se esperaba %d", len([]rune(guardado)), MaxDesc)
	}
	// Los saltos de línea se aplanan: si no, un campo de 200 caracteres podría
	// ocupar 200 líneas del chat.
	guardado, err = db.SetPerfil(ctx, perfilAna, CampoDescripcion, "una\nlínea\r\ny otra")
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(guardado, "\n\r") {
		t.Fatalf("quedaron saltos de línea: %q", guardado)
	}
}

func TestCasarseYDivorciarse(t *testing.T) {
	db, ctx := abrir(t)
	if _, err := db.SetPerfil(ctx, perfilAna, CampoNombre, "Ana"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetPerfil(ctx, perfilBeto, CampoNombre, "Beto"); err != nil {
		t.Fatal(err)
	}

	if err := db.Casarse(ctx, perfilAna, perfilBeto); err != nil {
		t.Fatalf("Casarse: %v", err)
	}
	pa, _ := db.PerfilDe(ctx, perfilAna)
	pb, _ := db.PerfilDe(ctx, perfilBeto)
	if pa.Pareja != perfilBeto || pb.Pareja != perfilAna {
		t.Fatalf("matrimonio asimétrico: perfilAna=%q perfilBeto=%q", pa.Pareja, pb.Pareja)
	}
	if pa.ParejaNombre != "Beto" || pb.ParejaNombre != "Ana" {
		t.Fatalf("nombres de pareja: %q / %q", pa.ParejaNombre, pb.ParejaNombre)
	}

	// Nadie se casa dos veces, y eso incluye a la otra parte.
	if err := db.Casarse(ctx, perfilAna, perfilCarla); !errors.Is(err, ErrYaCasado) {
		t.Fatalf("segundo matrimonio aceptado: %v", err)
	}
	if err := db.Casarse(ctx, perfilCarla, perfilBeto); !errors.Is(err, ErrYaCasado) {
		t.Fatalf("matrimonio con alguien casado aceptado: %v", err)
	}
	if err := db.Casarse(ctx, perfilAna, perfilAna); err == nil {
		t.Fatalf("se casó consigo misma")
	}

	ex, err := db.Divorciarse(ctx, perfilAna)
	if err != nil {
		t.Fatalf("Divorciarse: %v", err)
	}
	if ex != perfilBeto {
		t.Fatalf("ex = %q, se esperaba %q", ex, perfilBeto)
	}
	pa, _ = db.PerfilDe(ctx, perfilAna)
	pb, _ = db.PerfilDe(ctx, perfilBeto)
	if pa.Pareja != "" || pb.Pareja != "" {
		t.Fatalf("el divorcio dejó un lado casado: perfilAna=%q perfilBeto=%q", pa.Pareja, pb.Pareja)
	}
	if _, err := db.Divorciarse(ctx, perfilAna); !errors.Is(err, ErrNoCasado) {
		t.Fatalf("divorcio de soltera: %v", err)
	}
}

// La tabla de niveles y el puesto: quien no tiene XP no aparece (ni ocupa un
// puesto inventado).
func TestRankingYPosicion(t *testing.T) {
	db, ctx := abrir(t)
	if _, err := db.AddXP(ctx, perfilAna, 5000); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AddXP(ctx, perfilBeto, 900); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetPerfil(ctx, perfilAna, CampoNombre, "Ana"); err != nil {
		t.Fatal(err)
	}

	tabla, err := db.TopPorXP(ctx, 10)
	if err != nil {
		t.Fatalf("TopPorXP: %v", err)
	}
	if len(tabla) != 2 {
		t.Fatalf("la tabla tiene %d filas, se esperaban 2 (perfilCarla no tiene XP)", len(tabla))
	}
	if tabla[0].JID != perfilAna || tabla[1].JID != perfilBeto {
		t.Fatalf("orden incorrecto: %+v", tabla)
	}
	if tabla[0].Nombre != "Ana" {
		t.Fatalf("no trajo el nombre: %+v", tabla[0])
	}

	puesto, err := db.Posicion(ctx, perfilAna)
	if err != nil {
		t.Fatal(err)
	}
	if puesto != 1 {
		t.Fatalf("puesto de Ana = %d, se esperaba 1", puesto)
	}
	puesto, _ = db.Posicion(ctx, perfilCarla)
	if puesto != 0 {
		t.Fatalf("puesto de quien no tiene XP = %d, se esperaba 0", puesto)
	}
	// Un límite absurdo no debe devolver una tabla infinita.
	if tabla, _ := db.TopPorXP(ctx, -3); len(tabla) > 10 {
		t.Fatalf("límite negativo devolvió %d filas", len(tabla))
	}
}

// El nivel guardado sale de la curva única: subir XP y leer el perfil no puede
// dar dos niveles distintos (era el fallo entre AddXP y el perfil).
func TestNivelCoherenteConLaCurva(t *testing.T) {
	db, ctx := abrir(t)
	acc, err := db.AddXP(ctx, perfilAna, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	p, err := db.PerfilDe(ctx, perfilAna)
	if err != nil {
		t.Fatal(err)
	}
	if p.Nivel != acc.Level {
		t.Fatalf("el perfil dice nivel %d y la cuenta %d", p.Nivel, acc.Level)
	}
	if p.Nivel <= 1 {
		t.Fatalf("un millón de XP debería dar un nivel alto, dio %d", p.Nivel)
	}
	if p.XP != 1_000_000 {
		t.Fatalf("XP del perfil = %d", p.XP)
	}
}

func TestContadorDeComandos(t *testing.T) {
	db, ctx := abrir(t)
	for i := 0; i < 3; i++ {
		if err := db.SumarComando(ctx, perfilAna); err != nil {
			t.Fatal(err)
		}
	}
	p, err := db.PerfilDe(ctx, perfilAna)
	if err != nil {
		t.Fatal(err)
	}
	if p.Comandos != 3 {
		t.Fatalf("comandos = %d, se esperaban 3", p.Comandos)
	}
}

func TestAFK(t *testing.T) {
	db, ctx := abrir(t)
	if _, err := db.SetPerfil(ctx, perfilAna, CampoAFK, "1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SetPerfil(ctx, perfilAna, CampoAFKRazon, "estudiando"); err != nil {
		t.Fatal(err)
	}
	ausente, razon, _, err := db.AFKDe(ctx, perfilAna)
	if err != nil {
		t.Fatal(err)
	}
	if !ausente || razon != "estudiando" {
		t.Fatalf("AFK = %v / %q", ausente, razon)
	}
	if _, err := db.SetPerfil(ctx, perfilAna, CampoAFK, "0"); err != nil {
		t.Fatal(err)
	}
	ausente, _, _, _ = db.AFKDe(ctx, perfilAna)
	if ausente {
		t.Fatal("siguió ausente después de volver")
	}
	// Un JID que no existe no debe dar error: el hook lo consulta a diario.
	ausente, _, _, err = db.AFKDe(ctx, "5215599999999@s.whatsapp.net")
	if err != nil || ausente {
		t.Fatalf("AFKDe de un desconocido: %v / %v", ausente, err)
	}
}

// ─── esquema ────────────────────────────────────────────────────────────────

// Cada migración es un escalón: si una quedara vacía, el número de escalón
// dejaría de coincidir con el comentario que la documenta.
func TestMigracionesSinEscalonesVacios(t *testing.T) {
	for i, paso := range migrations {
		if len(paso) == 0 {
			t.Fatalf("la migración %d está vacía", i+1)
		}
		for _, sentencia := range paso {
			if strings.TrimSpace(sentencia) == "" {
				t.Fatalf("la migración %d tiene una sentencia vacía", i+1)
			}
		}
	}
}

// Las columnas del perfil tienen que existir de verdad en la base, no solo en
// el código: es el fallo que se cuela cuando la migración no se aplica.
func TestColumnasDelPerfilEnLaBase(t *testing.T) {
	db, ctx := abrir(t)
	filas, err := db.db.QueryContext(ctx, `SELECT name FROM pragma_table_info('users')`)
	if err != nil {
		t.Fatalf("pragma_table_info: %v", err)
	}
	defer filas.Close()

	hay := map[string]bool{}
	for filas.Next() {
		var nombre string
		if err := filas.Scan(&nombre); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		hay[nombre] = true
	}
	if err := filas.Err(); err != nil {
		t.Fatalf("filas: %v", err)
	}

	for _, col := range []string{
		"jid", "coins", "bank", "xp", "level", "created_at",
		"name", "description", "genre", "hobby", "birth",
		"marry", "afk", "afk_reason", "used_commands",
	} {
		if !hay[col] {
			t.Fatalf("falta la columna %q en users (¿migración sin aplicar?)", col)
		}
	}
}

// Mirar el perfil de otra persona no puede crearle la cuenta: eso llenaría la
// base de filas fantasma en un grupo donde se menciona mucho.
func TestPerfilExistenteNoCreaFilas(t *testing.T) {
	db, ctx := abrir(t)

	if _, ok, err := db.PerfilExistente(ctx, perfilCarla); err != nil || ok {
		t.Fatalf("PerfilExistente de quien no existe: ok=%v err=%v", ok, err)
	}
	var n int
	if err := db.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE jid = ?`, perfilCarla).Scan(&n); err != nil {
		t.Fatalf("COUNT: %v", err)
	}
	if n != 0 {
		t.Fatalf("PerfilExistente creó %d filas", n)
	}

	// Después de que la persona escriba (PerfilDe), sí existe y trae sus datos.
	if _, err := db.PerfilDe(ctx, perfilCarla); err != nil {
		t.Fatalf("PerfilDe: %v", err)
	}
	if _, err := db.SetPerfil(ctx, perfilCarla, CampoNombre, "Carla"); err != nil {
		t.Fatalf("SetPerfil: %v", err)
	}
	p, ok, err := db.PerfilExistente(ctx, perfilCarla)
	if err != nil || !ok {
		t.Fatalf("ya existía: ok=%v err=%v", ok, err)
	}
	if p.Nombre != "Carla" || p.JID != perfilCarla {
		t.Fatalf("perfil inesperado: %+v", p)
	}
}

// El nombre de la pareja se rellena también por esta vía, o la ficha diría
// «Casada con alguien» aunque la pareja tenga nombre.
func TestPerfilExistenteTraeNombreDePareja(t *testing.T) {
	db, ctx := abrir(t)
	if err := db.Casarse(ctx, perfilAna, perfilBeto); err != nil {
		t.Fatalf("Casarse: %v", err)
	}
	if _, err := db.SetPerfil(ctx, perfilBeto, CampoNombre, "Beto"); err != nil {
		t.Fatalf("SetPerfil: %v", err)
	}
	p, ok, err := db.PerfilExistente(ctx, perfilAna)
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if p.ParejaNombre != "Beto" {
		t.Fatalf("ParejaNombre = %q", p.ParejaNombre)
	}
}
