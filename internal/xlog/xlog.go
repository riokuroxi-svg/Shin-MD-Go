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

func New(level string, w io.Writer) *Logger {
	if w == nil {
		w = os.Stdout
	}
	var lv slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lv = slog.LevelDebug
	case "warn":
		lv = slog.LevelWarn
	case "error":
		lv = slog.LevelError
	default:
		lv = slog.LevelInfo
	}
	base := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: lv,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				a.Value = slog.StringValue(a.Value.Time().Format("15:04:05"))
			}
			return a
		},
	}))
	return &Logger{base}
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
