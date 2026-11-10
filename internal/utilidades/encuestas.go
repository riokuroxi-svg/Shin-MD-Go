// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// encuestas.go — Interpretar `.encuesta` y decidir si es encuesta o quiz.
//
// En el JS esto estaba en `#lib/poll-plus` con su propio parser. Se porta la
// idea entera porque resuelve tres cosas de golpe:
//
//	.encuesta ¿Pizza?|Pepperoni|Hawaiana        → encuesta normal
//	.encuesta ¿Capital de Japón?|Kioto|*Tokio   → quiz (el * marca la buena)
//	.encuesta multi ¿Qué días?|Lunes|Martes     → se pueden votar varias
//
// El quiz lo corrige WhatsApp, no el bot: funciona aunque el bot esté caído.
package utilidades

import (
	"fmt"
	"strings"
)

// Límites de WhatsApp: menos de 2 opciones no es una encuesta y más de 12 no
// se muestran.
const (
	MinOpciones = 2
	MaxOpciones = 12
	// MaxPregunta es el tope real que acepta el servidor para el título.
	MaxPregunta = 255
	MaxOpcion   = 100
)

// TipoEncuesta distingue los dos formatos que entiende WhatsApp.
type TipoEncuesta string

const (
	TipoNormal TipoEncuesta = "encuesta"
	TipoQuiz   TipoEncuesta = "quiz"
)

// Encuesta es lo que hay que mandar, ya validado.
type Encuesta struct {
	Tipo           TipoEncuesta
	Pregunta       string
	Opciones       []string
	Correcta       int // índice de la opción correcta, -1 si no es quiz
	Seleccionables int // cuántas puede marcar quien vota
}

// ErrorEncuesta explica en cristiano por qué no se puede mandar.
type ErrorEncuesta struct {
	Motivo string
}

func (e *ErrorEncuesta) Error() string { return e.Motivo }

func fallo(motivo string) error { return &ErrorEncuesta{Motivo: motivo} }

// ParsearEncuesta interpreta la línea que escribió el usuario.
func ParsearEncuesta(texto string) (*Encuesta, error) {
	t := strings.TrimSpace(texto)
	if t == "" {
		return nil, fallo("escríbeme la encuesta.\n> Ejemplo: ¿Qué pizza?|Pepperoni|Hawaiana")
	}

	// «multi» / «multiple» / «varias» delante → se puede votar más de una.
	multiple := false
	if i := strings.IndexAny(t, " \t"); i > 0 {
		switch strings.ToLower(t[:i]) {
		case "multi", "múlti", "multiple", "múltiple", "varias", "varios":
			multiple = true
			t = strings.TrimSpace(t[i:])
		}
	}

	if !strings.Contains(t, "|") {
		return nil, fallo("separa la pregunta de las opciones con *|* (pleca).\n" +
			"> Ejemplo: ¿Qué pizza?|Pepperoni|Hawaiana")
	}

	partes := []string{}
	for _, p := range strings.Split(t, "|") {
		if p = strings.TrimSpace(p); p != "" {
			partes = append(partes, p)
		}
	}
	if len(partes) == 0 {
		return nil, fallo("falta la pregunta")
	}

	pregunta := partes[0]
	crudas := partes[1:]

	if pregunta == "" {
		return nil, fallo("falta la pregunta (lo que va antes de la primera pleca)")
	}
	if len([]rune(pregunta)) > MaxPregunta {
		return nil, fallo(fmt.Sprintf("la pregunta es demasiado larga (máximo %d caracteres)", MaxPregunta))
	}
	if len(crudas) < MinOpciones {
		return nil, fallo(fmt.Sprintf("hacen falta al menos *%d* opciones", MinOpciones))
	}
	if len(crudas) > MaxOpciones {
		return nil, fallo(fmt.Sprintf("como mucho *%d* opciones (WhatsApp no muestra más)", MaxOpciones))
	}

	// ¿Alguna opción marcada con asterisco? Entonces es un quiz.
	correcta := -1
	opciones := make([]string, 0, len(crudas))
	for i, o := range crudas {
		marcada := strings.HasPrefix(o, "*") || strings.HasSuffix(o, "*")
		limpia := strings.Trim(o, "* ")
		if limpia == "" {
			return nil, fallo(fmt.Sprintf("la opción %d está vacía", i+1))
		}
		if len([]rune(limpia)) > MaxOpcion {
			return nil, fallo(fmt.Sprintf("la opción %d es demasiado larga (máximo %d caracteres)", i+1, MaxOpcion))
		}
		if marcada {
			if correcta >= 0 {
				return nil, fallo("solo puede haber *una* respuesta correcta marcada con asterisco")
			}
			correcta = i
		}
		opciones = append(opciones, limpia)
	}

	if correcta >= 0 {
		// Un quiz siempre es de una sola respuesta: WhatsApp no admite otra cosa.
		return &Encuesta{
			Tipo:           TipoQuiz,
			Pregunta:       pregunta,
			Opciones:       opciones,
			Correcta:       correcta,
			Seleccionables: 1,
		}, nil
	}

	seleccionables := 1
	if multiple {
		seleccionables = len(opciones)
	}
	return &Encuesta{
		Tipo:           TipoNormal,
		Pregunta:       pregunta,
		Opciones:       opciones,
		Correcta:       -1,
		Seleccionables: seleccionables,
	}, nil
}
