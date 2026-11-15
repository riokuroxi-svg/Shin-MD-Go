#!/bin/sh
# Shin-MD-Go — Copyright (C) 2026 riokuroxi-svg · AGPL-3.0-only
#
# dev-setup.sh — deja el entorno listo para compilar en una máquina limpia.
#
# No es para el bot en producción (usa `make build` o el Dockerfile), es para
# trabajar el código: instala el Go que pide el módulo, apunta las cachés fuera
# del árbol y compila una vez para calentar.
#
# Por qué las cachés van a .cache y no al directorio del proyecto:
#   - el módulo de SQLite puro pesa ~250 MB y el árbol de Go otros ~250 MB;
#   - si eso vive dentro del repo, cada copia de seguridad del proyecto arrastra
#     medio giga de dependencias, y en un contenedor con /tmp en RAM se llena el
#     disco a la primera compilación (nos pasó: los errores salían como
#     «cannot write: no space left on device» en mitad de un paquete cualquiera,
#     que se lee como un fallo del código y no del disco).
#
# Uso:
#   sh deploy/dev-setup.sh          # instala y compila
#   . deploy/dev-setup.sh && go test ./...
set -eu

GO_VERSION="${GO_VERSION:-1.27.3}"
RAIZ="$(cd "$(dirname "$0")/.." && pwd)"
DESTINO="${GO_INSTALL_DIR:-/tmp/go}"
CACHE="${XDG_CACHE_HOME:-$HOME/.cache}"

echo "→ Go $GO_VERSION en $DESTINO"
if [ ! -x "$DESTINO/bin/go" ]; then
    arch="$(uname -m)"
    case "$arch" in
        x86_64) arco=amd64 ;;
        aarch64 | arm64) arco=arm64 ;;
        *) echo "✕ Arquitectura no soportada: $arch" >&2; exit 1 ;;
    esac
    tmp="$(mktemp -d)"
    curl -sSL -o "$tmp/go.tgz" "https://dl.google.com/go/go${GO_VERSION}.linux-${arco}.tar.gz"
    mkdir -p "$DESTINO"
    tar -C "$DESTINO" --strip-components=1 -xzf "$tmp/go.tgz"
    rm -rf "$tmp"
fi
"$DESTINO/bin/go" version

export PATH="$DESTINO/bin:$PATH"
export GOMODCACHE="$CACHE/gomod"
export GOCACHE="$CACHE/gobuild"
# El compilador usa $TMPDIR para los objetos intermedios. Si /tmp es tmpfs
# (RAM), un build de este proyecto se lo come; por eso va a .cache.
export TMPDIR="$CACHE/tmp"
mkdir -p "$GOMODCACHE" "$GOCACHE" "$TMPDIR"

cd "$RAIZ"
echo "→ Descargando módulos"
go mod download all

echo "→ Compilando"
go build ./...

echo "→ Listo: go test ./... y go vet ./..."
