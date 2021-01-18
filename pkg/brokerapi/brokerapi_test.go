// +build integration

package brokerapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	"github.com/crossplane/crossplane-runtime/pkg/test/integration"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"github.com/go-logr/zapr"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	"github.com/pivotal-cf/brokerapi/v7/middlewares"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vshn/crossplane-service-broker/internal/broker"
	"github.com/vshn/crossplane-service-broker/internal/crossplane"
)

type preRunFunc func(c client.Client) error

const testNamespace = "test"

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
				newService("1", crossplane.RedisService),
				newServicePlan("1", "1-1", crossplane.RedisService).Composition,
			}),
			want: []domain.Service{
				{
					ID:                   "1",
					Name:                 string(crossplane.RedisService),
					Description:          "testservice description",
					Bindable:             true,
					InstancesRetrievable: true,
					BindingsRetrievable:  true,
					PlanUpdatable:        true,
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

			assert.NoError(t, err)
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
			name: "creates a redis instance",
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
				newService("1", crossplane.RedisService),
				newServicePlan("1", "1-1", crossplane.RedisService).Composition,
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
				service := newService("1", crossplane.RedisService)
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)

				return createObjects(context.TODO(), []runtime.Object{
					service,
					servicePlan.Composition,
					newInstance("1", servicePlan, crossplane.RedisService),
				})(c)
			},
			want:    &domain.ProvisionedServiceSpec{AlreadyExists: true},
			wantErr: nil,
		},
		{
			name: "creates a mariadb instance",
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
				newService("1", crossplane.MariaDBService),
				newServicePlan("1", "1-1", crossplane.MariaDBService).Composition,
			}),
			want:    &domain.ProvisionedServiceSpec{IsAsync: true},
			wantErr: nil,
		},
		{
			name: "creates a mariadb database instance",
			args: args{
				ctx:        ctx,
				instanceID: "2",
				details: domain.ProvisionDetails{
					PlanID:        "2-1",
					ServiceID:     "2",
					RawParameters: json.RawMessage(`{"parent_reference": "1"}`),
				},
				asyncAllowed: true,
			},
			preRunFn: createObjects(context.TODO(), []runtime.Object{
				newService("1", crossplane.MariaDBService),
				newServicePlan("1", "1-1", crossplane.MariaDBService).Composition,
				newService("2", crossplane.MariaDBDatabaseService),
				newServicePlan("2", "2-1", crossplane.MariaDBDatabaseService).Composition,
			}),
			want:    &domain.ProvisionedServiceSpec{IsAsync: true},
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

			assert.NoError(t, err)
			assert.Equal(t, *tt.want, got)
		})
	}
}

func TestBrokerAPI_LastOperation(t *testing.T) {
	type fields struct {
		broker *broker.Broker
		logger lager.Logger
	}
	type args struct {
		ctx        context.Context
		instanceID string
		details    domain.PollDetails
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name     string
		fields   fields
		args     args
		want     *domain.LastOperation
		wantErr  error
		preRunFn preRunFunc
	}{
		{
			name: "returns in progress state on unknown condition",
			args: args{
				ctx:        ctx,
				instanceID: "1",
				details: domain.PollDetails{
					PlanID: "1-1",
				},
			},
			preRunFn: func(c client.Client) error {
				service := newService("1", crossplane.RedisService)
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)

				return createObjects(context.TODO(), []runtime.Object{
					service,
					servicePlan.Composition,
					newInstance("1", servicePlan, crossplane.RedisService),
				})(c)
			},
			want: &domain.LastOperation{
				Description: "Unknown",
				State:       domain.InProgress,
			},
			wantErr: nil,
		},
		{
			name: "returns in progress on creating",
			args: args{
				ctx:        ctx,
				instanceID: "1",
				details: domain.PollDetails{
					PlanID: "1-1",
				},
			},
			preRunFn: func(c client.Client) error {
				service := newService("1", crossplane.RedisService)
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)

				instance := newInstance("1", servicePlan, crossplane.RedisService)
				err := createObjects(context.TODO(), []runtime.Object{
					service,
					servicePlan.Composition,
					instance,
				})(c)
				if err != nil {
					return err
				}

				return updateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonCreating)
			},
			want: &domain.LastOperation{
				Description: string(xrv1.ReasonCreating),
				State:       domain.InProgress,
			},
			wantErr: nil,
		},
		{
			name: "returns succeeded on available",
			args: args{
				ctx:        ctx,
				instanceID: "1",
				details: domain.PollDetails{
					PlanID: "1-1",
				},
			},
			preRunFn: func(c client.Client) error {
				service := newService("1", crossplane.RedisService)
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)

				instance := newInstance("1", servicePlan, crossplane.RedisService)
				err := createObjects(context.TODO(), []runtime.Object{
					service,
					servicePlan.Composition,
					instance,
				})(c)
				if err != nil {
					return err
				}

				return updateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
			},
			want: &domain.LastOperation{
				Description: string(xrv1.ReasonAvailable),
				State:       domain.Succeeded,
			},
			wantErr: nil,
		},
		{
			name: "returns failed on unavailable",
			args: args{
				ctx:        ctx,
				instanceID: "1",
				details: domain.PollDetails{
					PlanID: "1-1",
				},
			},
			preRunFn: func(c client.Client) error {
				service := newService("1", crossplane.RedisService)
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)

				instance := newInstance("1", servicePlan, crossplane.RedisService)
				err := createObjects(context.TODO(), []runtime.Object{
					service,
					servicePlan.Composition,
					instance,
				})(c)
				if err != nil {
					return err
				}

				return updateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonUnavailable)
			},
			want: &domain.LastOperation{
				Description: string(xrv1.ReasonUnavailable),
				State:       domain.Failed,
			},
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
			got, err := bAPI.LastOperation(tt.args.ctx, tt.args.instanceID, tt.args.details)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, *tt.want, got)
		})
	}
}

func TestBrokerAPI_Bind(t *testing.T) {
	type fields struct {
		broker *broker.Broker
		logger lager.Logger
	}
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		details    domain.BindDetails
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name     string
		fields   fields
		args     args
		want     *domain.Binding
		wantErr  error
		preRunFn preRunFunc
	}{
		{
			name: "requires instance to be ready before binding",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.BindDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)
				return createObjects(context.TODO(), []runtime.Object{
					newService("1", crossplane.RedisService),
					servicePlan.Composition,
					newInstance("1-1-1", servicePlan, crossplane.RedisService),
				})(c)
			},
			want:    nil,
			wantErr: errors.New(`instance is being updated and cannot be retrieved (correlation-id: "corrid")`),
		},
		{
			name: "creates a redis instance and binds it",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.BindDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)
				instance := newInstance("1-1-1", servicePlan, crossplane.RedisService)
				err := createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.RedisService),
					servicePlan.Composition,
					instance,
					newSecret(testNamespace, "creds", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
						xrv1.ResourceCredentialsSecretEndpointKey: "localhost",
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
					}),
				})(c)
				if err != nil {
					return err
				}

				return updateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
			},
			want: &domain.Binding{
				IsAsync: false,
				Credentials: crossplane.Credentials{
					"password": "supersecret",
				},
			},
			wantErr: nil,
		},
		{
			name: "creates a mariadb instance and tries to bind it",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.BindDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.MariaDBService)
				instance := newInstance("1-1-1", servicePlan, crossplane.MariaDBService)
				err := createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.MariaDBService),
					servicePlan.Composition,
					instance,
					newSecret(testNamespace, "creds", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
						xrv1.ResourceCredentialsSecretEndpointKey: "localhost",
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
					}),
				})(c)
				if err != nil {
					return err
				}

				return updateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
			},
			want:    nil,
			wantErr: errors.New(`FinishProvision deactivated until proper solution in place. Retrieving Endpoint needs implementation. (correlation-id: "corrid")`),
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

			got, err := bAPI.Bind(tt.args.ctx, tt.args.instanceID, tt.args.bindingID, tt.args.details, false)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, *tt.want, got)
		})
	}
}

func updateInstanceConditions(ctx context.Context, c client.Client, servicePlan *crossplane.Plan, instance *composite.Unstructured, t xrv1.ConditionType, status corev1.ConditionStatus, reason xrv1.ConditionReason) error {
	gvk, err := servicePlan.GVK()
	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(instance.GetName())

	err = c.Get(ctx, types.NamespacedName{Name: instance.GetName()}, cmp)
	if err != nil {
		return err
	}

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

func newService(serviceID string, serviceName crossplane.Service) *xv1.CompositeResourceDefinition {
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
					Name: "xrv1",
				},
			},
		},
	}
}

func newServicePlan(serviceID string, planID string, serviceName crossplane.Service) *crossplane.Plan {
	name := "small"
	return &crossplane.Plan{
		Labels: &crossplane.Labels{
			PlanName: name,
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
					Kind:       "CompositeInstance",
				},
			},
		},
	}
}

func newInstance(instanceID string, plan *crossplane.Plan, serviceName crossplane.Service) *composite.Unstructured {
	gvk, _ := plan.GVK()
	cmp := composite.New(composite.WithGroupVersionKind(gvk))
	cmp.SetName(instanceID)
	cmp.SetCompositionReference(&corev1.ObjectReference{
		Name: plan.Labels.PlanName,
	})
	cmp.SetLabels(map[string]string{
		crossplane.PlanNameLabel:    plan.Labels.PlanName,
		crossplane.ServiceNameLabel: string(serviceName),
	})
	cmp.SetResourceReferences([]corev1.ObjectReference{
		{
			Kind:       "Secret",
			Namespace:  testNamespace,
			APIVersion: "v1",
			Name:       "creds",
		},
	})

	return cmp
}

func newSecret(namespace, name string, stringData map[string]string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		StringData: stringData,
	}
}

func newNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
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
	assert.NoError(t, crossplane.Register(scheme))

	logger := lager.NewLogger("test")

	cp, err := crossplane.New([]string{"1", "2"}, testNamespace, m.GetConfig(), logger)
	if err != nil {
		return nil, nil, nil, err
	}
	return m, logger, cp, nil
}

func boolPtr(b bool) *bool {
	return &b
}
