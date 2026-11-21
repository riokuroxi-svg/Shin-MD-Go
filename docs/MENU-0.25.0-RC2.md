# 0.25.0-rc2 — ficha textual de Shin-MD

Esta revisión sustituye únicamente el cuerpo y el pie de la variante principal
de `.menu`. No añade banner, miniatura ni otro archivo multimedia. Los botones,
la hoja **Ver todo**, las páginas de categorías y sus fallbacks permanecen en
la interfaz centralizada.

## Contenido

La ficha sigue la estructura del bot de JavaScript:

- saludo según la hora y nombre saneado del usuario;
- **INFORMACIÓN SHIN-MD**: nombre configurado, versión del binario, motor
  `whatsmeow Multi-Device`, uptime real, clima y comandos únicos registrados;
- **INFORMACIÓN USUARIO**: nombre, rango, teléfono resuelto y prefijo;
- leyenda de insignias y aviso para abrir el selector de categorías;
- enlace al canal oficial;
- pie pequeño con `Shin-MD • Bot de WhatsApp Profesional` y el repositorio del
  bot de JavaScript.

No se copiaron los datos de las capturas. La versión llega de `main`, el uptime
del instante de arranque, la cantidad del registro de comandos, el rango del
contexto y el número de la identidad estable que resuelve WhatsApp. Si el
remitente solo tiene un LID sin teléfono conocido, la ficha dice
`No disponible` en vez de mostrar ese identificador como si fuera un número.

## Clima

La consulta usa los endpoints públicos de geocodificación y pronóstico actual
de Open-Meteo. Tiene un plazo total de tres segundos, redondea la temperatura y
guarda las respuestas correctas durante diez minutos. Un fallo no impide abrir
el menú: aparece `No disponible` y se vuelve a intentar después de un minuto.

La ciudad predeterminada es `Mexico City`; se puede cambiar sin recompilar:

```env
MENU_WEATHER_CITY="Tula de Allende"
```

## Compatibilidad

El comando continúa llamando solamente a `ReplyMenu`. La cadena de entrega no
cambia:

```text
.menu → ReplyMenu → baileys → nativa → texto
```

`.menucat` conserva sus `ListMessage` paginados, el máximo de diez filas y los
IDs `menu:<categoría>` y `menucat:pagina-N`. Las variantes `v2` y `v4` siguen
intactas. `MENU_IMAGE=1` y `.fax` continúan disponibles como funciones previas,
pero esta revisión no agrega ni adjunta imágenes a la portada textual.

## Comprobación en teléfono

1. Usar `MENU_IMAGE=0` y ejecutar `.menu`.
2. Verificar que nombre, versión, uptime, rango, número y prefijo corresponden
   a la ejecución actual, no a los valores de una captura.
3. Confirmar el bloque del clima o su fallback seguro si no hay red.
4. Abrir **Categorías**, avanzar y retroceder entre páginas y ejecutar una
   categoría.
5. Abrir **Ver todo** y probar perfil, ping, fax, copiar prefijo y repositorio.
6. Repetir con `INTERACTIVE_STRATEGIES=texto` para comprobar que el contenido y
   las órdenes escribibles sobreviven sin formatos ricos.
