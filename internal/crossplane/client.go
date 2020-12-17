package crossplane

import (
	"context"
	"sort"

	"code.cloudfoundry.org/lager"
	helm "github.com/crossplane-contrib/provider-helm/apis"
	crossplane "github.com/crossplane/crossplane/apis"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Crossplane client to access crossplane resources.
// TODO(mw): decide if k8sclient should be used or a standard client-go instead.
type Crossplane struct {
	client     k8sclient.Client
	logger     lager.Logger
	serviceIDs []string
}

// setupScheme configures the given runtime.Scheme with all required resources
func setupScheme(scheme *runtime.Scheme) error {
	sBuilder := runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		metav1.AddToGroupVersion(s, schema.GroupVersion{Group: "syn.tools", Version: "v1alpha1"})
		return nil
	})
	if err := sBuilder.AddToScheme(scheme); err != nil {
		return err
	}
	if err := helm.AddToScheme(scheme); err != nil {
		return err
	}
	if err := crossplane.AddToScheme(scheme); err != nil {
		return err
	}

	return nil
}

// New instantiates a crossplane client.
func New(serviceIDs []string, config *rest.Config, logger lager.Logger) (*Crossplane, error) {
	scheme := runtime.NewScheme()
	if err := setupScheme(scheme); err != nil {
		return nil, err
	}

	// TODO(mw): feels a little unnecessary to use controller-runtime just for this. Should we extract the code we need?
	k, err := k8sclient.New(config, k8sclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}

	cp := Crossplane{
		client:     k,
		logger:     logger,
		serviceIDs: serviceIDs,
	}

	return &cp, nil
}

type ServiceXRD struct {
	XRD         xv1.CompositeResourceDefinition
	Labels      *Labels
	Metadata    string
	Tags        string
	Description string
}

func (cp *Crossplane) ServiceXRDs(ctx context.Context) ([]ServiceXRD, error) {
	xrds := &xv1.CompositeResourceDefinitionList{}

	req, err := labels.NewRequirement(ServiceIDLabel, selection.In, cp.serviceIDs)
	if err != nil {
		return nil, err
	}

	err = cp.client.List(ctx, xrds, client.MatchingLabelsSelector{
		Selector: labels.NewSelector().Add(*req),
	})
	if err != nil {
		return nil, err
	}

	sxrds := make([]ServiceXRD, len(xrds.Items))
	for i, xrd := range xrds.Items {
		l, err := parseLabels(xrd.Labels)
		if err != nil {
			return nil, err
		}
		sxrds[i] = ServiceXRD{
			XRD:         xrd,
			Labels:      l,
			Metadata:    xrd.Annotations[MetadataAnnotation],
			Tags:        xrd.Annotations[TagsAnnotation],
			Description: xrd.Annotations[DescriptionAnnotation],
		}
	}

	return sxrds, nil
}

type Plan struct {
	Composition xv1.Composition
	Labels      *Labels
	Metadata    string
	Tags        string
	Description string
}

func (cp *Crossplane) Plans(ctx context.Context, serviceIDs []string) ([]Plan, error) {
	req, err := labels.NewRequirement(ServiceIDLabel, selection.In, serviceIDs)
	if err != nil {
		return nil, err
	}

	compositions := &xv1.CompositionList{}
	err = cp.client.List(ctx, compositions, client.MatchingLabelsSelector{
		Selector: labels.NewSelector().Add(*req),
	})
	sort.Slice(compositions.Items, func(i, j int) bool {
		return compositions.Items[i].Labels[PlanNameLabel] < compositions.Items[j].Labels[PlanNameLabel]
	})

	plans := make([]Plan, len(compositions.Items))
	for i, c := range compositions.Items {
		l, err := parseLabels(c.Labels)
		if err != nil {
			return nil, err
		}
		plans[i] = Plan{
			Composition: c,
			Labels:      l,
			Metadata:    c.Annotations[MetadataAnnotation],
			Tags:        c.Annotations[TagsAnnotation],
			Description: c.Annotations[DescriptionAnnotation],
		}
	}

	return plans, nil
}
