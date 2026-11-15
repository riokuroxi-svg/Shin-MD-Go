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
#    4. Deja el binario instalado, un .env listo para editar y —si puede—
#       el servicio en marcha (systemd en Linux con root, script de arranque
#       de Termux en Android, nohup en el resto).
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
#  VARIABLES
#    SHIN_GO_DIR=RUTA        dónde vive el código (defecto ~/shin-md-go)
#    SHIN_GO_REF=REF         rama o etiqueta de git (defecto main)
#    SHIN_GO_REPO=u/repo     otro repositorio (forks)
#    SHIN_GO_CON_YTDLP=0     no instalar yt-dlp
#    SHIN_GO_SIN_SERVICIO=1  compilar e instalar sin arrancar nada
#    SHIN_GO_DRY_RUN=1       dice qué haría, sin ejecutar
#    NO_COLOR=1              sin colores
set -eu

readonly REF="${SHIN_GO_REF:-main}"
REPO="${SHIN_GO_REPO:-riokuroxi-svg/Shin-MD-Go}"
DIR="${SHIN_GO_DIR:-$HOME/shin-md-go}"
GO_VERSION="1.26.0"
CON_YTDLP="${SHIN_GO_CON_YTDLP:-1}"
SIN_SERVICIO="${SHIN_GO_SIN_SERVICIO:-}"
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
  (cd "$DIR" && PATH="$HOME/.local/sdk/go/bin:$PATH" make build) \
    || morir "la compilación falló (mira arriba)"
  bien "$DIR/dist/shin"
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
  [ -z "$SIN_SERVICIO" ] || { dato "arranque desactivado (SHIN_GO_SIN_SERVICIO=1)"; return 0; }
  paso "Arranque"
  if [ "$TERMUX" = 1 ]; then
    if [ -z "$DRY_RUN" ]; then
      mkdir -p "$HOME/.termux/boot"
      cat > "$HOME/.termux/boot/shin" <<ARRANQUE
#!/data/data/com.termux/files/usr/bin/sh
cd "$PREFIX/bin" && exec "$PREFIX/bin/shin" >> "\$HOME/shin.log" 2>&1
ARRANQUE
      chmod +x "$HOME/.termux/boot/shin"
      bien "arranque al encender el teléfono, si tienes la app Termux:Boot (~/.termux/boot/shin)"
      dato "para arrancarlo ahora:  shin        (o ./shin desde $PREFIX/bin)"
    else
      dato "(dry-run) script de arranque de Termux"
    fi
    return 0
  fi
  if [ "$ROOT" = 1 ] && hay systemctl && [ -d /run/systemd/system ]; then
    if [ -z "$DRY_RUN" ]; then
      if [ -f "$DIR/deploy/shin.service" ]; then
        sed "s|/opt/shin|$DESTINO|g" "$DIR/deploy/shin.service" > /etc/systemd/system/shin.service
        systemctl daemon-reload
        systemctl enable --now shin >/dev/null 2>&1 || aviso "systemd lo registró pero no arrancó; mira: journalctl -u shin"
        bien "servicio systemd «shin» activo (reinicia solo si cae)"
      else
        aviso "no encontré deploy/shin.service; arranca a mano: $DESTINO/shin"
      fi
    else
      dato "(dry-run) unidad systemd desde deploy/shin.service"
    fi
  else
    dato "sin systemd ni root: arranca con  cd $DESTINO && ./shin"
    dato "(el bot pide el QR en pantalla la primera vez)"
  fi
}

# ─────────────────────────── main ───────────────────────────
paso "Shin-MD-Go · instalación en un comando"
dato "entorno: $ENTORNO · $ARQ"
[ -n "$DRY_RUN" ] && aviso "DRY-RUN: cuento qué haría, no ejecuto nada"

asegurar_git
asegurar_make
asegurar_go
asegurar_ytdlp
obtener_codigo
compilar
instalar
arrancar

paso "Listo"
if [ -n "$DRY_RUN" ]; then
  dato "dry-run terminado; nada se tocó"
else
  printf '\n  %s✓%s Shin-MD-Go quedó instalado.%s\n' "$VD" "$R" "$R"
  dato "primera vez: el bot muestra un QR en pantalla para vincular WhatsApp"
  dato "config: $DESTINO/.env · datos: $DESTINO/data/"
fi
