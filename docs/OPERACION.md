# Operación y estado · bloque B7.0

_Los comandos con los que se cuida el bot desde el propio WhatsApp ·
10-nov-2026 · versión 0.11.0_

En el bot de JS estos comandos eran un adorno: el proceso se caía, lo
reiniciabas desde Termux y mirabas el log a mano. En un binario Go el proceso no
se cae casi nunca, así que la información de estado pasa a ser **la única forma
de saber si está sano** sin entrar por SSH. Se leen desde el móvil, en un chat
contigo mismo.

---

## 1. Qué comando hace qué

| Comando | Alias | Quién | Qué cuenta |
|---|---|---|---|
| `.health` | `salud`, `statsbot`, `sistema` | todos (el detalle, solo el dueño) | Conexión, uptime, RAM, ffmpeg/yt-dlp, filas de la base, tope de la cola, últimos errores |
| `.health clear` | — | dueño | Vacía el historial de errores recientes |
| `.runtime` | `uptime` | todos | Tiempo activo, memoria, cola del día, conexión |
| `.warmup` | `tope`, `limite` | dueño | Tope diario del calentamiento anti-ban, con barra |
| `.infobot` | `botinfo` | todos | Ficha: nombre, versión, prefijos, dueños, número de comandos |
| `.compat` | `diagnostico` | dueño | Manda cuatro mensajes de prueba (texto, botones, imagen, formato) |
| `.allmenu` | `allcmds`, `listatotal` | todos | Todos los comandos en una lista plana |
| `.restart` | `reiniciar` | dueño | Reinicia **solo si hay quien lo vuelva a levantar** |
| `.update` | `fix` | dueño | `git pull --ff-only` y aviso de recompilar |
| `.subir` | `upload`, `escribir` | dueño | Escribe un archivo del bot desde el chat |
| `.cache` | `clearcache` | dueño | Ve y limpia los temporales |
| `.shell` | `r`, `sh` | dueño | Comandos del sistema (**apagado por defecto**) |
| `.setmenu` | `menuvariant`, `menustyle` | dueño | Estilo del menú (v1, v2, v4) |

---

## 2. Los errores recientes existen otra vez

En el JS había un módulo de diagnóstico y el dueño podía preguntar «¿qué se
rompió en la última hora?» por WhatsApp. En el port, los errores solo iban al
log del sistema: si estás en el móvil, no te enteras hasta abrir Termux.

Ahora `internal/xlog` guarda un **anillo de los últimos 64 avisos y errores**
(`Warn` y `Err` lo alimentan solos, sin tocar un solo `c.Log.Warn` del resto del
código). `.health` enseña los cinco últimos al dueño y solo el contador a los
demás.

Decisiones que importan:

- **No es un log.** Vive en memoria y se pierde al reiniciar; el log de verdad
  sigue en el archivo. Sirve para «¿qué pasó hace un rato?», no para auditar.
- **Está acotado.** Si algo falla en bucle, el anillo no crece: guarda 64 y
  tira los viejos. El contador total (`ContarErrores`) sí sigue sumando.
- **Los mensajes se recortan a 300 caracteres**: en WhatsApp tampoco caben, y
  una traza de Go ocupa media pantalla.

---

## 3. `.restart`: la diferencia entre reiniciar y matar el bot

El `.restart` del JS tenía dos caminos y uno de ellos dejaba el bot apagado para
siempre: con `npm start` no hay supervisor, así que hacía `process.exit(0)` y
ahí se acababa todo.

Aquí se pregunta primero **quién nos vuelve a levantar**:

| Situación | Camino | Qué hace |
|---|---|---|
| Hay `INVOCATION_ID` (systemd) o `SHIN_SUPERVISOR=1` | `systemd` | Avisa, espera 3 s y sale con 0; el supervisor lo relanza |
| Nadie nos sostiene, pero el dueño puso `SHIN_REINICIO=exec` | `exec` | Relanza el propio binario y luego sale |
| Nadie nos sostiene y no hay `SHIN_REINICIO` | `sin-salida` | **No hace nada y lo dice**: mejor un bot viejo que un bot apagado |

`caminoDeReinicio()` es una función pura y tiene prueba: los cuatro casos están
cubiertos sin reiniciar nada.

En el camino `exec`, si el hijo no llega a arrancar (binario borrado, permisos),
el padre **no sale**: se queda vivo. Es la misma lección del `vigia.mjs` del JS.

---

## 4. `.subir`: el candado anti-escape

Escribe un archivo del bot desde el chat, sin abrir Termux:

```
.subir notas.txt | hola mundo
.subir cmds/ejemplo.go | package cmds
```

El comando es solo del dueño, pero **el mensaje puede venir de una cuenta
comprometida**, así que la ruta se valida en `rutaSegura()`:

| Intento | Resultado |
|---|---|
| `notas.txt`, `cmds/x.go`, `docs/a/b.md` | ✅ dentro del bot |
| `cmds/../notas.txt` | ✅ se resuelve y sigue dentro |
| `../fuera.txt`, `a/../../b` | 🚫 sale de la carpeta |
| `/etc/passwd` | 🚫 rutas absolutas, no |
| `.git/config`, `sub/.git/hooks/…` | 🚫 cualquier componente `.git` |
| `nota\x00.txt` | 🚫 byte nulo |
| `…/bot-otro/x` (carpeta hermana con el mismo prefijo) | 🚫 no se confunde por «empieza por» |

Las cinco filas de la tabla son un caso de prueba cada una
(`TestRutaSeguraRechazaEscapes`, `TestRutaSeguraNoConfundePrefijos`).

Se escribe con permisos `0640` y se crean las carpetas que falten. Si el archivo
es `.go`, el mensaje de confirmación recuerda que hay que recompilar: un `.go`
nuevo no cambia nada hasta que se construye el binario.

---

## 5. `.shell`: encendido a propósito, nunca por descuido

El `.r` del JS ejecutaba cualquier cosa. Aquí el comando existe, pero está
**apagado**: sin `OWNER_SHELL=1` en el `.env` solo contesta que está apagado y
cómo encenderlo.

Por qué así: cualquiera que consiga la cuenta del dueño —o el teléfono— pasa de
«puede escribir mensajes» a «puede ejecutar comandos en el servidor». Es una
puerta que se abre a mano, no una que venga abierta.

Cuando está encendido: `sh -c`, **20 segundos de tope**, salida recortada a 3000
caracteres y el error se enseña con su código.

---

## 6. `.cache`: qué se borra y qué no

Los temporales viven en `StateDir` (`data/` por defecto): `tmp`, `media` y
`downloads`. Se puede ver cuánto ocupan y vaciar uno o todos.

- **La carpeta no se borra, se vacía.** Si se elimina el directorio, el
  siguiente envío falla hasta que alguien se acuerde de crearlo.
- **Los enlaces simbólicos no se siguen**: un enlace a `/` convertiría el
  conteo de tamaño en un recorrido del disco entero.
- **La sesión de WhatsApp y la base de datos no se tocan**: están fuera de esas
  tres carpetas a propósito.

---

## 7. `.setmenu`: tres variantes de verdad, no cuatro de mentira

El JS tenía cuatro estilos: tarjeta interactiva, banner con readmore, vídeo en
bucle y ficha compacta. Aquí se ofrecen las que este bot **sabe pintar**:

| Variante | Qué es |
|---|---|
| `v1` (por defecto) | Tarjeta con botones nativos y categorías |
| `v2` | Todo en un mensaje de texto, sin botones (funciona en cualquier cliente) |
| `v4` | Ficha compacta: cabecera, cola del día, uptime y categorías con su número |

`v3` —cabecera de vídeo— no se ofrece porque el bot no genera vídeo: aceptarla y
no hacer nada sería peor que decir que no existe. Si algún día hay `ffmpeg` en
todas las instalaciones, se puede añadir.

La variante se guarda en `settings` con una clave fija (`bot:ajustes` →
`menu_variant`), no por persona: el menú es del bot, no de quien escribe.

---

## 8. `.health`: lo que de verdad se mide

| Dato | De dónde sale |
|---|---|
| Conexión | `wa.Client.LoggedIn()` |
| Uptime | `StartedAt` del cliente |
| RAM del sistema | `/proc/meminfo` (`MemTotal` − `MemAvailable`) |
| RAM del bot | `/proc/self/status` → `VmRSS`; si no hay `/proc`, `runtime.MemStats.Sys` |
| ffmpeg / yt-dlp | `exec.LookPath` (no se ejecutan para saberlo) |
| Personas, chats, advertencias | `COUNT(*)` en SQLite (no cargando el JSON entero como el JS) |
| Tamaño de la base | El archivo **más** su `-wal` y `-shm`: el WAL puede ser más grande que la base |
| Cola del día | `Queue.Snapshot()`: enviados, tope y si el número está en modo lento |
| Errores | El anillo de `internal/xlog` |

Si `/proc` no existe (contenedor raro, otro sistema), se dice «sin dato» en vez
de un cero que parecería una alarma.

---

## 9. `.warmup`: el tope que antes era invisible

Cuando se alcanza el tope diario del calentamiento, el bot **deja de contestar
los comandos normales** (`.play`, `.sticker`…) mientras `.menu` y `.ping` siguen
funcionando. En el JS el único rastro era una línea en el log.

`.warmup` enseña el uso del día con barra, el reinicio del contador y si el
número está en modo lento. Es prioritario: se responde incluso con el tope
alcanzado, que es justo cuando hace falta.

El contador se reinicia a las **00:00 UTC**, no a medianoche local (el JS usaba
`toISOString`). Se dice tal cual para que nadie espere a la hora equivocada.

---

## 10. `.compat`: medir en vez de adivinar

Cuando un cliente no entiende un tipo de mensaje, WhatsApp no da error: lo
cambia por «Recibiste un mensaje que no es compatible con tu versión». Del lado
del bot todo salió bien, así que el log no dice nada.

`.compat` manda cuatro mensajes numerados —texto, botones, imagen y formato— y
luego el resumen. El que aparezca como «no compatible» es el culpable. Es solo
del dueño y solo en privado, para no llenar un grupo de pruebas.

La imagen de la prueba 3 es un PNG de 1×1 **incluido en el binario**: si la red
falla, el diagnóstico no.

---

## 11. Verificación

```
go build ./... && go vet ./... && gofmt -l .   → limpio
go test ./...                                  → 310 pruebas
go test -race -count=1 ./cmds ./internal/xlog ./internal/store → verde
```

| Archivo | Qué cubre |
|---|---|
| `internal/xlog/errores_test.go` | el anillo guarda, cuenta, no crece sin fin, recorta y se limpia |
| `internal/store/estadisticas_test.go` | conteos reales, tamaño con WAL, rutas imposibles |
| `cmds/owner_test.go` | candado anti-escape (9 rutas malas, 5 buenas, prefijos), decisión de reinicio, barras, tamaños legibles, medir/vaciar caché, registro y `OwnerOnly`, variantes de menú |

Los comandos que hablan con WhatsApp (`.restart` de verdad, `.compat` contra un
teléfono concreto) se prueban a mano antes de publicar, como el resto.

---

## 12. Lo que no se porta

- **`.ex` / `.e`** (ejecutar JavaScript dentro del bot): en un binario Go no hay
  equivalente, y meter un intérprete para eso sería peor que no tenerlo. El
  `.shell` con `OWNER_SHELL=1` cubre el caso de «quiero probar algo rápido».
- **`.ex` como recarga en caliente de comandos** (`.update` del JS recargaba los
  módulos con `import(?update=…)`). En Go los comandos se registran al arrancar:
  el flujo pasa a ser `.update` → recompilar → `.restart`, y el mensaje del
  comando lo dice en vez de fingir que recargó.
- **`.subircookies`** queda para el bloque de descargas (B6): es el mismo
  `.subir` con validación de que el archivo parece un `cookies.txt` de YouTube,
  y depende de que exista el descargador.
