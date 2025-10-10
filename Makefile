# Set Shell to bash, otherwise some targets fail with dash/zsh etc.
SHELL := /usr/bin/env bash

# Disable built-in rules
MAKEFLAGS += --no-builtin-rules
MAKEFLAGS += --no-builtin-variables
.SUFFIXES:
.SECONDARY:

PROJECT_ROOT_DIR = .
include Makefile.vars.mk

e2e_make := $(MAKE) -C e2e
go_build ?= CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v \
				-o $(BIN_FILENAME) \
				-ldflags "-X main.version=$(VERSION)" \
				cmd/crossplane-service-broker/main.go

# Run tests (see https://sdk.operatorframework.io/docs/building-operators/golang/references/envtest-setup)
ENVTEST_ASSETS_DIR=$(shell pwd)/testdata

all: lint test build ## Invokes the lint, test & build targets

.PHONY: test
test: ## Run tests
	go test -v ./... -coverprofile cover.out

integration-test: export ENVTEST_K8S_VERSION = 1.28.2
integration-test: export KUBEBUILDER_ATTACH_CONTROL_PLANE_OUTPUT = $(INTEGRATION_TEST_DEBUG_OUTPUT)
integration-test: $(CROSSPLANE_CRDS) ## Run integration tests with envtest
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest; \
	assets=$$(setup-envtest use -p path 1.25.x!); \
	mkdir -p $(TESTBIN_DIR); \
	cp "$${assets}/"* $(TESTBIN_DIR); \
		go test -tags=integration -v ./... -coverprofile cover.out

.PHONY: build
build: fmt vet $(BIN_FILENAME) ## Build binary

.PHONY: run
run: fmt vet ## Run against the configured Kubernetes cluster in KUBECONFIG
	go run cmd/crossplane-service-broker/main.go

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: lint_yaml
lint_yaml: $(YAML_FILES)
	$(YAMLLINT_DOCKER) -f parsable -c $(YAMLLINT_CONFIG) $(YAMLLINT_ARGS) -- $?

.PHONY: lint
lint: fmt vet lint_yaml ## Invokes the fmt and vet targets
	@echo 'Check for uncommitted changes ...'
	git diff --exit-code

.PHONY: docker-build
docker-build: export GOOS = linux
docker-build: $(BIN_FILENAME) ## Build the docker image
	docker build . -t $(DOCKER_IMG) -t $(QUAY_IMG) -t $(E2E_IMG)

.PHONY: docker-push
docker-push: ## Push the docker image
	docker push $(DOCKER_IMG)
	docker push $(QUAY_IMG)

clean: export KUBECONFIG = $(KIND_KUBECONFIG)
clean: e2e-clean kind-clean ## Cleans up the generated resources
	rm -r testbin/ dist/ bin/ cover.out $(BIN_FILENAME) || true

.PHONY: help
help: ## Show this help
	@grep -E -h '\s##\s' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

###
### Assets
###

$(testbin_created):
	mkdir -p $(TESTBIN_DIR)
	# a marker file must be created, because the date of the
	# directory may update when content in it is created/updated,
	# which would cause a rebuild / re-initialization of dependants
	@touch $(testbin_created)

# Build the binary without running generators
.PHONY: $(BIN_FILENAME)
$(BIN_FILENAME):
	$(go_build)

# TODO(mw): something with this target is off, $@ should be used instead of $*.yaml but I can't seem to make it work.
$(TESTDATA_CRD_DIR)/%.yaml: $(testbin_created)
	curl -sSLo $@ https://raw.githubusercontent.com/crossplane/crossplane/$(CROSSPLANE_VERSION)/cluster/crds/$*.yaml

###
### KIND
###

.PHONY: kind-setup
kind-setup: ## Creates a kind instance if one does not exist yet.
	@$(e2e_make) kind-setup

.PHONY: kind-clean
kind-clean: ## Removes the kind instance if it exists.
	@$(e2e_make) kind-clean

.PHONY: kind-run
kind-run: export KUBECONFIG = $(KIND_KUBECONFIG)
kind-run: kind-setup run ## Runs the service broker on the local host but configured for the kind cluster

kind-e2e-image: docker-build
	$(e2e_make) kind-e2e-image

###
### E2E Test
###

.PHONY: e2e-test
e2e-test: export KUBECONFIG = $(KIND_KUBECONFIG)
e2e-test: e2e-setup docker-build ## Run the e2e tests
	@$(e2e_make) test

.PHONY: e2e-setup
e2e-setup: export KUBECONFIG = $(KIND_KUBECONFIG)
e2e-setup: ## Run the e2e setup
	@$(e2e_make) setup

.PHONY: e2e-clean-setup
e2e-clean-setup: export KUBECONFIG = $(KIND_KUBECONFIG)
e2e-clean-setup: ## Clean the e2e setup (e.g. to rerun the e2e-setup)
	@$(e2e_make) clean-setup

.PHONY: e2e-clean
e2e-clean: ## Remove all e2e-related resources (incl. all e2e Docker images)
	@$(e2e_make) clean

###
### Documentation
###

include ./docs/docs.mk
