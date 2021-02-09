IMG_TAG ?= latest
VERSION ?= $(shell git describe --tags --always --dirty --match=v* || (echo "command failed $$?"; exit 1))

TESTDATA_DIR ?= $(PROJECT_ROOT_DIR)/testdata
TESTBIN_DIR ?= $(TESTDATA_DIR)/bin
TESTDATA_CRD_DIR = $(TESTDATA_DIR)/crds

KIND_BIN ?= $(TESTBIN_DIR)/kind
KIND_VERSION ?= 0.9.0
KIND_KUBECONFIG ?= $(TESTBIN_DIR)/kind-kubeconfig
KIND_NODE_VERSION ?= v1.20.0
KIND_CLUSTER ?= crossplane-service-broker
KIND_REGISTRY_NAME ?= kind-registry
KIND_REGISTRY_PORT ?= 5000

# Needs absolute path to setup env variables correctly.
ENVTEST_ASSETS_DIR = $(shell pwd)/testdata

DOCKER_CMD   ?= docker
DOCKER_ARGS  ?= --rm --user "$$(id -u)" --volume "$${PWD}:/src" --workdir /src

# Project parameters
BINARY_NAME ?= crossplane-service-broker
PROJECT_NAME ?= crossplane-service-broker

# Image URL to use all building/pushing image targets
DOCKER_IMG ?= docker.io/vshn/$(PROJECT_NAME):$(IMG_TAG)
QUAY_IMG ?= quay.io/vshn/$(PROJECT_NAME):$(IMG_TAG)

SHASUM ?= $(shell command -v sha1sum > /dev/null && echo "sha1sum" || echo "shasum -a1")
E2E_TAG ?= e2e_$(shell $(SHASUM) $(BINARY_NAME) | cut -b-8)
E2E_REPO ?= local.dev/$(PROJECT_NAME)/e2e
E2E_IMG = $(E2E_REPO):$(E2E_TAG)

ANTORA_PREVIEW_CMD ?= $(DOCKER_CMD) run --rm --publish 35729:35729 --publish 2020:2020 --volume "${PWD}":/preview/antora vshn/antora-preview:2.3.4 --style=syn --antora=docs

# Linting parameters
YAML_FILES      ?= $(shell git ls-files *.y*ml)
YAMLLINT_ARGS   ?= --no-warnings
YAMLLINT_CONFIG ?= .yamllint.yml
YAMLLINT_IMAGE  ?= docker.io/cytopia/yamllint:latest
YAMLLINT_DOCKER ?= $(DOCKER_CMD) run $(DOCKER_ARGS) $(YAMLLINT_IMAGE)

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
