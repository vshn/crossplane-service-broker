package integration

import (
	"context"
	"os"
	"testing"

	"code.cloudfoundry.org/lager"
	v1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	"github.com/crossplane/crossplane-runtime/pkg/test/integration"
	v14 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	v12 "k8s.io/api/core/v1"
	v13 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vshn/crossplane-service-broker/pkg/crossplane"
)

type PrePostRunFunc func(c client.Client) error
type CustomCheckFunc func(t *testing.T, c client.Client)

const TestNamespace = "test"

func UpdateInstanceConditions(ctx context.Context, c client.Client, servicePlan *crossplane.Plan, instance *composite.Unstructured, t v1.ConditionType, status v12.ConditionStatus, reason v1.ConditionReason) error {
	cmp, err := GetInstance(ctx, c, servicePlan, instance.GetName())
	if err != nil {
		return err
	}

	// safe to ignore error as `getInstance()` does the same already
	gvk, _ := servicePlan.GVK()
	// need to re-add as it gets reset after GETting.
	cmp.SetGroupVersionKind(gvk)
	cmp.SetConditions(v1.Condition{
		Type:               t,
		Status:             status,
		Reason:             reason,
		LastTransitionTime: v13.Now(),
	})
	return c.Update(ctx, cmp)
}

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

func NewTestService(serviceID string, serviceName crossplane.ServiceName) *v14.CompositeResourceDefinition {
	return &v14.CompositeResourceDefinition{
		TypeMeta: v13.TypeMeta{
			Kind:       "service",
			APIVersion: "syn.tools/v1alpha1",
		},
		ObjectMeta: v13.ObjectMeta{
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
		Spec: v14.CompositeResourceDefinitionSpec{
			Versions: []v14.CompositeResourceDefinitionVersion{
				{
					Name: "v1alpha1",
				},
			},
		},
	}
}

func NewTestServicePlan(serviceID, planID string, serviceName crossplane.ServiceName) *crossplane.Plan {
	name := "small" + planID
	return &crossplane.Plan{
		Labels: &crossplane.Labels{
			PlanName: name,
		},
		Composition: &v14.Composition{
			TypeMeta: v13.TypeMeta{
				Kind:       "servicePlan",
				APIVersion: "syn.tools/v1alpha1",
			},
			ObjectMeta: v13.ObjectMeta{
				Name: planID,
				Labels: map[string]string{
					crossplane.ServiceIDLabel:   serviceID,
					crossplane.ServiceNameLabel: string(serviceName),
					crossplane.PlanNameLabel:    name,
					crossplane.BindableLabel:    "false",
				},
				Annotations: map[string]string{
					crossplane.DescriptionAnnotation: "testservice-small plan description",
					crossplane.MetadataAnnotation:    `{"displayName": "small"}`,
				},
			},
			Spec: v14.CompositionSpec{
				Resources: []v14.ComposedTemplate{},
				CompositeTypeRef: v14.TypeReference{
					APIVersion: "syn.tools/v1alpha1",
					Kind:       kindForService(serviceName),
				},
			},
		},
	}
}

func NewTestServicePlanWithSize(serviceID, planID string, serviceName crossplane.ServiceName, name, sla string) *crossplane.Plan {
	return &crossplane.Plan{
		Labels: &crossplane.Labels{
			ServiceID:   serviceID,
			ServiceName: serviceName,
			PlanName:    name,
			SLA:         sla,
			Bindable:    false,
		},
		Composition: &v14.Composition{
			TypeMeta: v13.TypeMeta{
				Kind:       "servicePlan",
				APIVersion: "syn.tools/v1alpha1",
			},
			ObjectMeta: v13.ObjectMeta{
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
			Spec: v14.CompositionSpec{
				Resources: []v14.ComposedTemplate{},
				CompositeTypeRef: v14.TypeReference{
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

func NewTestInstance(instanceID string, plan *crossplane.Plan, serviceName crossplane.ServiceName, serviceID, parent string) *composite.Unstructured {
	gvk, _ := plan.GVK()
	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(instanceID)
	cmp.SetCompositionReference(&v12.ObjectReference{
		Name: plan.Labels.PlanName,
	})
	labels := map[string]string{
		crossplane.PlanNameLabel:    plan.Labels.PlanName,
		crossplane.ServiceIDLabel:   serviceID,
		crossplane.SLALabel:         plan.Labels.SLA,
		crossplane.ServiceNameLabel: string(serviceName),
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
	cmp.SetResourceReferences([]v12.ObjectReference{
		{
			Kind:       "Secret",
			Namespace:  TestNamespace,
			APIVersion: "v1",
			Name:       instanceID,
		},
	})

	return cmp
}

func NewTestSecret(namespace, name string, stringData map[string]string) *v12.Secret {
	return &v12.Secret{
		ObjectMeta: v13.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		StringData: stringData,
	}
}

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

	return cmp
}

func NewTestNamespace(name string) *v12.Namespace {
	return &v12.Namespace{
		ObjectMeta: v13.ObjectMeta{
			Name: name,
		},
	}
}

func CreateObjects(ctx context.Context, objs []runtime.Object) PrePostRunFunc {
	return func(c client.Client) error {
		for _, obj := range objs {
			if err := c.Create(ctx, obj.DeepCopyObject()); err != nil {
				return err
			}
		}
		return nil
	}
}

func RemoveObjects(ctx context.Context, objs []runtime.Object) PrePostRunFunc {
	return func(c client.Client) error {
		for _, obj := range objs {
			if err := c.Delete(ctx, obj); err != nil {
				return err
			}
		}
		return nil
	}
}

func SetupManager(t *testing.T) (*integration.Manager, lager.Logger, *crossplane.Crossplane, error) {
	if db := os.Getenv("DEBUG"); db != "" {
		zl, _ := zap.NewDevelopment()
		log.SetLogger(zapr.NewLogger(zl))
	}

	// unfortunately can't be set via an option on the manager as BinaryAssetsDir is not exposed.
	os.Setenv("KUBEBUILDER_ASSETS", "../../testdata/bin")

	m, err := integration.New(nil,
		integration.WithCRDPaths("../../testdata/crds"),
	)
	if err != nil {
		return nil, nil, nil, err
	}

	m.Run()

	scheme := m.GetScheme()
	assert.NoError(t, v14.AddToScheme(scheme))
	assert.NoError(t, crossplane.Register(scheme))

	logger := lager.NewLogger("test")

	require.NoError(t, CreateObjects(context.Background(), []runtime.Object{NewTestNamespace(TestNamespace)})(m.GetClient()))

	cp, err := crossplane.New([]string{"1", "2"}, TestNamespace, m.GetConfig())
	if err != nil {
		return nil, nil, nil, err
	}
	return m, logger, cp, nil
}

func BoolPtr(b bool) *bool {
	return &b
}
