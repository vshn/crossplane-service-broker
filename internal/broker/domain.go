package broker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	"github.com/pivotal-cf/brokerapi/v7/middlewares"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/pointer"

	"github.com/vshn/crossplane-service-broker/internal/crossplane"
)

func newService(service *crossplane.ServiceXRD, plans []domain.ServicePlan, logger lager.Logger) domain.Service {
	meta := &domain.ServiceMetadata{}
	if err := json.Unmarshal([]byte(service.Metadata), meta); err != nil {
		logger.Error("parse-metadata", err)
		meta.DisplayName = service.Labels.ServiceName
	}

	var tags []string
	if err := json.Unmarshal([]byte(service.Tags), &tags); err != nil {
		logger.Error("parse-tags", err)
	}

	return domain.Service{
		ID:                   service.Labels.ServiceID,
		Name:                 service.Labels.ServiceName,
		Description:          service.Description,
		Bindable:             service.Labels.Bindable,
		InstancesRetrievable: true,
		BindingsRetrievable:  service.Labels.Bindable,
		PlanUpdatable:        service.Labels.Updatable,
		Plans:                plans,
		Metadata:             meta,
		Tags:                 tags,
	}
}

func newServicePlan(plan *crossplane.Plan, logger lager.Logger) domain.ServicePlan {
	planName := plan.Labels.PlanName
	meta := &domain.ServicePlanMetadata{}
	if err := json.Unmarshal([]byte(plan.Metadata), meta); err != nil {
		logger.Error("parse-metadata", err)
		meta.DisplayName = planName
	}
	return domain.ServicePlan{
		ID:          plan.Composition.Name,
		Name:        planName,
		Description: plan.Description,
		Free:        pointer.BoolPtr(false),
		Bindable:    &plan.Labels.Bindable,
		Metadata:    meta,
	}
}

// toApiResponseError converts an error to a proper API error
func toApiResponseError(ctx context.Context, err error) *apiresponses.FailureResponse {
	id, ok := ctx.Value(middlewares.CorrelationIDKey).(string)
	if !ok {
		id = "unknown"
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
		return apiErr.AppendErrorMessage(fmt.Sprintf("(correlation-id: %q)", id))
	}

	return apiresponses.NewFailureResponseBuilder(
		fmt.Errorf("%w (correlation-id: %q)", err, id),
		http.StatusInternalServerError,
		"internal-server-error",
	).Build()
}
