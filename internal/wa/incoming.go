// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// incoming.go — Anti-corruption layer: convierte eventos/protobuf externos a
// modelos internos antes de que el router o los comandos los reciban.
package wa

import (
	"strings"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// IncomingMessage es la entrada del router. raw queda privado al adaptador para
// operaciones posteriores de medios; no expone eventos ni mensajes Protobuf.
type IncomingMessage struct {
	Chat                 JID
	Sender               JID
	SenderAlt            JID
	ID                   string
	Timestamp            time.Time
	PushName             string
	IsFromMe             bool
	IsGroup              bool
	HasContent           bool
	IsGroupStatusMention bool

	Parsed   Parsed
	Mentions []JID
	Quoted   *QuotedMessage

	raw *waE2E.Message
}

// NormalizeMessage transforma el evento externo de WhatsApp a la entrada del
// núcleo. El adaptador conserva el payload crudo en un campo no exportado para
// poder descargar un medio más tarde, sin hacerlo parte del contrato público.
func NormalizeMessage(evt *events.Message, prefixes []string) *IncomingMessage {
	if evt == nil {
		return nil
	}
	info := evt.Info
	parsed := ParseMessage(evt.Message, prefixes)
	out := &IncomingMessage{
		Chat:       jidFromWhatsmeow(info.Chat),
		Sender:     jidFromWhatsmeow(info.Sender).ToNonAD(),
		SenderAlt:  jidFromWhatsmeow(info.SenderAlt).ToNonAD(),
		ID:         string(info.ID),
		Timestamp:  info.Timestamp,
		PushName:   info.PushName,
		IsFromMe:   info.IsFromMe,
		IsGroup:    info.Chat.Server == types.GroupServer,
		HasContent: evt.Message != nil,
		Parsed:     parsed,
		raw:        evt.Message,
	}
	if evt.Message == nil {
		return out
	}
	msg := Unwrap(evt.Message)
	out.IsGroupStatusMention = msg != nil && msg.GetGroupStatusMentionMessage() != nil
	ci := ContextInfo(msg)
	if ci == nil {
		return out
	}
	for _, raw := range ci.GetMentionedJID() {
		if jid, err := types.ParseJID(raw); err == nil {
			out.Mentions = append(out.Mentions, jidFromWhatsmeow(jid).ToNonAD())
		}
	}
	if quoted := ci.GetQuotedMessage(); quoted != nil {
		q := Unwrap(quoted)
		out.Quoted = &QuotedMessage{
			ID:                 ci.GetStanzaID(),
			Text:               quotedText(q),
			GroupStatusMention: q != nil && q.GetGroupStatusMentionMessage() != nil,
		}
		if p := ci.GetParticipant(); p != "" {
			if jid, err := types.ParseJID(p); err == nil {
				out.Quoted.Participant = jidFromWhatsmeow(jid).ToNonAD()
			}
		}
		out.Quoted.HasImage = q != nil && q.GetImageMessage() != nil
	}
	return out
}

func quotedText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	switch {
	case msg.GetConversation() != "":
		return strings.TrimSpace(msg.GetConversation())
	case msg.GetExtendedTextMessage() != nil:
		return strings.TrimSpace(msg.GetExtendedTextMessage().GetText())
	case msg.GetImageMessage() != nil:
		return strings.TrimSpace(msg.GetImageMessage().GetCaption())
	case msg.GetVideoMessage() != nil:
		return strings.TrimSpace(msg.GetVideoMessage().GetCaption())
	case msg.GetDocumentMessage() != nil:
		return strings.TrimSpace(msg.GetDocumentMessage().GetCaption())
	default:
		return ""
	}
}

func normalizeGroupChange(evt *events.GroupInfo) *GroupChange {
	if evt == nil {
		return nil
	}
	out := &GroupChange{JID: jidFromWhatsmeow(evt.JID), Timestamp: evt.Timestamp.Unix()}
	out.Join = jidsFromWhatsmeow(evt.Join)
	out.Leave = jidsFromWhatsmeow(evt.Leave)
	out.Promote = jidsFromWhatsmeow(evt.Promote)
	out.Demote = jidsFromWhatsmeow(evt.Demote)
	return out
}

func jidsFromWhatsmeow(in []types.JID) []JID {
	out := make([]JID, len(in))
	for i, jid := range in {
		out[i] = jidFromWhatsmeow(jid)
	}
	return out
}
