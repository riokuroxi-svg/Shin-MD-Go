# Música — `.play`, previews y spotDL opcional

_20-nov-2026 · versión 0.26.0-rc1_

`.play` conserva su motor integrado y suma un backend externo opcional. El bot
Go no depende de Node y no descarga ejecutables durante el arranque.

## Fuentes integradas

| Fuente | Qué entrega | Marca |
|---|---|---|
| **Audius** | audio completo del catálogo libre | 🟢 descarga libre |
| **Deezer / iTunes** | ficha y preview oficial de 30 s | 🟡 preview |
| **spotDL** (opcional) | pista resuelta por el proveedor configurado en spotDL | 🟣 spotDL |

`Pista.Libre` solo se usa para las fuentes realmente libres. Una pista resuelta
por spotDL lleva `BackendSpotDL`; nunca se etiqueta como libre. spotDL obtiene
metadatos de Spotify y busca el audio en los proveedores configurados. El
operador debe habilitarlo únicamente para contenido que tenga derecho a
descargar.

## Por qué no se importó `spotdl-api`

El paquete npm `spotdl-api` inspiró la interfaz, pero su versión 1.0.0 incluye y
descarga el ejecutable Linux de spotDL sin distinguir arquitectura. Ese archivo
es x86-64 y no puede ejecutarse en Termux ARM64. También añadiría Node a un bot
que hoy se distribuye como un único binario Go.

La integración usa directamente el CLI estable de spotDL:

```text
Shin-MD-Go → exec.CommandContext → spotDL → ffmpeg
```

No hay shell, `eval`, `npm install` ni descarga automática. Cada argumento se
pasa por separado y el operador elige qué ejecutables confiar.

## Configuración

El backend está apagado de forma predeterminada:

```env
SPOTDL_PATH=
SPOTDL_FFMPEG=ffmpeg
SPOTDL_COOKIE_FILE=
SPOTDL_AUDIO=piped,soundcloud,bandcamp,youtube-music,youtube
SPOTDL_TIMEOUT=300
```

Al definir `SPOTDL_PATH`, el arranque resuelve ambos ejecutables y consulta
`spotdl --version`. `SPOTDL_AUDIO` admite únicamente `youtube-music`, `youtube`,
`soundcloud`, `bandcamp` y `piped`, separados por comas. Si se omite o queda
vacío, la configuración prioriza `piped,soundcloud,bandcamp` antes de YouTube.
`SPOTDL_COOKIE_FILE` pasa un archivo de cookies existente sin copiarlo ni
imprimir su contenido. Un fallo solo desactiva spotDL; Audius, Deezer e iTunes
siguen disponibles.

### VPS Linux

Instala ffmpeg y spotDL con el gestor que prefieras —por ejemplo `pipx`— y
configura las rutas:

```sh
pipx install spotdl
sudo apt install ffmpeg

SPOTDL_PATH=spotdl
SPOTDL_FFMPEG=ffmpeg
```

También puede usarse el ejecutable oficial de spotDL en Linux x86-64. No se
recomienda que el bot descargue automáticamente “latest”: una actualización no
debe cambiar el binario de producción sin revisión.

### Termux ARM64

No uses el ejecutable Linux que instala `spotdl-api`; es x86-64. Instala la
versión Python para la arquitectura del teléfono:

```sh
pkg install python ffmpeg
python -m pip install spotdl

SPOTDL_PATH=spotdl
SPOTDL_FFMPEG=ffmpeg
```

## Uso

```text
.play <nombre>             Audius + Deezer + iTunes, como antes
.play <URL de Spotify>     fuerza spotDL
.play spotify:track:...    fuerza spotDL
.play spotdl:<consulta>    fuerza spotDL con búsqueda de texto
.play N                    descarga el resultado N de la última lista
.escucha <nombre>          preview oficial de Deezer/iTunes
```

Una búsqueda de texto normal no abre un proceso externo si las fuentes rápidas
respondieron. Si no devolvieron nada y spotDL está habilitado, se usa como
fallback. El prefijo `spotdl:` permite pedirlo explícitamente.

## Límites del adaptador

- **una pista individual por descarga**: una playlist se separa en filas y cada
  resultado usa su URL canónica de pista;
- máximo **20 minutos** y **24 MB**;
- salida fija **MP3 a 128 kbps** para no rebasar WhatsApp;
- metadatos limitados a 4 MB y consultas a 500 caracteres;
- máximo 60 s para resolver metadatos y 30–900 s configurables para descargar;
- archivo temporal privado, eliminado al terminar;
- validación de archivo regular y cabecera MP3 antes de copiar un byte al envío;
- máximo dos descargas simultáneas por chat, igual que el motor anterior;
- al cancelar o vencer el contexto en Linux se mata el grupo completo de
  spotDL, ffmpeg y procesos auxiliares.

spotDL puede terminar con código cero aunque una pista individual falle. Por
eso el adaptador no confía solo en el código de salida: exige que exista
`audio.mp3`, que sea regular, que respete el tamaño y que tenga cabecera MP3.

## Pruebas

Las pruebas automatizadas usan un ejecutable fingido y verifican:

- JSON con logs antes del arreglo;
- URL canónica por pista y límite de resultados;
- consulta con metacaracteres entregada como un único argumento, sin shell, y
  rechazo de texto que intente convertirse en bandera del CLI;
- descarga temporal, URL canónica, cabecera MP3 y topes de duración/tamaño;
- cancelación por contexto y proceso ausente;
- cookies, lista cerrada de proveedores y configuración desde `.env` con plazo
  acotado;
- etiquetas `🟢`, `🟡` y `🟣`, lista numerada y semáforo por chat.

La comprobación manual del motor integrado descargó una pista libre de Audius
de 7.48 MB, MP3/ID3 a 320 kbps. El adaptador real resolvió con spotDL 4.5.2 y
4.6.0 una URL de Spotify a título, artista y duración. En este entorno, YouTube
devolvió `AudioProviderError`; el adaptador lo detectó aunque spotDL había
terminado su progreso y no envió un archivo inexistente. Una prueba posterior
de extremo a extremo con spotDL 4.6.0 y el orden predeterminado resolvió y bajó
“Monkeys Spinning Monkeys”, pista Creative Commons: MP3 válido de 2,078,466
bytes (1.98 MiB). Estos resultados no garantizan disponibilidad en cada red o
proveedor; por eso se documentan cookies y selección de proveedores. El
recorrido exitoso también se cubre de forma determinista con un proceso fingido
que genera un MP3 válido.
