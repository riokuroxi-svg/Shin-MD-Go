// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// model.go — Modelos propios para los contratos entre el adaptador WhatsApp,
// el router y los comandos. La conversión a tipos de whatsmeow queda en este
// paquete; los modelos no dependen de Protobuf.
package wa

import (
	"fmt"
	"strconv"

	"go.mau.fi/whatsmeow/types"
)

const (
	DefaultUserServer = "s.whatsapp.net"
	GroupServer       = "g.us"
	LegacyUserServer  = "c.us"
	BroadcastServer   = "broadcast"
	HiddenUserServer  = "lid"
	HostedServer      = "hosted"
	HostedLIDServer   = "hosted.lid"
	NewsletterServer  = "newsletter"
)

// JID es la dirección mínima que el núcleo necesita: usuario y servidor. Los
// campos de dispositivo se preservan al convertir, pero no forman parte de la
// API de negocio.
type JID struct {
	User       string
	Server     string
	device     uint16
	rawAgent   uint8
	integrator uint16
}

func NewJID(user, server string) JID { return JID{User: user, Server: server} }

func ParseJID(raw string) (JID, error) {
	j, err := types.ParseJID(raw)
	if err != nil {
		return JID{}, err
	}
	return jidFromWhatsmeow(j), nil
}

func (j JID) String() string {
	if j.Server == "" {
		return ""
	}
	if j.rawAgent > 0 {
		return fmt.Sprintf("%s.%d:%d@%s", j.User, j.rawAgent, j.device, j.Server)
	}
	if j.device > 0 {
		return fmt.Sprintf("%s:%d@%s", j.User, j.device, j.Server)
	}
	if j.User != "" {
		return j.User + "@" + j.Server
	}
	return j.Server
}

func (j JID) IsEmpty() bool { return j.Server == "" }

func (j JID) ToNonAD() JID {
	j.device = 0
	j.rawAgent = 0
	return j
}

func (j JID) UserInt() uint64 {
	n, _ := strconv.ParseUint(j.User, 10, 64)
	return n
}

func jidFromWhatsmeow(j types.JID) JID {
	return JID{User: j.User, Server: j.Server, device: j.Device, rawAgent: j.RawAgent, integrator: j.Integrator}
}

func (j JID) whatsmeow() types.JID {
	return types.JID{User: j.User, Server: j.Server, Device: j.device, RawAgent: j.rawAgent, Integrator: j.integrator}
}

// GroupParticipant y GroupInfo son la foto de grupo consumida por la lógica de
// permisos y comandos. La adaptación desde types.GroupInfo ocurre en wa.Client.
type GroupParticipant struct {
	JID          JID
	PhoneNumber  JID
	LID          JID
	IsAdmin      bool
	IsSuperAdmin bool
}

type GroupInfo struct {
	JID          JID
	OwnerJID     JID
	Name         string
	Topic        string
	IsAnnounce   bool
	Participants []GroupParticipant
}

// GroupChange contiene únicamente los cambios que usa Shin; no expone el
// evento GroupInfo completo de la librería.
type GroupChange struct {
	JID       JID
	Join      []JID
	Leave     []JID
	Promote   []JID
	Demote    []JID
	Timestamp int64
}

// PublicProfile es el subconjunto público de datos que muestra .wastalk.
type PublicProfile struct {
	VerifiedName string
	Status       string
	LID          JID
}

// PollSpec es la solicitud interna para emitir una encuesta o quiz.
type PollSpec struct {
	Question   string
	Options    []string
	Selectable int
	Quiz       bool
	Correct    int
}

// MessageKey es la referencia interna necesaria para fijar un mensaje.
type MessageKey struct {
	Chat   JID
	Sender JID
	ID     string
	FromMe bool
}

type RichCard struct {
	Text     string
	Language string
	Code     string
}

// QuotedMessage representa los datos citados que consumen los comandos, sin
// exponer ContextInfo ni Message de Protobuf.
type QuotedMessage struct {
	ID                 string
	Participant        JID
	Text               string
	HasImage           bool
	GroupStatusMention bool
}
