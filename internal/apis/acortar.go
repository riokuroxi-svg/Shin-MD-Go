// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// acortar.go — Puerto de cmds/utils/acortar.js (TinyURL, sin key).
package apis

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// urlTinyURL es variable para que las pruebas lo apunten a un httptest.Server.
var urlTinyURL = "https://tinyurl.com/api-create.php"

// Acortar pide a TinyURL la versión corta del enlace. Devuelve error plano
// cuando el servicio contesta algo que no es un enlace (lo hace ante URLs
// raras) y antes de gastar la llamada si el texto no pinta a enlace.
func Acortar(ctx context.Context, enlace string) (string, error) {
	enlace = strings.TrimSpace(enlace)
	if !strings.HasPrefix(strings.ToLower(enlace), "http://") &&
		!strings.HasPrefix(strings.ToLower(enlace), "https://") {
		return "", fmt.Errorf("no-es-enlace")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		urlTinyURL+"?url="+url.QueryEscape(enlace), nil)
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
	cuerpo, pasado, err := leerTecho(resp.Body, 4096)
	if err != nil || pasado {
		return "", fmt.Errorf("respuesta-rara")
	}
	corto := strings.TrimSpace(string(cuerpo))
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(corto, "http") {
		return "", fmt.Errorf("http-%d", resp.StatusCode)
	}
	return corto, nil
}
