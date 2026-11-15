# Makefile — Shin-MD-Go
# Uso: make build · make termux · make test · make run

BIN      := shin
VERSION  := 0.19.0
LDFLAGS  := -s -w -X main.version=$(VERSION)
GOFLAGS  := -trimpath

# Termux (Android): varios kernels llevan un filtro seccomp que mata con
# SIGSYS la syscall faccessat2, y cmd/go la dispara al buscar gcc/git en el
# PATH durante el build. Este módulo es Go puro: CGO_ENABLED=0 elimina la
# búsqueda de gcc y -buildvcs=false la de git, y el binario sale igual.
ifneq ($(TERMUX_VERSION)$(filter /data/data/com.termux/%,$(PREFIX)),)
export CGO_ENABLED=0
GOFLAGS += -buildvcs=false
endif

.PHONY: build run test vet fmt termux windows clean

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BIN) ./cmd/shin

run:
	go run ./cmd/shin

# Binario estático para Termux/Android (sin cgo, sin dependencias)
termux:
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BIN)-termux-arm64 ./cmd/shin

windows:
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BIN).exe ./cmd/shin

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

clean:
	rm -rf dist
