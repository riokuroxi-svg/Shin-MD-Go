# Cambios 0.21.1 — el código de vinculación ya llega

_20-nov-2026 · arreglo del `400 bad-request` al vincular con número_

## El síntoma

Al arrancar sin sesión, el bot preguntaba el número, lo normalizaba bien
(`+52 1 55 1234 5678` → `5215512345678`)… y WhatsApp contestaba:

```
INFO    vinculando con el número 5215512345678.
ERROR   no llegó el código
ERROR   login: pedir código de emparejamiento: info query returned status 400: bad-request
```

Ningún código de 8 letras. Ningún número podía vincularse: el bot nunca
llegaba a arrancar. El resto del arranque (DNS, TLS, comandos, cola) sí
estaba bien; por eso el fallo parecía del teléfono o del número.

## Las dos causas (una visible, otra escondida detrás)

### 1. El nombre visible del compañero no era válido

`internal/wa/client.go` llamaba a `PairPhone` así:

```go
c.waClient.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Shin-MD (Go)")
//                                                                  ^^^^^^^^^^^^^
```

`PairPhone` lo documenta en su propio código fuente
(`go.mau.fi/whatsmeow/pair-code.go`):

> The client display name must be formatted as `Browser (OS)`, and only
> common browsers/OSes are allowed (the server will validate it and return
> 400 if it's wrong).

`Shin-MD (Go)` no cumple `Navegador (SO)`, así que WhatsApp rechazaba la
petición con **400 bad-request** antes de emitir ningún código. Es el mismo
400 que sufrieron Baileys (`Chrome (Baileys)`), mautrix-whatsapp y otros
puentes.

**Arreglo:** constante `wa.PairDisplayName = "Chrome (Linux)"`, el valor que
mautrix-whatsapp usa en producción. En «Dispositivos vinculados» el bot
aparecerá como *Chrome (Linux)*, que es exactamente lo que espera el
servidor; el SO es solo una etiqueta para su validación.

### 2. Se pedía el código antes de que el pre-login estuviera listo

`Connect()` vuelve en cuanto termina el *noise handshake*, **antes** de que
el servidor conteste el primer `<pair-device>`. Pedir el código en ese hueco
es la otra fuente clásica del mismo 400 (y de `websocket not connected`).

**Arreglo:** `GetQRChannel(ctx)` se registra **antes** de `Connect()`. Ese
canal espera internamente al primer `<pair-device>` y emite el primer QR;
ese primer evento es la señal de «pre-login listo» y solo entonces se llama
a `PairPhone`. El QR no se dibuja (en el móvil donde corre el bot no hay
cámara que lo escanee) y el canal se drena en segundo plano en cuanto se
sale de `Login`, para que un QR que caduca no lo llene ni pare el emisor.
Si en 45 s no llega señal, se pide el código igualmente y se deja constancia
en el log: mejor intentarlo que rendirse.

## Cómo se comprobó (no es teoría)

Con el mismo número, en la misma red, con el binario viejo y el nuevo:

| binario | resultado |
|---|---|
| 0.21.0 (antes) | `ERROR login: pedir código de emparejamiento: info query returned status 400: bad-request` |
| 0.21.1 (después) | código `4NH3-T2KW` emitido y panel «tu código» en pantalla |

Pruebas que quedan en el repo:

- `TestPairDisplayNameEsNavegadorYSO` — falla si alguien vuelve a poner un
  nombre libre en `PairPhone` (comprueba formato y que navegador y SO sean de
  los que WhatsApp acepta).
- `go build`, `go vet`, `gofmt -l .` y `go test ./...` limpios; el binario
  `GOOS=linux GOARCH=arm64` (Termux) vuelve a compilar.

## Qué NO se tocó

Ni el resto del protocolo, ni la sesión SQLite, ni la cola anti-ban, ni los
213 comandos. Un solo bloque de `Login` y una constante.

## Si ya tenías el bot vinculado

No hace falta re-vincular: la sesión guardada se sigue usando igual. Este
arreglo solo cambia el camino «no hay sesión → pedir código».
