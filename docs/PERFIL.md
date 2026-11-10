# Perfil, nivel y matrimonio · bloque B3.1

_Quién es cada persona, cuánta experiencia tiene y con quién se casó ·
10-nov-2026 · versión 0.10.0_

El perfil del JS vivía en `cmds/profile/*.js` (15 archivos) y en dos sitios de la
base a la vez: `g_users` para lo de la persona y `g_chat_users` para lo del chat.
Aquí queda igual de repartido —porque es lo correcto— pero con una diferencia
que se nota al usarlo: **una sola curva de XP**.

Comandos portados, con sus alias del JS:

| Comando | Alias | Qué hace |
|---|---|---|
| `.profile` | `perfil`, `ficha`, `yo` | Ficha completa: nivel, XP, monedas del chat, racha, daily, cumpleaños, pasatiempo, estado civil y comandos usados |
| `.level` | `lvl`, `nivel`, `xp` | Nivel, tramo actual y barra de progreso |
| `.lboard` | `lb`, `leaderboard`, `topnivel` | Los 10 con más experiencia |
| `.setdesc` | `desc`, `setdescription` | Descripción |
| `.setgenre` | `setgenero`, `genre` | Género (decide «casado/casada/casadx») |
| `.sethobby` | `setpasatiempo`, `hobby` | Pasatiempo |
| `.setbirth` | `setcumple`, `birth`, `cumple` | Cumpleaños |
| `.deldesc` | `deldescription` | Borrar la descripción |
| `.delgenre` | `delgenero` | Borrar el género |
| `.delhobby` | `delpasatiempo`, `removehobby` | Borrar el pasatiempo |
| `.delbirth` | `delcumple` | Borrar el cumpleaños |
| `.marry` | `casarse`, `casar`, `casarme` | Proponer matrimonio (y aceptarlo, como en el JS) |
| `.acepto` | — | Aceptar una propuesta pendiente |
| `.divorce` | `divorcio`, `divorciarme` | Divorciarse |
| `.afk` | `ausente`, `away` | Modo ausente con motivo (`.afk off` lo quita) |

---

## 1. El bug de origen: había dos curvas de XP

En el JS convivían dos fórmulas para lo mismo:

```js
// src/lib/xp.js — la que usaba .level
const CRECIMIENTO_LEVEL = (Math.PI / Math.E) ** 1.618 * Math.E * 0.75
// cmds/profile/profile.js — la que usaba el perfil
const CRECIMIENTO = (Math.PI / Math.E) ** 1.618 * Math.E * 0.75
```

Idénticas por casualidad —misma expresión, dos nombres— pero **una de ellas ya
se había tocado una vez en el historial**, y el nivel que enseñaba `.level` no
cuadraba con el que enseñaba `.profile`. Además `AddXP` subía de nivel con
`level * 100`, una tercera fórmula escondida en el almacén.

Aquí queda **una sola**: `internal/xp/xp.go`, y el almacén llama a esa misma
función. No hay una segunda copia que pueda desviarse.

```
crecimiento = (π/e)^1.618 · e · 0.75  ≈ 1.5120
Min(n) = round(n^crecimiento · 2) + 1        (para n > 1)
Max(n) = round((n+1)^crecimiento · 2)
```

`Min(n+1) == Max(n) + 1` siempre: los tramos son contiguos y no hay XP que caiga
en un hueco. Hay una prueba por cada borde (`internal/xp/xp_test.go`).

**Los niveles empiezan en 1.** En el JS, una cuenta nueva nacía en 0 y el primer
mensaje la subía a 1 sin explicación; en `.lboard` aparecía gente de nivel 0 que
no había escrito nunca. Aquí `Minimo = 1` y el tramo del nivel 1 absorbe el del
0 (`Min(1) = 1`), así que nadie empieza «en negativo» ni sube dos veces por el
mismo mensaje.

**Tope de 10 000 niveles.** Con XP absurdo (un dato migrado mal, un
`UPDATE` a mano) la fórmula no explota: se recorta y `.level` sigue pintando
algo. Hay prueba con valores corruptos, negativos y enormes.

---

## 2. Qué es de la persona y qué es del chat

Igual que en el JS, y esto no se toca:

| Dato | Dónde vive | Por qué |
|---|---|---|
| Nombre, descripción, género, pasatiempo, cumpleaños | `users` (global) | Es la persona, no el grupo |
| Pareja (`.marry`) | `users` (global) | En el JS, casarte en un grupo te casaba en todos |
| XP y nivel | `users` (global) | El nivel te sigue a todos lados |
| Monedas, banco, salud, stamina, inventario | `chat_users` (por chat) | Cada grupo tiene su economía |
| Rachas de `.daily`/`.weekly`/`.monthly` | `users` (global) | Una racha no se rompe por cambiar de grupo |
| AFK | `settings` (global) | Estás ausente de todo el bot, no de un grupo |

Y una regla que en el JS no existía: **consultar el perfil de otra persona no le
crea la cuenta**. `PerfilDe` hace un upsert perezoso (necesario para uno mismo:
la primera vez que escribes hay que tener fila), pero `.profile @alguien` usa
`PerfilExistente`, que solo lee. Sin eso, un grupo donde se menciona mucho
llenaría `users` de cuentas fantasma de gente que nunca usó el bot, y `.info`
las contaría como usuarios.

Las columnas nuevas de `users` entran en la **migración 13** (`internal/store/db.go`).
Una prueba lee `pragma_table_info('users')` y comprueba que están todas: es el
fallo que se cuela cuando alguien añade la migración y no la aplica.

---

## 3. La boda: ahora hace falta que digan que sí

En el JS, `.marry @alguien` casaba a las dos personas **en el acto**. El
resultado se vio en producción: gente casada sin haber escrito nada, y para
deshacerlo había que pedirle al otro un `.divorce`.

Aquí hay propuesta y aceptación:

```
.marry @alguien        → propuesta (caduca a los 10 minutos)
.acepto                → acepta la última propuesta que te llegó
.marry @quien_propuso  → también la acepta (así se confirma en el JS)
```

Detalles que importan:

- **Las propuestas viven en memoria, con mutex.** Los handlers corren en
  goroutines distintas y un `map` escribiéndose desde dos a la vez en Go no da
  un error: da un pánico que tumba el proceso. Está probado con `-race`.
- **Caducan**, y se limpian al pasar: en un grupo donde la gente se propone
  matrimonio todo el día, un mapa que solo crece es una fuga de memoria.
- **Una propuesta se gasta una sola vez**: dos `.acepto` seguidos no pueden
  casar a nadie dos veces (prueba incluida).
- **El matrimonio es una transacción** en la base: o quedan casados los dos, o
  no queda ninguno. En el JS eran dos escrituras seguidas y un corte entre
  ambas dejaba a una persona casada con alguien que no lo sabía.
- Si el bot se reinicia, las propuestas se pierden. Es lo razonable: una
  propuesta de hace tres días ya no significa nada.

---

## 4. El AFK paga, y por eso se calcula con cuidado

`cmds/profile/afktime.js` era un `before`: al volver, el bot **pagaba** por el
tiempo ausente — 8 monedas por minuto y un bono al azar de 300 a 1500 cada 3
horas. Esa parte no existía en el Go y es dinero, así que se porta con pruebas.

La marca de «desde cuándo» se guardaba como texto (`02/01 15:04`), y con texto
no se puede calcular un tiempo. Ahora se guarda el instante en milisegundos y se
formatea al mostrarlo; el aviso de mención sigue enseñando la misma fecha que
antes.

`recompensaAFK(marca, ahora)` entra en `cmds/proteccion.go` con el reloj como
parámetro, para poder probarla sin esperar horas:

- 0 monedas y «desconocido» si la marca no se entiende (base migrada, dato
  viejo, basura): **nunca se paga por un dato que no se pudo leer**;
- 0 monedas si la marca está en el futuro (reloj movido, base de otra máquina):
  no se paga un tiempo negativo;
- el tope de bonos se comprueba en las pruebas con 3, 9 y 48 horas ausente.

El pago va a la cartera de **ese chat**, con el mismo `AddCoins` de la economía;
si la base falla, el aviso de vuelta sale igual y solo se pierde la recompensa
(queda en el log).

---

## 5. Lo que el texto libre no puede romper

El nombre, la descripción, el motivo del AFK y el género los escribe el usuario,
y el bot los repite después a terceros. En el JS eso permitía falsificar un
aviso del bot con `*` y `_`. Aquí todo pasa por `bot.Esc` / `bot.CleanName`
antes de salir:

- los saltos de línea se aplanan (un campo del perfil es **una** línea);
- los caracteres de formato (`*`, `_`, `` ` ``, `~`) se cambian por sus gemelos
  tipográficos (`∗`, `‗`, `´`, `∼`), que WhatsApp no interpreta;
- los invisibles (U+200B y compañía) se quitan: servían para colar un nombre
  distinto del que se ve.

La tabla de niveles lo tiene probado: un nombre como `a_b*c[d]` sale escapado y
no rompe el marco del mensaje.

---

## 6. Verificación

```
go build ./... && go vet ./... && gofmt -l .     → limpio
go test ./...                                    → 290 pruebas
go test -race -count=1 ./cmds ./internal/store ./internal/xp → verde
```

Lo que se prueba **sin abrir WhatsApp**:

| Archivo | Qué cubre |
|---|---|
| `internal/xp/xp_test.go` | tramos contiguos, curva creciente, bordes de `NivelPara`, datos corruptos, porcentaje, barra |
| `internal/store/perfil_test.go` | campos y topes, lista blanca, matrimonio y divorcio, ranking y puesto, AFK, columnas reales del esquema, consultar a otro sin crearle cuenta |
| `cmds/perfil_test.go` | formato de la tabla, escape de nombres, puesto, estado civil, JID corto, propuestas (un solo uso, caducidad, reemplazo), registro de los 14 comandos, que `.afk` no esté duplicado |
| `cmds/proteccion_test.go` | recompensa del AFK por tramos, marca ilegible, reloj hacia atrás, formato de fecha |

Los comandos que hablan con WhatsApp (`.marry` contra un contacto real,
`.lboard` en un grupo con gente) se prueban a mano antes de publicar, como el
resto de bloques.

---

## 7. Lo que falta de este bloque

- **`.afktime`** como comando suelto no se porta: en el JS no era un comando,
  era el gancho que pagaba al volver. Eso ya está en el hook.
- Las **imágenes de perfil** (`.profile` con foto dibujada) no se portan en este
  bloque; el resto del bot las usa donde aportan y aquí serían una imagen por
  cada consulta.
- El **top por monedas por chat** ya existe en la economía (`.economyboard`);
  este bloque añade el top por XP, que es global.
