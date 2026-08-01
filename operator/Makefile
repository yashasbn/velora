# Velora Operator Makefile
# Run targets from inside WSL2 from the operator/ directory.

# ---------------------------------------------------------------------------
# Config
# ---------------------------------------------------------------------------
BINARY_NAME        ?= velora-operator
IMG                ?= ghcr.io/yashasbn/velora-operator:latest
DAG_TRIGGER_IMG    ?= ghcr.io/yashasbn/velora-dag-trigger:latest
CONTROLLER_GEN     ?= $(shell which controller-gen 2>/dev/null || echo "go run sigs.k8s.io/controller-tools/cmd/controller-gen")
ENVTEST            ?= $(shell which setup-envtest 2>/dev/null || echo "go run sigs.k8s.io/controller-runtime/tools/setup-envtest")
GOBIN              ?= $(shell go env GOBIN)
ifeq ($(GOBIN),)
  GOBIN = $(shell go env GOPATH)/bin
endif

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------
.PHONY: build
build: generate fmt vet
	go build -o bin/$(BINARY_NAME) ./cmd/manager/

.PHONY: build-summarizer
build-summarizer: generate fmt vet
	go build -o bin/velora-summarizer ./cmd/summarizer/

.PHONY: build-all
build-all: build build-summarizer


.PHONY: run
run: generate fmt vet
	go run ./cmd/manager/ \
		--minio-endpoint=localhost:30090 \
		--minio-access-key=velora \
		--minio-secret-key=velora-minio-secret \
		--airflow-base-url=http://localhost:30081 \
		--airflow-username=admin \
		--airflow-password=admin

# ---------------------------------------------------------------------------
# Docker
# ---------------------------------------------------------------------------
.PHONY: docker-build
docker-build:
	docker build -t $(IMG) -f Dockerfile .

.PHONY: docker-push
docker-push:
	docker push $(IMG)

# ---------------------------------------------------------------------------
# Code generation
# ---------------------------------------------------------------------------
.PHONY: generate
generate: ## Generate DeepCopy methods for CRD types
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: ## Generate CRD manifests (CRD YAML, RBAC)
	$(CONTROLLER_GEN) crd rbac:roleName=velora-operator-role webhook paths="./..." \
		output:crd:artifacts:config=../config/crd/bases \
		output:rbac:artifacts:config=../config/rbac

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

# ---------------------------------------------------------------------------
# Testing
# ---------------------------------------------------------------------------
.PHONY: test
test: manifests generate fmt vet envtest
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use 1.30.0 --bin-dir $(GOBIN) -p path)" \
		go test ./tests/... -v -count=1 -timeout 120s

.PHONY: envtest
envtest: ## Download envtest binaries if not present
	@$(ENVTEST) use 1.30.0 --bin-dir $(GOBIN) -p path

# ---------------------------------------------------------------------------
# Install / Uninstall CRDs
# ---------------------------------------------------------------------------
.PHONY: install
install: manifests ## Install CRDs into the cluster pointed at by KUBECONFIG
	kubectl apply -f ../config/crd/bases/

.PHONY: uninstall
uninstall:
	kubectl delete -f ../config/crd/bases/ --ignore-not-found=true

# ---------------------------------------------------------------------------
# Deploy / Undeploy (applies Helm chart)
# ---------------------------------------------------------------------------
.PHONY: deploy
deploy:
	helm upgrade --install velora-operator ../helm/velora-operator \
		--namespace velora-system --create-namespace \
		--set image.repository=$(shell echo $(IMG) | cut -d: -f1) \
		--set image.tag=$(shell echo $(IMG) | cut -d: -f2)

.PHONY: undeploy
undeploy:
	helm uninstall velora-operator -n velora-system || true

# ---------------------------------------------------------------------------
# Linting
# ---------------------------------------------------------------------------
.PHONY: lint
lint:
	@command -v golangci-lint &>/dev/null || \
		(curl -sSfL https://raw.githubusercontent.com/golangci-lint/golangci-lint/master/install.sh | sh -s -- -b $(GOBIN) v1.59.0)
	golangci-lint run ./...

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------
.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
