module github.com/vshn/crossplane-service-broker

go 1.16

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/crossplane/crossplane v1.6.1
	github.com/crossplane/crossplane-runtime v0.15.1-0.20211029211307-c72bcdd922eb
	github.com/go-logr/zapr v0.4.0
	github.com/gorilla/mux v1.8.1
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/pascaldekloe/jwt v1.12.0
	github.com/pivotal-cf/brokerapi/v8 v8.2.3
	github.com/prometheus/client_golang v1.12.2
	github.com/stretchr/testify v1.7.2
	github.com/ulikunitz/xz v0.5.8 // indirect
	go.uber.org/zap v1.21.0
	k8s.io/api v0.21.3
	k8s.io/apimachinery v0.21.3
	k8s.io/client-go v0.21.3
	k8s.io/utils v0.0.0-20210722164352-7f3ee0f31471
	sigs.k8s.io/controller-runtime v0.9.6
)
