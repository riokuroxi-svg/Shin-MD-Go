// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// mejorar.go — Puerto de cmds/utils/hd.js («.hd», mejorar una imagen).
//
// CAMBIO DE MOTOR RESPECTO AL JS
// El JS subía la foto a una API «milagrosa» de terceros: cuando esa web se cae
// (o empieza a cobrar, o se queda tus fotos), el comando muere sin avisar.
// Aquí la mejora es LOCAL y honesta: reescalado 2× con interpolación bilineal
// suave + máscara de enfoque (unsharp). No inventa píxeles como una IA, pero
// recupera nitidez de fotos pequeñas o reescaladas por WhatsApp, y lo hace
// dentro del binario, sin que la imagen salga del servidor.
//
// Todo tiene techo porque lo dispara cualquiera desde el chat:
//
//	· entrada ≤ 12 Mpx (más es un muro de RAM para una «mejora»);
//	· salida ≤ 4096 px de lado (es lo que WhatsApp enseña grande).
package media

import (
	"fmt"
	"image"
)

const (
	// MejoraMaxPixeles es el techo de la entrada (ancho × alto).
	MejoraMaxPixeles = 12_000_000
	// MejoraMaxLado es el techo del lado de la salida.
	MejoraMaxLado = 4096
	// MejoraFactor es cuánto se agranda una imagen pequeña.
	MejoraFactor = 2
)

// Mejorar reescala y enfoca. Devuelve una imagen nueva; la original no se toca.
// Error «demasiado-grande» si la entrada ya pasa del techo de píxeles.
func Mejorar(img image.Image) (*image.NRGBA, error) {
	if img == nil {
		return nil, fmt.Errorf("imagen-vacia")
	}
	src := toNRGBA(img)
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("imagen-vacia")
	}
	if w*h > MejoraMaxPixeles {
		return nil, fmt.Errorf("demasiado-grande")
	}

	// Factor final: 2× en pequeñas, lo que quepa hasta 4096 en medianas, y
	// si ya es más grande que eso no se escala: solo se enfoca.
	factor := float64(MejoraFactor)
	if float64(w)*factor > MejoraMaxLado || float64(h)*factor > MejoraMaxLado {
		factor = float64(MejoraMaxLado) / float64(maxInt(w, h))
	}
	nw, nh := int(float64(w)*factor), int(float64(h)*factor)

	out := src
	if nw > w && nh > h {
		out = escalar(src, nw, nh)
	} else {
		// toNRGBA devuelve el mismo puntero si ya era NRGBA: copiar antes de
		// enfocar para no manchar la imagen del llamador.
		copia := image.NewNRGBA(image.Rect(0, 0, w, h))
		copy(copia.Pix, src.Pix)
		out = copia
	}
	enfocar(out)
	return out, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
