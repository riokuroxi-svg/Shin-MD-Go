// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// dispatcher.go — Reparte los mensajes entrantes entre varias gorutinas.
//
// El problema que resuelve, y que no es teórico:
//
// whatsmeow entrega los eventos a los handlers **de uno en uno**: su cola
// interna (2048 nodos) llama al handler y espera a que termine antes de coger
// el siguiente nodo. Con el handler pegado a la red, un solo `.sticker` de un
// vídeo (descargar + ffmpeg + subir ≈ 10–60 s) deja al bot sordo todo ese rato:
// los mensajes se van apilando hasta que la cola de WhatsApp revienta y los
// nodos se descartan. El síntoma en producción es "el bot se quedó mudo y
// luego contestó cosas viejas de golpe".
//
// La solución de manual es esta: pocas gorutinas fijas (no una por mensaje, que
// sería un vertedero de memoria) leyendo de una cola con techo. Si la cola se
// llena, se descarta el mensaje y se cuenta — mejor perder un comando suelto
// que bloquear la conexión entera y perder miles.
//
// Lo que NO se rompe al paralelizar:
//
//   - **El dinero.** Cada movimiento es una sentencia atómica en SQLite
//     (`WHERE coins >= ?`), no un leer-modificar-escribir. Dos apuestas del
//     mismo usuario a la vez se resuelven igual que si fueran seguidas.
//   - **El espaciado anti-ban.** Los envíos siguen pasando por la cola de
//     `wa.Queue`, que serializa y espera su retardo. Paralelizar el trabajo no
//     manda dos mensajes juntos: solo deja de hacer cola *antes* de enviar.
//   - **El orden dentro de una conversación** no está garantizado (dos mensajes
//     seguidos pueden contestarse al revés si uno tarda más). Es el precio de
//     no bloquear, y en este bot no hay comandos que dependan del orden.
package bot

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"go.mau.fi/whatsmeow/types/events"

	"shin-go/internal/xlog"
)

const (
	// WorkersPorDefecto: cuántos mensajes se procesan a la vez. Cuatro cubre de
	// sobra el caso normal (la mayoría de comandos tardan milisegundos) y deja
	// aire para dos o tres conversiones de medios sin que se pisen.
	WorkersPorDefecto = 4

	// ColaPorDefecto: cuántos mensajes pueden esperar. Con 4 workers, 128 en
	// cola son varios minutos de trabajo: si se llena es que algo se atascó de
	// verdad, no que haya prisa.
	ColaPorDefecto = 128

	// workersMax y colaMax acotan lo que se pueda configurar por entorno: 64
	// gorutinas contra SQLite (una sola conexión de escritura) no aceleran nada
	// y solo gastan memoria.
	workersMax = 16
	colaMax    = 1024
)

// Dispatcher es el repartidor de mensajes.
type Dispatcher struct {
	handle func(context.Context, *events.Message)

	// mu es de lectura/escritura: Submit lo toma COMPARTIDO mientras manda a la
	// cola y Close lo toma EN EXCLUSIVA para cerrarla. Sin eso, un Submit que
	// llegara justo en ese instante mandaría a un canal cerrado y el pánico se
	// llevaría por delante el proceso entero.
	mu      sync.RWMutex
	jobs    chan *events.Message
	log     *xlog.Logger
	workers int

	// Contadores (atómicos: los lee `.info` desde otra gorutina).
	recibidos   atomic.Uint64
	procesados  atomic.Uint64
	descartados atomic.Uint64
	activos     atomic.Int64

	cerrado atomic.Bool
	wg      sync.WaitGroup
}

// dispatcherActivo es el repartidor del proceso. Se guarda en un atómico
// porque lo lee `.info` desde un worker y lo escribe el arranque desde otra
// gorutina: un puntero normal ahí sería una carrera de las que solo se ven con
// `-race` (y en producción una vez cada mil reinicios).
var dispatcherActivo atomic.Pointer[Dispatcher]

// SetDispatcher registra el repartidor del proceso (lo llama main).
func SetDispatcher(d *Dispatcher) { dispatcherActivo.Store(d) }

// DispatcherActivo devuelve el repartidor (nil si aún no hay).
func DispatcherActivo() *Dispatcher { return dispatcherActivo.Load() }

// NewDispatcher crea el repartidor y arranca sus workers. `handle` es el
// router; `workers` y `cola` se recortan a valores razonables si vienen mal.
func NewDispatcher(handle func(context.Context, *events.Message), workers, cola int, log *xlog.Logger) *Dispatcher {
	if handle == nil {
		panic("bot.NewDispatcher: handler nil")
	}
	if workers <= 0 {
		workers = WorkersPorDefecto
	}
	if workers > workersMax {
		workers = workersMax
	}
	if cola <= 0 {
		cola = ColaPorDefecto
	}
	if cola > colaMax {
		cola = colaMax
	}
	return &Dispatcher{
		handle:  handle,
		jobs:    make(chan *events.Message, cola),
		workers: workers,
		log:     log,
	}
}

// Start lanza los workers. Se detienen cuando `ctx` se cancela (Ctrl+C) o
// cuando se llama a Close(); en los dos casos terminan el mensaje que tengan
// entre manos antes de salir, para no dejar una transacción a medias.
func (d *Dispatcher) Start(ctx context.Context) {
	// El canal se captura AQUÍ, no se lee del campo dentro del bucle: si el
	// worker leyera `d.jobs` en cada vuelta y Close lo pusiera a nil, se
	// quedaría esperando para siempre en un canal nil (recibir de nil bloquea
	// eternamente, y eso fue justo el primer bug de este archivo).
	d.mu.RLock()
	jobs := d.jobs
	d.mu.RUnlock()

	for i := 0; i < d.workers; i++ {
		d.wg.Add(1)
		go d.worker(ctx, jobs, i)
	}
}

func (d *Dispatcher) worker(ctx context.Context, jobs <-chan *events.Message, id int) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-jobs:
			if !ok {
				return
			}
			d.ejecutar(ctx, evt)
		}
	}
}

// ejecutar corre un mensaje con red de seguridad: si el handler entra en
// pánico, se registra y el worker sigue vivo. Sin este `recover`, un pánico
// mata la gorutina y el repartidor pierde un carril para siempre.
func (d *Dispatcher) ejecutar(ctx context.Context, evt *events.Message) {
	d.activos.Add(1)
	defer d.activos.Add(-1)

	defer func() {
		if rec := recover(); rec != nil {
			if d.log != nil {
				d.log.Err(fmt.Sprintf("pánico en el mensaje: %v\n%s", rec, debug.Stack()))
			}
		}
		d.procesados.Add(1)
	}()

	d.handle(ctx, evt)
}

// Submit encola un mensaje. Devuelve false si la cola está llena (y lo cuenta):
// el mensaje se pierde, pero la conexión sigue leyendo.
func (d *Dispatcher) Submit(evt *events.Message) bool {
	if evt == nil {
		return true
	}
	d.recibidos.Add(1)

	d.mu.RLock()
	jobs := d.jobs
	if jobs == nil || d.cerrado.Load() {
		d.mu.RUnlock()
		d.descartados.Add(1)
		return false
	}
	select {
	case jobs <- evt:
		d.mu.RUnlock()
		return true
	default:
		d.mu.RUnlock()
		d.descartados.Add(1)
		if d.log != nil && d.descartados.Load()%10 == 1 {
			// Se avisa de vez en cuando y no en cada mensaje: si la cola se
			// llenó una vez, el log no puede llenarse también.
			d.log.Warn(fmt.Sprintf("cola de mensajes llena: %d descartados en total", d.descartados.Load()))
		}
		return false
	}
}

// Close cierra la cola y espera a que los workers terminen lo que tienen entre
// manos. Llamarlo dos veces no pasa nada.
func (d *Dispatcher) Close() {
	if d.cerrado.Swap(true) {
		return
	}
	// Se cierra el canal pero NO se pone a nil: los workers ya tienen su copia
	// y un canal cerrado les devuelve ok=false. Ponerlo a nil solo servía para
	// que las estadísticas dijeran que no hay cola.
	d.mu.Lock()
	jobs := d.jobs
	if jobs != nil {
		close(jobs)
	}
	d.mu.Unlock()
	// Los workers que siguen esperando en `<-ctx.Done()` no salen con el canal
	// cerrado... bueno: sí, el `case evt, ok := <-jobs` con ok=false también
	// los saca. Así que esto espera a los dos casos.
	waitConTimeout(&d.wg)
}

// esperaCierre: cuánto se espera a que los workers acaben al apagar.
const esperaCierre = 5 * time.Second

// timeoutCierre devuelve el canal del reloj de cierre.
func timeoutCierre() <-chan time.Time { return time.After(esperaCierre) }

// waitConTimeout espera al WaitGroup, pero sin poder colgarse para siempre: si
// un handler se quedó atascado en una llamada de red, el proceso tiene que
// poder terminar igualmente.
func waitConTimeout(wg *sync.WaitGroup) {
	hecho := make(chan struct{})
	go func() {
		wg.Wait()
		close(hecho)
	}()
	select {
	case <-hecho:
	case <-timeoutCierre():
	}
}

// Estadisticas es la foto del repartidor (la pinta `.info`).
type Estadisticas struct {
	Workers     int
	Cola        int
	EnCola      int
	Activos     int64
	Recibidos   uint64
	Procesados  uint64
	Descartados uint64
}

// Stats devuelve los contadores.
func (d *Dispatcher) Stats() Estadisticas {
	if d == nil {
		return Estadisticas{}
	}
	d.mu.RLock()
	jobs := d.jobs
	d.mu.RUnlock()

	e := Estadisticas{
		Workers:     d.workers,
		Activos:     d.activos.Load(),
		Recibidos:   d.recibidos.Load(),
		Procesados:  d.procesados.Load(),
		Descartados: d.descartados.Load(),
	}
	if jobs != nil {
		e.Cola = cap(jobs)
		e.EnCola = len(jobs)
	}
	return e
}
