BIN_DIR := bin

.PHONY: build test lint cover fmt deb deb-linux

fmt:
	goimports -local github.com/dangogh -w .

build:
	go build -o $(BIN_DIR)/pvs-monitor ./cmd/pvs-monitor
	go build -o $(BIN_DIR)/pvs-mcp ./cmd/pvs-mcp
	go build -o $(BIN_DIR)/pvs-api ./cmd/pvs-api
	go build -o $(BIN_DIR)/pvs-ui ./cmd/pvs-ui

test:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | grep '^total:'
	npm test

lint:
	golangci-lint run

cover: test
	go tool cover -html=coverage.out

DEB_ARCH ?= arm64

deb-linux:
	$(eval GIT_TAG := $(shell git describe --tags --match 'v[0-9]*' 2>/dev/null | sed 's/^v//'))
	$(eval GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD))
	$(eval DIRTY := $(shell git diff --quiet && git diff --cached --quiet || echo ~dirty))
	$(eval VERSION := $(GIT_TAG)$(DIRTY))
	dch -b --newversion $(VERSION) --distribution unstable --force-distribution "git $(VERSION)"
	dpkg-buildpackage -us -uc -b -a$(DEB_ARCH) $(DPKG_FLAGS)
	mkdir -p dist
	mv ../pvs-monitoring_*.deb dist/ || true

# Cross-compiles natively (no QEMU/Docker needed since the Go build has no
# cgo dependency) for DEB_ARCH, defaulting to arm64.
deb:
	mkdir -p dist
	$(MAKE) deb-linux DPKG_FLAGS=-d

