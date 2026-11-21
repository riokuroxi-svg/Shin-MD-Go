# Botones e interactivos de WhatsApp

_Verificado el 20-nov-2026 contra Baileys 7.0.0-rc14, Baileys `master`
`bb0a87e`, whatsmeow `4cf8bc8e0aeb` y tráfico documentado en
`tulir/whatsmeow#1235`._

## La regla real

Los botones no son un flag de la librería. Un envío necesita dos capas:

1. el protobuf `InteractiveMessage` con `NativeFlowMessage`;
2. el nodo binario de stanza `biz` que describe `native_flow`.

WhatsApp puede aceptar la stanza aunque una capa esté mal y el teléfono mostrar
«mensaje no compatible» o no renderizar nada. Por eso se prueban por separado.
La investigación y la comparación campo por campo están en
[`INTERACTIVOS-0.24.0.md`](INTERACTIVOS-0.24.0.md).

## Un solo paquete

Todo vive en `internal/interactivos/`:

- `modelo.go`: botones, listas, tarjetas y peticiones;
- `protobuf.go`: formato directo y formato envuelto estilo Baileys;
- `lista.go`: `ListMessage` directo y compatibilidad `product_list`;
- `nodos.go`: `biz`, `quality_control`, `decision_id` y `decision_source`;
- `enviador.go`: interfaz y cadena `baileys → nativa → texto`;
- `interactivos_test.go`: fixture de Baileys y pruebas de degradación.

Los comandos no importan ese paquete ni protobufs. Llaman a `bot.Messenger` por
`ReplyMenu`, `ReplyButtons`, `ReplyImage`, `ReplyList` o `ReplyCarousel`.

## Catálogo

| Helper | `name` native-flow | Uso |
|---|---|---|
| `QuickReply(texto, id)` | `quick_reply` | devuelve un id al router |
| `CTAURL(texto, url)` | `cta_url` | abre un enlace |
| `CTACopy(texto, código)` | `cta_copy` | copia al portapapeles |
| `CTACall(texto, teléfono)` | `cta_call` | abre una llamada |
| `SendLocation(texto)` | `send_location` | solicita ubicación |
| `OpenWebview(título, url)` | `open_webview` | web embebida |
| `MPM(jidNegocio)` | `mpm` | catálogo multiproducto |
| `FlowButton(flowID, token, cta)` | `flow` | WhatsApp Flow publicado |
| `SingleSelect(título, secciones…)` | `single_select` | helper experimental; `.lista` no depende de él |

Una tarjeta con más de tres botones recibe `messageParamsJson.bottom_sheet`:
se ven tres en el hilo y los demás quedan detrás de «Ver todo», en lugar de
desaparecer sin aviso.

## Nodos que viajan

```xml
<biz actual_actors="2" host_storage="2" privacy_mode_ts="UNIX">
  <interactive type="native_flow" v="1">
    <native_flow v="9" name="mixed"/>
  </interactive>
  <quality_control decision_id="40_HEX" source_type="third_party">
    <decision_source value="df"/>
  </quality_control>
</biz>
```

El `decision_id` se genera con 20 bytes criptográficos nuevos por mensaje. En
privados se añade `<bot biz_bot="1"/>`; en grupos no. Debe existir un solo
`biz`: duplicarlo puede hacer que WhatsApp rechace la stanza.

La lista es la excepción: viaja como `ListMessage` de primer nivel, con payload
`SINGLE_SELECT` y secreto de 32 bytes. No recibe `AdditionalNodes`; whatsmeow
crea su único `<biz><list v="2" type="product_list"/></biz>`. La etiqueta
`product_list` es necesaria aunque el payload siga siendo de selección simple.
El constructor valida antes de enviar los límites del cliente: título 60,
botón 20, sección/fila 24, descripción de fila 72, hasta 10 secciones y **10
filas en total**. Una lista inválida no llega al socket: cae al texto accionable.

## Estrategias y fallback

Orden por defecto:

```text
baileys (envuelto) → nativa (directa) → texto
```

Si una estrategia devuelve error se prueba la siguiente. Para `SendList`, las
posiciones ricas se condensan en `lista_clasica → texto`, porque native-flow
`single_select` puede recibir ACK sin llegar a cuentas normales. El texto lista
IDs y URLs para que el menú siga siendo accionable. Como WhatsApp no confirma que la
app receptora dibujó el contenido —solo que aceptó la stanza—, un cliente raro
que responda éxito pero enseñe «no compatible» no se puede detectar desde el
bot. Para ese caso se fuerza el modo seguro:

```env
INTERACTIVE_STRATEGIES=texto
```

## Pruebas manuales

| Comando | Contenido |
|---|---|
| `.menu` | ficha «Nube de Shin»; dos accesos visibles y hoja inferior «Ver todo» |
| `.menucat` | `ListMessage` paginado: máximo 10 filas y selección por categoría |
| `.botones` | quick reply, URL, copiar y llamada |
| `.lista` | `ListMessage` directo + nodo `product_list`, dos secciones |
| `.carrusel` | tres tarjetas horizontales |

`go test ./internal/interactivos ./cmds` verifica el formato y que ningún
comando se salte la interfaz.

## Otros mensajes ricos

No se mezclaron con los interactivos. `internal/ui/rich.go` conserva los
constructores de encuesta ampliada, pin/despin, evento de calendario y respuesta
enriquecida (`RichText`, `RichCode`, `RichTable`, `RichTextCard`). Sus comandos
de prueba siguen siendo `.encuesta`, `.evento`, `.pin` y `.rich`; este último es
experimental y requiere `RICH_AI=1`.
