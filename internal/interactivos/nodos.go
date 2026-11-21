// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// nodos.go — metadatos de stanza que NO viven dentro del protobuf cifrado.
// Un protobuf perfecto sin este nodo puede recibir ACK de éxito y aun así no
// renderizar nada. Esta forma es la observada en tráfico oficial y la misma
// documentada para whatsmeow en tulir/whatsmeow#1235.
package interactivos

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/types"
)

func esPrivado(jid types.JID) bool {
	return jid.Server == types.DefaultUserServer || jid.Server == types.HiddenUserServer
}

// nodosBiz produce exactamente UN nodo biz por envío. Dos nodos biz hacen que
// el servidor rechace o descarte la stanza. decision_id se renueva siempre:
// son 20 bytes criptográficos, representados como 40 caracteres hexadecimales.
func nodosBiz(jid types.JID, nombreFlujo string, ai bool) ([]binary.Node, error) {
	decision := make([]byte, 20)
	if _, err := rand.Read(decision); err != nil {
		return nil, fmt.Errorf("generar decision_id: %w", err)
	}
	if nombreFlujo == "" {
		nombreFlujo = "mixed"
	}
	biz := binary.Node{
		Tag: "biz",
		Attrs: binary.Attrs{
			"actual_actors":   "2",
			"host_storage":    "2",
			"privacy_mode_ts": strconv.FormatInt(time.Now().Unix(), 10),
		},
		Content: []binary.Node{
			{
				Tag:   "interactive",
				Attrs: binary.Attrs{"type": "native_flow", "v": "1"},
				Content: []binary.Node{
					{Tag: "native_flow", Attrs: binary.Attrs{"v": "9", "name": nombreFlujo}},
				},
			},
			{
				Tag: "quality_control",
				Attrs: binary.Attrs{
					"decision_id": hex.EncodeToString(decision),
					"source_type": "third_party",
				},
				Content: []binary.Node{
					{Tag: "decision_source", Attrs: binary.Attrs{"value": "df"}},
				},
			},
		},
	}
	nodes := []binary.Node{biz}
	if ai && esPrivado(jid) {
		nodes = append(nodes, binary.Node{Tag: "bot", Attrs: binary.Attrs{"biz_bot": "1"}})
	}
	return nodes, nil
}

// BizNodes queda exportado para los pocos mensajes ricos que comparten el
// metadato de negocio. Los interactivos normales nunca lo llaman desde fuera:
// sus estrategias lo añaden por sí mismas, evitando duplicados.
func BizNodes(jid types.JID, ai bool) ([]binary.Node, error) {
	return nodosBiz(jid, "mixed", ai)
}
