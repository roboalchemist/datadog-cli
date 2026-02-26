BINARY    := datadog-cli
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS   := -ldflags "-s -w -X main.version=$(VERSION)"
GOFLAGS   :=
COVFILE   := coverage.out

.PHONY: build clean test test-unit test-integration install dev-install \
        deps fmt lint man docs-gen release-snapshot release check help

## build: Compile the binary to ./datadog-cli
build:
	go build $(LDFLAGS) $(GOFLAGS) -o $(BINARY) .

## clean: Remove build artifacts
clean:
	rm -f $(BINARY) $(COVFILE) coverage.html
	rm -rf dist/ man/

## test: Run smoke tests (no API key required)
test:
	go test ./... -run TestSmoke -v -timeout 30s

## test-unit: Run unit tests with race detector and 90%+ coverage
test-unit:
	go test ./... -race -coverprofile=$(COVFILE) -covermode=atomic -timeout 120s
	go tool cover -func=$(COVFILE) | tail -1

## test-integration: Run integration tests against mock server
test-integration:
	go test ./... -run TestIntegration -v -timeout 120s -tags integration

## install: Install binary to /usr/local/bin (requires sudo)
install: build
	sudo install -m 0755 $(BINARY) /usr/local/bin/$(BINARY)

## dev-install: Install binary via symlink for development
dev-install: build
	ln -sf $(PWD)/$(BINARY) /usr/local/bin/$(BINARY)

## deps: Download and tidy dependencies
deps:
	go mod download
	go mod tidy

## fmt: Format Go source files
fmt:
	gofmt -w .
	go mod tidy

## lint: Run golangci-lint
lint:
	golangci-lint run ./...

## man: Generate man pages
man:
	mkdir -p man
	go run ./cmd/gendocs man

## docs-gen: Generate markdown documentation
docs-gen:
	mkdir -p docs
	go run ./cmd/gendocs docs

## release-snapshot: Build release snapshot with goreleaser
release-snapshot:
	goreleaser release --snapshot --clean

## release: Create a new release (requires GITHUB_TOKEN)
release:
	@echo "Release via Gitea CI/CD pipeline."
	@echo "Tag the commit: git tag v$(VERSION) && git push origin v$(VERSION)"

## check: Run fmt + lint + test + test-unit
check: fmt lint test test-unit

## help: Show this help message
help:
	@echo "datadog-cli Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^##' Makefile | sed 's/## /  /' | column -t -s ':'
