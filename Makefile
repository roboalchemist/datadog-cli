BINARY    := datadog-cli
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || git rev-parse --short HEAD 2>/dev/null || echo "dev")
LDFLAGS   := -ldflags "-s -w -X main.version=$(VERSION)"
GOFLAGS   :=
COVFILE   := coverage.out

.PHONY: build clean test test-unit test-integration install dev-install \
        deps fmt lint man docs-gen release-snapshot release check update-specs help

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

## test-unit: Run unit tests (no build tags, no credentials required)
test-unit:
	go test ./...

## test-integration: Run integration tests against live Datadog API (requires DD_API_KEY and DD_APP_KEY)
test-integration:
	go test -tags integration -timeout 120s -v ./...

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

## update-specs: Fetch latest OpenAPI specs from DataDog/datadog-api-client-go
update-specs:
	@echo "Fetching Datadog OpenAPI specs from DataDog/datadog-api-client-go..."
	@mkdir -p specs
	@V1_INFO=$$(gh api "repos/DataDog/datadog-api-client-go/commits?path=.generator/schemas/v1/openapi.yaml&per_page=1" \
		--jq '.[0] | "\(.sha) \(.commit.author.date)"') && \
	V1_SHA=$$(echo $$V1_INFO | cut -d' ' -f1) && \
	V1_DATE=$$(echo $$V1_INFO | cut -d' ' -f2) && \
	gh api "repos/DataDog/datadog-api-client-go/contents/.generator/schemas/v1/openapi.yaml" \
		--header "Accept: application/vnd.github.raw+json" > specs/v1.yaml && \
	echo "  v1: commit $$V1_SHA ($$V1_DATE)"
	@V2_INFO=$$(gh api "repos/DataDog/datadog-api-client-go/commits?path=.generator/schemas/v2/openapi.yaml&per_page=1" \
		--jq '.[0] | "\(.sha) \(.commit.author.date)"') && \
	V2_SHA=$$(echo $$V2_INFO | cut -d' ' -f1) && \
	V2_DATE=$$(echo $$V2_INFO | cut -d' ' -f2) && \
	gh api "repos/DataDog/datadog-api-client-go/contents/.generator/schemas/v2/openapi.yaml" \
		--header "Accept: application/vnd.github.raw+json" > specs/v2.yaml && \
	echo "  v2: commit $$V2_SHA ($$V2_DATE)"
	@echo "Done. Updated specs/v1.yaml and specs/v2.yaml"

## check: Run fmt + lint + test + test-unit
check: fmt lint test test-unit

## help: Show this help message
help:
	@echo "datadog-cli Makefile"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^##' Makefile | sed 's/## /  /' | column -t -s ':'
