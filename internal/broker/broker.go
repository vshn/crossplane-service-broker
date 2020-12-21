package broker

import (
	"context"
	"encoding/json"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"

	"github.com/vshn/crossplane-service-broker/internal/crossplane"
)

type Broker struct {
	cp     *crossplane.Crossplane
	logger lager.Logger
}

func New(cp *crossplane.Crossplane, logger lager.Logger) *Broker {
	return &Broker{
		cp:     cp,
		logger: logger,
	}
}

// Services retrieves registered services and plans.
func (b Broker) Services(ctx context.Context) ([]domain.Service, error) {
	services := make([]domain.Service, 0)

	xrds, err := b.cp.ServiceXRDs(ctx)
	if err != nil {
		return nil, toApiResponseError(ctx, err)
	}

	for _, xrd := range xrds {
		plans, err := b.servicePlans(ctx, []string{xrd.Labels.ServiceID})
		if err != nil {
			b.logger.Error("plan retrieval failed", err, lager.Data{"serviceId": xrd.Labels.ServiceID})
		}

		services = append(services, newService(xrd, plans, b.logger))
	}

	return services, nil
}

// servicePlans retrieves a combined view of services and their plans.
func (b Broker) servicePlans(ctx context.Context, serviceIDs []string) ([]domain.ServicePlan, error) {
	plans := make([]domain.ServicePlan, 0)

	compositions, err := b.cp.Plans(ctx, serviceIDs)
	if err != nil {
		return nil, toApiResponseError(ctx, err)
	}

	for _, c := range compositions {
		plans = append(plans, newServicePlan(c, b.logger))
	}

	return plans, nil
}

// Provision creates a new service instance.
// TODO(mw): serviceID is not required, sounds wrong. Should we check if service exists here? and plan belongs to service?
func (b Broker) Provision(ctx context.Context, instanceID, planID string, params json.RawMessage) (domain.ProvisionedServiceSpec, error) {
	spec := domain.ProvisionedServiceSpec{}

	p, err := b.cp.Plan(ctx, planID)
	if err != nil {
		return spec, toApiResponseError(ctx, err)
	}

	_, exists, err := b.cp.Instance(ctx, instanceID, p)
	if err != nil {
		return spec, toApiResponseError(ctx, err)
	}
	if exists {
		// To avoid having to compare parameters,
		// only instances without any parameters are considered to be equal to another (i.e. existing)
		if params == nil {
			spec.AlreadyExists = true
			return spec, nil
		}
		return spec, apiresponses.ErrInstanceAlreadyExists
	}

	err = b.cp.CreateInstance(ctx, instanceID, p, params)
	if err != nil {
		return spec, toApiResponseError(ctx, err)
	}

	spec.IsAsync = true
	return spec, nil
}
