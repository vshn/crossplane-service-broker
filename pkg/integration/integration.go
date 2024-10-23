package integration

import (
	"context"
	"os"
	"testing"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vshn/crossplane-service-broker/pkg/config"
	integrationtest "github.com/vshn/crossplane-service-broker/pkg/integration/test/integration"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vshn/crossplane-service-broker/pkg/crossplane"
)

// PrePostRunFunc is used for setup/teardown funcs.
type PrePostRunFunc func(c client.Client) error

// TestNamespace is used for all resources created during tests.
const TestNamespace = "test"

// UpdateInstanceConditions updates an instance's conditions within the status.
func UpdateInstanceConditions(ctx context.Context, c client.Client, servicePlan *crossplane.Plan, instance *composite.Unstructured, t xrv1.ConditionType, status corev1.ConditionStatus, reason xrv1.ConditionReason) error {
	cmp, err := GetInstance(ctx, c, servicePlan, instance.GetName())
	if err != nil {
		return err
	}

	// safe to ignore error as `getInstance()` does the same already
	gvk, _ := servicePlan.GVK()
	// need to re-add as it gets reset after GETting.
	cmp.SetGroupVersionKind(gvk)
	cmp.SetConditions(xrv1.Condition{
		Type:               t,
		Status:             status,
		Reason:             reason,
		LastTransitionTime: metav1.Now(),
	})
	return c.Update(ctx, cmp)
}

// GetInstance retrieves the specified instance.
func GetInstance(ctx context.Context, c client.Client, servicePlan *crossplane.Plan, instanceID string) (*composite.Unstructured, error) {
	gvk, err := servicePlan.GVK()
	if err != nil {
		return nil, err
	}
	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(instanceID)

	err = c.Get(ctx, types.NamespacedName{Name: instanceID}, cmp)
	if err != nil {
		return nil, err
	}
	return cmp, nil
}

// NewTestService creates a new service.
func NewTestService(serviceID string, serviceName crossplane.ServiceName) *xv1.CompositeResourceDefinition {
	return &xv1.CompositeResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "service",
			APIVersion: "syn.tools/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "testservice" + serviceID,
			Labels: map[string]string{
				crossplane.ServiceIDLabel:   serviceID,
				crossplane.ServiceNameLabel: string(serviceName),
				crossplane.BindableLabel:    "true",
				crossplane.UpdatableLabel:   "true",
			},
			Annotations: map[string]string{
				crossplane.DescriptionAnnotation: "testservice description",
				crossplane.MetadataAnnotation:    `{"displayName": "testservice"}`,
				crossplane.TagsAnnotation:        `["foo","bar","baz"]`,
			},
		},
		Spec: xv1.CompositeResourceDefinitionSpec{
			Versions: []xv1.CompositeResourceDefinitionVersion{
				{
					Name: "v1alpha1",
				},
			},
		},
	}
}

// NewTestServicePlan creates a new service plan.
func NewTestServicePlan(serviceID, planID string, serviceName crossplane.ServiceName) *crossplane.Plan {
	return NewTestServicePlanWithSize(serviceID, planID, serviceName, "small"+planID, "standard")
}

// NewTestServicePlanWithSize creates a new service plan with a specified size.
func NewTestServicePlanWithSize(serviceID, planID string, serviceName crossplane.ServiceName, name, sla string) *crossplane.Plan {
	return &crossplane.Plan{
		Labels: &crossplane.Labels{
			ServiceID:   serviceID,
			ServiceName: serviceName,
			PlanName:    name,
			SLA:         sla,
			Bindable:    false,
		},
		Composition: &xv1.Composition{
			TypeMeta: metav1.TypeMeta{
				Kind:       "servicePlan",
				APIVersion: "syn.tools/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: planID,
				Labels: map[string]string{
					crossplane.ServiceIDLabel:   serviceID,
					crossplane.ServiceNameLabel: string(serviceName),
					crossplane.PlanNameLabel:    name,
					crossplane.SLALabel:         sla,
					crossplane.BindableLabel:    "false",
				},
				Annotations: map[string]string{
					crossplane.DescriptionAnnotation: "testservice-small plan description",
					crossplane.MetadataAnnotation:    `{"displayName": "small"}`,
				},
			},
			Spec: xv1.CompositionSpec{
				Resources: []xv1.ComposedTemplate{},
				CompositeTypeRef: xv1.TypeReference{
					APIVersion: "syn.tools/v1alpha1",
					Kind:       kindForService(serviceName),
				},
			},
		},
	}
}

func kindForService(name crossplane.ServiceName) string {
	switch name {
	case crossplane.RedisService:
		return "CompositeRedisInstance"
	case crossplane.MariaDBService:
		return "CompositeMariaDBInstance"
	case crossplane.MariaDBDatabaseService:
		return "CompositeMariaDBDatabaseInstance"
	}
	return "CompositeInstance"
}

// NewTestInstance sets up an instance for testing.
func NewTestInstance(instanceID string, plan *crossplane.Plan, serviceName crossplane.ServiceName, serviceID, parent string) *composite.Unstructured {
	gvk, _ := plan.GVK()
	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(instanceID)
	cmp.SetCompositionReference(&corev1.ObjectReference{
		Name: plan.Labels.PlanName,
	})
	labels := map[string]string{
		crossplane.PlanNameLabel:    plan.Labels.PlanName,
		crossplane.ServiceIDLabel:   serviceID,
		crossplane.SLALabel:         plan.Labels.SLA,
		crossplane.ServiceNameLabel: string(serviceName),
		crossplane.ClusterLabel:     "dbaas-test-cluster",
	}
	if parent != "" {
		labels[crossplane.ParentIDLabel] = parent
		cmp.Object["spec"] = map[string]interface{}{
			"parameters": map[string]interface{}{
				"parent_reference": parent,
			},
		}
	}
	cmp.SetLabels(labels)
	cmp.SetResourceReferences([]corev1.ObjectReference{
		{
			Kind:       "Secret",
			Namespace:  TestNamespace,
			APIVersion: "v1",
			Name:       instanceID,
		},
	})
	cmp.SetWriteConnectionSecretToReference(&xrv1.SecretReference{
		Name:      instanceID,
		Namespace: TestNamespace,
	})

	return cmp
}

// NewTestSecret creates a new secret.
func NewTestSecret(namespace, name string, stringData map[string]string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		StringData: stringData,
	}
}

// NewTestMariaDBUserInstance sets up a specific mariadb user instance.
func NewTestMariaDBUserInstance(instanceID, bindingID string) *composite.Unstructured {
	gvk := schema.GroupVersionKind{
		Group:   "syn.tools",
		Version: "v1alpha1",
		Kind:    "CompositeMariaDBUserInstance",
	}
	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(bindingID)
	cmp.Object["spec"] = map[string]interface{}{
		"parameters": map[string]interface{}{
			"parent_reference": instanceID,
		},
	}
	cmp.SetWriteConnectionSecretToReference(&xrv1.SecretReference{
		Name:      bindingID,
		Namespace: TestNamespace,
	})

	return cmp
}

func newTestNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
}

// CreateObjects sets up the specified objects.
func CreateObjects(ctx context.Context, objs []client.Object) PrePostRunFunc {
	return func(c client.Client) error {
		for _, obj := range objs {
			if err := c.Create(ctx, obj.DeepCopyObject().(client.Object)); err != nil {
				return err
			}
		}
		return nil
	}
}

// RemoveObjects removes the specified objects.
func RemoveObjects(ctx context.Context, objs []client.Object) PrePostRunFunc {
	return func(c client.Client) error {
		for _, obj := range objs {
			if err := c.Delete(ctx, obj); err != nil {
				return err
			}
		}
		return nil
	}
}

// SetupManager creates the envtest manager setup with required objects.
func SetupManager(t *testing.T) (*integrationtest.Manager, lager.Logger, *crossplane.Crossplane, error) {
	if db := os.Getenv("DEBUG"); db != "" {
		zl, _ := zap.NewDevelopment()
		log.SetLogger(zapr.NewLogger(zl))
	}

	// unfortunately can't be set via an option on the manager as BinaryAssetsDir is not exposed.
	err := os.Setenv("KUBEBUILDER_ASSETS", "../../testdata/bin")
	require.NoError(t, err)

	m, err := integrationtest.New(nil,
		integrationtest.WithCRDPaths("../../testdata/crds"),
	)
	if err != nil {
		return nil, nil, nil, err
	}

	m.Run()

	scheme := m.GetScheme()
	assert.NoError(t, xv1.AddToScheme(scheme))
	assert.NoError(t, crossplane.Register(scheme))

	logger := lager.NewLogger("test")

	require.NoError(t, CreateObjects(context.Background(), []client.Object{newTestNamespace(TestNamespace)})(m.GetClient()))

	brokerConfig := &config.Config{ServiceIDs: []string{"1", "2"}, Namespace: TestNamespace, UsernameClaim: "sub", EnableMetrics: true, MetricsDomain: "metrics.example.tld"}
	cp, err := crossplane.New(brokerConfig, m.GetConfig())
	if err != nil {
		return nil, nil, nil, err
	}
	return m, logger, cp, nil
}

// BoolPtr returns a pointer to bool.
func BoolPtr(b bool) *bool {
	return &b
}
