// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// rich.go — Lo que WhatsApp añadió DESPUÉS de los botones clásicos, y que hoy
// ya está en el proto de whatsmeow:
//
//	· richResponseMessage  → respuestas enriquecidas tipo IA (markdown, código,
//	  tablas, imágenes, látex) que pintan con formato nativo en la app
//	· EventMessage         → evento de calendario (con recordatorio)
//	· PinInChatMessage     → fijar/desfijar un mensaje (PIN_FOR_ALL)
//	· PollAddOptionMessage → AÑADIR una opción a una encuesta ya creada
//
// ⚠️ Estado de verificación, sin vender humo:
//   - poll/pin/event: estructura del proto es clara, el envío es un mensaje
//     normal (texto o con nodos), se prueban en teléfono.
//   - rich AI: el proto está y whatsmeow hasta tiene un paquete para LEER esas
//     respuestas (types/richresponse, 2026). Se envía con el nodo <bot>; que la
//     app lo pinte desde una cuenta normal hay que VERIFICARLO en teléfono.
//     Por eso va detrás de RICH_AI=1 y siempre con fallback a texto.
package ui

import (
	"time"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/proto/waAICommonDeprecated"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// ─── Respuestas enriquecidas (estilo IA) ─────────────────────────────────────

func richMessage(subs ...*waAICommonDeprecated.AIRichResponseSubMessage) *waE2E.Message {
	return &waE2E.Message{
		RichResponseMessage: &waE2E.AIRichResponseMessage{
			MessageType: waAICommonDeprecated.AIRichResponseMessageType_AI_RICH_RESPONSE_TYPE_STANDARD.Enum(),
			Submessages: subs,
		},
	}
}

// RichText: bloque de texto con markdown nativo (negritas, listas, tablas…).
func RichText(markdown string) *waE2E.Message {
	return richMessage(&waAICommonDeprecated.AIRichResponseSubMessage{
		MessageType: waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_TEXT.Enum(),
		MessageText: proto.String(markdown),
	})
}

// RichCode: bloque de código con lenguaje (coloreado por la app).
func RichCode(language, code string) *waE2E.Message {
	return richMessage(&waAICommonDeprecated.AIRichResponseSubMessage{
		MessageType: waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_CODE.Enum(),
		CodeMetadata: &waAICommonDeprecated.AIRichResponseCodeMetadata{
			CodeLanguage: proto.String(language),
			CodeBlocks: []*waAICommonDeprecated.AIRichResponseCodeMetadata_AIRichResponseCodeBlock{
				{CodeContent: proto.String(code)},
			},
		},
	})
}

// RichTable: tabla nativa (título + filas; la primera fila es cabecera).
func RichTable(title string, headers []string, rows [][]string) *waE2E.Message {
	meta := &waAICommonDeprecated.AIRichResponseTableMetadata{Title: proto.String(title)}
	if len(headers) > 0 {
		meta.Rows = append(meta.Rows, &waAICommonDeprecated.AIRichResponseTableMetadata_AIRichResponseTableRow{
			Items:     headers,
			IsHeading: proto.Bool(true),
		})
	}
	for _, r := range rows {
		meta.Rows = append(meta.Rows, &waAICommonDeprecated.AIRichResponseTableMetadata_AIRichResponseTableRow{Items: r})
	}
	return richMessage(&waAICommonDeprecated.AIRichResponseSubMessage{
		MessageType:   waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_TABLE.Enum(),
		TableMetadata: meta,
	})
}

// RichTextCard: texto + código en un solo mensaje (lo que hace un bot "IA").
func RichTextCard(text, language, code string) *waE2E.Message {
	return richMessage(
		&waAICommonDeprecated.AIRichResponseSubMessage{
			MessageType: waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_TEXT.Enum(),
			MessageText: proto.String(text),
		},
		&waAICommonDeprecated.AIRichResponseSubMessage{
			MessageType: waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_CODE.Enum(),
			CodeMetadata: &waAICommonDeprecated.AIRichResponseCodeMetadata{
				CodeLanguage: proto.String(language),
				CodeBlocks: []*waAICommonDeprecated.AIRichResponseCodeMetadata_AIRichResponseCodeBlock{
					{CodeContent: proto.String(code)},
				},
			},
		},
	)
}

// ─── Evento de calendario ────────────────────────────────────────────────────

// BuildEvent: evento con inicio/fin y recordatorio opcional (lo avisa WhatsApp,
// no el bot). Funciona en chats y grupos.
func BuildEvent(name, description string, start, end time.Time, extraGuests bool, reminder time.Duration) *waE2E.Message {
	ev := &waE2E.EventMessage{
		Name:               proto.String(name),
		Description:        proto.String(description),
		StartTime:          proto.Int64(start.Unix()),
		EndTime:            proto.Int64(end.Unix()),
		ExtraGuestsAllowed: proto.Bool(extraGuests),
	}
	if reminder > 0 {
		ev.HasReminder = proto.Bool(true)
		ev.ReminderOffsetSec = proto.Int64(int64(reminder.Seconds()))
	}
	return &waE2E.Message{EventMessage: ev}
}

// ─── Pin / unpin de mensaje ──────────────────────────────────────────────────

// Key arma una MessageKey para pin, edición y respuestas. Vive junto a esas
// operaciones, no en el paquete de interactivos.
func Key(chat types.JID, fromMe bool, id types.MessageID, sender types.JID) *waCommon.MessageKey {
	k := &waCommon.MessageKey{
		RemoteJID: proto.String(chat.String()),
		FromMe:    proto.Bool(fromMe),
		ID:        proto.String(id),
	}
	if !sender.IsEmpty() {
		k.Participant = proto.String(sender.String())
	}
	return k
}

func BuildPin(key *waCommon.MessageKey, pin bool) *waE2E.Message {
	t := waE2E.PinInChatMessage_UNPIN_FOR_ALL
	if pin {
		t = waE2E.PinInChatMessage_PIN_FOR_ALL
	}
	return &waE2E.Message{
		PinInChatMessage: &waE2E.PinInChatMessage{
			Key:               key,
			Type:              t.Enum(),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}
}

// ─── Encuesta: añadir opción después de creada ───────────────────────────────

// BuildPollAddOption: añade una opción a una encuesta EN VIVO. whatsmeow no
// trae helper para esto (Baileys tampoco): el proto está, esto lo arma.
func BuildPollAddOption(pollKey *waCommon.MessageKey, option string) *waE2E.Message {
	return &waE2E.Message{
		PollAddOptionMessage: &waE2E.PollAddOptionMessage{
			PollCreationMessageKey: pollKey,
			AddOption:              &waE2E.PollCreationMessage_Option{OptionName: proto.String(option)},
		},
	}
}
