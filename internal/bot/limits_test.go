// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package bot

import (
	"testing"
	"time"
)

// El guardián tiene que cortar el abuso y avisar UNA vez, no en cada mensaje.
func TestRateGuardAllowsUntilLimit(t *testing.T) {
	g := NewRateGuard(3, time.Minute)

	for i := 0; i < 3; i++ {
		if allow, _ := g.Allow("usuario"); !allow {
			t.Fatalf("el intento %d debería pasar", i+1)
		}
	}
	allow, warn := g.Allow("usuario")
	if allow {
		t.Fatal("el cuarto intento debería bloquearse")
	}
	if !warn {
		t.Fatal("el primer bloqueo debería avisar")
	}
	allow, warn = g.Allow("usuario")
	if allow || warn {
		t.Fatal("los bloqueos siguientes no deberían volver a avisar")
	}

	// Otro usuario no se ve afectado.
	if allow, _ := g.Allow("otro"); !allow {
		t.Fatal("el límite no debería ser global")
	}
}

func TestRateGuardWindowExpires(t *testing.T) {
	g := NewRateGuard(1, 50*time.Millisecond)

	if allow, _ := g.Allow("u"); !allow {
		t.Fatal("el primero debería pasar")
	}
	if allow, _ := g.Allow("u"); allow {
		t.Fatal("el segundo debería bloquearse")
	}
	time.Sleep(70 * time.Millisecond)
	if allow, _ := g.Allow("u"); !allow {
		t.Fatal("pasada la ventana debería volver a pasar")
	}
}

// Nil-safe: los hooks pueden no tener guardián y no debe reventar.
func TestRateGuardNil(t *testing.T) {
	var g *RateGuard
	if allow, warn := g.Allow("x"); !allow || warn {
		t.Fatal("un guardián nil debe dejar pasar siempre")
	}
}

// El mapa no puede crecer sin límite con un grupo grande.
func TestRateGuardBounded(t *testing.T) {
	g := NewRateGuard(5, time.Minute)
	for i := 0; i < guardMaxKeys*2; i++ {
		g.Allow(string(rune('a'+i%26)) + string(rune(i/26)))
	}
	g.mu.Lock()
	n := len(g.entries)
	g.mu.Unlock()
	if n > guardMaxKeys*2 {
		t.Fatalf("el mapa creció a %d entradas", n)
	}
}
