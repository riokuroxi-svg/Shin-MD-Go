// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// enlaces.go — Reglas para `.get`: qué URL se puede pedir y cuál no.
//
// POR QUÉ ESTO NO ES «PEDIR UNA URL Y YA»
// `.get` hace que el servidor del bot pida una dirección que elige cualquiera
// del grupo. Sin filtro, eso sirve para leer lo que hay detrás del cortafuegos:
//
//	.get http://192.168.1.1/            → el router de casa
//	.get http://127.0.0.1:8080/admin    → el panel del propio servidor
//	.get http://169.254.169.254/latest/ → las credenciales del VPS (metadata)
//
// Eso se llama SSRF y es el fallo clásico de los comandos «fetch». Aquí se
// resuelve el nombre y se comprueba **la IP a la que apunta**, no el texto de
// la URL: si `localhost.evil.com` resuelve a 127.0.0.1, también se bloquea.
package utilidades

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	// MaxDescarga es el tope de bytes que `.get` se trae (5 MB). Por encima,
	// o se corta la respuesta o el bot se queda sin memoria sirviendo páginas.
	MaxDescarga = 5 << 20
	// MaxTexto es lo que se manda al chat como texto: WhatsApp corta mucho
	// antes, así que se recorta y se avisa.
	MaxTexto = 3000
)

// ValidarURL comprueba forma, esquema y destino. Devuelve una URL limpia.
//
// `resolver` se inyecta para poder probar el filtro sin red: en producción es
// net.DefaultResolver.LookupIPAddr.
func ValidarURL(cruda string, resolver func(host string) ([]net.IP, error)) (*url.URL, error) {
	cruda = strings.TrimSpace(cruda)
	if cruda == "" {
		return nil, fmt.Errorf("escríbeme el enlace")
	}
	if !strings.HasPrefix(strings.ToLower(cruda), "http://") &&
		!strings.HasPrefix(strings.ToLower(cruda), "https://") {
		return nil, fmt.Errorf("el enlace tiene que empezar por http:// o https://")
	}

	u, err := url.Parse(cruda)
	if err != nil {
		return nil, fmt.Errorf("ese enlace no se entiende")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("solo http y https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("al enlace le falta el dominio")
	}
	// Credenciales dentro de la URL: se rechazan para no reenviarlas a un
	// tercero ni meterlas en un log.
	if u.User != nil {
		return nil, fmt.Errorf("no se permiten usuario y contraseña dentro del enlace")
	}

	host := u.Hostname()
	if esIPPrivadaLiteral(host) {
		return nil, fmt.Errorf("esa dirección es de la red interna del servidor: no se puede pedir")
	}

	if resolver == nil {
		return u, nil
	}
	ips, err := resolver(host)
	if err != nil {
		return nil, fmt.Errorf("no pude resolver *%s*", host)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("*%s* no tiene dirección", host)
	}
	for _, ip := range ips {
		if esIPPrivada(ip) {
			return nil, fmt.Errorf("ese dominio apunta a la red interna del servidor: no se puede pedir")
		}
	}
	return u, nil
}

// esIPPrivadaLiteral acepta el texto del host y decide si es una IP que no se
// puede pedir. Un dominio normal devuelve false (ya se comprobará al resolver).
func esIPPrivadaLiteral(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return esIPPrivada(ip)
}

// esIPPrivada es la lista de direcciones que no se piden nunca: loopback,
// redes privadas, enlace local (metadata del VPS), CGNAT y multicast.
func esIPPrivada(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// 100.64.0.0/10 (CGNAT): es «privada» de hecho en muchas redes de VPS y
	// operadores, aunque net.IP no la marque como tal.
	if v4 := ip.To4(); v4 != nil && v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
		return true
	}
	// 169.254.169.254 ya cae en enlace local, pero se deja explícito porque es
	// la que de verdad se ataca (credenciales de la nube).
	if ip.Equal(net.ParseIP("169.254.169.254")) {
		return true
	}
	return false
}

// ResolverDNSIPs resuelve un host con el resolutor del sistema, con el formato
// que espera ValidarURL.
func ResolverDNSIPs(host string) ([]net.IP, error) {
	dirs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	return dirs, nil
}

// RecortarParaChat deja un texto listo para mandar: quita nulos y controles que
// rompen el envío, y recorta con un aviso claro si no cabe.
func RecortarParaChat(texto string) string {
	texto = strings.Map(func(r rune) rune {
		switch {
		case r == 0:
			return -1
		case r == '\r':
			return ' '
		case r < 32 && r != '\n' && r != '\t':
			return -1
		default:
			return r
		}
	}, texto)

	if len([]rune(texto)) <= MaxTexto {
		return texto
	}
	return string([]rune(texto)[:MaxTexto]) +
		fmt.Sprintf("\n\n… (recortado: %d caracteres en total)", len([]rune(texto)))
}
