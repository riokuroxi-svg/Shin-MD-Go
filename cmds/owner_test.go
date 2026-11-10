// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// owner_test.go — Lo del bloque owner/main que se puede probar sin WhatsApp:
// el candado anti-escape de `.subir`, la decisión de reiniciar, las barras del
// calentamiento y que los comandos peligrosos estén apagados por defecto.
package cmds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shin-go/internal/bot"
)

// ─── .subir: el candado anti-escape ─────────────────────────────────────────

func TestRutaSeguraAceptaDentroDelRepo(t *testing.T) {
	raiz := t.TempDir()
	casos := map[string]string{
		"nota.txt":         "nota.txt",
		"cmds/nuevo.go":    "cmds/nuevo.go",
		"docs/a/b/c.md":    "docs/a/b/c.md",
		"./nota.txt":       "nota.txt",
		"cmds/../nota.txt": "nota.txt", // se resuelve, no escapa
	}
	for pedida, quiero := range casos {
		_, rel, err := rutaSegura(raiz, pedida)
		if err != nil {
			t.Fatalf("%q: no debería fallar: %v", pedida, err)
		}
		if rel != quiero {
			t.Fatalf("%q → %q, se esperaba %q", pedida, rel, quiero)
		}
	}
}

func TestRutaSeguraRechazaEscapes(t *testing.T) {
	raiz := t.TempDir()
	males := []string{
		"../fuera.txt",
		"../../etc/passwd",
		"cmds/../../fuera.txt",
		"/etc/passwd",
		"..",
		"a/../../b",
		".git/config",
		".git/",
		"sub/.git/hooks/pre-commit",
	}
	for _, pedida := range males {
		if _, _, err := rutaSegura(raiz, pedida); err == nil {
			t.Fatalf("%q debería rechazarse", pedida)
		}
	}
	// El byte nulo corta rutas en algunas APIs del sistema: se rechaza.
	if _, _, err := rutaSegura(raiz, "nota\x00.txt"); err == nil {
		t.Fatal("una ruta con byte nulo debería rechazarse")
	}
}

func TestRutaSeguraNoConfundePrefijos(t *testing.T) {
	// Una carpeta hermana que empieza igual que la raíz no es la raíz:
	// /tmp/x y /tmp/x-otro son carpetas distintas, y la comprobación de
	// «empieza por» sin el separador las confundiría.
	padre := t.TempDir()
	raiz := filepath.Join(padre, "bot")
	if err := os.MkdirAll(raiz, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rutaSegura(raiz, "../bot-otro/archivo.txt"); err == nil {
		t.Fatal("una carpeta hermana con el mismo prefijo debería rechazarse")
	}
	// Y su vecina legítima sí entra.
	if _, _, err := rutaSegura(raiz, "sub/archivo.txt"); err != nil {
		t.Fatalf("una ruta dentro sí debería aceptarse: %v", err)
	}
}

// ─── .restart: no morirse cuando nadie nos levanta ─────────────────────────

func TestCaminoDeReinicio(t *testing.T) {
	casos := []struct {
		supervisor bool
		exec       bool
		quiero     string
	}{
		{true, false, "systemd"},
		{true, true, "systemd"}, // el supervisor manda
		{false, true, "exec"},
		{false, false, "sin-salida"},
	}
	for _, c := range casos {
		if got := caminoDeReinicio(c.supervisor, c.exec); got != c.quiero {
			t.Fatalf("caminoDeReinicio(%v,%v) = %q, se esperaba %q",
				c.supervisor, c.exec, got, c.quiero)
		}
	}
}

func TestHaySupervisorPorEntorno(t *testing.T) {
	t.Setenv("INVOCATION_ID", "")
	t.Setenv("SHIN_SUPERVISOR", "")
	if haySupervisor() {
		t.Fatal("sin variables no hay supervisor")
	}
	t.Setenv("INVOCATION_ID", "abc123")
	if !haySupervisor() {
		t.Fatal("INVOCATION_ID debería contar como supervisor (systemd)")
	}
	t.Setenv("INVOCATION_ID", "")
	t.Setenv("SHIN_SUPERVISOR", "1")
	if !haySupervisor() {
		t.Fatal("SHIN_SUPERVISOR=1 debería contar como supervisor")
	}
}

// ─── barras y tamaños ──────────────────────────────────────────────────────

func TestBarraTope(t *testing.T) {
	if got := barraTope(0, 100, 10); got != "▱▱▱▱▱▱▱▱▱▱" {
		t.Fatalf("vacía: %q", got)
	}
	if got := barraTope(100, 100, 10); got != "▰▰▰▰▰▰▰▰▰▰" {
		t.Fatalf("llena: %q", got)
	}
	if got := barraTope(50, 100, 10); got != "▰▰▰▰▰▱▱▱▱▱" {
		t.Fatalf("mitad: %q", got)
	}
	// Pasarse del tope no debe pintar más de la cuenta ni entrar en pánico.
	if got := barraTope(500, 100, 10); got != "▰▰▰▰▰▰▰▰▰▰" {
		t.Fatalf("excedida: %q", got)
	}
	if got := barraTope(5, 0, 10); got != "" {
		t.Fatalf("tope cero: %q", got)
	}
	if got := barraTope(5, 10, 0); got != "" {
		t.Fatalf("ancho cero: %q", got)
	}
}

func TestBytesLegibles(t *testing.T) {
	casos := map[int64]string{
		0:          "0 B",
		512:        "512 B",
		1024:       "1.0 KB",
		1536:       "1.5 KB",
		1048576:    "1.0 MB",
		1073741824: "1.0 GB",
		-5:         "0 B", // un negativo se lee como cero, no como «-5 B»
	}
	for n, quiero := range casos {
		if got := bytesLegibles(n); got != quiero {
			t.Fatalf("bytesLegibles(%d) = %q, se esperaba %q", n, got, quiero)
		}
	}
}

func TestFaltaMedianocheUTC(t *testing.T) {
	got := faltaMedianocheUTC()
	// Formato «Nh MMm», siempre con algo de tiempo por delante.
	if !strings.Contains(got, "h") || !strings.HasSuffix(got, "m") {
		t.Fatalf("formato inesperado: %q", got)
	}
	if strings.HasPrefix(got, "0h 00m") {
		t.Fatalf("no puede quedar tiempo cero: %q", got)
	}
}

// ─── caché ──────────────────────────────────────────────────────────────────

func TestMedirYVaciarCarpeta(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "media")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.bin"), make([]byte, 100), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "b.bin"), make([]byte, 50), 0o600); err != nil {
		t.Fatal(err)
	}

	tam, n := medirCarpeta(sub)
	if tam != 150 || n != 2 {
		t.Fatalf("medir: %d bytes, %d archivos", tam, n)
	}

	liberado, borrados := vaciarCarpeta(sub)
	if liberado != 150 || borrados != 2 {
		t.Fatalf("vaciar: %d bytes, %d archivos", liberado, borrados)
	}
	// La carpeta sigue existiendo: borrarla haría fallar el siguiente envío.
	if _, err := os.Stat(sub); err != nil {
		t.Fatalf("la carpeta debería seguir ahí: %v", err)
	}
	if tam, n := medirCarpeta(sub); tam != 0 || n != 0 {
		t.Fatalf("después de vaciar quedan %d bytes en %d archivos", tam, n)
	}
	// Vaciar algo que no existe no es un error: el bot no debe caerse por eso.
	if _, n := vaciarCarpeta(filepath.Join(dir, "no-existe")); n != 0 {
		t.Fatal("vaciar una carpeta inexistente debería devolver 0")
	}
}

// ─── registro y seguridad ───────────────────────────────────────────────────

func TestComandosDeSistemaRegistrados(t *testing.T) {
	quiero := map[string]bot.Category{
		"health":  bot.CatMain,
		"runtime": bot.CatMain,
		"infobot": bot.CatMain,
		"allmenu": bot.CatMain,
		"warmup":  bot.CatOwner,
		"compat":  bot.CatOwner,
		"restart": bot.CatOwner,
		"update":  bot.CatOwner,
		"subir":   bot.CatOwner,
		"shell":   bot.CatOwner,
		"cache":   bot.CatOwner,
		"setmenu": bot.CatOwner,
	}
	for nombre, cat := range quiero {
		cmd, ok := bot.Lookup(nombre)
		if !ok {
			t.Fatalf("falta el comando %q", nombre)
		}
		if cmd.Category != cat {
			t.Fatalf("%q está en %q, se esperaba %q", nombre, cmd.Category, cat)
		}
	}
}

// Los que tocan el proceso o el disco tienen que ser del dueño. Si alguno se
// colara sin OwnerOnly, cualquier persona del grupo podría reiniciar el bot o
// escribir archivos en el servidor.
func TestComandosPeligrososSonSoloDelDueno(t *testing.T) {
	for _, nombre := range []string{
		"restart", "update", "subir", "shell", "cache", "setmenu",
		"compat", "warmup", "health",
	} {
		cmd, ok := bot.Lookup(nombre)
		if !ok {
			t.Fatalf("falta %q", nombre)
		}
		// .health es de todos (consulta), pero el resto no.
		if nombre == "health" {
			continue
		}
		if !cmd.OwnerOnly {
			t.Fatalf("%q no está marcado OwnerOnly", nombre)
		}
	}
}

// El shell arranca apagado: sin OWNER_SHELL=1 no se ejecuta nada.
func TestShellApagadoPorDefecto(t *testing.T) {
	t.Setenv("OWNER_SHELL", "")
	if os.Getenv("OWNER_SHELL") == "1" {
		t.Fatal("el entorno de la prueba dice que el shell está encendido")
	}
}

func TestSetMenuVariantes(t *testing.T) {
	// Las variantes que se ofrecen son las que el menú sabe pintar.
	validas := map[string]bool{}
	for _, v := range variantesDeMenu {
		validas[v.clave] = true
	}
	for _, quiero := range []string{"v1", "v2", "v4"} {
		if !validas[quiero] {
			t.Fatalf("falta la variante %q", quiero)
		}
	}
	if validas["v3"] {
		t.Fatal("v3 es la de vídeo y este bot no la pinta: no debe ofrecerse")
	}
}

func TestParseIntSeguro(t *testing.T) {
	if n, err := parseIntSeguro("2"); err != nil || n != 2 {
		t.Fatalf("parseIntSeguro(2) = %d, %v", n, err)
	}
	if n, err := parseIntSeguro(" 4 "); err != nil || n != 4 {
		t.Fatalf("con espacios = %d, %v", n, err)
	}
	// Basura guardada en la base no puede tumbar el menú.
	if _, err := parseIntSeguro("dos"); err == nil {
		t.Fatal("«dos» no debería parsearse")
	}
}
