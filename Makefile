MAKEFLAGS += --warn-undefined-variables
SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := all
.DELETE_ON_ERROR:
.SUFFIXES:

DOCKER_CMD   ?= docker
DOCKER_ARGS  ?= --rm --user "$$(id -u)" --volume "$${PWD}:/src" --workdir /src

# Project parameters
BINARY_NAME ?= crossplane-service-broker-poc

VERSION ?= $(shell git describe --tags --always --dirty --match=v* || (echo "command failed $$?"; exit 1))

IMAGE_NAME ?= docker.io/vshn/$(BINARY_NAME):$(VERSION)

ANTORA_PREVIEW_CMD ?= $(DOCKER_CMD) run --rm --publish 35729:35729 --publish 2020:2020 --volume "${PWD}":/preview/antora vshn/antora-preview:2.3.4 --style=syn --antora=docs

# Linting parameters
YAML_FILES      ?= $(shell find . -type f -name '*.yaml' -or -name '*.yml')
YAMLLINT_ARGS   ?= --no-warnings
YAMLLINT_CONFIG ?= .yamllint.yml
YAMLLINT_IMAGE  ?= docker.io/cytopia/yamllint:latest
YAMLLINT_DOCKER ?= $(DOCKER_CMD) run $(DOCKER_ARGS) $(YAMLLINT_IMAGE)

# Go parameters
GOCMD   ?= go
GOBUILD ?= $(GOCMD) build
GOCLEAN ?= $(GOCMD) clean
GOTEST  ?= $(GOCMD) test
GOGET   ?= $(GOCMD) get

.PHONY: all
all: lint test build

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -v \
		-o $(BINARY_NAME) \
		-ldflags "-X main.Version=$(VERSION) -X 'main.BuildDate=$(shell date)'" \
		cmd/broker/main.go
	@echo built '$(VERSION)'

.PHONY: test
test:
	$(GOTEST) -v -cover ./...

.PHONY: run
run:
	go run cmd/broker/main.go

.PHONY: clean
clean:
	$(GOCLEAN)
	rm -f $(BINARY_NAME)

.PHONY: docker
docker:
	DOCKER_BUILDKIT=1 docker build -t $(IMAGE_NAME) --build-arg VERSION="$(VERSION)" .
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
