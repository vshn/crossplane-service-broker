MAKEFLAGS += --warn-undefined-variables
SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := all
.DELETE_ON_ERROR:
.SUFFIXES:

TESTDATA_DIR ?= ./testdata
TESTBIN_DIR ?= $(TESTDATA_DIR)/bin
KIND_BIN ?= $(TESTBIN_DIR)/kind
KIND_VERSION ?= 0.9.0
KIND_KUBECONFIG ?= $(TESTBIN_DIR)/kind-kubeconfig
KIND_NODE_VERSION ?= v1.18.8
KIND_CLUSTER ?= crossplane-service-broker
KIND_REGISTRY_NAME ?= kind-registry
KIND_REGISTRY_PORT ?= 5000

# Needs absolute path to setup env variables correctly.
ENVTEST_ASSETS_DIR = $(shell pwd)/testdata

DOCKER_CMD   ?= docker
DOCKER_ARGS  ?= --rm --user "$$(id -u)" --volume "$${PWD}:/src" --workdir /src

# Project parameters
BINARY_NAME ?= crossplane-service-broker

VERSION ?= $(shell git describe --tags --always --dirty --match=v* || (echo "command failed $$?"; exit 1))

IMAGE_NAME ?= docker.io/vshn/$(BINARY_NAME):$(VERSION)
E2E_IMAGE ?= localhost:$(KIND_REGISTRY_PORT)/vshn/$(BINARY_NAME):e2e

ANTORA_PREVIEW_CMD ?= $(DOCKER_CMD) run --rm --publish 35729:35729 --publish 2020:2020 --volume "${PWD}":/preview/antora vshn/antora-preview:2.3.4 --style=syn --antora=docs

# Linting parameters
YAML_FILES      ?= $(shell git ls-files *.y*ml)
YAMLLINT_ARGS   ?= --no-warnings
YAMLLINT_CONFIG ?= .yamllint.yml
YAMLLINT_IMAGE  ?= docker.io/cytopia/yamllint:latest
YAMLLINT_DOCKER ?= $(DOCKER_CMD) run $(DOCKER_ARGS) $(YAMLLINT_IMAGE)

TESTDATA_CRD_DIR = $(TESTDATA_DIR)/crds
CROSSPLANE_VERSION = v1.0.0
CROSSPLANE_CRDS = $(addprefix $(TESTDATA_CRD_DIR)/, apiextensions.crossplane.io_compositeresourcedefinitions.yaml \
					apiextensions.crossplane.io_compositions.yaml \
					pkg.crossplane.io_configurationrevisions.yaml \
					pkg.crossplane.io_configurations.yaml \
					pkg.crossplane.io_controllerconfigs.yaml \
					pkg.crossplane.io_locks.yaml \
					pkg.crossplane.io_providerrevisions.yaml \
					pkg.crossplane.io_providers.yaml)

# Go parameters
GOCMD   ?= go
GOBUILD ?= $(GOCMD) build
GOCLEAN ?= $(GOCMD) clean
GOTEST  ?= $(GOCMD) test
GOGET   ?= $(GOCMD) get

BUILD_CMD ?= CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -v \
				-o $(BINARY_NAME) \
				-ldflags "-X main.Version=$(VERSION) -X 'main.BuildDate=$(shell date)'" \
				cmd/crossplane-service-broker/main.go

.PHONY: all
all: lint test build

.PHONY: build
build:
	$(BUILD_CMD)
	@echo built '$(VERSION)'

$(BINARY_NAME):
	$(BUILD_CMD)

.PHONY: test
test:
	$(GOTEST) -v -cover ./...

.PHONY: run
run:
	go run cmd/crossplane-service-broker/main.go

.PHONY: docker
docker: $(BINARY_NAME)
	DOCKER_BUILDKIT=1 docker build -t $(IMAGE_NAME) -t $(E2E_IMAGE) --build-arg VERSION="$(VERSION)" .
	@echo built image $(IMAGE_NAME)

.PHONY: lint
lint: fmt vet lint_yaml
	@echo 'Check for uncommitted changes ...'
	git diff --exit-code

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
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
integration_test: $(CROSSPLANE_CRDS)
	source ${ENVTEST_ASSETS_DIR}/setup-envtest.sh; fetch_envtest_tools $(ENVTEST_ASSETS_DIR); setup_envtest_env $(ENVTEST_ASSETS_DIR); go test -tags=integration -v ./... -coverprofile cover.out

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

clean: export KUBECONFIG = $(KIND_KUBECONFIG)
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	$(KIND_BIN) delete cluster --name $(KIND_CLUSTER) || true
	docker stop "$(KIND_REGISTRY_NAME)" || true
	docker rm "$(KIND_REGISTRY_NAME)" || true
	docker rmi "$(E2E_IMAGE)" || true
	rm -r testbin/ dist/ bin/ cover.out $(BIN_FILENAME) || true
	$(MAKE) -C e2e clean

.PHONY: install_bats
install_bats:
	$(MAKE) -C e2e install_bats

e2e_test: docker
	docker push $(E2E_IMAGE)
	$(MAKE) -C e2e run_bats -e KUBECONFIG=../$(KIND_KUBECONFIG)
