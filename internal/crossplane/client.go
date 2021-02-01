package crossplane

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"code.cloudfoundry.org/lager"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	crossplane "github.com/crossplane/crossplane/apis"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	cgscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// errInstanceNotFound is an instance doesn't exist
var errInstanceNotFound = errors.New("instance not found")

// Crossplane client to access crossplane resources.
type Crossplane struct {
	client            k8sclient.Client
	logger            lager.Logger
	DownstreamClients map[string]k8sclient.Client
	serviceIDs        []string
	namespace         string
}

// Register configures the given runtime.Scheme with all required resources
func Register(scheme *runtime.Scheme) error {
	if err := cgscheme.AddToScheme(scheme); err != nil {
		return err
	}

	sBuilder := runtime.NewSchemeBuilder(func(s *runtime.Scheme) error {
		metav1.AddToGroupVersion(s, schema.GroupVersion{Group: "syn.tools", Version: "v1alpha1"})
		return nil
	})
	if err := sBuilder.AddToScheme(scheme); err != nil {
		return err
	}
	if err := crossplane.AddToScheme(scheme); err != nil {
		return err
	}

	return nil
}

// New instantiates a crossplane client.
func New(serviceIDs []string, namespace string, config *rest.Config, logger lager.Logger) (*Crossplane, error) {
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
		namespace:  namespace,
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

// ServiceXRDs retrieves all defined services (defined by XRDs with the ServiceIDLabel) on the cluster.
func (cp Crossplane) ServiceXRDs(ctx context.Context) ([]*ServiceXRD, error) {
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

	compositions := &xv1.CompositionList{}
	err = cp.client.List(ctx, compositions, client.MatchingLabelsSelector{
		Selector: labels.NewSelector().Add(*req),
	})
	sort.Slice(compositions.Items, func(i, j int) bool {
		return compositions.Items[i].Labels[PlanNameLabel] < compositions.Items[j].Labels[PlanNameLabel]
	})

	plans := make([]*Plan, len(compositions.Items))
	for i, c := range compositions.Items {
		p, err := newPlan(c)
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
	composition := xv1.Composition{}
	err := cp.client.Get(ctx, types.NamespacedName{Name: planID}, &composition)
	if err != nil {
		return nil, err
	}
	return newPlan(composition)
}

// Instances retrieves an instance based on the given instanceID and plan. Besides the instanceID, the planName
// has to match.
// The `ok` parameter is *only* set to true if no error is returned and the instance already exists.
// Normal errors are returned as-is.
// FIXME(mw): is it correct to return `false, errInstanceNotFound` if PlanNameLabel does not match? And how should that be handled?
//            Ported from PoC code as-is, and errInstanceNotFound is handled as instance really not found, however as the ID exists, how
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
		return nil, false, errInstanceNotFound
	}

	return inst, true, nil
}

// FindInstanceWithoutPlan is used for retrieving an instance when the plan is unknown.
// It needs to iterate through all plans and fetch the instance using the supplied GVK.
// There's probably an optimization to be done here, as this seems fairly shitty, but for now it works.
func (cp Crossplane) FindInstanceWithoutPlan(ctx context.Context, id string) (inst *Instance, p *Plan, ok bool, err error) {
	plans, err := cp.Plans(ctx, cp.serviceIDs)
	if err != nil {
		return nil, nil, false, err
	}
	for _, plan := range plans {
		instance, exists, err := cp.Instance(ctx, id, plan)
		if err != nil {
			if err == errInstanceNotFound {
				// plan didn't match
				continue
			}
			return nil, nil, false, err
		}
		if !exists {
			continue
		}
		return instance, plan, true, nil
	}
	return nil, nil, false, errInstanceNotFound
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
		Name: plan.Composition.Name,
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

// DeleteInstance deletes a service instance
func (cp *Crossplane) DeleteInstance(ctx context.Context, instanceName string, plan *Plan) error {
	gvk, err := plan.GVK()
	if err != nil {
		return err
	}

	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(instanceName)

	return cp.client.Delete(ctx, cmp)
}

func (cp *Crossplane) getCredentials(ctx context.Context, name string) (*corev1.Secret, error) {
	secretRef := types.NamespacedName{
		Namespace: cp.namespace,
		Name:      name,
	}
	s := &corev1.Secret{}
	if err := cp.client.Get(ctx, secretRef, s); err != nil {
		return nil, fmt.Errorf("unable to get secret: %w", err)
	}
	if s.Data == nil {
		return nil, errors.New("nil secret data")
	}
	return s, nil
}
