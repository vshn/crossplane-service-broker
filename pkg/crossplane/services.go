package crossplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"code.cloudfoundry.org/lager"
	corev1 "k8s.io/api/core/v1"
)

// instanceSpecParamsPath is the path to an instance's parameters
const instanceSpecParamsPath = "spec.parameters"

// ErrInstanceNotReady is returned if credentials are fetched for an instance which is still provisioning.
var ErrInstanceNotReady = errors.New("instance not ready")

// Credentials contain connection information for accessing a service.
type Credentials map[string]interface{}

// Endpoint describes available service endpoints.
type Endpoint struct {
	Host     string
	Port     int32
	Protocol string
}

type ServiceName string

func (s ServiceName) IsValid() bool {
	switch s {
	case RedisService, MariaDBService, MariaDBDatabaseService:
		return true
	}
	return false
}

var (
	RedisService           ServiceName = "redis-k8s"
	MariaDBService         ServiceName = "mariadb-k8s"
	MariaDBDatabaseService ServiceName = "mariadb-k8s-database"
)

// ServiceBinder is an interface for service specific implementation for binding,
// retrieving credentials, etc.
type ServiceBinder interface {
	Bind(ctx context.Context, bindingID string) (Credentials, error)
	Unbind(ctx context.Context, bindingID string) error
	Deprovisionable(ctx context.Context) error
	GetBinding(ctx context.Context, bindingID string) (Credentials, error)
}

// FinishProvisioner is not currently implemented as provider-helm upgrade is TBD and we need to adjust endpoint retrieval anyway.
// FIXME(mw): determine fate of this interface
type FinishProvisioner interface {
	FinishProvision(ctx context.Context) error
}

// ProvisionValidater enables service implementations to check required additional params.
type ProvisionValidater interface {
	// ValidateProvisionParams can be used to check the params for validity. If valid, it should return all needed parameters
	// for the composition.
	ValidateProvisionParams(ctx context.Context, params json.RawMessage) (map[string]interface{}, error)
}

// ServiceBinderFactory reads the composite's labels service name and instantiates an appropriate ServiceBinder.
// FIXME(mw): determine fate of this. We might not need differentiation anymore, once provider-helm is upgraded.
func ServiceBinderFactory(c *Crossplane, serviceName ServiceName, id string, resourceRefs []corev1.ObjectReference, params map[string]interface{}, logger lager.Logger) (ServiceBinder, error) {
	switch serviceName {
	case RedisService:
		return NewRedisServiceBinder(c, resourceRefs, logger), nil
	case MariaDBService:
		return NewMariadbServiceBinder(c, id, logger), nil
	case MariaDBDatabaseService:
		return NewMariadbDatabaseServiceBinder(c, id, params, logger), nil
	}
	return nil, fmt.Errorf("service binder %q not implemented", serviceName)
}

func findResourceRefs(refs []corev1.ObjectReference, kind string) []corev1.ObjectReference {
	s := make([]corev1.ObjectReference, 0)
	for _, ref := range refs {
		if ref.Kind == kind {
			s = append(s, ref)
		}
	}
	return s
}
