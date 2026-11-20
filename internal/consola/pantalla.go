// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// pantalla.go — la cabecera fija arriba y el bot hablando abajo.
//
// Se dibuja en el BÚFER ALTERNATIVO de la terminal (\e[?1049h), que es lo que
// hace que este modo funcione de verdad en un móvil:
//
//	· la pantalla entera es nuestra, así que no hay «subir N líneas y borrar»:
//	  el encuadre va a la esquina y cada línea se limpia con \e[K. Nada se
//	  duplica ni deja restos aunque el terminal envuelva;
//	· si alguien escribe `clear`, cambia el tamaño de la ventana o el bot
//	  imprime algo suelto, al siguiente refresco todo vuelve a su sitio;
//	· al salir, la terminal del usuario recupera SU scrollback intacto: el bot
//	  no le contamina el historial con 400 cuadros.
//
// Sin terminal (systemd, pm2, docker logs, `> log.txt`) no se pinta NADA: el
// teletipo suelta líneas de texto normales, que es lo que quiere un VPS. Por
// eso el mismo binario sirve en Termux y en un servidor.
package consola

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/term"
)

// Pantalla es el modo panel: pinta cada segundo (y en cuanto llega un aviso).
type Pantalla struct {
	w        io.Writer
	cabecera func() []string
	pie      func() []string
	registro *Historial

	mu     sync.Mutex
	writer sync.Mutex // una pintura entera a la vez
	activa bool
	parar  chan struct{}
	hecho  chan struct{}
	ultima time.Time
	vigia  func() // deja de mirar SIGWINCH

	// medida: las columnas y filas con las que se dibuja. Es un campo en vez de
	// una llamada directa para poder ensayar la pantalla en 40 columnas (las de
	// Termux en vertical) desde `go test`, donde no hay terminal.
	medida func() (int, int)
}

// NuevaPantalla prepara el modo panel. `cabecera` son las filas de arriba (se
// recomienta Cabecera) y `pie` la ayuda de teclas; `registro` es el historial
// que el teletipo va llenando.
func NuevaPantalla(w io.Writer, cabecera func() []string, pie func() []string, registro *Historial) *Pantalla {
	return &Pantalla{
		w:        &escritorSeguro{w: w},
		cabecera: cabecera,
		pie:      pie,
		registro: registro,
		medida:   tamanoPantalla,
	}
}

// Arrancar entra en el búfer alternativo y repinta cada `cada`. Devuelvefalse
// si no hay pantalla donde pintar (entonces el llamador no hace nada raro).
func (p *Pantalla) Arrancar(cada time.Duration) bool {
	if !EsTerminal() {
		return false
	}
	return p.arrancar(cada)
}

// arrancar: el cuerpo, sin el filtro de terminal. Los tests del paquete lo
// llaman directo (en `go test` no hay tty y aun así hay que poder comprobar
// qué se escribe), y sólo él garantiza que el bucle se apaga con Parar.
func (p *Pantalla) arrancar(cada time.Duration) bool {
	p.mu.Lock()
	if p.activa {
		p.mu.Unlock()
		return true
	}
	p.activa = true
	p.parar = make(chan struct{})
	p.hecho = make(chan struct{})
	parar, hecho := p.parar, p.hecho
	p.vigia = vigilarTamaño(p.Refrescar)
	p.mu.Unlock()

	p.writer.Lock()
	entrarPantalla(p.w)
	p.pintarSinLock()
	p.writer.Unlock()

	go func() {
		defer close(hecho)
		t := time.NewTicker(cada)
		defer t.Stop()
		for {
			select {
			case <-parar:
				return
			case <-t.C:
				p.pintar()
			}
		}
	}()
	return true
}

// Refrescar fuerza un repintado (lo llama el teletipo al llegar un aviso y el
// watcher del tamaño de ventana). Con freno: repintar por cada línea de log en
// un móvil gasta batería y parpadea.
func (p *Pantalla) Refrescar() {
	p.mu.Lock()
	activa, ultima := p.activa, p.ultima
	if !activa || time.Since(ultima) < 120*time.Millisecond {
		p.mu.Unlock()
		return
	}
	p.ultima = time.Now()
	p.mu.Unlock()
	p.pintar()
}

// Ocultar sale del búfer alternativo sin apagar el refresco: el usuario ve los
// logs de siempre y, al pulsar v, el panel vuelve a su sitio.
//
// Se hace con el candado de escritura tomado a propósito. Si no, un repintado
// que ya estaba en vuelo (o el tick que cae en ese instante) escribía su cuadro
// en la terminal NORMAL, con sus «ve a la esquina» y sus «borra el resto»: el
// registro que uno acaba de bajar salía con líneas saltadas a media pantalla.
func (p *Pantalla) Ocultar() {
	p.writer.Lock()
	defer p.writer.Unlock()
	p.mu.Lock()
	if !p.activa {
		p.mu.Unlock()
		return
	}
	p.activa = false
	p.mu.Unlock()
	salirPantalla(p.w)
}

// Mostrar vuelve a entrar.
func (p *Pantalla) Mostrar() {
	p.writer.Lock()
	defer p.writer.Unlock()
	p.mu.Lock()
	if p.activa {
		p.mu.Unlock()
		return
	}
	p.activa = true
	nuevoVigia := p.vigia == nil
	if nuevoVigia {
		p.vigia = vigilarTamaño(p.Refrescar)
	}
	p.mu.Unlock()
	entrarPantalla(p.w)
	p.pintarSinLock()
}

// Activa: si el panel está ocupando la pantalla (lo pregunta el teletipo para
// saber si debe guardar los avisos o soltarlos).
func (p *Pantalla) Activa() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.activa
}

// Parar apaga el refresco y devuelve la terminal. Se puede llamar dos veces.
func (p *Pantalla) Parar() {
	p.mu.Lock()
	parar, hecho, vigia := p.parar, p.hecho, p.vigia
	p.parar, p.hecho, p.vigia = nil, nil, nil
	activa := p.activa
	p.activa = false
	p.mu.Unlock()

	// Primero se apaga el bucle y SE ESPERA a que termine, antes de tocar la
	// pantalla: si se hiciera al revés, Parar se quedaría esperando a un goroutine
	// bloqueado en el candado que él mismo está pidiendo (pantalla colgada al
	// cerrar, y en un servicio systemd mata a los 90 s con SIGKILL).
	if parar != nil {
		close(parar)
		if hecho != nil {
			select {
			case <-hecho:
			case <-time.After(2 * time.Second): // si no despierta, que se pueda cerrar igual
			}
		}
	}
	if vigia != nil {
		vigia()
	}
	if activa {
		p.writer.Lock()
		salirPantalla(p.w)
		p.writer.Unlock()
	}
}

// pintar compone el encuadre completo y lo escribe de una. Cada línea va
// truncada al ancho real: una línea que se sale hace scroll y descuadra todo
// (el fallo clásico en Termux).
func (p *Pantalla) pintar() {
	p.writer.Lock()
	defer p.writer.Unlock()
	p.pintarSinLock()
}

// pintarSinLock pinta con el candado ya tomado (lo llama arrancar/Mostrar, que
// necesitan escribir la entrada a la pantalla y el primer cuadro en la misma
// sección crítica).
func (p *Pantalla) pintarSinLock() {
	if !p.Activa() {
		return
	}
	ancho, filas := p.tamano()

	var cuerpo []string
	if p.cabecera != nil {
		cuerpo = append(cuerpo, p.cabecera()...)
	}
	if p.registro != nil {
		hueco := filas - len(cuerpo) - 3
		if hueco < 1 {
			hueco = 1
		}
		// Regla que separa el estado del registro, del ancho exacto de la
		// pantalla: más corta queda coja, más larga hace scroll.
		cuerpo = append(cuerpo, stTenue.Sprint(strings.Repeat("─", ancho)))
		for _, l := range p.registro.Ventana(hueco) {
			cuerpo = append(cuerpo, strings.TrimRight(l, "\r\n"))
		}
	}
	if p.pie != nil {
		cuerpo = append(cuerpo, p.pie()...)
	}

	// Si el contenido no cabe, sobra el registro por arriba: la cabecera y el
	// pie son lo que hay que ver siempre.
	if len(cuerpo) > filas {
		cuerpo = cuerpo[len(cuerpo)-filas:]
	}

	var b strings.Builder
	b.WriteString("\x1b[?2026h") // salida sincronizada: no se ve el barrido
	b.WriteString("\x1b[H")
	for _, l := range cuerpo {
		b.WriteString(truncarANSI(l, ancho))
		b.WriteString("\x1b[K\r\n")
	}
	b.WriteString("\x1b[J")
	b.WriteString("\x1b[?2026l")
	fmt.Fprint(p.w, b.String())
}

// tamano: las medidas con las que se va a dibujar.
func (p *Pantalla) tamano() (int, int) {
	if p.medida != nil {
		return p.medida()
	}
	return tamanoPantalla()
}

// escritorSeguro envuelve el escritor real. Hoy no añade nada (todo el dibujo
// pasa por el candado de Pantalla) pero deja un solo punto donde meter, si
// algún día hace falta, un \e[?2026 por escritura o un mutex con contexto.
type escritorSeguro struct {
	w io.Writer
}

func (e *escritorSeguro) Write(b []byte) (int, error) { return e.w.Write(b) }

// entrarPantalla / salirPantalla: el búfer alternativo, el cursor escondido y
// la pantalla limpia. Al salir, la terminal recupera lo suyo.
func entrarPantalla(w io.Writer) { fmt.Fprint(w, "\x1b[?1049h\x1b[?25l\x1b[H\x1b[2J") }
func salirPantalla(w io.Writer)  { fmt.Fprint(w, "\x1b[?25h\x1b[?1049l") }

// tamanoPantalla: columnas y filas reales (con sentido por si alguien mete la
// salida en un fichero: 100×30 es lo que cabría en una terminal razonable).
func tamanoPantalla() (int, int) {
	if term.IsTerminal(int(os.Stdout.Fd())) {
		if a, al, err := term.GetSize(int(os.Stdout.Fd())); err == nil && a > 0 && al > 0 {
			return encajar(a, 40, 240), encajar(al, 12, 200)
		}
	}
	return 100, 30
}
