[![Build](https://img.shields.io/github/workflow/status/vshn/crossplane-service-broker/Pull%20Request)][build]
![Go version](https://img.shields.io/github/go-mod/go-version/vshn/crossplane-service-broker)
[![Version](https://img.shields.io/github/v/release/vshn/crossplane-service-broker)][releases]
[![GitHub downloads](https://img.shields.io/github/downloads/vshn/crossplane-service-broker/total)][releases]
[![Docker image](https://img.shields.io/docker/pulls/vshn/crossplane-service-broker)][dockerhub]
[![License](https://img.shields.io/github/license/vshn/crossplane-service-broker)][license]

# Crossplane Service Broker

[Open Service Broker](https://github.com/openservicebrokerapi/servicebroker) API which provisions
Redis and MariaDB instances via [crossplane](https://crossplane.io/).

## Documentation

Most of the explanation on how this all works together currently lives in the [VSHN Knowledgebase](https://kb.vshn.ch/app-catalog/explanations/crossplane_service_broker.html).

## Contributing

You'll need:

- A running kubernetes cluster (minishift, minikube, k3s, ... you name it) with crossplane installed
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) and [kustomize](https://kubernetes-sigs.github.io/kustomize/installation/)
- Go development environment
- Your favorite IDE (with a Go plugin)
- docker
- make

These are the most common make targets: `build`, `test`, `docker-build`, `run`.

### Folder structure overview

```
.
├── cmd
│   └── crossplane-service-broker    # main file
├── deploy
│   └── base                         # deployment files
├── docs                             # antora docs
├── e2e                              # e2e testing files
├── pkg
│   ├── api                          # exposed HTTP API server
│   ├── brokerapi                    # service broker implementation
│   ├── config                       # app config
│   ├── crossplane                   # crossplane service layer
│   ├── integration                  # utilities for integration tests
│   └── reqcontext                   # request context mapping
└── testdata                         # integration testing files
```

### Run the service broker

You can run the operator in different ways:

1. using `make run` (provide your own env variables)
1. using `make kind-run` (uses KIND to install a cluster in docker and provides its own kubeconfig in `testbin/`)
1. using a configuration of your favorite IDE (see below for VSCode example)

Example VSCode run configuration:

```
{
  // Use IntelliSense to learn about possible attributes.
  // Hover to view descriptions of existing attributes.
  // For more information, visit: https://go.microsoft.com/fwlink/?linkid=830387
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Launch",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/cmd/crossplane-service-broker/main.go",
      "env": {
        "KUBECONFIG": "path/to/kubeconfig",
        "OSB_USERNAME": "test",
        "OSB_PASSWORD": "TEST",
        "OSB_SERVICE_IDS": "PROVIDE-SERVICE-UUIDS-HERE",
        "OSB_NAMESPACE": "test",
        "OSB_PLAN_UPDATE_SIZE_RULES": "xsmall>small|xsmall>medium|small>medium"
      },
      "args": []
    }
  ]
}
```

## Run integration tests

"Integration" testing is done using [envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest) and [crossplane's integration test helper](https://github.com/crossplane/crossplane-runtime/tree/master/pkg/test/integration).

```
make integration-test
```

Example VSCode run configuration (to be able to debug things):

```
{
  // Use IntelliSense to learn about possible attributes.
  // Hover to view descriptions of existing attributes.
  // For more information, visit: https://go.microsoft.com/fwlink/?linkid=830387
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Integration tests",
      "type": "go",
      "request": "launch",
      "mode": "test",
      "program": "${workspaceFolder}/pkg/brokerapi",
      "env": {
        "KUBEBUILDER_ASSETS": "../../testdata/bin",
        "DEBUG": "true" // optional
      },
      "args": [],
      "buildFlags": "-tags=integration"
    }
  ]
}
```

## Run E2E tests

The e2e tests currently only test if the deployment works. They do not represent a real
e2e test as of now but are meant as a base to build upon.

You need `node` and `npm` to run the tests, as it runs with [DETIK][detik].

To run e2e tests for newer K8s versions run

```
make e2e-test
```

To remove the local KIND cluster and other resources, run

```
make clean
```

## Integration tests

The integration tests aren't very clever yet and will need some improvements to catch issues.
They set the boundary at Kubernetes and avoid therefore the need to have a working crossplane installation with
all the custom resources necessary set up.

This means that certain kinds of bugs can't be catched (interaction between crossplane provisioner and this code).

All integration tests are currently within the `brokerapi` package. [`envtest`](https://book.kubebuilder.io/reference/envtest.html) is used
to setup a Kubernetes API server, and an etcd instance. Public CRDs from crossplane are being downloaded on first run, while
the CRDs for this service broker are manually copied in and live within `testdata/crds/*.syn.tools.yaml`.

If tests for CRDs within the `syn.tools` API group are written, ensure the corresponding CRD definition is within the aforementioned directory.

[build]: https://github.com/vshn/crossplane-service-broker/actions?query=workflow%3APull%20Request
[releases]: https://github.com/vshn/crossplane-service-broker/releases
[license]: https://github.com/vshn/crossplane-service-broker/blob/master/LICENSE
[dockerhub]: https://hub.docker.com/r/vshn/crossplane-service-broker
[detik]: https://github.com/bats-core/bats-detik
