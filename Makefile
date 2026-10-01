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

.PHONY: build install static release test race cover fmt fmtcheck vet lint tidycheck check clean

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

# The race detector needs cgo, unlike every other target here.
race:
	CGO_ENABLED=1 go test -race ./...

cover:
	go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out | tail -1

# Only tracked Go files: a checkout can hold untracked copies of the tree
# (git worktrees under .claude/) that aren't this change's business.
GOFILES = $(shell git ls-files '*.go')

fmt:
	gofmt -l -w $(GOFILES)

fmtcheck:
	@out="$$(gofmt -l $(GOFILES))"; test -z "$$out" || { echo "unformatted files:"; echo "$$out"; exit 1; }

vet:
	go vet ./...

# staticcheck is pinned and run through `go run`, so neither a developer
# nor CI needs it installed.
STATICCHECK := honnef.co/go/tools/cmd/staticcheck@v0.8.1

lint:
	go run $(STATICCHECK) ./...

# go.mod and go.sum must already be tidy; a dependency added without
# `go mod tidy` fails here rather than in someone else's checkout.
tidycheck:
	@cp go.mod go.mod.bak && cp go.sum go.sum.bak; \
	go mod tidy; \
	status=0; cmp -s go.mod go.mod.bak && cmp -s go.sum go.sum.bak || { echo "go.mod/go.sum not tidy — run go mod tidy"; status=1; }; \
	mv go.mod.bak go.mod; mv go.sum.bak go.sum; exit $$status

# What CI runs. Run it before pushing.
check: fmtcheck vet lint tidycheck race

clean:
	rm -rf bin dist coverage.out
