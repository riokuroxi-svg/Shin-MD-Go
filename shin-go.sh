#!/usr/bin/env sh
# Shin-MD-Go — instalación en un comando
# https://github.com/riokuroxi-svg/Shin-MD-Go
# Copyright (C) 2026 riokuroxi-svg · SPDX-License-Identifier: AGPL-3.0-only
#
#    curl -fsSL https://raw.githubusercontent.com/riokuroxi-svg/Shin-MD-Go/main/shin-go.sh | sh
#
#  Eso es todo. No hace falta instalar Go a mano, ni clonar, ni `make`.
#
#  QUÉ HACE, EN ORDEN
#    1. Detecta dónde corre (Termux, Linux, macOS, WSL, contenedor) y con
#       qué arquitectura.
#    2. Consigue las herramientas: git y el Go que pide el módulo (1.26.0).
#       El árbol de Go se baja de go.dev y se comprueba su SHA-256 contra
#       el .sha256 oficial antes de tocarlo. Sin root: va a ~/.local.
#    3. Clona el repo (o lo actualiza si ya está; jamás toca un clon cuyo
#       origin sea otro repo — p. ej. uno de desarrollo) y compila con `make`.
#    4. Deja el binario instalado ($PREFIX/bin/shin en Termux), un .env
#       listo para editar y ARRANCA el bot: pasa a ser este proceso (exec),
#       como el instalador del Shin-MD JS. Si no hay sesión de WhatsApp, el
#       propio bot se detiene a preguntarte tu número y te da el código de
#       8 letras. Con systemd (VPS con root) el arranque queda en la unidad.
#
#  OPCIONAL: yt-dlp
#    Los descargadores de vídeo sociales (.yt / .tiktok / .ig…) usan yt-dlp
#    cuando está instalado. Este script puede bajar el binario standalone
#    (no necesita Python) con SHIN_GO_CON_YTDLP=1 (defecto: sí; 0 para no).
#
#  POR QUÉ `sh` Y NO `bash`
#    Quien escribe `curl … | sh` está pidiendo sh, y en Alpine o busybox sh
#    no es bash. Nada aquí es bashismo: ni arrays, ni [[ ]]`, ni `local`.
#
#  ARGUMENTOS
#    --no-arrancar           deja todo listo, sin arrancar el bot
#    --code [NÚMERO]         se lo pasa al bot tal cual (vinculación)
#
#  VARIABLES
#    SHIN_GO_DIR=RUTA        dónde vive el código (defecto ~/shin-md-go)
#    SHIN_GO_REF=REF         rama o etiqueta de git (defecto main)
#    SHIN_GO_REPO=u/repo     otro repositorio (forks)
#    SHIN_GO_CON_YTDLP=0     no instalar yt-dlp
#    SHIN_GO_SIN_SERVICIO=1  compilar e instalar sin arrancar nada
#    SHIN_SIN_ARRANQUE=1     sinónimo (mismo nombre que el instalador JS)
#    SHIN_GO_DRY_RUN=1       dice qué haría, sin ejecutar
#    NO_COLOR=1              sin colores
set -eu

# ─────────────────────────── argumentos ───────────────────────────
# Lo que no sea para el instalador se lo paso al BOT (--code…), igual que
# el shin.sh del JS deriva todo al instalador y este al bot. POSIX no tiene
# arrays: se acumula en una cadena y se expande sin comillas al hacer exec.
BOT_ARGS=""
while [ $# -gt 0 ]; do
  case "$1" in
    --no-arrancar|--sin-arranque) SIN_SERVICIO_X=1 ;;
    --code|--code=*) BOT_ARGS="${BOT_ARGS:+$BOT_ARGS }$1" ;;
    *) printf '  ! argumento no reconocido: %s (se ignora)\n' "$1" >&2 ;;
  esac
  shift
done

readonly REF="${SHIN_GO_REF:-main}"
REPO="${SHIN_GO_REPO:-riokuroxi-svg/Shin-MD-Go}"
DIR="${SHIN_GO_DIR:-$HOME/shin-md-go}"
GO_VERSION="1.26.0"
CON_YTDLP="${SHIN_GO_CON_YTDLP:-1}"
SIN_SERVICIO="${SIN_SERVICIO_X:-${SHIN_GO_SIN_SERVICIO:-}}"
[ -n "${SHIN_SIN_ARRANQUE:-}" ] && SIN_SERVICIO=1
DRY_RUN="${SHIN_GO_DRY_RUN:-}"

# ─────────────────────────── salida ───────────────────────────
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ] && [ "${TERM:-}" != "dumb" ]; then
  R=$(printf '\033[0m'); RO=$(printf '\033[38;5;205m'); GR=$(printf '\033[38;5;245m')
  RJ=$(printf '\033[38;5;203m'); VD=$(printf '\033[38;5;114m'); AM=$(printf '\033[38;5;221m')
  NG=$(printf '\033[1m')
else
  R=""; RO=""; GR=""; RJ=""; VD=""; AM=""; NG=""
fi

paso()  { printf '\n%s▸ %s%s\n' "$RO$NG" "$*" "$R"; }
bien()  { printf '  %s✓%s %s\n' "$VD" "$R" "$*"; }
dato()  { printf '  %s·%s %s\n' "$GR" "$R" "$*"; }
aviso() { printf '  %s!%s %s\n' "$AM" "$R" "$*" >&2; }
morir() { printf '\n  %s✗ %s%s\n\n' "$RJ" "$*" "$R" >&2; exit 1; }

hay() { command -v "$1" >/dev/null 2>&1; }
ejecutar() {
  if [ -n "$DRY_RUN" ]; then dato "(dry-run) $*"; return 0; fi
  "$@"
}

# ─────────────────────── dónde estamos ───────────────────────
SO=$(uname -s 2>/dev/null || echo desconocido)
ARQ_CRUDA=$(uname -m 2>/dev/null || echo desconocida)
case "$ARQ_CRUDA" in
  aarch64|arm64)     ARQ=arm64 ;;
  armv7l|armv8l|arm) ARQ=armv7 ;;
  x86_64|amd64)      ARQ=amd64 ;;
  i686|i386)         ARQ=386 ;;
  *)                 ARQ="$ARQ_CRUDA" ;;
esac

ENTORNO="$SO"
TERMUX=0
case "${PREFIX:-}" in */com.termux/*) TERMUX=1; ENTORNO="Termux (Android)" ;; esac
if [ -n "${TERMUX_VERSION:-}" ]; then TERMUX=1; ENTORNO="Termux (Android)"; fi
case "$SO" in
  Darwin) ENTORNO="macOS" ;;
  Linux)
    if grep -qi microsoft /proc/version 2>/dev/null; then ENTORNO="$ENTORNO (WSL)"; fi
    if [ -f /.dockerenv ] || [ -f /run/.containerenv ]; then ENTORNO="$ENTORNO · contenedor"; fi
    ;;
esac
ROOT=0
if [ "$(id -u 2>/dev/null || echo 999)" = "0" ]; then ROOT=1; fi

# ─────────────────────── descargas con reintento ───────────────────────
INTENTOS=4
descargar() {  # $1 url, $2 destino
  _i=1
  while [ "$_i" -le "$INTENTOS" ]; do
    if hay curl; then
      if curl -fsSL --connect-timeout 20 --max-time 900 -o "$2" "$1" 2>/dev/null; then return 0; fi
    elif hay wget; then
      if wget -q -T 20 -O "$2" "$1" >/dev/null 2>&1; then return 0; fi
    else
      morir "no hay curl ni wget y no puedo descargar nada"
    fi
    if [ "$_i" -lt "$INTENTOS" ]; then
      _e=$((_i * 2))
      dato "la red falló (intento $_i de $INTENTOS); reintento en ${_e}s"
      sleep "$_e" 2>/dev/null || true
    fi
    _i=$((_i + 1))
  done
  return 1
}

# SHA-256 con lo que haya instalado: GNU, macOS, openssl o busybox.
huella() {
  if hay sha256sum; then
    sha256sum "$1" | cut -d' ' -f1
  elif hay shasum; then
    shasum -a 256 "$1" | cut -d' ' -f1
  elif hay openssl; then
    openssl dgst -sha256 "$1" | sed 's/^.*= *//'
  else
    echo ""
  fi
}

# ─────────────────────────── herramientas ───────────────────────────
asegurar_git() {
  if hay git; then bien "$(git --version | cut -d' ' -f3)"; return 0; fi
  if [ "$TERMUX" = 1 ]; then
    paso "Instalando git (pkg)"
    ejecutar pkg install -y git || morir "pkg no pudo instalar git"
  elif [ "$ROOT" = 1 ] && hay apt-get; then
    paso "Instalando git (apt)"
    ejecutar apt-get install -y git || morir "apt no pudo instalar git"
  elif [ "$ROOT" = 1 ] && hay apk; then
    paso "Instalando git (apk)"
    ejecutar apk add git || morir "apk no pudo instalar git"
  else
    morir "falta git y no puedo instalarlo sin root. Instálalo (apt install git / pkg install git) y repite."
  fi
  bien "git instalado"
}

asegurar_go() {
  # Si ya hay un Go suficiente (1.26+), para qué reinstalar.
  if hay go; then
    V=$(go version 2>/dev/null | sed 's/^.*go\([0-9.]*\).*$/\1/')
    case "$V" in
      1.26*|1.2[789]*|1.[3-9]*) bien "Go $V (ya estaba)"; return 0 ;;
    esac
    aviso "Go $V es viejo (pido $GO_VERSION); instalo uno aparte en ~/.local"
  fi

  # Termux: el tar oficial de go.dev es GOOS=linux y en Android se tropieza
  # con DNS (/etc/resolv.conf no existe) y con los certificados CA del
  # sistema. El paquete `golang` de Termux está compilado PARA Android y
  # no tiene esos problemas: es la vía correcta aquí.
  if [ "$TERMUX" = 1 ] && hay pkg && [ -z "$DRY_RUN" ]; then
    paso "Instalando Go (pkg install golang — nativo de Android)"
    pkg install -y golang || aviso "pkg golang falló; intento con el tar de go.dev"
    if hay go; then
      bien "$(go version | cut -d' ' -f3) vía pkg"
      return 0
    fi
  elif [ "$TERMUX" = 1 ]; then
    dato "(dry-run) pkg install -y golang"
  fi

  paso "Descargando Go $GO_VERSION ($SO/$ARQ)"
  SO_GO=$(printf '%s' "$SO" | tr 'A-Z' 'a-z')   # go.dev nombra los tarballs en minúscula
  caso_go="$SO_GO-$ARQ"
  case "$caso_go" in
    linux-armv7) TAR_GO="go${GO_VERSION}.linux-armv6l.tar.gz" ;;
    darwin-386)  morir "macOS de 32 bits no tiene Go moderno" ;;
    *)           TAR_GO="go${GO_VERSION}.${SO_GO}-${ARQ}.tar.gz" ;;
  esac
  TMPD=$(mktemp -d 2>/dev/null || echo /tmp/shin-go-$$)
  ejecutar mkdir -p "$TMPD" || true

  if [ -z "$DRY_RUN" ]; then
    # ojo: go.dev/dl/<nombre>.sha256 ya NO da el hash en crudo (devuelve un
    # HTML intersticial); el CDN canónico dl.google.com sí lo sirve en texto.
    descargar "https://dl.google.com/go/${TAR_GO}" "$TMPD/go.tgz" || morir "no pude bajar Go de go.dev"
    descargar "https://dl.google.com/go/${TAR_GO}.sha256" "$TMPD/go.sha256" || morir "no pude bajar la huella de Go"
    ESPERADA=$(cat "$TMPD/go.sha256")
    HALLADA=$(huella "$TMPD/go.tgz")
    if [ -z "$HALLADA" ]; then
      aviso "no hay herramienta de SHA-256; sigo sin verificar (no es lo ideal)"
    elif [ "$ESPERADA" != "$HALLADA" ]; then
      rm -rf "$TMPD"
      morir "la huella del toolchain NO coincide (esperaba $ESPERADA, hallé $HALLADA). No ejecuto nada."
    else
      bien "huella del toolchain verificada"
    fi
    mkdir -p "$HOME/.local/sdk"
    rm -rf "$HOME/.local/sdk/go"
    tar -C "$HOME/.local/sdk" -xzf "$TMPD/go.tgz" || morir "no pude descomprimir Go"
    rm -rf "$TMPD"
  else
    dato "(dry-run) dl.google.com/go/$TAR_GO → ~/.local/sdk/go + verificación SHA-256"
  fi
  PATH="$HOME/.local/sdk/go/bin:$PATH"; export PATH
  hay go || morir "instalé Go pero `go` no aparece en el PATH"
  bien "$(go version | cut -d' ' -f3) en ~/.local/sdk/go"
}

asegurar_ytdlp() {
  [ "$CON_YTDLP" = 1 ] || return 0
  paso "yt-dlp (opcional, para .yt/.tiktok)"
  if hay yt-dlp; then bien "yt-dlp ya estaba"; return 0; fi
  # Termux: el binario standalone de GitHub es glibc y NO corre bajo bionic
  # (Android); el paquete nativo del repo de Termux es la vía correcta aquí.
  if [ "$TERMUX" = 1 ]; then
    if [ -z "$DRY_RUN" ]; then
      if pkg install -y yt-dlp >/dev/null 2>&1 && hay yt-dlp; then
        bien "yt-dlp (paquete Termux)"
      else
        aviso "pkg no pudo instalar yt-dlp; .yt/.tiktok dirán que falta y cómo ponerlo"
      fi
    else
      dato "(dry-run) pkg install -y yt-dlp"
    fi
    return 0
  fi
  dato "binario standalone, sin Python"
  if [ -z "$DRY_RUN" ]; then
    mkdir -p "$HOME/.local/bin"
    if descargar "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp" "$HOME/.local/bin/yt-dlp"; then
      chmod +x "$HOME/.local/bin/yt-dlp"
      bien "yt-dlp en ~/.local/bin (actualícelo con: yt-dlp -U)"
    else
      aviso "no pude bajar yt-dlp; .yt/.tiktok dirán que falta y cómo ponerlo"
    fi
  else
    dato "(dry-run) bajaría yt-dlp standalone a ~/.local/bin"
  fi
  PATH="$HOME/.local/bin:$PATH"; export PATH
}

# Termux: el resolvedor puro de Go (netgo — el del toolchain y, al compilar
# con CGO_ENABLED=0, también el del bot) no lee las propiedades DNS de bionic
# como hacen los binarios C; lee $PREFIX/etc/resolv.conf. Si falta, TODA la
# resolución muere: «lookup proxy.golang.org on [::1]:53: connection refused».
# Los binarios C (pkg, git, curl) sí funcionaban, así que el síntoma engaña:
# parece "sin internet" cuando solo Go está ciego. Se crea UNA VEZ y nunca
# se pisa una configuración existente del usuario.
asegurar_dns() {
  [ "$TERMUX" = 1 ] || return 0
  if [ -s "$PREFIX/etc/resolv.conf" ]; then
    dato "DNS de Go: $PREFIX/etc/resolv.conf ya existe"
    return 0
  fi
  if [ -n "$DRY_RUN" ]; then
    dato "(dry-run) crearía $PREFIX/etc/resolv.conf con 8.8.8.8 y 1.1.1.1"
  else
    mkdir -p "$PREFIX/etc"
    printf 'nameserver 8.8.8.8\nnameserver 1.1.1.1\n' > "$PREFIX/etc/resolv.conf" \
      || aviso "no pude escribir $PREFIX/etc/resolv.conf; si el build falla con «lookup … [::1]:53», créalo tú con dos líneas «nameserver 8.8.8.8» y «nameserver 1.1.1.1»"
    bien "DNS de Go arreglado ($PREFIX/etc/resolv.conf → 8.8.8.8 / 1.1.1.1; edítalo si prefieres otros)"
  fi
  # Su gemelo TLS: con CGO_ENABLED=0, crypto/x509 solo mira /etc/ssl/… que
  # en Android no existe → «x509: certificate signed by unknown authority» en
  # cada HTTPS. El almacén real es $PREFIX/etc/tls/cert.pem (paquete
  # ca-certificates) y tanto el toolchain como el bot lo toman por
  # SSL_CERT_FILE. Garantiza que exista:
  if [ ! -s "$PREFIX/etc/tls/cert.pem" ]; then
    if [ -n "$DRY_RUN" ]; then
      dato "(dry-run) pkg install -y ca-certificates"
    elif pkg install -y ca-certificates >/dev/null 2>&1; then
      bien "ca-certificates instalado (almacén TLS de Termux)"
    else
      aviso "no pude instalar ca-certificates; si salen errores x509: pkg install ca-certificates"
    fi
  fi
}

# make tampoco se da por hecho: Termux y los contenedores mínimos no lo traen.
asegurar_make() {
  if hay make; then bien "make presente"; return 0; fi
  if [ "$TERMUX" = 1 ]; then
    paso "Instalando make (pkg)"
    ejecutar pkg install -y make || morir "pkg no pudo instalar make"
  elif [ "$ROOT" = 1 ] && hay apt-get; then
    paso "Instalando make (apt)"
    ejecutar apt-get install -y make || morir "apt no pudo instalar make"
  elif [ "$ROOT" = 1 ] && hay apk; then
    paso "Instalando make (apk)"
    ejecutar apk add make || morir "apk no pudo instalar make"
  else
    morir "falta make y no puedo instalarlo sin root (apt install make / pkg install make). go build a mano también sirve: go build -o dist/shin ./cmd/shin"
  fi
  bien "make instalado"
}

# ─────────────────────────── el bot ───────────────────────────
obtener_codigo() {
  paso "Código: $REPO ($REF)"
  if [ -d "$DIR/.git" ]; then
    dato "ya existe; actualizo"
    # Guardia: $DIR puede ser un clon DE DESARROLLO que casualmente se llame
    # igual. Antes de un fetch+reset --hard (que pisa cambios locales)
    # compruebo que su origin apunte a ESTE repo; si no, me detengo.
    URL_ORIGEN=$(git -C "$DIR" remote get-url origin 2>/dev/null || echo "")
    _n1=$(printf '%s' "$URL_ORIGEN" | sed 's|^[a-zA-Z]*://[^/]*/||; s|^[^@]*@[^:]*:||; s|\.git$||')
    _n2=$(printf '%s' "$REPO" | sed 's|\.git$||')
    if [ -z "$URL_ORIGEN" ] || [ "$_n1" != "$_n2" ]; then
      morir "$DIR ya es un repo distinto (origin: ${URL_ORIGEN:-ninguno}). Por seguridad no lo toco: ejecuta con SHIN_GO_DIR=otra/ruta o borra esa carpeta."
    fi
    if [ -z "$DRY_RUN" ]; then
      git -C "$DIR" fetch --depth 1 origin "$REF" || morir "git fetch falló"
      git -C "$DIR" reset --hard FETCH_HEAD || morir "git reset falló"
      bien "actualizado"
    else
      dato "(dry-run) git fetch + reset --hard en $DIR"
    fi
  else
    if [ -z "$DRY_RUN" ]; then
      git clone --depth 1 --branch "$REF" "https://github.com/$REPO.git" "$DIR" \
        || morir "git clone falló"
      bien "clonado en $DIR"
    else
      dato "(dry-run) git clone → $DIR"
    fi
  fi
}

compilar() {
  paso "Compilando"
  if [ -n "$DRY_RUN" ]; then dato "(dry-run) make build en $DIR"; return 0; fi
  # Termux: las CA raíz no están en las rutas clásicas de Linux; sin esta env
  # la fase HTTPS muere con «x509: certificate signed by unknown authority»
  # en cuanto el DNS (abajo, si hizo falta) ya deja pasar.
  if [ "$TERMUX" = 1 ] && [ -s "$PREFIX/etc/tls/cert.pem" ] && [ -z "${SSL_CERT_FILE:-}" ]; then
    export SSL_CERT_FILE="$PREFIX/etc/tls/cert.pem"
    dato "TLS del toolchain: SSL_CERT_FILE=$PREFIX/etc/tls/cert.pem"
  fi
  if (cd "$DIR" && PATH="$HOME/.local/sdk/go/bin:$PATH" make build); then
    bien "$DIR/dist/shin"; return 0
  fi
  [ "$TERMUX" = 1 ] || morir "la compilación falló (mira arriba)"

  # ── Termux: la escalera real contra el trío seccomp+DNS+TLS ──────────────
  #  Orden y porqués (todos medidos en un teléfono Android de seccomp duro):
  #
  #  · GODEBUG=netdns=cgo se descartó como escalón: no muerde en go1.27.3
  #    del repo (comprobado) y no dice qué falló.
  #
  #  · El escalón siguiente es AMBOS trucos proot —/etc/resolv.conf y
  #    SSL_CERT_FILE (ya exportado arriba)— porque el que solo quita el DNS
  #    solo cambia el error por el del siguiente piso. Y (clave) con
  #    GOMAXPROCS=1: el compilador Go lanza su interno «buscar la herramienta»
  #    (os/exec.LookPath) DESDE GORUTINAS DEL BUILD, y en Android 13+ ese
  #    LookPath alcanza faccessat2, que tu seccomp mata con SIGSYS antes de
  #    poder caer a faccessat. Corriendo todo el build en un solo procesador
  #    serializa esas llamadas hasta dejarlas ser inocuas; es la receta que
  #    sí compila go1.27.x en Termux sin tocar el binario de Go.
  #    Esto solo funciona gracias a que la cache modular ya está llena
  #    (el primer build la llenó tras arreglar DNS+TLS): aquí NO hay red.
  hay proot || pkg install -y proot >/dev/null 2>&1 || true
  if ! hay proot; then
    aviso "siguió el error y proot no está disponible. Mira arriba."
    morir "la compilación falló (mira arriba)"
  fi
  aviso "reintento 1: proot + GOMAXPROCS=1 (serializa LookPath; seccomp serializado queda inocuo)"
  if (cd "$DIR" && PATH="$HOME/.local/sdk/go/bin:$PATH" \
        proot -b "$PREFIX/etc/resolv.conf:/etc/resolv.conf" \
        env GOMAXPROCS=1 make build); then
    bien "$DIR/dist/shin (proot, serializado)"
    return 0
  fi
  aviso "reintento 2 (plan B): proot cómodo, sin serializar"
  if (cd "$DIR" && PATH="$HOME/.local/sdk/go/bin:$PATH" \
        proot -b "$PREFIX/etc/resolv.conf:/etc/resolv.conf" make build); then
    bien "$DIR/dist/shin (proot)"
    return 0
  fi
  morir "la compilación falló (mira arriba). Último recurso: compila «make termux» en una PC, muévelo a $PREFIX/bin/shin"
}

instalar() {
  paso "Instalación"
  if [ "$TERMUX" = 1 ]; then
    DESTINO="$PREFIX/bin"
  elif [ "$ROOT" = 1 ]; then
    DESTINO="/opt/shin"
  else
    DESTINO="$HOME/.local/shin"
  fi
  if [ -n "$DRY_RUN" ]; then dato "(dry-run) dist/shin + .env → $DESTINO"; return 0; fi
  mkdir -p "$DESTINO/data"
  cp "$DIR/dist/shin" "$DESTINO/shin"
  chmod +x "$DESTINO/shin"

  if [ ! -f "$DESTINO/.env" ]; then
    if [ -f "$DIR/.env.example" ]; then
      cp "$DIR/.env.example" "$DESTINO/.env"
    else
      printf '# Shin-MD-Go\nPREFIX=.\n' > "$DESTINO/.env"
    fi
    chmod 600 "$DESTINO/.env"
    dato ".env creado en $DESTINO/.env — edítalo a tu gusto"
  else
    dato ".env ya existía; no lo toco"
  fi
  bien "$DESTINO/shin"
}

arrancar() {
  [ -z "$SIN_SERVICIO" ] || { dato "arranque desactivado (--no-arrancar / SHIN_GO_SIN_SERVICIO=1)"; return 0; }

  # Termux: además del arranque inmediato, el de al encender el teléfono
  # (solo muerde si tienes la app Termux:Boot; sin ella, el archivo estorba
  # a nadie).
  if [ "$TERMUX" = 1 ]; then
    if [ -z "$DRY_RUN" ]; then
      mkdir -p "$HOME/.termux/boot"
      cat > "$HOME/.termux/boot/shin" <<ARRANQUE
#!/data/data/com.termux/files/usr/bin/sh
cd "$PREFIX/bin" && exec "$PREFIX/bin/shin" >> "\$HOME/shin.log" 2>&1
ARRANQUE
      chmod +x "$HOME/.termux/boot/shin"
      dato "arranque al encender el teléfono: ~/.termux/boot/shin (con la app Termux:Boot)"
    else
      dato "(dry-run) script de arranque de Termux"
    fi
  fi

  # VPS con root y systemd: ahí el arranque lo lleva la unidad, en segundo
  # plano. La vinculación se hace leyendo el código en el journal, o a mano.
  if [ "$ROOT" = 1 ] && [ "$TERMUX" != 1 ] && hay systemctl && [ -d /run/systemd/system ]; then
    paso "Servicio"
    if [ -n "$DRY_RUN" ]; then dato "(dry-run) unidad systemd desde deploy/shin.service"; return 0; fi
    if [ -f "$DIR/deploy/shin.service" ]; then
      sed "s|/opt/shin|$DESTINO|g" "$DIR/deploy/shin.service" > /etc/systemd/system/shin.service
      systemctl daemon-reload
      systemctl enable --now shin >/dev/null 2>&1 || aviso "systemd lo registró pero no arrancó; mira: journalctl -u shin"
      bien "servicio systemd «shin» activo (reinicia solo si cae)"
      dato "primera vez: vincula con   shin --code 521XXXXXXXXXX   y reinicia: systemctl restart shin"
    else
      aviso "no encontré deploy/shin.service; arranca a mano: shin --code 521XXXXXXXXXX"
    fi
    return 0
  fi

  # Donde no hay systemd —Termux incluido— el bot arranca Y PASA A SER este
  # proceso (exec), como hace el instalador del JS: Ctrl+C lo para, el
  # código de salida es el del bot y no queda un instalador zombie detrás.
  # Si no hay sesión de WhatsApp, el propio bot se detiene a preguntarte el
  # número (lee la terminal de verdad, da igual que vengas de un «curl | sh»)
  # o, si ni siquiera hay terminal, te dice la orden exacta y sale limpio.
  paso "Arrancando Shin-MD-Go"
  if [ -n "$DRY_RUN" ]; then dato "(dry-run) exec $DESTINO/shin $BOT_ARGS"; return 0; fi
  if hay pgrep && pgrep -x shin >/dev/null 2>&1; then
    aviso "ya hay un «shin» corriendo; no arranco otro."
    dato "para reiniciarlo: Ctrl+C en su terminal y vuelve a escribir  shin"
    return 0
  fi
  cd "$DESTINO" || morir "no pude entrar en $DESTINO"
  dato "deja esta terminal abierta · Ctrl+C para parar"
  # $BOT_ARGS sin comillas a propósito: porta --code y compañía.
  exec "$DESTINO/shin" $BOT_ARGS
}

# ─────────────────────────── main ───────────────────────────
paso "Shin-MD-Go · instalación en un comando"
dato "entorno: $ENTORNO · $ARQ"
[ -n "$DRY_RUN" ] && aviso "DRY-RUN: cuento qué haría, no ejecuto nada"

# El resumen va ANTES del arranque: cuando este último hace exec, nada de
# lo escrito después se llegaría a ver jamás.
resumen() {
  if [ -n "$DRY_RUN" ]; then dato "dry-run terminado; nada se tocó"; return 0; fi
  VER=$(sed -n 's/^VERSION  := //p' "$DIR/Makefile" 2>/dev/null || true)
  paso "Listo"
  printf '  %s✓%s Shin-MD-Go %s instalado en %s%s\n\n' "$VD" "$R" "${VER:-}" "$DESTINO" "$R"
  dato "arrancar (la próxima vez, ya instalado):"
  printf '      shin                        desde cualquier carpeta\n'
  printf '      shin --code 521XXXXXXXXXX   vincular con número a mano\n\n'
  dato "vincular WhatsApp: el bot pregunta tu número la primera vez y te da"
  dato "un código de 8 letras (Ajustes › Dispositivos vinculados › con número)"
  dato "parar: Ctrl+C en la terminal del bot"
  dato "config: $DESTINO/.env · datos: $DESTINO/data/"
  dato "actualizar: el mismo comando de siempre; .env y data/ jamás se tocan"
}

asegurar_git
asegurar_make
asegurar_go
asegurar_dns
asegurar_ytdlp
obtener_codigo
compilar
instalar
resumen
arrancar   # puede terminar en exec: si «vuelve», es que el bot salió
