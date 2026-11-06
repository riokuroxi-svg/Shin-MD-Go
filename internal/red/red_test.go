// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
package red

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func TestNuevoPrefijos(t *testing.T) {
	casos := []struct {
		nombre  string
		entrada string
		bits    int
		quiero  string
	}{
		{"v6 con barra", "2001:db8:1:2::/64", 64, "2001:db8:1:2::/64"},
		{"v6 sin barra", "2001:db8:1:2::", 64, "2001:db8:1:2::/64"},
		{"v6 que se recorta a la red", "2001:db8:1:2::9/64", 64, "2001:db8:1:2::/64"},
		{"v4 loopback", "127.0.0.0/8", 8, "127.0.0.0/8"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			r, err := Nuevo(c.entrada, Opciones{})
			if err != nil {
				t.Fatalf("Nuevo(%q): %v", c.entrada, err)
			}
			if r.Prefijo().Bits() != c.bits {
				t.Fatalf("bits = %d, quiero %d", r.Prefijo().Bits(), c.bits)
			}
			if r.Prefijo().String() != c.quiero {
				t.Fatalf("prefijo = %s, quiero %s", r.Prefijo(), c.quiero)
			}
		})
	}
}

func TestNuevoRechazaBasura(t *testing.T) {
	for _, malo := range []string{"", "  ", "abc", "1.2.3.4/33", "::/0", "0.0.0.0/0", "2001:db8::/16"} {
		if _, err := Nuevo(malo, Opciones{}); err == nil {
			t.Fatalf("Nuevo(%q) debió fallar", malo)
		}
	}
}

func TestTopeAcotaElTotal(t *testing.T) {
	r, err := Nuevo("2001:db8::/64", Opciones{Tope: 16})
	if err != nil {
		t.Fatal(err)
	}
	if r.Total() != 16 {
		t.Fatalf("Total = %d, quiero 16", r.Total())
	}

	// Un /30 solo tiene 4 direcciones: el tope no puede inventar más.
	r2, err := Nuevo("10.0.0.0/30", Opciones{Tope: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if r2.Total() != 4 {
		t.Fatalf("Total = %d, quiero 4", r2.Total())
	}
}

func TestSiguienteRecorreElBloque(t *testing.T) {
	r, _ := Nuevo("10.0.0.0/30", Opciones{MaxPorIP: 1})
	ahora := time.Now()

	vistos := map[netip.Addr]bool{}
	if r.Usables() != 3 {
		t.Fatalf("Usables = %d, quiero 3 en un /30", r.Usables())
	}
	for i := 0; i < 3; i++ {
		ip, ok := r.Siguiente(ahora)
		if !ok {
			t.Fatalf("petición %d: se agotó antes de tiempo", i)
		}
		if !r.Prefijo().Contains(ip) {
			t.Fatalf("%s no pertenece a %s", ip, r.Prefijo())
		}
		if vistos[ip] {
			t.Fatalf("%s repetida: el reparto no rota", ip)
		}
		vistos[ip] = true
	}
	if _, ok := r.Siguiente(ahora); ok {
		t.Fatal("con MaxPorIP=1 y un /30 (3 usables) la cuarta debió fallar")
	}
}

func TestVentanaDevuelveCapacidad(t *testing.T) {
	r, _ := Nuevo("10.0.0.0/30", Opciones{MaxPorIP: 1, Ventana: 20 * time.Millisecond})
	ahora := time.Now()

	for i := 0; i < 3; i++ {
		if _, ok := r.Siguiente(ahora); !ok {
			t.Fatalf("dirección %d: debió haber libres", i)
		}
	}
	if _, ok := r.Siguiente(ahora); ok {
		t.Fatal("debieron agotarse")
	}

	// Al pasar la ventana, vuelven a estar disponibles sin reiniciar nada.
	despues := ahora.Add(50 * time.Millisecond)
	if _, ok := r.Siguiente(despues); !ok {
		t.Fatal("la ventana debió liberar las direcciones")
	}
}

func TestEstado(t *testing.T) {
	r, _ := Nuevo("10.0.0.0/29", Opciones{MaxPorIP: 2, Ventana: time.Minute})
	ahora := time.Now()
	for i := 0; i < 3; i++ {
		if _, ok := r.Siguiente(ahora); !ok {
			t.Fatal("sin direcciones")
		}
	}
	e := r.Estado(ahora)
	if e.Total != 8 || e.MaxPorIP != 2 || e.VentanaSeg != 60 {
		t.Fatalf("estado inesperado: %+v", e)
	}
	if e.EnUso != 3 {
		t.Fatalf("EnUso = %d, quiero 3", e.EnUso)
	}
}

func TestConcurrenteSinRepetir(t *testing.T) {
	r, _ := Nuevo("10.0.0.0/24", Opciones{MaxPorIP: 1, Ventana: time.Hour})

	const gorutinas = 32
	var wg sync.WaitGroup
	var mu sync.Mutex
	vistas := map[netip.Addr]bool{}
	fallos := 0

	for i := 0; i < gorutinas; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ip, ok := r.Siguiente(time.Now())
			mu.Lock()
			defer mu.Unlock()
			if !ok {
				fallos++
				return
			}
			if vistas[ip] {
				fallos++
			}
			vistas[ip] = true
		}()
	}
	wg.Wait()

	if fallos > 0 {
		t.Fatalf("%d fallos: direcciones repetidas o agotadas", fallos)
	}
	if len(vistas) != gorutinas {
		t.Fatalf("se vieron %d direcciones, quiero %d", len(vistas), gorutinas)
	}
}

// La prueba que importa: cada petición HTTP sale por una dirección distinta y
// el otro extremo la ve tal cual. Usa loopback (127.0.0.0/8) porque funciona en
// cualquier Linux sin privilegios; en un servidor con /64 es el mismo camino.
func TestTransporteCambiaLaDireccionDeOrigen(t *testing.T) {
	if testing.Short() {
		t.Skip("prueba de red")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("sin loopback: %v", err)
	}
	var mu sync.Mutex
	origenes := map[string]int{}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		mu.Lock()
		origenes[host]++
		mu.Unlock()
		io.WriteString(w, host)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	r, err := Nuevo("127.0.0.0/28", Opciones{MaxPorIP: 1, Ventana: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	cliente := &http.Client{Transport: r.Transporte(nil), Timeout: 5 * time.Second}
	destino := "http://" + ln.Addr().String() + "/"

	pedidas := 0
	for i := 0; i < 10; i++ {
		resp, err := cliente.Get(destino)
		if err != nil {
			if errors.Is(err, ErrSinDirecciones) {
				break
			}
			t.Fatalf("petición %d: %v", i, err)
		}
		resp.Body.Close()
		pedidas++
	}

	// El bloque /28 se usa desde .1 hasta .15 contando desde la red, es decir 15
	// direcciones útiles menos la reservada.
	if pedidas < 2 {
		t.Fatalf("solo se completaron %d peticiones", pedidas)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(origenes) != pedidas {
		t.Fatalf("el servidor vio %d direcciones distintas de %d peticiones: %v",
			len(origenes), pedidas, origenes)
	}
	for ip, n := range origenes {
		if n != 1 {
			t.Fatalf("la dirección %s se usó %d veces con MaxPorIP=1", ip, n)
		}
	}
}

func TestTransporteNoReutilizaConexiones(t *testing.T) {
	r, _ := Nuevo("127.0.0.0/28", Opciones{})
	tr := r.Transporte(nil)
	if !tr.DisableKeepAlives {
		t.Fatal("el transporte debe desactivar keep-alive para rotar de verdad")
	}
	if tr.DialContext == nil {
		t.Fatal("el transporte debe llevar su propio DialContext")
	}
}
