package crossplane

import (
	"context"

	"code.cloudfoundry.org/lager"
)

// GenericServiceBinder defines a specific Mariadb service with enough data to retrieve connection credentials.
type GenericServiceBinder struct {
	serviceBinder
}

// NewGenericServiceBinder instantiates a Mariadb service instance based on the given CompositeMariadbInstance.
func NewGenericServiceBinder(c *Crossplane, instance *Instance, logger lager.Logger) *GenericServiceBinder {
	return &GenericServiceBinder{
		serviceBinder: serviceBinder{
			instance: instance,
			cp:       c,
			logger:   logger,
		},
	}
}

func (g GenericServiceBinder) Bind(ctx context.Context, bindingID string) (Credentials, error) {
	//TODO implement me
	panic("implement me")
}

func (g GenericServiceBinder) Unbind(ctx context.Context, bindingID string) error {
	//TODO implement me
	panic("implement me")
}

func (g GenericServiceBinder) Deprovisionable(ctx context.Context) error {
	//TODO this should check to make sure instances are free
	return nil
}

func (g GenericServiceBinder) GetBinding(ctx context.Context, bindingID string) (Credentials, error) {
	//TODO implement me
	panic("implement me")
}

func (g GenericServiceBinder) ValidateProvisionParams(ctx context.Context, params map[string]interface{}) (map[string]interface{}, error) {
	// @fixme: How to map site-specific labels?
	params[ClusterLabel] = params["subdomain"]
	return params, nil
}
