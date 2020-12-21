package crossplane

import (
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	xv1beta1 "github.com/crossplane/crossplane/apis/apiextensions/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Plan struct {
	Composition *xv1beta1.Composition
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

func newPlan(c *xv1beta1.Composition) (*Plan, error) {
	l, err := parseLabels(c.Labels)
	if err != nil {
		return nil, err
	}
	return &Plan{
		Composition: c,
		Labels:      l,
		Metadata:    c.Annotations[MetadataAnnotation],
		Tags:        c.Annotations[TagsAnnotation],
		Description: c.Annotations[DescriptionAnnotation],
	}, nil
}

type Instance struct {
	Composition *composite.Unstructured
	Labels      *Labels
}

func newInstance(c *composite.Unstructured) (*Instance, error) {
	l, err := parseLabels(c.GetLabels())
	if err != nil {
		return nil, err
	}
	return &Instance{
		Composition: c,
		Labels:      l,
	}, nil
}
