# Makefile for HAMi mock-device-plugin
#
# Run `make help` to see the available targets.

# ---- Variables (override on the command line, e.g. `make docker-build IMG=myrepo/mock:dev`) ----
REGISTRY    ?= projecthami
IMAGE_NAME  ?= mock-device-plugin
TAG         ?= latest
IMG         ?= $(REGISTRY)/$(IMAGE_NAME):$(TAG)

BINARY      ?= k8s-device-plugin
MAIN_PKG    ?= ./cmd/k8s-device-plugin
OUTPUT_DIR  ?= bin

GIT_DESCRIBE ?= $(shell git describe --always --long --dirty 2>/dev/null || echo "unknown")
LDFLAGS      ?= -X main.gitDescribe=$(GIT_DESCRIBE)

GO          ?= go
DOCKER      ?= docker
# Use `make ... DOCKER=nerdctl` on containerd-only hosts.

# Detect golangci-lint availability for the lint target.
GOLANGCI_LINT := $(shell command -v golangci-lint 2>/dev/null)

.DEFAULT_GOAL := help

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	$(GO) vet ./...

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum.
	$(GO) mod tidy

.PHONY: lint
lint: ## Run golangci-lint (if installed).
ifeq ($(GOLANGCI_LINT),)
	@echo "golangci-lint not found; install: https://golangci-lint.run/usage/install/ — skipping."
else
	golangci-lint run ./...
endif

.PHONY: test
test: ## Run unit tests.
	$(GO) test ./...

.PHONY: test-verbose
test-verbose: ## Run unit tests with verbose output.
	$(GO) test -v ./...

.PHONY: cover
cover: ## Run unit tests and report coverage.
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out

##@ Build

.PHONY: build
build: vet ## Build the device-plugin binary into bin/. (run `make fmt` separately to format)
	mkdir -p $(OUTPUT_DIR)
	CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o $(OUTPUT_DIR)/$(BINARY) $(MAIN_PKG)

.PHONY: clean
clean: ## Remove build artifacts.
	rm -rf $(OUTPUT_DIR) coverage.out

##@ Container

.PHONY: docker-build
docker-build: ## Build the container image ($(IMG)).
	$(DOCKER) build -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push the container image ($(IMG)).
	$(DOCKER) push $(IMG)

##@ Deployment (requires kubectl + a valid KUBECONFIG)

.PHONY: deploy
deploy: ## Deploy RBAC + DaemonSet to the current cluster.
	kubectl apply -f k8s-mock-rbac.yaml
	kubectl apply -f k8s-mock-plugin.yaml

.PHONY: undeploy
undeploy: ## Remove the DaemonSet + RBAC from the current cluster.
	kubectl delete -f k8s-mock-plugin.yaml --ignore-not-found
	kubectl delete -f k8s-mock-rbac.yaml --ignore-not-found
