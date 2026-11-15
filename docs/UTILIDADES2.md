# Utilidades (2/2) — bloque B5.2

_14-nov-2026 · versión 0.13.0_

Segunda tanda de `cmds/utils/` del bot de JS: **las que sí tocan una API
ajena**. Cada una se comprobó contra su servicio antes de portarla — una
utilidad que miente es peor que una que no existe — y las tres que dependían de
webs frágiles (`.hd`, `.carbon`, `.tts`) cambiaron de motor para dejar de
depender de ellas. Todo el código cliente vive en `internal/apis/`, con las
URLs inyectables: las pruebas levantan un `httptest.Server` y prueban el flujo
completo **sin tocar internet**.

| Comando | Alias | Qué hace | Servicio |
|---|---|---|---|
| `.traducir [idioma] [texto]` | `tr` `translate` `trad` | Traduce (28 idiomas, citando o escribiendo) | Google gtx → MyMemory |
| `.tts <texto>` | `voz` `decirvoz` `speak` | Texto → nota de voz | Google tw-ob |
| `.acortar <url>` | `shorturl` `shorten` `acorta` | Enlace corto con botones copiar/abrir | TinyURL |
| `.crypto [moneda…]` | `btc` `precio` | USD y MXN de BTC/ETH/SOL… | CoinGecko |
| `.githubstalk <usuario>` | `ghstalk` `gh` | Ficha pública con avatar | GitHub |
| `.gitclone <enlace｜búsqueda>` | `git` | Lista 5 o baja el `.zip` del repo | GitHub |
| `.letra <artista/canción>` | `lyrics` `lyric` | Letra en trozos de ≤3500 | lyrics.ovh |
| `.hd` (citando imagen) | `remini` `enhance` `mejorar` | 2× + enfoque **local** | — |
| `.carbon [tema] <código>` | `codigoimg` `codeimg` | Imagen del código **local** | — |
| `.ptv` (citando vídeo) | `videoredondo` `redondo` `notavideo` | Vídeo redondo (≤60 s, ≤16 MB) | — |
| `.tourl` (citando archivo) | `subirurl` `aurl` | Enlace temporal (caduca 24–72 h) | litterbox |
| `.ia <pregunta>` | `ai` `chatgpt` `gemini` `chat` | IA (necesita `GEMINI_KEY`) | Gemini |

`.benchdl` **no** se porta: medía los proveedores del descargador, que aún no
existe en Go (bloque B6). Medir lo que no hay sería mentir.

---

## 1. `.traducir` — el puerto más gordo del bloque

El `src/lib/translate.js` pesaba más que muchos comandos juntos y se porta
casi 1:1:

- **Misma gramática**: `.tr inglés hola`, `.tr en` (citando), `.tr` (citando,
  va al español). Si el texto ya está en el idioma pedido, el destino se
  invierte (`es ↔ en`) — que es lo que la persona quería el 99 % de las veces.
- **Misma tabla de 28 idiomas** con bandera y los alias «como los escribe la
  gente» (ingles/english/ing, mandarín/cn, br…). `ResolverIdioma` acepta
  `pt-BR` igual que aceptaba el JS.
- **Mismo detector propio, sin red**: alfabetos que cantan solos (japonés,
  coreano, ruso, árabe…), 8 bolsas de palabras vacías, pistas de tildes con
  peso (ñ¿¡→es 2.2, ãõç→pt 2.0…) y los mismos umbrales (0.1 / margen 0.012).
  Sirve para MyMemory, que no dice de qué idioma venía.
- **Dos proveedores encadenados**: Google (`translate_a/single`, `client=gtx`)
  y después MyMemory. El parseo de cada respuesta va en función suelta y se
  prueba con JSON de mentira.
- **Misma caché** FIFO de 200 traducciones: repetir «hola → en» no vuelve a
  salir a la red.

Diferencia honesta: la tarjeta nativa del JS llevaba `botMetadata` con el
aviso «traducción automática» y la fuente con favicon; aquí ese aviso va en el
*pie* de la tarjeta de botones (`❦ traducción automática vía Google Traductor:
puede no ser exacta`) y el botón «Copiar traducción» copia igual que allá.

## 2. `.tts` — cambio de motor, dicho claro

El JS hablaba el WebSocket de Edge ReadAloud (token + `Sec-MS-GEC` rotatorio):
ese ticket **no es público**, caduca, y cuando Microsoft lo gira el comando
muere sin avisar. Se usa el GET estable de Google Traductor (`client=tw-ob`):
voz genérica en español en vez de «Dalia», pero veinte años a la espalda y
cero secretos. El texto se trocea a **180 bytes** sin romcer runas ni palabras
(el endpoint corta peticiones largas) y los MP3 se concatenan marco a marco,
que es válido en MP3. Tope: **1000 caracteres** (~1 minuto de audio: más es
lata para el grupo). Si cualquier trozo falla, se aborta entero: una nota de
voz a la que le falta una frase miente.

## 3. Lo que WhatsApp limita (`.ptv`)

Los límites del JS son los del protocolo y se conservan tal cual
(`puedeSerPTV`, probada como función pura): **60 s, 16 MB, nada de GIFs**. El
envío es el campo `ptvMessage` (66) del mensaje con un `VideoMessage` dentro —
idéntico a lo que hace Baileys en `messages.js` —, no un flag `ptt` del vídeo
(esa clave solo existe en `AudioMessage`; cae en la trampa quien la infiere
por simetría).

## 4. `.hd` y `.carbon` — dejan de depender de webs ajenas

El JS subía tu foto a una API «mejora con IA» y tu código a carbonara. Cuando
esas webs se caen, empiezan a cobrar o se quedan lo recibido, el comando miente.
Ambos son ahora **locales**:

- `.hd`: reescalado 2× con la interpolación bilineal propia + máscara de
  enfoque (unsharp). No inventa píxeles como una IA; recupera nitidez de fotos
  pequeñas. Techos: **12 Mpx de entrada, 4096 px de lado en la salida**, y la
  imagen original jamás se toca (copia defensiva antes del enfoque).
- `.carbon`: ventana oscura dibujada en el binario (barra con los tres puntos,
  esquinas redondeadas, números de línea) con la fuente monoespaciada 7×13 de
  `golang.org/x/image` (dependencia ya presente: **cero librerías nuevas**),
  los mismos **9 temas** del JS y coloreado propio: comentarios (`//`, `#`,
  `/*…*/` entre líneas), cadenas con escapes, palabras clave comunes y
  números. Un `//` dentro de una cadena **no** es comentario — hay prueba que
  lo fija, porque es el bug clásico de los resaltadores a prisa. Topes:
  **40 líneas × 88 columnas**, PNG final ×2 determinista (mismo código → mismo
  PNG byte a byte).

## 5. `.gitclone` y `.githubstalk` — GitHub sin key

API pública (60 req/hora por IP, de sobra para un bot). El regex del enlace es
el del JS (`https://` o `git@`, con o sin `.git`). Búsqueda de texto libre → 5
resultados; si solo hay uno, baja el `.zip` directo, como allá. Novedad
respecto al JS: el `.zip` tiene **techo de 45 MB** (`MaxZip`) — un repo grande
tarda minutos en subirse al chat y mientras tanto el grupo espera. Por encima
se dice con el enlace y ya.

## 6. `.tourl` — litterbox con la misma política de reintentos

Multipart `reqtype=fileupload · time · fileToUpload`; los tiempos se prueban
**24h → 72h → 1h, dos intentos cada uno** con espera progresiva (el servicio a
veces rechaza las caducidades largas), igual que el JS. Tope **64 MB**. La
URL resultante va con botones copiar/abrir.

## 7. `.ia` (`.ai`) — Gemini detrás de `GEMINI_KEY`

Igual que el JS: **sin key no finge**; explica cómo conseguirla gratis en
aistudio.google.com/apikey y salir del paso. El system prompt es el del JS,
palabra por palabra; una imagen citada entra como contexto visual
(`inline_data`, tope 4 MB). Errores traducidos a lo entendible:
**key inválida** (400/403), **cupo** (429), **tardó demasiado**.

Dos diferencias documentadas:

- **Sin memoria por chat**: el JS guardaba 10 turnos en RAM y un reinicio se
  los comía igualmente. Estado que no sobrevive a un reinicio es una mentira
  amable; aquí cada consulta es de un turno.
- En el JS el nombre era `ai`; en Go ese alias lo ocupaba `.rich`
  (experimental, `RICH_AI=1`) como placeholder. El nombre pasa al comando de
  verdad y `.rich` se queda con `markdown`/`richai`.

## 8. Lo que no cambia

- Todo va por la **cola anti-ban** y respeta el calentamiento.
- Cada comando tiene su **plazo** (`context.WithTimeout`): una API lenta no
  puede congelar al repartidor.
- `.get` sigue siendo el único que pide URLs del usuario, y con filtro
  anti-SSRF. Estas APIs hablan con dominios fijos, no con lo que mande nadie.

## 9. Pruebas del bloque

45 pruebas nuevas, ninguna toca internet:

- `internal/apis`: resolver idiomas (28 casos con alias y `pt-BR`), las 4
  formas de petición, detector (español/inglés/portugués/francés/alemán +
  escrituras ja/ko/ru/ar), parseo de Google y MyMemory, **fallback con Google
  caído**, caché (la 2ª llamada no sale a la red), acortar (ok / no-enlace /
  respuesta rara), precios (alias, orden, no-encontrada), GitHub (stalk 404 →
  `ErrNoEncontrado`, búsqueda a 5, zip con nombre del `Content-Disposition`),
  letra (formatos y troceo exacto, 404), voz (troceo sin partir UTF-8,
  concatenación, límites por petición), subida multipart con **reintentos y
  orden de tiempos** del JS, Gemini (cuerpo con `systemInstruction`, 400/403 →
  key inválida, 429 → cupo, JSON roto).
- `internal/media/mejorar_test.go`: 2× exacto, techo 4096, 13.5 Mpx rechazado,
  **original intacto**, color estable en zonas planas.
- `internal/ui/carbon_test.go`: PNG determinista, tema desconocido → verde,
  muro de 500 líneas dentro del techo, coloreado (cadena con `//` dentro,
  keywords, números, comentario de bloque entre líneas).
- `cmds/utilidades2_test.go`: la regla pura de `.ptv` (tabla del JS) y los
  formateadores de moneda/bytes/extensión.
