BIN_DIR := bin

.PHONY: build test lint cover fmt deb

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

deb:
	dpkg-buildpackage -us -uc -b
	mkdir -p dist
	mv ../pvs-monitoring_*.deb dist/ || true

