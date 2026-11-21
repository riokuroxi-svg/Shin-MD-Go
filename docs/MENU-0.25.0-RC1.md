# 0.25.0-rc1 — «Nube de Shin»

Candidato de diseño publicado en `main` para probarlo en un cliente real de
WhatsApp. El estado anterior se conserva en un tag de retorno.

## Qué cambia

`.menu` conserva la misma operación, pero presenta una portada más clara:

```text
╭── ⋅ ──  ☁️  ✧  🪽  ── ⋅ ──╮

       ＳＨＩＮ–ＭＤ
     反魂 · NÚCLEO EN GO

╰── ⋅ ──  ★  ── ⋅ ──╯
```

Debajo aparecen el saludo, el número real de comandos, nombres, hooks y
prefijos. Los dos primeros accesos son **Categorías** y **Mi perfil**. Ping,
Fax, copiar prefijo y repositorio siguen dentro del mismo mensaje; como son más
de tres acciones, el protobuf ya probado añade la hoja inferior **Ver todo**.

WhatsApp no deja que un bot elija la tipografía del chat. El título fullwidth,
los ornamentos y el formato sí viajan en el texto; la forma concreta de cada
glifo depende de la fuente que tenga configurada el teléfono.

## Categorías: lista clásica y paginada

`.menucat` usa `ReplyList`, no construye protobufs en el comando. La cadena lo
convierte en un `ListMessage` de primer nivel y whatsmeow produce un solo nodo:

```xml
<biz><list v="2" type="product_list"/></biz>
```

Cada página contiene hasta ocho categorías y reserva hasta dos filas para
**Página anterior** y **Más categorías**. Así nunca supera los límites que se
validan antes de tocar el socket:

- encabezado: 60 caracteres;
- cuerpo: 1024;
- pie: 60;
- texto del botón: 20;
- sección y título de fila: 24;
- descripción de fila: 72;
- ID de fila: 200;
- máximo 10 secciones y 10 filas en total.

Los IDs de selección siguen siendo `menu:<categoría>`. La navegación usa
`menucat:pagina-N`. Ambos formatos ya entran por `CommandFromButton`, por lo
que no se añadió un router alternativo.

Si la construcción o el envío rico falla, la estrategia de texto conserva
órdenes escribibles como `.menu economia` y `.menucat pagina-2`.

## Compatibilidad

El cambio no toca el socket, los nodos `biz`, el fixture de Baileys ni las
estrategias de transporte. Usa exclusivamente el puerto existente:

```text
cmds/menu.go
  ├─ ReplyMenu  → baileys → nativa → texto
  └─ ReplyList  → lista_clasica → texto
```

La portada y las páginas tienen pruebas de regresión para:

- ornamentos y datos dinámicos;
- orden e IDs de los seis botones;
- cobertura exacta de todas las categorías;
- máximo de 10 filas por página;
- límites de texto de cada fila;
- inclusión automática de categorías futuras;
- degradación a texto de una lista inválida.

## Prueba en WhatsApp

1. Ejecutar `.menu` con `MENU_IMAGE=0`.
2. Confirmar que aparecen **Categorías**, **Mi perfil** y **Ver todo**.
3. Abrir **Ver todo** y probar Ping, Fax, copiar prefijo y repositorio.
4. Abrir **Categorías**, elegir una fila y comprobar que sale su menú.
5. Tocar **Más categorías**, elegir una fila de la segunda página y volver con
   **Página anterior**.
6. Probar también con `INTERACTIVE_STRATEGIES=texto` para confirmar que el menú
   sigue siendo utilizable sin formatos ricos.

Compilar y pasar pruebas automatizadas no demuestra que una versión concreta
de Android, iOS o Web lo dibuje. Esa confirmación solo se marca después de
verlo en el teléfono.

## Punto de retorno

La base anterior al rediseño es el commit
`f10107d74ad939624cbf97910a4ba3f33edc9a76`, conservado en el tag
`menu-pre-redesign-0.24.0`. `main` contiene el candidato 0.25.0-rc1.
