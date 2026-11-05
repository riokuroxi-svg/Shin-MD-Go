// Shin-MD-Go — esqueleto de migración de Shin-MD (JS) a Go.
// Copyright (C) 2026 riokuroxi-svg · SPDX-License-Identifier: AGPL-3.0-only
//
// config.go — configuración por variables de entorno, con los mismos
// nombres que ya usa Shin-MD para que el .env se reutilice tal cual.
//
// El bot de JS leía el .env solo (dotenv). Aquí se hace igual pero sin
// dependencias: si hay un archivo .env junto al binario se lee, y lo que
// venga exportado en el entorno tiene prioridad sobre el archivo.
//
//	env        .env       resultado
//	OWNERS=A   OWNERS=B   → A   (el entorno manda)
//	(sin)      OWNERS=B   → B
//	(sin)      (sin)      → valor por defecto
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	StateDir string // carpeta de datos (equivalente a ./data)
	DBFile   string // economía/perfil/settings
	AuthFile string // sesión de WhatsApp (antes auth.db / creds.json)

	Prefixes []string // BOT_PREFIX=".,/,!,+" (mismo .env)
	Owners   []string // OWNERS="521…,522…" — números crudos, se normalizan al usarlos

	LogLevel string
	RichAI   bool   // RICH_AI=1: respuestas enriquecidas estilo IA (experimental)
	EnvFile  string // ruta del .env que se leyó ("" si no había)

	// GachaFile: catálogo de personajes (mismo nombre de variable que el bot
	// de JS: CHARACTERS_FILE). Si el archivo no está, el gacha lo dice una vez
	// y sigue funcionando con el catálogo vacío en vez de romper.
	GachaFile   string // CHARACTERS_FILE (por defecto data/characters.json)
	BotName     string // BOT_NAME (por defecto "SHIN-MD")
	MenuImage   bool   // MENU_IMAGE=1: el .menu manda imagen de cabecera
	OwnerName   string // OWNER_NAME: cómo se llama el dueño en .owner
	OwnerNumber string // OWNER_NUMBER: número para el botón de llamada

	// Anti-ban (mismos parámetros que el motor JS)
	BaseDelay   time.Duration // 1200ms por defecto
	JitterSigma float64       // 0.25
	PerCharMS   int           // +25ms por carácter
	MaxDelay    time.Duration // tope del jitter (5s)
	WarmupStart int           // 20 mensajes/día
	WarmupEnd   int           // 500 mensajes/día a los 7 días
}

// variables es el entorno ya mezclado (sistema + .env).
type variables map[string]string

func (v variables) get(key string) string { return strings.TrimSpace(v[key]) }

func (v variables) str(key, def string) string {
	if got := v.get(key); got != "" {
		return got
	}
	return def
}

func (v variables) integer(key string, def int) int {
	if got := v.get(key); got != "" {
		if n, err := strconv.Atoi(got); err == nil {
			return n
		}
	}
	return def
}

func (v variables) float(key string, def float64) float64 {
	if got := v.get(key); got != "" {
		if f, err := strconv.ParseFloat(got, 64); err == nil {
			return f
		}
	}
	return def
}

// Load lee la configuración: primero el entorno del sistema y, para lo que
// falte, el .env (en la ruta de SHIN_ENV_FILE o el ./.env).
func Load() Config {
	path := strings.TrimSpace(os.Getenv("SHIN_ENV_FILE"))
	if path == "" {
		path = ".env"
	}
	return LoadFrom(path)
}

// LoadFrom hace lo mismo que Load pero con una ruta de .env explícita (lo usan
// las pruebas para no depender de dónde se ejecuten).
func LoadFrom(envPath string) Config {
	vars, used := merge(envPath)

	stateDir := vars.str("STATE_DIR", "data")

	prefixes := []string{".", "/", "#", "!"}
	if raw := vars.get("BOT_PREFIX"); raw != "" {
		prefixes = nil
		for _, p := range strings.Split(raw, ",") {
			if p = strings.TrimSpace(p); p != "" {
				prefixes = append(prefixes, p)
			}
		}
	}
	if len(prefixes) == 0 { // BOT_PREFIX con solo comas: no dejar al bot mudo
		prefixes = []string{"."}
	}

	owners := parseOwners(vars.get("OWNERS"))

	cfg := Config{
		StateDir: stateDir,
		DBFile:   vars.str("SHIN_DB_PATH", filepath.Join(stateDir, "shin.db")),
		AuthFile: vars.str("SHIN_AUTH_DB", filepath.Join(stateDir, "auth.db")),

		Prefixes:    prefixes,
		Owners:      owners,
		LogLevel:    vars.str("LOG_LEVEL", "info"),
		RichAI:      vars.str("RICH_AI", "0") == "1",
		EnvFile:     used,
		GachaFile:   vars.str("CHARACTERS_FILE", filepath.Join(stateDir, "characters.json")),
		BotName:     vars.str("BOT_NAME", "SHIN-MD"),
		MenuImage:   vars.str("MENU_IMAGE", "0") == "1",
		OwnerName:   vars.str("OWNER_NAME", "el dueño"),
		OwnerNumber: vars.str("OWNER_NUMBER", ""),

		BaseDelay:   time.Duration(vars.integer("ANTIBAN_BASE_MS", 1200)) * time.Millisecond,
		JitterSigma: vars.float("ANTIBAN_SIGMA", 0.25),
		PerCharMS:   vars.integer("ANTIBAN_PERCHAR_MS", 25),
		MaxDelay:    5 * time.Second,
		WarmupStart: vars.integer("WARMUP_START", 20),
		WarmupEnd:   vars.integer("WARMUP_END", 500),
	}

	// Un valor absurdo en el .env no debe romper el motor anti-ban: el suelo
	// del delay es 400 ms y el jitter no puede ser negativo.
	if cfg.BaseDelay < 400*time.Millisecond {
		cfg.BaseDelay = 400 * time.Millisecond
	}
	if cfg.JitterSigma < 0 {
		cfg.JitterSigma = 0
	}
	if cfg.PerCharMS < 0 {
		cfg.PerCharMS = 0
	}
	if cfg.WarmupStart < 1 {
		cfg.WarmupStart = 1
	}
	if cfg.WarmupEnd < cfg.WarmupStart {
		cfg.WarmupEnd = cfg.WarmupStart
	}
	return cfg
}

// parseOwners entiende las formas en que la gente escribe los números en el
// .env: "521…,522…", "+52 155 1234 5678" o una lista separada por espacios.
//
// El truco: primero se corta por comas y punto y coma (separadores claros) y
// solo se parte por espacios cuando CADA trozo ya parece un número completo
// (8 dígitos o más). Así "+52 155 1234 5678" es UN número y no cuatro, pero
// "5211111111111 5222222222222" sí son dos.
func parseOwners(raw string) []string {
	var out []string
	for _, chunk := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t'
	}) {
		pieces := strings.Fields(chunk)
		allFullNumbers := len(pieces) > 1
		for _, p := range pieces {
			if len(digitsOnly(p)) < 8 {
				allFullNumbers = false
				break
			}
		}
		if !allFullNumbers {
			if n := digitsOnly(chunk); n != "" {
				out = append(out, n)
			}
			continue
		}
		for _, p := range pieces {
			if n := digitsOnly(p); n != "" {
				out = append(out, n)
			}
		}
	}
	return out
}

// digitsOnly deja solo los dígitos (quita +, espacios, guiones y paréntesis).
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// merge junta el .env con el entorno del sistema (el entorno gana).
// Devuelve las variables y la ruta del .env que se usó ("" si no se leyó).
func merge(envPath string) (variables, string) {
	fileVars, used := parseEnvFile(envPath)

	out := make(variables, len(fileVars)+16)
	for k, v := range fileVars {
		out[k] = v
	}
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		// Una variable exportada pero VACÍA cuenta como no puesta: no debe
		// borrar lo que sí trae el .env (pasa al hacer `OWNERS= ./shin` para
		// "limpiarla" sin reiniciar la terminal).
		if strings.TrimSpace(kv[i+1:]) == "" {
			continue
		}
		out[kv[:i]] = kv[i+1:]
	}
	return out, used
}

// parseEnvFile lee un .env sencillo: KEY=valor, líneas con # de comentario,
// comillas opcionales ("..." o '...'), y `export` delante si viene del shell.
// No es un parser de shell completo: si algo no tiene forma de variable se
// ignora en silencio, que es lo que hace dotenv.
func parseEnvFile(path string) (map[string]string, string) {
	out := map[string]string{}
	if strings.TrimSpace(path) == "" {
		return out, ""
	}

	f, err := os.Open(path)
	if err != nil {
		return out, ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // una línea muy larga no debe cortar la lectura
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")

		i := strings.IndexByte(line, '=')
		if i <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		value := strings.TrimSpace(line[i+1:])
		if key == "" {
			continue
		}

		// Quita las comillas de fuera (y respeta un # dentro de ellas).
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		} else if j := strings.IndexByte(value, '#'); j >= 0 {
			value = strings.TrimSpace(value[:j])
		}
		out[key] = value
	}
	return out, path
}
