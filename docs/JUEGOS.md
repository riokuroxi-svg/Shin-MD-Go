# Juegos y riesgo · bloque B4.1

_Qué se apuesta, cuánto se paga y por qué los números no son los del bot de JS ·
5-nov-2026 · versión 0.7.0_

Los comandos de azar no traen tablas nuevas: se montan sobre la economía de
B3 (`chat_users`, `Spend`, `AddCoins`, `ClaimCooldown`, inventario validado).
Lo que sí cambió es **la aritmética del azar**, y no por gusto.

---

## 1. Los tres fallos de dinero que tenía el JS

El bot viejo hacía, en todos los juegos, lo mismo:

```js
let user = db.getChatUser(chat, quién)   // 1. leer
user.coins -= apuesta                    // 2. restar en JavaScript
db.setChatUser(chat, quién, 'coins', user.coins)  // 3. escribir el total
```

Eso deja tres agujeros, y los tres se veían en producción:

| Agujero | Cómo se notaba | Cómo queda aquí |
|---|---|---|
| **Doble cobro/pago** | dos mensajes en el mismo segundo leían el mismo saldo y escribían el mismo resultado: una de las dos apuestas desaparecía… o se cobraba dos veces | `UPDATE … SET coins = coins - ? WHERE coins >= ?` (una sola sentencia, condición dentro) |
| **Saldo negativo** | las multas (`crime`, `steal`, `dungeon`) restaban del banco "lo que faltara" sin comprobar: el banco acababa en −3,000 | `Charge()` baja cartera y banco en una sentencia con `MAX(…, 0)` y devuelve lo que **de verdad** se cobró |
| **Estadísticas imposibles** | la salud se restaba a mano y quedaba en −7; la stamina subía de 100 | `AddStat()` recorta siempre a `0..100` dentro del SQL |

Hay una prueba por cada uno: `TestApuestasSimultaneasNoRompenSaldo`,
`TestChargeNoCobraDeMas`, `TestAddStatRecorta`.

---

## 2. El azar: sin `Math.random()` y con la ruleta de verdad

`Math.random()` no lleva semilla. En un bot de un solo proceso da igual, pero
aquí se juega con dinero: dos procesos que arrancan a la vez reparten la misma
secuencia de tiradas, y quien lo sepa puede anticipar los `×300`. Se usa
`math/rand/v2`, que se siembra solo y es seguro con goroutines.

Y había dos juegos con las probabilidades **mal**:

### Ruleta

```js
// cmds/economy/roulette.js
const random = Math.floor(Math.random() * 37)
if (random <= 8) → verde, paga ×5     // 9 casillas de 37 = 24 %
if (random <= 22) → rojo, paga ×2
else → negro, paga ×2
```

Con esa rueda, apostar a verde devolvía **×5 con un 24 % de acierto**: 1.22 por
cada 1 apostado. Era una máquina de fabricar dinero, y con `.all` se vaciaba la
economía del grupo en una tarde.

Aquí la rueda es la europea de verdad: **18 rojos, 18 negros y un verde (el 0)**,
con la disposición real de los números.

| Apuesta | Acierta | Paga | Devolución |
|---|---|---|---|
| rojo / negro | 18/37 | ×2 | 97.3 % |
| verde (0) | 1/37 | ×14 | — |
| número exacto | 1/37 | ×36 | 97.3 % |

El verde se paga como número exacto porque el 0 **es** un número; el ×14 es
para quien apuesta "a verde" en general. Lo comprueba `TestRuletaDevolucion`
(200,000 tiradas) y `TestRuletaColoresDeLaRueda` (18/18/1).

### Casino (`.apostar`)

```js
let Aku = Math.floor(Math.random() * 101)   // el jugador, 0–100
let Kamu = Math.floor(Math.random() * 55)   // el bot,   0–54
```

El bot tiraba dados de 55 caras y el jugador de 101: **ganaba el bot el 85 % de
las veces**. Ahora los dos tiran 1–100 y, si empatan, se queda la casa — por eso
paga ×1.9 y no ×2 (devolución ≈ 94 %).

### Tragaperras

El JS mostraba un 3×3 y pagaba si una línea coincidía. Aquí son tres tambores
con tabla de premios calculada: se devuelve ≈ **86 %** de lo apostado (dos
iguales pagan poco, tres iguales pagan según la rareza). `TestSlotDevolucion`
compara la cuenta teórica con 200,000 giros simulados y falla si alguien toca
los pesos y deja la máquina pagando de más.

Símbolos, de común a raro: 🍒 ×6 · 🍋 ×8 · 🔔 ×12 · ⭐ ×25 · 💎 ×60 · 7️⃣ ×120 ·
✾ ×300 (los mismos dibujos que el JS).

---

## 3. Los comandos

### Juegos de apuesta (categoría `juegos`)

| Comando | Alias | Mínimo | Espera | Regla |
|---|---|---|---|---|
| `.coinflip <cant> [cara\|cruz]` | `cf`, `flip`, `caraocruz` | 100 | 10 s | 50/50, paga ×2 |
| `.slot <cant>` | `tragaperras`, `tragamonedas`, `slots` | 100 | 30 s | tabla de arriba |
| `.roulette <cant> <rojo\|negro\|verde\|0-36>` | `rt`, `ruleta` | 200 | 30 s | rueda europea |
| `.casino <cant>` | `apostar`, `dados`, `dado` | 100 | 30 s | 1–100 contra el bot, paga ×1.9 |
| `.ppt <piedra\|papel\|tijera\|lagarto\|spock>` | `piedrapapel`, `cachipun` | — | 1 min | gratis; gana 3,000–5,500, empata 800–1,500, pierde 1,000–3,000 |

Detalles:

- **`.coinflip`** acepta el lado (`cara`/`cruz`, también `sol`/`águila`/`sello`).
  Si no se dice nada, la moneda decide y se gana igual: el JS no dejaba elegir y
  el jugador no entendía por qué perdía "sin haber apostado a nada".
- **`.roulette`** acepta la cantidad y la casilla en cualquier orden
  (`.rt 500 rojo`, `.rt rojo 500`, `.rt 500 17`).
- **`.ppt`** es gratis a propósito (así era en el JS) y es el único juego que
  **crea** dinero; está en la lista de fuentes declaradas del punto 5.
- Todos usan `all`/`todo` para apostar la cartera entera, y todos avisan cuánto
  falta si el saldo no alcanza antes de tocar nada.

### Salidas y riesgo (categoría `economia`)

| Comando | Alias | Espera | Necesita | Resultado |
|---|---|---|---|---|
| `.pescar` | `fish`, `pesca` | 10 min | 🎣 caña | 60 %: +6,000–8,000 · 40 %: −2,000–4,000 · caña 8 % de romperse |
| `.cazar` | `hunt`, `caceria` | 12 min | 🏹 arco | 60 %: +10,000–13,000 · 40 %: −3,000–5,000 · arco 10 % |
| `.minar` | `mine`, `mineria`, `excavar` | 10 min | ⛏️ pico | +3,000–7,000 fijo y 50 % de evento (cofre, duende, roca, gato) |
| `.mazmorra` | `dungeon` | 17 min | 🪓 hacha o ✨ magia | 40 %: +12,000–15,000 · 30 %: −7,500–9,000 · 30 %: nada |
| `.aventura` | `adventure` | 20 min | ⚔️ espada o ✨ magia | 40 %: +14,000–18,000 · 30 %: −9,000–11,000 · 30 %: nada |
| `.ritual` | `invocar`, `invoke` | 12 min | 🗿 tótem | +8,000–11,000 (5 % legendario, 15 % bonus) |
| `.crime` | `crimen`, `delito` | 7 min | — | 40 %: +5,500–7,500 · 60 %: −4,000–6,000 |
| `.robar @alguien` | `steal`, `rob` | 1 h | — | 70 %: 3,000–9,000 de su cartera · 30 %: multa 2,000–5,000 |
| `.cofre` | `coffer` | 24 h | — | 50 %: 25,000 · 30 %: 40,000 · 20 %: vacío |
| `.heal` | `curar`, `medico` | 3 s | dinero | 500 por tramo de 10 de salud, 300 por tramo de stamina |
| `.pocion` | `potion`, `magia` | 3 s | 🧪 poción | magia al 100 % |

Todas las salidas gastan **1–5 de stamina** y algo de salud (menos pesca, caza y
mina, que gastan poca). Sin stamina o sin salud suficiente el comando **no se
ejecuta** y avisa; no hay forma de quedar en negativo.

**Herramientas.** El JS tenía "durabilidad 1–100 y se rompe a 10 o menos", con
la resta hecha a mano: en la práctica la herramienta se rompía en el primer uso y
el mensaje ni lo decía. Aquí la rotura es una probabilidad explícita (8 % caña,
10 % arco, 10 % pico) y **se avisa en el mismo mensaje** cuando pasa.

**`.robar`** era el comando más roto del JS: solo dejaba robar a quien llevaba
"1 hora inactivo", pero esa hora salía de un campo (`lastCmd`) que no existía —
la condición nunca se cumplía y nunca se podía robar a nadie. Aquí el límite es
por **víctima** (`last_robbed`, migración 10): nadie puede ser robado dos veces
en la misma hora, y el robo se hace con una transacción que nunca deja a la
víctima en negativo (`Steal()`), así que lo robado puede ser menos de lo pedido
si esa persona se gastó el dinero en el mismo segundo.

**`.heal`** cobra por tramos como el JS y **cobra primero, cura después**; si el
dinero no alcanza, devuelve lo poco que hubiera cobrado y no cura nada. Se puede
curar a otra persona mencionándola: paga siempre quien escribe.

**`.pocion`** es el antiguo `.heal pocion` del JS (que allí se decidía mirando el
nombre del comando). Restaura la **magia**, que es lo que se gasta cuando sales
sin arma.

---

## 4. Lo que se dejó fuera (y por qué)

| Del JS | Decisión |
|---|---|
| **`.mine` con botones pendientes** | El JS abría un mensaje con "🗝️ Abrirlo / 🚶 Ignorar" y guardaba la partida **3 minutos en memoria**. Si el bot se reiniciaba, el mensaje quedaba vivo pero la partida no: el jugador ya había pagado la stamina y no podía responder. Aquí el evento se resuelve en el mismo mensaje, con la misma probabilidad de éxito. |
| **Animación del `.slot`** (5 fotogramas editando el mensaje) | Editar un mensaje 5 veces cada 300 ms multiplica por 5 el riesgo de bloqueo y no aporta dinero. El resultado sale de una vez, con los tres tambores a la vista. |
| **`.math`** | Necesita un juego pendiente por chat (y en el JS vivía en un `global` sin límite: cualquiera podía abrir infinitos). Si entra, entrará con estado en la base, no en memoria. |
| **`.slut`** | Es contenido adulto y no es un juego de dinero; queda para el bloque de NSFW si se decide portarlo. |

---

## 5. La invariante del dinero

Regla del proyecto, comprobada por pruebas: **el dinero solo puede aparecer en
las fuentes declaradas**. Todo lo demás mueve dinero de un sitio a otro.

Fuentes (crean dinero de la nada):

```
.work  .daily  .weekly  .monthly  .cofre  .ppt
.pescar  .cazar  .minar  .mazmorra  .aventura  .ritual  .crime
```

Sumideros (lo destruyen): `.shop`, `.heal`, la rotura de herramientas, las
multas de `.crime`/`.robar` y las pérdidas de las apuestas.

Movimientos puros (no crean ni destruyen): `.pay`, `.robar`, apuestas ganadas o
perdidas, `.deposit`, `.withdraw`.

Las pruebas que sostienen esto:

| Prueba | Qué vigila |
|---|---|
| `TestStealMueveSinCrearDinero` | el total de las dos cuentas no cambia |
| `TestStealConcurrenteNoDuplica` | 4 robos a la vez sobre 10,000 no sacan más de 10,000 |
| `TestApuestasSimultaneasNoRompenSaldo` | 50 apuestas de 500 sobre 10,000: cobra 20, no 21 |
| `TestChargeCobraCarteraYBanco` / `TestChargeNoCobraDeMas` | las multas nunca dejan saldo negativo |
| `TestSlotDevolucion` | la tragaperras devuelve menos de lo que cobra |
| `TestRuletaDevolucion` | la ruleta devuelve ≈ 97 %, no 122 % |

---

## 6. Cómo se prueba a mano

1. `.daily` para tener saldo (y el paquete de bienvenida con pico, caña no…
   la caña hay que comprarla: `.shop cana`).
2. `.balance`, luego `.coinflip 1000`, `.slot 500`, `.rt 500 rojo`.
3. `.ppt tijera` y comprobar que el empate paga y que la espera es de 1 minuto.
4. `.minar` sin pico: tiene que avisar y **no** gastar stamina.
5. `.robar @ti_mismo`: tiene que rechazarlo. `.robar @otro` dos veces seguidas:
   la segunda tiene que decir que a esa persona ya le robaron hace poco.
6. En la base (`sqlite3 data/shin.db`):

```sql
SELECT user_id, coins, bank, health, stamina, magic FROM chat_users;
-- ninguna fila puede tener coins < 0, bank < 0 ni stats fuera de 0..100
```

---

<div align="center">

_Shin-MD-Go · AGPL-3.0-only · riokuroxi-svg_

</div>
