package broker

import (
	"context"
	"encoding/json"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"k8s.io/utils/pointer"

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
		return nil, err
	}

	for _, xrd := range xrds {
		plans, err := b.ServicePlans(ctx, []string{xrd.Labels.ServiceID})

		if err != nil {
			b.logger.Error("plan retrieval failed", err, lager.Data{"serviceId": xrd.Labels.ServiceID})
		}

		meta := &domain.ServiceMetadata{}
		if err := json.Unmarshal([]byte(xrd.Metadata), meta); err != nil {
			b.logger.Error("parse-metadata", err)
			meta.DisplayName = xrd.Labels.ServiceName
		}

		var tags []string
		if err := json.Unmarshal([]byte(xrd.Tags), &tags); err != nil {
			b.logger.Error("parse-tags", err)
		}

		services = append(services, domain.Service{
			ID:                   xrd.Labels.ServiceID,
			Name:                 xrd.Labels.ServiceName,
			Description:          xrd.Description,
			Bindable:             xrd.Labels.Bindable,
			InstancesRetrievable: true,
			BindingsRetrievable:  xrd.Labels.Bindable,
			PlanUpdatable:        false,
			Plans:                plans,
			Metadata:             meta,
			Tags:                 tags,
		})
	}

	return services, nil
}

// ServicePlans retrieves a combined view of services and their plans.
func (b Broker) ServicePlans(ctx context.Context, serviceIDs []string) ([]domain.ServicePlan, error) {
	plans := make([]domain.ServicePlan, 0)

	compositions, err := b.cp.Plans(ctx, serviceIDs)
	if err != nil {
		return nil, err
	}

	for _, c := range compositions {
		planName := c.Labels.PlanName
		meta := &domain.ServicePlanMetadata{}
		if err := json.Unmarshal([]byte(c.Metadata), meta); err != nil {
			b.logger.Error("parse-metadata", err)
			meta.DisplayName = planName
		}
		plans = append(plans, domain.ServicePlan{
			ID:          c.Composition.Name,
			Name:        planName,
			Description: c.Description,
			Free:        pointer.BoolPtr(false),
			Bindable:    &c.Labels.Bindable,
			Metadata:    meta,
		})
	}

	return plans, nil
}
