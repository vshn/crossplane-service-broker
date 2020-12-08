[![Build](https://img.shields.io/github/workflow/status/vshn/crossplane-service-broker/Build)][build]
![Go version](https://img.shields.io/github/go-mod/go-version/vshn/crossplane-service-broker)
[![Version](https://img.shields.io/github/v/release/vshn/crossplane-service-broker)][releases]
[![GitHub downloads](https://img.shields.io/github/downloads/vshn/crossplane-service-broker/total)][releases]
[![Docker image](https://img.shields.io/docker/pulls/vshn/crossplane-service-broker)][dockerhub]
[![License](https://img.shields.io/github/license/vshn/crossplane-service-broker)][license]

# Crossplane Service Broker

[Open Service Broker](https://github.com/openservicebrokerapi/servicebroker) API which provisions
Redis and MariaDB instances via [crossplane](https://crossplane.io/).

## Documentation

TBD using antora.

## Contributing

You'll need:

- A running kubernetes cluster (minishift, minikube, k3s, ... you name it) with crossplane installed
- [kubectl](https://kubernetes.io/docs/tasks/tools/install-kubectl/) and [kustomize](https://kubernetes-sigs.github.io/kustomize/installation/)
- Go development environment
- Your favorite IDE (with a Go plugin)
- docker
- make

These are the most common make targets: `build`, `test`, `docker`, `run`.

### Run the service broker

You can run the operator in different ways:

1. using `make run` (provide your own env variables)
1. using `make run_kind` (uses KIND to install a cluster in docker and provides its own kubeconfig in `testbin/`)
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
        "OSB_SERVICE_IDS": "PROVIDE-SERVICE-UUIDS-HERE"
      },
      "args": []
    }
  ]
}
```

## Run E2E tests

The e2e tests currently only test if the deployment works. They do not represent a real
e2e test as of now but are meant as a base to build upon.

You need `node` and `npm` to run the tests, as it runs with [DETIK][detik].

First, setup a local e2e environment
```
make install_bats setup_e2e_test
```

To run e2e tests for newer K8s versions run
```
make e2e_test
```

To remove the local KIND cluster and other resources, run
```
make clean
```

[build]: https://github.com/vshn/crossplane-service-broker/actions?query=workflow%3ABuild
[releases]: https://github.com/vshn/crossplane-service-broker/releases
[license]: https://github.com/vshn/crossplane-service-broker/blob/master/LICENSE
[dockerhub]: https://hub.docker.com/r/vshn/crossplane-service-broker
[detik]: https://github.com/bats-core/bats-detik
