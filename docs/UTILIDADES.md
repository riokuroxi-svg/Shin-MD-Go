# Utilidades — bloque B5.1 (Go)

_8-nov-2026 · versión 0.12.0_

Primera tanda de `cmds/utils/` del bot de JS: **todo lo que no necesita una API
de terceros**. La segunda tanda (traducir, voz, acortar, precios, IA, letra,
carbon, hd…) queda aparte porque cada una depende de un servicio ajeno, y una
utilidad que miente es peor que una que no existe.

| Comando | Alias | Qué hace |
|---|---|---|
| `.qr <texto>` | `qrcode` `qrgen` `makeqr` | Dibuja el QR **dentro del binario** |
| `.morse <texto>` | `amorse` `codificarmorse` | Texto → morse |
| `.demorse <código>` | — | Morse → texto |
| `.recordar <min>｜<mensaje>` | `recordatorio` `reminder` `alarma` | Recordatorio guardado en la base |
| `.recordar lista` | — | Tus recordatorios pendientes en este chat |
| `.olvidar <número>` | `delrecordatorio` `borrarrecordatorio` | Borra uno tuyo |
| `.say <texto>` | `decir` | Repite el texto como el bot (o el mensaje citado) |
| `.encuesta <pregunta>｜a｜b` | `poll` `votacion` `quiz` | Encuesta, quiz o voto múltiple |
| `.pfp [@quien]` | `getpic` `fotoperfil` | Foto de perfil |
| `.wastalk <número>` | `stalknum` `numbstalk` `perfilwa` `infowa` | Información pública de un número |
| `.readviewonce` | `read` `readvo` `verveces` | Abre un mensaje de «ver una vez» |
| `.get <url>` | `fetch` `pedir` | Pide una URL (**solo el dueño**) |

---

## 1. `.qr` — el QR se dibuja aquí

El JS llamaba a `api.qrserver.com`: sin internet, sin QR. Aquí se usa
`rsc.io/qr` (BSD-3, ~900 líneas, sin dependencias) y se manda un PNG en escala
de grises de **640 px** con **4 módulos de zona de silencio** — el mínimo que
exige la norma, y el motivo por el que muchos QR generados a mano no los lee
ningún lector.

Tope de **1200 bytes** de contenido: por encima, WhatsApp reescala la imagen y
el QR deja de leerse. Es preferible decir que no a mandar algo que parece un QR
y no lo es.

## 2. `.morse` / `.demorse`

Tabla completa (letras, números y los signos que usa el morse internacional).
Al decodificar, **las letras van pegadas** y solo hay espacio donde había `/`:
es lo mismo que hace el JS, y obliga a escribir la separación cuando se quiere
(`... --- ...` → `SOS`). Los grupos que no existen en la tabla se cuentan y se
devuelven como `?`, en vez de tragárselos en silencio. El mensaje de respuesta
dice cuántos símbolos se descartaron.

## 3. `.recordar` — el aviso sobrevive al reinicio

**Lo que estaba mal en el JS:** los recordatorios vivían en
`global.ginkoRecordatorios` con un `setTimeout`. Al reiniciar o recargar el
plugin, el aviso desaparecía sin decir nada.

**Cómo está aquí:** cada recordatorio es una fila (migración **14**,
tabla `recordatorios`) y **un solo vigilante** revisa cada 20 s.

- El vigilante **selecciona y borra en la misma transacción**: un aviso que ya
  salió no puede salir dos veces, aunque el proceso muera justo después.
- Arranca con 30 s de gracia para no escribir antes de que la sesión esté lista.
- Los avisos salen con **prioridad alta**: un recordatorio llega tarde por
  definición y no puede quedarse esperando el cupo del calentamiento.
- Tope de **24 h** por recordatorio (igual que el JS); si pides más, se
  programa para dentro de un día y el mensaje lo dice.
- `LimpiarRecordatoriosViejos` borra los que llevan más de 7 días vencidos: la
  tabla no crece para siempre.
- Solo el dueño del recordatorio lo puede borrar; el `DELETE` filtra por
  usuario, no se comprueba después de leer.

Sin `SetReferencia(...)` (que llama `cmd/shin/main.go` al arrancar) el vigilante
no hace nada en vez de caerse, y las pruebas pueden ejercitar la base sin
WhatsApp.

## 4. `.encuesta` — encuesta, quiz y voto múltiple

El JS solo sabía hacer encuestas normales. Aquí hay tres formas con el **mismo
comando**:

```
.encuesta ¿Pizza?|Pepperoni|Hawaiana|Mexicana        → encuesta normal
.encuesta Capital de México|Guadalajara|*CDMX*|MTY   → quiz (la de asteriscos)
.encuesta multi ¿Qué llevas?|Hielo|Vasos|Botana      → varias respuestas
```

**Por qué el protobuf se arma a mano:** `whatsmeow.BuildPollCreation` no expone
`PollType` ni `CorrectAnswer`, así que un quiz «nativo» por el helper es
imposible. El proto `waE2E.PollCreationMessage` **sí** los tiene
(`PollType_QUIZ` y `CorrectAnswer` apuntando a la opción correcta del mismo
slice que se envía) y se manda con `SendProto`, respetando la cola anti-ban.

**El `MessageSecret` de 32 bytes es obligatorio.** Sin él la encuesta se ve,
la gente vota, y **los votos no se pueden descifrar**: es el fallo más silencioso
de todo el comando. Se genera con `crypto/rand` y hay dos pruebas encima.

Límites: 2–12 opciones, 255 caracteres de pregunta, 100 por opción, y **una sola**
respuesta correcta (dos asteriscos es un error, no se elige la primera).

## 5. `.pfp` y `.wastalk`

`.pfp` pide la foto a `GetProfilePictureInfo` con `existing=true` (nunca se
provoca una petición al contacto) y la baja **con `http.Get` sobre `info.URL`**:
`whatsmeow.Client.Download` no acepta `*types.ProfilePictureInfo` — no implementa
`GetDirectPath` — y eso rompía el build. Límite de 8 MB y 20 s.

`.wastalk` acepta un número suelto (`.wastalk 52155…`) o una mención, y responde
con lo que WhatsApp da de forma pública: si existe en WhatsApp, foto, estado
(bio) y nombre de empresa verificado. Ese último vive en
`VerifiedName.Details.VerifiedName`, no en un campo directo: mirar
`VerifiedName != nil` no basta.

## 6. `.readviewonce`

`wa.MedioDe` ya desenvuelve `ViewOnceMessage` y `ViewOnceMessageV2` y mira
también el mensaje citado, así que el comando no tiene lógica propia de
desempaquetado: solo descarga y reenvía como foto, vídeo o audio. Si el mensaje
ya se borró, se dice; no se manda un archivo vacío.

## 7. `.get` — solo el dueño, y con filtro anti-SSRF

Pedir una URL desde el servidor del bot es pedir **la red interna del servidor**
si nadie lo impide: `http://127.0.0.1:…`, `http://192.168.…`,
`http://169.254.169.254` (metadatos de la nube) o un dominio público que
resuelve a una IP privada. Por eso:

- Comando **`OwnerOnly`**.
- `utilidades.ValidarURL` con `ResolverDNSIPs`: bloquea loopback, privadas,
  enlace local, CGNAT `100.64/10`, multicast, credenciales dentro de la URL
  (`http://usuario:clave@…`) y **cualquier dominio que resuelva a una IP de
  esas**.
- **No sigue redirecciones** (`CheckRedirect → http.ErrUseLastResponse`): una
  redirección es el truco clásico para saltarse el filtro (`https://público` →
  `http://127.0.0.1`).
- Tope de **5 MB** y 20 s; el texto se recorta a 3000 caracteres y se manda en
  un bloque de código; las imágenes se mandan como imagen y el resto como
  documento con nombre deducido del tipo.

## 8. Pruebas

Sin abrir WhatsApp: `internal/utilidades/utilidades_test.go` (morse ida y
vuelta, PNG de QR con firma válida, parser de encuesta/quiz/multi y sus
rechazos, filtro anti-SSRF con un resolutor falso, recortes) y
`cmds/utilidades_test.go` (el protobuf que se envía de verdad —normal, quiz y
multi—, el secreto de 32 bytes que no se repite, los atajos de texto y de quién
se habla en un comando).

`store`: `recordatorios_test.go` — vence una sola vez, un futuro no vence, solo
el dueño borra, orden por cuándo, rechaza vacíos y limpieza de los viejos.
