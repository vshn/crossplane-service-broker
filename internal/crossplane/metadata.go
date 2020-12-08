package crossplane

import "strconv"

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
	// InstanceIDLabel of the instance
	InstanceIDLabel = SynToolsBase + "/instance"
	// ParentIDLabel of the instance
	ParentIDLabel = SynToolsBase + "/parent"
	// BindableLabel of the instance
	BindableLabel = SynToolsBase + "/bindable"
	// DeletedLabel marks an object as deleted to clean up
	DeletedLabel = SynToolsBase + "/deleted"
)

type Labels struct {
	ServiceName string
	ServiceID   string
	PlanName    string
	InstanceID  string
	ParentID    string
	Bindable    bool
	Deleted     bool
}

func parseLabels(l map[string]string) (*Labels, error) {
	md := Labels{
		ServiceName: l[ServiceNameLabel],
		ServiceID:   l[ServiceIDLabel],
		PlanName:    l[PlanNameLabel],
		InstanceID:  l[InstanceIDLabel],
		ParentID:    l[ParentIDLabel],
		Bindable:    true,
		Deleted:     false,
	}
	var err error
	md.Bindable, err = parseBoolLabel(l[BindableLabel], true)
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
