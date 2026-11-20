# Cambios 0.22.0 — la consola se vuelve dinámica (Termux y VPS por igual)

_20-nov-2026 · onboarding con menú, pantalla viva y plan B para el QR_

## Qué se buscaba

El arranque pedía cosas a gritos y en orden fijo: quien abría el bot en Termux
veía un texto que subía y subía, y en un VPS por ssh pasaba lo mismo pero
además había que saberse las banderas de memoria. La petición era concreta:

```
Introduce tu número ejemplo: "+52"
1 pide QR
2 Code paring
```

Es decir: preguntar, no exigir. Y que la pantalla dijera dónde está el bot sin
tener que relanzar nada.

## 1. Un menú de dos puertas

Al arrancar **sin sesión** y **con terminal**, ahora aparece esto (no antes:
si hay `--code`, `PAIRING_NUMBER` o una sesión guardada, se sigue sin
preguntar nada):

```
  ¿Cómo quieres vincular el bot?

  ❯ 1) Código de 8 letras  (recomendado)
      te lo da este bot y lo escribes en WhatsApp
    2) Código QR
      para escanear desde OTRO teléfono (útil en un VPS)

  ↑↓ mover · Enter aceptar · 1-9 atajo · q salir
```

* Flechas, `j/k`, `1`/`2` y Enter. Se acepta con el número sin mover el
  selector.
* Implementado a mano (`internal/consola/menu.go`) sobre `/dev/tty` +
  `golang.org/x/term`, **no** con el `InteractiveSelect` de pterm: ese usa el
  paquete `keyboard`, que abre su propio `/dev/tty` y se cae (o se traga la
  entrada) en cuanto algo no es una terminal limpia.
* Al elegir, el menú **se retira** y deja una sola línea (`❯ Código QR`). Antes
  quedaba el bloque viejo pintado encima de la respuesta.

Después, la pregunta del número, con reintentos y diciendo **qué** se esperaba:

```
  ❯ Tu número (ej. +52 1 55 1234 5678): +52 55 1234 5678
```

`NormalizarTelefono` sigue aceptando lo que la gente escribe de verdad (con
espacios, guiones, paréntesis) y solo pide país + número.

## 2. La pantalla viva

Un panel que se repinta **en su sitio**, con el pulso del bot:

```
  反魂 SHIN-MD v0.22.0   ● conectado
  encendido  0m 42s
  cupo hoy   ▓▓▓░░░░░░░░░ 7/20
  comandos   213 · prefijos [. / # !]
  recibidos  44 · 41 respondidos · 2 en cola
  disco      ▓▓▓▓░░░░░░░░ 366M
  ram        ▓▓▓▓▓▓▓▓▓▓░░ 1.7G/1.9G
  panel web  http://127.0.0.1:3000
  ┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈┈
  20:11  ◑ Comandos cargados: 213 únicos · 748 nombres · 5 hoo…
  20:12  ⚠ desconectado — reconexión automática en curso
  e panel · s estado · p cmd · l log · q salir
```

Teclas de un carácter, sin reiniciar nada (solo funcionan si hay terminal):

| tecla | qué hace |
|------|----------|
| `e` | oculta el panel y devuelve la consola a los logs de siempre |
| `v` | vuelve a poner el panel |
| `s` | resumen en una línea (útil para copiar y pegar en un grupo) |
| `p` | cuántos comandos hay y con qué prefijos |
| `l` | los últimos avisos |
| `q` | parar, igual que Ctrl+C |

Tres decisiones que valen la pena explicadas, porque son el precio de que se repinte sin romperse:

* **No se usa `pterm.Area`.** El área de pterm envuelve el texto al ancho del
  terminal y en 50 columnas (Termux en vertical) el panel salía duplicado y
  hecho polvo — verificado reproduciendo la salida con `pyte`. El panel ahora
  escribe línea a línea, cada una truncada a su **ancho visible** (con `…` si
  se quedó algo fuera) y sube/borra con `\033[<n>A\r\033[J`.
* **Los logs pasan por un teletipo** (`consola.Teletipo`): mientras el panel
  está visible no se sueltos a pantalla — se guardan en un historial circular
  y se enseñan *dentro* del panel. Soltarlos fuera es exactamente lo que
  duplicaba las líneas. Con `l` se ven los últimos 15 envueltos.
* **El cursor queda después del bloque, no encima.** Si viviera en la primera
  fila del panel, cada refresco borraría también la línea de arriba (el log
  recién escrito parpadeaba y desaparecía).

## 3. Ctrl+C vuelve a ser Ctrl+C

`Atajos` ponía el terminal en `term.MakeRaw`, y `MakeRaw` apaga `ISIG`: desde
ahí Ctrl+C deja de ser una señal y llega como el byte `0x03`. Peor aún: al
salir, `parar()` esperaba a un goroutine bloqueado en la lectura de teclas, así
que **el proceso no se cerraba nunca** y el terminal se quedaba en modo crudo.
Comprobado con un arnés de `pty` real, no a ojo.

Se cambió por `modoTeclas` (`internal/consola/teclas_unix.go`), que apaga solo
`ECHO` e `ICANON` y deja `ISIG` intacto; en Windows/macOS se cae al modo crudo
de `x/term` y el byte `0x03` se traduce a la acción `ctrl-c`, que también llama a
`stop()`. Y la espera del goroutine tiene techo (1 s): si no despierta, el bot
cierra igual.

Después de `Ctrl+C`, `stty -a` devuelve `isig icanon echo` — terminal limpio.

## 4. El QR ya no se miente a sí mismo

Un `ref` de WhatsApp son ~370 caracteres, que en QR son **69–81 columnas**. En
Termux en vertical (50) dibujarlo lo partía en dos líneas: un QR roto que no
escanea nadie, con la pantalla entera ocupada. Ahora, si no cabe, no se pinta
y se dice el número exacto + tres salidas:

```
  el QR necesita 69 columnas y tu terminal tiene 50:
    dibujarlo aquí lo partiría y no escanearía.
  tres salidas, la que te venga bien:
  1. ensancha la ventana (o gira el teléfono) a 71 columnas y vuelve a arrancar
  2. ábrelo con la galería:   termux-open data/vincular-qr.png
  3. o elige 1) Código de 8 letras, que no necesita escanear nada
```

El PNG lo escribe `consola.GuardarQRPNG` (stdlib `image/png`, 8 px por módulo,
4 módulos de zona de silencio) y **se comprueba en los tests módulo a módulo**
contra `rsc.io/qr`: un PNG bonito con la matriz mal no escanea y nadie se
entera. El archivo se borra al terminar la vinculación (de lo contrario queda
un código caducado que alguien escanea mañana).

## 5. Sin terminal, mismo bot (sin inventarse nada)

En systemd, Docker, un tubo o `ssh host ./shin > log.txt`:

* no hay menú ni panel ni lector de teclas: `Atajos` no se enciende y
  `Panel.Arrancar` no pinta ni un byte;
* el arranque imprime la orden exacta que hay que usar (`--code 521…` o
  `PAIRING_NUMBER=…`) y sale con error claro, en texto plano, sin ANSI;
* la caja estática de `PanelListo` solo se pinta cuando el panel vivo **no**
  va a aparecer: con los dos se duplicaba la información y en una pantalla de
  móvil el arranque entero se iba arriba con el primer refresco.

## 6. Arreglos de a lado (que eran bugs, no maquillaje)

* `readerDe`: el búfer compartido era **uno global**, así que un menú que leía
  de `/dev/tty` y una pregunta que leía de `stdin` usaban el descriptor
  equivocado. Ahora es un mapa por archivo (`internal/consola/menu.go`).
* Carrera de datos en `Panel.Arrancar`: el goroutine leía `p.parar`/`p.hecho`
  sin candado mientras `Parar()` los ponía a `nil`. Lo pilla `go test -race`.
* Envuelto de logs ANSI-aware: `pterm` no envuelve (corta el terminal, a mitad
  de palabra) pero sí sangra lo que va tras un `\n`. Con el estilo ya aplicado,
  medir sobre el texto con escapes salía el doble de ancho. Se pela el estilo,
  se mide limpio y se vuelve a pintar por línea (`consola.alinear`).
* `internal/xlog.New` decide «¿pinto bonito?» mirando si el writer es un
  `*os.File` en una TTY; con el teletipo en medio eso ya no era cierto, así que
  ahora reconoce el `EsTeletipo()`.
* `Makefile`: `VERSION` sincronizado a 0.22.0 y objetivo `linux` (x86_64 para
  el VPS) además de `termux`, con `make release` para los dos.

## Cómo se probó

No «en mi máquina»: con un `pty` de verdad (`python3 pty.fork`), pantalla de
50×24 y 90×30, **reproduciendo la salida en `pyte`** para ver lo que vería el
emulador (términos: duplicados, restos del dibujo anterior, líneas de más de
`cols`, teclas que no llegan). Cubierto:

* menú: flechas, atajo numérico, `q`, Enter; y que al aceptar no queden
  restos;
* panel: refresco cada 2 s sin duplicar, `e`/`v`/`s`/`p`/`l`/`q`, y que un log
  no rompa el bloque;
* `Ctrl+C` (señal real, no byte) → cierra en ~1 s y devuelve el terminal
  cocido;
* QR: dibujado entero a 90 columnas; rechazado con aviso + PNG a 50;
* sin TTY (`| less`, sin `script`): cero ANSI, cero bloqueo, texto plano;
* vinculación por código real: `WVF3-ENG5` saliendo ya del arreglo del 400.

Y los tests de siempre, en verde y sin carreras:

```
$ go test ./...            # 18 paquetes, todos ok
$ go test -race ./internal/consola/ ./internal/xlog/
```

## Qué NO cambia

* La API, los comandos, la base de datos ni la sesión: **quien ya estaba
  vinculado no vuelve a vincular nada**.
* El orden de preferencia: `--code`/`PAIRING_NUMBER` → sesión guardada →
  (si hay terminal) menú.
* El QR sigue siendo la segunda opción, no la primera: en el móvil donde corre
  el bot no hay cámara que lo escanee.
