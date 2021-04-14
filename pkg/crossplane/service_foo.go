package crossplane

import (
	"context"
	"strconv"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pivotal-cf/brokerapi/v8/domain/apiresponses"
)

// FooServiceBinder defines a specific foo service with enough data to retrieve connection credentials.
type FooServiceBinder struct {
	serviceBinder
}

// NewFooServiceBinder instantiates a foo service instance based on the given CompositeFooInstance.
func NewFooServiceBinder(c *Crossplane, instance *Instance, logger lager.Logger) *FooServiceBinder {
	return &FooServiceBinder{
		serviceBinder: serviceBinder{
			instance: instance,
			cp:       c,
			logger:   logger,
		},
	}
}

// Bind retrieves the necessary external IP, password and ports.
func (fsb FooServiceBinder) Bind(ctx context.Context, bindingID string) (Credentials, error) {
	return fsb.GetBinding(ctx, bindingID)
}

// Unbind does nothing for foo bindings.
func (fsb FooServiceBinder) Unbind(_ context.Context, _ string) error {
	return nil
}

// Deprovisionable returns always nil for foo instances.
func (fsb FooServiceBinder) Deprovisionable(_ context.Context) error {
	return nil
}

// GetBinding returns the credentials for a foo service
func (fsb FooServiceBinder) GetBinding(ctx context.Context, bindingID string) (Credentials, error) {
	s, err := fsb.cp.GetConnectionDetails(ctx, fsb.instance.Composite)
	if err != nil {
		return nil, err
	}

	endpointBytes, ok := s.Data[xrv1.ResourceCredentialsSecretEndpointKey]
	if !ok {
		return nil, apiresponses.ErrBindingNotFound
	}
	port, err := strconv.Atoi(string(s.Data[xrv1.ResourceCredentialsSecretPortKey]))
	if err != nil {
		return nil, err
	}
	creds := Credentials{
		"endpoint": string(endpointBytes),
		"port":     port,
		"username": bindingID,
		"password": string(s.Data[xrv1.ResourceCredentialsSecretPasswordKey]),
	}

	return creds, nil
}
