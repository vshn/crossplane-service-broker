package crossplane

import (
	"context"
	"errors"
	"fmt"

	"code.cloudfoundry.org/lager"
	corev1 "k8s.io/api/core/v1"
)

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

// ServiceBinder is an interface for service specific implementation for binding,
// retrieving credentials, etc.
type ServiceBinder interface {
	Bind(ctx context.Context, bindingID string) (Credentials, error)
}

// FinishProvisioner is not currently implemented as provider-helm upgrade is TBD and we need to adjust endpoint retrieval anyway.
// FIXME(mw): determine fate of this interface
type FinishProvisioner interface {
	FinishProvision(ctx context.Context) error
}

// ServiceBinderFactory reads the composite's labels service name and instantiates an appropriate ServiceBinder.
// FIXME(mw): determine fate of this. We might not need differentiation anymore, once provider-helm is upgraded.
func ServiceBinderFactory(c *Crossplane, instance *Instance, logger lager.Logger) (ServiceBinder, error) {
	serviceName := instance.Labels.ServiceName
	switch serviceName {
	case serviceRedis:
		return NewRedisServiceBinder(c, instance, logger), nil
	case serviceMariadb:
		return NewMariadbServiceBinder(c, instance, logger), nil
	case serviceMariadbDatabase:
		return NewMariadbDatabaseServiceBinder(c, instance, logger), nil
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
