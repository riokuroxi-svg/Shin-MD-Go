// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// Paquete interactivos: el único lugar donde se describen botones, menús,
// listas y carruseles de WhatsApp. Los comandos solo conocen estos modelos y
// la interfaz Enviador; no construyen protobufs ni nodos binarios.
package interactivos

import (
	"encoding/json"

	"go.mau.fi/whatsmeow/proto/waE2E"
)

// Button es un botón native_flow: name + el JSON que WhatsApp interpreta.
type Button struct {
	Name   string
	Params string
}

func botonJSON(name string, params any) Button {
	raw, err := json.Marshal(params)
	if err != nil { // los params internos son structs serializables; defensa final
		raw = []byte("{}")
	}
	return Button{Name: name, Params: string(raw)}
}

func QuickReply(label, id string) Button {
	return botonJSON("quick_reply", struct {
		DisplayText string `json:"display_text"`
		ID          string `json:"id"`
	}{label, id})
}

func CTAURL(label, url string) Button {
	// El orden de los campos coincide con JSON.stringify en Baileys. JSON no
	// exige orden, pero mantenerlo permite comparar los bytes protobuf 1:1.
	return botonJSON("cta_url", struct {
		DisplayText string `json:"display_text"`
		URL         string `json:"url"`
		MerchantURL string `json:"merchant_url"`
	}{label, url, url})
}

func CTACopy(label, code string) Button {
	return botonJSON("cta_copy", struct {
		DisplayText string `json:"display_text"`
		CopyCode    string `json:"copy_code"`
	}{label, code})
}

func CTACall(label, phone string) Button {
	return botonJSON("cta_call", struct {
		DisplayText string `json:"display_text"`
		Phone       string `json:"phone_number"`
	}{label, phone})
}

func OpenWebview(title, url string) Button {
	return botonJSON("open_webview", struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	}{title, url})
}

func SendLocation(label string) Button {
	if label == "" {
		return Button{Name: "send_location", Params: "{}"}
	}
	return botonJSON("send_location", struct {
		DisplayText string `json:"display_text"`
	}{label})
}

func MPM(businessJID string) Button {
	return botonJSON("mpm", struct {
		BusinessPhoneNumber string `json:"business_phone_number"`
	}{businessJID})
}

func FlowButton(flowID, token, cta string) Button {
	return botonJSON("flow", struct {
		Version string `json:"flow_message_version"`
		Token   string `json:"flow_token"`
		ID      string `json:"flow_id"`
		CTA     string `json:"flow_cta"`
		Action  string `json:"flow_action"`
		Payload struct {
			Screen string `json:"screen"`
		} `json:"flow_action_payload"`
	}{
		Version: "3", Token: token, ID: flowID, CTA: cta, Action: "navigate",
		Payload: struct {
			Screen string `json:"screen"`
		}{Screen: "WELCOME"},
	})
}

// ListRow y ListSection describen tanto la lista pública como el botón
// single_select usado por las estrategias native_flow.
type ListRow struct {
	ID    string
	Title string
	Desc  string
}

type ListSection struct {
	Title string
	Rows  []ListRow
}

// Alias semánticos conservados para los comandos que crean single_select.
type SelectRow = ListRow
type SelectSection = ListSection

func SingleSelect(title string, sections ...SelectSection) Button {
	type rowJSON struct {
		ID          string `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description,omitempty"`
	}
	type sectionJSON struct {
		Title string    `json:"title"`
		Rows  []rowJSON `json:"rows"`
	}
	type paramsJSON struct {
		Title    string        `json:"title"`
		Sections []sectionJSON `json:"sections"`
	}
	out := paramsJSON{Title: title, Sections: make([]sectionJSON, 0, len(sections))}
	for _, s := range sections {
		sec := sectionJSON{Title: s.Title, Rows: make([]rowJSON, 0, len(s.Rows))}
		for _, r := range s.Rows {
			sec.Rows = append(sec.Rows, rowJSON{ID: r.ID, Title: r.Title, Description: r.Desc})
		}
		out.Sections = append(out.Sections, sec)
	}
	return botonJSON("single_select", out)
}

// Card es una tarjeta del carrusel horizontal.
type Card struct {
	Title   string
	Body    string
	Image   *waE2E.ImageMessage
	Buttons []Button
}

// Clase distingue la intención pública. Menú y botones comparten protobuf,
// pero conservar la intención hace que el fallback y las pruebas sean claros.
type Clase uint8

const (
	ClaseMenu Clase = iota + 1
	ClaseBotones
	ClaseLista
	ClaseCarrusel
)

// Peticion es la representación interna única de cualquier interactivo.
type Peticion struct {
	Clase      Clase
	Title      string
	Body       string
	Footer     string
	ButtonText string
	Image      *waE2E.ImageMessage
	Buttons    []Button
	Sections   []ListSection
	Cards      []Card
}
