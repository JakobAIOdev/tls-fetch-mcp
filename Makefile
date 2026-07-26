BINARY := bin/tls-fetch-mcp
PACKAGE := ./cmd/tls-fetch-mcp
GO_FILES := cmd/tls-fetch-mcp/*.go internal/fetch/*.go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build check clean format format-check release-snapshot test test-integration test-race vet

build:
	mkdir -p bin
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(PACKAGE)

test:
	go test ./...

test-race:
	go test -race ./...

test-integration:
	@test -n "$$TLS_FETCH_INTEGRATION_URL" || (echo "TLS_FETCH_INTEGRATION_URL is required" && exit 1)
	go test ./internal/fetch -run TestIntegrationTLSFetch -v

vet:
	go vet ./...

format:
	gofmt -w $(GO_FILES)

format-check:
	@test -z "$$(gofmt -l $(GO_FILES))" || (echo "Go files need formatting; run 'make format'" && exit 1)

check: format-check test vet build

release-snapshot:
	sh scripts/build-release.sh "$(VERSION)"

clean:
	rm -rf bin dist
