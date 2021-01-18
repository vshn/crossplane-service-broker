module github.com/vshn/crossplane-service-broker

go 1.15

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/crossplane-contrib/provider-helm v0.5.0
	github.com/crossplane/crossplane v1.0.0
	github.com/crossplane/crossplane-runtime v0.12.0
	github.com/go-logr/logr v0.3.0 // indirect
	github.com/go-logr/zapr v0.3.0
	github.com/gorilla/mux v1.8.0
	github.com/pivotal-cf/brokerapi/v7 v7.5.0
	github.com/stretchr/testify v1.6.1
	go.uber.org/zap v1.16.0
	k8s.io/api v0.19.3
	k8s.io/apimachinery v0.20.0
	k8s.io/client-go v0.19.3
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-runtime v0.6.4
)
