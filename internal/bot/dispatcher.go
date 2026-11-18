// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// dispatcher.go — Cola acotada por conversación para mensajes y cambios de grupo.
// Los workers son fijos; las tareas de cada chat mantienen FIFO y chats distintos
// pueden avanzar en paralelo. El callback de whatsmeow solo añade una tarea y sale.
package bot

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"shin-go/internal/wa"

	"shin-go/internal/xlog"
)

const (
	WorkersPorDefecto = 4
	ColaPorDefecto    = 128
	workersMax        = 16
	colaMax           = 1024
)

type tipoTarea uint8

const (
	tareaMensaje tipoTarea = iota
	tareaGrupo
)

type tareaDispatcher struct {
	tipo    tipoTarea
	mensaje *wa.IncomingMessage
	grupo   *wa.GroupChange
	puesta  time.Time
}

type colaChat struct {
	clave  string
	tareas []tareaDispatcher
	activa bool
}

// Dispatcher usa un scheduler FIFO por chat con un pool fijo de workers.
// Cada tipo de evento tiene como máximo la capacidad de cola configurada; así
// los mensajes no expulsan cambios de grupo ni viceversa. No crea goroutines
// por evento y el ready channel solo guarda una referencia por chat activo.
type Dispatcher struct {
	handle      func(context.Context, *wa.IncomingMessage)
	handleGroup func(context.Context, *wa.GroupChange)
	log         *xlog.Logger
	workers     int
	colaMax     int
	grupoMax    int

	mu        sync.Mutex
	colas     map[string]*colaChat
	ready     chan *colaChat
	pendMsg   int
	pendGrupo int
	started   bool
	closed    bool
	ctx       context.Context
	stop      chan struct{}
	wg        sync.WaitGroup

	recibidos         atomic.Uint64
	procesados        atomic.Uint64
	descartados       atomic.Uint64
	gruposRecibidos   atomic.Uint64
	gruposProcesados  atomic.Uint64
	gruposDescartados atomic.Uint64
	activos           atomic.Int64
	gruposActivos     atomic.Int64
	tiempoTotalNS     atomic.Int64
	tiempoMaxNS       atomic.Int64
	esperaTotalNS     atomic.Int64
	esperaMaxNS       atomic.Int64
	esperaCount       atomic.Uint64
}

var dispatcherActivo atomic.Pointer[Dispatcher]

func SetDispatcher(d *Dispatcher)   { dispatcherActivo.Store(d) }
func DispatcherActivo() *Dispatcher { return dispatcherActivo.Load() }

func NewDispatcher(handle func(context.Context, *wa.IncomingMessage), workers, cola int, log *xlog.Logger) *Dispatcher {
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
		handle:   handle,
		log:      log,
		workers:  workers,
		colaMax:  cola,
		grupoMax: cola,
		colas:    make(map[string]*colaChat),
		ready:    make(chan *colaChat, cola*2),
		stop:     make(chan struct{}),
	}
}

// SetGroupHandler configura el trabajo lento de eventos de grupo antes de Start.
func (d *Dispatcher) SetGroupHandler(fn func(context.Context, *wa.GroupChange)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.started && !d.closed {
		d.handleGroup = fn
	}
}

func (d *Dispatcher) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	d.mu.Lock()
	if d.started || d.closed {
		d.mu.Unlock()
		return
	}
	d.started = true
	d.ctx = ctx
	d.wg.Add(d.workers)
	d.mu.Unlock()

	for i := 0; i < d.workers; i++ {
		go d.worker(ctx)
	}
}

func (d *Dispatcher) Submit(evt *wa.IncomingMessage) bool {
	if evt == nil {
		return true
	}
	d.recibidos.Add(1)
	key := evt.Chat.String()
	if evt.Chat.IsEmpty() {
		key = evt.Sender.String()
	}
	if key == "" {
		key = "<unknown>"
	}
	return d.enqueue(key, tareaDispatcher{tipo: tareaMensaje, mensaje: evt, puesta: time.Now()})
}

// SubmitGroup no ejecuta lógica de grupo en el callback de whatsmeow.
// El llamador debe invalidar antes la caché de permisos, que sí es trabajo corto.
func (d *Dispatcher) SubmitGroup(evt *wa.GroupChange) bool {
	if evt == nil {
		return true
	}
	d.gruposRecibidos.Add(1)
	key := evt.JID.String()
	if key == "" {
		key = "<unknown-group>"
	}
	return d.enqueue(key, tareaDispatcher{tipo: tareaGrupo, grupo: evt, puesta: time.Now()})
}

func (d *Dispatcher) enqueue(key string, task tareaDispatcher) bool {
	d.mu.Lock()
	if !d.started || d.closed || d.ctx.Err() != nil {
		d.mu.Unlock()
		d.descarta(task)
		return false
	}
	if task.tipo == tareaGrupo && d.handleGroup == nil {
		d.mu.Unlock()
		d.descarta(task)
		return false
	}
	if task.tipo == tareaMensaje && d.pendMsg >= d.colaMax {
		d.mu.Unlock()
		d.descarta(task)
		d.avisaColaLlena(task.tipo)
		return false
	}
	if task.tipo == tareaGrupo && d.pendGrupo >= d.grupoMax {
		d.mu.Unlock()
		d.descarta(task)
		d.avisaColaLlena(task.tipo)
		return false
	}
	q := d.colas[key]
	if q == nil {
		q = &colaChat{clave: key}
		d.colas[key] = q
	}
	q.tareas = append(q.tareas, task)
	if task.tipo == tareaMensaje {
		d.pendMsg++
	} else {
		d.pendGrupo++
	}
	if !q.activa {
		q.activa = true
		d.ready <- q // cap(ready) cubre todos los pendientes posibles
	}
	d.mu.Unlock()
	return true
}

func (d *Dispatcher) worker(ctx context.Context) {
	defer d.wg.Done()
	for {
		select {
		case <-ctx.Done():
			d.descartaTodoPendiente()
			return
		case <-d.stop:
			return
		case q := <-d.ready:
			if q == nil {
				continue
			}
			d.mu.Lock()
			if d.closed || len(q.tareas) == 0 {
				d.mu.Unlock()
				continue
			}
			task := q.tareas[0]
			q.tareas[0] = tareaDispatcher{} // liberar el evento antes de compactar
			q.tareas = q.tareas[1:]
			if task.tipo == tareaMensaje {
				d.pendMsg--
			} else {
				d.pendGrupo--
			}
			d.mu.Unlock()

			waitNS := time.Since(task.puesta).Nanoseconds()
			d.esperaCount.Add(1)
			d.esperaTotalNS.Add(waitNS)
			for old := d.esperaMaxNS.Load(); waitNS > old && !d.esperaMaxNS.CompareAndSwap(old, waitNS); old = d.esperaMaxNS.Load() {
			}
			if ctx.Err() != nil {
				d.descarta(task)
				d.finTarea(q, ctx)
				continue
			}
			d.ejecuta(ctx, task)
			d.finTarea(q, ctx)
		}
	}
}

func (d *Dispatcher) finTarea(q *colaChat, ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed || ctx.Err() != nil {
		d.descartaPendientes(q)
		delete(d.colas, q.clave)
		return
	}
	if len(q.tareas) == 0 {
		q.activa = false
		delete(d.colas, q.clave)
		return
	}
	// Mantener la cola del chat activa y volver a ponerla al final del ready
	// permite que otros chats reciban un turno entre tareas consecutivas.
	d.ready <- q
}

func (d *Dispatcher) ejecuta(ctx context.Context, task tareaDispatcher) {
	inicio := time.Now()
	d.activos.Add(1)
	if task.tipo == tareaGrupo {
		d.gruposActivos.Add(1)
	}
	defer func() {
		d.activos.Add(-1)
		d.tiempoTotalNS.Add(time.Since(inicio).Nanoseconds())
		maxNS := time.Since(inicio).Nanoseconds()
		for old := d.tiempoMaxNS.Load(); maxNS > old && !d.tiempoMaxNS.CompareAndSwap(old, maxNS); old = d.tiempoMaxNS.Load() {
		}
		if task.tipo == tareaGrupo {
			d.gruposActivos.Add(-1)
			d.gruposProcesados.Add(1)
		} else {
			d.procesados.Add(1)
		}
		if rec := recover(); rec != nil && d.log != nil {
			d.log.Err(fmt.Sprintf("pánico en tarea del dispatcher: %v\n%s", rec, debug.Stack()))
		}
	}()
	if task.tipo == tareaGrupo {
		d.handleGroup(ctx, task.grupo)
	} else {
		d.handle(ctx, task.mensaje)
	}
}

func (d *Dispatcher) descarta(task tareaDispatcher) {
	if task.tipo == tareaGrupo {
		d.gruposDescartados.Add(1)
	} else {
		d.descartados.Add(1)
	}
}

func (d *Dispatcher) descartaTodoPendiente() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, q := range d.colas {
		d.descartaPendientes(q)
	}
	d.colas = make(map[string]*colaChat)
}

func (d *Dispatcher) descartaPendientes(q *colaChat) {
	for i, task := range q.tareas {
		d.descarta(task)
		if task.tipo == tareaMensaje {
			d.pendMsg--
		} else {
			d.pendGrupo--
		}
		q.tareas[i] = tareaDispatcher{}
	}
	q.tareas = nil
	q.activa = false
}

func (d *Dispatcher) avisaColaLlena(kind tipoTarea) {
	if d.log == nil {
		return
	}
	if kind == tareaGrupo {
		n := d.gruposDescartados.Load()
		if n == 1 || n%10 == 0 {
			d.log.Warn(fmt.Sprintf("cola de eventos de grupo llena: %d descartados", n))
		}
		return
	}
	n := d.descartados.Load()
	if n == 1 || n%10 == 0 {
		d.log.Warn(fmt.Sprintf("cola de mensajes llena: %d descartados", n))
	}
}

const esperaCierre = 5 * time.Second

// Close deja de aceptar tareas, cuenta y libera las pendientes, y espera a que
// terminen las activas. Los handlers reciben el contexto del proceso y deben
// respetarlo para que el cierre sea rápido.
func (d *Dispatcher) Close() {
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return
	}
	d.closed = true
	close(d.stop)
	for _, q := range d.colas {
		d.descartaPendientes(q)
	}
	d.colas = make(map[string]*colaChat)
	d.mu.Unlock()
	waitConTimeout(&d.wg)
}

func waitConTimeout(wg *sync.WaitGroup) {
	hecho := make(chan struct{})
	go func() {
		wg.Wait()
		close(hecho)
	}()
	select {
	case <-hecho:
	case <-time.After(esperaCierre):
	}
}

type Estadisticas struct {
	Workers           int
	Cola              int
	EnCola            int
	Activos           int64
	Recibidos         uint64
	Procesados        uint64
	Descartados       uint64
	GrupoCola         int
	GrupoEnCola       int
	GrupoActivos      int64
	GruposRecibidos   uint64
	GruposProcesados  uint64
	GruposDescartados uint64
	DuracionPromedio  time.Duration
	DuracionMaxima    time.Duration
	EsperaPromedio    time.Duration
	EsperaMaxima      time.Duration
}

func (d *Dispatcher) Stats() Estadisticas {
	if d == nil {
		return Estadisticas{}
	}
	d.mu.Lock()
	msgQueue, groupQueue := d.pendMsg, d.pendGrupo
	d.mu.Unlock()
	procesados := d.procesados.Load() + d.gruposProcesados.Load()
	e := Estadisticas{
		Workers:           d.workers,
		Cola:              d.colaMax,
		EnCola:            msgQueue,
		Activos:           d.activos.Load(),
		Recibidos:         d.recibidos.Load(),
		Procesados:        d.procesados.Load(),
		Descartados:       d.descartados.Load(),
		GrupoCola:         d.grupoMax,
		GrupoEnCola:       groupQueue,
		GrupoActivos:      d.gruposActivos.Load(),
		GruposRecibidos:   d.gruposRecibidos.Load(),
		GruposProcesados:  d.gruposProcesados.Load(),
		GruposDescartados: d.gruposDescartados.Load(),
		DuracionMaxima:    time.Duration(d.tiempoMaxNS.Load()),
		EsperaMaxima:      time.Duration(d.esperaMaxNS.Load()),
	}
	if procesados > 0 {
		e.DuracionPromedio = time.Duration(d.tiempoTotalNS.Load() / int64(procesados))
	}
	if n := d.esperaCount.Load(); n > 0 {
		e.EsperaPromedio = time.Duration(d.esperaTotalNS.Load() / int64(n))
	}
	return e
}

func chatKey(chat wa.JID) string {
	if chat.IsEmpty() {
		return "<unknown>"
	}
	return chat.String()
}
