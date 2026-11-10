// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// perfil_test.go — Pruebas del bloque de perfil que NO necesitan WhatsApp:
// el formato de la tabla, el estado civil, el puesto, los alias y el almacén
// de propuestas de matrimonio. Lo que toca la base ya se prueba en
// internal/store/perfil_test.go; aquí solo se revisa lo que se ve en el chat.
package cmds

import (
	"strings"
	"testing"
	"time"

	"shin-go/internal/bot"
	"shin-go/internal/store"
)

// ─── tabla de niveles ───────────────────────────────────────────────────────

func TestTextoTablaFormato(t *testing.T) {
	tabla := []store.FilaRanking{
		{JID: "5211111111111@s.whatsapp.net", Nombre: "Ana", XP: 12345, Nivel: 7},
		{JID: "5222222222222@s.whatsapp.net", Nombre: "", XP: 900, Nivel: 3},
		{JID: "5233333333333@s.whatsapp.net", Nombre: "Carla", XP: 10, Nivel: 1},
		{JID: "5244444444444@s.whatsapp.net", Nombre: "Dani", XP: 1, Nivel: 1},
	}
	texto := textoTabla(tabla, "5222222222222@s.whatsapp.net")

	if !strings.Contains(texto, "TABLA DE NIVELES") {
		t.Fatalf("falta el título:\n%s", texto)
	}
	// Medallas en los tres primeros; número a partir del cuarto.
	for _, medalla := range []string{"🥇", "🥈", "🥉"} {
		if !strings.Contains(texto, medalla) {
			t.Fatalf("falta la medalla %s:\n%s", medalla, texto)
		}
	}
	if !strings.Contains(texto, "4) *Dani*") {
		t.Fatalf("el cuarto puesto debe ir numerado:\n%s", texto)
	}
	// Sin nombre se cae al número del JID, nunca al JID completo.
	if strings.Contains(texto, "@s.whatsapp.net") {
		t.Fatalf("no debe aparecer el dominio del JID:\n%s", texto)
	}
	if !strings.Contains(texto, "5222222222222") {
		t.Fatalf("falta el número del que no tiene nombre:\n%s", texto)
	}
	// El que consulta va marcado con ▸.
	if !strings.Contains(texto, "▸") {
		t.Fatalf("no se marca la fila propia:\n%s", texto)
	}
	// El XP va con separador de miles, como en el resto del bot.
	if !strings.Contains(texto, "12,345 XP") {
		t.Fatalf("el XP no va con separador de miles:\n%s", texto)
	}
	// Y se cierra el marco.
	if !strings.HasSuffix(texto, "╯") {
		t.Fatalf("falta el cierre del marco:\n%s", texto)
	}
}

func TestTextoTablaEscapaNombres(t *testing.T) {
	tabla := []store.FilaRanking{
		{JID: "5211111111111@s.whatsapp.net", Nombre: "a_b*c[d]", XP: 5, Nivel: 1},
	}
	texto := textoTabla(tabla, "")
	// El nombre lo pone la gente: si no se escapa, rompe el formato de WhatsApp.
	// Esc no usa barras invertidas: cambia los caracteres por sus gemelos
	// tipográficos (＊, ‗, ´, ∼), así que aquí no debe quedar ninguno de los
	// originales con significado en el marcado.
	if strings.Contains(texto, "a_b*c[d]") {
		t.Fatalf("el nombre no se escapó:\n%s", texto)
	}
	if !strings.Contains(texto, "a‗b∗c[d]") {
		t.Fatalf("escape inesperado:\n%s", texto)
	}
}

func TestTextoTablaVacia(t *testing.T) {
	// Sin filas solo queda el marco: no debe entrar en pánico ni salir raro.
	texto := textoTabla(nil, "")
	if !strings.Contains(texto, "TABLA DE NIVELES") || !strings.HasSuffix(texto, "╯") {
		t.Fatalf("tabla vacía mal formada:\n%s", texto)
	}
}

func TestTextoTablaSoloUnaFila(t *testing.T) {
	texto := textoTabla([]store.FilaRanking{
		{JID: "5211111111111@s.whatsapp.net", Nombre: "Ana", XP: 1, Nivel: 1},
	}, "5211111111111@s.whatsapp.net")
	if strings.Contains(texto, "🥈") {
		t.Fatalf("con una sola fila no debe haber plata:\n%s", texto)
	}
	if !strings.Contains(texto, "🥇") || !strings.Contains(texto, "▸") {
		t.Fatalf("falta el oro o la marca propia:\n%s", texto)
	}
}

// ─── detalles de formato ────────────────────────────────────────────────────

func TestPuesto(t *testing.T) {
	casos := map[int]string{0: "", -3: "", 1: " _(Top #1)_", 27: " _(Top #27)_"}
	for pos, quiero := range casos {
		if got := puesto(pos); got != quiero {
			t.Fatalf("puesto(%d) = %q, se esperaba %q", pos, got, quiero)
		}
	}
}

func TestEstadoCivil(t *testing.T) {
	if got := estadoCivil(&store.Perfil{}); got != "Soltero/a" {
		t.Fatalf("sin pareja: %q", got)
	}
	casos := []struct{ genero, quiero string }{
		{"Mujer", "Casada con "},
		{"femenino", "Casada con "},
		{"Hombre", "Casado con "},
		{"", "Casadx con "},
		{"otro", "Casadx con "},
	}
	for _, c := range casos {
		p := &store.Perfil{Genero: c.genero, Pareja: "52999@s.whatsapp.net", ParejaNombre: "Beto"}
		got := estadoCivil(p)
		if !strings.HasPrefix(got, c.quiero) {
			t.Fatalf("género %q: %q no empieza por %q", c.genero, got, c.quiero)
		}
		if !strings.Contains(got, "Beto") {
			t.Fatalf("género %q: falta el nombre de la pareja en %q", c.genero, got)
		}
	}
	// Pareja sin nombre guardado: se dice «alguien», no una cadena vacía.
	if got := estadoCivil(&store.Perfil{Pareja: "52999@s.whatsapp.net"}); !strings.Contains(got, "alguien") {
		t.Fatalf("pareja sin nombre: %q", got)
	}
}

func TestCorto(t *testing.T) {
	casos := map[string]string{
		"5215512345678@s.whatsapp.net":    "5215512345678",
		"5215512345678:12@s.whatsapp.net": "5215512345678",
		"sinarroba":                       "sinarroba",
		"@s.whatsapp.net":                 "@s.whatsapp.net", // arroba en 0: se deja tal cual
		"":                                "",
	}
	for jid, quiero := range casos {
		if got := corto(jid); got != quiero {
			t.Fatalf("corto(%q) = %q, se esperaba %q", jid, got, quiero)
		}
	}
}

func TestO(t *testing.T) {
	if got := o("  ", "defecto"); got != "defecto" {
		t.Fatalf("vacío: %q", got)
	}
	if got := o("algo", "defecto"); got != "algo" {
		t.Fatalf("con valor: %q", got)
	}
}

// ─── propuestas de matrimonio ───────────────────────────────────────────────

func TestPropuestaDeUnSoloUso(t *testing.T) {
	proponer("52999@s.whatsapp.net", "52111@s.whatsapp.net")
	de, ok := retirarPropuesta("52999@s.whatsapp.net")
	if !ok || de != "52111@s.whatsapp.net" {
		t.Fatalf("la propuesta no salió: %q %v", de, ok)
	}
	// Un segundo «acepto» no puede volver a casar a nadie.
	if _, ok := retirarPropuesta("52999@s.whatsapp.net"); ok {
		t.Fatal("la propuesta se pudo aceptar dos veces")
	}
}

func TestPropuestaCaduca(t *testing.T) {
	proponer("52888@s.whatsapp.net", "52111@s.whatsapp.net")
	// Se envejece a mano: esperar 10 minutos en una prueba no es una opción.
	propuestas.Lock()
	p := propuestas.m["52888@s.whatsapp.net"]
	p.cuando = time.Now().Add(-caducidadPropuesta - time.Second)
	propuestas.m["52888@s.whatsapp.net"] = p
	propuestas.Unlock()

	if _, ok := retirarPropuesta("52888@s.whatsapp.net"); ok {
		t.Fatal("una propuesta caducada no debe aceptarse")
	}
}

func TestPropuestaSinDestinatario(t *testing.T) {
	if _, ok := retirarPropuesta("00000@s.whatsapp.net"); ok {
		t.Fatal("no hay propuesta y dijo que sí")
	}
}

func TestPropuestaReemplazaLaAnterior(t *testing.T) {
	para := "52777@s.whatsapp.net"
	proponer(para, "52111@s.whatsapp.net")
	proponer(para, "52222@s.whatsapp.net")
	de, ok := retirarPropuesta(para)
	if !ok || de != "52222@s.whatsapp.net" {
		t.Fatalf("debe valer la última propuesta, salió %q", de)
	}
}

// ─── registro de comandos ──────────────────────────────────────────────────

// El bloque de perfil tiene que estar completo en el menú: si un comando
// desaparece del registro, deja de funcionar sin que nadie se entere.
func TestComandosDePerfilRegistrados(t *testing.T) {
	quiero := []string{
		"profile", "level", "lboard",
		"setdesc", "setgenre", "sethobby", "setbirth",
		"deldesc", "delgenre", "delhobby", "delbirth",
		"marry", "acepto", "divorce",
	}
	for _, nombre := range quiero {
		cmd, ok := bot.Lookup(nombre)
		if !ok {
			t.Fatalf("falta el comando %q", nombre)
		}
		if cmd.Category != bot.CatPerfil {
			t.Fatalf("%q está en la categoría %q", nombre, cmd.Category)
		}
	}
}

// El AFK vive en proteccion.go, con su hook: aquí no debe haber otro registro
// que lo pise (el primero que se registra gana y el otro queda inalcanzable).
func TestAFKNoSeDuplica(t *testing.T) {
	veces := 0
	for _, cmd := range bot.All() {
		if cmd.Name == "afk" {
			veces++
		}
	}
	if veces != 1 {
		t.Fatalf("hay %d comandos .afk registrados; debe haber exactamente 1", veces)
	}
}

// ─── ficha de .profile ─────────────────────────────────────────────────────

func fichaDePrueba() ficha {
	return ficha{
		Nombre:     "Ana",
		Nivel:      7,
		Puesto:     " _(Top #3)_",
		Dentro:     4200,
		Necesaria:  12000,
		Barra:      "▰▰▰▱▱▱▱▱▱▱",
		Porcentaje: 35,
		Cartera:    150000,
		Banco:      250000,
		Moneda:     "monedas",
		Cumple:     "14 de marzo",
		Pasatiempo: "dibujar",
		Genero:     "Mujer",
		Civil:      "Casada con Beto",
		Dias:       4,
		Semanas:    2,
		Daily:      "disponible ✓",
		Comandos:   1234,
	}
}

func TestTextoPerfilCompleto(t *testing.T) {
	texto := textoPerfil(fichaDePrueba())

	for _, quiero := range []string{
		"*PERFIL*",
		"*Ana*",
		"*7*",               // nivel
		"_(Top #3)_",        // puesto
		"4,200",             // XP dentro del nivel
		"12,000",            // XP necesaria
		"▰▰▰▱▱▱▱▱▱▱",        // barra
		"35 %",              // porcentaje
		"*150,000*",         // cartera
		"*250,000*",         // banco
		"*400,000 monedas*", // total = cartera + banco
		"14 de marzo",
		"dibujar",
		"Mujer",
		"Casada con Beto",
		"4 días · 2 semanas",
		"disponible ✓",
		"1,234", // comandos
	} {
		if !strings.Contains(texto, quiero) {
			t.Fatalf("falta %q en la ficha:\n%s", quiero, texto)
		}
	}
	if !strings.HasSuffix(texto, "╯") {
		t.Fatalf("falta el cierre del marco:\n%s", texto)
	}
	// Sin descripción ni AFK no se añaden las dos líneas finales.
	if strings.Contains(texto, "Ausente") {
		t.Fatalf("no debería salir la línea de AFK:\n%s", texto)
	}
}

func TestTextoPerfilConDescripcionYAFK(t *testing.T) {
	f := fichaDePrueba()
	f.Descripcion = "me gusta el café"
	f.AvisoAFK = "exámenes"
	texto := textoPerfil(f)
	if !strings.Contains(texto, "> me gusta el café") {
		t.Fatalf("falta la descripción:\n%s", texto)
	}
	if !strings.Contains(texto, "💤 Ausente: *exámenes*") {
		t.Fatalf("falta el aviso de AFK:\n%s", texto)
	}
}

func TestTextoPerfilEscapaTextoLibre(t *testing.T) {
	f := fichaDePrueba()
	f.Cumple = "1_de*marzo*"
	f.Pasatiempo = "a_b"
	f.Genero = "x*y"
	f.Descripcion = "d_e*f"
	f.AvisoAFK = "g_h*i"
	texto := textoPerfil(f)
	for _, crudo := range []string{"1_de*marzo*", "a_b", "x*y", "d_e*f", "g_h*i"} {
		if strings.Contains(texto, crudo) {
			t.Fatalf("no se escapó %q:\n%s", crudo, texto)
		}
	}
}

func TestTextoPerfilSinDatos(t *testing.T) {
	// Una cuenta nueva: nada de campos vacíos entre asteriscos, que en WhatsApp
	// dejan el mensaje con marcas sueltas.
	texto := textoPerfil(ficha{
		Nombre: "Nuevo", Nivel: 1, Necesaria: 2, Barra: "▱▱▱▱▱▱▱▱▱▱",
		Cumple: "Sin especificar", Pasatiempo: "No definido", Genero: "Sin especificar",
		Civil: "Soltero/a", Daily: "disponible ✓", Moneda: "monedas",
	})
	for _, quiero := range []string{"Sin especificar", "No definido", "Soltero/a", "*1*"} {
		if !strings.Contains(texto, quiero) {
			t.Fatalf("falta %q:\n%s", quiero, texto)
		}
	}
	if strings.Contains(texto, "**") {
		t.Fatalf("hay un campo vacío entre asteriscos:\n%s", texto)
	}
}

func TestMotivoAFK(t *testing.T) {
	if got := motivoAFK(nil); got != "" {
		t.Fatalf("perfil nil: %q", got)
	}
	if got := motivoAFK(&store.Perfil{}); got != "" {
		t.Fatalf("sin AFK: %q", got)
	}
	if got := motivoAFK(&store.Perfil{AFK: true}); got != "sin motivo" {
		t.Fatalf("AFK sin motivo: %q", got)
	}
	if got := motivoAFK(&store.Perfil{AFK: true, AFKRazon: "viaje"}); got != "viaje" {
		t.Fatalf("AFK con motivo: %q", got)
	}
}
