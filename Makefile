# BuildKit Operator for BuilderHub
# Uses Kubebuilder v4 / controller-runtime v0.18

# Image
REGISTRY ?= ghcr.io/builderhub
IMAGE ?= build-operator
TAG ?= latest
IMG ?= $(REGISTRY)/$(IMAGE):$(TAG)

# Go
GO ?= go
GOFLAGS ?=
GOPROXY ?= https://proxy.golang.org,direct

# Tools
CONTROLLER_GEN ?= controller-gen
KUSTOMIZE ?= kustomize

# Directories
BIN_DIR ?= bin

.PHONY: all
all: build

##@ General
.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development
.PHONY: generate manifests
generate manifests: ## Generate CRDs, RBAC, webhooks
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases output:rbac:dir=config/rbac output:webhook:dir=config/webhook

.PHONY: fmt
fmt: ## Run go fmt
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: test
test: fmt vet ## Run tests
	$(GO) test ./... -coverprofile=coverage.out

.PHONY: test-envtest
test-envtest: manifests ## Run envtest unit tests
	KUBEBUILDER_ASSETS="$(shell $(GO) run sigs.k8s.io/controller-runtime/tools/setup-envtest@latest use -i -p path)" $(GO) test ./... -v -count=1

##@ Build
.PHONY: build
build: generate ## Build manager binary
	$(GO) build -o $(BIN_DIR)/manager cmd/main.go

.PHONY: run
run: manifests generate ## Run against the configured Kubernetes cluster (--leader-elect=false for local dev)
	$(GO) run ./cmd/main.go --leader-elect=false

##@ Docker
.PHONY: docker-build
docker-build: ## Build Docker image
	docker build -t $(IMG) .

.PHONY: docker-push
docker-push: ## Push Docker image
	docker push $(IMG)

##@ Deployment
.PHONY: install
install: manifests ## Install CRDs into the cluster
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests ## Uninstall CRDs from the cluster
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

.PHONY: deploy
deploy: manifests ## Deploy operator to the cluster
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy operator from the cluster
	$(KUSTOMIZE) build config/default | kubectl delete -f -

##@ Kind
.PHONY: kind-create
kind-create: ## Create kind cluster
	kind create cluster --name builderhub

.PHONY: kind-delete
kind-delete: ## Delete kind cluster
	kind delete cluster --name builderhub

.PHONY: kind-load
kind-load: docker-build ## Load image into kind
	kind load docker-image $(IMG) --name builderhub
