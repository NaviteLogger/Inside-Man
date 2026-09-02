# Inside Man — single entry point for local development.
#
# Every target runs against the pinned toolchain in .tools/bin; nothing depends
# on what happens to be installed system-wide.

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

.PHONY: cluster
cluster: bootstrap ## Create the local kind cluster (idempotent)
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

.PHONY: up
up: cluster ## Install/upgrade the umbrella chart onto the cluster
	@helm upgrade --install $(RELEASE) $(CHART) \
		--namespace $(NAMESPACE) --create-namespace \
		--wait --timeout 15m

.PHONY: down
down: ## Delete the local kind cluster
	@kind delete cluster --name $(CLUSTER) || true
	@rm -f $(KUBECONFIG)

.PHONY: lint
lint: bootstrap ## Lint the chart and shell scripts
	@helm lint $(CHART)
	@bash -n scripts/*.sh

.PHONY: template
template: bootstrap ## Render the chart to stdout (no cluster needed)
	@helm template $(RELEASE) $(CHART) --namespace $(NAMESPACE)

.PHONY: secrets
secrets: bootstrap ## Scan the working tree and history for leaked secrets
	@gitleaks dir . --config .gitleaks.toml --no-banner --redact
	@gitleaks git . --config .gitleaks.toml --no-banner --redact

.PHONY: e2e
e2e: up ## Full end-to-end verification against the live cluster
	@bash scripts/e2e.sh
