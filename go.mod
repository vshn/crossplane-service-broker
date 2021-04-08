module github.com/vshn/crossplane-service-broker

go 1.15

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/crossplane/crossplane v1.1.1
	github.com/crossplane/crossplane-runtime v0.13.0
	github.com/go-logr/logr v0.4.0 // indirect
	github.com/go-logr/zapr v0.4.0
	github.com/go-openapi/spec v0.19.5 // indirect
	github.com/gorilla/mux v1.8.0
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/pivotal-cf/brokerapi/v7 v7.5.0
	github.com/stretchr/testify v1.7.0
	github.com/ulikunitz/xz v0.5.8 // indirect
	go.uber.org/zap v1.16.0
	k8s.io/api v0.20.1
	k8s.io/apimachinery v0.20.1
	k8s.io/client-go v0.20.1
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-runtime v0.8.0
)
