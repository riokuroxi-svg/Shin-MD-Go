# Música — bloque B6.0

_14-nov-2026 · versión 0.14.0_

El reproductor de música, portado del play.js de Shin-MD con una regla
distinta en su corazón: **solo catálogos que permiten bajar audio**.

## El cambio de regla

El `.play` del JS sacaba audio de YouTube con un patio trasero (ytdl-core en
`lib/play-fuentes.js`, un pipeline de lemas en `lib/ytdlp2.js` y, cuando
todo eso fallaba, yt-dlp por consola). Ese patio funcionaba cuando YouTube
no lo estaba cerrando, que era cada dos semanas, y rompía sus condiciones de
servicio.

Aquí la opción es al revés: en vez de fingir que el usuario puede bajar lo
que no debe, el comando **busca donde sí se puede**.

| Fuente | Qué da | Cuándo baja |
|---|---|---|
| **Audius** (`api.audius.co`) | audio completo, descarga libre | siempre |
| **Deezer / iTunes** | ficha + **preview oficial** de 30 s | nunca el tema completo |
| Internet Archive | audio de dominio público/BF | (`musica.FuenteArchivo`, próximos bloques) |

La línea **`Pista.Libre`** no es configurable a propósito: si el tema no
tiene licencia de descarga, la respuesta lo dice y se ofrece el preview —
**no se piratea**. Como escribimos en el motor: una función que miente es
peor que una que falta.

## Comandos

```
.play <nombre>   Busca en las tres fuentes en paralelo y ordena:
                 libre delante, después por duración. Lista numerada:
                 ▸ .play villagers santa
                 ▸ .play 2      ← baja la 2.ª de la última lista
                 · Si la única candidata libre es una, la baja directo.

.escucha <nombre>   Preview oficial de 30 s (Deezer/iTunes). Dice de dónde
                    sale; no finge que el tema completo está ahí.
```

### Cambios de UX contra el JS

- **Sin botones nativos** de WhatsApp: whatsmeow no manda quick-replies
  todavía, así que la lista es numerada y `.play n` la resuelve.
- **Sin caché en RAM ni en disco**: era el punto más frágil del play.js
  (llaves con defer dentro de defer, disco atascado) y aquí bajar una
  canción libre tarda segundos, no los minutos de yt-dlp.
- **Semáforo por chat conservado**: máx. **2 bajadas a la vez por chat**,
  para que la escucha no tumbe al bot (el play.js hacía justo esto).

## Qué el motor `internal/musica` ya garantiza

Aunque la fuente mienta, el motor se protege:

1. **Tope duro de 24 MB** por descarga (`MaxAudio`): no llena el disco de
   un teléfono con un enlace demasiado grande.
2. **Solo audio**: si el Content-Type no es audio/* o octet-stream, se
   cancela antes de escribir un byte (así que un servidor que devuelve HTML
   de error no te cuela basura).
3. **`Client.Timeout` corto (15 s)**: una fuente caída no retiene al handler.
4. **Garantía de origen**: cada `Pista` lleva `Origen` (su página), para
   que la ficha que manda `.escucha` diga de dónde salió la ficha, no magia.

## Mensajes de error honestos

Cuando falla, el usuario no ve un «ytdlp ha muerto» cifrado sino el motivo
:`«esa pista no tiene descarga libre, solo ficha y preview de 30 s»`,
`«supera 24 MB»`, `«la fuente devolvió algo que no era audio»`.

## Pruebas

6 casos nuevos contra `httptest` (ninguna petición real): las tres fuentes
se fingen en un único servidor local, se afirma que:

- `Buscar` junta las fuentes y deja el **libre primero**;
- `Descargar` baja el MP3 libre y **rechaza HTML disfrazado de audio**;
- el semáforo corta la tercera bajada del chat;
- la lista numerada y «.play 2» eligen la pista correcta;
- los motivos de fallo son los esperados (sin pistas vs. fuentes caídas).

`internal/musica` es un paquete aparte y se prueba solo con la herramienta
de laboratorio `go run ./cmd/shin-musica buscar "lofi"` contra las APIs
reales, sin arrancar WhatsApp.
