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

lint:
	golangci-lint run

cover: test
	go tool cover -html=coverage.out

deb-linux:
	$(eval MAIN_COUNT := $(shell git rev-list --count origin/main 2>/dev/null || git rev-list --count main))
	$(eval GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD))
	$(eval GIT_SHA := $(shell git rev-parse --short HEAD))
	$(eval DIRTY := $(shell git diff --quiet && git diff --cached --quiet || echo .dirty))
	$(eval VERSION := $(if $(filter main,$(GIT_BRANCH)),1.0.$(MAIN_COUNT)$(if $(DIRTY),~dirty),1.0.$(MAIN_COUNT)~$(GIT_SHA)$(DIRTY)))
	dch $(if $(filter main,$(GIT_BRANCH)),,-b) --newversion $(VERSION) --distribution unstable --force-distribution "git $(VERSION)"
	dpkg-buildpackage -us -uc -b $(DPKG_FLAGS)
	mkdir -p dist
	mv ../pvs-monitoring_*.deb dist/ || true

deb:
	mkdir -p dist
	chmod a+w dist
	docker build --platform linux/arm64 -t pvs-deb-builder -f Dockerfile.deb .
	docker run --rm --platform linux/arm64 \
		-v "$(CURDIR)/dist":/out \
		pvs-deb-builder sh -c \
		"cp -r /build /tmp/src && cd /tmp/src && make deb-linux DPKG_FLAGS=-d && cp dist/*.deb /out/"

