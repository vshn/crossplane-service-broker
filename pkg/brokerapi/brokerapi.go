package brokerapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/vshn/crossplane-service-broker/pkg/crossplane"
	"github.com/vshn/crossplane-service-broker/pkg/reqcontext"
)

// BrokerAPI implements a ServiceBroker.
type BrokerAPI struct {
	broker *Broker
	logger lager.Logger
}

func New(cp *crossplane.Crossplane, logger lager.Logger) (*BrokerAPI, error) {
	return &BrokerAPI{
		broker: NewBroker(cp),
		logger: logger,
	}, nil
}

// Services gets the catalog of services offered by the service broker
//   GET /v2/catalog
func (b BrokerAPI) Services(ctx context.Context) ([]domain.Service, error) {
	rctx := reqcontext.NewReqContext(ctx, b.logger, nil)
	rctx.Logger.Info("get-catalog")

	res, err := b.broker.Services(rctx)
	return res, APIResponseError(rctx, err)
}

// Provision creates a new service instance
//   PUT /v2/service_instances/{instance_id}
func (b BrokerAPI) Provision(ctx context.Context, instanceID string, details domain.ProvisionDetails, asyncAllowed bool) (domain.ProvisionedServiceSpec, error) {
	rctx := reqcontext.NewReqContext(ctx, b.logger, lager.Data{
		"instance-id": instanceID,
		"plan-id":     details.PlanID,
		"service-id":  details.ServiceID,
	})
	rctx.Logger.Info("provision-instance")

	if !asyncAllowed {
		return domain.ProvisionedServiceSpec{}, APIResponseError(rctx, apiresponses.ErrAsyncRequired)
	}

	res, err := b.broker.Provision(rctx, instanceID, details.PlanID, details.RawParameters)
	return res, APIResponseError(rctx, err)
}

// Deprovision deletes an existing service instance
//  DELETE /v2/service_instances/{instance_id}
func (b BrokerAPI) Deprovision(ctx context.Context, instanceID string, details domain.DeprovisionDetails, asyncAllowed bool) (domain.DeprovisionServiceSpec, error) {
	rctx := reqcontext.NewReqContext(ctx, b.logger, lager.Data{
		"instance-id": instanceID,
		"plan-id":     details.PlanID,
		"service-id":  details.ServiceID,
	})
	rctx.Logger.Info("deprovision-instance")

	res, err := b.broker.Deprovision(rctx, instanceID, details.PlanID)
	return res, APIResponseError(rctx, err)
}

// GetInstance fetches information about a service instance
//   GET /v2/service_instances/{instance_id}
func (b BrokerAPI) GetInstance(ctx context.Context, instanceID string) (domain.GetInstanceDetailsSpec, error) {
	rctx := reqcontext.NewReqContext(ctx, b.logger, lager.Data{
		"instance-id": instanceID,
	})
	rctx.Logger.Info("get-instance")

	res, err := b.broker.GetInstance(rctx, instanceID)
	return res, APIResponseError(rctx, err)
}

// Update modifies an existing service instance
//  PATCH /v2/service_instances/{instance_id}
func (b BrokerAPI) Update(ctx context.Context, instanceID string, details domain.UpdateDetails, asyncAllowed bool) (domain.UpdateServiceSpec, error) {
	rctx := reqcontext.NewReqContext(ctx, b.logger, lager.Data{
		"instance-id": instanceID,
		"plan-id":     details.PlanID,
		"service-id":  details.ServiceID,
	})
	rctx.Logger.Info("update-service-instance")

	res, err := b.broker.Update(rctx, instanceID, details.ServiceID, details.PreviousValues.PlanID, details.PlanID)
	if err != nil {
		switch err {
		case ErrPlanChangeNotPermitted, ErrServiceUpdateNotPermitted:
			err = apiresponses.NewFailureResponse(err, http.StatusUnprocessableEntity, "update-instance-failed")
		}
		return res, APIResponseError(rctx, err)
	}

	return res, nil
}

// LastOperation fetches last operation state for a service instance
//   GET /v2/service_instances/{instance_id}/last_operation
func (b BrokerAPI) LastOperation(ctx context.Context, instanceID string, details domain.PollDetails) (domain.LastOperation, error) {
	rctx := reqcontext.NewReqContext(ctx, b.logger, lager.Data{
		"instance-id": instanceID,
		"plan-id":     details.PlanID,
		"service-id":  details.ServiceID,
	})
	rctx.Logger.Info("last-operation", lager.Data{"operation-data": details.OperationData})

	res, err := b.broker.LastOperation(rctx, instanceID, details.PlanID)
	return res, APIResponseError(rctx, err)
}

// Bind creates a new service binding
//   PUT /v2/service_instances/{instance_id}/service_bindings/{binding_id}
func (b BrokerAPI) Bind(ctx context.Context, instanceID, bindingID string, details domain.BindDetails, asyncAllowed bool) (domain.Binding, error) {
	rctx := reqcontext.NewReqContext(ctx, b.logger, lager.Data{
		"instance-id": instanceID,
		"binging-id":  bindingID,
		"plan-id":     details.PlanID,
		"service-id":  details.ServiceID,
	})
	rctx.Logger.Info("bind-instance")

	res, err := b.broker.Bind(rctx, instanceID, bindingID, details.PlanID)
	return res, APIResponseError(rctx, err)
}

// Unbind deletes an existing service binding
//   DELETE /v2/service_instances/{instance_id}/service_bindings/{binding_id}
func (b BrokerAPI) Unbind(ctx context.Context, instanceID, bindingID string, details domain.UnbindDetails, asyncAllowed bool) (domain.UnbindSpec, error) {
	rctx := reqcontext.NewReqContext(ctx, b.logger, lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
		"plan-id":     details.PlanID,
		"service-id":  details.ServiceID,
	})
	rctx.Logger.Info("unbind-instance")

	res, err := b.broker.Unbind(rctx, instanceID, bindingID, details.PlanID)
	return res, APIResponseError(rctx, err)
}

// GetBinding fetches an existing service binding
//   GET /v2/service_instances/{instance_id}/service_bindings/{binding_id}
// TODO(mw): adjust to use details.PlanID when https://github.com/pivotal-cf/brokerapi/pull/138 is merged.
func (b BrokerAPI) GetBinding(ctx context.Context, instanceID, bindingID string) (domain.GetBindingSpec, error) {
	rctx := reqcontext.NewReqContext(ctx, b.logger, lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
	})
	rctx.Logger.Info("get-binding")

	res, err := b.broker.GetBinding(rctx, instanceID, bindingID)
	return res, APIResponseError(rctx, err)
}

// LastBindingOperation fetches last operation state for a service binding
//   GET /v2/service_instances/{instance_id}/service_bindings/{binding_id}/last_operation
func (b BrokerAPI) LastBindingOperation(ctx context.Context, instanceID, bindingID string, details domain.PollDetails) (domain.LastOperation, error) {
	rctx := reqcontext.NewReqContext(ctx, b.logger, lager.Data{
		"instance-id": instanceID,
		"binding-id":  bindingID,
		"plan-id":     details.PlanID,
		"service-id":  details.ServiceID,
	})
	rctx.Logger.Info("last-binding-operation")

	res := domain.LastOperation{}
	return res, APIResponseError(rctx, errors.New("not implemented"))
}

// APIResponseError converts an error to a proper API error
func APIResponseError(rctx *reqcontext.ReqContext, err error) error {
	if err == nil {
		return nil
	}

	var kErr *k8serrors.StatusError
	if errors.As(err, &kErr) {
		err = apiresponses.NewFailureResponseBuilder(
			kErr,
			int(kErr.ErrStatus.Code),
			"invalid",
		).WithErrorKey(string(kErr.ErrStatus.Reason)).Build()
	}

	var apiErr *apiresponses.FailureResponse
	if errors.As(err, &apiErr) {
		return apiErr.AppendErrorMessage(fmt.Sprintf("(correlation-id: %q)", rctx.CorrelationID))
	}

	return apiresponses.NewFailureResponseBuilder(
		fmt.Errorf("%w (correlation-id: %q)", err, rctx.CorrelationID),
		http.StatusInternalServerError,
		"internal-server-error",
	).Build()
}
