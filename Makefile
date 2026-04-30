.PHONY: build build-windows build-web rebuild rebuild-pp rebuild-web test lint download-geo release clean clean-bin

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo "dev")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
COMMIT ?= $(shell git rev-parse --verify HEAD 2>/dev/null || echo "none")
GOCACHE ?= $(CURDIR)/.cache/go-build

LDFLAGS := -s -w \
	-X main.version=$(VERSION) \
	-X main.buildDate=$(DATE) \
	-X main.gitCommit=$(COMMIT)

build:
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/pp-core ./cmd/pp-core
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o bin/pp-client ./cmd/pp-client

build-windows:
	GOCACHE=$(GOCACHE) CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o bin/pp-client.exe ./cmd/pp-client

build-web:
	GOCACHE=$(GOCACHE) CGO_ENABLED=1 go build -ldflags="$(LDFLAGS)" -o bin/pp-web ./cmd/pp-web

rebuild:
	bash scripts/rebuild.sh

rebuild-pp:
	bash scripts/rebuild.sh pp

rebuild-web:
	bash scripts/rebuild.sh pp-web

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

download-geo:
	bash scripts/download-geo.sh

release:
	goreleaser release --clean

clean-bin:
	rm -f bin/pp-core bin/pp-client bin/pp-web bin/pp-client.exe

clean:
	rm -rf bin/ dist/ pp-web/frontend/dist
