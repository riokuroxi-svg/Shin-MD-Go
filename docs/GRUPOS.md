# Grupos y administración · bloque B4

_Cómo administra el bot un grupo · 5-nov-2026 · versión 0.6.0_

---

## 1. Comandos

| Comando | Alias | Qué hace | Quién |
|---|---|---|---|
| `.warn @alguien [razón]` | `advertir` | apunta una advertencia; si el grupo tiene límite, al llegar **expulsa** | admin |
| `.warns @alguien` | `advertencias` | lista las advertencias, la más nueva primero | admin |
| `.delwarn @alguien <n\|all>` | `quitarwarn` | borra la número *n* (1 = la más nueva) o todas | admin |
| `.setwarnlimit <1-10\|0>` | `warnlimit` | límite antes de expulsar; `0` desactiva | admin |
| `.kick @alguien` | `ban`, `expulsar` | expulsa a **una** persona | admin + bot admin |
| `.kick all\|inactive\|num +52` | | **solo lista** a quién afectaría (ver §3) | admin + bot admin |
| `.promote @alguien` | `promover` | lo hace admin | admin + bot admin |
| `.demote @alguien` | `degradar` | le quita el admin | admin + bot admin |
| `.open` / `.close` | `abrir` / `cerrar` | todos escriben / solo admins | admin + bot admin |
| `.link` / `.revoke` | `enlace` / `restablecer` | ver el enlace / restablecerlo | admin + bot admin |
| `.tagall [texto]` | | menciona a todos (lista los números) | admin |
| `.hidetag [texto]` | `tag` | menciona a todos **sin** listar los números | admin |
| `.gp` | `groupinfo` | nombre, miembros, admins, creador, modo, límite | cualquiera |
| `.setgpname <texto>` | | cambia el nombre | admin + bot admin |
| `.setgpdesc <texto>` | | cambia la descripción | admin + bot admin |
| `.setgpbanner` | `fotogrupo` | pone como foto la imagen que **respondes** | admin + bot admin |
| `.count [@alguien]` | `mensajes` | cuántos mensajes lleva (total y últimas 24 h) | cualquiera |
| `.topcount` | `topmensajes` | ranking de mensajes de los últimos 30 días | cualquiera |
| `.topinactive` | `inactivos` | quiénes no han escrito en 30 días | cualquiera |
| `.bot on\|off` | | apaga o enciende el bot en ese grupo | admin |
| `.antistatus on\|off` | `antiestados` | borra los estados que se comparten en el grupo | admin |
| `.welcome on\|off` | `bienvenida` | activa el aviso de entrada | admin |
| `.goodbye on\|off` | `despedida` | activa el aviso de salida | admin |
| `.setwelcome <texto>` | | texto de bienvenida (`reset` = el de fábrica) | admin |
| `.setgoodbye <texto>` | | texto de despedida (`reset` = el de fábrica) | admin |

**Marcadores** de los textos de bienvenida y despedida: `@user` (menciona a quien
entra o sale), `#grupo` / `#nombre` (nombre del grupo) y `#miembros`. El texto se
guarda con tope de **500 caracteres** y el mensaje final se recorta a 900: una
plantilla enorme no puede convertirse en 40 KB cada vez que entra alguien.

Ejemplo:

```
.setwelcome 👋 Hola @user, te damos la bienvenida a *#grupo*. Ya somos #miembros.
.welcome on
```

---

## 2. Lo que el bot nunca hace

Estas protecciones no estaban en el JS y son la diferencia entre "un bot de
administración" y "un bot que puede romper tu grupo":

- **No se expulsa ni se degrada a sí mismo.** Si el bot se quita el admin, se
  queda sin permisos para todo lo demás y hay que arreglarlo desde el teléfono.
- **No se expulsa al creador del grupo** ni se le quita el admin.
- **No se expulsa al dueño del bot** (el de `OWNERS`).
- **No se expulsa a un administrador.** Primero se le degrada, y eso es una
  decisión aparte y explícita.
- **Sin info del grupo, no se toca a nadie.** Si WhatsApp no responde con los
  participantes, el comando se corta con un aviso en vez de actuar a ciegas.

Todo eso se comprueba **antes** de llamar a WhatsApp, y la comparación de
identidades es por JID, teléfono y LID a la vez (el mismo usuario puede llegar
como `521…@s.whatsapp.net` o como `123…@lid`).

---

## 3. Las expulsiones en lote: no se portaron (a propósito)

El bot de JS tenía tres comandos que expulsaban **decenas de personas de golpe**:

```js
// cmds/group/kick.js del bot de JS
if (args[0] === 'all') { /* expulsa a TODOS los que no sean admin */ }
if (args[0] === 'num') { /* expulsa a todos con un prefijo de país */ }
if (args[0] === 'inactive') { /* expulsa a los que no escribieron en 30 días */ }
// ...con await new Promise(r => setTimeout(r, 3000)) entre cada uno
```

Aquí **esos modos solo listan**. El motivo es doble:

1. **Riesgo de ban.** WhatsApp trata una ráfaga de expulsiones como abuso. Un
   `.kick all` mal escrito (o escrito por alguien que consiguió ser admin) vacía
   el grupo en minutos y es la vía más rápida a que limiten el número.
2. **No hay vuelta atrás.** Un enlace de grupo no devuelve a la gente: los que
   se fueron no saben por qué y muchos no vuelven.

Así que `.kick all`, `.kick inactive` y `.kick num +52` responden con **la lista
de a quién afectaría** y un aviso de que no han expulsado a nadie. Expulsar es
siempre uno a uno, con nombre y apellido: `.kick @persona`.

Si algún día quieres el modo automático, se puede añadir con una confirmación
explícita (`.kick all confirmar`) — pero no va por defecto.

---

## 4. Advertencias

Se guardan **como filas** (`group_warns`), no como un array dentro del usuario.

| Antes (JS) | Ahora |
|---|---|
| `chat_users.warnings = [{reason, timestamp, by}, …]` | una fila por advertencia |
| Borrar la #3 = leer el array entero, `splice`, reescribirlo | `DELETE` de una fila |
| Contar = traer todas | `COUNT(*)` |
| Dos `.warn` a la vez = una se pierde | cada una es un `INSERT` |

Cada advertencia guarda **razón** (recortada a 300 caracteres), **quién la puso**
y **cuándo**. Si alguien acumula más de 100, se van tirando las más viejas: sin
tope, un grupo que no expulsa acumula advertencias para siempre.

Con `.setwarnlimit 3` (por ejemplo), a la tercera advertencia el bot expulsa y
limpia el contador. Con `.setwarnlimit 0` no expulsa nunca: solo cuenta.

---

## 5. Actividad

Cada mensaje de grupo suma uno en `chat_activity(chat_id, user_id, día)` con un
UPSERT atómico. De ahí salen `.count`, `.topcount` y `.topinactive`.

En el JS esto era `chat_users.stats = {fecha: {msgs: n}}`: en **cada mensaje** se
leía el objeto entero y se volvía a escribir. Un año de actividad en un grupo
activo son cientos de claves releídas y reescritas miles de veces al día, y con
dos mensajes a la vez se perdía la cuenta.

`.topinactive` cruza los participantes reales del grupo (de WhatsApp) con la
tabla de actividad: quien no aparece en 30 días entra en la lista.

---

## 6. antistatus

Cuando alguien comparte un **estado** en el grupo, el bot lo borra y le apunta
una advertencia. Condiciones para actuar (las mismas del JS):

- el grupo tiene `.antistatus on`,
- quien lo comparte **no** es admin,
- el bot **sí** es admin (sin permiso de borrar, intentarlo solo genera errores),
- y el mensaje se identifica con certeza: es un estado de grupo, o cita uno.

Ese último punto es una corrección: el JS borraba a partir de campos que a veces
venían vacíos y acababa borrando el mensaje de **otra** persona. Aquí, si no se
sabe quién lo mandó, no se borra nada.

---

## 7. Bienvenida, despedida y caché de permisos

Los avisos de entrada y salida escuchan los eventos de grupo de WhatsApp
(`events.GroupInfo`), no los mensajes. De ahí salen dos cosas:

1. **Bienvenida y despedida** con su plantilla y sus marcadores. Se mandan de una
   en una por la cola anti-ban, y como mucho cinco por evento: si entran cuarenta
   personas de golpe, el bot no escupe cuarenta mensajes.
2. **La caché de permisos se tira en el momento.** La caché de grupos dura un
   minuto; si alguien asciende a otra persona y le escribe justo después, con la
   caché vieja el bot diría que no es admin. Con el evento, se refresca al
   instante.

---

## 8. `.bot off`

Apaga el bot en ese grupo: deja de responder comandos, de contar mensajes y de
borrar enlaces. El owner sigue teniendo acceso (para poder volver a encenderlo).
El ajuste se guarda en la base y se aplica en cuanto se escribe el comando (la
caché de 30 s se invalida a mano).

---

## 9. Cómo comprobarlo tú

```bash
go test ./internal/store -run 'Warn|Actividad|Ajustes|Top' -v
go test ./cmds -run 'Grupo|SameUser|Plantilla|Mention' -v
```

En un grupo de pruebas:

```
.setwarnlimit 2
.warn @alguien prueba        → 1 advertencia
.warn @alguien otra          → expulsado (era el límite)
.warns @alguien              → vacío tras la expulsión
.welcome on
.setwelcome Hola @user a #grupo
   (que entre alguien)
.bot off                     → el bot deja de responder… salvo al owner
.antistatus on
   (comparte un estado siendo no-admin) → se borra y queda advertido
```

---

<div align="center">

_Shin-MD-Go · AGPL-3.0-only · riokuroxi-svg_

</div>
