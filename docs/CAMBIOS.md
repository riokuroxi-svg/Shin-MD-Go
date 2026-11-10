# Cambios 0.2.0 — caza de bugs del núcleo (B0)

_Auditoría del bloque B0 · 5-nov-2026_
_Todo lo de abajo está arreglado y respaldado por una prueba que falla si el
bug vuelve. Nada de "debería funcionar": `go build`, `go vet`, `gofmt -l .` y
`go test ./...` limpios, y el binario arm64 vuelve a compilar._

---

## 1. El bot se congelaba entero (crítico)

**Síntoma:** después del primer comando *no prioritario* (`.daily`, `.perfil`,
`.encuesta`, `.lista`…), el bot dejaba de contestar **todo**, incluido `.menu`.
Sin error, sin log, sin crash: el proceso seguía vivo y mudo.

**Raíz:** la cola de envío pedía el mismo candado dos veces.

```go
// internal/wa/queue.go (antes)
q.mu.Lock()
if !priority && q.sentToday >= q.warmBudget(today) {   // ← mu ya tomado
    ...
}
// y warmBudget() empezaba con:
q.mu.Lock()   // ← el mismo candado, otra vez
```

`sync.Mutex` en Go **no es reentrante**: el segundo `Lock()` espera a que se
libere el primero, que está en la misma goroutine. Bloqueo eterno. Y como el
candado se quedaba tomado, cualquier envío posterior (`SendText`, `SendButtons`,
la propia cola) se apilaba detrás. El bot entero, muerto en silencio.

**Arreglo:** un solo candado (`mu`) para el estado y otro independiente (`gate`)
para serializar los envíos, sin anidarlos nunca. `warmBudget` pasó a
`budgetAt` y se llama **siempre** con el candado ya tomado por quien la usa
(está documentado en el código).

**Prueba:** `TestQueueNonPriorityDoesNotDeadlock`.

> Nota al margen: este fue el caro. Se veía como "el bot se colgó" y no como
> un candado, y en el JS el mismo sitio tampoco avisaba de nada.

---

## 2. El calentamiento nunca calentaba

**Síntoma:** el número se quedaba para siempre en 20 mensajes no prioritarios
al día. Nunca llegaba a 500.

**Raíz (dos capas):**

1. El ancla del warm-up vivía en memoria y se ponía a "hoy" en el primer
   envío de cada arranque → `días = 0` siempre → cupo = `WARMUP_START` siempre.
2. Al agotar el cupo la cola devolvía `ErrWarmupLimit`, pero los envíos lo
   ignoraban a propósito (`err != ErrWarmupLimit`). Resultado: el límite no
   existía, ni corto ni largo.

**Arreglo:**

- El ancla se **persiste en SQLite** (`settings`, JID de sistema) y se inyecta
  al arrancar con `Queue.SetAnchor`. Ver `store.WarmupAnchor()`.
- Se cuentan **días de calendario**, no horas: antes, a las 15:00 del primer
  día, el cupo salía en **66** en lugar de 20 (lo cazamos en el arranque de
  prueba). Ver `dayNumber()` y `TestQueueBudgetCountsCalendarDays`.
- Al pasarse del cupo **no se descarta la respuesta** (dejar al usuario en
  visto es peor que ir lento): se manda en **modo lento**, con la espera
  duplicada y techo en `2 × ANTIBAN_MAX`. `.ping` lo muestra como
  `modo lento`.

---

## 3. La "cola" no era una cola

**Raíz:** `Queue.Wait` solo dormía un tiempo aleatorio y volvía. Si dos
comandos entraban a la vez, **los dos dormían en paralelo y los dos mandaban a
la vez** → ráfaga justo cuando el motor anti-ban existe para evitarlas.

**Arreglo:** `Queue.Do(ctx, largo, priority, envio)` toma un candado, espera el
turno **descontando lo que ya pasó desde el último envío real** y solo entonces
manda. Un mensaje a la vez, siempre. Todos los envíos (`SendText`,
`SendButtons`, `SendCarousel`, `SendList`, `SendPoll`, `SendEvent`, `SendRich`,
`PinMessage`, `SendImage`) pasan por ahí. Ya no hay una sola ruta que se salte
la cola.

**Prueba:** `TestQueueSerializesSends` (cuenta cuántos envíos coinciden en el
tiempo; debe ser exactamente 1).

> Esto fue difícil de ver: los mensajes salían "bien", solo que de tres en
> tres. — Roxi

---

## 4. Las tarjetas se saltaban el cupo diario

**Raíz:** `SendButtons`, `SendList`, `SendCarousel` e `SendImage` llamaban a la
cola con `priority = true` **fijo**. Todo lo que tuviera botones era
"prioritario", es decir, el calentamiento no aplicaba a nada de lo que se
manda con tarjeta.

**Arreglo:** la prioridad es una propiedad del **comando**, no del formato.
`bot.Context` lleva `CmdPriority` (lo pone el router desde `Command.Priority`)
y todos los `Reply…` lo respetan. `Context.ReplyButtons` / `ReplyImage`
arrastran ese valor.

---

## 5. `.daily` pagaba monedas sin límite

**Raíz:** no había comprobación. Había un `key := "daily_" + fecha` y un
comentario `// (marcador de posición…)`. O sea: `.daily` daba premio cada vez
que lo escribieras.

**Arreglo:** tabla nueva `daily_claims (jid, day)` con **una sola sentencia
atómica** (`INSERT … ON CONFLICT DO UPDATE … WHERE day <> excluded.day`). Si
la fila cambió, este mensaje fue el primero del día. Sin carreras entre dos
`.daily` seguidos, sin filas basura por fecha. `.perfil` muestra si ya lo
reclamaste.

**Prueba:** `TestClaimDailyOncePerDay`.

---

## 6. `.pin` decía "responde a un mensaje" aunque estuvieras respondiendo

**Raíz:** solo miraba `ExtendedTextMessage.GetContextInfo()`. El mensaje citado
de una **foto** vive en `ImageMessage`, el de un audio en `AudioMessage`, el de
un sticker en `StickerMessage`… Con cualquier cosa que no fuera texto, el citado
"no existía". Además, en grupos, el `MessageKey` se armaba con el remitente
**del que escribe** y no con el autor del mensaje citado.

**Arreglo:** `wa.ContextInfo(msg)` mira todos los tipos de contenido (y
desenvuelve efímeros). El participante del clave sale del propio
`ContextInfo.Participant`.

**Pruebas:** `TestContextInfoFromMedia`, `TestContextInfoInsideEphemeral`.

---

## 7. Comandos que no se ejecutaban (y sin error)

**Raíz:** WhatsApp envuelve los mensajes en sobres (`EphemeralMessage`,
`ViewOnceMessage`, `DocumentWithCaptionMessage`, `EditedMessage`…). El parser
solo miraba el sobre: `.menu` en un chat con mensajes efímeros llegaba como
mensaje vacío y el bot no hacía nada. Silencio total.

**Arreglo:** `wa.Unwrap()` desenvuelve en cadena (tope de 8 niveles para no
colgarse con un sobre mal formado) y se usa antes de interpretar **y** antes de
buscar el citado.

**Pruebas:** `TestParseCommandInsideEphemeral`, `TestParseCommandDeeplyWrapped`,
`TestUnwrapBounded`.

---

## 8. El owner no era owner

**Raíz (dos casos reales):**

1. `OWNERS="+52 155 1234 5678"` se comparaba tal cual contra
   `sender.User` (`5215512345678`) → nunca coincidía. Con un `+` o un espacio
   en el `.env`, los comandos de owner quedaban inaccesibles.
2. Los chats nuevos de WhatsApp usan **LID** en vez del teléfono, y el número
   real viene en `Info.SenderAlt`. Comparar solo `sender.User` daba "no eres
   owner" a alguien que sí lo era. (Estaba anotado como "el bug clásico" en un
   comentario, pero nunca se había arreglado.)

**Arreglo:** `bot/owner.go`: `NormalizeNumber` (solo dígitos, corta `@dominio`
y `:dispositivo`), `NormalizeOwners` (quita vacíos y duplicados) y `IsOwner`
(compara teléfono **y** alternativo). Se aplica al arrancar y en el router.

**Pruebas:** `TestNormalizeNumber`, `TestNormalizeOwners`,
`TestIsOwnerPhoneAndLID`.

---

## 9. Cierres y bucles que el usuario no ve

- **`context.Background()` en cada handler:** un Ctrl+C no cancelaba nada; un
  envío en curso seguía su curso. Ahora el cliente guarda el contexto del
  proceso y los handlers lo heredan. Un `.pin` cancelado se registra como
  "cancelado", no como falla.
- **Mensajes propios reprocesados:** el bot analizaba también los mensajes que
  envía su propio número. Con un menú cuyo texto de respaldo lista
  `▸ Ping · .ping` es cuestión de tiempo acabar en bucle. Ahora `IsFromMe` se
  ignora.
- **Estados y difusiones:** contestar a `status@broadcast` es la vía rápida al
  ban. Filtrados antes de cualquier otra cosa.
- **Fuga en el cooldown:** el mapa `jid → marcas` crecía sin límite en grupos
  grandes. Ahora se poda al pasar de 256 usuarios.
- **La sesión de WhatsApp sin proteger:** `auth.db` (las llaves del número)
  se creaba con permisos `644`; la economía con `600`. Ahora `auth.db` y sus
  archivos `-wal`/`-shm` van con `600` (`secureSessionFiles`).
- **`.ping` mentía un poco:** medía el tiempo de *construir* el mensaje y lo
  llamaba latencia. Ahora muestra latencia de recepción real
  (`time.Since(Info.Timestamp)`), tiempo del handler, uptime y estado de la
  cola (`enviados/cupo`, marca de modo lento).

---

## 10. Detalles menores

- `store.Setting` metía la clave directamente en una ruta JSON de SQLite
  (`'$.' || ?`): una clave con comillas podía reescribir el JSON del usuario.
  Ahora `validKey` solo admite `[A-Za-z0-9_-]`.
- `AddXP` con un coste mayor que la XP acumulada dejaba XP negativa y nivel
  raro. Se recorta a 0 / nivel 1.
- `CommandFromButton` tenía una rama muerta (`len(args)==0 && trim != ""`,
  imposible). Simplificada.
- Log falso al cancelar la vinculación: imprimía "Shin-MD en marcha" aunque
  el QR se hubiera cancelado. Corregido.

---

## 11. El `.env` no se leía (y los owners del `.env` tampoco)

**Raíz:** el bot de JS usaba `dotenv` y cargaba el `.env` sin más. La versión en
Go solo miraba `os.Getenv`, así que copiar tu `.env` al lado del binario no
hacía nada: arrancaba sin prefijos personalizados, sin owners y con los valores
por defecto (lo vimos en el arranque de prueba: `OWNERS está vacío`).

**Arreglo:** `config.Load()` mezcla entorno + `.env` (sin dependencias, con
comillas, comentarios y `export`). El entorno del sistema **gana** sobre el
archivo, y una variable exportada pero vacía **no** borra lo que trae el
`.env`. Además se recortan valores imposibles (`ANTIBAN_BASE_MS=10` → 400 ms,
sigma negativa → 0, `WARMUP_START=0` → 1, `BOT_PREFIX=,,` → `.`), que antes
dejaban al bot sin prefijos y sordo.

Y en la misma pasada salió que `main()` leía `OWNERS` con `os.Getenv` por su
cuenta, así que aunque hubiera `.env` **los owners seguían sin llegar** (el
arranque de prueba avisaba `OWNERS está vacío` con el `.env` delante). Ahora
los owners salen de `config`, ya en dígitos: `OWNERS="+52 155 1234 5678"` es
**un** número y no cuatro, y `5211111111111 5222222222222` sí son dos.

**Pruebas:** `TestLoadFromDotEnv`, `TestLoadDefaultsNoEnvFile`,
`TestLoadClampsAbsurdValues`, `TestEnvOverridesDotEnv`, `TestOwnersFromDotEnv`.

---

## Estado de los bloques

| Bloque | Estado |
|---|---|
| **B0 · Núcleo** | ✅ hecho · **revisado y corregido en 0.2.0** |
| B1 · Router real | 🚧 en curso — ya van: `IsFromMe`, broadcast, owner por LID y por teléfono, contexto cancelable, poda de cooldown. Faltan hooks `before` (antilink, antistatus, afk), self-mode y `isBotAdmin` real con `GetGroupInfo` |
| B2–B8 | ⬜ sin cambios |

---

## Cómo se comprueba

```bash
go build ./...        # compila
go vet ./...          # limpio
gofmt -l .            # sin salida
go test ./...         # verde (≈5 s; las pruebas de la cola esperan de verdad)
make termux           # arm64 estático, 17 MB
```

Pruebas nuevas en esta ronda: **25** (8 de la cola, 5 de envoltorios y contexto
citado, 3 de owners, 4 de base de datos, 5 de configuración). Total del
repo: **42**.

---

# Cambios 0.4.0 — bloque B2 y revisión de seguridad

## 12. El dibujo del menú no filtraba caracteres invisibles

**Raíz:** `ui.sanitizeText` quitaba los caracteres de control (`< 0x20`) pero no
los invisibles de Unicode. El importante es **U+202E (right-to-left override)**:
mete el texto al revés en lo que se dibuja, así que sirve para falsificar la
imagen del menú (por ejemplo, para que un comando falso parezca el oficial).

**Arreglo:** se filtran U+200B–U+200F, U+202A–U+202E, U+2060 y U+FEFF, además
de convertir el espacio duro. Lo cazó la prueba `TestSanitizeText` mientras la
escribía, no un usuario en producción.

## 13. Nombre de perfil con formato (inyección en los avisos del bot)

**Raíz:** el `PushName` es texto libre y se metía tal cual en los mensajes
(`"🔗 Antilink — " + nombre + ", los enlaces…"`). Alguien llamado `*Owner*` hacía
que el aviso del bot pareciera firmado por otra persona.

**Arreglo:** `bot.CleanName` (limpia, recorta, escapa) y `bot.ClipText` para
todo texto de usuario que el bot repita. También se escapa el nombre de
categoría en `.menu <x>`.

## 14. Los hooks no tenían ningún límite

**Raíz:** el cooldown protegía los comandos, pero el antilink respondía a
**cada** mensaje con enlace. 50 enlaces seguidos = 50 mensajes salientes del
bot: spam saliente, o sea ban.

**Arreglo:** `bot.RateGuard` (3 avisos/min en antilink, 1 cada 10 s en afk, 2
cada 10 min en `.report`), con aviso único cada 5 minutos y mapa acotado a 2048
usuarios. El afk además limita a 5 mencionados por mensaje.

## 15. Topes de WhatsApp en `.encuesta`

**Raíz:** la pregunta y las opciones iban tal cual. WhatsApp admite 255
caracteres de pregunta, 100 por opción y 12 opciones; pasado eso el mensaje se
envía y el teléfono lo tira (o queda a medias), sin error visible.

**Arreglo:** recorte a esos límites y `splitSemi` con techo de 20 trozos.

## 16. `.info` contaba datos internos a cualquiera

**Raíz:** usuarios registrados, identidades conocidas y desde cuándo calienta el
número se mostraban a quien escribiera el comando.

**Arreglo:** esos datos solo los ve el owner; el resto ve versión, uptime,
conexión, cola y memoria.

## 17. Nombres de comando sin validar

El id de un botón es un dato del mensaje. Ahora el router exige
`[A-Za-z0-9_-]` y 32 caracteres como máximo antes de tocar el registro
(`bot.IsSafeCommandName`).

**Pruebas nuevas en esta ronda:** 22 (fax, límites, salida, seguridad).
Total del repo: **84**.

# Cambios 0.5.0 — bloque B3 (economía y gacha)

Estos fallos son del **bot de JS original**, no del puerto: se documentan aquí
porque el código de Go está escrito para no repetirlos, y cada uno tiene su
prueba.

## 18. El dinero se duplicaba con dos mensajes a la vez

**Raíz:** cada comando hacía `const user = db.getChatUser(...)` → calculaba en
JavaScript → `db.setChatUser(...)`. Entre la lectura y la escritura caben todos
los mensajes que quieras: dos `.shop espada` simultáneos leen 1 000 y los dos
escriben 200. Una espada gratis. Lo mismo en `.deposit`, `.withdraw`,
`.givecoins` y en los enfriamientos (`.work` dos veces seguidas pagaba dos
veces).

**Arreglo:** cada movimiento es **una sola sentencia con la condición dentro**
(`UPDATE ... SET coins = coins - ? WHERE ... AND coins >= ?`). Si no se cumple,
`RowsAffected()` es 0 y el comando avisa. Pruebas:
`TestSpendConcurrenteNoDuplica` (40 cobros de 100 sobre 1 000 → pasan 10),
`TestTransferConcurrente` (8 transferencias con banco para una → el total no
cambia), `TestClaimCooldownConcurrente` (20 `.daily` a la vez → cobra 1),
`TestClaimRollConcurrente` (10 reclamos del mismo personaje → gana uno).

## 19. Los saldos podían quedar en negativo o fuera de rango

**Raíz:** `.withdraw` restaba sin comprobar contra el banco del momento y no
había techo. Un saldo negativo (o `NaN` cuando `parseInt` devolvía `NaN`)
contagiaba rankings, robos y compras.

**Arreglo:** recorte en SQL a `[0, 1 000 000 000 000]`, transferencias con tope
de 1 000 000 000, objetos con tope de 9 999 unidades, `parseAmount` estricto
(`2500abc`, `-500`, `1..2` y `0` se rechazan).

## 20. `.daily` y las rachas se podían cobrar dos veces

**Raíz:** `lastDailyGlobal` se leía y se escribía por separado, y la racha subía
en JavaScript. Dos mensajes simultáneos daban dos premios y dos días de racha.

**Arreglo:** la racha se calcula **en el mismo UPDATE** (`CASE` de reinicio y de
tope) y solo entra si ha pasado la cadencia: `ClaimStreak`. El premio del día se
reserva con `ClaimCooldown` antes de pagar. Pruebas: `TestRachaDiariaSubeYBaja`,
`TestRachaTope`, `TestRachaEsGlobalNoPorChat`.

## 21. El gacha guardaba los rolls en un JSON que crecía para siempre

**Raíz:** cada `.roll` metía una entrada en `g_chats.rolls` y **nunca se
borraba**; `characters` se apuntaba en tres sitios a la vez (tabla
`characters`, tabla `user_characters` y el JSON `g_chat_users.characters`, que
además podía ser array o texto).

**Arreglo:** `gacha_rolls` (con mensaje, reserva y caducidad) y `gacha_owned`
(clave `(chat, personaje)`: un solo dueño por chat). Los rolls caducados se
borran al arrancar (`PurgeRolls`). `.claim` responde al **id del mensaje**
citado, no a una búsqueda en texto, y el reclamo es un UPDATE condicional
(`TestClaimRollBasico`, `TestUnDuenoPorPersonaje`).

## 22. El gacha entero dependía de un archivo que no estaba en el repo

**Raíz:** las 18 copias de `loadCharacters` leían `./core/characters.json`, que
no existe en el repositorio ni lo genera ningún script: un clon limpio tenía 18
comandos rotos con `ENOENT`. Y las ilustraciones se pedían a tres tableros, uno
de ellos con una clave de API escrita dentro del código.

**Arreglo:** `internal/gacha` con una sola lectura, caché por fecha de
modificación, aviso único si falta el archivo y **varias formas de JSON
aceptadas** (la del JS, lista suelta y lista de series). Las imágenes se buscan
en un solo tablero de contenido seguro, **sin claves en el código**, con 8 s de
espera y 4 MB de tope; si no hay imagen, el personaje se anuncia igual
(`internal/gacha/catalogo_test.go`, `imagen_test.go`).

**Pruebas nuevas en esta ronda:** 66. Total del repo: **150**.

# Cambios 0.6.0 — bloque B4 (grupos y admin)

## 23. Las advertencias se perdían

**Raíz:** `chat_users.warnings` era un array JSON dentro del usuario. Para poner
una advertencia había que leerlo entero, añadir al principio y reescribirlo. Dos
`.warn` a la vez (dos admins, o un admin apurado) leían la misma lista y el
segundo borraba el trabajo del primero. Y si el JSON se corrompía, el `catch`
dejaba al usuario **sin advertencias para siempre**, sin ningún aviso.

**Arreglo:** tabla `group_warns` (una fila por advertencia, con motivo, autor y
fecha). Borrar la #3 es un `DELETE`, contar es un `COUNT(*)`, y 20 `.warn`
simultáneos se guardan los 20 (`TestWarnConcurrente`).

## 24. El conteo de mensajes reescribía un JSON en cada mensaje

**Raíz:** `chat_users.stats = { "2026-11-05": { msgs: 42 }, … }`. En **cada
mensaje** del grupo se leía ese objeto completo y se volvía a escribir. En un
grupo con un año de historia son cientos de claves leídas y escritas miles de
veces al día, y con mensajes simultáneos se perdía la cuenta.

**Arreglo:** `chat_activity(chat_id, user_id, day, msgs)` con
`INSERT … ON CONFLICT DO UPDATE SET msgs = msgs + 1`: una sentencia, sin lectura
previa. Probado con 50 mensajes en paralelo (`TestActividadConcurrente`).

## 25. antistatus borraba el mensaje de otra persona

**Raíz:** al borrar un estado compartido, el JS elegía a quién borrar con una
cadena de condicionales sobre campos que a veces venían vacíos y caía en
`msg.sender` como último recurso. En una cita de un estado, el borrado podía ir
a parar al mensaje equivocado (o a nadie).

**Arreglo:** solo se borra lo que se identifica con certeza (el mensaje que **es**
un estado de grupo, o el citado con su autor real tomado de `contextInfo`), el
borrado pasa por la cola anti-ban y **si el borrado falla no se advierte**: sin
prueba, no hay acusación.

## 26. `.kick all`, `.kick num` y `.kick inactive` vaciaban grupos

**Raíz:** tres modos que expulsaban en bucle (con 3 s de pausa) a todos los que
no fueran admin, a todos los de un prefijo de país, o a los que llevaban 30 días
callados. Un `.kick all` mal escrito vacía un grupo de 500 personas, y esas
personas no vuelven: no hay "deshacer".

**Arreglo:** esos modos **solo listan** a quién afectarían, con su número y un
aviso explícito de que no se ha expulsado a nadie. La expulsión individual exige
nombrar a la persona. Además, antes de tocar a nadie se comprueba que no sea el
creador del grupo, un admin, el dueño del bot o el propio bot (ver
**docs/GRUPOS.md §2 y §3**).

**Pruebas nuevas en esta ronda:** 20. Total del repo: **170**.

---

# Cambios 0.7.0 — bloque B4.1 (juegos de azar)

Los tres fallos de esta tanda son de **probabilidad**, no de código: el juego
contestaba perfectamente, pero pagaba más de lo que cobraba y eso se comía la
economía del grupo en silencio.

## 27. La ruleta pagaba ×5 con un 24 % de acierto

**Síntoma:** `.rt 1000 verde` acertaba muchísimo más de lo que debería y el
saldo del grupo subía sin parar. Con `.rt all verde` se podía fabricar dinero
en serie.

**Raíz:** `cmds/economy/roulette.js` tiraba un número del 0 al 36 y pintaba de
verde los **9 primeros**:

```js
const random = Math.floor(Math.random() * 37)
if (random <= 8) { /* verde */ paga = amount * 5 }
```

Nueve casillas de 37 son el 24 %; pagando ×5, cada moneda apostada devolvía
1.22. No era una ruleta: era una fuente de dinero.

**Arreglo:** la rueda europea real —**18 rojos, 18 negros y el 0 verde**— con
la disposición auténtica de los números. Rojo/negro pagan ×2 (devolución 97.3 %)
y el número exacto ×36. `TestRuletaColoresDeLaRueda` cuenta las casillas
(18/18/1) y `TestRuletaDevolucion` simula 200,000 tiradas y falla si la
devolución se sale del 93–100 %.

## 28. El casino tiraba dados cargados contra el jugador

**Síntoma:** `.apostar` perdía casi siempre. La gente dejó de usarlo y no sabía
por qué.

**Raíz:** dos rangos distintos para el mismo duelo:

```js
let Aku = Math.floor(Math.random() * 101)  // el jugador: 0–100
let Kamu = Math.floor(Math.random() * 55)  // el bot:     0–54
```

El bot ganaba el **85 %** de las tiradas y el mensaje lo presentaba como un
duelo justo.

**Arreglo:** los dos tiran 1–100. El empate se queda la casa y por eso el premio
es ×1.9 en vez de ×2 (devolución ≈ 94 %), que es exactamente lo que un casino
hace de verdad. Y ahora se explica en `docs/JUEGOS.md`.

## 29. Las pérdidas y las multas escribían saldos negativos

**Síntoma:** bancos en −3,000; salud en −7; stamina en 118.

**Raíz:** todas las actividades (`crime`, `rob`, `pescar`, `cazar`, `minar`,
`dungeon`, `adventure`) restaban así:

```js
if (user.coins >= cantidad) user.coins -= cantidad
else { const resto = cantidad - user.coins; user.coins = 0; user.bank -= resto }
```

Nada comprobaba que el banco tuviera ese resto, y la salud/stamina se restaban a
mano sin recortar. Con dos comandos a la vez, la misma resta se aplicaba dos
veces sobre valores leídos antes de escribir.

**Arreglo:** dos operaciones nuevas en la capa de datos:

- `Charge()` cobra primero de la cartera y luego del banco, **en una sola
  sentencia** con `MAX(…, 0)`, y devuelve lo que realmente pudo cobrar (que es
  lo que se le dice al jugador).
- `AddStat()` suma o resta salud/stamina/magia recortando siempre a `0..100`
  dentro del SQL.

**Pruebas nuevas en esta ronda:** 21. Total del repo: **191**.

---

# Cambios 0.8.0 — bloque B5 (stickers y medios)

## 30. `.toimg` subía los stickers a ezgif.com

**Síntoma:** convertir un sticker animado fallaba una de cada tres veces, y a
veces tardaba 20 segundos. En un servidor sin salida a internet, nunca.

**Raíz:** `cmds/utils/toimg.js` no convertía nada: mandaba el archivo por POST a
`https://ezgif.com/webp-to-mp4` y esperaba la respuesta. La conversión dependía
de un sitio de terceros — sin avisar, sin límite y mandándole al sitio lo que el
usuario había mandado al bot.

**Arreglo:** `ffmpeg` local (`libx264`, fondo blanco porque el alfa de un
sticker queda negro en vídeo, 60 s de tope y cancelación). Y si no hay ffmpeg
—o esa compilación no sabe leer WebP animado, que pasa con la estática de
Linux— se manda el **primer fotograma** en PNG con una nota. Los tres caminos
están probados.

## 31. El sticker salía sin autor (y el EXIF se escribía con una librería de terceros)

**Síntoma:** el sticker llegaba, se veía bien, y al abrir "Ver paquete" salía
vacío.

**Raíz:** dos cosas. La primera es que el paquete vive en un chunk `EXIF` que hay
que escribir a mano (22 bytes de cabecera TIFF + JSON con la longitud en el byte
14). La segunda, la que casi nadie recuerda: el WebP lleva un chunk `VP8X` con
un byte de banderas donde hay que **marcar el bit 0x08** para declarar que hay
EXIF dentro. Sin ese bit el archivo es válido, el sticker se ve y WhatsApp
ignora el paquete.

**Arreglo:** `internal/media/exif.go` escribe y **lee** el paquete (42 líneas,
sin `node-webpmux`), marca el bit de EXIF y reemplaza el paquete anterior en vez
de acumular bloques. `TestEXIFIdaYVuelta` hace el camino completo y comprueba
que el tamaño declarado del RIFF cuadre con el archivo real.

**Extra del mismo bloque:** el WebP que escribimos lleva formato extendido
(`VP8X` + `VP8L`), y los decodificadores de Go solo aceptan `VP8L` "pelado" —
nuestro propio `.toimg` no podía leer nuestros propios stickers. Se arregló
quitando el envoltorio al leer (`desenvolverWebP`), no al escribir: el archivo
que se manda a WhatsApp conserva su `VP8X`.

**Pruebas nuevas en esta ronda:** 24. Total del repo: **215**.

---

# Cambios 0.8.1 — concurrencia (auditoría con `-race`)

## 32. Un mensaje lento dejaba al bot sordo (y la cola de WhatsApp se llenaba)

**Síntoma:** si alguien mandaba un vídeo al `.sticker` (o un `.toimg` de un
sticker animado), el bot dejaba de contestar entre 10 y 60 segundos. Después
llegaba una ráfaga de respuestas viejas, o directamente ninguna.

**Raíz:** whatsmeow **no paraleliza los handlers**. Su bucle de nodos
(`handlerQueueLoop`) llama al handler y espera a que termine antes de coger el
siguiente nodo de una cola interna de 2048. Como `.sticker` hace tres cosas
lentas seguidas (descargar el archivo, convertirlo con ffmpeg y subir el
resultado), todo lo que llegaba mientras tanto se apilaba — y una conversación
activa en dos grupos basta para llenar 2048 nodos. Cuando se llena, los mensajes
se descartan y el bot "pierde" comandos.

**Arreglo:** `internal/bot/dispatcher.go`. Cuatro gorutinas fijas (configurable,
tope 16) leen de una cola con techo (128 por defecto). Si la cola se llena, el
mensaje se descarta **y se cuenta**, en vez de bloquear la conexión. Lo que no
se rompe al paralelizar:

- **El dinero**, porque cada movimiento es una sentencia atómica en SQLite;
- **el espaciado anti-ban**, porque los envíos siguen pasando por `wa.Queue`,
  que serializa;
- **el orden dentro de una conversación**, que sí puede cambiar (dos mensajes
  seguidos pueden responderse al revés si uno tarda más). En este bot ningún
  comando depende de eso.

`.info` muestra ahora `Mensajes: 2 de 4 activos · 1/128 en espera · 0 descartados`.

## 33. Una descarga de medios atascada se comía un worker para siempre

**Síntoma:** latente. El `ctx` que se pasaba a `DownloadMedia` era el del
proceso, no el del mensaje: **sin plazo**.

**Raíz:** `whatsmeow.Download` respeta el contexto, pero el contexto no tenía
fecha límite. Una descarga que se cae a la mitad puede quedarse esperando
indefinidamente (conexión medio abierta, servidor que no cierra). Con un solo
handler eso solo colgaba el bot; con varios, cada descarga atascada se come un
carril hasta que no queda ninguno.

**Arreglo:** 45 segundos de plazo dentro de `DownloadMedia` y un mensaje claro
("la descarga tardó más de 45s y se canceló"). Como el repartidor las cancela,
el carril vuelve a estar libre.

## 34. Dos punteros globales sin protección (carrera latente)

`activeRouter` y el repartidor se leían desde los eventos de grupo y desde
`.info` (otra gorutina) mientras el arranque los escribía. En la práctica no
petaba porque se escriben una vez al principio, pero es exactamente el tipo de
carrera que aparece una vez cada mil reinicios y que `-race` sí ve. Ahora son
`atomic.Pointer`.

## 35. Un bug de mi propio despachador, cazado por las pruebas

La primera versión guardaba el canal de trabajo en un campo y `Close()` lo ponía
a `nil`. Los workers leían el campo en cada vuelta del `select`, así que al
cerrar se quedaban esperando en un canal `nil` — y recibir de un canal `nil`
bloquea **para siempre**. Las pruebas (`TestCierreNoSeCuelga`) lo detectaron
antes de subir nada: ahora los workers se quedan con su propia copia del canal.

**Auditoría completa con `-race`:** `go test -race ./...` pasa en los 8 paquetes
(215 → **226 pruebas**). Se revisaron además, uno por uno, los patrones que
suelen romper un bot en Go: mapas compartidos sin candado (los cuatro tienen su
`sync.Mutex` y su techo de tamaño), lecturas de campos de `Context` desde otras
gorutinas (cada `Context` es de un solo mensaje), `WaitGroup.Add` dentro de la
gorutina (aquí se llama antes), `time.After` en bucles calientes (no hay bucles
calientes) y transacciones SQLite que abren otra conexión con `MaxOpenConns(1)`
(las tres transacciones piden todo lo que necesitan **antes** de `BeginTx`).

## 36. Direcciones de origen propias (`internal/red`)

El bot no tenía forma de repartir su tráfico saliente: todo salía por la misma
dirección. Cuando se empiecen a resolver enlaces (bloque B6), eso concentra el
patrón y es justo lo que dispara un bloqueo.

**Qué se añadió.** Un paquete nuevo, `internal/red`, con tres piezas:

- `Rotador`: reparte las peticiones entre las direcciones de un prefijo propio
  (`RED_PREFIJO`), respetando un tope por dirección y una ventana de tiempo.
  Reserva la primera dirección del bloque (router del segmento en IPv6, red en
  IPv4) y nunca la entrega.
- `Transporte`: devuelve un `*http.Transport` que sale por una dirección
  distinta en **cada conexión**. Desactiva keep-alive a propósito: reutilizar
  una conexión reutiliza su dirección de origen, que es lo que se quiere evitar.
- `Estado`: resumen para `.info`.

**Configuración.** `RED_PREFIJO` (vacío = comportamiento anterior, sale por la
dirección del sistema), `RED_MAX_POR_IP` (def. 50) y `RED_VENTANA` en segundos
(def. 60). Los tres se recortan a valores razonables con `vars.num`, igual que
`WORKERS` y `QUEUE`. Un prefijo mal escrito **no tumba el bot**: se avisa por
registro y se sigue saliendo por la dirección del sistema.

**Validación de prefijos.** Se rechaza cualquier bloque más grande que /32 en
IPv6 o /8 en IPv4: `::/0` no es un bloque de origen, es media internet. Lo
detectó la prueba `TestNuevoRechazaBasura`, no una revisión a ojo.

**Pruebas (9 nuevas, 226 → 235).** La que importa es
`TestTransporteCambiaLaDireccionDeOrigen`: levanta un servidor HTTP local y
comprueba que cada petición llega desde una dirección distinta y que ninguna se
repite con `MaxPorIP=1`. Se ejecuta sobre loopback (`127.0.0.0/8`), que funciona
en cualquier Linux sin privilegios; en un servidor con `/64` es el mismo camino
de código.

**Despliegue.** `deploy/red-ipv6.sh` con cuatro modos (`revisar`, `montar`,
`probar`, `deshacer`). El modo `montar` configura `ip_nonlocal_bind`, enruta el
prefijo a `lo` y **cierra la entrada al bloque con nftables**: rechaza conexiones
nuevas hacia el prefijo y solo acepta las ya establecidas. Sin eso, el bloque se
convierte en un proxy abierto y el proveedor termina suspendiendo el servidor.

**Lo que este paquete no hace, y es deliberado:** no genera `poToken`, no salta
captchas ni ninguna protección de plataforma. Administra direcciones propias y
nada más.

## 37. Música: catálogos que sí permiten descargar (`internal/musica`)

El bot no tenía forma de buscar ni entregar música. El bloque añade tres fuentes
con permisos distintos, y la diferencia entre ellas está en el código, no en la
configuración:

| Fuente | Qué entrega | `Pista.Libre` |
|---|---|---|
| Audius | audio completo (catálogo libre y descentralizado) | `true` |
| Deezer | ficha + preview oficial de 30 s | `false` |
| iTunes | ficha + preview oficial de 30 s | `false` |

`Descargar` se niega con `ErrNoLibre` cuando la pista no es libre. No es un
ajuste: es una negativa deliberada, y hay una prueba que la fija
(`TestDescargarSeNiegaConPreview`).

**Búsqueda combinada.** `Buscar` consulta las tres fuentes en paralelo y
devuelve `Busqueda{Pistas, Fallos}`: una fuente caída no invalida las demás, y
el fallo queda registrado con su motivo. Ordena primero lo que tiene audio
completo.

**Tres guardas que salieron de probar contra las APIs de verdad:**

1. **`is_streamable`.** Audius devuelve en la búsqueda pistas que después
   responden `404` al pedirles el flujo. Ahora se filtran. Lo descubrió una
   descarga real que falló, no una revisión de código.
2. **`artwork.mirrors` es una lista**, no una cadena. Un `map[string]string`
   rompía el análisis de la respuesta entera; ahora se lee como `RawMessage` y
   se filtra tamaño por tamaño.
3. **Tope de 24 MB.** Un `mix` de tres horas pesa 436 MB y el intento se aborta
   por cabecera, sin descargar nada. La herramienta de laboratorio, además,
   pasa a la siguiente candidata en vez de rendirse.

**Plazos y honestidad.** Todo lleva `context` con plazo, la descarga se corta
por `LimitReader`, y el `User-Agent` identifica al bot con su repositorio: no se
hace pasar por un navegador.

**Pruebas (10 nuevas).** Respuestas reales recortadas como fixtures, más casos
de tope, contenido que no es audio, plazo agotado y fuente caída. El banco de
pruebas es `cmd/shin-musica`:

```sh
go run ./cmd/shin-musica buscar "lofi"
go run ./cmd/shin-musica bajar  "chill instrumental" -o /tmp/x.mp3
```

**Verificación real (desde una IP de datacenter, la peor para estas pruebas):**

```
bajando: CITY INSTRUMENTAL chill — iconoschill (3:30) · fuente audius
  8387658 bytes (8.00 MB) en 1.279s · 6405 KB/s
  cabecera: MP3 con etiqueta ID3 ✓
```

`file(1)` lo confirma: `Audio file with ID3 version 2.4.0, contains: MPEG ADTS,
layer III, v1, 320 kbps, 48 kHz, Stereo`.

## 38. Imagen de ejecución (`deploy/sandbox`)

`Dockerfile` multi-etapa: compila con CGO desactivado (SQLite es Go puro) y la
imagen final solo lleva el binario, ffmpeg y certificados, corriendo como
usuario sin privilegios. Medido: binario estático de **19 MB**, frente a los
~700 MB del toolchain. `docker-compose.yml` trae dos servicios: `shin` (el bot)
y `musica` (el banco de pruebas), compartiendo el volumen de la sesión.

---

# Cambios 0.10.0 — bloque B3.1 (perfil, nivel y matrimonio)

_Port de `cmds/profile/*.js` · 10-nov-2026_
_`go build`, `go vet`, `gofmt -l .` limpios; `go test ./...` → 290 pruebas;
`go test -race -count=1` verde en los tres paquetes que toca esta ronda._

---

## 39. Había tres curvas de XP y el nivel no cuadraba consigo mismo

**Síntoma:** el nivel que enseñaba `.level` no era el que enseñaba `.profile`, y
el que guardaba la base era un tercero. En `.lboard` aparecía gente de nivel 0
que nunca había escrito.

**Raíz:** tres fórmulas para lo mismo, en tres sitios:

```js
// src/lib/xp.js — la de .level
const CRECIMIENTO_LEVEL = (Math.PI / Math.E) ** 1.618 * Math.E * 0.75
// cmds/profile/profile.js — la del perfil
const CRECIMIENTO = (Math.PI / Math.E) ** 1.618 * Math.E * 0.75
// el almacén, al dar XP
level = Math.floor(xp / (level * 100))   // ← nada que ver con las otras dos
```

Las dos primeras eran la misma expresión con dos nombres: bastaba tocar una para
que los dos comandos dijeran cosas distintas. La tercera, escondida en el
almacén, es la que fijaba el nivel real.

**Arreglo:** una sola curva, en `internal/xp/xp.go`, y el almacén la usa a ella
(`AddXP` → `xp.NivelPara`). Tramos contiguos por construcción
(`Min(n+1) == Max(n)+1`), tope de 10 000 niveles y niveles que **empiezan en 1**
(antes la cuenta nacía en 0 y el primer mensaje la subía sin explicación).

**Pruebas:** `internal/xp/xp_test.go` (7) y `TestNivelCoherenteConLaCurva` en el
almacén, que da XP a mano y compara el nivel guardado con el que dice la curva.

---

## 40. `.marry` casaba a dos personas sin que la segunda escribiera nada

**Síntoma:** gente casada sin haber aceptado nada; para deshacerlo había que
convencer al otro de que escribiera `.divorce`.

**Raíz:** en `cmds/profile/marry.js` la primera llamada **solo proponía**, pero
la confirmación era volver a escribir `.marry` con el nombre del otro. Si dos
personas se mencionaban en el mismo grupo, el estado se cerraba sin permiso
explícito; y la escritura eran dos `setUser` seguidos, así que un corte entre
ambos dejaba a una casada y a la otra no.

**Arreglo:** propuesta → aceptación (`.acepto` o `.marry` de vuelta, que es como
se confirmaba en el JS y la gente ya conoce), caducidad de 10 minutos, un solo
uso, y el matrimonio en **una transacción** (`Casarse`). Las propuestas viven en
memoria con mutex: los handlers corren en goroutines y un `map` sin candado en
Go no da error, tumba el proceso.

**Pruebas:** `TestPropuestaDeUnSoloUso`, `TestPropuestaCaduca`,
`TestPropuestaReemplazaLaAnterior`, `TestCasarseYDivorciarse` (con `-race`).

---

## 41. El AFK no pagaba nada (y en el JS sí)

**Síntoma:** al volver del modo ausente, el bot solo decía «volvió». En el bot
viejo pagaba por el tiempo ausente.

**Raíz:** el pago vivía en `cmds/profile/afktime.js`, un gancho `before` que no
se portó cuando se hizo el hook de AFK del bloque B1. Además la marca de «desde
cuándo» se guardaba como texto (`02/01 15:04`), y con texto no se puede calcular
un tiempo.

**Arreglo:** la marca pasa a guardarse en milisegundos (se sigue mostrando con
el mismo formato de antes) y `recompensaAFK()` aplica la regla del JS —8 monedas
por minuto y un bono al azar de 300 a 1500 cada 3 horas— con el reloj como
parámetro. Si la marca no se entiende o está en el futuro, **no se paga**: mejor
cero que inventar dinero.

**Pruebas:** `cmds/proteccion_test.go` (recompensa por tramos, marca ilegible,
reloj hacia atrás, formato de fecha).

---

## 42. Dos comandos peleándose por el mismo nombre (`setdesc`)

**Síntoma:** `go test ./cmds` falló al añadir el perfil con
`el nombre "setdesc" lo usan "setgpdesc" y "setdesc"`.

**Raíz:** al portar los grupos se le puso el alias `setdesc` a `.setgpdesc`,
pero en el JS `setdesc` es **la descripción del perfil** (`cmds/profile/setdesc.js`)
y el comando de grupo es solo `setgpdesc`. El primero que se registra gana, así
que la descripción del grupo habría quedado inalcanzable para siempre.

**Arreglo:** el alias vuelve a donde estaba en el JS. La prueba
`TestSinAliasRepetidos` (ya existente) es la que lo detectó; ahora también hay
`TestComandosDePerfilRegistrados` para los 14 comandos del bloque.

---

## 43. Las migraciones estaban numeradas de mentira

**Síntoma:** los comentarios decían «migración 11» a tres escalones distintos y
la documentación de los juegos hablaba de la 12 cuando la de los juegos era la
10.

**Raíz:** los números de los comentarios se escribieron a mano y se quedaron
viejos al insertar escalones en medio. El número real es la **posición** en la
lista, así que la documentación mentía.

**Arreglo:** renumerados los comentarios de `internal/store/db.go` según su
posición real (juegos 10, grupos 11, rolls 12, perfil **13**) y corregidas las
tres referencias en `docs/ECONOMIA.md`, `docs/JUEGOS.md` y `docs/MIGRACION.md`.
Ninguna migración publicada se editó: los datos ya migrados no se tocan.

**Pruebas:** `TestMigracionesSinEscalonesVacios` (ningún escalón vacío) y
`TestColumnasDelPerfilEnLaBase`, que lee `pragma_table_info('users')` y
comprueba que las columnas existen de verdad en el archivo, no solo en el código.

---

# Cambios 0.11.0 — bloque B7.0 (operación: estado y comandos de dueño)

_Port de `cmds/main/*.js` y `cmds/owner/*.js` · 10-nov-2026_
_`go build`, `go vet`, `gofmt -l .` limpios; `go test ./...` → 310 pruebas._

---

## 44. `.restart` podía dejar el bot apagado para siempre

**Síntoma (JS):** con `npm start` no hay supervisor: el comando hacía
`process.exit(0)` y el bot se quedaba muerto hasta que alguien mirara el
teléfono.

**Arreglo:** antes de salir se pregunta quién nos levanta otra vez.
`caminoDeReinicio(haySupervisor, puedeExec)` devuelve `systemd`, `exec` o
`sin-salida`, y en el último caso **no se sale**: se contesta que no se puede y
cómo arreglarlo. Si el camino es `exec` y el hijo no arranca, el padre se queda
vivo. Hay prueba de los cuatro casos.

**Pruebas:** `TestCaminoDeReinicio`, `TestHaySupervisorPorEntorno`.

---

## 45. `.subir` escribía donde le dijeran

**Síntoma:** el candado del JS era una comprobación de prefijo sobre la ruta ya
resuelta; funcionaba, pero dejaba casos raros: rutas absolutas que acababan
escritas dentro del repo, un `.git` anidado de un submódulo que no estaba
protegido y una carpeta hermana con el mismo prefijo (`bot-otro`).

**Arreglo:** `rutaSegura()` rechaza rutas absolutas, byte nulo, cualquier
componente `.git` (no solo el de la raíz) y compara con el **separador**
incluido, así que `bot-otro` no se confunde con `bot`. Nueve rutas malas y cinco
buenas en la prueba.

**Pruebas:** `TestRutaSeguraAceptaDentroDelRepo`, `TestRutaSeguraRechazaEscapes`,
`TestRutaSeguraNoConfundePrefijos`.

---

## 46. Los errores solo existían en el log del sistema

**Síntoma:** el JS tenía un módulo de diagnóstico y el dueño podía preguntar
«¿qué se rompió?» por WhatsApp. En el port, un fallo solo aparecía en el log del
servidor: desde el móvil, invisible.

**Arreglo:** `internal/xlog` gana un anillo de 64 avisos/errores alimentado por
`Warn` y `Err` (sin tocar ni un `c.Log.Warn` del resto del código).
`ContarErrores()` es el total desde el arranque; `.health clear` lo vacía (solo
el dueño). Los mensajes se recortan a 300 caracteres y el anillo no crece: si
algo falla en bucle no se come la RAM.

**Pruebas:** `internal/xlog/errores_test.go` (5).

---

## 47. `.health` contaba usuarios cargando la base entera

**Síntoma (JS):** `db.getUser().length` y `db.getChat().length`: para saber
cuánta gente hay se cargaba todo el JSON en memoria.

**Arreglo:** `store.Estadisticas()` son tres `COUNT(*)` y un `stat`. El tamaño
incluye `-wal` y `-shm`, que es justo lo que se olvida al mirar el disco (el WAL
puede ser más grande que la base).

**Pruebas:** `TestEstadisticasCuenta`.

---

## 48. `.ex` no existe y no se finge que existe

El `.ex`/`.e` del JS ejecutaba JavaScript dentro del bot. En un binario Go no hay
equivalente, y meter un intérprete de JS solo para eso sería peor que no
tenerlo. El caso de uso real («probar algo rápido en el servidor») lo cubre
`.shell`, que viene **apagado** y se enciende con `OWNER_SHELL=1`.

El `.update` del JS recargaba los módulos con `import(?update=…)`; aquí el flujo
es `.update` → recompilar → `.restart`, y el mensaje lo dice en vez de aparentar
una recarga que no ocurrió.

---

# Cambios 0.12.0 — bloque B5.1 (utilidades sin API ajena)

_Port de `cmds/utils/` (primera tanda) · 8-nov-2026_
_`go build`, `go vet`, `gofmt -l .` limpios; `go test ./...` → 347 pruebas
(una se salta sola si no hay `ffmpeg` en la máquina)._

---

## 49. El QR dependía de una API que puede no responder

**Síntoma (JS):** `.qr` montaba la URL de `api.qrserver.com` y mandaba la imagen
que devolviera. Sin internet para ese dominio —o con el servicio caído— el
comando no daba ni un QR ni un error claro: un mensaje con el enlace roto.

**Arreglo:** el QR se calcula y se dibuja dentro del binario con `rsc.io/qr`
(BSD-3), PNG de 640 px en grises y **4 módulos de zona de silencio**. Tope de
1200 bytes de contenido: más que eso y WhatsApp reescala la imagen hasta que
ningún lector la reconoce.

**Pruebas:** `TestImagenQR*` (firma y tamaño de PNG, silencio, rechazos).

---

## 50. Los recordatorios se perdían en cada reinicio

**Síntoma (JS):** `global.ginkoRecordatorios` con `setTimeout`. Un reinicio, un
`reload` del plugin o un crash y el aviso **desaparecía sin decir nada**: quien
lo programó se enteraba al no recibirlo.

**Arreglo:** tabla `recordatorios` (**migración 14**) y un vigilante único que
revisa cada 20 s. `RecordatoriosVencidos` **selecciona y borra en la misma
transacción**, así que un aviso que ya salió no puede salir dos veces aunque el
proceso muera en medio. Los avisos van con prioridad alta (llegan tarde por
definición) y hay limpieza de vencidos de más de 7 días.

**Detalle que faltaba:** el JS ofrecía un botón «Mis recordatorios» pero **no
tenía comando de borrado ni de lista**. Aquí existe `.recordar lista` y
`.olvidar <número>` (solo el dueño del aviso: el `DELETE` filtra por usuario).

**Pruebas:** `internal/store/recordatorios_test.go` (6) — vence una sola vez,
un futuro no vence, solo el dueño borra, orden por cuándo, rechaza vacíos,
limpieza.

---

## 51. Las encuestas no podían ser quiz (y casi nadie se enteró)

**Lo que se suponía:** que whatsmeow no permite quiz nativo porque
`BuildPollCreation` no expone la respuesta correcta.

**Lo que hay:** el proto `waE2E.PollCreationMessage` **sí** trae `PollType` y
`CorrectAnswer`. Lo que no los expone es el helper. Como `SendProto` (ya en
`internal/wa`) manda un `waE2E.Message` cualquiera por la cola anti-ban, el quiz
se arma a mano: `PollType_QUIZ` y `CorrectAnswer` apuntando **a la misma
estructura de opción** que va en la lista (copiarla no sirve: WhatsApp la
rechaza).

**El `MessageSecret` de 32 bytes es obligatorio.** Sin él la encuesta se ve, la
gente vota y **los votos no se pueden descifrar**: nadie se entera, porque no hay
error. Se genera con `crypto/rand` y hay dos pruebas encima.

**Pruebas:** `cmds/utilidades_test.go` — normal, quiz (con el puntero
compartido), `multi`, y 20 encuestas seguidas con secretos distintos.

---

## 52. `.get` podía pedir la red interna del servidor

**Síntoma:** un comando que hace `fetch(url)` con la URL que llega por WhatsApp
es un SSRF con ruedas: `http://127.0.0.1:8080`, `http://192.168.1.1`, o
`http://169.254.169.254` (metadatos de la nube, que en AWS regalan credenciales).
Y el truco de siempre: un dominio público que **resuelve** a una IP privada, o
un `302` desde un dominio legítimo hacia `127.0.0.1`.

**Arreglo:** tres capas. `.get` es **`OwnerOnly`**; `ValidarURL` con
`ResolverDNSIPs` bloquea loopback, privadas, enlace local, CGNAT `100.64/10`,
multicast, credenciales dentro de la URL y **dominios que resuelven a esas IPs**;
y el cliente HTTP **no sigue redirecciones** (`ErrUseLastResponse`). Más topes:
5 MB, 20 s, texto recortado a 3000 caracteres.

**Pruebas:** `TestValidarURL*` con un resolutor falso (nunca se consulta DNS real
en las pruebas).

---

## 53. La foto de perfil no se podía bajar como las demás

**Síntoma:** `whatsmeow.Client.Download(ctx, info)` donde `info` es
`*types.ProfilePictureInfo` **no compila**: `missing method GetDirectPath`. Ese
`Download` solo acepta un `DownloadableMessage` (imagen, vídeo, audio,
documento).

**Arreglo:** `FotoPerfil` pide la info con `existing=true` (así nunca se provoca
una petición al contacto) y baja el binario con `http.Get` sobre `info.URL`, con
tope de 8 MB y plazo de 20 s.

---

## 54. `VerifiedName` no siempre se puede leer

**Síntoma:** `info.VerifiedName != nil` no implica que haya nombre: el nombre
vive en `VerifiedName.Details.VerifiedName`, y el certificado puede venir sin
detalles. Acceder a `Details` sin comprobarlo es un `nil panic` en producción,
en el único comando que la mayoría de la gente usa para «espiar» un número.

**Arreglo:** `nombreVerificado()` comprueba las dos capas y devuelve cadena
vacía si no hay nada.

---

## 55. `.say` no podía firmar como el bot

**Síntoma (JS):** `.say` reenviaba el texto tal cual, así que cualquiera podía
escribir formato (`*texto*`, enlaces, menciones) y hacerlo pasar por un mensaje
del bot.

**Arreglo:** el texto pasa por `bot.Esc` y se recorta a 2000 caracteres. Sin
texto, repite el mensaje citado; sin cita y sin texto, explica el formato.
