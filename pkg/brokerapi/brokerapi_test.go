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
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	"github.com/crossplane/crossplane-runtime/pkg/test/integration"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"github.com/go-logr/zapr"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/middlewares"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vshn/crossplane-service-broker/internal/broker"
	"github.com/vshn/crossplane-service-broker/internal/crossplane"
)

type preRunFunc func(c client.Client) error
type customCheckFunc func(t *testing.T, c client.Client)

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
				newServicePlan("1", "1-2", crossplane.RedisService).Composition,
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
							Name:        "small1-1",
							Description: "testservice-small plan description",
							Free:        boolPtr(false),
							Bindable:    boolPtr(false),
							Metadata: &domain.ServicePlanMetadata{
								DisplayName: "small",
							},
						},
						{
							ID:          "1-2",
							Name:        "small1-2",
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
			require.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp)

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
			wantErr: errors.New(`This service plan requires client support for asynchronous service operations. (correlation-id: "corrid")`),
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
					newInstance("1", servicePlan, crossplane.RedisService, "", ""),
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
			require.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp)

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

func TestBrokerAPI_Deprovision(t *testing.T) {
	type fields struct {
		broker *broker.Broker
		logger lager.Logger
	}
	type args struct {
		ctx          context.Context
		instanceID   string
		details      domain.DeprovisionDetails
		asyncAllowed bool
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name          string
		fields        fields
		args          args
		want          *domain.DeprovisionServiceSpec
		wantErr       error
		preRunFn      preRunFunc
		customCheckFn customCheckFunc
	}{
		{
			name: "requires instance to exist before deprovisioning",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				details: domain.DeprovisionDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)
				return createObjects(context.TODO(), []runtime.Object{
					newService("1", crossplane.RedisService),
					servicePlan.Composition,
				})(c)
			},
			customCheckFn: nil,
			want:          nil,
			wantErr:       errors.New(`instance does not exist (correlation-id: "corrid")`),
		},
		{
			name: "removes instance",
			args: args{
				ctx:        ctx,
				instanceID: "1",
				details: domain.DeprovisionDetails{
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
					newInstance("1", servicePlan, crossplane.RedisService, "", ""),
				})(c)
			},
			customCheckFn: func(t *testing.T, c client.Client) {
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)
				_, err := getInstance(ctx, c, servicePlan, "1")
				assert.EqualError(t, err, `compositeredisinstances.syn.tools "1" not found`)
			},
			want:    &domain.DeprovisionServiceSpec{IsAsync: false},
			wantErr: nil,
		},
		{
			name: "prevents removing instance if Deprovisionable returns an error",
			args: args{
				ctx:        ctx,
				instanceID: "1",
				details: domain.DeprovisionDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
				asyncAllowed: true,
			},
			preRunFn: func(c client.Client) error {
				service := newService("1", crossplane.MariaDBService)
				servicePlan := newServicePlan("1", "1-1", crossplane.MariaDBService)
				mdbs := newServicePlan("2", "2-1", crossplane.MariaDBDatabaseService)

				dbInstance := newInstance("2", mdbs, crossplane.MariaDBDatabaseService, "", "1")
				dbInstance.Object["spec"] = map[string]interface{}{
					"parameters": map[string]interface{}{
						"parent_reference": "1",
					},
				}

				return createObjects(context.TODO(), []runtime.Object{
					service,
					servicePlan.Composition,
					newService("2", crossplane.MariaDBDatabaseService),
					mdbs.Composition,
					newInstance("1", servicePlan, crossplane.MariaDBService, "", ""),
					dbInstance,
				})(c)
			},
			customCheckFn: nil,
			want:          nil,
			wantErr:       errors.New(`instance is still in use by "2" (correlation-id: "corrid")`),
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
			require.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp)

			bAPI := BrokerAPI{
				broker: b,
				logger: logger,
			}
			got, err := bAPI.Deprovision(tt.args.ctx, tt.args.instanceID, tt.args.details, tt.args.asyncAllowed)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, *tt.want, got)

			if tt.customCheckFn != nil {
				tt.customCheckFn(t, m.GetClient())
			}
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
					newInstance("1", servicePlan, crossplane.RedisService, "", ""),
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

				instance := newInstance("1", servicePlan, crossplane.RedisService, "", "")
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

				instance := newInstance("1", servicePlan, crossplane.RedisService, "", "")
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

				instance := newInstance("1", servicePlan, crossplane.RedisService, "", "")
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
			require.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp)

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
					newInstance("1-1-1", servicePlan, crossplane.RedisService, "", ""),
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
				instance := newInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")
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
				instance := newInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", "")
				err := createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.MariaDBService),
					servicePlan.Composition,
					instance,
					newSecret(testNamespace, "1-1-1", map[string]string{
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
			wantErr: errors.New(`service MariaDB Galera Cluster is not bindable. You can create a bindable database on this cluster using cf create-service mariadb-k8s-database default my-mariadb-db -c '{"parent_reference": "1-1-1"}' (correlation-id: "corrid")`),
		},
		{
			name: "creates a mariadb instance and tries to bind it without having endpoint in secret",
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
				instance := newInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", "")
				err := createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.MariaDBService),
					newService("2", crossplane.MariaDBDatabaseService),
					servicePlan.Composition,
					instance,
					newSecret(testNamespace, "1-1-1", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
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
			require.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp)

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

func TestBrokerAPI_GetBinding(t *testing.T) {
	type fields struct {
		broker *broker.Broker
		logger lager.Logger
	}
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name     string
		fields   fields
		args     args
		want     *domain.GetBindingSpec
		wantErr  error
		preRunFn preRunFunc
	}{
		{
			name: "requires instance to be ready before getting a binding",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)
				return createObjects(context.TODO(), []runtime.Object{
					newService("1", crossplane.RedisService),
					servicePlan.Composition,
					newInstance("1-1-1", servicePlan, crossplane.RedisService, "", ""),
				})(c)
			},
			want:    nil,
			wantErr: errors.New(`instance is being updated and cannot be retrieved (correlation-id: "corrid")`),
		},
		{
			name: "creates a redis instance and gets the binding",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)
				instance := newInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")
				err := createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.RedisService),
					newServicePlan("1", "1-2", crossplane.RedisService).Composition,
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
			want: &domain.GetBindingSpec{
				Credentials: crossplane.Credentials{
					"password": "supersecret",
				},
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
			require.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp)

			bAPI := BrokerAPI{
				broker: b,
				logger: logger,
			}

			got, err := bAPI.GetBinding(tt.args.ctx, tt.args.instanceID, tt.args.bindingID)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, *tt.want, got)
		})
	}
}

func TestBrokerAPI_GetInstance(t *testing.T) {
	type fields struct {
		broker *broker.Broker
		logger lager.Logger
	}
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name     string
		fields   fields
		args     args
		want     *domain.GetInstanceDetailsSpec
		wantErr  error
		preRunFn preRunFunc
	}{
		{
			name: "gets an instance without parameters",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)
				instance := newInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")

				return createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.RedisService),
					newServicePlan("1", "1-2", crossplane.RedisService).Composition,
					servicePlan.Composition,
					instance,
				})(c)
			},
			want: &domain.GetInstanceDetailsSpec{
				PlanID:     "1-1",
				ServiceID:  "1",
				Parameters: nil,
			},
			wantErr: nil,
		},

		{
			name: "gets an instance with parameters",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.MariaDBDatabaseService)
				instance := newInstance("1-1-1", servicePlan, crossplane.MariaDBDatabaseService, "", "")
				_ = fieldpath.Pave(instance.Object).SetValue("spec.parameters.parent_reference", "1")

				return createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.MariaDBDatabaseService),
					newServicePlan("1", "1-2", crossplane.MariaDBDatabaseService).Composition,
					servicePlan.Composition,
					instance,
				})(c)
			},
			want: &domain.GetInstanceDetailsSpec{
				PlanID:    "1-1",
				ServiceID: "1",
				Parameters: map[string]interface{}{
					"parent_reference": "1",
				},
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
			require.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp)

			bAPI := BrokerAPI{
				broker: b,
				logger: logger,
			}

			got, err := bAPI.GetInstance(tt.args.ctx, tt.args.instanceID)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, *tt.want, got)
		})
	}
}

func TestBrokerAPI_Update(t *testing.T) {
	type fields struct {
		broker *broker.Broker
		logger lager.Logger
	}
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		details    domain.UpdateDetails
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name     string
		fields   fields
		args     args
		want     *domain.UpdateServiceSpec
		wantErr  error
		preRunFn preRunFunc
	}{
		{
			name: "service update not permitted",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.UpdateDetails{
					ServiceID: "2",
					PlanID:    "2-1",
					PreviousValues: domain.PreviousValues{
						PlanID: "1-1",
					},
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)
				instance := newInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.RedisService),
					newService("2", crossplane.RedisService),
					newServicePlan("1", "1-2", crossplane.RedisService).Composition,
					newServicePlan("2", "2-1", crossplane.RedisService).Composition,
					servicePlan.Composition,
					instance,
				})(c)
			},
			want:    nil,
			wantErr: errors.New(`service update not permitted (correlation-id: "corrid")`),
		},
		{
			name: "plan size change not permitted",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.UpdateDetails{
					ServiceID: "1",
					PlanID:    "1-2",
					PreviousValues: domain.PreviousValues{
						PlanID: "1-1",
					},
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlanWithSize("1", "1-1", crossplane.RedisService, "small", "standard")
				instance := newInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.RedisService),
					newService("2", crossplane.RedisService),
					newServicePlanWithSize("1", "1-2", crossplane.RedisService, "large", "standard").Composition,
					servicePlan.Composition,
					instance,
				})(c)
			},
			want:    nil,
			wantErr: errors.New(`plan change not permitted (correlation-id: "corrid")`),
		},
		{
			name: "upgrade standard -> premium possible",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.UpdateDetails{
					ServiceID: "1",
					PlanID:    "1-2",
					PreviousValues: domain.PreviousValues{
						PlanID: "1-1",
					},
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlanWithSize("1", "1-1", crossplane.RedisService, "small", "standard")
				instance := newInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.RedisService),
					newService("2", crossplane.RedisService),
					newServicePlanWithSize("1", "1-2", crossplane.RedisService, "small-premium", "premium").Composition,
					servicePlan.Composition,
					instance,
				})(c)
			},
			want:    &domain.UpdateServiceSpec{},
			wantErr: nil,
		},
		{
			name: "upgrade standard -> premium possible (also works without PreviousValues)",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.UpdateDetails{
					ServiceID: "1",
					PlanID:    "1-2",
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlanWithSize("1", "1-1", crossplane.RedisService, "small", "standard")
				instance := newInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.RedisService),
					newService("2", crossplane.RedisService),
					newServicePlanWithSize("1", "1-2", crossplane.RedisService, "small-premium", "premium").Composition,
					servicePlan.Composition,
					instance,
				})(c)
			},
			want:    &domain.UpdateServiceSpec{},
			wantErr: nil,
		},
		{
			name: "downgrade premium -> standard possible",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.UpdateDetails{
					ServiceID: "1",
					PlanID:    "1-2",
					PreviousValues: domain.PreviousValues{
						PlanID: "1-1",
					},
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlanWithSize("1", "1-1", crossplane.RedisService, "small-premium", "premium")
				instance := newInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.RedisService),
					newService("2", crossplane.RedisService),
					newServicePlanWithSize("1", "1-2", crossplane.RedisService, "small", "standard").Composition,
					servicePlan.Composition,
					instance,
				})(c)
			},
			want:    &domain.UpdateServiceSpec{},
			wantErr: nil,
		},
		{
			name: "upgrade super-large-standard -> super-large-premium possible",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.UpdateDetails{
					ServiceID: "1",
					PlanID:    "1-2",
					PreviousValues: domain.PreviousValues{
						PlanID: "1-1",
					},
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlanWithSize("1", "1-1", crossplane.RedisService, "super-large", "standard")
				instance := newInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.RedisService),
					newService("2", crossplane.RedisService),
					newServicePlanWithSize("1", "1-2", crossplane.RedisService, "super-large-premium", "premium").Composition,
					servicePlan.Composition,
					instance,
				})(c)
			},
			want:    &domain.UpdateServiceSpec{},
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
			require.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp)

			bAPI := BrokerAPI{
				broker: b,
				logger: logger,
			}

			got, err := bAPI.Update(tt.args.ctx, tt.args.instanceID, tt.args.details, false)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, *tt.want, got)
		})
	}
}

func TestBrokerAPI_Unbind(t *testing.T) {
	type fields struct {
		broker *broker.Broker
		logger lager.Logger
	}
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		details    domain.UnbindDetails
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name     string
		fields   fields
		args     args
		want     *domain.UnbindSpec
		wantErr  error
		preRunFn preRunFunc
	}{
		{
			name: "requires instance to be ready before unbinding",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.UnbindDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.RedisService)
				return createObjects(context.TODO(), []runtime.Object{
					newService("1", crossplane.RedisService),
					servicePlan.Composition,
					newInstance("1-1-1", servicePlan, crossplane.RedisService, "", ""),
				})(c)
			},
			want:    nil,
			wantErr: errors.New(`instance is being updated and cannot be retrieved (correlation-id: "corrid")`),
		},
		{
			name: "removes a MariaDB user instance",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "binding-1",
				details: domain.UnbindDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			preRunFn: func(c client.Client) error {
				servicePlan := newServicePlan("1", "1-1", crossplane.MariaDBDatabaseService)
				instance := newInstance("1-1-1", servicePlan, crossplane.MariaDBDatabaseService, "", "")
				instance.Object["spec"] = map[string]interface{}{
					"parameters": map[string]interface{}{
						"parent_reference": "1",
					},
				}
				err := createObjects(context.TODO(), []runtime.Object{
					newNamespace(testNamespace),
					newService("1", crossplane.MariaDBDatabaseService),
					servicePlan.Composition,
					instance,
					newMariaDBUserInstance("1-1-1", "binding-1"),
					newSecret(testNamespace, "binding-1-password", map[string]string{
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
					}),
				})(c)
				if err != nil {
					return err
				}

				return updateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
			},
			want: &domain.UnbindSpec{
				IsAsync: false,
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
			require.NoError(t, tt.preRunFn(m.GetClient()))

			b := broker.New(cp)

			bAPI := BrokerAPI{
				broker: b,
				logger: logger,
			}

			got, err := bAPI.Unbind(tt.args.ctx, tt.args.instanceID, tt.args.bindingID, tt.args.details, false)
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
	cmp, err := getInstance(ctx, c, servicePlan, instance.GetName())
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

func getInstance(ctx context.Context, c client.Client, servicePlan *crossplane.Plan, instanceID string) (*composite.Unstructured, error) {
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
					Name: "v1alpha1",
				},
			},
		},
	}
}

func newServicePlan(serviceID string, planID string, serviceName crossplane.Service) *crossplane.Plan {
	name := "small" + planID
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
					Kind:       kindForService(serviceName),
				},
			},
		},
	}
}

func newServicePlanWithSize(serviceID string, planID string, serviceName crossplane.Service, name string, sla string) *crossplane.Plan {
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

func kindForService(name crossplane.Service) string {
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

func newInstance(instanceID string, plan *crossplane.Plan, serviceName crossplane.Service, serviceID, parent string) *composite.Unstructured {
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

func newMariaDBUserInstance(instanceID, bindingID string) *composite.Unstructured {
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

	cp, err := crossplane.New([]string{"1", "2"}, testNamespace, m.GetConfig())
	if err != nil {
		return nil, nil, nil, err
	}
	return m, logger, cp, nil
}

func boolPtr(b bool) *bool {
	return &b
}
