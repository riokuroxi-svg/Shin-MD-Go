# Makefile — Shin-MD-Go
# Uso: make build · make termux · make test · make run

BIN      := shin
VERSION  := 0.8.0
LDFLAGS  := -s -w -X main.version=$(VERSION)
GOFLAGS  := -trimpath

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
