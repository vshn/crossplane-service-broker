package crossplane

import (
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Plan is a wrapper around a Composition representing a service plan.
type Plan struct {
	Composition *xv1.Composition
	Labels      *Labels
	Metadata    string
	Tags        string
	Description string
}

// GVK returns the group, version, kind type for the composite type ref.
func (p Plan) GVK() (schema.GroupVersionKind, error) {
	groupVersion, err := schema.ParseGroupVersion(p.Composition.Spec.CompositeTypeRef.APIVersion)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return groupVersion.WithKind(p.Composition.Spec.CompositeTypeRef.Kind), nil
}

func newPlan(c xv1.Composition) (*Plan, error) {
	l, err := parseLabels(c.Labels)
	if err != nil {
		return nil, err
	}
	return &Plan{
		Composition: &c,
		Labels:      l,
		Metadata:    c.Annotations[MetadataAnnotation],
		Tags:        c.Annotations[TagsAnnotation],
		Description: c.Annotations[DescriptionAnnotation],
	}, nil
}

// Instance is a wrapper around a specific instance  (a composite).
type Instance struct {
	Composite *composite.Unstructured
	Labels    *Labels
}

// ID returns the instance name.
func (i Instance) ID() string {
	return i.Composite.GetName()
}

// Ready returns if the instance contains a ready = true status.
func (i Instance) Ready() bool {
	return i.Composite.GetCondition(xrv1.TypeReady).Status == corev1.ConditionTrue
}

// Parameters returns the specified parameters if available.
func (i Instance) Parameters() map[string]interface{} {
	p, err := fieldpath.Pave(i.Composite.Object).GetValue(instanceSpecParamsPath)
	if err != nil {
		p = make(map[string]interface{})
	}
	v, ok := p.(map[string]interface{})
	if !ok {
		return make(map[string]interface{})
	}
	return v
}

// ResourceRefs returns all referenced resources.
func (i Instance) ResourceRefs() []corev1.ObjectReference {
	return i.Composite.GetResourceReferences()
}

func newInstance(c *composite.Unstructured) (*Instance, error) {
	l, err := parseLabels(c.GetLabels())
	if err != nil {
		return nil, err
	}
	return &Instance{
		Composite: c,
		Labels:    l,
	}, nil
}

// MariaDBProvisionAdditionalParams are the required parameters to create a mariadb database instance.
type MariaDBProvisionAdditionalParams struct {
	ParentReference string `json:"parent_reference"`
}
