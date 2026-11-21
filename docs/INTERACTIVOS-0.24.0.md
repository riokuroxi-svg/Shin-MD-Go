# 0.24.0 — botones nativos: Baileys, whatsmeow y fallback real

Investigación cerrada el **20 de noviembre de 2026** contra:

- `@whiskeysockets/baileys` **7.0.0-rc14** (la etiqueta `latest` de npm);
- `WhiskeySockets/Baileys` `master` en `bb0a87e0326ca8f882fe716c7ee42da362f73fc0`;
- el otro Shin-MD en `bda09e447e87d7d4f2ed98f9e04a5475e324d55d`;
- el `whatsmeow` fijado por este módulo, `4cf8bc8e0aeb`;
- la captura de tráfico y el arreglo documentados en
  [`tulir/whatsmeow#1235`](https://github.com/tulir/whatsmeow/pull/1235);
- las pruebas con cuentas normales de
  [`evolution-go#246`](https://github.com/evolution-foundation/evolution-go/pull/246)
  y [`whatsapp-mcp#15`](https://github.com/AriOliv/whatsapp-mcp/pull/15).

## Una precisión importante sobre «lo que manda Baileys»

Baileys oficial no expone hoy una llamada de alto nivel que mágicamente arregle
los botones native-flow. El patrón que usan los bots es:

1. construir `proto.Message.InteractiveMessage`;
2. envolverlo (en la variante solicitada) en `viewOnceMessage.message`;
3. generar el mensaje con `generateWAMessageFromContent`;
4. enviarlo con `relayMessage` y el nodo binario `biz` en `additionalNodes`.

Por eso se compararon dos cosas por separado: el **protobuf cifrado** y los
**nodos de stanza sin cifrar**. Un protobuf correcto no compensa un `biz`
incompleto: el servidor puede aceptar el envío y el teléfono enseñar «mensaje
no compatible» o no pintar nada.

## Comparación campo por campo

| Capa/campo | Go 0.23.1 | Baileys-style comprobado | Go 0.24.0 |
|---|---|---|---|
| `Message.viewOnceMessage` (campo 37) | ausente | presente | presente en estrategia `baileys` |
| `viewOnce.message` | — | mensaje interior | igual |
| `Message.messageContextInfo` (35) | ausente | dentro del envoltorio | igual |
| `deviceListMetadata` (1) | ausente | objeto vacío presente | igual |
| `deviceListMetadataVersion` (2) | ausente | `2` | `2` |
| `Message.interactiveMessage` (45) | primer nivel | dentro del mensaje interior | igual |
| `Interactive.header` (1) | condicional; sin `subtitle` | `title`, `subtitle=""`, `hasMediaAttachment` | igual |
| `Interactive.body` (2) | texto | texto | igual |
| `Interactive.footer` (3) | texto + un `false` extra | texto | igual |
| oneof `nativeFlowMessage` (6) | presente | presente | presente |
| `buttons` (1) | `name` + JSON | `name` + JSON | igual |
| `messageParamsJson` (2) | ausente | hoja inferior cuando hay >3 | se genera cuando hay >3 |
| `messageVersion` (3) | **9** | **1** | **1** |

El último renglón era una mezcla de capas: el **protobuf** usa
`messageVersion=1`; el `9` pertenece al nodo binario
`<native_flow v="9" name="mixed"/>`.

La prueba `TestProtobufBaileysCampoPorCampoYBytePorByte` contiene una fixture
generada por `generateWAMessageFromContent` + `proto.Message.encode` de Baileys
7.0.0-rc14. El constructor Go se
compara primero con `proto.Equal` y luego **byte por byte** con esa fixture.

## El nodo `biz` completo

La 0.23.1 ya mandaba `biz`, pero le faltaban dos piezas que no son decorativas:
`decision_id` y `decision_source`. Ahora cada envío lleva exactamente:

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

`decision_id` son 20 bytes de `crypto/rand`, nuevos en cada envío. En un chat
privado se añade además `<bot biz_bot="1"/>`; en un grupo no. Se comprueba que
viaja **un solo** `biz`, porque duplicarlo también rompe la entrega.

## La lista no usa `single_select` native-flow

Las pruebas más recientes con cuentas normales reportan otro fallo silencioso:
`single_select` dentro de `InteractiveMessage` puede recibir ACK y no llegar. Por
eso `SendList` usa un `ListMessage` de primer nivel, sin
`DocumentWithCaptionMessage` y sin `AdditionalNodes`, más un secreto aleatorio
de 32 bytes para iOS. Whatsmeow crea el único nodo de stanza:

```xml
<biz><list v="2" type="product_list"/></biz>
```

El protobuf conserva `ListType=SINGLE_SELECT`; `product_list` es la etiqueta de
stanza que permite la entrega, no un catálogo de productos dentro del payload.
La revisión fijada de whatsmeow deriva esa etiqueta de la tabla textual del enum.
`lista.go` ajusta esa entrada durante la inicialización, antes de arrancar
ninguna goroutine. Así se evita bajar a un fork anterior de toda la biblioteca
por un cambio de una línea, sin duplicar el nodo `biz`; una prueba fija las dos
capas y el pin de whatsmeow impide que cambien por sorpresa.

## Un paquete y estrategias intercambiables

Todo lo interactivo vive ahora en `internal/interactivos/`:

- modelos: `Button`, `ListSection`, `Card`, `Peticion`;
- interfaz: `Enviador` (`SendMenu`, `SendButtons`, `SendList`, `SendCarousel`);
- estrategia `baileys`: protobuf envuelto + contexto v2 + `biz` completo;
- estrategia `nativa`: el mismo interior, directo en el primer nivel;
- estrategia `lista_clasica`: `ListMessage` directo + `product_list`;
- estrategia `texto`: mensaje corriente y accionable (`.ping`, `.menu`, URL…).

El orden por defecto es:

```text
baileys → nativa → texto
```

Si un envío devuelve error/ACK inválido, se prueba el siguiente. En `SendList`,
las posiciones ricas se condensan en `lista_clasica → texto`; no se intenta el
`single_select` silencioso. El texto nunca se puede quitar de la cola, incluso
si el `.env` lo omite. Se puede cambiar el orden para diagnóstico:

```env
INTERACTIVE_STRATEGIES=baileys,nativa,texto
# seguridad total mientras se prueba un teléfono raro:
INTERACTIVE_STRATEGIES=texto
```

WhatsApp no confirma que el receptor **haya dibujado** un mensaje; solo confirma
que aceptó la stanza. Por eso ningún bot puede detectar a posteriori el cartel
«no compatible» si el servidor respondió éxito. Para ese caso existe el modo
forzado `texto`. Con la estructura corregida, el camino normal es Baileys.

## Los comandos no conocen el mecanismo

`.menu`, `.menucat`, `.botones`, `.lista` y `.carrusel` llaman exclusivamente a
`bot.Messenger` mediante `ReplyMenu`, `ReplyButtons`, `ReplyList` y
`ReplyCarousel`. No importan el paquete concreto, no construyen protobufs y no
escriben nodos `biz`.

`TestComandosNoSaltanElPuertoInteractivo` recorre los fuentes de `cmds/` y falla
si alguien vuelve a llamar `wa.Client.Send*`, importa protobufs o arma un
`InteractiveMessage`/`biz` dentro de un comando.

## Decisiones frente a las recomendaciones complementarias

- No se añadió un `replace` a un fork de whatsmeow: el fork de referencia parte
  de `33cfac5`, anterior al `4cf8bc8` ya fijado por el proyecto. Los botones
  suministran el nodo completo por `AdditionalNodes`; la lista aplica localmente
  la única etiqueta que falta y deja a whatsmeow crear su nodo.
- Se conserva `baileys` envuelto como primer intento porque era un requisito y
  la fixture lo fija; #1235 confirma que el desempaquetado de `viewOnceMessage`
  también funciona. `nativa` directa sigue inmediatamente después.
- El carrusel existe y conserva imágenes y parámetros de URL/llamada/copia en
  pruebas, pero no reemplaza todavía a `.menu`: hacerlo sin una prueba real de
  teléfono cambiaría una estrategia comprobada en bytes por otra experimental.
- Los taps de botones, listas y native-flow se parsean y enrutan al comando. No
  se archivan como mensajes sintéticos: este bot no mantiene un historial de
  chats ni webhooks de mensajes.
- El panel TUI ya quedó anclado al búfer alternativo en 0.23.0; la vinculación
  permanece fuera de la TUI hasta `Connected` desde 0.23.1.

## Estado de validación

La construcción está cubierta por la fixture de Baileys, pruebas de nodos,
respuestas `native_flow`, orden/fallo de estrategias, race detector y builds
multiplataforma. No se abrió una sesión real de WhatsApp durante esta entrega:
el render final en Android/iOS/Web sigue siendo una comprobación de teléfono.
Un ACK de envío demuestra que el servidor aceptó la stanza, no que cada versión
de la app la haya dibujado.
