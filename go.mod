module github.com/vshn/crossplane-service-broker

go 1.16

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/crossplane/crossplane v1.11.5
	github.com/crossplane/crossplane-runtime v0.19.2
	github.com/go-logr/zapr v1.2.3
	github.com/gorilla/mux v1.8.1
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/pascaldekloe/jwt v1.12.0
	github.com/pivotal-cf/brokerapi/v8 v8.2.1
	github.com/prometheus/client_golang v1.14.0
	github.com/stretchr/testify v1.8.1
	go.uber.org/zap v1.24.0
	k8s.io/api v0.26.1
	k8s.io/apimachinery v0.26.1
	k8s.io/client-go v0.26.1
	k8s.io/utils v0.0.0-20221128185143-99ec85e7a448
	sigs.k8s.io/controller-runtime v0.14.1
)
