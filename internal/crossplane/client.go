package crossplane

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"code.cloudfoundry.org/lager"
	helm "github.com/crossplane-contrib/provider-helm/apis"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	crossplane "github.com/crossplane/crossplane/apis"
	xv1beta1 "github.com/crossplane/crossplane/apis/apiextensions/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ErrInstanceNotFound is an instance doesn't exist
var ErrInstanceNotFound = errors.New("instance not found")

const (
	// instanceSpecParamsPath is the path to an instance's parameters
	instanceSpecParamsPath = "spec.parameters"

	// instanceParamsParentReferenceName is the name of an instance's parent reference parameter
	instanceParamsParentReferenceName = "parent_reference"
	// instanceSpecParamsParentReferencePath is the path to an instance's parent reference parameter
	instanceSpecParamsParentReferencePath = instanceSpecParamsPath + "." + instanceParamsParentReferenceName
)

// Crossplane client to access crossplane resources.
// TODO(mw): decide if k8sclient should be used or a standard client-go instead.
type Crossplane struct {
	client     k8sclient.Client
	logger     lager.Logger
	serviceIDs []string
}

// Register configures the given runtime.Scheme with all required resources
func Register(scheme *runtime.Scheme) error {
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
	if err := Register(scheme); err != nil {
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
	XRD         xv1beta1.CompositeResourceDefinition
	Labels      *Labels
	Metadata    string
	Tags        string
	Description string
}

// ServiceXRDs retrieves all defined services (defined by XRDs with the ServiceIDLabel) on the cluster.
func (cp Crossplane) ServiceXRDs(ctx context.Context) ([]*ServiceXRD, error) {
	xrds := &xv1beta1.CompositeResourceDefinitionList{}

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

	sxrds := make([]*ServiceXRD, len(xrds.Items))
	for i, xrd := range xrds.Items {
		l, err := parseLabels(xrd.Labels)
		if err != nil {
			return nil, err
		}
		sxrds[i] = &ServiceXRD{
			XRD:         xrd,
			Labels:      l,
			Metadata:    xrd.Annotations[MetadataAnnotation],
			Tags:        xrd.Annotations[TagsAnnotation],
			Description: xrd.Annotations[DescriptionAnnotation],
		}
	}

	return sxrds, nil
}

// Plans retrieves all plans per passed service. Plans are deployed Compositions with the ServiceIDLabel
// assigned. The plans are ordered by name.
func (cp Crossplane) Plans(ctx context.Context, serviceIDs []string) ([]*Plan, error) {
	req, err := labels.NewRequirement(ServiceIDLabel, selection.In, serviceIDs)
	if err != nil {
		return nil, err
	}

	compositions := &xv1beta1.CompositionList{}
	err = cp.client.List(ctx, compositions, client.MatchingLabelsSelector{
		Selector: labels.NewSelector().Add(*req),
	})
	sort.Slice(compositions.Items, func(i, j int) bool {
		return compositions.Items[i].Labels[PlanNameLabel] < compositions.Items[j].Labels[PlanNameLabel]
	})

	plans := make([]*Plan, len(compositions.Items))
	for i, c := range compositions.Items {
		p, err := newPlan(&c)
		if err != nil {
			return nil, err
		}
		plans[i] = p
	}

	return plans, nil
}

// Plan retrieves a single plan as deployed using a Composition. The planID corresponds to the
// Compositions name.
func (cp Crossplane) Plan(ctx context.Context, planID string) (*Plan, error) {
	composition := &xv1beta1.Composition{}
	err := cp.client.Get(ctx, types.NamespacedName{Name: planID}, composition)
	if err != nil {
		return nil, err
	}
	return newPlan(composition)
}

// Instances retrieves an instance based on the given instanceID and plan. Besides the instanceID, the planName
// has to match.
// The `ok` parameter is *only* set to true if no error is returned and the instance already exists.
// Normal errors are returned as-is.
// FIXME(mw): is it correct to return `false, ErrInstanceNotFound` if PlanNameLabel does not match? And how should that be handled?
//            Ported from PoC code as-is, and ErrInstanceNotFound is handled as instance really not found, however as the ID exists, how
//            can we speak about having no instance with that name? It's a UUID after all.
func (cp Crossplane) Instance(ctx context.Context, id string, plan *Plan) (inst *Instance, ok bool, err error) {
	gvk, err := plan.GVK()

	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(id)

	err = cp.client.Get(ctx, types.NamespacedName{
		Name: id,
	}, cmp)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	inst, err = newInstance(cmp)
	if err != nil {
		return nil, false, err
	}

	if inst.Labels.PlanName != plan.Labels.PlanName {
		// TODO(mw): should we log here? PoC code logs.
		return nil, false, ErrInstanceNotFound
	}

	return inst, true, nil
}

// CreateInstance sets a new composite with assigned plan and params up.
// TODO(mw): simplify, refactor
func (cp Crossplane) CreateInstance(ctx context.Context, id string, plan *Plan, params json.RawMessage) error {
	l := map[string]string{
		InstanceIDLabel: id,
	}
	// Copy relevant labels from plan
	for _, name := range []string{ServiceIDLabel, ServiceNameLabel, PlanNameLabel, ClusterLabel, SLALabel} {
		l[name] = plan.Composition.Labels[name]
	}

	gvk, err := plan.GVK()
	if err != nil {
		return err
	}

	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(id)
	cmp.SetCompositionReference(&corev1.ObjectReference{
		Name: plan.Labels.PlanName,
	})
	paramMap := map[string]interface{}{}
	if params != nil {
		if err := json.Unmarshal(params, &paramMap); err != nil {
			return err
		}
		if parentReference, err := fieldpath.Pave(paramMap).GetString(instanceParamsParentReferenceName); err == nil {
			// Set parent reference in a label so we can search for it later.
			l[ParentIDLabel] = parentReference
		}
	}
	if err := fieldpath.Pave(cmp.Object).SetValue(instanceSpecParamsPath, paramMap); err != nil {
		return err
	}
	cmp.SetLabels(l)
	cp.logger.Debug("create-instance", lager.Data{"instance": cmp})
	return cp.client.Create(ctx, cmp)
}
