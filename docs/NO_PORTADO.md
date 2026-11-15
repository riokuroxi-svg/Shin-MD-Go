# Lo que no se porta del JS — y por qué

_14-nov-2026 · la cara B de la migración_

El test `TestParidadCatalogo` (en `cmds/paridad_test.go`) hace cumplir esto:
todo nombre de comando del JS está registrado en Go o está aquí, con su
motivo. Si falta en los dos lados, CI truena.

## NSFW suelto (sin candado) — NO

El JS tenía ~40 comandos de «reacciones» explícitas (`anal`, `violar`,
`bukkake`, `orgy`, `yaoi`…) que se servían sin ningún candado, en cualquier
grupo. La vista "violar" es contenido de agresión sexual: **no se porta
jamás**. Del resto, tres búsquedas sí se portan **con candado** (`.nsfw on`
en grupos, apagado de serie): `danbooru`, `gelbooru`, `rule34`.

Tampoco: `xnxx`, `xvideos` — sin API pública fiable; scraping porno roto cada
dos semanas es mentir.

## Sub-bots (`serbot` / `jadibot`) — no aplica 1:1

El sistema de sub-bots del JS (`code`, `join`/`unir`, `leave`, `logout`,
`reload`, `bots`): cada usuario abría **su propia sesión de WhatsApp** desde
la tuya. En whatsmeow eso es otra arquitectura (un proceso/sesión por
socket); cubrirlo no es port: es obra nueva y fuera del scope 1:1. Si se
quiere "un bot, varios números", eso es despliegue (repetir `shin-go.sh`),
no comandos.

## Downloader "a lo viejo" — ya decidido

| JS | Sustituto Go | Motivo |
|---|---|---|
| play.js (ytdl/yt-dlp) | `.play` (Audius/Deezer/iTunes con licencia) | PoToken 2026; además el JS fue siempre riesgo |
| play2, playsoundcloud, soundcloud | `.play` | sin descarga libre genérica en esas fuentes |
| spotify | `.escucha` (preview) | sin OAuth el audio completo no llega |
| benchdl | — | media los proveedores del JS; sin proveyedores no hay qué medir |

## Del dueño / del sistema del JS

- `setprefix`, `setowner`, `setcurrency`: eran cambios en vivo de la config del bot; en Go son del `.env` (más franco: sin estados sorpresa).
- `setbotname`, `setboticon`, `setbanner`, `setchannel`, `setlink`, `setusername`: cambiar el nombre/avatar/sitio del bot por comando de texto no lo cubre whatsmeow como el JS lo fingía (eran stubs del panel).
- `setstatus`, `setpfp`: pendiente de validar en whatsmeow (`SetStatusMessage` existe pero no probado su alcance); cuando se valide, se portan.
- `subircookies`: mandar credenciales de red por el chat a la base sin cifrado era un riesgo del JS: no.
- `deepseek`/`r1`/…: otra llave, otro proveedor; `.ai` cubre (y el JS lo compartía).
- Varia: `terminos` era texto del menu (ya está en `.menu`), `demo` un saludo, `kuro` un easter-egg sin función, `guardar` reemplazado por `.tourl`.

## Tests de B8 que quedan por hacer de la paridad

Esta es la paridad de **catálogo**. La otra mitad de B8 es la paridad de
**suite**: el JS tenía 126 archivos de prueba (no 161; el número 161 era de
mi propio README viejo y quedó mal citado). En Go hay ya tests por paquete
del motor (`internal/*`) y por comando puro. Lo que falta en detalle está en
`docs/MIGRACION.md` al final de B8.
