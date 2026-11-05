# Stickers y medios · bloque B5

_Fotos y vídeos → sticker WebP con paquete, y de vuelta a imagen · 5-nov-2026 ·
versión 0.8.0_

```
.sticker        foto, vídeo o sticker → sticker de 512×512
.sticker -c     con forma      (círculo, corazón, estrella…)
.sticker -sepia con efecto     (sepia, blur, invert…)
.toimg          sticker → PNG (o MP4 si es animado)
.stickerinfo    qué paquete y qué autor lleva un sticker
```

---

## 1. De qué está hecho (y de qué no)

El bot de JS usaba `sharp` (módulo **nativo**, compilado en C++) y
`node-webpmux` para escribir el EXIF. Eso obligaba a tener Node ≥ 22.5 con
binarios precompilados para la arquitectura del servidor — y en Termux
(Android/arm64) es justo donde más falla.

Aquí solo hay **dos dependencias, las dos en Go puro**:

| Pieza | Quién la hace |
|---|---|
| leer PNG, JPEG, GIF | biblioteca estándar |
| leer WebP | `golang.org/x/image/webp` |
| escalar, recortar con antialias | 60 líneas en `internal/media/media.go` |
| formas y efectos | `internal/media/mascaras.go` y `filtros.go` |
| **escribir** WebP | `github.com/HugoSmits86/nativewebp` |
| EXIF del paquete | `internal/media/exif.go` (42 líneas) |
| vídeo ⇄ WebP animado | `ffmpeg` del sistema, si está |

Compilado en estático (`CGO_ENABLED=0`) el binario sigue siendo un solo
archivo: en el teléfono no hay que instalar nada para los stickers de foto.

**Lo único que necesita `ffmpeg` es el vídeo.** Y si no está, el bot lo dice con
el comando exacto para instalarlo en vez de fallar de forma rara.

---

## 2. Formas (`-c`, `-v`, `-t`…)

Se dibujan como **máscaras** (círculo, polígono o curva) y se rasterizan con
4×4 muestras por píxel, así que el borde queda suave y no dentado. En el JS eran
archivos SVG que había que traer.

| Atajo | Forma | Atajo | Forma |
|---|---|---|---|
| `-c` | círculo | `-y` | pentágono |
| `-t` | triángulo | `-h` | hexágono |
| `-s` | estrella | `-o` | octógono |
| `-v` | corazón | `-e` | elipse |
| `-d` | rombo | `-z` | cruz |
| `-r` | redondeado | `-f` | marco |
| `-cuadrado` | cuadrado | `-estrella6` | estrella de 6 puntas |

También se aceptan los nombres completos (`-corazon`, `-circulo`…). El
`TestAliasDeFormasDelJS` comprueba que cada atajo llega a su forma: si alguien
cambia una tabla y `-c` deja de ser el círculo, la prueba falla.

## 3. Efectos

| Efecto | Qué hace |
|---|---|
| `-grayscale` | blanco y negro (luminancia real, no media de canales) |
| `-sepia` | tono sepia con la matriz clásica |
| `-invert` · `-negate` | negativo |
| `-brighten` · `-darken` | ±45 de brillo |
| `-blur` | desenfoque de caja, separado en horizontal y vertical |
| `-sharpen` | máscara de nitidez (centro ×5, vecinos ×−1) |
| `-rotate90` · `-rotate180` | giros |
| `-flip` · `-flop` | espejo vertical y horizontal |
| `-tint` | tinte violeta suave |
| `-normalice` | estira el contraste entre el percentil 2 y el 98 |

Todos están probados con una imagen de tablero (con degradados suaves el
enfoque no cambia nada, y la prueba lo detectaba).

## 4. El paquete: cómo se escribe el EXIF a mano

WhatsApp saca el nombre del paquete de un chunk `EXIF` dentro del WebP. Su
contenido es un JSON:

```json
{"sticker-pack-id":"https://github.com/riokuroxi-svg/Shin-MD-Go",
 "sticker-pack-name":"Shin-MD-Go",
 "sticker-pack-publisher":"riokuroxi-svg",
 "emojis":["✨"]}
```

precedido de **22 bytes** de cabecera TIFF (con la etiqueta `AW` de WhatsApp) y
con la longitud del JSON escrita en el **byte 14** en little-endian.

Escribirlo a mano tiene un detalle que se olvida fácil y que deja el sticker
"sin autor": el chunk **`VP8X`** (el envoltorio del WebP) lleva un byte de
banderas donde hay que marcar el bit `0x08` (EXIF). Si no se marca, el archivo
es válido, el sticker se ve… y WhatsApp ignora el paquete. `AplicarEXIF()` lo
marca.

`TestEXIFIdaYVuelta` hace el camino completo (imagen → WebP → EXIF → volver a
leer el paquete → volver a decodificar la imagen) y comprueba además que el
tamaño declarado del RIFF cuadre con el archivo real.

Los valores por defecto se cambian sin tocar código:

```env
STICKER_PACK=Shin-MD-Go
STICKER_AUTHOR=riokuroxi-svg
```

## 5. `.toimg`: nada de ezgif

El `toimg.js` original hacía esto:

```js
import { webp2mp4 } from '../lib/webp2mp4.js'
// dentro: POST a https://ezgif.com/webp-to-mp4 con el sticker
```

O sea: cada vez que alguien convertía un sticker animado, el archivo **se subía
a un sitio de terceros**, se esperaba a que respondiera y se bajaba el vídeo.
Eso depende de que ese sitio siga vivo, no funciona sin internet y manda a un
desconocido lo que manda el usuario.

Aquí:

1. Sticker **fijo** → se decodifica en Go y se manda **PNG** (con transparencia).
2. Sticker **animado** → `ffmpeg` local (`libx264`, fondo blanco, 6 s).
3. Si `ffmpeg` no está **o** esa compilación no sabe leer WebP animado (la
   estática de Linux, por ejemplo, no siempre lo trae), se manda el **primer
   fotograma** en PNG con una nota en el pie. Un sticker animado sale quieto,
   pero sale: nunca un "no puedo".

Ese tercer caso no es teórico: se descubrió probando con el build estático de
ffmpeg 7.0.2, que devuelve `image data not found` con los WebP animados que
genera el propio `libwebp_anim`. La prueba automática cubre los tres caminos.

## 6. Límites y seguridad

| Límite | Valor | Por qué |
|---|---|---|
| Tamaño de descarga | 16 MB | un sticker o una foto no llegan ahí; evita que un archivo gigante se coma la memoria |
| Duración del vídeo | 3 s (recorta) | WhatsApp corta los stickers animados a ~6 s y convertir 30 s tarda una eternidad |
| Tiempo de ffmpeg | 60 s y se cancela | un proceso colgado no puede bloquear el bot |
| Tamaño del sticker | ~200–600 KB | el máximo de WhatsApp es 1 MB; la prueba falla si se pasa |
| Nombre del paquete | 64 caracteres | cabe en el EXIF y no rompe el JSON |
| Argumentos | lista cerrada | una forma o un efecto inventado se rechaza con la ayuda, no se pasa a nada |
| Archivos | nunca a disco | todo va por memoria y por tuberías: no quedan temporales si el proceso muere |

`.sticker` acepta **citar** un mensaje: se puede responder a una foto con
`.s -c` y el bot coge la foto citada. Es lo que hacía `m.quoted` en Baileys.

## 7. Cómo se prueba

```bash
go test ./internal/media -v        # 21 pruebas: EXIF, formas, filtros, vídeo
go test ./cmds -run Sticker -v     # opciones y alias
```

Con ffmpeg instalado, `TestVideoAWebPConFFmpeg` genera un vídeo con el propio
ffmpeg, lo convierte, comprueba que el WebP lleva el chunk `ANIM`, le mete el
EXIF, lo vuelve a leer y saca el primer fotograma. Sin ffmpeg, esa prueba se
salta y `TestSinFFmpegAvisaBien` comprueba que el error sea el correcto.

A mano:

1. Manda una foto con el pie `.s` → llega el sticker.
2. Responde a esa foto con `.s -corazon -sepia` → llega el mismo recorte con
   forma y efecto.
3. `.toimg` citando el sticker → llega un PNG con transparencia.
4. `.stickerinfo` → nombre del paquete y autor.
5. Abre el sticker en WhatsApp → "Ver paquete" debe mostrar `Shin-MD-Go`.

---

<div align="center">

_Shin-MD-Go · AGPL-3.0-only · riokuroxi-svg_

</div>
