# Auditoría de comandos y proveedores — noviembre de 2026

_Fecha de comprobación: 20-nov-2026 (America/Mexico_City)_

Esta auditoría revisa los comandos que dependen de servicios externos. Se
hicieron peticiones HTTP reales desde el entorno de desarrollo y pruebas de
contrato locales con `httptest`. **No sustituye una prueba desde una cuenta real
de WhatsApp**: la animación, los recibos de lectura, la presencia y los botones
aún deben comprobarse en un chat de prueba antes de publicar una versión final.

## Fallos encontrados y decisión

| Comando | Cómo respondía antes | Comprobación | Decisión aplicada | Respuesta/comportamiento actual |
|---|---|---|---|---|
| `.pat` y reacciones anime | Enviaban la frase y, cuando fallaba waifu.pics, solo quedaba texto. Un GIF tampoco se podía subir fingiendo `video/mp4`. | waifu.pics no fue utilizable; `nekos.best/api/v2/pat` y su catálogo respondieron 200. | **Reparar.** Migración a nekos.best, descarga limitada y URL validada; GIF convertido localmente a H.264 MP4 y enviado con `gifPlayback=true`. | Envía la reacción animada. Si no hay ffmpeg, usa el GIF con MIME real; si el proveedor falla, conserva el texto y explica el fallo. |
| `.waifu`, `.neko`, `.ppcouple` | La ayuda aún decía waifu.pics y dependía de ese proveedor retirado. | Los endpoints PNG `waifu` y `neko` de nekos.best respondieron 200. | **Reparar.** Unificar en nekos.best y corregir la ayuda. | Envía imágenes PNG con MIME detectado. |
| `.anime` | `Jikan no responde ahora; prueba en un rato.` | Jikan agotó el plazo de 15 s; AniList GraphQL respondió 200 por POST con ficha válida. | **Reparar.** Sustituir Jikan por AniList GraphQL. | Muestra portada, título, nota, episodios, año, sinopsis limpia y botón a AniList. |
| `.apk` | `Aptoide no responde ahora.` porque usaba `/apt/7/apksearch`, ya inválido. | `ws75.aptoide.com/api/7/apps/search` respondió 200 y `info.status=OK`. | **Reparar.** Usar el contrato API 7 actual y validar hosts de icono/APK. | Tarjeta con icono, paquete, versión, tamaño, estado antimalware y botón de descarga al host oficial. |
| `.ytsearch` | `El buscador de YouTube (Piped) no responde ahora…`; apuntaba a Kavin y decodificaba un arreglo raíz antiguo. | Ducks Party y Private Coffee respondieron 200; el contrato actual usa `{ "items": [...] }`. | **Reparar.** Dos instancias con failover, decodificación de `items`, compatibilidad antigua y validación estricta del ID de YouTube. | Lista hasta cinco videos; también conserva emisiones en vivo o resultados sin duración. |
| `.deezer`, `.dzr`, `.deezermusic` | `esa fuente no responde ahora (…403…)`. | `api.deezer.com/search` respondió 403 de forma repetida. | **Retirar.** Era un acceso directo redundante y no se sustituye por un catálogo distinto fingiendo ser Deezer. | Los nombres ya no se anuncian ni se registran. Se usan `.play` y `.escucha`, que toleran fallos parciales y conservan Audius/iTunes/spotDL. |
| `.gelbooru`, `.gbooru` | `gelbooru.com no responde ahora.` | El DAPI público respondió 401. | **Retirar.** No se incrustan credenciales y no se añade scraping frágil. | Los nombres ya no se anuncian ni se registran. Permanecen `.danbooru` y `.rule34`, detrás de `.nsfw on`. |
| `.play` | Daba texto numerado o enviaba audio directamente; no mostraba la portada ni acciones debajo. | Audius e iTunes respondieron 200; Deezer 403; spotDL ya tenía prueba real de MP3. | **Reparar UX y fallback.** | Muestra carátula validada, datos y botones para descargar, oír preview, elegir resultado y abrir la fuente. `.play N` sigue siendo respaldo; los previews M4A/AAC ya no se declaran falsamente como MP3. |
| `.rw` / `.roll` | La imagen pedía responder con `.claim`; un botón `.claim` sin identidad del roll no podía funcionar de forma segura. | whatsmeow permite pre-generar un ID oficial y usarlo en `SendRequestExtra.ID`. | **Reparar.** El mismo ID opaco se guarda en SQLite, se usa en el mensaje y viaja en `claim:<id>`. | La imagen lleva **Reclamar** debajo; citar el mensaje con `.claim` sigue funcionando. La transacción impide doble reclamo. |

## Proveedores que siguen operativos

Los siguientes contratos respondieron correctamente en la comprobación:

| Función/comandos | Proveedor | Resultado |
|---|---|---|
| `.play` | Audius | HTTP 200 |
| `.play` / `.escucha` | iTunes Search | HTTP 200 |
| `.tr` (respaldo) | MyMemory | HTTP 200 y traducción válida |
| `.tts` | Google Translate TTS | HTTP 200 |
| `.imagen` | Openverse | HTTP 200 |
| `.clima` | Open-Meteo + geocoding | HTTP 200 |
| `.letra` | lyrics.ovh | HTTP 200 |
| `.crypto` | CoinGecko | HTTP 200 |
| `.githubstalk` / metadatos GitHub | GitHub API | HTTP 200 |
| `.danbooru` | Danbooru JSON | HTTP 200 |
| `.rule34` | Rule34 DAPI | HTTP 200 |
| `.tourl` | Litterbox/Catbox | servicio accesible; el upload conserva límites locales |
| `.acortar` | TinyURL | servicio accesible |

Google Translate de texto respondió 429, pero `.tr` ya encadena MyMemory y por
eso no queda inutilizado. Gemini no puede verificarse sin `GEMINI_KEY`; `.ai`
debe seguir contestando que falta configuración cuando la clave no existe.

## Comandos dependientes del operador o de una entrada concreta

No se declaran “rotos” sin una entrada real:

- `.yt`, `.tiktok`, `.facebook`, etc. dependen del ejecutable `yt-dlp` y del
  sitio concreto. El arranque informa si falta el binario y los límites siguen
  siendo locales.
- `.play <Spotify>` depende de `SPOTDL_PATH`, ffmpeg y, según el proveedor de
  audio, cookies. El camino sin spotDL permanece disponible.
- `.mediafire` y `.gdrive` necesitan un archivo público real; conservan plazo,
  tope de tamaño, HTTPS obligatorio y revalidación anti-SSRF en cada redirección.
- `.ai` requiere una clave de Gemini proporcionada por el operador.

## Transporte y comportamiento anti-abuso

También se reparó el comportamiento común de los comandos:

- recibos de lectura con `MarkRead`; en grupos los IDs se agrupan **por
  remitente**, como exige whatsmeow;
- presencia `composing` mientras corre el handler, refrescada para trabajos
  largos y seguida de `paused`; el ciclo reserva el chat hasta completar
  `paused`, para que una generación anterior no pause una respuesta nueva;
- cola serializada, ritmo, jitter, calentamiento y rate limit existentes;
- supresión de textos, medios e interactivos exactamente repetidos durante 12
  segundos, basada en una huella del contenido visible, tipo y chat;
- imágenes, previews y descargas provenientes de APIs tienen límites, formato
  comprobado y bloqueo de destinos privados/redirecciones inseguras;
- no se falsifican ni aleatorizan versión, dispositivo, IDs u otros metadatos
  para evadir controles de WhatsApp.

## Verificación pendiente en WhatsApp real

Antes de llamar a esto release estable hay que comprobar en un chat de prueba:

1. `.pat @alguien`: animación y caption.
2. `.rw`: imagen, botón **Reclamar**, reserva de 20 s y carrera entre dos usuarios.
3. `.play <tema>`: portada, selector, preview y descarga.
4. vistos en privado y grupo; en grupo, mensajes de dos remitentes.
5. `composing` durante una descarga larga y `paused` al terminar.
6. fallback de botones en Android/iOS y en una cuenta sin soporte del formato rico.
