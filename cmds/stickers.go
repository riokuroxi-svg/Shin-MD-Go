// Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
//
// stickers.go — Puerto de cmds/stickers/sticker.js y cmds/utils/toimg.js.
//
//	.sticker                 convierte en sticker la foto/vídeo que mandas o citas
//	.sticker -c              lo recorta en círculo          (formas: ver abajo)
//	.sticker -sepia          le aplica un efecto            (efectos: ver abajo)
//	.toimg                   sticker → PNG
//	.toimg (animado)         sticker animado → MP4
//	.stickerinfo             qué paquete lleva un sticker
//
// Dos decisiones que se apartan del JS, y por qué:
//
//   - El JS subía el sticker a **ezgif.com** para convertirlo a vídeo. Aquí se
//     hace con `ffmpeg` local: sin mandar nada a un tercero, sin depender de que
//     ese sitio siga vivo y sin necesitar internet en el servidor.
//   - Los efectos y las formas del JS eran comandos de `sharp` y archivos SVG.
//     Aquí son píxeles (internal/media): el mismo resultado en cualquier máquina
//     y sin instalar nada.
package cmds

import (
	"errors"
	"fmt"
	"strings"

	"shin-go/internal/bot"
	"shin-go/internal/media"
	"shin-go/internal/wa"
)

// maxMedio: tope de descarga. Un sticker o una foto de WhatsApp no pasan de
// unos pocos MB; 16 MB deja margen para un vídeo y evita que un archivo
// gigante se coma la memoria del proceso.
const maxMedio = 16 << 20

// opcionesSticker traduce los argumentos a las opciones de conversión.
//
//	-c/--circulo   forma  ·  -sepia/-blur/…  efecto  ·  -foto  encaja entera
func opcionesSticker(args []string) (media.Opciones, error) {
	op := media.Opciones{Lado: media.Lado, FormaAuto: true}
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		nombre := strings.ToLower(strings.TrimLeft(a, "-"))
		switch nombre {
		case "foto", "fit", "completo":
			op.FormaAuto = true
			continue
		case "llenar", "fill", "cover":
			op.FormaAuto = false
			continue
		}
		if f, ok := media.Filtro(nombre); ok {
			op.Efecto = f.Nombre
			continue
		}
		if m, ok := media.Mascara(nombre); ok {
			op.Forma = m.Nombre
			continue
		}
		return op, fmt.Errorf("no conozco *%s*", a)
	}
	return op, nil
}

// ayudaSticker es el texto de la ayuda (y el que sale cuando la opción no vale).
func ayudaSticker(c *bot.Context) string {
	p := c.Bot.FirstPrefix()
	return fmt.Sprintf(
		"「✎」Opciones del *%ssticker*:\n\n"+
			"> *Formas:* %s\n"+
			"> *Efectos:* %s\n"+
			"> *Extra:* `-foto` (encaja entera) · `-llenar` (recorta y llena)\n\n"+
			"Ejemplos:\n> %ssticker\n> %ssticker -c\n> %ssticker -sepia\n> %ssticker -corazon -brighten",
		p,
		strings.Join(media.NombresMascaras(), " · "),
		strings.Join(media.NombresFiltros(), " · "),
		p, p, p, p)
}

func registerSticker() {
	bot.Register(bot.Command{
		Name:     "sticker",
		Aliases:  []string{"s", "stiker", "figu", "figura"},
		Category: bot.CatStickers,
		Desc:     "Convertir una foto o vídeo en sticker (con formas y efectos)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			op, err := opcionesSticker(c.Args)
			if err != nil {
				return c.Reply(fmt.Sprintf("ꕥ %v\n\n%s", err, ayudaSticker(c)))
			}

			medio, ok := wa.MedioDe(c.Event.Message)
			if !ok {
				return c.Reply(fmt.Sprintf(
					"ꕥ Mándame una *foto*, un *vídeo* o un *sticker*, o cita uno y escribe *%ssticker*.\n\n%s",
					c.Bot.FirstPrefix(), ayudaSticker(c)))
			}

			datos, err := c.Bot.DownloadMedia(c.Ctx, medio.Data)
			if err != nil {
				return c.Reply("ꕥ No pude descargar el archivo. Vuelve a mandarlo, por favor.")
			}
			if len(datos) == 0 {
				return c.Reply("ꕥ El archivo llegó vacío.")
			}
			if len(datos) > maxMedio {
				return c.Reply("ꕥ El archivo es demasiado grande para un sticker (máximo 16 MB).")
			}

			paquete := media.NuevoPaquete(
				c.Bot.Config().StickerPack, c.Bot.Config().StickerAuth)
			paquete.Nombre = limitarTexto(paquete.Nombre, 64)
			paquete.Autor = limitarTexto(paquete.Autor, 64)

			// 1) Sticker o vídeo → WebP animado (necesita ffmpeg).
			if medio.Tipo == wa.MedioSticker || medio.Tipo == wa.MedioVideo {
				if !media.HayFFmpeg() {
					// Un sticker ya convertido se puede reenviar tal cual si no
					// se ha pedido ninguna forma ni efecto.
					if medio.Tipo == wa.MedioSticker && op.Forma == "" && op.Efecto == "" {
						bloque, err := paquete.EXIF()
						if err != nil {
							return err
						}
						salida, err := media.AplicarEXIF(datos, bloque)
						if err != nil {
							return err
						}
						return c.Bot.SendSticker(c.Ctx, c.Chat, salida, inicioAnimo(datos), c.Priority())
					}
					return c.Reply(fmt.Sprintf(
						"ꕥ Para convertir *vídeos* (o cambiarle la forma a un sticker animado) " +
							"hace falta *ffmpeg* en el servidor.\n" +
							"> Instálalo con `apt install ffmpeg` (o `pkg install ffmpeg` en Termux).\n" +
							"> Las *fotos* sí funcionan sin nada más."))
				}
				salida, err := media.VideoAWebP(c.Ctx, datos, 3)
				if err != nil {
					return c.Reply(fmt.Sprintf("ꕥ No pude convertir el vídeo: %v", err))
				}
				bloque, err := paquete.EXIF()
				if err != nil {
					return err
				}
				salida, err = media.AplicarEXIF(salida, bloque)
				if err != nil {
					return err
				}
				return c.Bot.SendSticker(c.Ctx, c.Chat, salida, true, c.Priority())
			}

			// 2) Foto (o un PDF/imagen) → WebP fijo.
			webp, err := media.Sticker(datos, op)
			if err != nil {
				if errors.Is(err, media.ErrFormato) {
					return c.Reply("ꕥ Ese archivo no es una imagen que pueda convertir.")
				}
				return c.Reply(fmt.Sprintf("ꕥ No pude convertir la imagen: %v", err))
			}
			bloque, err := paquete.EXIF()
			if err != nil {
				return err
			}
			webp, err = media.AplicarEXIF(webp, bloque)
			if err != nil {
				return err
			}
			return c.Bot.SendSticker(c.Ctx, c.Chat, webp, false, c.Priority())
		},
	})
}

// inicioAnimo: un WebP es animado si tiene el chunk ANIM.
func inicioAnimo(webp []byte) bool { return media.EsAnimado(webp) }

// limitarTexto recorta un nombre a `n` caracteres (con puntos suspensivos) para
// que quepa en el EXIF del sticker.
func limitarTexto(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

// ─── .toimg ─────────────────────────────────────────────────────────────────

func registerToImg() {
	bot.Register(bot.Command{
		Name:     "toimg",
		Aliases:  []string{"toimage", "afoto", "png"},
		Category: bot.CatStickers,
		Desc:     "Convertir un sticker en imagen (o en vídeo si es animado)",
		Cooldown: 3,
		Run: func(c *bot.Context) error {
			medio, ok := wa.MedioDe(c.Event.Message)
			if !ok {
				return c.Reply(fmt.Sprintf(
					"ꕥ Cita un *sticker* (o mándalo con *%stoimg*) para convertirlo en imagen.",
					c.Bot.FirstPrefix()))
			}
			if medio.Tipo != wa.MedioSticker {
				return c.Reply("ꕥ Eso no es un sticker. Cita el sticker que quieres convertir.")
			}

			datos, err := c.Bot.DownloadMedia(c.Ctx, medio.Data)
			if err != nil {
				return c.Reply("ꕥ No pude descargar el sticker. Prueba otra vez.")
			}
			if len(datos) > maxMedio {
				return c.Reply("ꕥ El sticker es demasiado grande para convertirlo.")
			}

			// Animado → vídeo local (nada de ezgif).
			if media.EsAnimado(datos) {
				nota := "🎬 Sticker animado"
				if media.HayFFmpeg() {
					if mp4, err := media.WebPAVideo(c.Ctx, datos, 6); err == nil {
						return c.Bot.SendVideo(c.Ctx, c.Chat, mp4, nota, c.Priority())
					} else {
						c.Log.Warn(fmt.Sprintf("toimg: ffmpeg no pudo convertir el sticker (%v); se manda el primer fotograma", err))
					}
					nota = "🖼 Primer fotograma (este ffmpeg no lee WebP animado)"
				} else {
					nota = "🖼 Primer fotograma (sin ffmpeg para sacar el vídeo)"
				}
				// Red de seguridad: si no hay ffmpeg (o el que hay no trae el
				// decodificador de WebP animado), se manda la primera imagen.
				img, err := media.PrimerFotograma(datos)
				if err != nil {
					return c.Reply("ꕥ Este sticker es *animado* y no pude sacar ni el primer fotograma.\n" +
						"> Con *ffmpeg* instalado saldría el vídeo completo.")
				}
				png, err := media.EncodePNG(img)
				if err != nil {
					return c.Reply(fmt.Sprintf("ꕥ No pude crear la imagen: %v", err))
				}
				return c.Bot.SendImage(c.Ctx, c.Chat, png, nota, c.Priority())
			}

			img, err := media.Decode(datos)
			if err != nil {
				return c.Reply("ꕥ No pude leer el sticker (¿está dañado?).")
			}
			png, err := media.EncodePNG(img)
			if err != nil {
				return c.Reply(fmt.Sprintf("ꕥ No pude crear la imagen: %v", err))
			}
			return c.Bot.SendImage(c.Ctx, c.Chat, png, "🖼 Sticker convertido a PNG", c.Priority())
		},
	})
}

// ─── .stickerinfo ───────────────────────────────────────────────────────────

func registerStickerInfo() {
	bot.Register(bot.Command{
		Name:     "stickerinfo",
		Aliases:  []string{"infosticker", "packinfo"},
		Category: bot.CatStickers,
		Desc:     "Ver el paquete y el autor de un sticker",
		Cooldown: 2,
		Run: func(c *bot.Context) error {
			medio, ok := wa.MedioDe(c.Event.Message)
			if !ok || medio.Tipo != wa.MedioSticker {
				return c.Reply(fmt.Sprintf(
					"ꕥ Cita un *sticker* y escribe *%sstickerinfo*.", c.Bot.FirstPrefix()))
			}
			datos, err := c.Bot.DownloadMedia(c.Ctx, medio.Data)
			if err != nil {
				return c.Reply("ꕥ No pude descargar el sticker.")
			}
			paquete, ok, err := media.LeerEXIF(datos)
			if err != nil || !ok {
				return c.Reply("ꕥ Ese sticker no lleva paquete (lo hizo otro bot o una app).")
			}
			animado := "no"
			if media.EsAnimado(datos) {
				animado = "sí"
			}
			return c.Reply(fmt.Sprintf(
				"「✿」*Sticker*\n> Paquete: *%s*\n> Autor: *%s*\n> Emojis: %s\n> Animado: *%s*\n> Tamaño: *%d KB*",
				paquete.Nombre, paquete.Autor, strings.Join(paquete.Emojis, " "),
				animado, len(datos)/1024))
		},
	})
}

func init() {
	registerSticker()
	registerToImg()
	registerStickerInfo()
}
