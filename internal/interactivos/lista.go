// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// lista.go — ListMessage clásico para cuentas normales. No se envuelve y no
// añade AdditionalNodes: whatsmeow reconoce el mensaje y genera su único <biz>.
package interactivos

import (
	"crypto/rand"
	"fmt"
	"unicode/utf8"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func init() {
	// La revisión fijada de whatsmeow calcula <biz><list type="…"> leyendo
	// esta tabla exportada a partir del enum del payload. El payload debe seguir
	// siendo SINGLE_SELECT, pero las cuentas normales solo reciben la stanza
	// cuando el atributo dice product_list. Cambiar esta etiqueta aquí conserva
	// el protobuf correcto y evita mantener un fork completo por una sola línea.
	//
	// Es una inicialización previa a cualquier goroutine; no se modifica el mapa
	// durante el uso del cliente. La prueba de la lista fija ambas capas.
	waE2E.ListMessage_ListType_name[int32(waE2E.ListMessage_SINGLE_SELECT)] = "PRODUCT_LIST"
}

func stringsRunes(s string) int { return utf8.RuneCountInString(s) }

func construirListaClasica(p Peticion) (*waE2E.Message, error) {
	if p.Clase != ClaseLista {
		return nil, fmt.Errorf("la estrategia de lista no aplica a clase %d", p.Clase)
	}
	if stringsRunes(p.Body) == 0 {
		return nil, fmt.Errorf("lista sin descripción")
	}
	if stringsRunes(p.Title) > 60 {
		return nil, fmt.Errorf("título de lista supera 60 caracteres")
	}
	if stringsRunes(p.Body) > 1024 {
		return nil, fmt.Errorf("descripción de lista supera 1024 caracteres")
	}
	if stringsRunes(p.ButtonText) == 0 || stringsRunes(p.ButtonText) > 20 {
		return nil, fmt.Errorf("botón de lista debe tener entre 1 y 20 caracteres")
	}
	if stringsRunes(p.Footer) > 60 {
		return nil, fmt.Errorf("pie de lista supera 60 caracteres")
	}
	if len(p.Sections) == 0 || len(p.Sections) > 10 {
		return nil, fmt.Errorf("lista debe tener entre 1 y 10 secciones")
	}

	totalRows := 0
	sections := make([]*waE2E.ListMessage_Section, 0, len(p.Sections))
	for _, section := range p.Sections {
		if len(section.Rows) == 0 {
			return nil, fmt.Errorf("sección %q sin filas", section.Title)
		}
		if stringsRunes(section.Title) > 24 {
			return nil, fmt.Errorf("título de sección supera 24 caracteres")
		}
		totalRows += len(section.Rows)
		if totalRows > 10 {
			return nil, fmt.Errorf("lista supera el máximo de 10 filas")
		}

		rows := make([]*waE2E.ListMessage_Row, 0, len(section.Rows))
		for _, row := range section.Rows {
			if row.ID == "" || row.Title == "" {
				return nil, fmt.Errorf("fila de lista sin id o título")
			}
			if stringsRunes(row.ID) > 200 {
				return nil, fmt.Errorf("id de fila supera 200 caracteres")
			}
			if stringsRunes(row.Title) > 24 {
				return nil, fmt.Errorf("título de fila supera 24 caracteres")
			}
			if stringsRunes(row.Desc) > 72 {
				return nil, fmt.Errorf("descripción de fila supera 72 caracteres")
			}
			rows = append(rows, &waE2E.ListMessage_Row{
				RowID:       proto.String(row.ID),
				Title:       proto.String(row.Title),
				Description: proto.String(row.Desc),
			})
		}
		sections = append(sections, &waE2E.ListMessage_Section{
			Title: proto.String(section.Title),
			Rows:  rows,
		})
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("generar secreto de lista: %w", err)
	}
	return &waE2E.Message{
		ListMessage: &waE2E.ListMessage{
			Title:       proto.String(p.Title),
			Description: proto.String(p.Body),
			ButtonText:  proto.String(p.ButtonText),
			ListType:    waE2E.ListMessage_SINGLE_SELECT.Enum(),
			Sections:    sections,
			FooterText:  proto.String(p.Footer),
		},
		MessageContextInfo: &waE2E.MessageContextInfo{
			MessageSecret: secret,
		},
	}, nil
}
