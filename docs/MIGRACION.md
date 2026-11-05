# Migrar Shin-MD (JS/Baileys) → Go (whatsmeow)

_Documento de decisión técnica · 13-oct-2026_
_Analizado contra el repo real `riokuroxi-svg/Shin-MD` (commit `49cc640`, 211 comandos,
161 pruebas) y contra el código fuente de whatsmeow del 13-oct-2026._

---

## 1. Respuesta corta a "¿es más estable y trae botones estables?"

**Más estable: sí, pero la ganancia está en otro sitio del que crees.**
No es que Go "no se caiga": es que **desaparece la clase de fallo que hoy te
obliga a mantener 35.000 líneas de parches**. Mira tu propio repo:

| Hoy en JS | Qué pasa en Go |
|---|---|
| `baileys@6.7.24` (fork de terceros) | `whatsmeow` es una **biblioteca de protocolo**, no un fork parcheado |
| `scripts/patch-baileys-pairing.cjs` (17 KB de parches al `postinstall`) | **No existe**. No hay nada que parchear: el emparejamiento por código está en `PairPhone()` |
| 15 reintentos → `clearSession()` que te borraba la sesión | `EnableAutoReconnect` reconecta solo; tu lógica de "sesión registrada nunca se borra" pasa a ser 3 líneas de política |
| `node_modules` + `sharp` nativo + Node ≥22.5 | **1 binario**. Medido aquí: 18 MB (x86_64) · 17 MB (arm64 **estático**, para Termux) |
| PM2/watchdog porque el proceso se muere | `systemd` + `defer`/`recover`; el binario no tiene runtime que pueda quedarse sin memoria por GC de JS |
| `Math.random()` repartido por el código | el motor anti-ban vive en un solo sitio con `crypto`/`math` y **se prueba en 1 ms** |

**Botones: el mito hay que desmontarlo.** En Go *no hay* una función
`SendButtons()`. Los botones de WhatsApp **no son un flag de la librería**, son:

1. un protobuf `InteractiveMessage` → `nativeFlowMessage` con `messageVersion: 9`
   y botones `quick_reply` / `cta_url` / `cta_copy`;
2. **más** unos nodos binarios que envuelven el mensaje:
   `<biz><interactive type=native_flow v=1><native_flow v=9 name=mixed>`,
   `<quality_control source_type=third_party>` y (en privados) `<bot biz_bot=1>`.

Eso ya lo tienes resuelto y **documentado en tu propio `wa-nodes.js`**: si faltan
los nodos, el servidor acepta el mensaje y el teléfono lo tira sin pintar nada,
sin error. En Go es exactamente igual **pero está tipado y puede ir en un solo
archivo** — que es lo que hace este esqueleto en `internal/ui/buttons.go`
(`BuildButtons` + `BizNodes`), enviado con
`whatsmeow.SendRequestExtra{AdditionalNodes: &nodes}` (campo que existe en la
librería oficial: *"use this only if you know what you are doing"*).

Traducción honesta:

- **La estabilidad de los botones no viene de Go.** Viene de (a) usar la versión
  correcta del `nativeFlowMessage`, (b) mandar los nodos `<biz>` y (c) tener
  fallback a texto. Da igual el idioma: mal armado, no se pinta; bien armado, se
  pinta. Lo que Go te da es que ese conocimiento quede **en un solo archivo
  compilado**, en vez de copiado en tres sitios y con un `@ts-expect-error`
  pendiente de validar en teléfono (literalmente lo que dice tu `wa-nodes.js`).
- **La estabilidad del bot sí mejora de verdad** al quitar la capa de forks y
  parches de encima, y al eliminar `node_modules`.

---

## 2. Lo que se gana y lo que se pierde (sin marketing)

### Se gana
- **Sin dependencia de forks de npm.** Tu repo ya arrastra `aptoide-scraper`,
  `ytdl-core` (parcheado a `@distube/ytdl-core`), `baileys` con parche propio,
  `libsignal` override… Cada uno es un riesgo de supply-chain y de abandono.
- **Concurrencia real sin `async` frágil.** Cola de envío, descargas y panel web
  en goroutines con `context` cancelable: el clásico "la cola se congeló si un
  envío falla" no puede pasar (hay timeout por envío y `recover` por handler).
- **Un binario por plataforma.** Termux: `GOOS=linux GOARCH=arm64` y listo — sin
  compilar `sharp`, sin `node-gyp`, sin 300 MB de `node_modules` en el teléfono.
- **Errores que se ven antes de arrancar.** Ejemplo real de esta sesión: al portar
  la economía, la prueba en Go cazó que una cuenta nueva quedaba con `level: 0`
  (en JS el default era 1). Eso en JS se descubre cuando un usuario se queja.
- **Pruebas de 1 ms.** `go test ./...` prueba botones, parseo de respuestas y
  SQLite **sin abrir WhatsApp**. Tu suite actual de 161 pruebas corre sobre un bot
  que hay que levantar.
- **Licencias compatibles:** whatsmeow es MPL-2.0 (compatible con tu AGPL-3.0).
  Nada de forks con licencia dudosa.

### Se pierde / hay que trabajarlo
- **Comandos de descarga.** El 90 % de los scrapers "bonitos" viven en npm. En Go
  tienes 3 opciones: (1) llamar a `yt-dlp`/`gallery-dl` por `os/exec` (lo que ya
  haces de facto con `ytdlp-cliente.js`), (2) reescribir el scraper en Go, (3)
  **dejar esos comandos como microservicio Node al lado** (ver §5, opción híbrida).
- **Las 161 pruebas de JS no se convierten solas.** Se reescriben (son 28 archivos;
  las de UI/DB son las que más fácil portan porque aquí son puras).
- **Menos "recetas de comunidad".** Baileys tiene foros enteros con trucos
  (álbumes, encuestas, view-once…). whatsmeow tiene menos ejemplos: lo que en
  Baileys es `copy-paste`, aquí a veces es leer el proto. Compensación: el proto
  es la fuente de verdad y no cambia con cada fork.
- **El panel web**: `express` → `net/http` + `html/template`. Se porta, pero
  `tailwind`/plantillas hay que rehacerlas igual que están.
- **Curva de aprendizaje**: si un comando es "petición + formateo", portarlo toma
  minutos; si usa 4 dependencias npm, hay que decidir cada dependencia a mano.

---

## 3. Mapa de equivalencias (archivo por archivo)

| Shin-MD (JS) | Shin-MD-Go | Notas |
|---|---|---|
| `index.js` + `boot/index.js` | `cmd/shin/main.go` | flags `--code`, `--version`; `signal.NotifyContext` = cierre limpio |
| `src/core/socket.js` | `internal/wa/client.go` | `AddEventHandler` para `Connected/Disconnected/LoggedOut/Message` |
| `src/core/auth.js` (`node:sqlite`, WAL, checkpoint) | `store/sqlstore` de whatsmeow | ya trae migraciones versionadas propias; WAL con `_pragma=journal_mode(WAL)` |
| `src/core/engine.js` + `throttler.js` + `network/queue.js` | `internal/wa/queue.go` | misma fórmula: 1200 ms · σ0.25 · +25 ms/car · 20→500 en 7 días |
| `src/core/health.js` (403, backoff, loop 515) | `internal/wa/client.go` + política de eventos | whatsmeow maneja el backoff; tu política se reduce a "no borrar sesión por red" |
| `src/commands/router.js` | `internal/bot/router.go` | prefijos → cooldown 10/min → permisos → handler, con `recover()` |
| `src/commands/loader.js` (import dinámico) | `internal/bot/command.go` (`Register`) | los comandos se registran solos desde `cmds/*.go` |
| `src/commands/interactive.js` + `lib/wa-nodes.js` | `internal/ui/buttons.go` | **un solo sitio** para proto + nodos + fallback |
| `lib/native-params.js`, `native-actions.js` | `internal/ui/buttons.go` + `parse.go` | parámetros JSON tipados como `Button{Name, Params}` |
| `src/serialize.js` | `internal/wa/parse.go` | texto / botón / lista → comando |
| `src/storage/database.js` + `migrations.js` | `internal/store/{db,store}.go` | SQLite puro Go (`modernc.org/sqlite`), sin cgo |
| `src/services/logger.js` (chalk + rotación) | `internal/xlog` + `log/slog` + rotación propia | mismos iconos ◐ ◑ ✓ ✕ |
| `src/services/anti-crash.js`, `memory-monitor`, `temp-cleaner` | `recover()` en el router + un goroutine de limpieza | en Go el "OOM del runtime" ya no aplica igual |
| `src/web/server.js` (express + Basic auth) | `net/http` + `html/template` | failsafe LOOPBACK y chmod 600 se conservan |
| `cmds/**/*.js` (217 archivos) | `cmds/*.go` | se portan por bloques (§6) |
| `scripts/patch-baileys-pairing.cjs` | — | **se elimina**, no hace falta |
| `scripts/verificar-pairing.js` | — | `PairPhone()` oficial + prueba manual |

---

<!-- roxi estuvo aquí, dejando notas a las 4 de la mañana -->

## 4. Botones: cómo queda, probado

```go
// cmds/menu.go
return c.ReplyButtons(
    "SHIN-MD",                                       // header/título de la tarjeta
    body,                                            // texto
    "Toca un botón",                                 // footer
    bot.QuickReply("📋 Categorías", "menucat"),      // → ".menucat"
    bot.CTACopy("📮 Copiar prefijo", "."),           // copia al portapapeles
    bot.CTAURL("🌐 Repositorio", "https://…"),       // abre enlace
)
```

- El id del botón se convierte en comando con la misma convención que ya usas:
  `menu:economia` → comando `menu` con args `["economia"]` (`CommandFromButton`).
- Si el envío falla → se manda el mismo contenido en **texto plano con los
  botones listados** (`ui.Plain`). Nunca silencio.
- Probado en `internal/ui/buttons_test.go`: `messageVersion=9`, params JSON
  correctos y estructura del nodo `biz` (`biz > interactive > native_flow + quality_control`
  y `bot` **solo** en chats privados).

> ⚠️ Nota de campo: los botones nativos en cuentas *no-Business* funcionan
> mientras WhatsApp no cambie el `native_flow`. Por eso el fallback a texto no es
> opcional y por eso el `<bot biz_bot=1>` va solo en privados (igual que tu JS).

---

## 5. Estrategia recomendada: núcleo en Go + lo que coma `yt-dlp` donde esté

Dos caminos, en serio, sin adornos:

**A. Reescritura completa (strangler, recomendada)**
Núcleo Go primero (ya está el esqueleto) y los comandos se portan por lotes.
El bot de JS sigue corriendo con otro número mientras se porta. Cuando el lote
pasa las pruebas, se apaga el viejo. Ventaja: un solo bot al final.
Costo: los comandos de descarga se reescriben en Go o llaman binarios externos.

**B. Híbrido (si quieres migrar sin perder features)**
Binario Go = sesión de WhatsApp + anti-ban + router + economía (lo que se cae y
lo que más duele).
Microservicio Node **solo** para descargas/scrapers (`/yt`, `/ig`, `/fb`) en
`127.0.0.1:8787`; el bot Go lo llama con `net/http`. Ventaja: cero reescritura de
scrapers, y si Node se cae, el bot sigue vivo (hoy se cae el bot entero).
Costo: dos runtimes.

**Recomendación concreta:** empieza por **A** con los comandos que no dependen de
npm (menús, admin, economía, gacha, perfil, utilidades — la mayoría), y decide
**B** solo para el bloque de descargas si te da flojera pelear con cada scraper.

---

## 6. Plan por bloques (mismo estilo que tu `docs/plan-bloques.md`)

- **B0 · Núcleo** ✅ *hecho* · auditoría de fallos en **docs/CAMBIOS.md** (0.2.0)
  cliente + sesión SQLite + emparejamiento QR/código + cola anti-ban + logger +
  botones + router + economy store + `go test` verde + binario arm64.
  *Verificación:* `go build ./...`, `go vet ./...`, `go test ./...` → ok.
- **B1 · Paridad de router** ✅ *hecho (0.3.0)*
  hooks `before` (antilink, antistatus, afk), self-mode, `isAdmins/isBotAdmins`
  reales con `GetGroupInfo`, owner por LID **y** por teléfono.
  Ya dentro: owner por teléfono **y** por LID (`bot/owner.go`), filtro de
  `IsFromMe`, estados y difusiones bloqueados antes de todo, contexto
  cancelable en los handlers y poda del mapa de cooldowns.
  Además: hooks `before` (`bot/hooks.go`), antilink y afk reales, modo self,
  `AdminOnly`/`BotAdminOnly` aplicados con `GetGroupInfo` y comparación por
  LID **y** teléfono, y la capa LID completa (ver **docs/LID.md**).
  *Prueba:* chat de prueba + grupo de prueba; `.lid` para ver la identidad.
- **B2 · Menús y meta** ✅ *hecho (0.4.0)*
  `.menu`, `.menucat`, `.fax` (imagen dibujada en Go, sin dependencias),
  `.info`, `.owner`, `.report`, `.ping` con botones.
- **B3 · Economía y perfil**
  daily, work, banco, ranking, inventario, gacha (tu `ginko-db` se convierte en
  tablas con migraciones; el formato de dinero/XP se conserva para no perder datos).
- **B4 · Grupos y admin**
  antistatus, warn/kick/promote, mute, horario. (antilink y afk ya entraron en B1.)
- **B5 · Stickers y media**
  `sticker`/`toimg`/`tovid` con `ffmpeg` por `os/exec` (igual que hoy) y
  `EXIF` de sticker escrito a mano en Go.
- **B6 · Descargas**
  aquí decides A o B (ver §5). `play`, `yt`, `tiktok`, `ig`, `fb`, `x`.
- **B7 · Panel web + operación**
  `/health`, `/queue`, `/memoria` con Basic auth, rotación de logs, unidad
  `systemd` (`Restart=always`, `MemoryMax=512M`) o script de Termux.
- **B8 · Paridad de pruebas**
  portar las 161 pruebas → en Go deben quedar **más** porque la UI y la DB se
  prueban sin red. Objetivo: 1 test por comando + los tests de motor actuales.

Regla de oro igual que la tuya: **nada entra sin prueba que lo respalde**.

---

## 7. Riesgos reales (y cómo se tapan)

| Riesgo | Mitigación |
|---|---|
| whatsmeow no tiene helper de botones | ya resuelto: `internal/ui` (proto tipado + nodos) + tests |
| Cambios de proto en WhatsApp | el empaquetado está en un archivo; el test de `messageVersion` avisa |
| `modernc.org/sqlite` (puro Go) es algo más lento que el nativo | para un bot de WhatsApp sobra; si molesta: `mattn/go-sqlite3` (necesita cgo, complica Termux) |
| Un solo maintainer en whatsmeow (tulir) | es el estándar de facto desde 2022 (Beeper/Matrix), versionado por fecha; a diferencia de un fork de Baileys, es una dependencia **declarada y pública** |
| Perder features de npm | opción híbrida (§5B) |
| Portar 211 comandos | lotes por categoría, no todo a la vez; el JS sigue vivo hasta cerrar cada lote |

---

## 8. Cómo probar los botones antes de migrar nada

1. `go run ./cmd/shin --code <tu número de pruebas>` (número secundario, **nunca
   el principal** — la misma regla de tu README).
2. Manda `.menu`: debe llegar la tarjeta con 4 botones.
3. Toca "📋 Categorías": el router debe recibir `menucat` y responder.
4. Prueba en un grupo: los botones deben seguir saliendo (sin el nodo `bot`).
5. Apaga la conexión a mitad de un envío: el mensaje debe caer a texto plano,
   no desaparecer.

---

## 9. Qué haría yo ahora (orden exacto)

1. Copiar este esqueleto a un repo nuevo `Shin-MD-Go` (AGPL, mismo header).
2. Portar B1–B2 y usarlo **en paralelo** con el bot de JS en un número de prueba
   durante una semana: ahí vas a ver de verdad la diferencia de estabilidad
   (desconexiones, RAM, arranque).
3. Con datos en mano, decidir la suerte del bloque de descargas (A vs B) y seguir
   con B3–B8 en lotes.
