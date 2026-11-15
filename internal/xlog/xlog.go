// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// xlog.go — logger estructurado con los mismos iconos que ya usa Shin-MD
// (◐ ◑ ✓ ✕ ⚠) pero sobre log/slog de la biblioteca estándar. Cero deps.
package xlog

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/term"

	"shin-go/internal/consola"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger envuelve slog con métodos cortos e iconos.
type Logger struct{ *slog.Logger }

func nivelDe(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// handlerConsola — la cara de los logs en una terminal de verdad, sabor JS:
//
//	09:28:24  consola lista
//
// sin el ruido «time= level= msg=» de slog. El color solo toca el reloj y
// el icono que el propio mensaje ya trae (✓ ⚠ ✕); a archivo va el handler
// de texto completo, porque un log sí es para grep, no para mirar.
type handlerConsola struct {
	min slog.Level
	w   io.Writer
}

func (h handlerConsola) Enabled(_ context.Context, l slog.Level) bool { return l >= h.min }

func (h handlerConsola) Handle(_ context.Context, r slog.Record) error {
	hora := r.Time.Format("15:04:05")
	msg := r.Message
	tag := "INFO"
	switch {
	case r.Level >= slog.LevelError, strings.HasPrefix(msg, "✕"):
		tag = "ERROR"
		msg = strings.TrimPrefix(msg, "✕ ")
		msg = consola.Rojo(msg)
	case r.Level >= slog.LevelWarn, strings.HasPrefix(msg, "⚠"):
		tag = "AVISO"
		msg = strings.TrimPrefix(msg, "⚠ ")
		msg = consola.Ambar(msg)
	case strings.HasPrefix(msg, "✓"):
		tag = "OK"
		msg = strings.TrimPrefix(msg, "✓ ")
		msg = consola.Menta(msg)
	case r.Level <= slog.LevelDebug:
		tag = "DEBUG"
		msg = consola.Tenue(msg)
	}
	// Lo poco que lleve atributos (un Ctx) va compacto al final, no en
	// columna: en pantalla manda leerlo, no alinearlo.
	r.Attrs(func(a slog.Attr) bool {
		msg += " " + consola.Tenue(a.Key+"="+a.Value.String())
		return true
	})
	consola.Prefijo(h.w, hora, tag, msg)
	return nil
}

func (h handlerConsola) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h handlerConsola) WithGroup(_ string) slog.Handler      { return h }

func handlerTexto(lv slog.Level, w io.Writer) slog.Handler {
	return slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: lv,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("15:04:05"))
			}
			return a
		},
	})
}

func New(level string, w io.Writer) *Logger {
	if w == nil {
		w = os.Stdout
	}
	lv := nivelDe(level)
	if f, ok := w.(*os.File); ok && consola.ConColor() && term.IsTerminal(int(f.Fd())) {
		return &Logger{slog.New(handlerConsola{min: lv, w: w})}
	}
	return &Logger{slog.New(handlerTexto(lv, w))}
}

// fanout reparte cada registro a los dos handlers (consola bonita y archivo
// con formato completo). WithAttrs clona ambos, que es lo que espera slog.
type fanout []slog.Handler

func (f fanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f fanout) Handle(ctx context.Context, r slog.Record) error {
	var primero error
	for _, h := range f {
		if !h.Enabled(ctx, r.Level) {
			continue
		}
		if err := h.Handle(ctx, r.Clone()); err != nil && primero == nil {
			primero = err
		}
	}
	return primero
}

func (f fanout) WithAttrs(a []slog.Attr) slog.Handler {
	f2 := make(fanout, len(f))
	for i, h := range f {
		f2[i] = h.WithAttrs(a)
	}
	return f2
}

func (f fanout) WithGroup(g string) slog.Handler {
	f2 := make(fanout, len(f))
	for i, h := range f {
		f2[i] = h.WithGroup(g)
	}
	return f2
}

// NewDoble: consola legible Y archivo completo a la vez (log a disco con
// rotación). En pantalla manda leer; en archivo manda grepear.
func NewDoble(level string, consolaW io.Writer, archivoW io.Writer) *Logger {
	if consolaW == nil {
		consolaW = os.Stdout
	}
	lv := nivelDe(level)
	return &Logger{slog.New(fanout{
		handlerConsola{min: lv, w: consolaW},
		handlerTexto(lv, archivoW),
	})}
}

func (l *Logger) Debug(m string) { l.Logger.Debug("◐ " + m) }
func (l *Logger) Info(m string)  { l.Logger.Info("◑ " + m) }
func (l *Logger) Ok(m string)    { l.Logger.Info("✓ " + m) }

// Warn y Err además guardan el mensaje en el anillo que lee `.health`: el log
// del sistema no sirve cuando el bot se cae a las 3 de la mañana y lo único
// que tienes a mano es WhatsApp.
func (l *Logger) Warn(m string) {
	Record("aviso", m)
	l.Logger.Warn("⚠ " + m)
}

func (l *Logger) Err(m string) {
	Record("error", m)
	l.Logger.Error("✕ " + m)
}

// Ctx devuelve un logger hijo con datos del mensaje (equivalente a los
// campos estructurados que ya manejas en pino/chalk).
func (l *Logger) Ctx(ctx context.Context) *Logger { return l }

var _ = context.Background
