module github.com/vshn/crossplane-service-broker

go 1.15

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/crossplane/crossplane v1.1.1
	github.com/crossplane/crossplane-runtime v0.13.0
	github.com/go-logr/logr v0.4.0 // indirect
	github.com/go-logr/zapr v0.4.0
	github.com/gorilla/mux v1.8.0
	github.com/pascaldekloe/jwt v1.10.0
	github.com/pivotal-cf/brokerapi/v8 v8.0.0
	github.com/prometheus/client_golang v1.7.1
	github.com/stretchr/testify v1.7.0
	go.uber.org/zap v1.16.0
	k8s.io/api v0.20.1
	k8s.io/apimachinery v0.20.1
	k8s.io/client-go v0.20.1
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-runtime v0.8.0
)
