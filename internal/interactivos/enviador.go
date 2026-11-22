// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// enviador.go — interfaz pequeña + cadena de estrategias. Ningún comando sabe
// si el mensaje salió directo, envuelto al estilo Baileys o como texto.
package interactivos

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
)

// Transport es la única pieza dependiente del socket. La implementa internal/wa
// y en pruebas se sustituye por una grabadora o por errores programados.
type Transport interface {
	Send(context.Context, types.JID, *waE2E.Message, []binary.Node) error
}

// IDTransport es la extensión opcional usada cuando una acción debe apuntar al
// mismo mensaje que la contiene. El ID lo genera whatsmeow, no el comando.
type IDTransport interface {
	SendWithID(context.Context, types.JID, *waE2E.Message, []binary.Node, string) error
}

func enviarTransportado(ctx context.Context, t Transport, jid types.JID, msg *waE2E.Message, nodes []binary.Node, messageID string) error {
	if messageID == "" {
		return t.Send(ctx, jid, msg, nodes)
	}
	conID, ok := t.(IDTransport)
	if !ok {
		return errors.New("el transporte no admite ID pre-generado")
	}
	return conID.SendWithID(ctx, jid, msg, nodes, messageID)
}

type Logger interface{ Warn(string) }

// Enviador es el puerto que usan wa.Client y, a través de bot.Messenger, todos
// los comandos. Menú, botones y lista son operaciones explícitas y estables.
type Enviador interface {
	SendMenu(context.Context, types.JID, string, string, string, ...Button) error
	SendButtons(context.Context, types.JID, string, string, string, *waE2E.ImageMessage, ...Button) error
	SendButtonsWithID(context.Context, types.JID, string, string, string, string, *waE2E.ImageMessage, ...Button) error
	SendList(context.Context, types.JID, string, string, string, string, ...ListSection) error
	SendCarousel(context.Context, types.JID, string, string, ...Card) error
}

// Estrategia es intercambiable: recibe la misma petición y o la entrega o
// devuelve error para que Cadena pruebe la siguiente.
type Estrategia interface {
	Nombre() string
	Enviar(context.Context, Transport, types.JID, Peticion) error
}

type estrategiaProto struct {
	nombre   string
	envolver bool
}

func (e estrategiaProto) Nombre() string { return e.nombre }
func (e estrategiaProto) Enviar(ctx context.Context, t Transport, jid types.JID, p Peticion) error {
	var (
		msg   *waE2E.Message
		flujo string
		err   error
	)
	if e.envolver {
		msg, flujo, err = construirBaileys(p)
	} else {
		msg, flujo, err = construirNativo(p)
	}
	if err != nil {
		return err
	}
	nodes, err := nodosBiz(jid, flujo, true)
	if err != nil {
		return err
	}
	return enviarTransportado(ctx, t, jid, msg, nodes, p.MessageID)
}

type estrategiaListaClasica struct{}

func (estrategiaListaClasica) Nombre() string { return "lista_clasica" }
func (estrategiaListaClasica) Enviar(ctx context.Context, t Transport, jid types.JID, p Peticion) error {
	msg, err := construirListaClasica(p)
	if err != nil {
		return err
	}
	// Sin AdditionalNodes: el whatsmeow fijado reconoce ListMessage y añade
	// exactamente un <biz><list v="2" type="product_list"/></biz>.
	return enviarTransportado(ctx, t, jid, msg, nil, p.MessageID)
}

type estrategiaTexto struct{}

func (estrategiaTexto) Nombre() string { return "texto" }
func (estrategiaTexto) Enviar(ctx context.Context, t Transport, jid types.JID, p Peticion) error {
	text := textoPlano(p)
	if strings.TrimSpace(text) == "" {
		return errors.New("fallback de texto vacío")
	}
	return enviarTransportado(ctx, t, jid, &waE2E.Message{Conversation: proto.String(text)}, nil, p.MessageID)
}

// Constructores públicos: permiten cambiar el orden en tests o en una futura
// configuración sin tocar los comandos.
func Baileys() Estrategia      { return estrategiaProto{nombre: "baileys", envolver: true} }
func Nativa() Estrategia       { return estrategiaProto{nombre: "nativa", envolver: false} }
func ListaClasica() Estrategia { return estrategiaListaClasica{} }
func Texto() Estrategia        { return estrategiaTexto{} }

// Estrategias interpreta una lista separada por comas. El orden por defecto
// pone primero el formato que se comparó byte a byte con Baileys. Si el .env
// omite texto, se añade al final como red de seguridad.
func Estrategias(orden string) []Estrategia {
	if strings.TrimSpace(orden) == "" {
		orden = "baileys,nativa,texto"
	}
	var out []Estrategia
	vistos := map[string]bool{}
	for _, raw := range strings.Split(orden, ",") {
		nombre := strings.ToLower(strings.TrimSpace(raw))
		switch nombre {
		case "baileys", "envuelto", "wrapped":
			nombre = "baileys"
		case "nativa", "nativo", "native":
			nombre = "nativa"
		case "texto", "text", "plain":
			nombre = "texto"
		default:
			continue
		}
		if vistos[nombre] {
			continue
		}
		vistos[nombre] = true
		switch nombre {
		case "baileys":
			out = append(out, Baileys())
		case "nativa":
			out = append(out, Nativa())
		case "texto":
			out = append(out, Texto())
		}
	}
	if !vistos["texto"] {
		out = append(out, Texto())
	}
	return out
}

type Cadena struct {
	transport   Transport
	log         Logger
	estrategias []Estrategia
}

func NuevaCadena(transport Transport, log Logger, estrategias ...Estrategia) *Cadena {
	if len(estrategias) == 0 {
		estrategias = Estrategias("")
	}
	return &Cadena{transport: transport, log: log, estrategias: estrategias}
}

func (c *Cadena) enviar(ctx context.Context, jid types.JID, p Peticion) error {
	if c == nil || c.transport == nil {
		return errors.New("enviador interactivo sin transporte")
	}
	var fallos []error
	for i, estrategia := range c.estrategias {
		if estrategia == nil {
			continue
		}
		err := estrategia.Enviar(ctx, c.transport, jid, p)
		if err == nil {
			return nil
		}
		fallos = append(fallos, fmt.Errorf("%s: %w", estrategia.Nombre(), err))
		if c.log != nil {
			for _, siguiente := range c.estrategias[i+1:] {
				if siguiente != nil {
					c.log.Warn("interactivo " + estrategia.Nombre() + " falló (" + err.Error() + "); probando " + siguiente.Nombre())
					break
				}
			}
		}
	}
	if len(fallos) == 0 {
		return errors.New("no hay estrategias interactivas")
	}
	return errors.Join(fallos...)
}

func (c *Cadena) SendMenu(ctx context.Context, jid types.JID, title, body, footer string, buttons ...Button) error {
	return c.enviar(ctx, jid, Peticion{Clase: ClaseMenu, Title: title, Body: body, Footer: footer, Buttons: buttons})
}

func (c *Cadena) SendButtons(ctx context.Context, jid types.JID, title, body, footer string, image *waE2E.ImageMessage, buttons ...Button) error {
	return c.enviar(ctx, jid, Peticion{Clase: ClaseBotones, Title: title, Body: body, Footer: footer, Image: image, Buttons: buttons})
}

func (c *Cadena) SendButtonsWithID(ctx context.Context, jid types.JID, messageID, title, body, footer string, image *waE2E.ImageMessage, buttons ...Button) error {
	if strings.TrimSpace(messageID) == "" {
		return errors.New("ID de mensaje vacío")
	}
	return c.enviar(ctx, jid, Peticion{Clase: ClaseBotones, MessageID: messageID, Title: title, Body: body, Footer: footer, Image: image, Buttons: buttons})
}

func (c *Cadena) SendList(ctx context.Context, jid types.JID, title, body, buttonText, footer string, sections ...ListSection) error {
	p := Peticion{Clase: ClaseLista, Title: title, Body: body, ButtonText: buttonText, Footer: footer, Sections: sections}
	if c == nil {
		return c.enviar(ctx, jid, p)
	}

	// Las dos estrategias de InteractiveMessage usan single_select y pueden
	// recibir ACK sin llegar al teléfono. Para listas, cada posición rica de la
	// configuración se colapsa en un único ListMessage clásico; texto conserva
	// su posición (INTERACTIVE_STRATEGIES=texto sigue forzando modo seguro).
	lista := &Cadena{transport: c.transport, log: c.log}
	yaAgregada := false
	for _, estrategia := range c.estrategias {
		if estrategia == nil {
			continue
		}
		if estrategia.Nombre() == "texto" {
			lista.estrategias = append(lista.estrategias, estrategia)
		} else if !yaAgregada {
			lista.estrategias = append(lista.estrategias, ListaClasica())
			yaAgregada = true
		}
	}
	return lista.enviar(ctx, jid, p)
}

func (c *Cadena) SendCarousel(ctx context.Context, jid types.JID, body, footer string, cards ...Card) error {
	return c.enviar(ctx, jid, Peticion{Clase: ClaseCarrusel, Body: body, Footer: footer, Cards: cards})
}
