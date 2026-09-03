# Inside Man development entry point.
#
# Every target uses the pinned toolchain in .tools/bin, so nothing depends on
# what happens to be installed system-wide.

SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

REPO_ROOT   := $(shell pwd)
TOOLS_BIN   := $(REPO_ROOT)/.tools/bin
export PATH := $(TOOLS_BIN):$(PATH)

CLUSTER     := inside-man
NAMESPACE   := inside-man
RELEASE     := inside-man
CHART       := charts/inside-man
KIND_IMAGE  := kindest/node:v1.36.4
KUBECONFIG  := $(REPO_ROOT)/kubeconfig
export KUBECONFIG

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nInside Man\n\nUsage: make <target>\n\n"} \
		/^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@echo

.PHONY: bootstrap
bootstrap: ## Install the pinned toolchain into .tools/bin
	@bash scripts/bootstrap.sh

# Targets ask for the tools they actually use, so a chart lint does not pull a
# Go toolchain down from go.dev.
.PHONY: bootstrap-cluster
bootstrap-cluster:
	@bash scripts/bootstrap.sh kubectl kind helm

.PHONY: bootstrap-helm
bootstrap-helm:
	@bash scripts/bootstrap.sh helm

.PHONY: bootstrap-gitleaks
bootstrap-gitleaks:
	@bash scripts/bootstrap.sh gitleaks

.PHONY: cluster
cluster: bootstrap-cluster ## Create the local kind cluster (idempotent)
	@if kind get clusters 2>/dev/null | grep -qx "$(CLUSTER)"; then \
		echo "cluster $(CLUSTER) already exists"; \
	else \
		kind create cluster --config scripts/kind-cluster.yaml --image $(KIND_IMAGE) --wait 120s; \
	fi
	@kind export kubeconfig --name $(CLUSTER)
	@kubectl cluster-info

.PHONY: deps
deps: bootstrap ## Resolve and vendor chart dependencies into $(CHART)/charts
	@helm dependency update $(CHART)

.PHONY: images
images: cluster ## Build the BFF and UI images and load them into the cluster
	@docker build -q -t inside-man-bff:local bff > /dev/null
	@docker build -q -t inside-man-ui:local ui > /dev/null
	@kind load docker-image inside-man-bff:local inside-man-ui:local --name $(CLUSTER) > /dev/null

.PHONY: up
up: images ## Install/upgrade the umbrella chart onto the cluster
	@helm upgrade --install $(RELEASE) $(CHART) \
		--namespace $(NAMESPACE) --create-namespace \
		--wait --timeout 15m

.PHONY: down
down: ## Delete the local kind cluster
	@kind delete cluster --name $(CLUSTER) || true
	@rm -f $(KUBECONFIG)

.PHONY: lint
lint: bootstrap-helm ## Lint the chart, shell scripts, Go and TypeScript
	@helm lint $(CHART)
	@bash -n scripts/*.sh
	@cd bff && gofmt -l . | (! grep .) && go vet ./...
	@cd ui && npm ci --silent && npm run typecheck

.PHONY: test
test: ## Run the BFF and UI unit tests
	@cd bff && go test ./...
	@cd ui && npm ci --silent && npm test

.PHONY: template
template: bootstrap-helm ## Render the chart to stdout (no cluster needed)
	@helm template $(RELEASE) $(CHART) --namespace $(NAMESPACE)

.PHONY: secrets
secrets: bootstrap-gitleaks ## Scan the working tree and history for leaked secrets
	@gitleaks dir . --config .gitleaks.toml --no-banner --redact
	@gitleaks git . --config .gitleaks.toml --no-banner --redact

.PHONY: demo
demo: up ## Build, load and deploy the demo app onto the cluster
	@for s in frontend api backend; do \
		echo "building demo-$$s"; \
		docker build -q -t demo-$$s:local examples/demo-app/$$s > /dev/null; \
		kind load docker-image demo-$$s:local --name $(CLUSTER) > /dev/null; \
	done
	@kubectl apply -f examples/demo-app/manifests.yaml
	@kubectl wait --for=condition=Available deploy --all -n demo --timeout=5m

.PHONY: e2e
e2e: demo ## Full end-to-end verification against the live cluster
	@bash scripts/e2e.sh
