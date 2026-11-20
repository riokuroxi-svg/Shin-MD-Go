# Cambios 0.23.0 — la consola se parece a la del otro bot (y manda sola en un VPS)

_20-nov-2026 · portada compacta, pantalla en el búfer alternativo, `shin start·stop·status·logs`_

## De dónde parte esto

La 0.22.0 trajo el menú y la pantalla viva, pero se quedó a medias en lo que de
verdad importa en una terminal de móvil. La respuesta al probarla fue clara:

```
No me gusta cómo ha quedado.
En un VPS no puedes poner control c — es algo difícil.
En mi otro bot funciona diferente y se ve mejor en la consola.
```

Tres cosas, y las tres se pueden señalar con el dedo:

1. **El arte grande estorba.** El banner de letras y el código de vinculación
   dibujado a bloques ocupaban media pantalla de Termux, partían el borde en
   cuanto la ventana tenía menos de ~62 columnas y no se podían seleccionar con
   el dedo para copiar el código.
2. **Las barras `█▓▒░` se ven como manchas** con la fuente del móvil y
   descuadran el ancho de la caja.
3. **Todo el flujo dependía del teclado**: `Ctrl+C` para parar, una tecla para
   cada cosa. En un VPS por ssh, con la sesión cayéndose a cada rato, eso no
   existe.

Así que en vez de inventar otra cosa se copió el modelo del otro bot
(`Shin-MD` en JS, v3.7.9): ancho de casa de 58 columnas, bordes de una sola
columna, el estado arriba fijo y el registro del bot debajo, y el código
enseñado con letras separadas —no con arte— para no confundir la `R` con la `2`.

## 1. La portada: seis líneas

```
  ❀  ─────────────────────────────────────────

  S H I N - M D  ·  whatsmeow
  反魂  ·  bot de WhatsApp multi-device

  ─────────────────────────────────────────────
  versión 0.23.0  ·  go1.27.4  ·  termux
  hecho por riokuroxi-svg  ·  AGPL-3.0-only
```

Se quita `pterm.DefaultBigText` del arranque. Mismo contenido, seis renglones, y
en cualquier ancho.

## 2. El código de vinculación, para copiarlo con el dedo

```
╭ tu código ─────────────────────────────────────────────╮
│                                                        │
│                escríbelo en el teléfono               │
│                                                        │
│                   R 3 R Y - 7 K A 2                    │
│                                                        │
│     caduca en ~60 s · si expira, vuelve a arrancar     │
│                                                        │
╰────────────────────────────────────────────────────────╯
  cuenta: 380970546112
  1. abre WhatsApp en ese teléfono
  2. Ajustes › Dispositivos vinculados
  3. Vincular un dispositivo › con número
  4. teclea el código de arriba
```

Cuatro filas menos que el banner de letras, la mitad de alto, y el código es
texto seleccionable. La paleta pasa a ser la de la casa en HEX (`#F28FB4`
sakura, `#9B7BD4` lila, `#5FC6A9` menta, `#E0A34E` ambar) medida sobre el ancho
**visible**, no sobre los bytes: los escapes de color antes contaban como
caracteres y las líneas salían el doble de anchas de lo que medían.

## 3. La pantalla del bot

Cabecera fija arriba, registro debajo, en el **búfer alternativo** (`\e[?2026h`
+ `\e[H` + `\e[K\r\n` por línea + `\e[J`, salida sincronizada para que no se vea
el barrido):

```
╭ shin · v0.23.0 · en línea ─────────────────────────────╮
│ conexión                                   ● conectado │
│ encendido                                       1m 48s │
│ cuenta                                            Tula │
│ comandos                      213 · prefijos [. / # !] │
│ cupo de hoy                      0/157 no prioritarios │
│ mensajes                   0 recibidos · 0 respondidos │
│ cola                                       0 en espera │
│ disco                                      89M de 991M │
│ memoria                                      1.4G/1.9G │
│ panel web                        http://127.0.0.1:5560 │
╰────────────────────────────────────────────────────────╯
──────────────────────────────────────────────────────────
◐ 22:36:08  Cargando plugin: anti-llamada
◐ 22:36:08  216G libres en /home
! 22:36:09  no se pudo abrir el panel web: puerto ocupado
  q salir · e registros · r reconectar · ? ayuda
  para parar: q · Ctrl+C · o «shin stop» desde otra sesión
```

Por qué así y no «subir N líneas y borrar» (lo que hacía la 0.22.0):

- se repinta **cada segundo** y en cuanto llega un aviso, así que un `clear`, un
  `Ctrl+L`, girar el teléfono o que el bot imprima algo suelto no lo descuadra:
  al refresco siguiente vuelve a estar exacto;
- al salir, la terminal del usuario **recupera su scrollback intacto** — el bot
  no contamina el historial con 400 cuadros;
- cada línea va truncada al ancho real: una línea que se sale hace scroll y
  rompe el encuadre entero, que era el fallo que se veía en Termux.

Las barras de uso (`▓░`, `█`) salen del sitio del que nadie las pidió y entran
en su lugar filas `etiqueta … valor` alineadas a la derecha: mismo dato, seis
filas menos y sin glifos que la fuente del móvil no sabe dibujar.

Teclas, y ya está (no hay que aprenderse nada para empezar):

| tecla | qué hace |
|-------|----------|
| `e`   | bajar al registro (salir del panel) / volver con la misma tecla |
| `v`   | volver al panel |
| `↑` `↓` | releer el historial hacia atrás, sin salir del panel |
| `r`   | reconectar el socket de WhatsApp (la sesión no se toca) |
| `?`   | ayuda en el pie |
| `q`   | parar, igual que `Ctrl+C` |

## 4. Sin teclado no se promete teclado

El pie y los mensajes cambian según dónde corre el bot, porque «pulsad Ctrl+C»
en un servicio es una mentira:

- **con terminal** → `para parar: q · Ctrl+C · o «shin stop» desde otra sesión`
- **sin terminal** (systemd, pm2, docker, `nohup`, `> log.txt`) →
  `sin teclado (servicio): «shin stop» o el gestor de servicios`

Y en ese segundo caso **no se dibuja nada**: el panel no entra en el búfer
alternativo y el arranque suelta el estado como líneas de texto grepeables. Es
lo que hace que el mismo binario sirva en el móvil y en el servidor.

## 5. `shin start · stop · status · logs`

Lo que faltaba para el VPS. Cuatro órdenes, sin systemd y sin PM2:

```
$ shin start
  ✓ Shin-MD-Go arrancado en segundo plano (pid 28497).
  log      data/shin.log
  estado   shin status
  a partir de ahora puedes cerrar esta sesión

$ shin status
  ● en ejecución · pid 28497 · 1h 04m · 24M
  log: data/shin.log · 1.2M · actualizado hace 3s
  ── lo último del log ─────────────
  │ 22:48:27  conectado con WhatsApp

$ shin logs -f        # seguir el log, como journalctl -f
$ shin stop           # SIGTERM y esperar el cierre limpio
$ shin restart
```

Detalles que no son de adorno:

- el hijo se suelta con `setsid`, así que sobrevive a que se caiga la SSH;
- el PID vive en `data/shin.pid`; si el fichero miente (un `kill -9`, un
  reinicio de la máquina), `stop` y `status` lo dicen y **lo limpian**, no se
  quedan esperando ni dicen «en ejecución» de algo que murió;
- un proceso zombi no cuenta como vivo: sin esa comprobación `stop` se pasaba
  nueve segundos esperando a alguien que ya no existe;
- `logs` lee desde el final del fichero (un log con semanas no se traga entero
  para enseñar 50 líneas) y al seguirlo vigila el tamaño, porque un log rotado
  cambia de inode y quien persigue el descriptor deja de ver líneas sin avisar.

## 6. Lo que salió por el camino

Cosas que sólo se ven dibujando de verdad en un pseudo-terminal de 50 columnas:

- **la escalera.** El modo «tecla a tecla» apagaba `OPOST` (como hace
  `MakeRaw`), así que el terminal dejaba de traducir `\n` a `\r\n`. Todo lo que
  escribiera otro camino —el registro al bajar con `e`, las cajas, el log— salía
  con cada línea empezando donde acabó la anterior. Ahora `OPOST` no se toca.
- **dos relojes.** `Historial` añadía su hora a líneas que ya la traían
  (`22:35 ◐ 22:35:14 …`). El historial guarda la línea tal cual la pinta
  `Prefijo`.
- **un repintado a destiempo.** Al pulsar `e` podía caer un refresco entre la
  salida del búfer alternativo y la primera línea del registro, y pintaba el
  cuadro en la terminal normal. La entrada, la salida y el repintado comparten
  ahora un único candado de escritura.
- **`Parar()` colgaba.** El vigilante de `SIGWINCH` esperaba un canal que nadie
  cerraba (`signal.Stop` no lo cierra), así que el programa no terminaba y en
  un servicio systemd lo mataba con SIGKILL a los 90 s.
- **el envoltorio se comía las sangrías**, igual que le pasó al JS antes: al
  partir una línea dentro de una caja se perdían los espacios de la izquierda.
  Ahora la sangría se calcula, se quita para envolver y se devuelve a cada trozo.
- **`SUCCESS`/`ERROR` de librería** desentonaban con el resto (y ocupan en un
  log): los prefijos de pterm pasan a ser `✓ ✕ ! ◐` con los colores de la casa.

## Comprobarlo

```sh
make test              # gofmt, vet y todas las pruebas
go test ./internal/consola/ -run 'Marco|Pantalla|Pie|Banner' -v
```

En Termux, con el binario ya instalado:

```sh
shin --code            # la portada, el menú y el código, en 50 columnas
```

En un VPS:

```sh
./shin start --code 3412345678   # se puede cerrar la SSH enseguida
./shin status && ./shin logs -f
./shin stop
```

## Cómo se degrada cada cosa

| dónde | pantalla | color | teclado | cómo se para |
|-------|----------|-------|---------|--------------|
| Termux | panel cada segundo | 24 bits | `e v ↑ ↓ r ? q` | `q`, `Ctrl+C` o `shin stop` |
| ssh con terminal | panel cada segundo | 24 bits | igual | igual |
| systemd / pm2 / `> log.txt` | texto plano | ninguno (`NO_COLOR` incluido) | no hay | `shin stop`, `systemctl stop` |
| `shin start` | texto plano en el log | ninguno | no hay | `shin stop` |
