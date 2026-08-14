MODULE  := github.com/JohnSBarden/footui
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X '$(MODULE)/internal/cli.Version=$(VERSION)'

.PHONY: build test lint run clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/footy ./cmd/footy

test:
	go test ./...

lint:
	golangci-lint run ./...

run: build
	./bin/footy

clean:
	rm -rf bin
