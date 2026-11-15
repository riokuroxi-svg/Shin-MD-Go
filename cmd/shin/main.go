// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// main.go — Arranque. Equivale a index.js → boot/index.js en el JS.
//
//	shin                 → QR si no hay sesión
//	shin --code 521…     → código de 8 dígitos
//	shin --version
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.mau.fi/whatsmeow/types/events"
	"shin-go/cmds" // sus init() registran los comandos

	"shin-go/internal/bot"
	"shin-go/internal/config"
	"shin-go/internal/red"
	"shin-go/internal/store"
	"shin-go/internal/wa"
	"shin-go/internal/web"
	"shin-go/internal/xlog"
)

var version = "0.18.1"

func main() {
	var (
		phone   = flag.String("code", "", "vincular con número (código de 8 letras; sin número, lo pregunta)")
		showVer = flag.Bool("version", false, "mostrar versión")
	)
	// `--code` solo (sin número) = «vincula, aunque ya haya sesión»: el
	// paquete flag exige valor, así que un --code pelado se normaliza a
	// --code= antes de Parse; flag.Visit lo sigue contando como pasado.
	args := os.Args[1:]
	for i, a := range args {
		if (a == "--code" || a == "-code") && (i == len(args)-1 || strings.HasPrefix(args[i+1], "-")) {
			args[i] = a + "="
		}
	}
	_ = flag.CommandLine.Parse(args)

	if *showVer {
		fmt.Println("Shin-MD (Go)", version)
		return
	}

	cmds.SetVersion(version) // .info muestra la versión real del binario

	cfg := config.Load()
	log := xlog.New(cfg.LogLevel, os.Stdout)

	// Termux/Android: sin /etc/resolv.conf el resolvedor puro de Go pregunta
	// a 127.0.0.1:53 y [::1]:53, donde nadie escucha, y TODA la red muere
	// («lookup … on [::1]:53: connection refused»). Con CGO_ENABLED=0, que es
	// como se compila en Termux por el tema faccessat2/SIGSYS, eso es siempre.
	// Esto debe ir ANTES del primer uso de red (WhatsApp viene más abajo).
	if servidores := red.ArreglarDNSAndroid(); len(servidores) > 0 {
		log.Info("◑ DNS Android: resolución directa vía " + strings.Join(servidores, " · "))
	}

	// Y su gemelo: TLS. Con CGO_ENABLED=0 (así se compila en Termux por el
	// faccessat2), crypto/x509 solo mira /etc/ssl/… que en Android no existen:
	// sin esta env, incluso con DNS arreglado, WhatsApp contestaría
	// «x509: certificate signed by unknown authority» a todo.
	if pem, esTermux := red.ArreglarCertificadosAndroid(); pem != "" {
		log.Info("◑ TLS Android: raíces CA de " + pem)
	} else if esTermux {
		log.Warn("⚠ sin almacén de CA: ejecuta «pkg install ca-certificates» o TLS fallará")
	}

	// Log a archivo con rotación (4 MB × 3 backups): un bot que vive semanas
	// en Termux no puede fiarlo todo a la consola. LOG_FILE=off lo desactiva.
	// Si el disco está lleno o no hay permisos, se avisa y se sigue solo con
	// stdout: el log es un extra, no una razón para no arrancar.
	if cfg.LogFile != "" && !strings.EqualFold(cfg.LogFile, "off") {
		if rot, err := xlog.AbrirRotativo(cfg.LogFile, 4<<20, 3); err != nil {
			log.Warn("no se pudo abrir " + cfg.LogFile + ": " + err.Error())
		} else {
			log = xlog.New(cfg.LogLevel, io.MultiWriter(os.Stdout, rot))
			defer rot.Cerrar()
			log.Info("Log: " + cfg.LogFile + " (rotación 4 MB × 3)")
		}
	}
	if cfg.EnvFile != "" {
		log.Info("Configuración: .env cargado desde " + cfg.EnvFile)
	}

	// ctx se cancela con Ctrl+C → cierre limpio (adeós PM2 y sus reinicios sucios).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := store.Open(cfg.DBFile)
	if err != nil {
		log.Err("no se pudo abrir la base de datos: " + err.Error())
		os.Exit(1)
	}
	defer db.Close()

	client, err := wa.New(ctx, cfg, log, db)
	if err != nil {
		log.Err("no se pudo crear el cliente: " + err.Error())
		os.Exit(1)
	}
	defer client.Close()

	// ── Vinculación: la regla del JS, calcada ────────────────────────────
	// Un número escrito a mano (--code <num> / PAIRING_NUMBER) manda siempre
	// y no se pregunta dos veces. Con sesión registrada y nada forzado, no
	// se pregunta nada. Con terminal detrás —aunque vengas de «curl | sh»—
	// se pregunta el número. Sin terminal no se arranca a ciegas: se explica
	// la orden exacta y se sale, porque seguir acababa en un QR que nadie
	// pidió y que no se puede escanear desde el propio móvil.
	codePasado := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "code" {
			codePasado = true
		}
	})
	numero := wa.NormalizarTelefono(*phone)
	if numero == "" {
		numero = wa.NormalizarTelefono(os.Getenv("PAIRING_NUMBER"))
	}
	plan := wa.PlanDeVinculacion(client.LoggedIn(), codePasado, numero, wa.HayTerminal(), "no hay sesión")
	switch plan.Accion {
	case wa.AccionNada:
		log.Info("sesión existente: no hace falta vincular nada.")
	case wa.AccionUsar:
		numero = plan.Numero
		log.Info("vinculando con el número " + plan.Numero + ".")
	case wa.AccionPreguntar:
		wa.PanelVinculacion(os.Stdout)
		numero = wa.PreguntarNumero(os.Stdout)
		if numero == "" {
			log.Err("ese número no tiene dígitos usables.")
			os.Exit(1)
		}
		log.Info("vinculando con el número " + numero + ".")
	case wa.AccionSalir:
		wa.ExplicarSinTerminal(os.Stderr, plan.Motivo)
		os.Exit(1)
	}

	// El ancla del calentamiento (el día en que este número arrancó por primera
	// vez) se guarda en SQLite: si viviera en memoria, cada reinicio volvería a
	// 20 mensajes/día y el warm-up nunca llegaría a 500.
	// Limpieza de rolls caducados del gacha. En el JS cada roll quedaba
	// guardado dentro del JSON del chat para siempre; aquí se borran los que
	// caducaron hace más de un día.
	if borrados, err := db.PurgeRolls(ctx, time.Now()); err != nil {
		log.Warn("no se pudieron limpiar los rolls del gacha: " + err.Error())
	} else if borrados > 0 {
		log.Info(fmt.Sprintf("rolls caducados borrados: %d", borrados))
	}

	anchor, err := db.WarmupAnchor(ctx, time.Now())
	if err != nil {
		log.Warn("no se pudo leer el ancla del calentamiento: " + err.Error())
	}
	client.Queue.SetAnchor(anchor)

	// Los owners vienen de config (entorno + .env). Se normalizan a dígitos:
	// "+52 155 1234 5678" y "5215512345678" tienen que ser el mismo dueño.
	owners := bot.NormalizeOwners(cfg.Owners)
	if len(owners) == 0 {
		log.Warn("OWNERS está vacío: nadie podrá usar los comandos de owner")
	}

	uniq, aliases := bot.Count()
	log.Info(fmt.Sprintf("Comandos cargados: %d únicos · %d nombres · %d hooks", uniq, aliases, bot.HookCount()))

	router := bot.NewRouter(cfg, log, client, db, owners)

	// Los mensajes se atienden en varias gorutinas. whatsmeow entrega los
	// eventos de uno en uno: sin esto, un `.sticker` de vídeo (descarga +
	// ffmpeg + subida) dejaba al bot sordo hasta un minuto y los mensajes se
	// apilaban en su cola interna hasta descartarse.
	// Direcciones propias: si hay RED_PREFIJO, el tráfico saliente del bot se
	// reparte entre las direcciones del bloque. Sin prefijo, todo sigue igual.
	if r := prepararRed(cfg, log); r != nil {
		bot.SetRed(r)
	}

	// El vigilante de recordatorios necesita la base y el cliente: sin esto,
	// `.recordar` guardaría el aviso y nadie lo mandaría nunca.
	cmds.SetReferencia(db, client, log, ctx)

	despachador := bot.NewDispatcher(router.Handle, cfg.Workers, cfg.QueueSize, log)
	despachador.Start(ctx)
	defer despachador.Close()
	bot.SetDispatcher(despachador)
	log.Info(fmt.Sprintf("Mensajes en paralelo: %d a la vez · %d en espera",
		cfg.Workers, cfg.QueueSize))
	client.OnMessage(func(ctx context.Context, evt *events.Message) {
		despachador.Submit(evt)
	})
	// Cambios de grupo (entradas, salidas, ascensos…): la bienvenida, la
	// despedida y el refresco de la caché de permisos.
	client.OnGroupInfo(func(ctx context.Context, evt *events.GroupInfo) {
		bot.HandleGroupInfo(ctx, db, log, evt)
	})

	if err := client.Login(ctx, numero); err != nil {
		if ctx.Err() != nil { // Ctrl+C mientras se esperaba el QR
			log.Info("vinculación cancelada")
			return
		}
		log.Err("login: " + err.Error())
		os.Exit(1)
	}
	if ctx.Err() != nil {
		return
	}

	snap := client.Queue.Snapshot()
	log.Info(fmt.Sprintf("Calentamiento: desde %s · cupo de hoy %d mensajes no prioritarios",
		snap.Anchor.Format("02/01/2006"), snap.Budget))

	// Panel web local (B7): /health /queue /memoria /metrics. Escucha en
	// 127.0.0.1 salvo LOOPBACK=0 con PANEL_PASSWORD (failsafe en web.Nuevo).
	panel := web.Nuevo(web.Config{
		Puerto:   cfg.WebPort,
		Publico:  cfg.PanelPublic,
		Password: cfg.PanelPass,
	}, func() web.Estado {
		s := client.Queue.Snapshot()
		d := despachador.Stats()
		return web.Estado{
			Conectado: client.WA.IsConnected(), Version: version,
			Arranque:  client.StartedAt,
			SentToday: s.SentToday, Budget: s.Budget, ModoLento: s.Slowed,
			Anchor:  s.Anchor,
			Workers: d.Workers, EnCola: d.EnCola, ColaMax: d.Cola,
			Recibidos: int64(d.Recibidos), Procesados: int64(d.Procesados), Descartados: int64(d.Descartados),
		}
	}, log.Info)
	panel.Arrancar(ctx)

	log.Ok("Shin-MD en marcha. Ctrl+C para salir.")
	<-ctx.Done()
	log.Info("Cerrando limpio…")
}

// prepararRed monta el rotador de direcciones si hay prefijo configurado.
//
// Un prefijo mal escrito no tumba el bot: se avisa y se sigue saliendo por la
// dirección del sistema, que es lo que hacía antes de existir esta opción.
func prepararRed(cfg config.Config, log *xlog.Logger) *red.Rotador {
	if cfg.RedPrefijo == "" {
		return nil
	}
	r, err := red.Nuevo(cfg.RedPrefijo, red.Opciones{
		MaxPorIP: cfg.RedPorIP,
		Ventana:  cfg.RedVentana,
	})
	if err != nil {
		log.Warn("RED_PREFIJO no se pudo usar: " + err.Error())
		return nil
	}
	log.Info(fmt.Sprintf("Red propia: %s · %d direcciones · %d peticiones por dirección cada %s",
		r.Prefijo(), r.Usables(), cfg.RedPorIP, cfg.RedVentana))
	return r
}
