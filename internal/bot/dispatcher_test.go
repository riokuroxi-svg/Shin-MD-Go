// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// dispatcher_test.go — Lo que se prueba aquí es exactamente el fallo que motivó
// el archivo: un mensaje lento no puede dejar al bot sordo, un pánico no puede
// matar un carril y una cola llena tiene que descartar (y contarlo) en vez de
// bloquear la conexión.
//
// Todas las pruebas están pensadas para correr con `-race`.
package bot

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// mensaje con un identificador, que es lo único que el repartidor no toca.
func mensaje(id string) *events.Message {
	return &events.Message{Info: types.MessageInfo{ID: types.MessageID(id)}}
}

// ─── el caso central: lo lento no bloquea a lo rápido ───────────────────────

func TestUnMensajeLentoNoBloqueaALosDemas(t *testing.T) {
	const rapidosEsperados = 3

	var rapidos atomic.Int64
	lentoCorriendo := make(chan struct{})
	lentoTerminado := make(chan struct{})
	todosLosRapidos := make(chan struct{})

	d := NewDispatcher(func(ctx context.Context, evt *events.Message) {
		if evt.Info.ID == "lento" {
			close(lentoCorriendo)
			time.Sleep(400 * time.Millisecond)
			close(lentoTerminado)
			return
		}
		if rapidos.Add(1) == rapidosEsperados {
			close(todosLosRapidos)
		}
	}, WorkersPorDefecto, ColaPorDefecto, nil)
	d.Start(t.Context())
	defer d.Close()

	if !d.Submit(mensaje("lento")) {
		t.Fatal("no se pudo encolar el mensaje lento")
	}
	<-lentoCorriendo // el lento ya está ocupando un carril

	for i := 0; i < rapidosEsperados; i++ {
		if !d.Submit(mensaje(fmt.Sprintf("rapido-%d", i))) {
			t.Fatalf("no se pudo encolar el rápido %d", i)
		}
	}

	select {
	case <-todosLosRapidos:
		// bien: los tres terminaron mientras el lento seguía durmiendo
	case <-lentoTerminado:
		t.Fatal("los mensajes rápidos esperaron a que terminara el lento")
	case <-time.After(2 * time.Second):
		t.Fatal("los mensajes rápidos no se procesaron")
	}
}

// Un solo worker sí bloquea (así se comportaba el bot antes, que es lo que
// entrega whatsmeow por defecto). La prueba deja constancia de la diferencia.
func TestUnSoloWorkerSerializa(t *testing.T) {
	var segundo atomic.Bool
	empezo := make(chan struct{})

	d := NewDispatcher(func(ctx context.Context, evt *events.Message) {
		if evt.Info.ID == "primero" {
			close(empezo)
			time.Sleep(250 * time.Millisecond)
			return
		}
		segundo.Store(true)
	}, 1, 8, nil)
	d.Start(t.Context())
	defer d.Close()

	d.Submit(mensaje("primero"))
	<-empezo
	d.Submit(mensaje("segundo"))
	time.Sleep(100 * time.Millisecond)

	if segundo.Load() {
		t.Fatal("con un solo worker el segundo mensaje no debería haber empezado")
	}
	// Y al acabar el primero, el segundo pasa.
	time.Sleep(300 * time.Millisecond)
	if !segundo.Load() {
		t.Fatal("el segundo mensaje no se procesó al liberarse el worker")
	}
}

// ─── cola llena: se descarta y se cuenta, no se bloquea ─────────────────────

func TestColaLlenaDescartaSinBloquear(t *testing.T) {
	suelta := make(chan struct{})
	corriendo := make(chan struct{})

	d := NewDispatcher(func(ctx context.Context, evt *events.Message) {
		select {
		case <-corriendo:
		default:
			close(corriendo)
		}
		<-suelta // el worker se queda aquí
	}, 1, 1, nil) // un worker y un hueco de cola
	d.Start(t.Context())

	d.Submit(mensaje("en-curso"))
	<-corriendo
	d.Submit(mensaje("en-cola"))

	// A partir de aquí la cola está llena: Submit tiene que devolver false
	// inmediatamente, sin quedarse esperando.
	hecho := make(chan bool, 1)
	go func() { hecho <- d.Submit(mensaje("sobra")) }()
	select {
	case ok := <-hecho:
		if ok {
			t.Fatal("Submit aceptó un mensaje con la cola llena")
		}
	case <-time.After(time.Second):
		t.Fatal("Submit se bloqueó con la cola llena: eso es justo lo que no puede pasar")
	}

	st := d.Stats()
	if st.Descartados != 1 {
		t.Fatalf("descartados = %d, se esperaba 1", st.Descartados)
	}
	if st.Recibidos != 3 {
		t.Fatalf("recibidos = %d, se esperaba 3", st.Recibidos)
	}

	close(suelta)
	d.Close()
}

// ─── un pánico no se lleva un carril ────────────────────────────────────────

func TestPanicoNoMataElWorker(t *testing.T) {
	var despues atomic.Bool

	d := NewDispatcher(func(ctx context.Context, evt *events.Message) {
		if evt.Info.ID == "boom" {
			panic("handler roto a propósito")
		}
		despues.Store(true)
	}, 1, 8, nil) // un solo carril: si el pánico se lo lleva, no queda ninguno
	d.Start(t.Context())
	defer d.Close()

	d.Submit(mensaje("boom"))
	d.Submit(mensaje("tranqui"))

	limite := time.After(2 * time.Second)
	for !despues.Load() {
		select {
		case <-limite:
			t.Fatal("el worker murió con el pánico: el mensaje siguiente nunca se procesó")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Y el contador de procesados sigue cuadrando (el pánico cuenta como
	// procesado: se atendió, aunque fuera mal).
	limite = time.After(2 * time.Second)
	for d.Stats().Procesados < 2 {
		select {
		case <-limite:
			t.Fatalf("procesados = %d, se esperaban 2", d.Stats().Procesados)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// ─── cierre limpio ─────────────────────────────────────────────────────────

func TestCierreNoSeCuelga(t *testing.T) {
	var terminados atomic.Int64
	d := NewDispatcher(func(ctx context.Context, evt *events.Message) {
		time.Sleep(20 * time.Millisecond)
		terminados.Add(1)
	}, 2, 16, nil)
	d.Start(t.Context())

	for i := 0; i < 4; i++ {
		d.Submit(mensaje(fmt.Sprintf("m-%d", i)))
	}

	inicio := time.Now()
	d.Close()
	if dur := time.Since(inicio); dur > esperaCierre+time.Second {
		t.Fatalf("Close tardó %s", dur)
	}
	// Cerrar dos veces no pasa nada.
	d.Close()

	// Y después de cerrado, Submit no acepta nada (ni se cuelga).
	if d.Submit(mensaje("tarde")) {
		t.Fatal("Submit aceptó un mensaje con el repartidor cerrado")
	}
}

// La cancelación del contexto (Ctrl+C) también para los workers.
func TestContextoCanceladoParaLosWorkers(t *testing.T) {
	ctx, cancelar := context.WithCancel(context.Background())
	d := NewDispatcher(func(ctx context.Context, evt *events.Message) {}, 4, 8, nil)
	d.Start(ctx)

	salieron := make(chan struct{})
	go func() {
		d.wg.Wait()
		close(salieron)
	}()

	cancelar()
	select {
	case <-salieron:
	case <-time.After(2 * time.Second):
		t.Fatal("los workers no se detuvieron al cancelar el contexto")
	}
}

// ─── concurrencia bruta (esto es lo que mira -race) ────────────────────────

func TestSubmitYCierreConcurrentes(t *testing.T) {
	d := NewDispatcher(func(ctx context.Context, evt *events.Message) {
		time.Sleep(time.Millisecond)
	}, 4, 32, nil)
	d.Start(t.Context())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d.Submit(mensaje(fmt.Sprintf("m-%d", i)))
		}(i)
	}
	// Cerrar mientras otros siguen mandando: no puede haber pánico por
	// "send on closed channel" ni carrera en el puntero de la cola.
	go func() { time.Sleep(5 * time.Millisecond); d.Close() }()
	wg.Wait()
	d.Close()

	st := d.Stats()
	if st.Recibidos != 50 {
		t.Fatalf("recibidos = %d, se esperaban 50", st.Recibidos)
	}
	if st.Descartados > 50 {
		t.Fatalf("descartados = %d", st.Descartados)
	}
}

// ─── configuración y estadísticas ──────────────────────────────────────────

func TestNewDispatcherRecortaValoresAbsurdos(t *testing.T) {
	d := NewDispatcher(func(context.Context, *events.Message) {}, 0, 0, nil)
	st := d.Stats()
	if st.Workers != WorkersPorDefecto || st.Cola != ColaPorDefecto {
		t.Fatalf("valores por defecto = %+v", st)
	}

	d = NewDispatcher(func(context.Context, *events.Message) {}, 9999, 999999, nil)
	st = d.Stats()
	if st.Workers != workersMax || st.Cola != colaMax {
		t.Fatalf("los topes no se aplicaron: %+v", st)
	}
}

func TestSubmitNilNoCuenta(t *testing.T) {
	d := NewDispatcher(func(context.Context, *events.Message) {}, 1, 4, nil)
	d.Start(t.Context())
	defer d.Close()

	if !d.Submit(nil) {
		t.Fatal("Submit(nil) debería ser inocuo")
	}
	if st := d.Stats(); st.Recibidos != 0 {
		t.Fatalf("un mensaje nil contó como recibido: %+v", st)
	}
}
