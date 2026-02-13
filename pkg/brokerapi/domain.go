package brokerapi

import (
	"encoding/json"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/v8/domain"
	"k8s.io/utils/pointer"

	"github.com/vshn/crossplane-service-broker/pkg/crossplane"
)

func newService(service *crossplane.ServiceXRD, plans []domain.ServicePlan, logger lager.Logger) domain.Service {
	meta := &domain.ServiceMetadata{}
	if err := json.Unmarshal([]byte(service.Metadata), meta); err != nil {
		logger.Error("parse-metadata", err)
		meta.DisplayName = string(service.Labels.ServiceName)
	}

	var tags []string
	if err := json.Unmarshal([]byte(service.Tags), &tags); err != nil {
		logger.Error("parse-tags", err, lager.Data{"service": service.XRD.Name})
	}

	return domain.Service{
		ID:                   service.Labels.ServiceID,
		Name:                 string(service.Labels.ServiceName),
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
		logger.Error("parse-metadata", err, lager.Data{"plan": plan.Composition.Name})
		meta.DisplayName = planName
	}
	schemas := &domain.ServiceSchemas{}
	if err := json.Unmarshal([]byte(plan.Schemas), schemas); err != nil {
		logger.Info("parse-schemas", lager.Data{"msg": "No schemas or not parsable", "plan": plan.Composition.Name})
	}
	return domain.ServicePlan{
		ID:          plan.Composition.Name,
		Name:        planName,
		Description: plan.Description,
		Free:        pointer.BoolPtr(false),
		Bindable:    &plan.Labels.Bindable,
		Metadata:    meta,
		Schemas:     schemas,
	}
}
