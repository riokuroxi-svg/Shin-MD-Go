<div align="center">

# 反魂 · Shin-MD-Go

**El núcleo de Shin-MD, migrado a Go sobre whatsmeow.**

[![Estado](https://img.shields.io/badge/estado-🚧_en_desarrollo-EE3D86?style=flat-square&labelColor=2F1A24)](https://github.com/riokuroxi-svg/Shin-MD-Go)
[![CI](https://img.shields.io/github/actions/workflow/status/riokuroxi-svg/Shin-MD-Go/test.yml?style=flat-square&label=CI&labelColor=2F1A24&color=EE3D86)](https://github.com/riokuroxi-svg/Shin-MD-Go/actions)
![Go](https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&labelColor=2F1A24)
![Tests](https://img.shields.io/badge/pruebas-verdes_✅-3FB950?style=flat-square&labelColor=2F1A24)
![Licencia](https://img.shields.io/badge/licencia-AGPL--3.0--only-EE3D86?style=flat-square&labelColor=2F1A24)
![Termux](https://img.shields.io/badge/Termux-arm64_estático-000000?style=flat-square&labelColor=2F1A24)

<img alt="Aviso: en desarrollo" src="https://img.shields.io/badge/⚠️_ESTO_NO_ES_EL_BOT_COMPLETO_AÚN-faltan_los_bloques_B2--B8-EE3D86?style=for-the-badge&labelColor=2F1A24">

</div>

---

> **🚧 Proyecto en desarrollo.**
> Este repositorio es la **migración del núcleo** de
> [Shin-MD](https://github.com/riokuroxi-svg/Shin-MD) (JavaScript/Baileys) a **Go**, sobre
> [whatsmeow](https://github.com/tulir/whatsmeow).
> Lo que está aquí **compila, pasa pruebas y arranca de verdad**, pero todavía no tiene
> los 211 comandos: el bot de JS sigue siendo el bot de producción hasta cerrar los
> bloques B4–B8. Las versiones se publican cuando el bloque está verificado, no antes.

## ¿Por qué una versión en Go?

No es un capricho de lenguaje: es quitar de en medio la parte que se rompe.

| En el bot de JS | En Shin-MD-Go |
|---|---|
| `baileys` (fork de terceros) + `patch-baileys-pairing.cjs` parcheando el `postinstall` | Biblioteca de protocolo declarada (whatsmeow). **No hay nada que parchear** |
| `node_modules` + `sharp` nativo + Node ≥ 22.5 | **1 binario** (17 MB estático para Termux) |
| 15 reintentos → `clearSession()` que borraba la sesión en un corte de red | `EnableAutoReconnect`: la sesión no se borra por red, nunca |
| Cola de envío con `async` frágil (se congelaba si un envío fallaba) | Goroutine con `context`, timeout y `recover()` por handler |
| Pruebas que necesitan medio bot levantado | `go test ./...` prueba botones, parseo y SQLite **sin abrir WhatsApp** |

El motor anti-ban está portado **1:1** (misma huella: σ 0.25 · base 1200 ms · +25 ms/car ·
warm-up 20→500 en 7 días), así que sigue siendo una obra derivada de Shin-MD según el
[NOTICE](NOTICE).

## Estado actual — bloques B0 y B1 ✅

| Bloque | Qué incluye | Estado |
|---|---|---|
| **B0 · Núcleo** | cliente, sesión SQLite, pairing QR/código, cola anti-ban, logger, botones, router, economía, pruebas, binario arm64 | ✅ **hecho · revisado en 0.2.0** |
| **B0.1 · Caza de bugs** | bloqueo de la cola, calentamiento real, prioridad por comando, `.daily` atómico, `.pin` con media, envoltorios, owners por LID | ✅ **hecho** ([docs/CAMBIOS.md](docs/CAMBIOS.md)) |
| **B1 · Router real** | hooks `before` (antilink, afk), self-mode, admins de grupo por LID y por teléfono, **capa LID completa** | ✅ **hecho** ([docs/LID.md](docs/LID.md)) |
| **B2 · Menús y meta** | `.fax` (menú en imagen dibujada en Go), `.info`, `.owner`, `.report`, menú con imagen | ✅ **hecho** |
| **B3 · Economía, perfil y gacha** | `.work` `.daily` `.weekly` `.monthly`, banco (`.deposit` `.withdraw`), `.pay`, `.balance`, `.economyboard`, `.shop`, `.inventory`, `.economy on/off` y **gacha** (`.roll` `.claim` `.harem` `.setfav`) con migración de los datos del bot de JS | ✅ **hecho** ([docs/ECONOMIA.md](docs/ECONOMIA.md)) |
| B4 · Grupos y admin | warns, kick/promote, mute, horario, antistatus (el antilink ya entró en B1) | ⬜ |
| B5 · Stickers y media | sticker/toimg/tovid con `ffmpeg`, EXIF de sticker en Go | ⬜ |
| B6 · Descargas | play/yt/tiktok/ig/fb/x (decidir: Go puro o microservicio Node) | ⬜ |
| B7 · Panel web + operación | `/health`, rotación de logs, unidad `systemd` | ⬜ |
| B8 · Paridad de pruebas | portar las 161 pruebas del JS y sumar las nuevas | ⬜ |

Detalle completo del plan: **[docs/MIGRACION.md](docs/MIGRACION.md)**.

## Lo que ya funciona (probado, no prometido)

**Comandos cargados hoy: 39 únicos · 135 nombres · 2 hooks.**

| Comando | Qué hace |
|---|---|
| `.menu` / `.menucat` | menú con botones nativos y categorías |
| `.ping` | latencia real de recepción, uptime, estado de conexión y de la cola (con botones) |
| `.work` · `.daily` · `.weekly` · `.monthly` | ganas dinero: 2 000–4 000 por turno y rachas de hasta 1 015 000 |
| `.botones` | banco de pruebas: quick_reply, cta_url, cta_copy, cta_call |
| `.lista` | desplegable clásico (`ListMessage`) |
| `.carrusel` | tarjetas deslizables (`carouselMessage`) |
| `.encuesta ¿Café?; sí; no` | encuesta |
| `.evento` | evento de calendario con recordatorio |
| `.pin` | fija el mensaje al que respondes (texto, foto, vídeo, audio o sticker) |
| `.rich` | respuesta enriquecida estilo IA (experimental, `RICH_AI=1`) |
| `.antilink on\|off` | borra enlaces de quien no es admin (hook) |
| `.afk [motivo]` | avisa cuando te mencionan mientras estás ausente (hook) |
| `.lid` | diagnóstico: cómo ve el bot tu identidad (teléfono · LID) |
| `.self` · `.public` | el bot solo obedece al owner / responde a todos |
| `.fax` | **menú dibujado como imagen** (fuente propia, cero dependencias) |
| `.info` | versión, uptime, conexión, cola, memoria y estado de la base |
| `.owner` | contacto del dueño con botones (llamar, escribir, repo) |
| `.report` | el usuario te avisa de algo, con límite anti-spam |
| `.balance` · `.deposit` · `.withdraw` | cartera, banco y traspaso entre los dos (`all` acepta todo) |
| `.pay @persona 5000` | pasar dinero a otra persona (sale del banco) |
| `.economyboard` | ranking del grupo, de 10 en 10 |
| `.shop` · `.inventory` | tienda (7 objetos) e inventario con salud/stamina/magia |
| `.economy on\|off` | activa o apaga la economía del grupo (solo admins) |
| `.roll` · `.claim` · `.harem` · `.setfav` | gacha: sacar un personaje, reclamarlo respondiendo al mensaje, ver tu harem y marcar favorito |
| `.perfil` | ficha con dinero, nivel y rachas |

### Botones: catálogo completo

`quick_reply` · `cta_url` · `cta_copy` · `cta_call` · `send_location` · `open_webview` ·
`mpm` · `flow` · `single_select` · tarjetas con **imagen de cabecera** · **carrusel** ·
lista clásica · **añadir opción a encuesta** · **pin** · **evento** · **rich AI**.
Todos con **fallback a texto plano** si WhatsApp no los pinta.

Catálogo, comparativa con Baileys y estado de verificación: **[docs/BOTONES.md](docs/BOTONES.md)**.

## Instalación

```bash
git clone https://github.com/riokuroxi-svg/Shin-MD-Go
cd Shin-MD-Go

make build        # dist/shin
make run          # arranque directo
make termux       # binario estático arm64 para Android/Termux
make test         # pruebas
```

Requisitos: **Go 1.26 o superior**. Nada más — ni Node, ni Python, ni compiladores.

### Arrancar

```bash
./dist/shin                 # muestra el QR en la terminal
./dist/shin --code 52155…   # vincular con código de 8 dígitos
```

### Variables de entorno

Mismos nombres que el bot de JS, para reutilizar tu `.env`:

El bot **lee el `.env` solo** (el que esté junto al binario, o el que digas en
`SHIN_ENV_FILE`). Lo que exportes en la terminal tiene prioridad sobre el
archivo, así que `OWNERS=521… ./shin` sigue funcionando.

```env
BOT_NAME=SHIN-MD      # nombre que aparece en el menú y en .info
MENU_IMAGE=0          # 1 = .menu sale como imagen dibujada
OWNER_NAME=Roxi       # cómo te llama .owner
OWNER_NUMBER=52155…   # número para los botones de llamar/WhatsApp
BOT_PREFIX=".,/,!,+"
OWNERS="5215512345678"        # acepta +, espacios y guiones: "+52 155 1234 5678"
STATE_DIR=data
SHIN_DB_PATH=data/shin.db
SHIN_AUTH_DB=data/auth.db
LOG_LEVEL=info
ANTIBAN_BASE_MS=1200
ANTIBAN_SIGMA=0.25
ANTIBAN_PERCHAR_MS=25
WARMUP_START=20
WARMUP_END=500
RICH_AI=0        # 1 = respuestas enriquecidas estilo IA (experimental)
CHARACTERS_FILE=data/characters.json   # catálogo del gacha (si falta, lo dice)
```

### El menú en imagen (`.fax`)

`.fax` dibuja el menú con una fuente de mapa de bits propia (5×7) y `image/png`
del paquete estándar: **sin librerías, sin fuentes TTF, sin ffmpeg**. Con
`MENU_IMAGE=1` el propio `.menu` sale así.

Todo tiene techo (40 líneas × 40 columnas, 1600×3000 px, PNG de 4 MB), porque
lo dispara cualquiera desde el chat. Si el dibujo falla, cae a texto plano.

### Identidades: LID y teléfono

WhatsApp puede mandar al mismo usuario como `521…@s.whatsapp.net` (teléfono) o
como `123…@lid` (identificador interno). Este bot los trata como **la misma
persona**: la llave de la economía es siempre el teléfono cuando se conoce, las
parejas se guardan en SQLite y sobreviven a la sesión, y si alguien apareció
antes solo como LID sus monedas se **fusionan** al aprender el teléfono.

La red solo resuelve teléfono → LID; el sentido contrario únicamente se puede
contestar con lo ya visto. Explicado a fondo, con la comparación con Baileys:
**[docs/LID.md](docs/LID.md)**. Para comprobarlo en el teléfono: `.lid`.

### Calentamiento (warm-up)

El cupo diario de mensajes **no prioritarios** arranca en `WARMUP_START` y sube
hasta `WARMUP_END` en siete días. El primer día se guarda en la base de datos
(`settings`), así que **reiniciar el bot no reinicia el calentamiento**. Al
pasarse del cupo el bot no deja de responder: entra en **modo lento** (espera
duplicada) y `.ping` lo marca como `modo lento`. Los comandos con
`Priority: true` (`.menu`, `.ping`, `.menucat`) no gastan cupo.

Los envíos salen **de uno en uno**: la cola espera contra el último envío real,
no contra el inicio del handler.

## Arquitectura

```
Shin-MD-Go/
├── cmd/shin/main.go          arranque (equivale a index.js + boot/index.js)
├── cmds/                     1 archivo .go = 1 comando (equivalen a cmds/**/*.js)
├── internal/
│   ├── config/               .env
│   ├── xlog/                 logger con los iconos ◐ ◑ ✓ ✕
│   ├── ui/                   botones, tarjetas, fax (imagen), fuentes
│   ├── wa/                   cliente, cola anti-ban, parseo, envíos, LID
│   ├── bot/                  router, hooks, permisos de grupo, owners
│   ├── gacha/                catálogo de personajes e ilustraciones
│   └── store/                SQLite puro Go (economía por chat, rachas, gacha)
├── docs/                     MIGRACION.md (plan) · BOTONES.md (catálogo)
│                             ECONOMIA.md · CAMBIOS.md · LID.md · SEGURIDAD.md
└── Makefile
```

Mapa de equivalencias archivo por archivo con el bot de JS: [docs/MIGRACION.md](docs/MIGRACION.md).

## Pruebas

```bash
go test ./...    # 150 pruebas: botones, carrusel, lista, pin, evento, encuesta,
                 # parseo y envoltorios, cola anti-ban, SQLite, economía
                 # (concurrencia de dinero y reclamos), rachas, catálogo y gacha
go vet ./...     # limpio
gofmt -l .       # limpio
```

CI en cada push: [.github/workflows/test.yml](.github/workflows/test.yml).

## Historial

- **0.5.0** — bloque B3: **economía completa** (`.work`, `.daily`, `.weekly`,
  `.monthly`, banco, `.pay`, ranking, tienda e inventario) con la economía **por
  chat** como en el JS y las rachas globales, más el **gacha** (`.roll`,
  `.claim`, `.harem`, `.setfav`, ranking) y la migración de datos del bot de JS.
  Todo el dinero se mueve con sentencias condicionales: dos mensajes a la vez no
  pueden duplicar saldo. Detalle en **[docs/ECONOMIA.md](docs/ECONOMIA.md)**.
- **0.4.0** — bloque B2: `.fax` (dibujo de imagen sin dependencias), `.info`,
  `.owner`, `.report`, menú con imagen y **revisión de seguridad** (inyección
  de formato, amplificación, topes de CPU y de la base). Ver
  **[docs/SEGURIDAD.md](docs/SEGURIDAD.md)**.
- **0.3.0** — bloque B1: hooks (antilink, afk), modo self, permisos de admin de
  grupo y **capa LID completa** (store espejo, resolver, fusión de cuentas).
  Detalle en **[docs/LID.md](docs/LID.md)**.
- **0.2.0** — auditoría del núcleo: 9 fallos de raíz corregidos (uno de ellos
  congelaba el bot por completo). Detalle en **[docs/CAMBIOS.md](docs/CAMBIOS.md)**.
- **0.1.0** — primer esqueleto del bloque B0.

## Aviso

Usa **un número secundario**. Ningún bot es 100% inmune a un ban, ni este ni el de JS.

## Licencia

AGPL-3.0-only · ver [LICENSE](LICENSE) y [NOTICE](NOTICE).
Basado en [Shin-MD](https://github.com/riokuroxi-svg/Shin-MD) por riokuroxi-svg.
