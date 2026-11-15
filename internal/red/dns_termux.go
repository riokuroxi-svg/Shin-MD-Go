// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// dns_termux.go — DNS funcional en Termux sin root, sin proot y sin rezar.
package red

import (
	"bufio"
	"context"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// ArreglarDNSAndroid devuelve y aplica los servidores DNS que el bot usará
// cuando el terreno es Termux/Android y el resolvedor normal de Go está
// ciego: con CGO_ENABLED=0 netgo lee /etc/resolv.conf, que en Android no
// existe ni puede crearse sin root, y su plan B de fábrica es preguntar a
// 127.0.0.1:53 y [::1]:53, donde nunca escucha nadie («lookup … on [::1]:53
// connection refused»). El parche de Termux para el toolchain cambia la ruta
// por $PREFIX/etc/resolv.conf, pero hay builds en los que no muerde (visto
// en go1.27.3 del repo oficial), así que aquí se resuelve a nivel código.
//
// Qué hace, por capas:
//
//   - No es Termux o /etc/resolv.conf YA nombra servidores → nil, no toco.
//   - $PREFIX/etc/resolv.conf trae nameserver… → se respetan (el instalador
//     lo escribe si faltaba; el usuario puede editarlo y mandar el suyo).
//   - Nada de lo anterior → 8.8.8.8 y 1.1.1.1.
//
// Devuelve la lista final aplicada (o nil si no hizo falta).
func ArreglarDNSAndroid() []string {
	servidores := resolvedorNecesario(os.Getenv("PREFIX"), os.Getenv("TERMUX_VERSION") != "")
	if len(servidores) == 0 {
		return nil
	}
	instalarResolvedor(servidores)
	return servidores
}

// resolvedorNecesario decide y devuelve los servidores SIN tocar nada global:
// así se puede probar con archivos temporales.
func resolvedorNecesario(prefix string, esTermux bool) []string {
	if !esTermux && !strings.Contains(prefix, "com.termux") {
		return nil // Linux de escritorio, contenedor, etc.: el estándar basta
	}
	if len(nameserversDe("/etc/resolv.conf")) > 0 {
		return nil // sorprendentemente sano (proot, chroot, root): no estorbar
	}
	if prefix != "" {
		if s := nameserversDe(filepath.Join(prefix, "etc", "resolv.conf")); len(s) > 0 {
			return s
		}
	}
	return []string{"8.8.8.8", "1.1.1.1"}
}

// instalarResolvedor sustituye net.DefaultResolver por uno puro-Go que marca
// los servidores dados. El Dial personalizado ignora la dirección «de
// fábrica» (que sería ::1:53, el callejón sin salida) y marca siempre uno de
// los nuestros, en turno rotatorio. Go usa UDP y, si la respuesta llega
// truncada (TC), reintenta por TCP con el mismo Dial: se sirven ambas.
func instalarResolvedor(servidores []string) {
	dialero := &net.Dialer{Timeout: 5 * time.Second}
	var turno atomic.Uint32
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "udp" && network != "tcp" {
				return dialero.DialContext(ctx, network, address)
			}
			i := int(turno.Add(1) - 1)
			servidor := servidores[i%len(servidores)]
			return dialero.DialContext(ctx, network, net.JoinHostPort(servidor, "53"))
		},
	}
}

// nameserversDe extrae las líneas «nameserver IP» de un resolv.conf, con la
// misma tolerancia que el lector del propio Go: comentarios «#» y «;»,
// líneas cortas y direcciones no-IP se saltan, y nunca más de tres (el límite
// estándar). Ruta inexistente o ilegible → nil, sin escándalo.
func nameserversDe(ruta string) []string {
	f, err := os.Open(ruta)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var salida []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		linea := sc.Text()
		if i := strings.IndexAny(linea, "#;"); i >= 0 {
			linea = linea[:i] // comentario al final de la línea también cuenta
		}
		campos := strings.Fields(linea)
		if len(campos) < 2 || campos[0] != "nameserver" {
			continue
		}
		candidato := strings.Trim(campos[1], "[]")
		if _, err := netip.ParseAddr(candidato); err != nil {
			continue // si no es IP, resolverla requeriría… DNS. Gallina y huevo.
		}
		salida = append(salida, candidato)
		if len(salida) == 3 {
			break
		}
	}
	return salida
}
