// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// protobuf.go — las dos representaciones intercambiables del mismo mensaje:
//
//	nativo   → Message.interactiveMessage
//	baileys  → Message.viewOnceMessage.message.{messageContextInfo,interactiveMessage}
//
// El segundo calca el patrón generateWAMessageFromContent + proto.Message que
// usan hoy los bots Baileys. No es "ver una vez" para el usuario: aquí el
// FutureProofMessage es el envoltorio de compatibilidad del protobuf.
package interactivos

import (
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

func flujo(buttons []Button) *waE2E.InteractiveMessage_NativeFlowMessage {
	nf := &waE2E.InteractiveMessage_NativeFlowMessage{
		// Es la versión DEL PROTO. El 9 pertenece al nodo binario
		// <native_flow v="9">; poner 9 aquí era mezclar dos capas distintas.
		MessageVersion: proto.Int32(1),
	}
	for _, b := range buttons {
		nf.Buttons = append(nf.Buttons, &waE2E.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
			Name:             proto.String(b.Name),
			ButtonParamsJSON: proto.String(b.Params),
		})
	}
	if len(buttons) > 3 {
		// WhatsApp enseña hasta tres en el hilo; los demás van a una hoja
		// inferior. Sin este campo los botones 4+ pueden desaparecer sin error.
		params := struct {
			BottomSheet struct {
				InThreadButtonsLimit int    `json:"in_thread_buttons_limit"`
				DividerIndices       []int  `json:"divider_indices"`
				ListTitle            string `json:"list_title"`
				ButtonTitle          string `json:"button_title"`
			} `json:"bottom_sheet"`
			HasMultipleButtons bool `json:"has_multiple_buttons"`
		}{HasMultipleButtons: true}
		params.BottomSheet.InThreadButtonsLimit = 3
		params.BottomSheet.DividerIndices = []int{}
		params.BottomSheet.ListTitle = "Más opciones"
		params.BottomSheet.ButtonTitle = "Ver todo"
		if raw, err := json.Marshal(params); err == nil {
			nf.MessageParamsJSON = proto.String(string(raw))
		}
	}
	return nf
}

func cabecera(title string, image *waE2E.ImageMessage) *waE2E.InteractiveMessage_Header {
	if title == "" && image == nil {
		return nil
	}
	h := &waE2E.InteractiveMessage_Header{
		Title:              proto.String(title),
		Subtitle:           proto.String(""),
		HasMediaAttachment: proto.Bool(image != nil),
	}
	if image != nil {
		h.Media = &waE2E.InteractiveMessage_Header_ImageMessage{ImageMessage: image}
	}
	return h
}

func pie(text string) *waE2E.InteractiveMessage_Footer {
	if text == "" {
		return nil
	}
	return &waE2E.InteractiveMessage_Footer{Text: proto.String(text)}
}

func interactivo(p Peticion) (*waE2E.InteractiveMessage, string, error) {
	switch p.Clase {
	case ClaseMenu, ClaseBotones:
		if len(p.Buttons) == 0 {
			return nil, "", fmt.Errorf("mensaje sin botones")
		}
		return &waE2E.InteractiveMessage{
			Header: cabecera(p.Title, p.Image),
			Body:   &waE2E.InteractiveMessage_Body{Text: proto.String(p.Body)},
			Footer: pie(p.Footer),
			InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
				NativeFlowMessage: flujo(p.Buttons),
			},
		}, "mixed", nil

	case ClaseLista:
		// single_select dentro de NativeFlow recibe ACK pero no se entrega de
		// forma fiable a cuentas normales. SendList usa ListMessage clásico con
		// el nodo product_list que genera whatsmeow.
		return nil, "", fmt.Errorf("la lista requiere la estrategia clásica")

	case ClaseCarrusel:
		if len(p.Cards) == 0 {
			return nil, "", fmt.Errorf("carrusel sin tarjetas")
		}
		cards := make([]*waE2E.InteractiveMessage, 0, len(p.Cards))
		for _, c := range p.Cards {
			if len(c.Buttons) == 0 {
				return nil, "", fmt.Errorf("tarjeta %q sin botones", c.Title)
			}
			cards = append(cards, &waE2E.InteractiveMessage{
				Header: cabecera(c.Title, c.Image),
				Body:   &waE2E.InteractiveMessage_Body{Text: proto.String(c.Body)},
				Footer: pie(""),
				InteractiveMessage: &waE2E.InteractiveMessage_NativeFlowMessage_{
					NativeFlowMessage: flujo(c.Buttons),
				},
			})
		}
		return &waE2E.InteractiveMessage{
			Header: cabecera(p.Title, p.Image),
			Body:   &waE2E.InteractiveMessage_Body{Text: proto.String(p.Body)},
			Footer: pie(p.Footer),
			InteractiveMessage: &waE2E.InteractiveMessage_CarouselMessage_{
				CarouselMessage: &waE2E.InteractiveMessage_CarouselMessage{
					Cards:            cards,
					MessageVersion:   proto.Int32(1),
					CarouselCardType: waE2E.InteractiveMessage_CarouselMessage_HSCROLL_CARDS.Enum(),
				},
			},
		}, "carousel", nil
	default:
		return nil, "", fmt.Errorf("clase interactiva desconocida: %d", p.Clase)
	}
}

func contextoBaileys() *waE2E.MessageContextInfo {
	return &waE2E.MessageContextInfo{
		DeviceListMetadata:        &waE2E.DeviceListMetadata{},
		DeviceListMetadataVersion: proto.Int32(2),
	}
}

// construirNativo deja interactiveMessage en el primer nivel. Es la estrategia
// limpia del proto y sirve también como plan B frente al envoltorio de Baileys.
func construirNativo(p Peticion) (*waE2E.Message, string, error) {
	im, flujoNombre, err := interactivo(p)
	if err != nil {
		return nil, "", err
	}
	return &waE2E.Message{
		MessageContextInfo: contextoBaileys(),
		InteractiveMessage: im,
	}, flujoNombre, nil
}

// construirBaileys reproduce campo por campo el contenido que se pasa a
// generateWAMessageFromContent en Baileys 7.0.0-rc14: el contexto y el
// InteractiveMessage viven DENTRO de viewOnceMessage.message.
func construirBaileys(p Peticion) (*waE2E.Message, string, error) {
	inner, flujoNombre, err := construirNativo(p)
	if err != nil {
		return nil, "", err
	}
	return &waE2E.Message{
		ViewOnceMessage: &waE2E.FutureProofMessage{Message: inner},
	}, flujoNombre, nil
}

func textoPlano(p Peticion) string {
	switch p.Clase {
	case ClaseLista:
		var b strings.Builder
		if p.Title != "" {
			b.WriteString(p.Title + "\n")
		}
		b.WriteString(p.Body)
		for _, sec := range p.Sections {
			b.WriteString("\n\n▪ " + sec.Title)
			for _, row := range sec.Rows {
				b.WriteString("\n  ▸ " + row.Title)
				if row.Desc != "" {
					b.WriteString(" — " + row.Desc)
				}
				if row.ID != "" {
					b.WriteString("  ·  " + comandoTexto(row.ID))
				}
			}
		}
		if p.Footer != "" {
			b.WriteString("\n\n" + p.Footer)
		}
		return b.String()
	case ClaseCarrusel:
		var b strings.Builder
		b.WriteString(p.Body)
		for _, card := range p.Cards {
			b.WriteString("\n\n━━ " + card.Title + " ━━\n" + card.Body)
			b.WriteString(botonesEnTexto(card.Buttons))
		}
		if p.Footer != "" {
			b.WriteString("\n\n" + p.Footer)
		}
		return b.String()
	default:
		var b strings.Builder
		if p.Title != "" {
			b.WriteString(p.Title + "\n")
		}
		b.WriteString(p.Body)
		b.WriteString(botonesEnTexto(p.Buttons))
		if p.Footer != "" {
			b.WriteString("\n\n" + p.Footer)
		}
		return b.String()
	}
}

// comandoTexto convierte el ID interno que devuelve un botón ("menu:economia")
// en la sintaxis que el parser acepta cuando una persona la escribe
// (".menu economia"). Mostrar el ID con dos puntos haría que el fallback
// pareciera accionable, pero el router buscaría un comando llamado
// "menu:economia".
func comandoTexto(id string) string {
	id = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(id), "."))
	if id == "" {
		return ""
	}
	if cmd, args, ok := strings.Cut(id, ":"); ok {
		id = strings.TrimSpace(cmd)
		if args = strings.TrimSpace(args); args != "" {
			id += " " + args
		}
	}
	return "." + id
}

func botonesEnTexto(buttons []Button) string {
	var b strings.Builder
	for _, button := range buttons {
		var params struct {
			DisplayText         string `json:"display_text"`
			Title               string `json:"title"`
			ID                  string `json:"id"`
			URL                 string `json:"url"`
			CopyCode            string `json:"copy_code"`
			PhoneNumber         string `json:"phone_number"`
			BusinessPhoneNumber string `json:"business_phone_number"`
		}
		_ = json.Unmarshal([]byte(button.Params), &params)
		label := params.DisplayText
		if label == "" {
			label = params.Title
		}
		if label == "" {
			label = button.Name
		}
		b.WriteString("\n▸ " + label)
		switch {
		case params.ID != "":
			b.WriteString("  ·  " + comandoTexto(params.ID))
		case params.URL != "":
			b.WriteString("  ·  " + params.URL)
		case params.CopyCode != "":
			b.WriteString("  ·  " + params.CopyCode)
		case params.PhoneNumber != "":
			b.WriteString("  ·  " + params.PhoneNumber)
		case params.BusinessPhoneNumber != "":
			b.WriteString("  ·  " + params.BusinessPhoneNumber)
		}
		if button.Name == "single_select" {
			var lista struct {
				Sections []struct {
					Rows []struct {
						ID    string `json:"id"`
						Title string `json:"title"`
					} `json:"rows"`
				} `json:"sections"`
			}
			if json.Unmarshal([]byte(button.Params), &lista) == nil {
				for _, sec := range lista.Sections {
					for _, row := range sec.Rows {
						b.WriteString("\n  • " + row.Title + "  ·  " + comandoTexto(row.ID))
					}
				}
			}
		}
	}
	return b.String()
}
