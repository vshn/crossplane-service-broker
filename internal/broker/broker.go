package broker

import (
	"context"
	"encoding/json"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
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
func (b Broker) Provision(ctx context.Context, instanceID, planID string, params json.RawMessage) (domain.ProvisionedServiceSpec, error) {
	res := domain.ProvisionedServiceSpec{}

	p, err := b.cp.Plan(ctx, planID)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}

	_, exists, err := b.cp.Instance(ctx, instanceID, p)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}
	if exists {
		// To avoid having to compare parameters,
		// only instances without any parameters are considered to be equal to another (i.e. existing)
		if params == nil {
			res.AlreadyExists = true
			return res, nil
		}
		return res, toApiResponseError(ctx, apiresponses.ErrInstanceAlreadyExists)
	}

	err = b.cp.CreateInstance(ctx, instanceID, p, params)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}

	res.IsAsync = true
	return res, nil
}

func (b Broker) Deprovision(ctx context.Context, instanceID, planID string) (domain.DeprovisionServiceSpec, error) {
	res := domain.DeprovisionServiceSpec{
		IsAsync: false,
	}

	p, instance, err := b.getPlanInstance(ctx, planID, instanceID)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance, b.logger)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}
	if err := sb.Deprovisionable(ctx); err != nil {
		return res, toApiResponseError(ctx, err)
	}

	if err := b.cp.DeleteInstance(ctx, instance.Composite.GetName(), p); err != nil {
		return res, toApiResponseError(ctx, err)
	}
	return res, nil
}

func (b Broker) Bind(ctx context.Context, instanceID, bindingID, planID string) (domain.Binding, error) {
	res := domain.Binding{
		IsAsync: false,
	}

	_, instance, err := b.getPlanInstance(ctx, planID, instanceID)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}
	if !instance.Ready() {
		return res, toApiResponseError(ctx, apiresponses.ErrConcurrentInstanceAccess)
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance, b.logger)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}

	if fp, ok := sb.(crossplane.FinishProvisioner); ok {
		if err := fp.FinishProvision(ctx); err != nil {
			return res, toApiResponseError(ctx, err)
		}
	}

	creds, err := sb.Bind(ctx, bindingID)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}

	res.Credentials = creds

	return res, nil
}

func (b Broker) Unbind(ctx context.Context, instanceID, bindingID, planID string) (domain.UnbindSpec, error) {
	res := domain.UnbindSpec{
		IsAsync: false,
	}

	_, instance, err := b.getPlanInstance(ctx, planID, instanceID)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}
	if !instance.Ready() {
		return res, toApiResponseError(ctx, apiresponses.ErrConcurrentInstanceAccess)
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance, b.logger)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}

	err = sb.Unbind(ctx, bindingID)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}
	return res, nil
}

func (b Broker) LastOperation(ctx context.Context, instanceID, planID string) (domain.LastOperation, error) {
	res := domain.LastOperation{}

	_, instance, err := b.getPlanInstance(ctx, planID, instanceID)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}

	condition := instance.Composite.GetCondition(xrv1.TypeReady)
	res.Description = "Unknown"
	if desc := string(condition.Reason); len(desc) > 0 {
		res.Description = desc
	}
	res.State = domain.InProgress

	switch condition.Reason {
	case xrv1.ReasonAvailable:
		res.State = domain.Succeeded
		sb, err := crossplane.ServiceBinderFactory(b.cp, instance, b.logger)
		if err != nil {
			return res, toApiResponseError(ctx, err)
		}
		if fp, ok := sb.(crossplane.FinishProvisioner); ok {
			if err := fp.FinishProvision(ctx); err != nil {
				return res, toApiResponseError(ctx, err)
			}
		}
		b.logger.Info("provision-succeeded", lager.Data{"reason": condition.Reason, "message": condition.Message})
	case xrv1.ReasonCreating:
		res.State = domain.InProgress
		b.logger.Info("provision-in-progress", lager.Data{"reason": condition.Reason, "message": condition.Message})
	case xrv1.ReasonUnavailable, xrv1.ReasonDeleting:
		b.logger.Info("provision-failed", lager.Data{"reason": condition.Reason, "message": condition.Message})
		res.State = domain.Failed
	}
	return res, nil
}

func (b Broker) GetBinding(ctx context.Context, instanceID, bindingID string) (domain.GetBindingSpec, error) {
	res := domain.GetBindingSpec{}

	_, instance, err := b.getPlanInstance(ctx, "", instanceID)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}
	if !instance.Ready() {
		return res, toApiResponseError(ctx, apiresponses.ErrConcurrentInstanceAccess)
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance, b.logger)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}

	creds, err := sb.GetBinding(ctx, bindingID)
	if err != nil {
		return res, toApiResponseError(ctx, err)
	}

	res.Credentials = creds

	return res, nil
}

func (b Broker) getPlanInstance(ctx context.Context, planID, instanceID string) (*crossplane.Plan, *crossplane.Instance, error) {
	if planID == "" {
		instance, p, exists, err := b.cp.FindInstanceWithoutPlan(ctx, instanceID)
		if err != nil {
			return nil, nil, err
		}
		if !exists {
			return nil, nil, apiresponses.ErrInstanceDoesNotExist
		}
		return p, instance, nil
	}
	p, err := b.cp.Plan(ctx, planID)
	if err != nil {
		return nil, nil, err
	}

	instance, exists, err := b.cp.Instance(ctx, instanceID, p)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, nil, apiresponses.ErrInstanceDoesNotExist
	}
	return p, instance, nil
}
