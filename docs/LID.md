# LID: por qué en Baileys no salía y cómo se resuelve aquí

_Documento técnico · 5-nov-2026_
_Contrastado con el código de whatsmeow instalado (`v0.0.0-20261013142727`) y con
la documentación pública de Baileys 7 y el fork `@itsliaaa/baileys`._

---

## 1. Qué es un LID (en dos líneas)

WhatsApp dejó de mandar siempre el teléfono. Ahora el mismo usuario puede
aparecer con dos identificadores distintos:

```
5215512345678@s.whatsapp.net   ← PN   (Phone Number, el de siempre)
123456789012345@lid            ← LID  (Linked ID, interno, sin teléfono)
```

No son dos usuarios: es **la misma persona**. Si el bot no lo sabe, la trata
como dos: dos cuentas de monedas, dos perfiles, y el admin del grupo deja de
ser admin porque "no coincide el número".

---

## 2. La limitación que nadie puede saltarse

**La resolución es de un solo sentido: teléfono → LID.**

- Preguntar «¿cuál es el LID de este teléfono?» → sí se puede (`GetUserInfo`,
  `GetLIDForPN`).
- Preguntar «¿cuál es el teléfono de este LID?» → **no existe esa consulta**.
  Solo se puede contestar si esa pareja ya se vio antes en el tráfico
  entrante, y entonces está en la caché local.

Es exactamente lo que documenta la migración de Baileys 7:

> *"Network resolution is one-way: PN → LID. […] The `getPNForLID` lookup only
> succeeds for pairs Baileys has previously cached from inbound traffic — it
> cannot fetch a PN for an unknown LID."*
> — [baileys.wiki/migration/v7](https://baileys.wiki/migration/v7)

Traducción a la práctica: **el 90 % del trabajo no es "resolver", es
"acordarse"**. Quien no guarda las parejas que ve, no las puede resolver nunca.

---

## 3. Por qué en tu bot de JS "no sirve" aunque programes la función

Programar `findUserId()` no arregla nada si la base no guarda las llaves donde
se persisten las parejas. El fork `@itsliaaa/baileys` sí trae el helper:

```js
const ids = await sock.findUserId('6281111111111@s.whatsapp.net')
// { phoneNumber: '6281111111111@s.whatsapp.net', lid: '43411111111111@lid' }
```

…pero eso se apoya en el `auth state` del fork, y ahí están los tres requisitos
que rompen la fiesta cuando la base es vieja:

1. **`lid-mapping`** en el `SignalDataTypeMap` — sin esta clave no se guarda
   ninguna pareja, y `findUserId` devuelve `lid: undefined` para siempre.
2. **`device-list` y `tctoken`** — necesarias para que la sesión Signal se cree
   ya en formato LID (Baileys 7 crea **todas** las sesiones nuevas así).
3. **`lidDbMigrated` / `LIDMigrationTimestamp`** — la marca que le dice a
   WhatsApp "puedes mandarme las parejas". Sin ella, el servidor no las manda y
   la caché se queda vacía.

Un fork desactualizado puede tener el método, pero no las llaves: el método
existe, se ejecuta sin error… y devuelve `undefined`. Y de ahí viene el
diagnóstico que te salió: *"no sirve por el Baileys desactualizado"*. Es
literal — no es que tu código esté mal, es que **la base no tiene dónde
guardar la respuesta**.

> El otro fork que investigué (`toxic-baileys`) lo dice en su propia tabla de
> features: *"LID Mapping System: full LIDMappingStore with LRU cache,
> persistent key store, bidirectional lookups"* y *"lidDbMigrated login flag:
> signals WhatsApp to push down LID mappings on connect"*. Es decir: el trabajo
> real es el store + la bandera de migración, no el helper.

---

## 4. Lo que whatsmeow ya trae (esto es la buena noticia)

Revisando la librería que usa este repo, el soporte **no hay que inventarlo**:

| Pieza | Dónde vive en whatsmeow | Para qué sirve aquí |
|---|---|---|
| Store de parejas | `store.LIDStore` (`PutLIDMapping`, `GetPNForLID`, `GetLIDForPN`, `GetManyLIDsForPNs`) | guardar y consultar LID ↔ teléfono |
| LID del propio bot | `Device.LID` | saber cómo aparece el bot en un grupo |
| Marca de migración | `Device.LIDMigrationTimestamp` | que el servidor mande las parejas |
| Alternativo en cada mensaje | `events.Message.Info.SenderAlt` / `RecipientAlt` | la pareja **gratis**, sin consultar nada |
| LID por teléfono (red) | `types.UserInfo.LID` (`GetUserInfo`) | el único sentido que la red resuelve |
| Participantes de grupo | `types.GroupParticipant{JID, PhoneNumber, LID}` | permisos de admin correctos |
| Dueño del grupo | `types.GroupInfo.OwnerJID` / `OwnerPN` | dueño por las dos vías |
| Aprendizaje automático | historial (`PhoneNumberToLidMappings`) y sync de migración (`LidMigrationMappingSyncMessage`) | la librería sola va llenando el store |

Y lo más importante: cuando llega un mensaje con `SenderAlt`, whatsmeow **ya
guarda la pareja** (`cli.StoreLIDPNMapping`). O sea, el "acordarse" ya está
resuelto por el protocolo… mientras el store exista y sobreviva.

---

## 5. Lo que añade este repo

Tres piezas, en este orden:

### 5.1 `internal/wa/lidstore.go` — store espejo

Envuelve el `store.LIDStore` de la librería e implementa su misma interfaz:

```
whatsmeow aprende una pareja
        ↓
lidStore.PutLIDMapping / PutManyLIDMappings
        ↓                        ↘
base del bot (SQLite)            store original de la librería
  tabla `identities`               (sesión)
        ↓
si al reconectar la librería no la tiene, GetPNForLID la saca de la base
```

Resultado: **las parejas no se pierden al cambiar de sesión, de teléfono o de
dispositivo**. La librería reconstruye su tabla al conectar; el bot ya tiene la
suya desde el primer segundo.

### 5.2 `internal/wa/identity.go` — Resolver

Una sola pregunta para todo el bot: *¿quién es este JID?*

```go
id := client.IDs.Resolve(ctx, sender, senderAlt)
id.Key   // llave estable para la base: el TELÉFONO si se conoce
id.PN    // teléfono (vacío si aún no se sabe)
id.LID   // identificador interno (vacío si WhatsApp no lo dio)
id.Known() // ¿ya tenemos los dos lados?
```

Reglas:

1. **La llave es el teléfono siempre que se conozca.** Los datos del bot de JS
   se guardaron así y siguen cuadrando.
2. **La pareja se aprende gratis** desde `SenderAlt` (lo manda WhatsApp en la
   mayoría de mensajes).
3. Si viene solo como LID: se busca en el store de la librería y luego en la
   base del bot. **Nunca se consulta la red**: esa consulta no existe.
4. Si aun así no aparece, **la llave es el LID**. Se prefiere una llave fea a
   perder el mensaje, y en cuanto se aprenda la pareja las cuentas se fusionan.

### 5.3 Fusión de cuentas (lo que evita perder monedas)

Si alguien escribió cuando solo se conocía su LID, su economía quedó en una
fila `…@lid`. Al aprender el teléfono, `store.PutIdentity` **mueve** esa fila a
la llave buena; y si ya existían las dos, **suma** monedas, banco y XP y
traspasa la marca del `.daily` (para que no lo reclame dos veces).

Está probado en `TestIdentityMergesAccounts` y `TestIdentitySumsBothAccounts`.

---

## 6. Permisos de grupo: el bug que nadie ve hasta que alguien no puede usar `.kick`

Los participantes del grupo pueden venir con LID y el mensaje con teléfono (o
al revés). Comparar cadenas a pelo da dos fallos simétricos:

- el admin real **no** es reconocido como admin,
- o peor: alguien que no lo es **sí** lo es, si hereda la cadena de otro.

`internal/bot/group.go` compara por las cuatro formas:

```
participante.JID | participante.PhoneNumber | participante.LID | identidad resuelta del remitente
```

y `GroupInfo.OwnerPN` cubre al creador. Probado en
`TestParticipantIsAdminAcrossLID`.

---

## 7. Cómo comprobarlo tú, en el teléfono (10 segundos)

```
.lid          → tu identidad: llave, teléfono, LID, si eres admin, si el bot lo es
.perfil       → tus monedas (no se duplican aunque entres por LID)
.en un grupo  → .antilink on   (necesita que el bot sea admin para borrar)
```

`.lid` en un chat privado y `.lid` en un grupo, con el mismo usuario, tienen que
mostrar **la misma llave**. Si en un grupo sale la llave del LID y en privado el
teléfono, es que todavía no se había visto esa pareja: en cuanto llegue un
mensaje con `SenderAlt` se completa sola y se fusionan las cuentas.

---

## 8. Limitaciones, dichas claro

| Caso | Qué pasa | Mitigación |
|---|---|---|
| LID que nunca se vio | la llave es el LID (`…@lid`) | en cuanto aparece en un mensaje con alternativo, se completa y se fusiona |
| El usuario nunca manda nada | no se aprende nada de él | se resuelve al verlo en `GetGroupInfo` (participantes traen `PhoneNumber`) |
| LID alojado (`@hosted.lid`) | tratado como LID normal | `IsLID` cubre los dos servidores |
| Grupo con participantes sin teléfono | imposible saber el PN de terceros | no hace falta: se compara por `LID`, que el grupo sí da |

---

## 9. Resumen en una tabla

| | Fork de Baileys | Shin-MD-Go |
|---|---|---|
| Helper de consulta | `findUserId()` (si la base lo soporta) | `ID.Resolve()` |
| Persistencia de parejas | depende del `auth state` del fork | tabla `identities` propia, sobrevive a todo |
| LID → teléfono | solo lo cacheado | solo lo cacheado **+** base del bot |
| Llave de la economía | a mano en cada comando | una sola (`Context.Key()`) |
| Admin de grupo | comparación que se rompe con LID | 4 vías + dueño (`OwnerPN`) |
| Diagnóstico | ninguno | `.lid` |
| Fusión de cuentas al aprender la pareja | no la hace nadie | automática (`PutIdentity`) |
