// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// gemini.go — Puerto de cmds/utils/chatgpt.js (Gemini vía GEMINI_KEY).
//
// DIFERENCIA CON EL JS: allí había memoria por chat en RAM (10 turnos), que
// una caída del proceso se llevaba igualmente. Aquí cada mensaje es una
// consulta de un solo turno: el estado que no sobrevive a un reinicio no es
// estado, es una mentira amable.
package apis

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// urlGemini es variable para que las pruebas lo apunten a un httptest.Server.
// %s = modelo.
var urlGemini = "https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent"

// PromptGemini es el system prompt del JS, palabra por palabra.
const PromptGemini = "Eres Shin-MD, un asistente inteligente de WhatsApp creado por riokuroxi-svg. " +
	"Responde siempre en español, de forma clara, educada, concisa y con formato markdown pulido. " +
	"Si te envían una imagen, descríbela con precisión."

// Gemini consulta el modelo. key vacía la comprueba el comando antes de
// llamar (aquí no llega). Los fallos se traducen a motivos entendibles:
// «key-invalida», «cupo» (429), «tiempo-agotado»…
func Gemini(ctx context.Context, key, model, texto, imagenB64, imagenMime string) (string, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" && imagenB64 == "" {
		return "", fmt.Errorf("sin-texto")
	}
	if texto == "" {
		texto = "Describe esta imagen."
	}

	type parte struct {
		Text   string `json:"text,omitempty"`
		Inline *struct {
			Mime string `json:"mime_type"`
			Data string `json:"data"`
		} `json:"inline_data,omitempty"`
	}
	cuerpo := map[string]any{
		"systemInstruction": map[string]any{"parts": []parte{{Text: PromptGemini}}},
	}
	p := parte{Text: texto}
	if imagenB64 != "" {
		p.Inline = &struct {
			Mime string `json:"mime_type"`
			Data string `json:"data"`
		}{Mime: imagenMime, Data: imagenB64}
	}
	cuerpo["contents"] = []map[string]any{{"role": "user", "parts": []parte{p}}}

	jsonCuerpo, _ := json.Marshal(cuerpo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf(urlGemini, model)+"?key="+key, bytes.NewReader(jsonCuerpo))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := clienteHTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("tiempo-agotado")
		}
		return "", fmt.Errorf("sin-red")
	}
	defer func() { _ = resp.Body.Close() }()
	datos, pasado, err := leerTecho(resp.Body, 2<<20)
	if err != nil || pasado {
		return "", fmt.Errorf("respuesta-rara")
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest, http.StatusForbidden:
		return "", fmt.Errorf("key-invalida")
	case http.StatusTooManyRequests:
		return "", fmt.Errorf("cupo")
	default:
		return "", fmt.Errorf("http-%d", resp.StatusCode)
	}
	return leerGemini(datos)
}

// leerGemini saca el texto de candidates[0].content.parts[].text, suelta para
// poder probarla con respuestas de mentira.
func leerGemini(datos []byte) (string, error) {
	var d struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(datos, &d) != nil || len(d.Candidates) == 0 {
		return "", fmt.Errorf("respuesta-rara")
	}
	var b strings.Builder
	for _, p := range d.Candidates[0].Content.Parts {
		b.WriteString(p.Text)
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "", fmt.Errorf("respuesta-rara")
	}
	return out, nil
}
