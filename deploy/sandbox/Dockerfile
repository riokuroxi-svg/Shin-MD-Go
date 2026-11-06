# Shin-MD-Go — imagen de ejecución
#
# Multi-etapa: se compila con el toolchain completo y la imagen final solo lleva
# el binario estático. Sin shell, sin compilador, sin paquetes de más: menos
# superficie, menos peso (de ~700 MB a ~40 MB).

# ── etapa 1: compilar ────────────────────────────────────────────────────
FROM golang:1.27-alpine AS build

WORKDIR /src

# Las dependencias se copian primero: mientras no cambien, la capa de descarga
# se reutiliza y la compilación no vuelve a bajar el mundo entero.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 porque SQLite es modernc (Go puro): el binario queda estático y
# corre igual en Alpine, Debian o Termux.
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/shin ./cmd/shin && \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/shin-musica ./cmd/shin-musica

# ── etapa 2: ejecutar ────────────────────────────────────────────────────
FROM alpine:3.21

# ffmpeg para stickers y recortes; ca-certificates para HTTPS; tzdata para que
# las fechas del bot no salgan en UTC.
RUN apk add --no-cache ffmpeg ca-certificates tzdata && \
    adduser -D -u 10001 shin

COPY --from=build /out/shin /usr/local/bin/shin
COPY --from=build /out/shin-musica /usr/local/bin/shin-musica

# La sesión de WhatsApp vive en /data: es el único volumen que hay que conservar.
RUN mkdir -p /data && chown -R shin:shin /data
VOLUME ["/data"]

USER shin
WORKDIR /data

ENV STATE_DIR=/data \
    TZ=America/Mexico_City

# No se expone ningún puerto: este bot no escucha, solo habla con WhatsApp.
ENTRYPOINT ["/usr/local/bin/shin"]
