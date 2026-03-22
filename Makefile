# GORT — GitOps Reconciliation Tool
# All container operations use $(CONTAINER_ENGINE), defaulting to podman.

CONTAINER_ENGINE ?= podman
IMAGE_REGISTRY   ?= quay.io/chcollin
IMAGE_REPO       ?= $(IMAGE_REGISTRY)/gort
VERSION          ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
IMAGE_TAG        ?= $(IMAGE_REPO):$(VERSION)

GOBIN            ?= $(shell go env GOPATH)/bin
CONTROLLER_GEN   ?= $(GOBIN)/controller-gen
GOLANGCI_LINT    ?= $(GOBIN)/golangci-lint
CHECKMAKE        ?= $(GOBIN)/checkmake

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

.PHONY: markdown-lint
markdown-lint: ## Lint all markdown files with markdownlint-cli2
	npx --yes markdownlint-cli2 "docs/**/*.md" "*.md"

.PHONY: makefile-lint
makefile-lint: $(CHECKMAKE) ## Lint this Makefile with checkmake
	$(CHECKMAKE) Makefile

# ── Documentation Check ────────────────────────────────────────────────────────

.PHONY: docs-check
docs-check: ## Fail if docs/plans/ contains no .md files
	@echo "Checking for plan documents in docs/plans/..."
	@count=$$(find docs/plans -name '*.md' 2>/dev/null | wc -l); \
	if [ "$$count" -eq 0 ]; then \
		echo "ERROR: No plan documents found in docs/plans/. Every change must include a plan."; \
		exit 1; \
	fi
	@echo "OK: $$count plan document(s) found."

# ── Code Generation ────────────────────────────────────────────────────────────

.PHONY: generate
generate: $(CONTROLLER_GEN) ## Generate DeepCopy methods for CRD types
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: $(CONTROLLER_GEN) ## Generate CRD manifests
	$(CONTROLLER_GEN) crd paths="./api/..." output:crd:artifacts:config=config/crd

# ── Container Image ────────────────────────────────────────────────────────────

.PHONY: image-build
image-build: ## Build the container image using $(CONTAINER_ENGINE)
	$(CONTAINER_ENGINE) build -t $(IMAGE_TAG) -f Containerfile .

.PHONY: image-push
image-push: ## Push the container image using $(CONTAINER_ENGINE)
	$(CONTAINER_ENGINE) push $(IMAGE_TAG)

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
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

$(CHECKMAKE):
	$(GO) install github.com/mrtazz/checkmake/cmd/checkmake@latest

# ── Help ───────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Print this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' | sort
