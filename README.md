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

## Estado actual — bloques B0 a B5.1 y B7.0 ✅

| Bloque | Qué incluye | Estado |
|---|---|---|
| **B0 · Núcleo** | cliente, sesión SQLite, pairing QR/código, cola anti-ban, logger, botones, router, economía, pruebas, binario arm64 | ✅ **hecho · revisado en 0.2.0** |
| **B0.1 · Caza de bugs** | bloqueo de la cola, calentamiento real, prioridad por comando, `.daily` atómico, `.pin` con media, envoltorios, owners por LID | ✅ **hecho** ([docs/CAMBIOS.md](docs/CAMBIOS.md)) |
| **B1 · Router real** | hooks `before` (antilink, afk), self-mode, admins de grupo por LID y por teléfono, **capa LID completa** | ✅ **hecho** ([docs/LID.md](docs/LID.md)) |
| **B2 · Menús y meta** | `.fax` (menú en imagen dibujada en Go), `.info`, `.owner`, `.report`, menú con imagen | ✅ **hecho** |
| **B4 · Grupos y admin** | warns con límite y expulsión automática, `.kick` `.promote` `.demote`, abrir/cerrar, enlace y su restablecimiento, `.tagall`/`.hidetag`, info y datos del grupo, bienvenida y despedida, `.antistatus`, conteo de actividad, `.bot on/off` | ✅ **hecho** ([docs/GRUPOS.md](docs/GRUPOS.md)) |
| **B3 · Economía, perfil y gacha** | `.work` `.daily` `.weekly` `.monthly`, banco (`.deposit` `.withdraw`), `.pay`, `.balance`, `.economyboard`, `.shop`, `.inventory`, `.economy on/off` y **gacha** (`.roll` `.claim` `.harem` `.setfav`) con migración de los datos del bot de JS | ✅ **hecho** ([docs/ECONOMIA.md](docs/ECONOMIA.md)) |
| **B3.1 · Perfil, nivel y matrimonio** | `.profile` `.level` `.lboard`, ficha editable, `.marry`/`.acepto`/`.divorce`, AFK con recompensa, **una sola curva de XP** | ✅ **hecho** ([docs/PERFIL.md](docs/PERFIL.md)) |
| **B4.1 · Juegos de azar** | `.coinflip` `.slot` `.roulette` `.casino` `.ppt` y las salidas de riesgo, con probabilidades recalculadas | ✅ **hecho** ([docs/JUEGOS.md](docs/JUEGOS.md)) |
| **B5 · Stickers y media** | `.sticker` (13 formas, 14 efectos), `.toimg`, `.stickerinfo`, EXIF propio en Go, vídeo local con `ffmpeg` | ✅ **hecho** ([docs/STICKERS.md](docs/STICKERS.md)) |
| **B5.1 · Utilidades (1/2)** | `.qr` `.morse` `.demorse` `.recordar` `.olvidar` `.say` `.encuesta` `.pfp` `.wastalk` `.readviewonce` `.get`, encuestas y quiz nativos, recordatorios que sobreviven al reinicio | ✅ **hecho** ([docs/UTILIDADES.md](docs/UTILIDADES.md)) |
| **B7.0 · Operación y estado** | `.health` `.runtime` `.warmup` `.infobot` `.compat` `.allmenu` `.restart` `.update` `.subir` `.cache` `.shell` `.setmenu` | ✅ **hecho** ([docs/OPERACION.md](docs/OPERACION.md)) |
| **B5.2 · Utilidades (2/2)** | `.traducir` (2 proveedores + detector propio) `.tts` `.acortar` `.tourl` `.crypto` `.letra` `.gitclone` `.githubstalk` `.ai` (con `GEMINI_KEY`) y `.hd` + `.carbon` **locales** (sin subir nada a webs ajenas); `.benchdl` queda para B6 | ✅ **hecho** ([docs/UTILIDADES2.md](docs/UTILIDADES2.md)) |
| **B6.0 · Música (escucha y descarga)** | `.play` (catálogos libres: Audius/Deezer/iTunes en paralelo, lista numerada, semáforo por chat) · `.escucha` (preview oficial) — *decisión: solo donde se permite descargar, no se piratea YouTube* | ✅ **hecho** ([docs/MUSICA.md](docs/MUSICA.md)) |
| **B6.1 · Descargas sociales** | `.yt` `.ytmp3` `.tiktok` `.ig` `.fb` `.x` vía **yt-dlp cuando está instalado** (decisión auditada: YouTube 2026 exige PoToken; no se finge, se explica) — tope 100 MB, plazo 5 min | ✅ **hecho** |
| **B7 · Panel web** | `/health` `/queue` `/memoria` `/metrics` con failsafe loopback + Basic auth con candado, **rotación de logs** (4 MB × 3), unidad `systemd` | ✅ **hecho** ([docs/PANEL.md](docs/PANEL.md)) |
| **B8 · Paridad de catálogo y tests** | los 640 nombres del JS están en Go o justificados en [NO_PORTADO](docs/NO_PORTADO.md); `TestParidadCatalogo` lo blinda · 416 tests en 16 paquetes | ✅ **hecho** |

Detalle completo del plan: **[docs/MIGRACION.md](docs/MIGRACION.md)**.

## Lo que ya funciona (probado, no prometido)

**Comandos cargados hoy: 131 únicos · 418 nombres · 4 hooks.**

| Comando | Qué hace |
|---|---|
| `.menu` / `.menucat` | «Nube de Shin»: ficha dinámica del bot y usuario, botones nativos y categorías paginadas |
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
| `.warn` · `.warns` · `.delwarn` · `.setwarnlimit` | advertencias con motivo, autor y fecha; al llegar al límite, expulsa |
| `.kick` · `.promote` · `.demote` | expulsar y cambiar roles (con protecciones: ni el bot, ni el creador, ni los admins) |
| `.open` · `.close` · `.link` · `.revoke` | abrir, cerrar, ver el enlace y restablecerlo |
| `.tagall` · `.hidetag` | mencionar a todos, con o sin listar los números |
| `.gp` · `.setgpname` · `.setgpdesc` · `.setgpbanner` | información y datos del grupo |
| `.welcome` · `.goodbye` · `.setwelcome` · `.setgoodbye` | avisos de entrada y salida con marcadores `@user` `#grupo` `#miembros` |
| `.antistatus` | borra los estados compartidos en el grupo y advierte |
| `.count` · `.topcount` · `.topinactive` | actividad del grupo (contador propio, no el JSON del JS) |
| `.bot on\|off` | apaga el bot en ese grupo |
| `.coinflip` · `.slot` · `.roulette` · `.casino` · `.ppt` | juegos de apuesta (cara o cruz, tragaperras, ruleta europea, dados y piedra-papel-tijera) |
| `.pescar` · `.cazar` · `.minar` | salidas con herramienta: piden caña, arco o pico y gastan stamina |
| `.mazmorra` · `.aventura` · `.ritual` | salidas de riesgo: hacha/espada/tótem, magia si no llevas arma |
| `.crime` · `.robar` | dinero rápido (40 % de éxito) y robo a otra persona (uno por hora y por víctima) |
| `.cofre` · `.heal` · `.pocion` | cofre diario, curar salud/stamina con dinero y recuperar magia con poción |
| `.sticker` (`.s`) | foto, vídeo o sticker → sticker de 512×512, con **13 formas** (`-c` círculo, `-v` corazón…) y **14 efectos** (`-sepia`, `-blur`…) |
| `.toimg` · `.stickerinfo` | sticker → PNG o MP4 (**sin ezgif**: ffmpeg local) y datos del paquete del sticker |
| `.tr inglés hola` | traducción con dos proveedores encadenados, detector de idioma propio y botón de copiar |
| `.tts` · `.ia` | nota de voz del texto · IA con `GEMINI_KEY` (sin key explica cómo conseguirla, no finge) |
| `.acortar` · `.crypto` · `.letra` | TinyURL con botones · BTC/ETH/SOL en USD y MXN · letra troceada |
| `.githubstalk` · `.gitclone` | ficha de GitHub con avatar · lista 5 repos o baja el `.zip` (tope 45 MB) |
| `.hd` · `.carbon` | mejora 2× + enfoque **local** · código → imagen **local** (sin subir nada a webs ajenas) |
| `.ptv` · `.tourl` | vídeo redondo (límites reales del protocolo) · archivo → enlace temporal que caduca solo |

### Botones: catálogo completo

`quick_reply` · `cta_url` · `cta_copy` · `cta_call` · `send_location` · `open_webview` ·
`mpm` · `flow` · `single_select` · tarjetas con **imagen de cabecera** · **carrusel** ·
lista clásica · **añadir opción a encuesta** · **pin** · **evento** · **rich AI**.
Todos con **fallback a texto plano** si WhatsApp no los pinta.

Catálogo, comparativa con Baileys y estado de verificación: **[docs/BOTONES.md](docs/BOTONES.md)**.

## Instalación

**Un comando** (Termux, Linux, macOS, WSL — detecta el entorno, instala Go con
huella verificada, compila y deja el servicio listo):

```bash
curl -fsSL https://raw.githubusercontent.com/riokuroxi-svg/Shin-MD-Go/main/shin-go.sh | sh
```

El mismo comando sirve para **actualizar** (si el código ya está, lo trae
fresco). Variables: `SHIN_GO_DIR=RUTA`, `SHIN_GO_REF=vX.Y.Z`,
`SHIN_GO_CON_YTDLP=0` (saltar yt-dlp), `SHIN_GO_SIN_SERVICIO=1`,
`SHIN_GO_DRY_RUN=1` (ensayo sin tocar nada). También instala **yt-dlp**
standalone en `~/.local/bin` para `.yt`/`.tiktok`/`.ig`.

Modo manual (si prefieres mirar antes de instalar):

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
./dist/shin                 # la primera vez te PREGUNTA tu número y te da el código
./dist/shin --code 52155…   # el número a mano, sin pregunta
PAIRING_NUMBER=52155… ./dist/shin   # lo mismo, por entorno (servicios)
```

Mientras corre, la pantalla es fija: arriba el estado (conexión, encendido,
cuenta, comandos, cupo, cola, disco, memoria, el panel web) y debajo el
registro del bot, repintado cada segundo y en cuanto llega un aviso. Se dibuja
en el búfer alternativo, así que un `clear` o girar el teléfono no lo descuadran
y, al salir, tu terminal recupera su historial intacto. Teclas: `e` bajar al
registro (y volver con la misma), `↑`/`↓` releer, `r` reconectar el socket, `?`
ayuda, `q` parar.

**En un servidor no hay teclado, y no hace falta.** Estas cuatro órdenes son
todo lo que se necesita —sin systemd y sin PM2, y el proceso sobrevive a que se
cierre la SSH:

```bash
./dist/shin start --code 52155…   # arranca en segundo plano y escribe el log
./dist/shin status                # ● en ejecución · pid · 1h 04m · 24M + lo último del log
./dist/shin logs -f               # seguir el log (Ctrl+C sólo deja de mirar, no para el bot)
./dist/shin stop                  # SIGTERM y esperar el cierre limpio
```

Sin terminal (systemd, pm2, `nohup`, `> log.txt`) **no se pinta nada**: el bot
suelta líneas de texto grepeables, con el estado al arrancar y sin prometer una
tecla que no existe. El PID vive en `data/shin.pid`; si el fichero miente —un
`kill -9`, un reinicio—, `status` y `stop` lo dicen y lo limpian.

La vinculación es siempre **por código de 8 letras** (Ajustes › Dispositivos
vinculados › Vincular con número): el QR no existe — el bot corre en el
propio móvil, donde nadie podría escanearlo. Sin sesión y sin terminal
donde preguntar (un servicio, un tubo), no se arranca a ciegas: te dice la
orden exacta y para.

### Variables de entorno

Mismos nombres que el bot de JS, para reutilizar tu `.env`:

El bot **lee el `.env` solo** (el que esté junto al binario, o el que digas en
`SHIN_ENV_FILE`). Lo que exportes en la terminal tiene prioridad sobre el
archivo, así que `OWNERS=521… ./shin` sigue funcionando.

```env
BOT_NAME=SHIN-MD      # nombre que aparece en el menú y en .info
MENU_IMAGE=0          # 1 = .menu sale como imagen dibujada
MENU_WEATHER_CITY="Mexico City"  # ciudad del clima de la ficha textual
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
INTERACTIVE_STRATEGIES=baileys,nativa,texto  # orden; si falta «texto», se añade al final
WORKERS=4        # mensajes que se atienden a la vez (1 = uno detrás de otro)
QUEUE=128        # cuántos mensajes pueden esperar antes de descartar
CHARACTERS_FILE=data/characters.json   # catálogo del gacha (si falta, lo dice)
GEMINI_KEY=             # key de aistudio.google.com/apikey para .ai (vacío = .ai explica cómo pedirla)
GEMINI_MODEL=gemini-1.5-flash

# Panel web (B7) — escucha en 127.0.0.1 salvo LOOPBACK=0 (que exige clave)
PORT=3000
LOOPBACK=1
PANEL_PASSWORD=
LOG_FILE=data/shin.log   # rotación 4 MB × 3 · «off» = solo stdout

# Direcciones de origen propias (opcional)
RED_PREFIJO=          # p. ej. 2001:db8:1:2::/64 — vacío = IP del sistema
RED_MAX_POR_IP=50     # peticiones por dirección dentro de la ventana
RED_VENTANA=60        # segundos de la ventana
```

### Música (`.song`, `.play`) y banco de pruebas

`internal/musica` busca en **Audius** (audio completo, catálogo libre),
**Deezer** e **iTunes** (ficha + preview oficial de 30 s). Lo comercial nunca se
descarga: `Pista.Libre` lo impide y hay una prueba que lo fija.

```sh
go run ./cmd/shin-musica buscar "lofi"
go run ./cmd/shin-musica bajar  "chill instrumental" -o /tmp/cancion.mp3
```

### Red propia (opcional)

Si el servidor tiene un bloque IPv6 (casi todos los VPS traen un `/64`), el bot
puede repartir su tráfico saliente entre las direcciones del bloque en vez de
salir siempre por la misma:

```sh
sh deploy/red-ipv6.sh revisar                     # ¿qué hay en este equipo?
sudo sh deploy/red-ipv6.sh montar 2001:db8:1:2::/64
sh deploy/red-ipv6.sh probar 2001:db8:1:2::/64    # ¿sale por direcciones distintas?
```

El script enruta el prefijo a `lo`, activa `ip_nonlocal_bind` y **cierra la
entrada al bloque con nftables** para que no quede un proxy abierto (eso es lo
que hace que un proveedor suspenda el servidor). `.info` muestra una línea
`Red:` con el estado, o `IP del sistema` si no hay bloque configurado.

Esto administra direcciones propias: no genera `poToken` ni salta ninguna
protección de plataforma.

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
│   ├── xlog/                 logger con los iconos ◐ ◑ ✓ ✕ y rotación propia
│   ├── web/                  panel HTTP (/, /health, /queue, /memoria, /metrics)
│   ├── ui/                   botones, tarjetas, fax (imagen), fuentes
│   ├── wa/                   cliente, cola anti-ban, parseo, envíos, LID
│   ├── bot/                  router, hooks, permisos de grupo, owners
│   ├── apis/                 clientes de terceros del B5.2 (traducir, voz,
│   │                         precios, GitHub, letra, subida, Gemini) — cada
│   │                         URL es inyectable: las pruebas no tocan internet
│   ├── gacha/                catálogo de personajes e ilustraciones
│   ├── utilidades/           morse, QR dibujado en Go, parser de encuestas,
│   │                         filtro anti-SSRF para `.get`
│   └── store/                SQLite puro Go (economía por chat, rachas, gacha,
│                             advertencias y actividad de grupo, recordatorios)
├── docs/                     MIGRACION.md (plan) · BOTONES.md (catálogo)
│                             ECONOMIA.md · GRUPOS.md · CAMBIOS.md · LID.md
│                             SEGURIDAD.md · UTILIDADES.md · UTILIDADES2.md
│                             PERFIL.md · JUEGOS.md · STICKERS.md · OPERACION.md
│                             PANEL.md · ARQUITECTURA_ADAPTADOR_WHATSAPP.md
└── Makefile
```

Mapa de equivalencias archivo por archivo con el bot de JS: [docs/MIGRACION.md](docs/MIGRACION.md).

## Pruebas

```bash
go test ./...    # 392 pruebas: botones, carrusel, lista, pin, evento, encuesta,
                 # parseo y envoltorios, cola anti-ban, SQLite, economía
                 # (concurrencia de dinero y reclamos), rachas, catálogo, gacha,
                 # advertencias y actividad de grupo, juegos de azar (probabilidad
                 # de ruleta, tragaperras y casino, y topes de las apuestas),
                 # stickers (EXIF, formas, filtros y conversión con ffmpeg),
                 # repartidor de mensajes (paralelo, cola con techo, cierres),
                 # utilidades (morse ida y vuelta, QR y su PNG, parser de
                 # encuesta/quiz/multi, el protobuf que se envía, filtro anti-SSRF)
                 # (la única que se salta sola es la de vídeo, si no hay ffmpeg)
go test -race ./...   # lo mismo con el detector de carreras: los 8 paquetes en verde
go vet ./...     # limpio
gofmt -l .       # limpio
```

CI en cada push: [.github/workflows/test.yml](.github/workflows/test.yml).

## Historial

- **0.25.0-rc2** — **la ficha textual del bot de JavaScript, sin imágenes**:
  la portada principal muestra saludo, nombre del usuario, versión real,
  motor whatsmeow, uptime, clima, comandos únicos, rango, teléfono y prefijo.
  Conserva los botones, las listas paginadas y la degradación automática a
  texto. El pie pequeño enlaza el repositorio del bot de JavaScript. El clima
  usa Open-Meteo con plazo de tres segundos y caché de diez minutos. Detalles
  en [`docs/MENU-0.25.0-RC2.md`](docs/MENU-0.25.0-RC2.md).
- **0.25.0-rc1** — **«Nube de Shin», candidato para probar en teléfono**: la
  portada de `.menu` adopta el diseño con nube, ala y estrella, nombre Unicode,
  saludo y resumen legible. Las dos rutas principales quedan a la vista y las
  otras acciones usan la hoja nativa «Ver todo». `.menucat` deja de crear una
  hilera de quick replies y usa `ListMessage` paginados de ocho categorías más
  navegación, siempre dentro del máximo de 10 filas. Los IDs siguen entrando
  al mismo router y cualquier error conserva el fallback escribible. El
  candidato está publicado en `main` para probarlo; el tag
  `menu-pre-redesign-0.24.0` conserva el punto anterior por si hay que volver.
  Detalles en [`docs/MENU-0.25.0-RC1.md`](docs/MENU-0.25.0-RC1.md).
- **0.24.0** — **botones nativos con el protobuf de Baileys y degradación
  escalonada**: el mensaje lleva `viewOnceMessage.message`, contexto de
  dispositivo v2, `nativeFlowMessage.messageVersion=1` (el `9` corresponde al
  nodo binario, no al protobuf) y el `biz` completo con `decision_id` de 20
  bytes + `decision_source=df`. Una fixture producida por Baileys 7.0.0-rc14 se
  compara campo por campo y byte por byte contra Go. Todo vive en
  `internal/interactivos`: los comandos solo ven `bot.Messenger`, y la cadena
  configurable prueba `baileys → nativa → texto`; el menú de texto conserva
  las órdenes si fallan los formatos ricos. Las listas van como `ListMessage`
  directo y whatsmeow emite un único nodo `product_list`, no el `single_select`
  que puede recibir ACK y desaparecer en cuentas normales. Detalles en
  [`docs/INTERACTIVOS-0.24.0.md`](docs/INTERACTIVOS-0.24.0.md).
- **0.23.1** — **el código de vinculación ya no queda debajo de la TUI**:
  en una sesión nueva, `PairPhone` imprime las ocho letras como texto estático
  y `Login` se queda ahí —sin `clear`, búfer alternativo ni repintados— hasta
  reunir las dos confirmaciones de WhatsApp: `PairSuccess` (el teléfono aceptó
  el código) y `Connected` (terminó la reconexión autenticada). Solo entonces
  arranca el panel. También se aplica al QR. Una prueba de regresión bloquea
  deliberadamente `Connected` y demuestra que la barrera no abre ni la TUI
  puede tapar el código. Detalles en
  [`docs/CAMBIOS-0.23.1.md`](docs/CAMBIOS-0.23.1.md).
- **0.23.0** — **la consola, como la del otro bot (y usable en un VPS sin
  teclado)**: portada de seis líneas en vez de banner de letras gigantes; el
  código de vinculación en texto seleccionable con las letras separadas (no
  arte de bloques, que no se podía copiar con el dedo y partía el borde en 50
  columnas); cajas de la casa —58 columnas, borde de una sola columna `│ ╭ ╮`,
  cero `█▓▒░`—; paleta en HEX de la casa medida sobre el ancho **visible**; la
  pantalla viva pasa al **búfer alternativo** con salida sincronizada y
  repintado cada segundo + en cada aviso (inmune a `clear`, a girar el móvil y
  al scroll, y el historial del usuario sale intacto al salir); el pie ya no
  promete `Ctrl+C` donde no hay teclado; y órdenes nuevas
  `shin start · stop · status · logs · restart` (pid en `data/shin.pid`,
  `setsid`, SIGTERM esperado, zombis no cuentan como vivo, log leído desde el
  final y reabierto si se rotó). Sin terminal no se dibuja nada: texto plano.
  De paso, cinco fallos que sólo se ven en un pty de 50 columnas: `OPOST`
  apagado (salía la escalera), dos relojes en cada línea de historial, un
  repintado escribiendo en la terminal normal, `Parar()` colgado por el watcher
  de `SIGWINCH` y el envoltorio comiéndose las sangrías. Detalles en
  [`docs/CAMBIOS-0.23.0.md`](docs/CAMBIOS-0.23.0.md).
- **0.22.0** — **consola dinámica en Termux y VPS**: menú de dos puertas para
  vincular (código de 8 letras / QR) con flechas y atajo numérico, pantalla
  viva que se repinta en su sitio, plan B medido para el QR que no cabe
  (PNG a `data/vincular-qr.png` + `termux-open`), y `Ctrl+C` devuelto a su
  significado de siempre dentro del modo de teclas. La cara se rediseñó entera
  en 0.23.0; las ideas de usabilidad se quedaron.
- **0.21.0** — **adaptador WhatsApp desacoplado + repartidor FIFO por
  chat**: `cmd/shin`, `cmds` e `internal/bot` dejan de importar tipos,
  eventos y Protobufs de `whatsmeow` en producción (31 → 13 archivos con
  dependencia directa, concentrados en `internal/wa` e `internal/ui`; ver
  [docs/ARQUITECTURA_ADAPTADOR_WHATSAPP.md](docs/ARQUITECTURA_ADAPTADOR_WHATSAPP.md)).
  La recepción se normaliza en `wa.IncomingMessage` / `wa.GroupChange` con
  payload crudo privado para medios; los envíos de encuestas/quiz y tarjetas
  pasan por `wa.PollSpec` y `wa.RichCard`, y `wa.Client` encapsula el
  cliente subyacente (`Connected`, `AccountName`, `OwnID`). El repartidor
  pasa a cola acotada **FIFO por conversación** con pool fijo de workers,
  cupo separado para eventos de grupo (invalidación síncrona de caché +
  despacho asíncrono) y métricas de espera/duración expuestas en el panel
  web (`/health`, `/queue`, `/metrics`).
- **0.20.0** — **la cara, bien (pterm)**: la estética deja de ser ANSI a
  mano y pasa a [pterm](https://pterm.sh), la librería de consola bonita
  por excelencia. Banner `SHIN-MD` en letras grandes (BigText) con
  degradado sakura→magenta solo en terminal con color; los logs ya son
  tags de herramienta (`INFO` magenta · `OK` verde · `AVISO` ámbar ·
  `ERROR` rojo) con la hora en gris; las cajas de vinculación, tu código y
  el panel de arranque se dibujan con `DefaultBox` y **envuelven el texto
  al ancho real de la terminal**: en un móvil en vertical ya nada se sale
  de la pantalla, sea cual sea su columna. El spinner «conectando with
  WhatsApp…» gira mientras se enlaza. En Windows sin color y en pipes, todo
  degrada solo a texto plano (NO_COLOR incluido).
- **0.19.0** — **la cara**: el bot ahora se ve como el JS. Banner marca
  Shin (`S H I N — M D / 反魂`, versión · runtime · plataforma) en cuanto
  arranca; los logs de consola pasan de slog crudo a la forma
  `09:28:24  mensaje` con el icono coloreado (✓ verde, ⚠ ámbar, ✕ rojo) —
  al archivo rotativo va el formato completo, sin ANSI; y al conectar se
  dibuja la caja `┌ SHIN-MD-GO … ┐` del JS con ● ACTIVO · pid, cuenta,
  comandos · prefijos, disco/ram del equipo (`syscall.Statfs`/`Sysinfo`,
  permitidas por el seccomp de Android) y la URL del panel web.
  `NO_COLOR=1` y `TERM=dumb` la dejan en texto plano; Windows compila
  (vitales por build-tag). Paquete nuevo `internal/consola` con tests de
  ancho visible (los códigos de escape no ocupan pantalla).
- **0.18.1** — hotfix del día: el cherry-pick dejó marcadores «<<<<<<<» sin resolver en la cabecera de shin-go.sh (script dash-roto); mismo contenido, cero marcadores.
- **0.18.0** — **el arranque del JS sabor calcado**. El instalador ya no te
  deja un «ahora ejecuta shin»: ARRANCA el bot por `exec` (el bot pasa a ser
  el proceso, Ctrl+C lo para, no hay padre zombie) y, si no hay sesión, el
  propio binario se detiene y **te pregunta tu número de WhatsApp** —lee la
  terminal real detrás aunque vengas de un `curl | sh`, igual que
  `leerDeTerminal` del JS—, te dibuja el panel «vinculación» y, tras pedir el
  código a WhatsApp, el panel «tu código» con el `XXXX-XXXX` en grande y los
  cuatro pasos. La decisión es una función pura portada 1:1
  (`PlanDeVinculacion`: nada/usar/preguntar/salir), con `NormalizarTelefono`
  que aplica las reglas del JS (México `52…`→`521…`, Argentina `54…`→`549…`,
  basura fuera). `--code` solo re-vincula; `PAIRING_NUMBER` para servicios;
  y **el QR se retira de la consola**: en el móvil donde corre el bot nadie
  podía escanearlo, era una trampa honesta. Sin terminal ni número no se
  arranca a ciegas: dice la orden exacta y sale. El instalador acepta
  `--no-arrancar` y `--code` en pasarela, como el del JS. Paquete
  `qrterminal` fuera de go.mod.
- **0.17.9** — Termux: **compilación en proot serializada contra el SIGSYS.
  Cuarta barrera, del *compilador* esta vez: con DNS y TLS ya resueltos,
  `go build` explotaba en `syscall.faccessat2 → os/exec.LookPath` (stack
  de `cmd/go` pura): el CLI busca su herramienta interna desde gorutinas
  del build, y los kernels Android 13+ matan cualquier `faccessat2` con
  SIGSYS antes de poder caer a `faccessat`. El instalador ya no gasta el
  segundo intento en `netdns=cgo` (probado inerte en go1.27.3): salta a
  **proot + `GOMAXPROCS=1`**, que serializa esas llamadas hasta dejarlas
  inocuas — solo posible porque la cache modular quedó llena en el build
  anterior (no hay red dentro de este proot). Si aún falla, plan B: proot
  cómodo.
- **0.17.8** — Termux: **TLS raíz resuelto**. Después del DNS vino su gemelo:
  «x509: certificate signed by unknown authority» en cada HTTPS. Con
  `CGO_ENABLED=0`, crypto/x509 solo mira las rutas clásicas de Linux
  (`/etc/ssl/certs/…`) que Android no tiene. El instalador asegura el paquete
  `ca-certificates` y exporta `SSL_CERT_FILE=$PREFIX/etc/tls/cert.pem` para
  el toolchain; y el bot, en runtime, fija esa misma env por código
  (`red.ArreglarCertificadosAndroid`) si el usuario no la fijó antes — sin
  esto, aunque arrancara, WhatsApp le diría lo mismo en cada conexión.
- **0.17.7** — Termux: **DNS que funciona de verdad, en dos niveles**.
  El parche del toolchain (que lee `$PREFIX/etc/resolv.conf`) no muerde en
  todas las builds de go1.27.x — visto en un teléfono real con el archivo
  ya creado y el DNS yéndose a `[::1]:53` igual. (1) Runtime: el bot lleva
  ahora su propio resolvedor —`red.ArreglarDNSAndroid` instala un
  `net.DefaultResolver` puro-Go que toma los nameservers de
  `$PREFIX/etc/resolv.conf` (editable por ti) o 8.8.8.8/1.1.1.1, solo cuando
  `/etc/resolv.conf` no nombra ninguno; sin esto el bot arrancaba pero jamás
  hablaba con WhatsApp. (2) Build: si el `make build` muere bajando módulos,
  el instalador reintenta solo con `GODEBUG=netdns=cgo` (bionic/netd) y luego
  con `proot -b …resolv.conf:/etc/resolv.conf`. Investigación multilingüe
  (EN/RU/ZH) detrás: termux/termux-packages#29042 y yetone/magpie#2872.
- **0.17.6** — Termux: el instalador crea `$PREFIX/etc/resolv.conf` (si no
  existe) con 8.8.8.8/1.1.1.1. Motivo: el resolvedor puro de Go —el del
  toolchain y, con `CGO_ENABLED=0`, también el del bot— no lee las
  propiedades DNS de bionic como los binarios C; sin ese archivo toda la
  red de Go muere con «lookup … on [::1]:53: connection refused» aunque
  pkg/git/curl funcionen. Nunca pisa una configuración existente.
- **0.17.5** — **Termux: SIGSYS por `faccessat2`, reparado en build y
  runtime** (reporte desde un teléfono real). Varios kernels de Android
  vetan esa syscall por seccomp y responden matando el proceso, sin error
  capturable. (1) Build: al detectar Termux el Makefile compila con
  `CGO_ENABLED=0 -buildvcs=false` — el módulo es Go puro; gcc y la estampa
  VCS eran las dos búsquedas que explotaban. (2) Runtime: el bot ya no usa
  `exec.LookPath` en ninguna parte — el nuevo `utilidades.BuscarBin` recorre
  el PATH con `os.Stat` (syscall permitida) y todos los `exec.Command`
  reciben ruta absoluta, que es cuando Go no repite la búsqueda por dentro;
  sin esto `.yt`, `.sticker` o `.ex` habrían tumbado el proceso entero en
  esos teléfonos. (3) Instalador: yt-dlp llega vía `pkg install yt-dlp` en
  Termux (el standalone de GitHub es glibc y no corre bajo bionic).
- **0.17.3** — instalador más listo en Termux: Go llega vía `pkg install
  golang` (el tar de go.dev es GOOS=linux y en Android tropieza con DNS y
  los certificados del sistema) y se comprueba `make` (`pkg/apt/apk` según
  toque). Mensaje honesto del arranque automático: requiere la app
  Termux:Boot.
- **0.17.2** — hotfix de dependencias: el `go.mod` fijado no compilaba en una
  máquina limpia (el resolver de Go trataba varias versiones fijadas —x/sys
  v0.48, sqlite v1.44.3, libc v1.67.6— como si no proveyeran sus paquetes;
  nuestras máquinas de desarrollo no lo veían por caché caliente). Un
  `go mod tidy` con el toolchain real sube a x/sys v0.50, sqlite v1.62,
  libc v1.81.1… y la suite entera (16 paquetes + race) sigue en verde.
  Encontrado ejecutando el instalador público en una máquina sin caché.
- **0.17.1** — hotfix del instalador: `go.dev/dl/<tarball>.sha256` ya no sirve
  el hash en crudo (devuelve un HTML intersticial y la verificación abortaba,
  como debe ser). Las descargas de Go ahora van directas al CDN canónico
  `dl.google.com/go/`. Encontrado probando `curl … | sh` en una máquina limpia.
- **0.17.0** — **100% del catálogo**: las ~140 reacciones anime (shares.js)
  con waifu.pics y frases del JS; la economía del harem (.sell .buychar
  .trade .robwaifu .giveallharem…); los paquetes de stickers en SQLite (en RAM
  se perdían); el resto de perfil/grupos/fun/owner; `.adminonly .rpg .slut
  .einfo .math`; buscadores (.imagen .mediafire .gdrive .apk .ytsearch); los
  booru con candado `.nsfw on`; `.ex` del owner (30 s, 3400 runas);
  efectos de audio por ffmpeg; y el **test de paridad**: los 640 nombres del
  JS están en Go o justificados, y CI lo exige. Qué no entró y por qué:
  **[docs/NO_PORTADO.md](docs/NO_PORTADO.md)**.
- **0.16.0** — bloque B6.1 + operación: **`shin-go.sh`** (instalación en un
  comando estilo shin.sh, con toolchain Go verificado por SHA-256) y los
  sociales `.yt` `.ytmp3` `.tiktok` `.ig` `.fb` `.x` como capa honesta sobre
  yt-dlp: si falta, enseña a ponerlo; si YouTube pide PoToken, lo explica
  en vez de fingir. Tope 100 MB / plazo 5 min. Auditoría de nov-2026:
  YouTube bloquea datacenter y exige PoToken — un scraper Go propio sería
  prometer humo.
- **0.15.0** — bloque B6.0: **música con licencia**. `.play` busca en
  Audius (audio completo libre), Deezer e iTunes (previews) a la vez y
  ordena: descargable delante; `.escucha` da el fragmento oficial de 30 s.
  Se quitó el patio de yt-dlp del JS: aquí solo se baja de donde se puede,
  con tope de 24 MB, solo-audio y un semáforo de 2 bajadas por chat. Detalle
  en **[docs/MUSICA.md](docs/MUSICA.md)**.
- **0.14.0** — bloque B7: **panel web y operación real**. `/health` `/queue`
  `/memoria` `/metrics` por HTTP en loopback (express → `net/http` pelado),
  con el failsafe del JS (LOOPBACK=0 sin clave vuelve a 127.0.0.1), Basic
  auth comparada por SHA-256 en tiempo constante y candado de 10 fallos/5
  min. El log por fin va a archivo de verdad: `data/shin.log` con rotación
  propia de 4 MB × 3 (una dependencia menos), y `deploy/shin.service` para
  que systemd lo reinicie sin PM2. Detalle en **[docs/PANEL.md](docs/PANEL.md)**.
- **0.13.0** — bloque B5.2: **utilidades con API ajena, sobrias**. `.tr` con
  dos proveedores encadenados y el detector de idioma propio del JS (28
  idiomas, alias y todo), `.tts` con motor estable en vez del WebSocket
  frágil de Edge, `.acortar`, `.crypto`, `.letra`, `.githubstalk`,
  `.gitclone` y `.tourl`; `.hd` y `.carbon` dejan de subir tus cosas a webs
  de terceros y pasan a ser **locales** (enfoque propio y renderer de código
  con cero dependencias nuevas); `.ia`/`.ai` con `GEMINI_KEY` — el nombre
  `ai` se recupera del placeholder `.rich` para la IA de verdad, como en el
  bot de JS. `.benchdl` se queda en B6: medía el descargador que aún no
  existe. Detalle en **[docs/UTILIDADES2.md](docs/UTILIDADES2.md)**.
- **0.12.0** — bloque B5.1: **utilidades sin API ajena**. `.qr` (el QR se dibuja
  dentro del binario: el JS lo pedía a `api.qrserver.com`, así que sin internet
  el de antes no funcionaba), `.morse`/`.demorse`, `.recordar` + `.olvidar` con
  **recordatorios en SQLite** que sobreviven al reinicio (el JS usaba `setTimeout`
  y se los comía una caída), `.say`, `.encuesta` con **encuesta, quiz y voto
  múltiple nativos** (`PollCreationMessage` armado a mano porque el helper de
  whatsmeow no expone `PollType` ni `CorrectAnswer`), `.pfp`, `.wastalk`,
  `.readviewonce` y `.get` con filtro anti-SSRF. Detalle en
  **[docs/UTILIDADES.md](docs/UTILIDADES.md)**.
- **0.11.0** — bloque B7.0: **operación desde el chat**. `.health` (RAM,
  uptime, ffmpeg/yt-dlp, filas de la base, tope de la cola y últimos errores),
  `.runtime`, `.warmup`, `.infobot`, `.compat` (cuatro mensajes de prueba para
  saber qué no lee tu cliente), `.allmenu`, `.restart`, `.update`, `.subir`,
  `.cache`, `.shell` y `.setmenu`. `.restart` ya no puede dejar el bot apagado y
  `.shell` viene apagado. Detalle en **[docs/OPERACION.md](docs/OPERACION.md)**.
- **0.10.0** — bloque B3.1: **perfil, nivel y matrimonio**. `.profile`,
  `.level`, `.lboard`, la edición de la ficha (`.setdesc`, `.setgenre`,
  `.sethobby`, `.setbirth` y sus borrados), `.marry`/`.acepto`/`.divorce` y el
  AFK con recompensa. **Una sola curva de XP**: el JS tenía tres fórmulas para
  lo mismo y el nivel de `.level` no coincidía con el de `.profile`. La boda ya
  no se cierra sin permiso y el matrimonio se guarda en una transacción.
  Detalle en **[docs/PERFIL.md](docs/PERFIL.md)**.
- **0.8.1** — auditoría de concurrencia. Los mensajes se atienden en paralelo
  (4 gorutinas, cola con techo): antes, un `.sticker` de vídeo dejaba al bot
  sordo hasta un minuto porque whatsmeow entrega los eventos de uno en uno.
  Además: plazo de 45 s en las descargas, dos punteros globales a atómicos y
  `go test -race` limpio. Detalle en **[docs/CAMBIOS.md](docs/CAMBIOS.md)** §32–35.
- **0.8.0** — bloque B5: **stickers y medios**. `.sticker` (con formas y
  efectos), `.toimg` y `.stickerinfo`, todo en Go puro salvo el vídeo. El
  paquete EXIF se escribe a mano (sin `node-webpmux`) y la conversión a vídeo es
  **local** (el JS subía el sticker a ezgif.com). Detalle en
  **[docs/STICKERS.md](docs/STICKERS.md)**.
- **0.7.0** — bloque B4.1: **juegos de azar y de riesgo**. `.coinflip`,
  `.slot`, `.roulette`, `.casino`, `.ppt` y las salidas que dan dinero
  (`.pescar`, `.cazar`, `.minar`, `.mazmorra`, `.aventura`, `.ritual`,
  `.crime`, `.robar`, `.cofre`) más `.heal`/`.pocion`. Las probabilidades se
  recalcularon: la ruleta del JS acertaba el 24 % pagando ×5 y el casino tiraba
  dados cargados contra el jugador. Detalle en **[docs/JUEGOS.md](docs/JUEGOS.md)**.
- **0.6.0** — bloque B4: **administración de grupos**. Advertencias con motivo,
  autor y fecha (y expulsión automática al llegar al límite), `.kick`,
  `.promote`/`.demote`, abrir/cerrar, enlace y su restablecimiento, menciones
  masivas, datos del grupo, **bienvenida y despedida** con marcadores,
  `.antistatus` y **conteo de actividad** propio. Las expulsiones en lote del JS
  no se portaron: solo listan. Detalle en **[docs/GRUPOS.md](docs/GRUPOS.md)**.
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
