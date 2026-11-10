// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// qr.go — Código QR dibujado en el propio binario.
//
// El JS mandaba la URL de un servicio de terceros (`api.qrserver.com`) y dejaba
// que WhatsApp se trajera la imagen: el contenido del QR (que puede ser una
// contraseña, un wifi o un token) salía del servidor del bot hacia una web
// ajena, y si esa web estaba caída el comando no funcionaba.
//
// Aquí se genera con `rsc.io/qr` (licencia BSD-3, la misma que ya venía como
// dependencia de qrterminal) y se pinta en PNG con la biblioteca estándar.
package utilidades

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"rsc.io/qr"
)

const (
	// MaxQR es el tope de contenido. Un QR de más de ~2 900 bytes no cabe en
	// ningún nivel de corrección, y por encima de esto tampoco se lee bien.
	MaxQR = 1200
	// ladoQR es el tamaño del PNG que se manda: el suficiente para escanearlo
	// desde la pantalla de otro teléfono.
	ladoQR = 640
	// silencio son los módulos en blanco alrededor (la «zona de silencio» que
	// pide la norma: sin ella, muchos lectores no enganchan el código).
	silencio = 4
)

// ImagenQR devuelve un PNG cuadrado con el QR de `contenido`.
//
// `contenido` vacío o demasiado largo es error, no un QR vacío: un QR de la
// nada se ve igual que uno bueno hasta que alguien intenta escanearlo.
func ImagenQR(contenido string) ([]byte, error) {
	if contenido == "" {
		return nil, fmt.Errorf("no hay nada que poner en el QR")
	}
	if len([]byte(contenido)) > MaxQR {
		return nil, fmt.Errorf("el contenido es demasiado largo para un QR (%d bytes, máximo %d)",
			len([]byte(contenido)), MaxQR)
	}

	// Nivel M: aguanta ~15 % de daño, que es lo que se espera de una foto de
	// pantalla. Con L (7 %) un QR fotografiado con reflejo deja de leerse.
	codigo, err := qr.Encode(contenido, qr.M)
	if err != nil {
		return nil, fmt.Errorf("no se pudo codificar el QR: %w", err)
	}
	codigo.Scale = 1 // se escala a mano abajo para controlar el tamaño exacto

	modulos := codigo.Size + 2*silencio
	escala := ladoQR / modulos
	if escala < 1 {
		escala = 1
	}
	lado := modulos * escala

	img := image.NewGray(image.Rect(0, 0, lado, lado))
	// Fondo blanco en todo el lienzo (Gray16 cero es negro: hay que pintarlo).
	for i := range img.Pix {
		img.Pix[i] = 0xff
	}
	negro := color.Gray{Y: 0}
	for y := 0; y < codigo.Size; y++ {
		for x := 0; x < codigo.Size; x++ {
			if !codigo.Black(x, y) {
				continue
			}
			x0 := (x + silencio) * escala
			y0 := (y + silencio) * escala
			for dy := 0; dy < escala; dy++ {
				for dx := 0; dx < escala; dx++ {
					img.SetGray(x0+dx, y0+dy, negro)
				}
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("no se pudo escribir el PNG: %w", err)
	}
	return buf.Bytes(), nil
}
