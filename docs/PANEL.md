# Panel web y operación — bloque B7

_14-nov-2026 · versión 0.14.0_

Puerto de `src/web/server.js` (express → `net/http` pelado) más las dos
piezas que faltaban para dejar el bot solo en un servidor: **log a archivo
con rotación** y **unidad systemd**.

## Endpoints

El panel arranca siempre en `127.0.0.1:PORT` (defecto 3000). Misma forma que
el JS:

| Ruta | Qué devuelve |
|---|---|
| `/` | nombre, versión, **huella anti-ban** (`shin-gauss-0.25-1200ms`), estado y `ts` |
| `/health` | conexión, uptime, cola (enviados hoy, cupo, modo lento), repartidor (workers, pendientes, descartados) y RSS en MB |
| `/queue` | la cola anti-ban en detalle con su regla de cálculo |
| `/memoria` | RSS real (`/proc` en Linux), heap, GC, goroutines |
| `/metrics` | proceso (pid, uptime, Go) + SO + motor, para graficar |

Cualquier otra ruta es **404** (el `/` del mux de Go lo caza todo: si no se
filtra, un typo te devuelve la ficha del bot como si existiera).

```sh
curl -s 127.0.0.1:3000/health
# {"ok":true,"state":"conectado","uptimeSec":7123,"queue":{"sentToday":37,
#  "budget":120,"slowMode":false,...},"dispatcher":{"workers":4,...},...}
```

## Seguridad (paridad con el JS más la reseña)

- **Failsafe**: `LOOPBACK=0` sin `PANEL_PASSWORD` → el panel vuelve a
  `127.0.0.1` aunque lo hayas pedido, y lo avisa en el log. Riesgo, cola y
  memoria no se enseñan a la red abierta.
- Panel expuesto ⇒ **Basic auth** con usuario fijo `admin`. La contraseña se
  compara por **SHA-256 en tiempo constante** (así no se filtra su longitud
  por temporización: siempre se comparan 32 bytes).
- **Candado anti fuerza bruta**: 10 fallos por IP → `429` durante 5 minutos,
  con `Retry-After`. Bloqueada la IP, ni con la contraseña buena entra.

```env
PORT=3000
LOOPBACK=1            # 0 = exponer; exige PANEL_PASSWORD
PANEL_PASSWORD=       # pon una clave larga si expones
```

## Log a archivo con rotación

Hasta ahora el log salía solo por stdout (el comentario «sigue en el archivo»
era una herencia del JS que no era verdad aquí). Ahora:

- **por defecto** se escribe también a `data/shin.log`, con rotación de
  **4 MB × 3 backups** (`shin.log.1` … `.3`; lo más viejo se borra);
- `LOG_FILE=off` lo apaga (con systemd, journald ya recoge stdout);
- si el archivo no se puede abrir (disco lleno, permisos), se avisa y el bot
  arranca igual con solo stdout: **el log es un extra, no una razón para no
  arrancar**;
- permisos `0600` en el archivo: los logs llevan números de teléfono.

La rotación es propia (~60 líneas, `internal/xlog/archivo.go`): una
dependencia menos para una función que cabe en una pantalla.

## systemd

`deploy/shin.service`: `Restart=on-failure`, cierre limpio con SIGTERM
(`TimeoutStopSec=10s` para que la cola de envío muera bien) y endurecimiento
(`NoNewPrivileges`, `ProtectSystem=full`, escritura solo en `data/`).

```sh
sudo cp dist/shin /opt/shin/shin && sudo cp .env /opt/shin/.env
sudo systemctl enable --now shin
journalctl -u shin -f   # el log, sin PM2 ni reinicios sucios
```

## Pruebas del bloque

- `internal/web`: failsafe con/sin contraseña, puerto absurdo → 3000, forma
  del JSON de los 5 endpoints, 404 en rutas desconocidas, 401 con y sin
  usuario correcto, y el candado (10 fallos → 429 incluso con clave buena).
  Todo con `httptest`: no se abre ningún puerto real.
- `internal/xlog`: rotación al pasar el tope, techo de backups (no existe
  `.4` cuando pediste 3) y tronco heredado que se rota al abrir.
