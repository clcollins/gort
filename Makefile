# GORT — GitOps Reconciliation Tool
# All container operations use $(CONTAINER_SUBSYS), defaulting to podman.

CONTAINER_SUBSYS ?= podman
IMAGE_REGISTRY   ?= quay.io/clcollins
IMAGE_REPO       ?= $(IMAGE_REGISTRY)/gort
VERSION          ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
IMAGE_TAG        ?= $(IMAGE_REPO):$(VERSION)

GOBIN            ?= $(shell go env GOPATH)/bin
CONTROLLER_GEN   ?= $(shell command -v controller-gen 2>/dev/null || echo "$(GOBIN)/controller-gen")
GOLANGCI_LINT    ?= $(shell command -v golangci-lint 2>/dev/null || echo "$(GOBIN)/golangci-lint")
CHECKMAKE        ?= $(shell command -v checkmake 2>/dev/null || echo "$(GOBIN)/checkmake")

GO               := go
GOFMT            := gofmt

# ── Build ──────────────────────────────────────────────────────────────────────

.PHONY: build
build: ## Build the gort binary
	$(GO) build -ldflags="-X main.version=$(VERSION)" -o bin/gort ./cmd/gort/...

.PHONY: all
all: fmt vet lint test image-build ## Run all checks and build the container image

# ── Test ───────────────────────────────────────────────────────────────────────

.PHONY: test
test: ## Run all Go tests with the race detector
	$(GO) test -race -count=1 -timeout=5m ./...

.PHONY: test-verbose
test-verbose: ## Run all Go tests with verbose output
	$(GO) test -race -count=1 -v -timeout=5m ./...

.PHONY: cover
cover: ## Run tests and emit coverage profile (coverage.out)
	$(GO) test -race -count=1 -timeout=5m -coverprofile=coverage.out -covermode=atomic ./...

# ── Code Quality ───────────────────────────────────────────────────────────────

.PHONY: fmt
fmt: ## Check formatting (gofmt)
	@echo "Checking gofmt..."
	@diff=$$($(GOFMT) -l .); if [ -n "$$diff" ]; then \
		echo "The following files are not formatted:"; echo "$$diff"; exit 1; \
	fi

.PHONY: fmt-fix
fmt-fix: ## Apply gofmt formatting
	$(GOFMT) -w .

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Run golangci-lint
	$(GOLANGCI_LINT) run ./...

MARKDOWNLINT ?= $(shell command -v markdownlint-cli2 2>/dev/null || echo "npx --yes markdownlint-cli2")

.PHONY: markdown-lint
markdown-lint: ## Lint all markdown files with markdownlint-cli2
	$(MARKDOWNLINT) "docs/**/*.md" "*.md"

.PHONY: makefile-lint
makefile-lint: $(CHECKMAKE) ## Lint this Makefile with checkmake
	$(CHECKMAKE) Makefile

.PHONY: yaml-lint
yaml-lint: ## Lint YAML files in config/
	yamllint -c .yamllint.yaml config/

.PHONY: promrules-check
promrules-check: ## Validate Prometheus alerting rules with promtool
	bash .github/scripts/validate-prometheus-rules.sh

.PHONY: containerfile-check
containerfile-check: ## Check Containerfile base image tags and registries
	bash .github/scripts/check-containerfile-tags.sh Containerfile
	bash .github/scripts/check-containerfile-tags.sh test/Containerfile.ci

# ── Documentation Check ────────────────────────────────────────────────────────

.PHONY: docs-check
docs-check: ## Fail if docs/plans/ contains no .md files
	@count=$$(find docs/plans -name '*.md' 2>/dev/null | wc -l); \
	[ "$$count" -gt 0 ] || \
		{ echo "ERROR: No plan documents found in docs/plans/. Every change must include a plan."; exit 1; }; \
	echo "OK: $$count plan document(s) found."

# ── Code Generation ────────────────────────────────────────────────────────────

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate DeepCopy methods for CRD types
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: $(CONTROLLER_GEN) ## Generate CRD manifests
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:artifacts:config=config/crd

# ── Container Image ────────────────────────────────────────────────────────────

.PHONY: image-build
image-build: ## Build the container image using $(CONTAINER_SUBSYS)
	$(CONTAINER_SUBSYS) build -t $(IMAGE_TAG) -f Containerfile .

.PHONY: image-push
image-push: ## Push the container image using $(CONTAINER_SUBSYS)
	$(CONTAINER_SUBSYS) push $(IMAGE_TAG)

# ── Local CI ───────────────────────────────────────────────────────────────────

CI_CONTAINER_FILE ?= test/Containerfile.ci
CI_IMAGE          ?= gort-ci:local

.PHONY: ci-build
ci-build: ## Build the CI container image
	$(CONTAINER_SUBSYS) build -f $(CI_CONTAINER_FILE) -t $(CI_IMAGE) test/

.PHONY: ci-all
ci-all: ci-build ## Build CI container and run ci-checks inside it (local entry point)
	$(CONTAINER_SUBSYS) run --rm --userns=keep-id -v $$(pwd):/src:Z -w /src $(CI_IMAGE) make ci-checks

.PHONY: ci-checks
ci-checks: tidy-check fmt vet cover lint build docs-check markdown-lint yaml-lint makefile-lint promrules-check containerfile-check ## Run all checks serially (intended to run inside the CI container)

# ── Dependencies / Tools ───────────────────────────────────────────────────────

.PHONY: tidy
tidy: ## Run go mod tidy
	$(GO) mod tidy

.PHONY: tidy-check
tidy-check: ## Verify go.mod and go.sum are tidy (no uncommitted changes after go mod tidy)
	$(GO) mod tidy
	git diff --exit-code go.mod go.sum

$(CONTROLLER_GEN):
	$(GO) install sigs.k8s.io/controller-tools/cmd/controller-gen@latest

$(GOLANGCI_LINT):
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6

$(CHECKMAKE):
	$(GO) install github.com/mrtazz/checkmake/cmd/checkmake@v0.2.2

# ── Clean ──────────────────────────────────────────────────────────────────────

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin/

# ── Help ───────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Print this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' | sort
