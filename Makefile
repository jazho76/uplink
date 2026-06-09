BINARY  := vmm
PKG     := github.com/jazho76/vmm/internal/version

VERSION := $(shell git describe --tags --match 'v*' --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS := -X $(PKG).Version=$(VERSION) -X $(PKG).Commit=$(COMMIT) -X $(PKG).Date=$(DATE)

.PHONY: build dist vet test tidy install clean release

build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

dist:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64 .

vet:
	go vet ./...

test:
	go test ./...

tidy:
	go mod tidy

install:
	go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/$(BINARY) .
	@echo "installed $(CURDIR)/$(BINARY)"

clean:
	rm -rf bin

release:
	@echo "$(VERSION)" | grep -qE '^v[0-9]+\.[0-9]+\.[0-9]+$$' || { echo "Pass a semver: make release VERSION=v0.1.0"; exit 1; }
	git tag $(VERSION)
	git push origin $(VERSION)
