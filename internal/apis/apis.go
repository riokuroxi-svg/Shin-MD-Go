// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// apis.go — Clientes de las APIs de terceros del bloque B5.2.
//
// REGLAS DE LA CASA PARA PEDIR COSAS FUERA
//
//  1. Nada de SDKs: todas estas APIs contestan JSON o texto con una sola
//     petición HTTP; traer una librería por cada una sería volver al
//     node_modules del que huímos.
//  2. Todo tiene plazo (context) y tope de bytes: un comando lo dispara
//     cualquiera del grupo y una API lenta no puede congelar una goroutine
//     del repartidor.
//  3. Las URLs bases son variables de paquete: las pruebas las apuntan a un
//     httptest.Server y así se prueba el flujo entero sin tocar internet.
package apis

import (
	"io"
	"net/http"
	"time"
)

// userAgent es la firma del bot ante las APIs; la misma que usa `.get`.
const userAgent = "Shin-MD-Go (+https://github.com/riokuroxi-svg/Shin-MD-Go)"

// clienteHTTP es el cliente compartido de las APIs. Las redirecciones se
// siguen con normalidad aquí (el zipball de GitHub vive tras un 302 a
// codeload); el filtro anti-SSRF es cosa de `.get`, que pide URLs del usuario.
var clienteHTTP = &http.Client{Timeout: 45 * time.Second}

// leerTecho lee como máximo techo+1 bytes y dice si se pasó.
func leerTecho(r io.Reader, techo int64) (datos []byte, pasado bool, err error) {
	datos, err = io.ReadAll(io.LimitReader(r, techo+1))
	if err != nil {
		return nil, false, err
	}
	return datos, int64(len(datos)) > techo, nil
}

// pedir hace un GET con la firma del bot y el plazo que traiga el contexto.
func pedir(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	return clienteHTTP.Do(req)
}
