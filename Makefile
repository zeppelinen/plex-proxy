GO ?= go
BINARY := plex-proxy
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X github.com/zeppelinen/plex-proxy/internal/version.Version=$(VERSION) -X github.com/zeppelinen/plex-proxy/internal/version.Commit=$(COMMIT) -X github.com/zeppelinen/plex-proxy/internal/version.Date=$(DATE)

.PHONY: all test race vet fmt build release e2e clean

all: fmt vet test build

fmt:
	@test -z "$$($(GO)fmt -l .)"

vet:
	$(GO) vet ./...

test:
	$(GO) test ./...

race:
	$(GO) test -race ./...

build:
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) ./cmd/plex-proxy

release:
	./scripts/build-release.sh

e2e:
	./test/e2e/run.sh

clean:
	rm -rf bin dist
