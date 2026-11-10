// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// proteccion_test.go — Pruebas de lo que no necesita WhatsApp en el bloque de
// protección: por ahora la recompensa de volver del AFK, que es dinero y por
// tanto lo que más duele si sale mal.
package cmds

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"shin-go/internal/bot"
)

func TestRecompensaAFK(t *testing.T) {
	ahora := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

	casos := []struct {
		nombre  string
		ausente time.Duration
		minimo  int64 // sin contar bonos: 8 por minuto
		bonos   int64 // 300–1500 cada 3 h
	}{
		{"un minuto", time.Minute, 8, 0},
		{"media hora", 30 * time.Minute, 240, 0},
		{"dos horas", 2 * time.Hour, 960, 0},
		{"justo 3 horas", 3 * time.Hour, 1440, 1},
		{"nueve horas", 9 * time.Hour, 4320, 3},
		{"dos días", 48 * time.Hour, 23040, 16},
	}
	for _, c := range casos {
		marca := strconv.FormatInt(ahora.Add(-c.ausente).UnixMilli(), 10)
		coins, tiempo := recompensaAFK(marca, ahora)
		if coins < c.minimo {
			t.Fatalf("%s: %d monedas, mínimo %d", c.nombre, coins, c.minimo)
		}
		// Los bonos son al azar: se acota el máximo posible.
		if max := c.minimo + c.bonos*1500; coins > max {
			t.Fatalf("%s: %d monedas, techo %d", c.nombre, coins, max)
		}
		if tiempo == "" || tiempo == "desconocido" {
			t.Fatalf("%s: tiempo raro %q", c.nombre, tiempo)
		}
	}
}

func TestRecompensaAFKSinMarca(t *testing.T) {
	ahora := time.Now()
	for _, marca := range []string{"", "   ", "-1", "0", "no es un número", "999999999999999999999999"} {
		coins, tiempo := recompensaAFK(marca, ahora)
		if coins != 0 || tiempo != "desconocido" {
			t.Fatalf("marca %q: (%d, %q); se esperaba (0, desconocido)", marca, coins, tiempo)
		}
	}
}

func TestRecompensaAFKRelojAtras(t *testing.T) {
	// Si la marca está en el futuro (reloj movido, base de otra máquina) no se
	// paga nada y no se dice una duración negativa.
	ahora := time.Now()
	futuro := strconv.FormatInt(ahora.Add(time.Hour).UnixMilli(), 10)
	coins, tiempo := recompensaAFK(futuro, ahora)
	if coins != 0 || tiempo != "desconocido" {
		t.Fatalf("futuro: (%d, %q)", coins, tiempo)
	}
}

func TestFechaCorta(t *testing.T) {
	// El formato que ve la gente: «día/mes hora:minuto», en UTC como antes.
	ms := time.Date(2026, 3, 7, 9, 4, 0, 0, time.UTC).UnixMilli()
	if got := fechaCorta(ms); got != "07/03 09:04" {
		t.Fatalf("fechaCorta = %q", got)
	}
}

func TestAFKEsDelPerfil(t *testing.T) {
	// En el JS .afk vive en la categoría profile; aquí debe salir en el mismo
	// menú, no escondido en utilidades.
	for _, cmd := range bot.All() {
		if cmd.Name == "afk" && string(cmd.Category) != "perfil" {
			t.Fatalf(".afk está en la categoría %q", cmd.Category)
		}
	}
}

func TestEsperaTextoDelAFK(t *testing.T) {
	// formatWait es el que pinta el tiempo ausente; se comprueba el caso que
	// usa el AFK (minutos y horas) para no depender de su orden interno.
	if got := formatWait(int64((2*time.Hour + 5*time.Minute).Seconds())); !strings.Contains(got, "2 horas") {
		t.Fatalf("formatWait = %q", got)
	}
}
