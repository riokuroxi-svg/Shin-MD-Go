// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// letra.go — Puerto de cmds/utils/letra.js (lyrics.ovh, sin key) con el mismo
// troceo de ~3500 caracteres para que WhatsApp no corte el mensaje.
package apis

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// urlLyrics es variable para que las pruebas lo apunten a un httptest.Server.
var urlLyrics = "https://api.lyrics.ovh/v1"

// LetraMensajeMax es el techo de cada mensaje de letra (sin la cabecera).
const LetraMensajeMax = 3500

// Letra busca la letra de una canción. Error "no-encontrada" cuando el
// servicio no la tiene (contesta 404 o un JSON vacío).
func Letra(ctx context.Context, artista, cancion string) (string, error) {
	if strings.TrimSpace(artista) == "" || strings.TrimSpace(cancion) == "" {
		return "", fmt.Errorf("sin-datos")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		urlLyrics+"/"+url.PathEscape(artista)+"/"+url.PathEscape(cancion), nil)
	if err != nil {
		return "", err
	}
	resp, err := pedir(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("tiempo-agotado")
		}
		return "", fmt.Errorf("sin-red")
	}
	defer func() { _ = resp.Body.Close() }()
	cuerpo, pasado, err := leerTecho(resp.Body, 1<<20)
	if err != nil || pasado {
		return "", fmt.Errorf("respuesta-rara")
	}
	var d struct {
		Lyrics string `json:"lyrics"`
	}
	_ = json.Unmarshal(cuerpo, &d) // un 404 devuelve {"error":"…"}: cae aquí vacío
	letra := strings.TrimSpace(d.Lyrics)
	if resp.StatusCode != http.StatusOK || letra == "" {
		return "", fmt.Errorf("no-encontrada")
	}
	return letra, nil
}

// TrocearLetra divide la letra en mensajes de como mucho `max` caracteres,
// cortando por el salto de línea más cercano (como hacía el JS: si el salto
// está a más de 400 caracteres del límite, se corta a lo bruto).
func TrocearLetra(letra string, max int) []string {
	letra = strings.TrimSpace(letra)
	if letra == "" {
		return nil
	}
	if max < 400 {
		max = 400
	}
	var trozos []string
	resto := letra
	for len(resto) > max {
		corte := strings.LastIndex(resto[:max], "\n")
		i := max
		if corte > max-400 && corte > 0 {
			i = corte
		}
		trozos = append(trozos, resto[:i])
		resto = strings.TrimLeft(resto[i:], "\n")
	}
	if resto != "" {
		trozos = append(trozos, resto)
	}
	return trozos
}

// ParseLetraPeticion entiende los dos formatos del JS:
//
//	.letra Bad Bunny/Diles   → artista «Bad Bunny», canción «Diles»
//	.letra Bad Bunny Diles   → primera palabra artista, el resto canción
func ParseLetraPeticion(texto string) (artista, cancion string) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return "", ""
	}
	if strings.Contains(texto, "/") {
		partes := strings.SplitN(texto, "/", 2)
		return strings.TrimSpace(partes[0]), strings.TrimSpace(partes[1])
	}
	partes := strings.Fields(texto)
	if len(partes) < 2 {
		return "", ""
	}
	return partes[0], strings.Join(partes[1:], " ")
}
