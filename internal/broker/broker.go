package broker

import (
	"context"
	"encoding/json"
	"errors"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	corev1 "k8s.io/api/core/v1"

	"github.com/vshn/crossplane-service-broker/internal/crossplane"
)

var (
	// ErrInstanceNotFound is an instance doesn't exist
	ErrInstanceNotFound = errors.New("instance not found")
	// ErrServiceUpdateNotPermitted when updating an instance
	ErrServiceUpdateNotPermitted = errors.New("service update not permitted")
	// ErrPlanChangeNotPermitted when updating an instance's plan (only premium<->standard is permitted)
	ErrPlanChangeNotPermitted = errors.New("plan change not permitted")
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
		return nil, err
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
		return res, err
	}

	_, exists, err := b.cp.Instance(ctx, instanceID, p)
	if err != nil {
		return res, err
	}
	if exists {
		// To avoid having to compare parameters,
		// only instances without any parameters are considered to be equal to another (i.e. existing)
		if params == nil {
			res.AlreadyExists = true
			return res, nil
		}
		return res, apiresponses.ErrInstanceAlreadyExists
	}

	err = b.cp.CreateInstance(ctx, instanceID, p, params)
	if err != nil {
		return res, err
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
		return res, err
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance, b.logger)
	if err != nil {
		return res, err
	}
	if err := sb.Deprovisionable(ctx); err != nil {
		return res, err
	}

	if err := b.cp.DeleteInstance(ctx, instance.Composite.GetName(), p); err != nil {
		return res, err
	}
	return res, nil
}

func (b Broker) Bind(ctx context.Context, instanceID, bindingID, planID string) (domain.Binding, error) {
	res := domain.Binding{
		IsAsync: false,
	}

	_, instance, err := b.getPlanInstance(ctx, planID, instanceID)
	if err != nil {
		return res, err
	}
	if !instance.Ready() {
		return res, apiresponses.ErrConcurrentInstanceAccess
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance, b.logger)
	if err != nil {
		return res, err
	}

	if fp, ok := sb.(crossplane.FinishProvisioner); ok {
		if err := fp.FinishProvision(ctx); err != nil {
			return res, err
		}
	}

	creds, err := sb.Bind(ctx, bindingID)
	if err != nil {
		return res, err
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
		return res, err
	}
	if !instance.Ready() {
		return res, apiresponses.ErrConcurrentInstanceAccess
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance, b.logger)
	if err != nil {
		return res, err
	}

	err = sb.Unbind(ctx, bindingID)
	if err != nil {
		return res, err
	}
	return res, nil
}

func (b Broker) LastOperation(ctx context.Context, instanceID, planID string) (domain.LastOperation, error) {
	res := domain.LastOperation{}

	_, instance, err := b.getPlanInstance(ctx, planID, instanceID)
	if err != nil {
		return res, err
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
			return res, err
		}
		if fp, ok := sb.(crossplane.FinishProvisioner); ok {
			if err := fp.FinishProvision(ctx); err != nil {
				return res, err
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
		return res, err
	}
	if !instance.Ready() {
		return res, apiresponses.ErrConcurrentInstanceAccess
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance, b.logger)
	if err != nil {
		return res, err
	}

	creds, err := sb.GetBinding(ctx, bindingID)
	if err != nil {
		return res, err
	}

	res.Credentials = creds

	return res, nil
}

func (b Broker) GetInstance(ctx context.Context, instanceID string) (domain.GetInstanceDetailsSpec, error) {
	res := domain.GetInstanceDetailsSpec{}

	p, instance, err := b.getPlanInstance(ctx, "", instanceID)
	if err != nil {
		return res, err
	}

	params, err := instance.Parameters()
	if err != nil {
		return res, err
	}

	res.PlanID = p.Composition.GetName()
	res.ServiceID = p.Labels.ServiceID
	res.Parameters = params

	return res, nil
}

func (b Broker) Update(ctx context.Context, instanceID, serviceID, oldPlanID, newPlanID string) (domain.UpdateServiceSpec, error) {
	res := domain.UpdateServiceSpec{}

	_, instance, err := b.getPlanInstance(ctx, oldPlanID, instanceID)
	if err != nil {
		return res, err
	}
	if instance.Labels.ServiceID != serviceID {
		return res, ErrServiceUpdateNotPermitted
	}

	np, err := b.cp.Plan(ctx, newPlanID)
	if err != nil {
		return res, err
	}

	slaChangePermitted := func() bool {
		instanceSLA := instance.Labels.SLA
		newPlanSLA := np.Labels.SLA
		instancePlanSize := instance.Labels.PlanSize
		newPlanSize := np.Labels.PlanSize
		instanceService := instance.Labels.ServiceID
		newPlanService := np.Labels.ServiceID

		// switch from redis to mariadb not permitted
		if instanceService != newPlanService {
			return false
		}
		// xsmall -> large not permitted, only xsmall <-> xsmall-premium
		if instancePlanSize != newPlanSize {
			return false
		}
		if instanceSLA == crossplane.SLAPremium && newPlanSLA == crossplane.SLAStandard {
			return true
		}
		if instanceSLA == crossplane.SLAStandard && newPlanSLA == crossplane.SLAPremium {
			return true
		}
		return false
	}

	if !slaChangePermitted() {
		return res, ErrPlanChangeNotPermitted
	}

	instance.Composite.SetCompositionReference(&corev1.ObjectReference{
		Name: np.Composition.GetName(),
	})
	instanceLabels := instance.Composite.GetLabels()
	for _, l := range []string{
		crossplane.PlanNameLabel,
		crossplane.SLALabel,
	} {
		instanceLabels[l] = np.Composition.Labels[l]
	}
	instance.Composite.SetLabels(instanceLabels)

	if err := b.cp.UpdateInstance(ctx, instance, np); err != nil {
		return res, err
	}

	return res, nil
}

func (b Broker) getPlanInstance(ctx context.Context, planID, instanceID string) (*crossplane.Plan, *crossplane.Instance, error) {
	if planID == "" {
		b.logger.Info("find-instance-without-plan", lager.Data{"instance-id": instanceID})

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
