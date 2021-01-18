// +build integration

package brokerapi

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"code.cloudfoundry.org/lager"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	"github.com/crossplane/crossplane-runtime/pkg/test/integration"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	xv1beta1 "github.com/crossplane/crossplane/apis/apiextensions/v1beta1"
	"github.com/go-logr/zapr"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	"github.com/pivotal-cf/brokerapi/v7/middlewares"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vshn/crossplane-service-broker/internal/broker"
	"github.com/vshn/crossplane-service-broker/internal/crossplane"
)

type preRunFunc func(c client.Client) error

func TestBrokerAPI_Services(t *testing.T) {
	type fields struct {
		broker *broker.Broker
		logger lager.Logger
	}
	tests := []struct {
		name     string
		fields   fields
		ctx      context.Context
		want     []domain.Service
		wantErr  bool
		preRunFn preRunFunc
	}{
		{
			name: "returns the catalog successfully",
			ctx:  context.TODO(),
			preRunFn: createObjects(context.TODO(), []runtime.Object{
				newService("1"),
				newServicePlan("1", "1-1").Composition,
			}),
			want: []domain.Service{
				{
					ID:                   "1",
					Name:                 "testservice",
					Description:          "testservice description",
					Bindable:             true,
					InstancesRetrievable: true,
					BindingsRetrievable:  true,
					PlanUpdatable:        false,
					Plans: []domain.ServicePlan{
						{
							ID:          "1-1",
							Name:        "small",
							Description: "testservice-small plan description",
							Free:        boolPtr(false),
							Bindable:    boolPtr(false),
							Metadata: &domain.ServicePlanMetadata{
								DisplayName: "small",
							},
						},
					},
					Metadata: &domain.ServiceMetadata{
						DisplayName: "testservice",
					},
					Tags: []string{"foo", "bar", "baz"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, logger, cp, err := setupManager(t)
			if err != nil {
				assert.FailNow(t, fmt.Sprintf("unable to setup integration test manager: %s", err))
				return
			}
			defer m.Cleanup()
			assert.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp, logger)

			bAPI := BrokerAPI{
				broker: b,
				logger: logger,
			}
			got, err := bAPI.Services(tt.ctx)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.Equal(t, tt.want, got)

		})
	}
}

func TestBrokerAPI_Provision(t *testing.T) {
	type fields struct {
		broker *broker.Broker
		logger lager.Logger
	}
	type args struct {
		ctx          context.Context
		instanceID   string
		details      domain.ProvisionDetails
		asyncAllowed bool
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name     string
		fields   fields
		args     args
		want     *domain.ProvisionedServiceSpec
		wantErr  error
		preRunFn preRunFunc
	}{
		{
			name: "requires async",
			args: args{
				ctx:          ctx,
				instanceID:   "1",
				details:      domain.ProvisionDetails{},
				asyncAllowed: false,
			},
			preRunFn: func(c client.Client) error {
				return nil
			},
			want:    nil,
			wantErr: apiresponses.ErrAsyncRequired,
		},
		{
			name: "specified plan must exist",
			args: args{
				ctx:        ctx,
				instanceID: "1",
				details: domain.ProvisionDetails{
					PlanID: "1-1",
				},
				asyncAllowed: true,
			},
			preRunFn: func(c client.Client) error {
				return nil
			},
			want:    nil,
			wantErr: errors.New(`compositions.apiextensions.crossplane.io "1-1" not found (correlation-id: "corrid")`),
		},
		{
			name: "creates an instance",
			args: args{
				ctx:        ctx,
				instanceID: "1",
				details: domain.ProvisionDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
				asyncAllowed: true,
			},
			preRunFn: createObjects(context.TODO(), []runtime.Object{
				newService("1"),
				newServicePlan("1", "1-1").Composition,
			}),
			want:    &domain.ProvisionedServiceSpec{IsAsync: true},
			wantErr: nil,
		},
		{
			name: "returns already exists if instance already exists",
			args: args{
				ctx:        ctx,
				instanceID: "1",
				details: domain.ProvisionDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
				asyncAllowed: true,
			},
			preRunFn: func(c client.Client) error {
				service := newService("1")
				servicePlan := newServicePlan("1", "1-1")

				return createObjects(context.TODO(), []runtime.Object{
					service,
					servicePlan.Composition,
					newInstance("1", servicePlan),
				})(c)
			},
			want:    &domain.ProvisionedServiceSpec{AlreadyExists: true},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, logger, cp, err := setupManager(t)
			if err != nil {
				assert.FailNow(t, fmt.Sprintf("unable to setup integration test manager: %s", err))
				return
			}
			defer m.Cleanup()
			assert.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp, logger)

			bAPI := BrokerAPI{
				broker: b,
				logger: logger,
			}
			got, err := bAPI.Provision(tt.args.ctx, tt.args.instanceID, tt.args.details, tt.args.asyncAllowed)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}

			assert.Nil(t, err)
			assert.Equal(t, *tt.want, got)
		})
	}
}

func newService(serviceID string) *xv1.CompositeResourceDefinition {
	return &xv1.CompositeResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			Kind:       "service",
			APIVersion: "syn.tools/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "testservice",
			Labels: map[string]string{
				crossplane.ServiceIDLabel:   serviceID,
				crossplane.ServiceNameLabel: "testservice",
				crossplane.BindableLabel:    "true",
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
					Name: "v1",
				},
			},
		},
	}
}

func newServicePlan(serviceID string, planID string) *crossplane.Plan {
	name := "small"
	return &crossplane.Plan{
		Labels: &crossplane.Labels{
			PlanName: name,
		},
		Composition: &xv1beta1.Composition{
			TypeMeta: metav1.TypeMeta{
				Kind:       "servicePlan",
				APIVersion: "syn.tools/v1alpha1",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name: planID,
				Labels: map[string]string{
					crossplane.ServiceIDLabel:   serviceID,
					crossplane.ServiceNameLabel: "testservice",
					crossplane.PlanNameLabel:    name,
					crossplane.BindableLabel:    "false",
				},
				Annotations: map[string]string{
					crossplane.DescriptionAnnotation: "testservice-small plan description",
					crossplane.MetadataAnnotation:    `{"displayName": "small"}`,
				},
			},
			Spec: xv1beta1.CompositionSpec{
				Resources: []xv1beta1.ComposedTemplate{},
				CompositeTypeRef: xv1beta1.TypeReference{
					APIVersion: "syn.tools/v1alpha1",
					Kind:       "CompositeInstance",
				},
			},
		},
	}
}

func newInstance(instanceID string, plan *crossplane.Plan) *composite.Unstructured {
	gvk, _ := plan.GVK()
	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(instanceID)
	cmp.SetCompositionReference(&corev1.ObjectReference{
		Name: plan.Labels.PlanName,
	})
	cmp.SetLabels(map[string]string{
		crossplane.PlanNameLabel: plan.Labels.PlanName,
	})
	return cmp
}

func createObjects(ctx context.Context, objs []runtime.Object) preRunFunc {
	return func(c client.Client) error {
		for _, obj := range objs {
			if err := c.Create(ctx, obj); err != nil {
				return err
			}
		}
		return nil
	}
}

func setupManager(t *testing.T) (*integration.Manager, lager.Logger, *crossplane.Crossplane, error) {
	if db := os.Getenv("DEBUG"); db != "" {
		zl, _ := zap.NewDevelopment()
		log.SetLogger(zapr.NewLogger(zl))
	}

	m, err := integration.New(nil,
		integration.WithCRDPaths("../../testdata/crds"),
	)
	if err != nil {
		return nil, nil, nil, err
	}

	m.Run()

	scheme := m.GetScheme()
	assert.NoError(t, xv1.AddToScheme(scheme))
	assert.NoError(t, xv1beta1.AddToScheme(scheme))
	assert.NoError(t, crossplane.Register(scheme))

	logger := lager.NewLogger("test")

	cp, err := crossplane.New([]string{"1", "2"}, m.GetConfig(), logger)
	if err != nil {
		return nil, nil, nil, err
	}
	return m, logger, cp, nil
}

func boolPtr(b bool) *bool {
	return &b
}
