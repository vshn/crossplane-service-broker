package crossplane

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"code.cloudfoundry.org/lager"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	crossplane "github.com/crossplane/crossplane/apis"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"github.com/vshn/crossplane-service-broker/pkg/api/auth"
	"github.com/vshn/crossplane-service-broker/pkg/config"
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

	"github.com/vshn/crossplane-service-broker/pkg/reqcontext"
)

// errInstanceNotFound is an instance doesn't exist
var errInstanceNotFound = errors.New("instance not found")

// Crossplane client to access crossplane resources.
type Crossplane struct {
	config *config.Config
	client client.Client
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
func New(brokerConfig *config.Config, restConfig *rest.Config) (*Crossplane, error) {
	scheme := runtime.NewScheme()
	if err := Register(scheme); err != nil {
		return nil, err
	}

	// TODO(mw): feels a little unnecessary to use controller-runtime just for this. Should we extract the code we need?
	k, err := client.New(restConfig, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}

	cp := Crossplane{
		client: k,
		config: brokerConfig,
	}

	return &cp, nil
}

// ServiceXRD is a wrapper around a CompositeResourceDefinition (XRD) which represents a service.
type ServiceXRD struct {
	XRD         xv1.CompositeResourceDefinition
	Labels      *Labels
	Metadata    string
	Tags        string
	Description string
}

// ServiceXRDs retrieves all defined services (defined by XRDs with the ServiceIDLabel) on the cluster.
func (cp Crossplane) ServiceXRDs(rctx *reqcontext.ReqContext) ([]*ServiceXRD, error) {
	xrds := &xv1.CompositeResourceDefinitionList{}

	req, err := labels.NewRequirement(ServiceIDLabel, selection.In, cp.config.ServiceIDs)
	if err != nil {
		return nil, err
	}

	err = cp.client.List(rctx.Context, xrds, client.MatchingLabelsSelector{
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
func (cp Crossplane) Plans(rctx *reqcontext.ReqContext, serviceIDs []string) ([]*Plan, error) {
	req, err := labels.NewRequirement(ServiceIDLabel, selection.In, serviceIDs)
	if err != nil {
		return nil, err
	}

	compositions := &xv1.CompositionList{}
	err = cp.client.List(rctx.Context, compositions, client.MatchingLabelsSelector{
		Selector: labels.NewSelector().Add(*req),
	})
	if err != nil {
		return nil, err
	}
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
func (cp Crossplane) Plan(rctx *reqcontext.ReqContext, planID string) (*Plan, error) {
	composition := xv1.Composition{}
	err := cp.client.Get(rctx.Context, types.NamespacedName{Name: planID}, &composition)
	if err != nil {
		return nil, err
	}
	return newPlan(composition)
}

// Instance retrieves an instance based on the given instanceID and plan. Besides the instanceID, the planName
// has to match.
// The `ok` parameter is *only* set to true if no error is returned and the instance already exists.
// Normal errors are returned as-is.
// FIXME(mw): is it correct to return `false, errInstanceNotFound` if PlanNameLabel does not match? And how should that be handled?
//            Ported from PoC code as-is, and errInstanceNotFound is handled as instance really not found, however as the ID exists, how
//            can we speak about having no instance with that name? It's a UUID after all.
func (cp Crossplane) Instance(rctx *reqcontext.ReqContext, id string, plan *Plan) (inst *Instance, ok bool, err error) {
	gvk, err := plan.GVK()
	if err != nil {
		return nil, false, err
	}

	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(id)

	err = cp.client.Get(rctx.Context, types.NamespacedName{
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
func (cp Crossplane) FindInstanceWithoutPlan(rctx *reqcontext.ReqContext, id string) (inst *Instance, p *Plan, ok bool, err error) {
	plans, err := cp.Plans(rctx, cp.config.ServiceIDs)
	if err != nil {
		return nil, nil, false, err
	}
	for _, plan := range plans {
		instance, exists, err := cp.Instance(rctx, id, plan)
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
func (cp Crossplane) CreateInstance(rctx *reqcontext.ReqContext, id string, plan *Plan, params map[string]interface{}) error {
	l, err := cp.prepareLabels(rctx, id, plan, params)
	if err != nil {
		return err
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

	if err := fieldpath.Pave(cmp.Object).SetValue(instanceSpecParamsPath, params); err != nil {
		return err
	}
	cmp.SetLabels(l)
	rctx.Logger.Debug("create-instance", lager.Data{"instance": cmp})
	return cp.client.Create(rctx.Context, cmp)
}

func (cp Crossplane) prepareLabels(rctx *reqcontext.ReqContext, id string, plan *Plan, params map[string]interface{}) (map[string]string, error) {
	principal, err := auth.PrincipalFromContext(rctx.Context, cp.config)
	if err != nil {
		return nil, err
	}

	l := map[string]string{
		InstanceIDLabel: id,
		PrincipalLabel:  string(principal),
	}

	// Copy relevant labels from plan
	planLabels := []string{ServiceIDLabel, ServiceNameLabel, PlanNameLabel, ClusterLabel, SLALabel}
	for _, name := range planLabels {
		l[name] = plan.Composition.Labels[name]
	}

	// slightly ugly having this service specific label setting leaking out to the generic code. Can be cleaned up later
	// in case more specific code is needed.
	if params[instanceParamsParentReferenceName] != nil {
		// Additionally set parent reference in a label so we can search for it later.
		l[ParentIDLabel] = params[instanceParamsParentReferenceName].(string)
	}
	return l, nil
}

// UpdateInstance updates `instance` on k8s.
func (cp *Crossplane) UpdateInstance(rctx *reqcontext.ReqContext, instance *Instance, plan *Plan) error {
	gvk, err := plan.GVK()
	if err != nil {
		return err
	}
	instance.Composite.SetGroupVersionKind(gvk)

	return cp.client.Update(rctx.Context, instance.Composite.GetUnstructured())
}

// DeleteInstance deletes a service instance
func (cp *Crossplane) DeleteInstance(rctx *reqcontext.ReqContext, instanceName string, plan *Plan) error {
	gvk, err := plan.GVK()
	if err != nil {
		return err
	}

	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(instanceName)

	return cp.client.Delete(rctx.Context, cmp)
}

// GetConnectionDetails returns the connection details of an instance
func (cp *Crossplane) GetConnectionDetails(ctx context.Context, instance *composite.Unstructured) (*corev1.Secret, error) {
	if instance == nil {
		return nil, fmt.Errorf("composite is nil")
	}
	secretRef := instance.GetWriteConnectionSecretToReference()

	if secretRef == nil {
		return nil, fmt.Errorf("instance doesn't contain secret ref %q", instance.GetName())
	}

	s := &corev1.Secret{}
	if err := cp.client.Get(ctx, types.NamespacedName{
		Name:      secretRef.Name,
		Namespace: secretRef.Namespace,
	}, s); err != nil {
		return nil, fmt.Errorf("unable to get secret: %w", err)
	}
	if s.Data == nil {
		return nil, errors.New("nil secret data")
	}
	return s, nil
}
