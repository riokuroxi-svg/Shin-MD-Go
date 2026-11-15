// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// voz.go — Puerto de cmds/utils/tts.js (texto → nota de voz).
//
// CAMBIO DE MOTOR RESPECTO AL JS
// El JS hablaba el protocolo WebSocket de Edge ReadAloud (token + Sec-MS-GEC
// rotatorio); ese «ticket» no documentado caduca y cuando Microsoft lo cambia
// el comando muere sin avisar. Aquí se usa el endpoint simple de Google
// Traductor (client=tw-ob, un GET por trozo): voz distinta, pero es un GET
// con 20 años a la espalda y cero secretos. Los MP3 se concatenan a lo bruto,
// marco a marco, que es válido para MP3.
package apis

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

// urlVoz es variable para que las pruebas lo apunten a un httptest.Server.
var urlVoz = "https://translate.google.com/translate_tts"

const (
	// VozMaxTotal: cuánto texto se acepta por comando (el audio resultante
	// ronda el minuto; más es lata para el grupo).
	VozMaxTotal = 1000
	// vozTrozo: el endpoint corta peticiones largas; 180 bytes deja margen
	// para texto multibyte sin pasar el límite de ~200 caracteres.
	vozTrozo = 180
)

// Voz convierte texto a MP3. Cada trozo es UNA petición; si una falla se
// aborta entera (una voz a la que le falta una frase miente).
func Voz(ctx context.Context, texto, idioma string) ([]byte, error) {
	texto = strings.TrimSpace(texto)
	if texto == "" {
		return nil, fmt.Errorf("sin-texto")
	}
	if idioma = ResolverIdioma(idioma); idioma == "" {
		idioma = "es"
	}
	runas := []rune(texto)
	if len(runas) > VozMaxTotal {
		texto = string(runas[:VozMaxTotal])
	}

	var mp3 bytes.Buffer
	for _, trozo := range TrocearVoz(texto, vozTrozo) {
		datos, err := pedirVoz(ctx, trozo, idioma)
		if err != nil {
			return nil, err
		}
		mp3.Write(datos)
	}
	if mp3.Len() < 500 { // un «audio» de 100 bytes es un error disfrazado
		return nil, fmt.Errorf("audio-vacio")
	}
	return mp3.Bytes(), nil
}

func pedirVoz(ctx context.Context, texto, idioma string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		urlVoz+"?ie=UTF-8&client=tw-ob&tl="+url.QueryEscape(idioma)+
			"&q="+url.QueryEscape(texto), nil)
	if err != nil {
		return nil, err
	}
	// Este endpoint no contesta sin cabeceras de navegador.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Shin-MD)")
	req.Header.Del("Accept")
	resp, err := clienteHTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("tiempo-agotado")
		}
		return nil, fmt.Errorf("sin-red")
	}
	defer func() { _ = resp.Body.Close() }()
	datos, pasado, err := leerTecho(resp.Body, 4<<20)
	if err != nil || pasado || len(datos) < 200 {
		return nil, fmt.Errorf("respuesta-rara")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http-%d", resp.StatusCode)
	}
	return datos, nil
}

// TrocearVoz parte texto en runas sin pasar de `maxBytes` por trozo,
// cortando por el último espacio disponible y sin romper UTF-8 por la mitad
// (un MP3 con medio carácter suena a trabaula).
func TrocearVoz(texto string, maxBytes int) []string {
	if maxBytes < 40 {
		maxBytes = 40
	}
	var trozos []string
	resto := strings.TrimSpace(texto)
	for len(resto) > maxBytes {
		trozo := resto[:maxBytes]
		// No cortar una runa a la mitad: retroceder hasta un borde válido.
		for !utf8.ValidString(trozo) {
			trozo = trozo[:len(trozo)-1]
		}
		// Preferir cortar por espacio para no morder una palabra.
		if i := strings.LastIndexByte(trozo, ' '); i > maxBytes/2 {
			trozo = trozo[:i]
		}
		trozos = append(trozos, strings.TrimSpace(trozo))
		resto = strings.TrimSpace(resto[len(trozo):])
	}
	if resto != "" {
		trozos = append(trozos, resto)
	}
	return trozos
}
