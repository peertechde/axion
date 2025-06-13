# Verbosity control (V=1 for verbose output)
V = 1
Q = $(if $(filter 1,$V),,@)
M = $(shell if [ "$$(tput colors 2> /dev/null || echo 0)" -ge 8 ]; then printf "\033[34;1m▶\033[0m"; else printf "▶"; fi)

MODULE = $(shell $(GO) list -m)
PKGS = $(or $(PKG),$(shell $(GO) list ./...))
BIN  = bin

# Version and build information
DATE ?= $(shell date -u +%FT%T%z)
VERSION ?= $(shell git describe --tags --always --dirty --match=v* 2> /dev/null || cat .version 2> /dev/null || echo v0)

GO = go
GO_GOOS = GOOS=linux
GO_ARCH = GOARCH=amd64

GO_BUILD_LDFLAGS = -s -w -extldflags '-static' \
	-X $(MODULE)/pkg/version.Version=$(VERSION) \
	-X $(MODULE)/pkg/version.BuildDate=$(DATE)

GO_BUILD_FLAGS = -trimpath -a -ldflags "$(GO_BUILD_LDFLAGS)"
GO_BUILD = CGO_ENABLED=0 $(GO_GOOS) $(GO_ARCH) $(GO) build $(GO_BUILD_FLAGS)

# Tools
SWAGGER = swagger
GOFMT = gofmt

# =============================================================================
# Build Targets
# =============================================================================

.PHONY: build-axionctl
build-axionctl: $(BIN) ## Build axionctl CLI binary
	$(info $(M) building axionctl...)
	$Q $(GO_BUILD) -o $(BIN)/axionctl $(MODULE)/cmd/axionctl

.PHONY: build-axiond
build-axiond: $(BIN) ## Build axiond server binary
	$(info $(M) building axiond...)
	$Q $(GO_BUILD) -o $(BIN)/axiond $(MODULE)/cmd/axiond

$(BIN):
	@mkdir -p $@

# =============================================================================
# Testing Targets
# =============================================================================

.PHONY: test
test: ## Run all tests
	$(info $(M) running tests...)
	$Q $(GO) test -race -cover ./...

.PHONY: test-verbose
test-verbose: ## Run tests with verbose output
	$(info $(M) running tests with verbose output...)
	$Q $(GO) test -race -cover -v ./...

# =============================================================================
# Code Generation
# =============================================================================

.PHONY: check-tools
check-tools: ## Check if required tools are installed
	$(info $(M) checking for required tools...)
	@which $(SWAGGER) > /dev/null || (echo "Error: swagger is not installed" && exit 1)

.PHONY: generate-swagger
generate-swagger: check-tools api/openapi.yaml ## Generate server, client and model code from OpenAPI spec
	$(info $(M) generating code from OpenAPI specification...)
	$Q $(SWAGGER) generate server \
		-f api/openapi.yaml \
		-t api/ \
		--exclude-main
	$Q $(SWAGGER) generate client \
		-f api/openapi.yaml \
		-t api/

.PHONY: validate-swagger
validate-swagger: check-tools api/openapi.yaml ## Validate OpenAPI specification
	$(info $(M) validating OpenAPI specification...)
	$Q $(SWAGGER) validate api/openapi.yaml

# =============================================================================
# Utilities
# =============================================================================

.PHONY: fmt
fmt: ## Format Go code
	$(info $(M) formatting Go code...)
	$Q $(GOFMT) -s -w $(shell find . -name "*.go" -not -path "./vendor/*" -not -path "./api/*")

.PHONY: clean
clean: ## Clean up
	$(info $(M) cleaning...)
	$Q rm -rf $(BIN)
	$Q rm -rf api/models api/client api/restapi

.PHONY: help
help: ## Show available targets
	@grep -hE '^[ a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-17s\033[0m %s\n", $$1, $$2}'

.PHONY: version
version: ## Show version
	@echo $(VERSION)

.DEFAULT_GOAL := help