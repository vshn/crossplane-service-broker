module github.com/vshn/crossplane-service-broker

go 1.16

require (
	cloud.google.com/go/iam v0.7.0 // indirect
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/crossplane/crossplane v1.10.1
	github.com/crossplane/crossplane-runtime v0.19.0-rc.0.0.20221012013934-bce61005a175
	github.com/go-logr/zapr v1.2.0
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/pascaldekloe/jwt v1.10.0
	github.com/pivotal-cf/brokerapi/v8 v8.2.1
	github.com/prometheus/client_golang v1.12.2
	github.com/stretchr/testify v1.7.2
	github.com/ulikunitz/xz v0.5.8 // indirect
	go.uber.org/zap v1.21.0
	k8s.io/api v0.24.0
	k8s.io/apimachinery v0.24.0
	k8s.io/client-go v0.24.0
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	sigs.k8s.io/controller-runtime v0.11.0
)
