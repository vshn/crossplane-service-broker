package brokerapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	corev1 "k8s.io/api/core/v1"

	"github.com/vshn/crossplane-service-broker/pkg/crossplane"
	"github.com/vshn/crossplane-service-broker/pkg/reqcontext"
)

var (
	// ErrServiceUpdateNotPermitted when updating an instance
	ErrServiceUpdateNotPermitted = errors.New("service update not permitted")
	// ErrPlanChangeNotPermitted when updating an instance's plan (only premium<->standard is permitted)
	ErrPlanChangeNotPermitted = errors.New("plan change not permitted")
)

// Broker implements the service broker
type Broker struct {
	cp *crossplane.Crossplane
}

// NewBroker sets up a new broker.
func NewBroker(cp *crossplane.Crossplane) *Broker {
	return &Broker{
		cp: cp,
	}
}

// Services retrieves registered services and plans.
func (b Broker) Services(rctx *reqcontext.ReqContext) ([]domain.Service, error) {
	services := make([]domain.Service, 0)

	xrds, err := b.cp.ServiceXRDs(rctx)
	if err != nil {
		return nil, err
	}

	for _, xrd := range xrds {
		plans, err := b.servicePlans(rctx, []string{xrd.Labels.ServiceID})
		if err != nil {
			rctx.Logger.Error("plan retrieval failed", err, lager.Data{"serviceId": xrd.Labels.ServiceID})
		}

		services = append(services, newService(xrd, plans, rctx.Logger))
	}

	return services, nil
}

// servicePlans retrieves a combined view of services and their plans.
func (b Broker) servicePlans(rctx *reqcontext.ReqContext, serviceIDs []string) ([]domain.ServicePlan, error) {
	plans := make([]domain.ServicePlan, 0)

	compositions, err := b.cp.Plans(rctx, serviceIDs)
	if err != nil {
		return nil, err
	}

	for _, c := range compositions {
		plans = append(plans, newServicePlan(c, rctx.Logger))
	}

	return plans, nil
}

// Provision creates a new service instance.
func (b Broker) Provision(rctx *reqcontext.ReqContext, instanceID, planID string, params json.RawMessage) (domain.ProvisionedServiceSpec, error) {
	res := domain.ProvisionedServiceSpec{}

	p, err := b.cp.Plan(rctx, planID)
	if err != nil {
		return res, err
	}

	_, exists, err := b.cp.Instance(rctx, instanceID, p)
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

	ap := map[string]interface{}{}
	if params != nil {
		// ServiceBinderFactory is used out of convenience, however it seems the wrong approach here - might refactor later.
		sb, err := crossplane.ServiceBinderFactory(b.cp, p.Labels.ServiceName, "", nil, nil, rctx.Logger)
		if err != nil {
			return res, err
		}
		if pv, ok := sb.(crossplane.ProvisionValidater); ok {
			ap, err = pv.ValidateProvisionParams(rctx.Context, params)
			if err != nil {
				return res, apiresponses.NewFailureResponse(err, http.StatusBadRequest, "validate-provision-failed")
			}
		}
	}

	err = b.cp.CreateInstance(rctx, instanceID, p, ap)
	if err != nil {
		return res, err
	}

	res.IsAsync = true
	return res, nil
}

// Deprovision removes a provisioned instance.
func (b Broker) Deprovision(rctx *reqcontext.ReqContext, instanceID, planID string) (domain.DeprovisionServiceSpec, error) {
	res := domain.DeprovisionServiceSpec{
		IsAsync: false,
	}

	p, instance, err := b.getPlanInstance(rctx, planID, instanceID)
	if err != nil {
		return res, err
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance.Labels.ServiceName, instance.ID(), instance.ResourceRefs(), instance.Parameters(), rctx.Logger)
	if err != nil {
		return res, err
	}
	if err := sb.Deprovisionable(rctx.Context); err != nil {
		return res, err
	}

	if err := b.cp.DeleteInstance(rctx, instance.Composite.GetName(), p); err != nil {
		return res, err
	}
	return res, nil
}

// Bind creates a binding between a provisioned service instance and an application.
func (b Broker) Bind(rctx *reqcontext.ReqContext, instanceID, bindingID, planID string) (domain.Binding, error) {
	res := domain.Binding{
		IsAsync: false,
	}

	_, instance, err := b.getPlanInstance(rctx, planID, instanceID)
	if err != nil {
		return res, err
	}
	if !instance.Ready() {
		return res, apiresponses.ErrConcurrentInstanceAccess
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance.Labels.ServiceName, instance.ID(), instance.ResourceRefs(), instance.Parameters(), rctx.Logger)
	if err != nil {
		return res, err
	}

	if fp, ok := sb.(crossplane.FinishProvisioner); ok {
		if err := fp.FinishProvision(rctx.Context); err != nil {
			return res, err
		}
	}

	creds, err := sb.Bind(rctx.Context, bindingID)
	if err != nil {
		return res, err
	}

	res.Credentials = creds

	return res, nil
}

// Unbind removes a binding.
func (b Broker) Unbind(rctx *reqcontext.ReqContext, instanceID, bindingID, planID string) (domain.UnbindSpec, error) {
	res := domain.UnbindSpec{
		IsAsync: false,
	}

	_, instance, err := b.getPlanInstance(rctx, planID, instanceID)
	if err != nil {
		return res, err
	}
	if !instance.Ready() {
		return res, apiresponses.ErrConcurrentInstanceAccess
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance.Labels.ServiceName, instance.ID(), instance.ResourceRefs(), instance.Parameters(), rctx.Logger)
	if err != nil {
		return res, err
	}

	err = sb.Unbind(rctx.Context, bindingID)
	if err != nil {
		return res, err
	}
	return res, nil
}

// LastOperation retrieves an instance's status.
func (b Broker) LastOperation(rctx *reqcontext.ReqContext, instanceID, planID string) (domain.LastOperation, error) {
	res := domain.LastOperation{}

	_, instance, err := b.getPlanInstance(rctx, planID, instanceID)
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
		sb, err := crossplane.ServiceBinderFactory(b.cp, instance.Labels.ServiceName, instance.ID(), instance.ResourceRefs(), instance.Parameters(), rctx.Logger)
		if err != nil {
			return res, err
		}
		if fp, ok := sb.(crossplane.FinishProvisioner); ok {
			if err := fp.FinishProvision(rctx.Context); err != nil {
				return res, err
			}
		}
		rctx.Logger.Info("provision-succeeded", lager.Data{"reason": condition.Reason, "message": condition.Message})
	case xrv1.ReasonCreating:
		res.State = domain.InProgress
		rctx.Logger.Info("provision-in-progress", lager.Data{"reason": condition.Reason, "message": condition.Message})
	case xrv1.ReasonUnavailable, xrv1.ReasonDeleting:
		rctx.Logger.Info("provision-failed", lager.Data{"reason": condition.Reason, "message": condition.Message})
		res.State = domain.Failed
	}
	return res, nil
}

// GetBinding retrieves a binding to get credentials.
func (b Broker) GetBinding(rctx *reqcontext.ReqContext, instanceID, bindingID string) (domain.GetBindingSpec, error) {
	res := domain.GetBindingSpec{}

	_, instance, err := b.getPlanInstance(rctx, "", instanceID)
	if err != nil {
		return res, err
	}
	if !instance.Ready() {
		return res, apiresponses.ErrConcurrentInstanceAccess
	}

	sb, err := crossplane.ServiceBinderFactory(b.cp, instance.Labels.ServiceName, instance.ID(), instance.ResourceRefs(), instance.Parameters(), rctx.Logger)
	if err != nil {
		return res, err
	}

	creds, err := sb.GetBinding(rctx.Context, bindingID)
	if err != nil {
		return res, err
	}

	res.Credentials = creds

	return res, nil
}

// GetInstance gets a provisioned instance.
func (b Broker) GetInstance(rctx *reqcontext.ReqContext, instanceID string) (domain.GetInstanceDetailsSpec, error) {
	res := domain.GetInstanceDetailsSpec{}

	p, instance, err := b.getPlanInstance(rctx, "", instanceID)
	if err != nil {
		return res, err
	}

	res.PlanID = p.Composition.GetName()
	res.ServiceID = p.Labels.ServiceID

	params := instance.Parameters()
	if len(params) > 0 {
		res.Parameters = params
	}

	return res, nil
}

// Update allows to change the SLA level from standard -> premium (and vice-versa).
func (b Broker) Update(rctx *reqcontext.ReqContext, instanceID, serviceID, oldPlanID, newPlanID string) (domain.UpdateServiceSpec, error) {
	res := domain.UpdateServiceSpec{}

	_, instance, err := b.getPlanInstance(rctx, oldPlanID, instanceID)
	if err != nil {
		return res, err
	}
	if instance.Labels.ServiceID != serviceID {
		return res, ErrServiceUpdateNotPermitted
	}

	np, err := b.cp.Plan(rctx, newPlanID)
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

	if err := b.cp.UpdateInstance(rctx, instance, np); err != nil {
		return res, err
	}

	return res, nil
}

func (b Broker) getPlanInstance(rctx *reqcontext.ReqContext, planID, instanceID string) (*crossplane.Plan, *crossplane.Instance, error) {
	if planID == "" {
		rctx.Logger.Info("find-instance-without-plan", lager.Data{"instance-id": instanceID})

		instance, p, exists, err := b.cp.FindInstanceWithoutPlan(rctx, instanceID)
		if err != nil {
			return nil, nil, err
		}
		if !exists {
			return nil, nil, apiresponses.ErrInstanceDoesNotExist
		}
		return p, instance, nil
	}
	p, err := b.cp.Plan(rctx, planID)
	if err != nil {
		return nil, nil, err
	}

	instance, exists, err := b.cp.Instance(rctx, instanceID, p)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, nil, apiresponses.ErrInstanceDoesNotExist
	}
	return p, instance, nil
}
