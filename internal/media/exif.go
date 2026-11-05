// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// exif.go — El "paquete" del sticker: quién lo hizo y con qué nombre.
//
// Un sticker WebP es un RIFF; WhatsApp lee el nombre del paquete de un chunk
// `EXIF` que va **después** del chunk de imagen. El contenido es el JSON
//
//	{"sticker-pack-id":"…","sticker-pack-name":"…","sticker-pack-publisher":"…","emojis":["…"]}
//
// precedido de 20 bytes de cabecera TIFF (con la etiqueta 0x5741 = "AW", que es
// la que usa WhatsApp para este campo) y con la longitud del JSON escrita en el
// byte 14 en little-endian.
//
// Lo que hacía el JS con `node-webpmux` (una librería de terceros que escribe la
// lista de chunks a mano) se hace aquí en 40 líneas, y con una prueba que lee el
// resultado: si WhatsApp cambia el formato, la prueba revienta antes que el bot.
package media

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
)

// Paquete es lo que se ve en WhatsApp al abrir un sticker.
type Paquete struct {
	ID     string   `json:"sticker-pack-id"`
	Nombre string   `json:"sticker-pack-name"`
	Autor  string   `json:"sticker-pack-publisher"`
	Emojis []string `json:"emojis"`
}

// Los mismos valores por defecto del bot de JS.
const (
	PaqueteIDPorDefecto     = "https://github.com/riokuroxi-svg/Shin-MD-Go"
	PaqueteNombrePorDefecto = "Shin-MD-Go"
	PaqueteAutorPorDefecto  = "riokuroxi-svg"
)

// NuevoPaquete arma el paquete aplicando los valores por defecto.
func NuevoPaquete(nombre, autor string, emojis ...string) Paquete {
	if nombre == "" {
		nombre = PaqueteNombrePorDefecto
	}
	if autor == "" {
		autor = PaqueteAutorPorDefecto
	}
	if len(emojis) == 0 {
		emojis = []string{"✨"}
	}
	return Paquete{ID: PaqueteIDPorDefecto, Nombre: nombre, Autor: autor, Emojis: emojis}
}

// cabeceraEXIF son los 20 bytes fijos. El 0x5741 del byte 10 es "AW" y el 0x16
// del byte 18 es la longitud que se sobrescribe con el tamaño real del JSON.
var cabeceraEXIF = []byte{
	0x49, 0x49, 0x2A, 0x00, 0x08, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x41, 0x57, 0x07, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x16, 0x00, 0x00, 0x00,
}

// EXIF arma el bloque EXIF completo (cabecera + JSON) listo para meter en el
// RIFF.
func (p Paquete) EXIF() ([]byte, error) {
	if p.ID == "" {
		p.ID = PaqueteIDPorDefecto
	}
	js, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if len(js) > 0xFFFF {
		return nil, fmt.Errorf("el paquete es demasiado grande (%d bytes de JSON)", len(js))
	}
	exif := make([]byte, len(cabeceraEXIF)+len(js))
	copy(exif, cabeceraEXIF)
	binary.LittleEndian.PutUint32(exif[14:], uint32(len(js)))
	copy(exif[len(cabeceraEXIF):], js)
	return exif, nil
}

// AplicarEXIF mete el bloque EXIF dentro del WebP y devuelve el archivo final.
//
// Un WebP es "RIFF <tamaño> WEBP" seguido de chunks (VP8X, VP8L, EXIF…), cada
// uno con nombre de 4 letras, tamaño de 4 bytes y relleno a número par. Si ya
// venía un EXIF se reemplaza: no tiene sentido acumular paquetes.
func AplicarEXIF(webp, exif []byte) ([]byte, error) {
	if len(webp) < 12 || string(webp[0:4]) != "RIFF" || string(webp[8:12]) != "WEBP" {
		return nil, fmt.Errorf("el archivo no es un WebP válido")
	}
	chunks, err := trocear(webp[12:])
	if err != nil {
		return nil, err
	}

	// El chunk EXIF va el último: es donde lo busca WhatsApp.
	salida := make([]byte, 0, len(webp)+len(exif)+16)
	for _, c := range chunks {
		nombre := string(c.cabecera[0:4])
		if nombre == "EXIF" {
			continue // se reemplaza el que hubiera: no se acumulan paquetes
		}
		datos := c.datos
		if nombre == "VP8X" && len(datos) >= 1 {
			// VP8X avisa de qué metadatos hay dentro. Sin el bit 0x08 (EXIF),
			// WhatsApp puede ignorar el bloque: es un byte que decide si el
			// sticker se ve con autor o "sin paquete".
			copia := append([]byte(nil), datos...)
			copia[0] |= 0x08
			datos = copia
		}
		salida = append(salida, c.cabecera...)
		salida = append(salida, datos...)
		if len(datos)%2 == 1 {
			salida = append(salida, 0) // el relleno de RIFF
		}
	}

	bloque := make([]byte, 8, 8+len(exif)+1)
	copy(bloque, "EXIF")
	binary.LittleEndian.PutUint32(bloque[4:], uint32(len(exif)))
	bloque = append(bloque, exif...)
	if len(exif)%2 == 1 {
		bloque = append(bloque, 0)
	}
	salida = append(salida, bloque...)

	final := make([]byte, 12, 12+len(salida))
	copy(final, "RIFF")
	binary.LittleEndian.PutUint32(final[4:], uint32(len(salida)+4))
	copy(final[8:], "WEBP")
	final = append(final, salida...)
	return final, nil
}

// LeerEXIF saca el paquete de un WebP ("" si no tiene). Sirve para las pruebas
// y para `.stickerinfo`.
func LeerEXIF(webp []byte) (Paquete, bool, error) {
	if len(webp) < 12 || string(webp[0:4]) != "RIFF" {
		return Paquete{}, false, fmt.Errorf("el archivo no es un WebP válido")
	}
	chunks, err := trocear(webp[12:])
	if err != nil {
		return Paquete{}, false, err
	}
	for _, c := range chunks {
		if string(c.cabecera[0:4]) != "EXIF" {
			continue
		}
		datos := c.datos
		if len(datos) < len(cabeceraEXIF) {
			return Paquete{}, false, fmt.Errorf("el bloque EXIF está truncado")
		}
		js := datos[len(cabeceraEXIF):]
		var p Paquete
		if err := json.Unmarshal(bytes.TrimRight(js, "\x00"), &p); err != nil {
			return Paquete{}, false, err
		}
		return p, true, nil
	}
	return Paquete{}, false, nil
}

type chunk struct {
	cabecera []byte // 8 bytes: nombre + tamaño
	datos    []byte
}

// trocear parte el cuerpo de un RIFF en sus chunks.
func trocear(cuerpo []byte) ([]chunk, error) {
	var out []chunk
	i := 0
	for i+8 <= len(cuerpo) {
		nombre := string(cuerpo[i : i+4])
		tam := int(binary.LittleEndian.Uint32(cuerpo[i+4 : i+8]))
		if tam < 0 || i+8+tam > len(cuerpo) {
			return nil, fmt.Errorf("chunk %q con tamaño imposible (%d)", nombre, tam)
		}
		out = append(out, chunk{
			cabecera: cuerpo[i : i+8],
			datos:    cuerpo[i+8 : i+8+tam],
		})
		i += 8 + tam
		if tam%2 == 1 {
			i++
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("el WebP no tiene chunks")
	}
	return out, nil
}
