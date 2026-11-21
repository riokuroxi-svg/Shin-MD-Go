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
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"shin-go/cmds" // sus init() registran los comandos

	"shin-go/internal/bot"
	"shin-go/internal/config"

	"shin-go/internal/consola"
	"shin-go/internal/red"
	"shin-go/internal/store"
	"shin-go/internal/wa"
	"shin-go/internal/web"
	"shin-go/internal/xlog"
)

var version = "0.24.0"

// numeroDeCodigo es el número pasado en --code, tal cual (se normaliza al
// usarlo). Vive aquí y no en una variable local de main porque la
// conversación de vinculación está en vinculacion.go.
var numeroDeCodigo string

func main() {
	// «shin start|stop|status|logs» no arrancan el bot: atienden la orden. Van
	// antes que el parseo de banderas porque en un VPS estas cuatro se escriben
	// solas, sin saberse de memoria qué flag hace qué.
	if len(os.Args) > 1 && ordenDeServicio(os.Args[1]) {
		os.Exit(servicio(os.Args[1], os.Args[2:]))
	}

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
	numeroDeCodigo = *phone

	if *showVer {
		fmt.Println("Shin-MD (Go)", version)
		return
	}

	fmt.Print(consola.Banner(version)) // seis líneas, siempre; sin arte de bloques

	cmds.SetVersion(version) // .info muestra la versión real del binario

	cfg := config.Load()
	// Los logs de la consola pasan por un teletipo: mientras el panel vivo
	// está en pantalla, se guardan para enseñarlos DENTRO del panel (si se
	// soltaran sueltos, romperían el dibujo y lo duplicarían en un móvil).
	// 400: con el panel en pantalla el usuario puede retroceder con las flechas
	// para releer, y en un móvil de 24 filas eso son ~16 pantallas de historial.
	historial := &consola.Historial{Max: 400}
	teletipo := consola.NuevoTeletipo(os.Stdout, historial)
	log := xlog.New(cfg.LogLevel, teletipo)

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
			log = xlog.NewDoble(cfg.LogLevel, teletipo, rot) // consola bonita + archivo grepeable
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

	// ── Vinculación ────────────────────────────────────────────────────
	// Antes de nada: quién es y cómo entra. Esta llamada es una BARRERA, no solo
	// «pedir el código»: sin sesión no vuelve hasta PairSuccess + Connected.
	// Todo lo que puede abrir el búfer alternativo (en especial la TUI) debe
	// quedarse DEBAJO. Así las ocho letras permanecen como texto quieto y
	// copiable; arrancar el panel entre PairPhone y Connected las taparía.
	if err := vincular(ctx, client, log); err != nil {
		if ctx.Err() != nil {
			log.Info("vinculación cancelada")
			return
		}
		log.Err("login: " + err.Error())
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
	despachador.SetGroupHandler(func(ctx context.Context, evt *wa.GroupChange) {
		bot.DispatchGroupInfo(ctx, db, log, evt)
	})
	despachador.Start(ctx)
	defer despachador.Close()
	bot.SetDispatcher(despachador)
	log.Info(fmt.Sprintf("Mensajes en paralelo: %d a la vez · %d en espera",
		cfg.Workers, cfg.QueueSize))
	client.OnMessage(func(ctx context.Context, evt *wa.IncomingMessage) {
		despachador.Submit(evt)
	})
	// Cambios de grupo (entradas, salidas, ascensos…): la bienvenida, la
	// despedida y el refresco de la caché de permisos.
	client.OnGroupInfo(func(ctx context.Context, evt *wa.GroupChange) {
		bot.InvalidateGroupInfo(evt)
		despachador.SubmitGroup(evt)
	})

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
			Conectado: client.Connected(), Version: version,
			Arranque:  client.StartedAt,
			SentToday: s.SentToday, Budget: s.Budget, ModoLento: s.Slowed,
			Anchor:  s.Anchor,
			Workers: d.Workers, EnCola: d.EnCola, ColaMax: d.Cola,
			Recibidos: int64(d.Recibidos), Procesados: int64(d.Procesados), Descartados: int64(d.Descartados),
			Activos: d.Activos, GrupoEnCola: d.GrupoEnCola, GrupoColaMax: d.GrupoCola,
			GrupoActivos:    d.GrupoActivos,
			GruposRecibidos: int64(d.GruposRecibidos), GruposProcesados: int64(d.GruposProcesados),
			GruposDescartados: int64(d.GruposDescartados),
			EsperaPromedioMS:  d.EsperaPromedio.Milliseconds(), EsperaMaximaMS: d.EsperaMaxima.Milliseconds(),
			TrabajoPromedioMS: d.DuracionPromedio.Milliseconds(), TrabajoMaximoMS: d.DuracionMaxima.Milliseconds(),
		}
	}, log.Info)
	if err := panel.Arrancar(ctx); err != nil {
		log.Warn("no se pudo iniciar el panel web: " + err.Error())
	}

	// ── La pantalla del bot ────────────────────────────────────────────
	// Cabecera fija arriba (conexión, cuenta, comandos, cupo, cola, disco,
	// memoria, panel web) y el registro del bot debajo, ocupando lo que sobre.
	// Se pinta en el búfer alternativo y se repinta cada segundo —y en cuanto
	// llega un aviso—, así que ni el `clear`, ni girar el teléfono, ni una
	// ventana que cambia de tamaño lo descuadran.
	//
	// Teclas:
	//
	//	e  bajar al registro (sale del panel; v vuelve)
	//	r  reconectar el socket de WhatsApp (sin perder la sesión)
	//	?  ayuda
	//	q  parar (igual que Ctrl+C)
	//
	// En un servicio, en un tubo o sin terminal no se dibuja nada: ahí el bot
	// escribe líneas de texto, que es lo que quiere journald, y se para con
	// «shin stop» o con el gestor de servicios — no con un Ctrl+C que no existe.
	foto := func() consola.Estado {
		s := client.Queue.Snapshot()
		d := despachador.Stats()
		return consola.Estado{
			Version: version, Cuenta: client.AccountName(),
			Conectado: client.Connected(), Prefijos: cfg.Prefixes,
			Comandos: uniq, Uptime: client.Uptime(),
			Enviados: s.SentToday, Cupo: s.Budget, EnCola: d.EnCola,
			Procesados: d.Procesados, Recibidos: d.Recibidos,
			Web: fmt.Sprintf("http://127.0.0.1:%d", cfg.WebPort),
		}
	}
	ayuda := false // con ? se mete un bloque de ayuda en el pie
	pantalla := consola.NuevaPantalla(os.Stdout,
		func() []string { return consola.Cabecera(foto()) },
		func() []string {
			pie := consola.Pie(consola.AnchoFijo())
			if ayuda {
				pie = append(pie,
					consola.Ayuda("  e registro / v panel · r reconectar · q parar"),
					consola.Ayuda("  shin stop desde otra sesión · shin status"),
					consola.Ayuda("  comandos: "+fmt.Sprintf("%d con prefijos [%s]", uniq, strings.Join(cfg.Prefixes, " "))),
				)
			}
			return pie
		},
		historial)

	// El teletipo guarda los avisos mientras el panel está en pantalla (y pide
	// un refresco: si no, un aviso tardaría hasta un segundo en aparecer). Al
	// bajar al registro con `e` se sueltan por stdout, como siempre.
	pintar := func() { pantalla.Refrescar() }
	teletipo.Activar(pantalla.Activa(), pintar)
	if pantalla.Arrancar(time.Second) {
		defer pantalla.Parar()
	}

	if consola.EsServicio() {
		// Sin terminal no hay pantalla que refrescar, así que el arranque tiene
		// que decir él solo lo que el panel diría: líneas sueltas, grepeables,
		// sin recuadros (un recuadro dentro de journald es ruido) y sin prometer
		// una tecla que no existe.
		log.Ok("Shin-MD-Go en marcha · sin teclado: se para con «shin stop»")
		for _, l := range consola.LineasEstado(foto(), 78) {
			fmt.Fprintln(os.Stdout, "  "+consola.SinColor(l))
		}
		fmt.Fprintln(os.Stdout, consola.Tenue("  shin status · shin logs -f · shin stop"))
	} else {
		log.Ok("Shin-MD-Go en marcha. q o Ctrl+C para parar.")
	}

	// `e` es una palanca, no dos órdenes: con el registro a la vista se vuelve
	// al panel con la misma tecla (y con v, por costumbre). Al bajar al
	// registro se sueltan los avisos acumulados —si no, la primera vez que se
	// pulsa la pantalla quedaría vacía, que es lo que parecía un fallo.
	viendoRegistro := false
	bajarAlRegistro := func() {
		viendoRegistro = true
		pantalla.Ocultar()
		consola.ImprimirHistorial(os.Stdout, historial.Ultimos(24))
		teletipo.Activar(false, nil)
	}
	subirAlPanel := func() {
		viendoRegistro = false
		historial.Abajo()
		pantalla.Mostrar()
		teletipo.Activar(true, pintar)
	}
	pararAtajos := consola.Atajos(ctx, map[string]func(){
		"e": func() {
			if viendoRegistro {
				subirAlPanel()
				return
			}
			bajarAlRegistro()
		},
		"v": subirAlPanel,
		"r": func() {
			ayuda = false
			log.Info("reconectando el socket de WhatsApp…")
			if err := client.Reconectar(ctx); err != nil {
				log.Warn("reconexión: " + err.Error())
			}
		},
		"?":      func() { ayuda = !ayuda; pantalla.Refrescar() },
		"q":      func() { stop() }, // cancela el contexto: todo cierra limpio
		"ctrl-c": func() { stop() }, // por si el terminal no deja mandar la señal
		// Flechas: releer el registro hacia atrás sin salir del panel.
		"up":   func() { historial.Atras(3); pantalla.Refrescar() },
		"down": func() { historial.Adelante(3); pantalla.Refrescar() },
	})
	defer pararAtajos()

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
