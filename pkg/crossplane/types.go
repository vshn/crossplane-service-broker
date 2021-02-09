package crossplane

import (
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Plan struct {
	Composition *xv1.Composition
	Labels      *Labels
	Metadata    string
	Tags        string
	Description string
}

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

type Instance struct {
	Composite *composite.Unstructured
	Labels    *Labels
}

func (i Instance) ID() string {
	return i.Composite.GetName()
}

func (i Instance) Ready() bool {
	return i.Composite.GetCondition(xrv1.TypeReady).Status == corev1.ConditionTrue
}

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

type MariaDBProvisionAdditionalParams struct {
	ParentReference string `json:"parent_reference"`
}
