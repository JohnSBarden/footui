MODULE  := github.com/JohnSBarden/footui
BINARY  := footy
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -X '$(MODULE)/internal/cli.Version=$(VERSION)'

# The SQLite driver is modernc.org/sqlite, a pure-Go implementation, which
# is the whole reason CGO_ENABLED=0 works here: footy ships as one static
# binary with no libsqlite3 to find at runtime.
GOFLAGS_STATIC := CGO_ENABLED=0

# Platforms built by `make release`.
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64

.PHONY: build install static release test cover fmt vet lint check clean

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/$(BINARY)

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/$(BINARY)

# A stripped, reproducible static binary. -trimpath keeps build paths out
# of the output so the same source produces the same bytes anywhere.
static:
	$(GOFLAGS_STATIC) go build -trimpath -ldflags "$(LDFLAGS) -s -w" -o bin/$(BINARY) ./cmd/$(BINARY)

release:
	@rm -rf dist && mkdir -p dist
	@for platform in $(PLATFORMS); do \
		os=$${platform%/*}; arch=$${platform#*/}; \
		echo "building $$os/$$arch"; \
		$(GOFLAGS_STATIC) GOOS=$$os GOARCH=$$arch \
			go build -trimpath -ldflags "$(LDFLAGS) -s -w" \
			-o dist/$(BINARY)-$$os-$$arch ./cmd/$(BINARY) || exit 1; \
	done
	@echo && ls -lh dist

test:
	go test ./...

cover:
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

fmt:
	gofmt -l -w .

vet:
	go vet ./...

lint:
	golangci-lint run ./...

# What CI should run.
check: vet test
	@test -z "$$(gofmt -l .)" || { echo "unformatted files:"; gofmt -l .; exit 1; }

clean:
	rm -rf bin dist coverage.out
