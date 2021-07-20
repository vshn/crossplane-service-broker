package crossplane

import (
	"errors"

	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// SLAPremium represents the string for the premium SLA
	SLAPremium = "premium"
	// SLAStandard represents the string for the standard SLA
	SLAStandard = "standard"
)

func getPlanSLAIndex(sla string) int {
	m := map[string]int{
		SLAStandard: 10,
		SLAPremium:  20,
	}
	i, ok := m[sla]
	if !ok {
		return -1
	}
	return i
}

const (
	// SizeXSmall represents the string for the xsmall plan size
	SizeXSmall = "xsmall"
	// SizeSmall represents the string for the small plan size
	SizeSmall = "small"
	// SizeMedium represents the string for the medium plan size
	SizeMedium = "medium"
	// SizeLarge represents the string for the large plan size
	SizeLarge = "large"
	// SizeXLarge represents the string for the xlarge plan size
	SizeXLarge = "xlarge"
)

// ErrSLAUnknown we do not know the provided SLA and cannot compare it
var ErrSLAUnknown = errors.New("unable to compare SLAs")

// ErrSizeUnknown we do not know the provided plan size and cannot compare it
var ErrSizeUnknown = errors.New("unable to compare plan sizes")

func getPlanSizeIndex(size string) int {
	m := map[string]int{
		SizeXSmall: 10,
		SizeSmall:  20,
		SizeMedium: 30,
		SizeLarge:  40,
		SizeXLarge: 50,
	}
	i, ok := m[size]
	if !ok {
		return -1
	}
	return i
}

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

// Cmp returns 0 if the plan size is equal to b, they might differ in their SLA.
// It will return -1 if the plan size is less than b, or 1 if the
// quantity is greater than b.
// Cmp will return an error if the plans are not comparable. For example if they are not
// part of the same service or the plan sizes are unknown.
func (p Plan) Cmp(b Plan) (int, error) {
	slaP := getPlanSLAIndex(p.Labels.SLA)
	slaB := getPlanSLAIndex(b.Labels.SLA)
	if slaP < 0 || slaB < 0 {
		return 0, ErrSLAUnknown
	}
	sizeP := getPlanSizeIndex(p.Labels.PlanSize)
	sizeB := getPlanSizeIndex(b.Labels.PlanSize)
	if sizeP < 0 || sizeB < 0 {
		return 0, ErrSizeUnknown
	}

	if sizeP < sizeB {
		return -1, nil
	}
	if sizeP > sizeB {
		return 1, nil
	}
	return 0, nil
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
