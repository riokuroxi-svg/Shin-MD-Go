# Seguridad: qué puede hacer un usuario y qué le impide el bot

_Revisión del núcleo · 5-nov-2026 · versión 0.4.0_

Un bot de WhatsApp es, a efectos prácticos, **un proceso que ejecuta lo que le
llega de internet y responde gastando tu número**. Este documento lista los
riesgos reales (no los teóricos), qué se hizo en el código y qué queda abierto
porque la plataforma no lo permite cerrar.

---

## 1. Inyección de formato en los mensajes

**El problema.** WhatsApp interpreta `*negrita*`, `_cursiva_`, `` `código` `` y
`~~~tachado~~~`. El nombre de perfil (`PushName`) es texto libre: cualquiera
puede llamarse `*Owner*_oficial_` y, si el bot lo copia tal cual en un aviso,
el mensaje **del bot** parecerá decir otra cosa.

**Qué se hizo.**

| Dónde | Qué se aplica |
|---|---|
| Nombres de usuario en avisos | `bot.CleanName` — quita saltos, controles e invisibles, recorta y escapa el formato |
| Texto que el usuario escribe y el bot repite (motivo de `.afk`, `.report`) | `bot.ClipText` — limpia, recorta a 140/500 caracteres y escapa |
| Nombre de categoría en `.menu <x>` | `bot.CleanName` antes de repetirlo |
| Texto analizado para detectar enlaces | `bot.CleanInvisible` — quita invisibles pero **no** escapa (si escapara, `h_t_t_p_s` no se detectaría) |
| Dibujo del `.fax` | `ui.sanitizeText` — quita controles e invisibles, incluido **U+202E** (invierte el orden del texto dibujado) |

**Pruebas:** `TestCleanNameStripsFormatAndInvisibles`,
`TestNameInjectionIsNeutralized`, `TestAFKReasonCannotForgeMessages`,
`TestSanitizeStripsFormatOverride`.

---

## 2. Enlaces disfrazados en el antilink

**El problema.** Se puede escribir un dominio con caracteres invisibles
(`https://ejemplo.com`) o usar un acortador para saltarse una lista negra. Y al
revés: una detección demasiado agresiva borra mensajes legítimos ("informe.final",
"v1.2", un correo electrónico).

**Qué se hizo.** `bot/links.go` normaliza (quita invisibles, espacios duros,
controles) y detecta por capas: esquemas, dominios de invitación conocidos y
dominios con extensión real de 2+ caracteres antes del punto. Los **correos se
descartan antes de analizar**. Probado con 22 casos, positivos y negativos.

**Lo que no se puede cerrar:** un acortador nuevo o un dominio recién registrado
puede pasar. El antilink reduce ruido, no es un antivirus.

---

## 3. Amplificación y spam saliente (riesgo de ban)

**El problema.** El recurso escaso es tu número. Un usuario que escribe 50
enlaces seguidos hacía que el bot respondiera 50 veces: eso es exactamente lo
que WhatsApp marca como spam.

**Qué se hizo.**

| Capa | Límite |
|---|---|
| Comandos | 10 por minuto y usuario (`router.rateLimited`) + cooldown por comando |
| Avisos de antilink | 3 por minuto y usuario; después borra en silencio y avisa una sola vez cada 5 min |
| Avisos de afk | 1 cada 10 s por usuario, y máximo 5 mencionados por mensaje |
| `.report` | 2 cada 10 minutos por usuario |
| Envíos totales | cola anti-ban: un envío a la vez, jitter y cupo diario de calentamiento |
| Texto de usuario reenviado | tope de 1000 caracteres (`MaxUserText`) y `ClipText` |
| Respuestas largas | `bot.Chunk` parte en trozos de 3500 caracteres |

Los dos guardas (`RateGuard`) tienen techo de 2048 entradas y poda: un bot en
grupos grandes no acumula estado infinito.

**Pruebas:** `TestRateGuardAllowsUntilLimit`, `TestRateGuardWindowExpires`,
`TestRateGuardBounded`, `TestChunkProtectsBigOutput`.

---

## 4. Coste de CPU por mensaje (DoS de bajo esfuerzo)

**El problema.** El `.fax` dibuja una imagen en Go. Es lo único del bot que hace
trabajo de CPU proporcional a la entrada, y lo dispara cualquiera.

**Qué se hizo.** Topes duros en `ui/fax.go`: 40 líneas, 40 columnas, 1600×3000
px, escala 2–6, PNG de 4 MB como máximo, `png.BestSpeed`. El texto se recorta
**antes** de reservar memoria, así que la memoria no depende de lo que mande el
usuario. Probado con 2000 líneas de basura: termina muy por debajo de 2 s.

**Prueba:** `TestRenderFaxIsBounded`, `TestRenderFaxLimitsHugeInput`.

---

## 5. Inyección en la base de datos

**El problema.** Los SQL usan parámetros (`?`), pero había dos sitios donde el
dato del usuario entraba en **estructuras**, no en valores:

1. `settings`: la clave iba dentro de una ruta JSON de SQLite (`'$.' || clave`).
   Una clave con comillas o puntos podía reescribir el JSON entero del usuario.
2. Declaraciones de tabla con nombres dinámicos — se eliminaron; todo es
   estático.

**Qué se hizo.** `store.validKey` solo admite `[A-Za-z0-9_-]` (máx. 48), y
`GetSetting` lee con `json_extract(json, ?)` parametrizado. Probado con
`a.b`, `a'"`, `x'; DROP TABLE settings; --`, vacío y con espacios.

**Prueba:** `TestSettingsRejectBadKeys`.

---

## 6. Nombres de comando

El id de un botón es un dato del mensaje y puede venir manipulado. El router
solo acepta nombres con `[A-Za-z0-9_-]` (máx. 32) antes de buscar en el
registro (`bot.IsSafeCommandName`) — así un "comando" no puede colarse como
ruta, como clave de la base ni como parte de una URL.

**Prueba:** `TestIsSafeCommandName`.

---

## 7. Datos en disco

| Archivo | Permisos | Contenido |
|---|---|---|
| `data/auth.db` (+ `-wal`/`-shm`) | `600` | **llaves de la sesión de WhatsApp**: con esto se suplanta el número |
| `data/shin.db` | `600` | economía, settings, identidades |
| carpeta `data/` | `700` | — |

Añadido en 0.2.0: `secureSessionFiles` fuerza `600` en la sesión, que antes se
creaba legible por otros usuarios del servidor.

**Lo que queda abierto:** si alguien tiene acceso de lectura al usuario del
sistema, tiene la sesión. No hay cifrado del `auth.db` (whatsmeow no lo ofrece);
la única defensa real es un usuario de sistema dedicado y un disco con permisos.

---

## 8. Fuga de información en las respuestas

- `.lid` muestra **tu propia** identidad, no la de otros.
- `.info` oculta al no-owner los datos internos (usuarios registrados,
  identidades conocidas); el owner los ve porque le sirven para operar.
- Los errores de un handler se registran en el log y **no** se le mandan al
  usuario (un `err.Error()` puede revelar rutas o esquemas internos).

---

## 9. Lo que este bot NO protege (dicho claro)

| Riesgo | Por qué no se puede cerrar aquí |
|---|---|
| Ban del número | ningún bot es inmune; el antilink y la cola reducen el riesgo, no lo eliminan |
| Suplantación de un miembro | WhatsApp no verifica identidad ante el bot; se puede comparar por LID/teléfono, no por "quién es de verdad" |
| Fotos, vídeos y documentos de los usuarios | el bot no los procesa en este bloque (B5 los abre: ahí hará falta validar tipo y tamaño de verdad, no la extensión) |
| `ffmpeg` y binarios externos | se llamarán por `os/exec` con argumentos controlados, nunca con una cadena de shell armada desde el chat |
| Extracción de comandos por fuerza bruta | el cooldown y el cupo diario lo hacen lento, no imposible |

---

## 10. Cómo revisar esto tú mismo

```bash
go test ./... -run 'Guard|Clean|Chunk|Injection|Sanitize|Settings|SafeCommand'
grep -rn "os/exec\|exec.Command" .            # todavía no hay ninguno
grep -rn "Sprintf(.*SELECT\|Query(.*+ " .     # consultas armadas a mano
```

## Gacha (0.5.0)

- **Ninguna clave de API en el código.** El bot de JS llevaba una clave de
  gelbooru escrita dentro de `rollwaifu.js`; una clave en un repositorio público
  está quemada. Aquí se consulta un tablero que no pide clave.
- **Lo que llega de fuera se trata como no confiable.** La imagen se descarga con
  8 s de espera, tope de 4 MB, comprobación de `Content-Type` (`image/*`) y
  extensión `.jpg/.png`; los componentes de la URL se limpian (`..` se descarta)
  antes de pegarse. Probado en `internal/gacha/imagen_test.go`.
- **Un catálogo ausente no rompe el bot**: se avisa una vez y los comandos
  responden que está vacío.
- **Reclamos**: un solo dueño por personaje y chat (clave primaria) y reclamo
  condicional (`claimed_by IS NULL AND expires_at > ?`), así que no se puede
  reclamar dos veces ni quedarse con lo caducado.

Verificación rápida:

```bash
go test ./internal/gacha -v
grep -rn "api_key\|apikey" .   # no debe salir nada
```

## Grupos y administración (0.6.0)

- **Las operaciones destructivas no se automatizan.** `.kick all`, `.kick num` y
  `.kick inactive` listan, no expulsan (docs/GRUPOS.md §3). El bucle de
  expulsiones con pausa existe en `wa.KickLote` pero ningún comando lo llama.
- **Nadie se expulsa solo.** Se bloquea antes de llamar a WhatsApp: creador del
  grupo, administradores, el dueño del bot (`OWNERS`) y el propio bot. Sin
  información del grupo, no se actúa.
- **Menciones acotadas.** `tagall`/`hidetag` mandan como mucho 50 menciones
  (`wa.MaxMentions`) y las plantillas de bienvenida se recortan a 500/900
  caracteres: un texto enorme no se convierte en un mensaje enorme.
- **`antistatus` no acusa sin prueba.** Si el borrado falla, no se pone la
  advertencia (`cmds/grupos_avisos.go`).
- **Los borrados pasan por la cola anti-ban.** Antes `DeleteMessage` enviaba por
  fuera de la cola: justo en el caso de borrados en ráfaga, que es donde más se
  nota un patrón de bot.
- **`.bot off`** silencia el bot en ese grupo (comandos, hooks y contador), y el
  dueño siempre puede volver a encenderlo.

Verificación rápida:

```bash
go test ./internal/store -run 'Warn|Actividad|Ajustes' -v
go test ./cmds -run 'Grupo|SameUser|Plantilla' -v
grep -rn "KickLote" cmds/      # no debe salir nada: ningún comando lo usa
```

## Juegos de azar y riesgo (0.7.0)

- **El dinero no se puede duplicar apostando.** El cobro es un `UPDATE … WHERE
  coins >= ?` y el pago una suma sobre el saldo actual; no hay ninguna lectura
  seguida de escritura (`docs/JUEGOS.md §1`). 50 apuestas simultáneas de 500
  sobre un saldo de 10,000 cobran 20, ni una más (`TestApuestasSimultaneasNoRompenSaldo`).
- **Una apuesta por persona y juego a la vez.** El enfriamiento se reserva en la
  base con condición dentro del `UPDATE` (`ClaimCooldown`): de 20 intentos
  simultáneos pasa exactamente uno (`TestCooldownDeJuegoEsUnico`).
- **Ninguna pérdida puede dejar saldo negativo.** `Charge()` cobra cartera y
  banco en una sentencia con `MAX(…, 0)` y devuelve lo que de verdad cobró, que
  es lo que se le dice al jugador.
- **Salud, stamina y magia no salen de `0..100`** (`AddStat`).
- **Las probabilidades están escritas y probadas.** La ruleta es 18/18/1 y
  devuelve 97 % (`TestRuletaDevolucion`); la tragaperras devuelve 86 %
  (`TestSlotDevolucion`). Si alguien toca los números, la prueba falla; no se
  puede convertir un juego en una fábrica de dinero sin que salte.
- **Robar es una transacción entre dos personas.** `Steal()` descuenta
  condicionalmente y abona en la misma transacción: con 4 robos a la vez sobre
  10,000 no salen más de 10,000 (`TestStealConcurrenteNoDuplica`). No se puede
  robar a uno mismo ni al bot, y cada víctima tiene una hora de tregua.
- **Nada de partidas pendientes en memoria.** El `.mine` del JS dejaba la
  partida abierta 3 minutos (y se perdía al reiniciar); aquí todo se resuelve en
  el mismo mensaje.

Verificación rápida:

```bash
go test ./internal/store -run 'Charge|AddStat|Steal|Apuestas|Cooldown' -v
go test ./cmds -run 'Ruleta|Slot|PPT|Mina|Juegos' -v
grep -rn "Math.random" .        # no debe salir nada: es Go
```
