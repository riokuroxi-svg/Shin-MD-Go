// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// red.go — direcciones de origen propias para el tráfico del bot.
//
// La idea es la misma que usan las empresas de proxies, sin comprarles nada:
// elegir desde qué dirección sale cada petición. Si el servidor tiene un /64
// enrutado hacia él, basta repartir las peticiones entre sus direcciones para
// que ninguna concentre el patrón que dispara un bloqueo.
//
// Lo que este paquete NO hace: saltar captchas, derrotar atestación (poToken)
// ni romper protecciones. Solo administra espacio de direcciones propio.
package red

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"time"
)

// Opciones de un rotador. Los ceros toman los valores por defecto.
type Opciones struct {
	// MaxPorIP es cuántas peticiones admite cada dirección dentro de la
	// ventana antes de pasar a la siguiente. 0 => 50.
	MaxPorIP int

	// Ventana es el tiempo tras el cual se olvida el conteo de una dirección.
	// 0 => un minuto.
	Ventana time.Duration

	// Tope limita cuántas direcciones del prefijo se usan de verdad. Un /64
	// tiene 2^64 direcciones; no hace falta reservar memoria para todas.
	// 0 => 65536.
	Tope uint64
}

func (o Opciones) normalizar() Opciones {
	if o.MaxPorIP <= 0 {
		o.MaxPorIP = 50
	}
	if o.Ventana <= 0 {
		o.Ventana = time.Minute
	}
	if o.Tope == 0 {
		o.Tope = 1 << 16
	}
	return o
}

type uso struct {
	cuenta    int
	ultimoUso time.Time
}

// Rotador reparte peticiones entre las direcciones de un prefijo.
// Es seguro para uso concurrente.
type Rotador struct {
	prefijo netip.Prefix
	opts    Opciones
	total   uint64

	mu     sync.Mutex
	cursor uint64
	usos   map[netip.Addr]*uso
}

// Nuevo crea un rotador para un prefijo. Acepta "2001:db8:1:2::/64",
// "2001:db8:1:2::" (se asume /64) y "127.0.0.0/8" para pruebas locales.
func Nuevo(prefijo string, opts Opciones) (*Rotador, error) {
	opts = opts.normalizar()

	p, err := netip.ParsePrefix(prefijo)
	if err != nil {
		// Sin barra: se interpreta como dirección y se asume tamaño de bloque.
		addr, err2 := netip.ParseAddr(prefijo)
		if err2 != nil {
			return nil, fmt.Errorf("prefijo inválido %q: %w", prefijo, err)
		}
		bits := 64
		if addr.Is4() {
			bits = 8
		}
		p = netip.PrefixFrom(addr, bits)
	}
	p = p.Masked()
	if p.Addr().Is4In6() {
		p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
	}
	if !p.Addr().IsValid() {
		return nil, fmt.Errorf("prefijo inválido %q", prefijo)
	}

	// Un prefijo demasiado grande no es un bloque de origen: es media internet
	// o el espacio global entero. Se exige al menos /32 en IPv6 y /8 en IPv4.
	minimo := 32
	if p.Addr().Is4() {
		minimo = 8
	}
	if p.Bits() < minimo {
		return nil, fmt.Errorf("prefijo %s demasiado grande (mínimo /%d)", p, minimo)
	}

	bitsHost := p.Addr().BitLen() - p.Bits()
	if bitsHost <= 0 {
		return nil, fmt.Errorf("el prefijo %s no deja direcciones libres", p)
	}
	// Cantidad de direcciones: 2^bitsHost, recortado al tope.
	total := opts.Tope
	if bitsHost < 32 {
		if n := uint64(1) << uint(bitsHost); n < total {
			total = n
		}
	}
	if total < 2 {
		return nil, fmt.Errorf("el prefijo %s no deja direcciones usables", p)
	}

	return &Rotador{
		prefijo: p,
		opts:    opts,
		total:   total,
		usos:    make(map[netip.Addr]*uso, int(min(total, 1024))),
	}, nil
}

// Total es cuántas direcciones distintas se repartirán.
func (r *Rotador) Total() uint64 { return r.total }

// Usables es Total menos la primera dirección del bloque, que se reserva
// (en IPv6 es la del router del segmento; en IPv4, la de red).
func (r *Rotador) Usables() uint64 { return r.total - 1 }

// Prefijo devuelve el bloque configurado.
func (r *Rotador) Prefijo() netip.Prefix { return r.prefijo }

func (r *Rotador) dir(n uint64) netip.Addr {
	b := r.prefijo.Addr().AsSlice()
	// Se suma el desplazamiento desde el final del bloque, que es donde vive
	// la parte de host.
	for i := len(b) - 1; i >= 0 && n > 0; i-- {
		suma := uint64(b[i]) + (n & 0xff)
		b[i] = byte(suma)
		n = (n >> 8) + (suma >> 8)
	}
	a, _ := netip.AddrFromSlice(b)
	return a
}

// Siguiente devuelve la próxima dirección de origen disponible, respetando el
// presupuesto por dirección. Devuelve false cuando no queda ninguna libre en la
// ventana actual.
func (r *Rotador) Siguiente(ahora time.Time) (netip.Addr, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for intento := uint64(0); intento < r.total; intento++ {
		// La dirección 0 del bloque se reserva: en IPv6 suele ser la del router
		// y en IPv4 la de red.
		r.cursor++
		if r.cursor >= r.total {
			r.cursor = 1
		}

		ip := r.dir(r.cursor)
		u := r.usos[ip]
		if u == nil {
			u = &uso{}
			r.usos[ip] = u
		}
		if ahora.Sub(u.ultimoUso) >= r.opts.Ventana {
			u.cuenta = 0
			u.ultimoUso = ahora
		}
		if u.cuenta >= r.opts.MaxPorIP {
			continue
		}
		u.cuenta++
		u.ultimoUso = ahora
		return ip, true
	}
	return netip.Addr{}, false
}

// Estado resume el reparto actual. Sirve para .info y para los registros.
type Estado struct {
	Prefijo    string
	Total      uint64
	EnUso      int
	Agotadas   int
	MaxPorIP   int
	VentanaSeg int
}

// Estado calcula el resumen. El barrido es sobre las direcciones ya vistas, no
// sobre el prefijo completo, así que es barato.
func (r *Rotador) Estado(ahora time.Time) Estado {
	r.mu.Lock()
	defer r.mu.Unlock()

	e := Estado{
		Prefijo:    r.prefijo.String(),
		Total:      r.total,
		MaxPorIP:   r.opts.MaxPorIP,
		VentanaSeg: int(r.opts.Ventana.Seconds()),
	}
	for _, u := range r.usos {
		e.EnUso++
		if ahora.Sub(u.ultimoUso) < r.opts.Ventana && u.cuenta >= r.opts.MaxPorIP {
			e.Agotadas++
		}
	}
	return e
}

// Reiniciar olvida todos los conteos. Útil al cambiar de configuración.
func (r *Rotador) Reiniciar() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usos = make(map[netip.Addr]*uso, int(min(r.total, 1024)))
	r.cursor = 0
}

// Transporte devuelve un *http.Transport que sale por una dirección distinta en
// cada conexión. Si base es nil se parte de una configuración sana por defecto.
//
// Cada petición usa una conexión nueva (sin keep-alive) porque reutilizar una
// conexión reutiliza también la dirección de origen, que es justo lo que se
// quiere evitar aquí.
func (r *Rotador) Transporte(base *http.Transport) *http.Transport {
	t := base
	if t == nil {
		t = &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			MaxIdleConns:          64,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		}
	}
	t.DisableKeepAlives = true

	t.DialContext = func(ctx context.Context, red, dir string) (net.Conn, error) {
		ip, ok := r.Siguiente(time.Now())
		if !ok {
			return nil, ErrSinDirecciones
		}
		d := &net.Dialer{
			Timeout:   15 * time.Second,
			KeepAlive: -1,
			LocalAddr: &net.TCPAddr{IP: ip.AsSlice()},
		}
		return d.DialContext(ctx, red, dir)
	}
	return t
}

// ErrSinDirecciones se devuelve cuando el prefijo está agotado en la ventana.
var ErrSinDirecciones = fmt.Errorf("red: no quedan direcciones libres en la ventana")
