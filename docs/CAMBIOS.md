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
