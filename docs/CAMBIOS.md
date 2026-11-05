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
