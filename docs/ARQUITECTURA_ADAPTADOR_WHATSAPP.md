# Medición y migración gradual del adaptador WhatsApp

**Línea base:** 17 de noviembre de 2026 (sobre `0.20.0`).

## Método de medición

Se inspeccionaron imports reales de archivos Go con `go/parser`/`go/ast` (sin contar comentarios ni coincidencias de texto), se distinguieron archivos de producción y `_test.go`, y se contrastó el grafo de imports de paquetes de producción con `go list -e`. Los números de archivos cuentan una vez cada archivo que tiene el import; los paquetes son directorios Go distintos. El inventario de selectores diferencia los usos de `types.JID` de un import de `types` sin usos de JID.

`waProto` no es un import literal en el código. Los protos WhatsApp se importan bajo nombres como `waE2E`, `waCommon` y `waAICommonDeprecated`, desde rutas `go.mau.fi/whatsmeow/proto/...`. Los imports del runtime `google.golang.org/protobuf` se cuentan aparte.

## Línea base cuantitativa

| Dependencia directa | Producción | Pruebas | Observación |
|---|---:|---:|---|
| Paquete `go.mau.fi/whatsmeow` o subpaquetes | 31 archivos / 5 paquetes | 12 / 4 | Los paquetes de producción son `cmd/shin`, `cmds`, `internal/bot`, `internal/ui` e `internal/wa`. |
| Eventos externos `.../types/events` | 7 / 4 | 2 / 2 | En producción: `cmd/shin`, `cmds`, `internal/bot`, `internal/wa`. |
| Protos WhatsApp `.../proto/...` | 12 / 3 | 5 / 4 | En producción: `cmds`, `internal/ui`, `internal/wa`. Predomina `waE2E`. |
| Runtime Protobuf `google.golang.org/protobuf/...` | 8 / 3 | 4 / 3 | En producción: `cmds`, `internal/ui`, `internal/wa`. |
| Import de `go.mau.fi/whatsmeow/types` | 25 / 4 | 9 / 4 | Incluye JID, metadatos de grupos y otros tipos externos. |
| Selector explícito `types.JID` | 25 / 4 | 5 / 3 | Paquetes: `cmds`, `internal/bot`, `internal/ui`, `internal/wa`. |

### Imports directos por paquete de producción

El grafo (`go list -e`) mostró estas dependencias relevantes:

- `cmd/shin` importa directamente eventos externos.
- `cmds` importa directamente `waE2E`, `types`, `types/events` y `protobuf/proto`.
- `internal/bot` importa directamente `types` y `types/events`.
- `internal/ui` importa `binary`, `types`, protos WhatsApp y `protobuf/proto`.
- `internal/wa` concentra el cliente de whatsmeow, stores, eventos, tipos, protos y conversiones de red.

No todos esos imports son el mismo tipo de acoplamiento. Ejemplos relevantes del flujo de recepción: `Router.Handle` recibe `*events.Message`, parsea `evt.Message` (protobuf), y `bot.Context` expone el evento externo. Varios comandos vuelven a inspeccionar ese protobuf para citas, menciones o medios. Por otro lado, el parser de grupo en `cmds/grupos_avisos.go` recibe `*events.GroupInfo`. Envíos: `.encuesta` construye `waE2E.Message` en `cmds/utilidades.go`; la librería de tarjetas de `internal/ui` también genera protos directamente. `internal/wa` ya posee wrappers útiles de parsing/envío, pero no todos los caminos los atraviesan.

## Criterio para los pilotos

1. **Recepción primero:** definir un evento interno normalizado para chat, remitente, texto/command-id de botones, timestamp, menciones, cita y descriptor de medio. Las estructuras externas quedan privadas al adaptador y se convierten antes del router. Conservar un handle opaco para operaciones que necesitan los bytes de un medio; no publicar ni hacer escapar el protobuf. Migrar un comando real que utilice respuestas de botón/cita/medio, no solo un texto trivial.
2. **Envío después:** tomar un flujo real como la encuesta/quiz, definir una entrada interna pequeña que represente opciones, selección y respuesta correcta, y construir/enviar el proto dentro del adaptador con el mismo secreto y semántica actuales. Mantener `ui.Button`/fallback como contrato propio; no rediseñar todos los formatos de salida en esta etapa.
3. Mantener las rutas anteriores mientras se comparan conversiones y resultados; retirar cada camino externo solo después de probar equivalencia. No cambiar límites funcionales ni agregar una interfaz que duplique la API completa de whatsmeow.

## Comparación posterior (17 de noviembre de 2026)

Se repitió el mismo recorrido AST por el árbol Go completo. Los conteos de imports separan producción y `_test.go`; las referencias se cuentan por selectores AST y los comentarios no se consideran código ni se incluyen en los totales.

| Dependencia directa | Producción antes → después | Pruebas antes → después | Resultado |
|---|---:|---:|---|
| `go.mau.fi/whatsmeow` y subpaquetes | 31/5 → **13/2** | 12/4 → **7/3** | En producción queda concentrado en `internal/wa` e `internal/ui`; ya no lo importan `cmd/shin`, `cmds` ni `internal/bot`. |
| `.../types/events` | 7/4 → **2/1** | 2/2 → **1/1** | Producción únicamente en el adaptador. |
| Protobufs WhatsApp `.../proto/...` | 12/3 → **10/2** | 5/4 → **6/3** | La producción restante es `internal/wa` e `internal/ui`; suben pruebas de adaptador que verifican el protobuf construido. |
| Runtime `google.golang.org/protobuf/...` | 8/3 → **7/2** | 4/3 → **4/2** | Queda en la capa de presentación externa y el adaptador. |
| Import de `whatsmeow/types` | 25/4 → **8/2** | 9/4 → **3/2** | Conversión externa concentrada en `internal/wa`; `internal/ui` conserva su uso para claves de protocolo. |
| Selectores `types.JID`, `types.NewJID` y `*Server` (AST) | 25/4 → **8/2** | 5/3 → **2/2** | Total: 30/4 → **10/2** archivos/paquetes con referencias AST. |

Los totales comparables de archivos Go (producción + pruebas) para los imports principales quedan: whatsmeow **43/5 → 20/3**; eventos **9/4 → 3/1**; protos WhatsApp **17/4 → 16/3**; protobuf genérico **12/4 → 11/3**; `types` **34/4 → 11/2**. La cantidad de protos no baja tanto como otros imports porque sus constructores se probaron en el adaptador en vez de dejar las pruebas de payload en `cmds`. En el alcance de esta fase, `cmds` y `internal/bot` tienen **cero imports externos de producción**; `internal/bot/hooks_test.go` conserva una prueba de hook con fixtures Protobuf. Los imports de producción restantes se ubican en `internal/wa` e `internal/ui`.

### Flujos implementados y equivalencia comprobada

- **Recepción:** `Client.OnMessage` traduce a `wa.IncomingMessage` antes de llamar al dispatcher/router. El modelo transporta chat, remitente y alternativo, ID, timestamp, push name, flags, parsed text/botón, menciones, cita (ID/autor/texto/imagen/estado) y conserva el payload crudo únicamente en un campo privado para descargar medios. `internal/bot.Context` y los callbacks ya no exponen eventos/protobufs.
- **Consumidores reales:** los comandos de grupo leen cambios normalizados; `.pin`, la reclamación de gacha, anti-status, menciones, texto citado y medios usan campos/modelos internos. La foto de grupo se mapea a `wa.GroupInfo` y participantes `wa.JID` con teléfono/LID/roles, que son los metadatos que usan las decisiones de protección y permisos.
- **Envío:** `.encuesta`/quiz produce `wa.PollSpec`; el adaptador valida límites, construye las opciones y tipo de encuesta, mantiene el índice de respuesta correcta apuntando a la opción enviada y genera el secreto aleatorio de 32 bytes. `.rich` pasa `wa.RichCard`. El cliente whatsmeow se hizo privado al paquete, y `Connected`, `AccountName` y `OwnID` reemplazan accesos directos al cliente/store desde comandos y `cmd/shin`.
- **Pruebas añadidas/actualizadas:** normalización de metadatos reales, menciones/cita/medio opaco; equivalencia del contrato normal/quiz/multi y test de construcción Protobuf dentro del adaptador; pruebas de JID/dispatcher migradas a modelos internos.

La sustitución de firmas `JID` y algunos accesos a medios se extendió por archivos de `cmds` que compartían esos contratos; no se reescribieron los comandos ni se alteraron sus argumentos, límites o textos. No se migraron todos los formatos de envío ni las APIs completas de `internal/ui`. Sigue pendiente decidir y probar individualmente cualquier superficie externa que permanezca en `internal/wa`/`internal/ui` (por ejemplo, métodos de envío Protobuf aún no usados por los comandos) antes de ampliar otra fase.

### Validación

- `gofmt -l .` — limpio.
- `go test -count=1 ./...` — **pasa** (todos los paquetes en verde).
- `go vet ./...` — **pasa**.
- `go build ./...` — **pasa**.
- Cross-build estático Termux (`CGO_ENABLED=0 GOOS=linux GOARCH=arm64`, `./cmd/shin`) y `windows/amd64` — **pasa**.

La línea base AST se conserva en este documento; las mediciones de velocidad, memoria en ejecución real y comportamiento en teléfono quedan pendientes para contrastar en producción.
