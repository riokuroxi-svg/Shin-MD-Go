# Economía · bloque B3

_Cómo funciona el dinero en Shin-MD-Go · 5-nov-2026 · versión 0.7.0_

Este documento explica **qué hace cada comando, con qué números y dónde se
guarda**. Los números son los mismos del bot de JS a propósito: la gente ya
conoce cuánto paga un `.daily` y las cuentas migradas tienen que seguir
valiendo lo mismo.

---

## 1. La decisión que ordena todo: el dinero es **por chat**

El bot de JS guarda la economía en `g_chat_users`, con clave
`(chat_id, user_id)`. O sea: tus monedas en un grupo no son las mismas que en
otro. Se copia igual, y no es capricho:

- **Migrar es 1:1.** Cada fila del JS entra en una fila de aquí. Si se hubiera
  inventado una economía global, habría que decidir a qué grupo pertenece cada
  saldo viejo — y no hay forma de saberlo.
- **Un grupo no contamina a otro.** Quien abusa de la economía en un grupo no
  arrastra el saldo al resto.
- **Encaja con los ajustes por chat** que ya existían: apagar la economía en un
  grupo (`.economy off`) no toca a los demás.

Las **rachas sí son globales** (una por persona), también como el JS
(`g_users.streak`). Reclamar el `.daily` en dos grupos el mismo día sube la
racha una vez y paga una vez; el segundo grupo solo cumple su espera de 24 h.

### Tablas

```sql
-- migración 6: economía por chat
chat_users(chat_id, user_id, coins, bank, health, stamina, magic,
           inventory, last_work, last_daily, last_weekly, last_monthly,
           PRIMARY KEY (chat_id, user_id))

-- migración 7: rachas y perfil global (una fila por persona)
users.streak / last_daily_global
users.weekly_streak / last_weekly_global
users.monthly_streak / last_monthly_global

-- migración 8: marca de siembra del saldo antiguo
users.economy_seeded
```

Los enfriamientos se guardan en **milisegundos desde 1970**, igual que el JS
(`lastwork`, `lastdaily`…). Así una base migrada conserva las esperas en curso
en vez de regalar un `.daily` extra el día de la actualización.

---

## 2. Comandos

| Comando | Alias | Qué hace |
|---|---|---|
| `.work` | `w`, `chambear`, `trabajar` | 2,000–4,000 por turno · espera **3 min** |
| `.daily` | `diario`, `recompensa`, `gratis` | 20,000 + 5,000 por día · espera **24 h** |
| `.weekly` | `semanal` | 40,000 + 5,000 por semana · espera **7 días** |
| `.monthly` | `mensual` | 60,000 + 5,000 por mes · espera **30 días** |
| `.balance` | `bal`, `coins`, `bank`, `saldo` | cartera, banco y total |
| `.deposit` | `dep`, `depositar`, `d` | de la cartera al banco (`all` acepta todo) |
| `.withdraw` | `with`, `retirar` | del banco a la cartera (`all` acepta todo) |
| `.pay` | `givecoins`, `transfer`, `dar` | a otra persona, **desde tu banco** |
| `.economyboard` | `eboard`, `baltop`, `ranking`, `top` | ranking del grupo, de 10 en 10 |
| `.shop` | `tienda`, `buy`, `comprar` | lista de precios o comprar |
| `.inventory` | `inv`, `inventario`, `mochila` | objetos, salud, stamina, magia |
| `.economy` | `economia`, `eco` | `on`/`off` en el grupo (solo admins) |
| `.perfil` | `profile`, `yo` | ficha con dinero, nivel y rachas |

### Premios

```
daily(racha)   = min(20,000 + (racha-1)·5,000, 1,015,000)   racha máx 200
weekly(racha)  = min(40,000 + (racha-1)·5,000,   185,000)   racha máx 30
monthly(racha) = min(60,000 + (racha-1)·5,000,    95,000)   racha máx 8
```

La racha se pierde si pasa **1.5 veces** la cadencia sin reclamar (36 h en el
diario, 10.5 días en el semanal, 45 días en el mensual). Si se pierde, el pago
sale como día 1 y el mensaje avisa.

**Paquete de bienvenida:** la primera vez que alguien usa `.daily` en un chat
recibe 15,000 extra, un pico (100/100), 2 pociones y salud/stamina/magia al
100 %. En el JS se detectaba con "no tiene pico"; aquí se mira lo mismo, así
que quien ya lo recibió no lo cobra dos veces.

### Tienda

| Objeto | Precio | Tipo | Usos |
|---|---|---|---|
| ⚔️ Espada | 8,000 | arma | permanente |
| 🪓 Hacha | 7,500 | arma | permanente |
| 🏹 Arco | 7,000 | arma | permanente |
| ⛏️ Pico | 6,500 | herramienta | permanente |
| 🎣 Caña de pescar | 6,000 | herramienta | permanente |
| 🗿 Tótem | 4,000 | consumible | 3 |
| 🧪 Poción | 1,500 | consumible | 1 |

Los precios viven en un solo sitio (`internal/store/catalogo.go`). En el JS
estaban escritos a mano en `shop.js` **y otra vez** en `invoke.js`, y ya se
habían desincronizado.

### Cantidades

`.deposit`, `.withdraw` y `.pay` aceptan `2500`, `2,500`, `2.500`, `25k`,
`1.5k`, `2m`, `¥3000` y `all`/`todo`. Lo que **no** aceptan: negativos, cero,
`2500abc` ni texto. El JS usaba `parseInt`, con lo que `2500abc` valía 2500 y
`abc` era `NaN` (y según dónde cayera, dejaba el saldo en `NaN`).

### Dinero

Se guarda el **nombre** de la moneda (`Yenes`, que es lo que trae el JS) y se
pinta el símbolo: `¥`, `$`, `€`, `R$`, `S/`, `Bs.` — y `¥` para lo que no
reconozca, en vez de romper el mensaje.

---

## 3. Lo que se arregló de raíz (y no es cosmético)

### 3.1 El dinero se duplicaba con dos mensajes a la vez

**En el JS:**

```js
const user = db.getChatUser(msg.chat, msg.sender);  // lee 1,000
db.setChatUser(msg.chat, msg.sender, 'coins', user.coins - 800); // escribe 200
```

Dos `.shop espada` enviados a la vez leen los dos 1,000, y los dos escriben
200: **una espada gratis**. Lo mismo con `.deposit`, `.withdraw`, `.pay` y con
los enfriamientos (`.work` dos veces seguidas pagaba dos veces).

**Aquí** cada movimiento es una sentencia con su condición dentro:

```sql
UPDATE chat_users SET coins = coins - ?
 WHERE chat_id = ? AND user_id = ? AND coins >= ?
```

Si la condición no se cumple, `RowsAffected()` es 0 y el comando responde "no
tienes suficiente". La base de datos decide, no el código de arriba.

Está probado con goroutines de verdad, no de palabra:
`TestSpendConcurrenteNoDuplica` lanza 40 cobros de 100 sobre un saldo de 1,000
y exige que pasen exactamente 10; `TestTransferConcurrente` lanza 8
transferencias con banco para una y exige que el total no cambie.

### 3.2 El saldo podía quedar en negativo

`AddCoins` recorta a `[0, 1 billón]` **en SQL** (`MIN(MAX(...))`) y `Transfer`
exige `bank >= cantidad`. Antes, un `.withdraw` más grande que el banco dejaba
la cuenta en rojo y todo lo demás (rankings, robos, compras) trabajaba con
números imposibles.

### 3.3 Los topes

Sin techo, un `.shop pocion 999999999` o una base migrada con basura rompen la
aritmética. Ahora:

- saldo máximo por cuenta: **1,000,000,000,000**
- transferencia máxima: **1,000,000,000**
- unidades de un objeto: **9,999**
- salud/stamina/magia: **0–100**

Y el inventario se valida contra el catálogo: el JSON de la base **no** puede
llenarse de claves inventadas. Si el JSON está roto (una migración a medias,
una edición a mano), se lee como inventario vacío en vez de tumbar el comando —
probado en `TestInventarioToleraBasura`.

### 3.4 La puerta de la economía

`economyGate` del JS se respeta tal cual: si el chat tiene `adminonly` o la
economía apagada, **todos** los comandos de dinero responden el mismo aviso y
sugieren `.economy on`. Aquí eso son dos ajustes por chat en la tabla
`settings`, sin tabla nueva.

---

## 4. Cómo se migran los datos del bot de JS

El bot de JS puede volcar sus tablas a JSON y este bot los importa. Lo que
importa, campo por campo:

| Origen (JS) | Destino (Go) | Nota |
|---|---|---|
| `g_chat_users.chat_id` | `chat_users.chat_id` | tal cual |
| `g_chat_users.user_id` | `chat_users.user_id` | Pasar por la tabla `identities` si es un LID: la llave es el teléfono cuando se conoce |
| `coins`, `bank` | `coins`, `bank` | recortados a los topes |
| `health`, `stamina`, `magic` | iguales | 0–100 |
| `inventory`, `tools`, `weapons` | `inventory` | se **aplanan** en un solo mapa con el catálogo |
| `lastdaily`, `lastweekly`, `lastmonthly`, `lastwork` | iguales | milisegundos: las esperas siguen corriendo |
| `g_users.streak` / `lastDailyGlobal` | `users.streak` / `last_daily_global` | racha global |
| `g_users.weeklyStreak`, `monthlyStreak` | `users.weekly_streak`, `monthly_streak` | |
| `g_users.exp`, `level` | `users.xp`, `level` | |

Los `tools`/`weapons` del JS guardan `{durability, maxDurability}`; el
inventario de aquí guarda cantidades. Al importar, un objeto de equipo pasa a
**1 unidad** (el equipo no se gasta) y los consumibles a su cantidad.

### Saldo de la versión anterior (0.3.0)

La 0.3.0 guardaba el dinero **global** en `users.coins`. Al actualizar:

1. La migración 8 añade `users.economy_seeded` (por defecto 0).
2. La primera vez que alguien escribe en un chat se crea su fila y, si tiene
   saldo global sin sembrar, se le pasa **a ese chat** y se marca sembrado.
3. En los demás grupos arranca a 0: nadie cobra el saldo dos veces.

Probado en `TestUpgradeDesdeVersion5` (base de la 0.4.0 → monedas intactas) y
en `TestSiembraSaldoGlobalUnaVez` (no se duplica al entrar a un segundo grupo).

---

## 5. Cómo comprobarlo tú

```bash
go test ./internal/store -run 'Economia|Spend|Transfer|Racha|Cooldown|Inventario|Siembra|Upgrade' -v
go test ./cmds -run 'Amount|Reward|Comando|Alias|Currency|Wait' -v
```

En el teléfono, en un grupo de pruebas:

```
.economy            → ver si está activa
.daily              → primer cobro + paquete de bienvenida
.daily              → debe decir cuánto falta
.deposit all
.balance
.pay @alguien 5000  → sale del banco
.economyboard       → tiene que salir con tu saldo
.shop espada
.inventory
.economy off        → y cualquier comando de dinero queda cerrado
```

---

## 6. El gacha (personajes)

Mismas reglas de juego que el JS, con la capa de datos arreglada:

| Comando | Alias | Qué hace |
|---|---|---|
| `.roll` | `rollwaifu`, `rw` | saca un personaje al azar · espera **15 min** |
| `.claim` | `c`, `reclamar` | reclama el personaje **respondiendo a su mensaje** · espera **30 min** |
| `.harem` | `waifus`, `claims` | tus personajes, 20 por página, con valor total |
| `.setfav` | `favorito` | marca tu favorito (uno solo; es el que no se puede robar) |
| `.waifusboard` | `gachaboard` | ranking del grupo por valor del harem |
| `.gacha on\|off` | | activa o apaga el gacha del grupo (solo admins) |
| `.gachainfo` | `personajes` | cuántos personajes y series hay en el catálogo |

Tiempos del JS: el roll queda **reservado 20 s** para quien lo sacó y se puede
reclamar durante **60 s**. Un personaje tiene **un solo dueño por chat** (clave
`(chat, personaje)`), así que dos personas respondiendo a la vez no pueden ganar
las dos.

**Catálogo:** `CHARACTERS_FILE` (por defecto `data/characters.json`). Se acepta
el formato del JS (`{"serie": {"name": …, "characters": […]}}`), una lista suelta
de personajes y una lista de series. Si el archivo falta, el bot lo dice **una
vez** y los comandos contestan "el catálogo está vacío" en vez de reventar (en el
JS, un clon limpio tenía 18 comandos rotos por un archivo que ni siquiera estaba
en el repositorio).

**Ilustraciones:** se buscan en un tablero de contenido seguro, **sin claves de
API dentro del código** (el JS llevaba una escrita a mano), con 8 s de espera y
4 MB de tope. Si no hay imagen, el personaje se anuncia igual y se puede
reclamar.

## 7. Los juegos de azar

Los comandos de riesgo (`coinflip`, `slot`, `roulette`, `casino`, `ppt`,
`crime`, `robar`, `pescar`, `cazar`, `minar`, `mazmorra`, `aventura`, `ritual`,
`cofre`, `heal` y `pocion`) están en **B4.1** y se apoyan en esta misma capa:
`Spend`, `AddCoins`, `ClaimCooldown`, `Charge`, `Steal`, `AddStat` y el
inventario validado. No hay tablas nuevas de dinero, solo columnas de espera
(migración 12).

Sus números, sus probabilidades y las tres trampas que traía el JS (ruleta
pagando de más, casino con dados cargados y tragaperras con devolución > 100 %)
están en **[docs/JUEGOS.md](JUEGOS.md)**.

<div align="center">

_Shin-MD-Go · AGPL-3.0-only · riokuroxi-svg_

</div>
