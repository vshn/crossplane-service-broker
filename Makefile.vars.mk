ARCHS ?= linux/amd64
IMG_TAG ?= latest
IMG_REGISTRIES ?= docker.io quay.io
IMG_NAME ?= crossplane-service-broker
IMG_PATHS ?= $(foreach reg, $(IMG_REGISTRIES), $(reg)/vshn/$(IMG_NAME))
BUILD_TAGS := $(foreach path, $(IMG_PATHS), $(path):$(IMG_TAG))
VERSION ?= $(shell git describe --tags --always --dirty --match=v* || (echo "command failed $$?"; exit 1))

BIN_FILENAME ?= $(PROJECT_ROOT_DIR)/crossplane-service-broker

TESTDATA_DIR ?= $(PROJECT_ROOT_DIR)/testdata
TESTBIN_DIR ?= $(TESTDATA_DIR)/bin

DOCKER_CMD   ?= docker
DOCKER_ARGS  ?= --rm --user "$$(id -u)" --volume "$${PWD}:/src" --workdir /src

KIND_VERSION ?= 0.11.1
KIND_NODE_VERSION ?= v1.28.0
KIND ?= $(TESTBIN_DIR)/kind

ENABLE_LEADER_ELECTION ?= false

KIND_KUBECONFIG ?= $(TESTBIN_DIR)/kind-kubeconfig-$(KIND_NODE_VERSION)
KIND_CLUSTER ?= crossplane-service-broker-$(KIND_NODE_VERSION)
KIND_KUBECTL_ARGS ?= --validate=true

SHASUM ?= $(shell command -v sha1sum > /dev/null && echo "sha1sum" || echo "shasum -a1")
E2E_TAG ?= e2e_$(shell $(SHASUM) $(BIN_FILENAME) | cut -b-8)
E2E_REPO ?= local.dev/$(IMG_NAME)/e2e
E2E_IMG = $(E2E_REPO):$(E2E_TAG)

TESTDATA_CRD_DIR = $(TESTDATA_DIR)/crds
CROSSPLANE_VERSION = v1.15.3
CROSSPLANE_CRDS = $(addprefix $(TESTDATA_CRD_DIR)/, apiextensions.crossplane.io_compositeresourcedefinitions.yaml \
					apiextensions.crossplane.io_compositions.yaml \
					pkg.crossplane.io_configurationrevisions.yaml \
					pkg.crossplane.io_configurations.yaml \
					pkg.crossplane.io_controllerconfigs.yaml \
					pkg.crossplane.io_locks.yaml \
					pkg.crossplane.io_providerrevisions.yaml \
					pkg.crossplane.io_providers.yaml)

PROVIDERSQL_VERSION = v0.9.0

testbin_created = $(TESTBIN_DIR)/.created

# Linting parameters
YAML_FILES      ?= $(shell git ls-files *.y*ml)
YAMLLINT_ARGS   ?= --no-warnings
YAMLLINT_CONFIG ?= .yamllint.yml
YAMLLINT_IMAGE  ?= docker.io/cytopia/yamllint:latest
YAMLLINT_DOCKER ?= $(DOCKER_CMD) run $(DOCKER_ARGS) $(YAMLLINT_IMAGE)
