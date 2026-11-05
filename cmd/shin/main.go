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
	"syscall"
	"time"

	"go.mau.fi/whatsmeow/types/events"
	"shin-go/cmds" // sus init() registran los comandos

	"shin-go/internal/bot"
	"shin-go/internal/config"
	"shin-go/internal/store"
	"shin-go/internal/wa"
	"shin-go/internal/xlog"
)

var version = "0.8.1"

func main() {
	var (
		phone   = flag.String("code", "", "vincular con número (código de 8 dígitos)")
		showVer = flag.Bool("version", false, "mostrar versión")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("Shin-MD (Go)", version)
		return
	}

	cmds.SetVersion(version) // .info muestra la versión real del binario

	cfg := config.Load()
	log := xlog.New(cfg.LogLevel, os.Stdout)
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

	if err := client.Login(ctx, *phone); err != nil {
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
	log.Ok("Shin-MD en marcha. Ctrl+C para salir.")
	<-ctx.Done()
	log.Info("Cerrando limpio…")
}
