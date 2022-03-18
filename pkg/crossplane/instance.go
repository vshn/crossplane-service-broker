package crossplane

import (
	"fmt"

	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	corev1 "k8s.io/api/core/v1"
)

// Instance is a wrapper around a specific instance (a composite).
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

// ParentReference returns the parent reference
func (i Instance) ParentReference() (string, error) {
	return getParentRef(i.Parameters())
}

func getParentRef(params map[string]interface{}) (string, error) {
	p, ok := params[instanceParamsParentReferenceName]
	if !ok {
		return "", fmt.Errorf("required param %q not found", instanceParamsParentReferenceName)
	}
	return p.(string), nil
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

// GetClusterName returns the cluster name of the instance
func (instance Instance) GetClusterName() string {
	labels := instance.Composite.GetLabels()
	return labels["service.syn.tools/cluster"]
}
