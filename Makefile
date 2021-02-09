# Set Shell to bash, otherwise some targets fail with dash/zsh etc.
SHELL := /bin/bash

# Disable built-in rules
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --warn-undefined-variables
.SHELLFLAGS := -eu -o pipefail -c
.SUFFIXES:
.SECONDARY:

PROJECT_ROOT_DIR = .
include Makefile.vars.mk

.PHONY: all
all: lint test build ## Invokes lint, test & build

.PHONY: build
build: $(BINARY_NAME) ## Build binary
	@echo built '$(VERSION)'

$(BINARY_NAME):
	$(BUILD_CMD)

.PHONY: test
test: ## Run tests
	$(GOTEST) -v -cover ./...

.PHONY: run
run: ## Run against the configured Kubernetes cluster
	go run cmd/crossplane-service-broker/main.go

.PHONY: docker-build
docker-build: $(BINARY_NAME) ## Build the docker image
	DOCKER_BUILDKIT=1 docker build -t $(DOCKER_IMG) -t $(QUAY_IMG) -t $(E2E_IMG) --build-arg VERSION="$(VERSION)" .
	@echo built image $(IMAGE_NAME)

.PHONY: docker-push
docker-push: ## Push the docker image
	docker push $(DOCKER_IMG)
	docker push $(QUAY_IMG)

.PHONY: lint
lint: fmt vet lint_yaml  ## Invokes the fmt, vet and lint_yaml targets
	@echo 'Check for uncommitted changes ...'
	git diff --exit-code

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: lint_yaml
lint_yaml: $(YAML_FILES)
	$(YAMLLINT_DOCKER) -f parsable -c $(YAMLLINT_CONFIG) $(YAMLLINT_ARGS) -- $?

.PHONY: docs-serve
docs-serve:
	$(ANTORA_PREVIEW_CMD)

$(TESTBIN_DIR):
	mkdir $(TESTBIN_DIR)

# TODO(mw): something with this target is off, $@ should be used instead of $*.yaml but I can't seem to make it work.
$(TESTDATA_CRD_DIR)/%.yaml:
	curl -sSLo $@ https://raw.githubusercontent.com/crossplane/crossplane/$(CROSSPLANE_VERSION)/cluster/charts/crossplane/crds/$*.yaml

.PHONY: integration_test
integration_test: $(CROSSPLANE_CRDS) ## Run integration tests with envtest
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; \
	fetch_envtest_tools $(ENVTEST_ASSETS_DIR); \
	setup_envtest_env $(ENVTEST_ASSETS_DIR); \
	go test -tags=integration -v ./... -coverprofile cover.out

.PHONY: setup_e2e_test
setup_e2e_test: export KUBECONFIG = $(KIND_KUBECONFIG)
setup_e2e_test: $(KIND_BIN)
	@kubectl config use-context kind-$(KIND_CLUSTER)

.PHONY: run_kind
run_kind: export KUBECONFIG = $(KIND_KUBECONFIG)
run_kind: setup_e2e_test run

$(KIND_BIN): export KUBECONFIG = $(KIND_KUBECONFIG)
$(KIND_BIN): $(TESTBIN_DIR)
	curl -Lo $(KIND_BIN) "https://kind.sigs.k8s.io/dl/v$(KIND_VERSION)/kind-$$(uname)-amd64"
	chmod +x $(KIND_BIN)
	docker run -d -p "$(KIND_REGISTRY_PORT):5000" --name "$(KIND_REGISTRY_NAME)" docker.io/library/registry:2
	$(KIND_BIN) create cluster --name $(KIND_CLUSTER) --image kindest/node:$(KIND_NODE_VERSION) --config=e2e/kind-config.yaml
	@docker network connect "kind" "$(KIND_REGISTRY_NAME)" || true
	kubectl cluster-info

.PHONY: clean
clean: export KUBECONFIG = $(KIND_KUBECONFIG)
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	$(KIND_BIN) delete cluster --name $(KIND_CLUSTER) || true
	docker stop "$(KIND_REGISTRY_NAME)" || true
	docker rm "$(KIND_REGISTRY_NAME)" || true
	docker rmi "$(E2E_IMG)" || true
	rm -r testbin/ dist/ bin/ cover.out $(BINARY_NAME) || true
	$(MAKE) -C e2e clean

.PHONY: install_bats
install_bats:
	$(MAKE) -C e2e install_bats

e2e_test: docker-build
	$(MAKE) -C e2e run_bats -e KUBECONFIG=../$(KIND_KUBECONFIG)

.PHONY: help
help: ## Show this help
	@grep -E -h '\s##\s' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
