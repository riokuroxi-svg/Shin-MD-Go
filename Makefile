# Makefile — Shin-MD-Go
# Uso: make build · make termux · make test · make run

BIN      := shin
VERSION  := 0.26.0-rc1
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

.PHONY: build run test vet fmt termux linux windows release clean

build:
	go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BIN) ./cmd/shin

run:
	go run ./cmd/shin

# Binario estático para Termux/Android (sin cgo, sin dependencias)
termux:
	GOOS=linux GOARCH=arm64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BIN)-termux-arm64 ./cmd/shin

# VPS de toda la vida (x86_64, sin cgo: corre también en distros mínimas)
linux:
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o dist/$(BIN)-linux-amd64 ./cmd/shin

# Los dos binarios que se copian a mano (el de Termux y el del VPS)
release: termux linux

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
