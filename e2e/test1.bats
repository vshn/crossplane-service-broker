#!/usr/bin/env bats

load "lib/utils"
load "lib/detik"
load "lib/crossplane-service-broker"

DETIK_CLIENT_NAME="kubectl"
DETIK_CLIENT_NAMESPACE="crossplane-service-broker"
DEBUG_DETIK="true"

@test "reset the debug file" {
	reset_debug
}

@test "verify the deployment" {
  go run sigs.k8s.io/kustomize/kustomize/v3 build test1 > debug/test1.yaml
  run kubectl apply -f debug/test1.yaml
  echo "$output"

  try "at most 20 times every 2s to find 1 pod named 'crossplane-service-broker' with 'status' being 'running'"

}