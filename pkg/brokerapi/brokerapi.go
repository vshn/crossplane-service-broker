package brokerapi

import (
	"context"
	"errors"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	"github.com/pivotal-cf/brokerapi/v7/middlewares"
	"k8s.io/client-go/rest"

	"github.com/vshn/crossplane-service-broker/internal/broker"
	"github.com/vshn/crossplane-service-broker/internal/crossplane"
)

// BrokerAPI implements a ServiceBroker.
type BrokerAPI struct {
	broker *broker.Broker
	logger lager.Logger
}

func New(serviceIDs []string, namespace string, config *rest.Config, logger lager.Logger) (*BrokerAPI, error) {
	cp, err := crossplane.New(serviceIDs, namespace, config, logger.WithData(lager.Data{"module": "crossplane"}))
	if err != nil {
		return nil, err
	}
	b := broker.New(cp, logger.WithData(lager.Data{"module": "broker"}))
	return &BrokerAPI{
		broker: b,
		logger: logger,
	}, nil
}

// Services gets the catalog of services offered by the service broker
//   GET /v2/catalog
func (b BrokerAPI) Services(ctx context.Context) ([]domain.Service, error) {
	logger := requestScopedLogger(ctx, b.logger)
	logger.Info("get-catalog")

	return b.broker.Services(ctx)
}

// Provision creates a new service instance
//   PUT /v2/service_instances/{instance_id}
func (b BrokerAPI) Provision(ctx context.Context, instanceID string, details domain.ProvisionDetails, asyncAllowed bool) (domain.ProvisionedServiceSpec, error) {
	logger := requestScopedLogger(ctx, b.logger).WithData(lager.Data{"instance-id": instanceID})
	logger.Info("provision-instance", lager.Data{"plan-id": details.PlanID, "service-id": details.ServiceID})

	if !asyncAllowed {
		return domain.ProvisionedServiceSpec{}, apiresponses.ErrAsyncRequired
	}

	return b.broker.Provision(ctx, instanceID, details.PlanID, details.RawParameters)
}

// Deprovision deletes an existing service instance
//  DELETE /v2/service_instances/{instance_id}
func (b BrokerAPI) Deprovision(ctx context.Context, instanceID string, details domain.DeprovisionDetails, asyncAllowed bool) (domain.DeprovisionServiceSpec, error) {
	logger := requestScopedLogger(ctx, b.logger).WithData(lager.Data{"instance-id": instanceID})
	logger.Info("deprovision-instance", lager.Data{"plan-id": details.PlanID, "service-id": details.ServiceID})

	return b.broker.Deprovision(ctx, instanceID, details.PlanID)
}

// GetInstance fetches information about a service instance
//   GET /v2/service_instances/{instance_id}
func (b BrokerAPI) GetInstance(ctx context.Context, instanceID string) (domain.GetInstanceDetailsSpec, error) {
	spec := domain.GetInstanceDetailsSpec{}
	return spec, errors.New("not implemented")
}

// Update modifies an existing service instance
//  PATCH /v2/service_instances/{instance_id}
func (b BrokerAPI) Update(ctx context.Context, instanceID string, details domain.UpdateDetails, asyncAllowed bool) (domain.UpdateServiceSpec, error) {
	spec := domain.UpdateServiceSpec{}
	return spec, errors.New("not implemented")
}

// LastOperation fetches last operation state for a service instance
//   GET /v2/service_instances/{instance_id}/last_operation
func (b BrokerAPI) LastOperation(ctx context.Context, instanceID string, details domain.PollDetails) (domain.LastOperation, error) {
	logger := requestScopedLogger(ctx, b.logger).WithData(lager.Data{"instance-id": instanceID})
	logger.Info("last-operation", lager.Data{"operation-data": details.OperationData, "plan-id": details.PlanID, "service-id": details.ServiceID})

	return b.broker.LastOperation(ctx, instanceID, details.PlanID)
}

// Bind creates a new service binding
//   PUT /v2/service_instances/{instance_id}/service_bindings/{binding_id}
func (b BrokerAPI) Bind(ctx context.Context, instanceID, bindingID string, details domain.BindDetails, asyncAllowed bool) (domain.Binding, error) {
	logger := requestScopedLogger(ctx, b.logger).WithData(lager.Data{"instance-id": instanceID, "binding-id": bindingID})
	logger.Info("bind-instance", lager.Data{"plan-id": details.PlanID, "service-id": details.ServiceID})

	return b.broker.Bind(ctx, instanceID, bindingID, details.PlanID)
}

// Unbind deletes an existing service binding
//   DELETE /v2/service_instances/{instance_id}/service_bindings/{binding_id}
func (b BrokerAPI) Unbind(ctx context.Context, instanceID, bindingID string, details domain.UnbindDetails, asyncAllowed bool) (domain.UnbindSpec, error) {
	logger := requestScopedLogger(ctx, b.logger).WithData(lager.Data{"instance-id": instanceID, "binding-id": bindingID})
	logger.Info("unbind-instance", lager.Data{"plan-id": details.PlanID, "service-id": details.ServiceID})

	return b.broker.Unbind(ctx, instanceID, bindingID, details.PlanID)
}

// GetBinding fetches an existing service binding
//   GET /v2/service_instances/{instance_id}/service_bindings/{binding_id}
// TODO(mw): adjust to use details.PlanID when https://github.com/pivotal-cf/brokerapi/pull/138 is merged.
func (b BrokerAPI) GetBinding(ctx context.Context, instanceID, bindingID string) (domain.GetBindingSpec, error) {
	logger := requestScopedLogger(ctx, b.logger).WithData(lager.Data{"instance-id": instanceID, "binding-id": bindingID})
	logger.Info("get-binding", lager.Data{"binding-id": bindingID})

	return b.broker.GetBinding(ctx, instanceID, bindingID)
}

// LastBindingOperation fetches last operation state for a service binding
//   GET /v2/service_instances/{instance_id}/service_bindings/{binding_id}/last_operation
func (b BrokerAPI) LastBindingOperation(ctx context.Context, instanceID, bindingID string, details domain.PollDetails) (domain.LastOperation, error) {
	spec := domain.LastOperation{}
	return spec, errors.New("not implemented")
}

func requestScopedLogger(ctx context.Context, logger lager.Logger) lager.Logger {
	id, ok := ctx.Value(middlewares.CorrelationIDKey).(string)
	if !ok {
		id = "unknown"
	}

	return logger.WithData(lager.Data{"correlation-id": id})
}
