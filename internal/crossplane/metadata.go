package crossplane

import (
	"fmt"
	"strconv"
)

const (
	// SynToolsBase is the base domain
	SynToolsBase = "service.syn.tools"

	// DescriptionAnnotation of the instance
	DescriptionAnnotation = SynToolsBase + "/description"
	// MetadataAnnotation of the instance
	MetadataAnnotation = SynToolsBase + "/metadata"
	// DeletionTimestampAnnotation marks when an object got deleted
	DeletionTimestampAnnotation = SynToolsBase + "/deletionTimestamp"
	// TagsAnnotation of the instance
	TagsAnnotation = SynToolsBase + "/tags"
)

const (
	// ServiceNameLabel of the instance
	ServiceNameLabel = SynToolsBase + "/name"
	// ServiceIDLabel of the instance
	ServiceIDLabel = SynToolsBase + "/id"
	// PlanNameLabel of the instance
	PlanNameLabel = SynToolsBase + "/plan"
	// ClusterLabel name of the cluster this instance is deployed to
	ClusterLabel = SynToolsBase + "/cluster"
	// SLALabel SLA level for this instance
	SLALabel = SynToolsBase + "/sla"
	// InstanceIDLabel of the instance
	InstanceIDLabel = SynToolsBase + "/instance"
	// ParentIDLabel of the instance
	ParentIDLabel = SynToolsBase + "/parent"
	// BindableLabel of the instance
	BindableLabel = SynToolsBase + "/bindable"
	// UpdatableLabel of the instance
	UpdatableLabel = SynToolsBase + "/updatable"
	// DeletedLabel marks an object as deleted to clean up
	DeletedLabel = SynToolsBase + "/deleted"
)

type Labels struct {
	ServiceName Service
	ServiceID   string
	PlanName    string
	InstanceID  string
	ParentID    string
	Bindable    bool
	Updatable   bool
	Deleted     bool
}

func parseLabels(l map[string]string) (*Labels, error) {
	md := Labels{
		ServiceName: Service(l[ServiceNameLabel]),
		ServiceID:   l[ServiceIDLabel],
		PlanName:    l[PlanNameLabel],
		InstanceID:  l[InstanceIDLabel],
		ParentID:    l[ParentIDLabel],
		Bindable:    true,
		Deleted:     false,
	}
	var err error

	if !md.ServiceName.IsValid() {
		return nil, fmt.Errorf("service %q not valid", md.ServiceName)
	}

	md.Bindable, err = parseBoolLabel(l[BindableLabel], true)
	if err != nil {
		return nil, err
	}
	md.Updatable, err = parseBoolLabel(l[UpdatableLabel], false)
	if err != nil {
		return nil, err
	}
	md.Deleted, err = parseBoolLabel(l[DeletedLabel], false)
	if err != nil {
		return nil, err
	}
	return &md, nil
}

func parseBoolLabel(s string, def bool) (bool, error) {
	if s == "" {
		return def, nil
	}
	return strconv.ParseBool(s)
}
