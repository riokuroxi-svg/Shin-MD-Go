// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// subir.go — Puerto de cmds/utils/tourl.js (litterbox.catbox.moe, sin key).
// Misma política de reintentos que el JS: 24h, 72h y 1h × 2 intentos, con
// espera progresiva entre ellos. El archivo caduca solo: no queda basura.
package apis

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// urlLitterbox es variable para que las pruebas lo apunten a un httptest.Server.
var urlLitterbox = "https://litterbox.catbox.moe/resources/internals/api.php"

// MaxSubida es el tope de archivo a subir (64 MB): más grande no cabe por la
// descarga de WhatsApp con plazo y el chat no necesita un enlace a un disco.
const MaxSubida = 64 << 20

// Dormir es reemplazable en las pruebas para que el reintento no espere de verdad.
var Dormir = time.Sleep

// SubirTemporal sube los bytes y devuelve (url, caducidad). Los tiempos se
// prueban en este orden porque a veces el servicio rechaza las de más horas.
func SubirTemporal(ctx context.Context, datos []byte, mime, nombre string) (string, string, error) {
	if len(datos) == 0 {
		return "", "", fmt.Errorf("vacio")
	}
	if len(datos) > MaxSubida {
		return "", "", fmt.Errorf("muy-grande")
	}
	var ultimo error
	for _, tiempo := range []string{"24h", "72h", "1h"} {
		for intento := 1; intento <= 2; intento++ {
			url, err := subirUnaVez(ctx, datos, mime, nombre, tiempo)
			if err == nil {
				return url, tiempo, nil
			}
			ultimo = err
			select {
			case <-ctx.Done():
				return "", "", fmt.Errorf("tiempo-agotado")
			default:
			}
			Dormir(time.Duration(intento) * time.Second)
		}
	}
	if ultimo == nil {
		ultimo = fmt.Errorf("subida-fallida")
	}
	return "", "", ultimo
}

// subirUnaVez arma el multipart (reqtype, time, fileToUpload) igual que el JS.
func subirUnaVez(ctx context.Context, datos []byte, mime, nombre, tiempo string) (string, error) {
	var cuerpo bytes.Buffer
	w := multipart.NewWriter(&cuerpo)
	_ = w.WriteField("reqtype", "fileupload")
	_ = w.WriteField("time", tiempo)
	part, err := w.CreatePart(map[string][]string{
		"Content-Disposition": {fmt.Sprintf(`form-data; name="fileToUpload"; filename="%s"`, nombre)},
		"Content-Type":        {mime},
	})
	if err != nil {
		return "", err
	}
	_, _ = part.Write(datos)
	if err := w.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, urlLitterbox, &cuerpo)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := clienteHTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("tiempo-agotado")
		}
		return "", fmt.Errorf("sin-red")
	}
	defer func() { _ = resp.Body.Close() }()
	texto, pasado, err := leerTecho(resp.Body, 4096)
	if err != nil || pasado {
		return "", fmt.Errorf("respuesta-rara")
	}
	url := strings.TrimSpace(string(texto))
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(url, "https://") {
		return "", fmt.Errorf("http-%d", resp.StatusCode)
	}
	return url, nil
}
