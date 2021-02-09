// +build integration

package brokerapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/middlewares"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vshn/crossplane-service-broker/pkg/crossplane"
	"github.com/vshn/crossplane-service-broker/pkg/integration"
)

func TestBrokerAPI_Services(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		want      []domain.Service
		wantErr   bool
		resources []runtime.Object
	}{
		{
			name: "returns the catalog successfully",
			ctx:  context.TODO(),
			resources: []runtime.Object{
				integration.NewTestService("1", crossplane.RedisService),
				integration.NewTestServicePlan("1", "1-1", crossplane.RedisService).Composition,
				integration.NewTestServicePlan("1", "1-2", crossplane.RedisService).Composition,
			},
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
							Free:        integration.BoolPtr(false),
							Bindable:    integration.BoolPtr(false),
							Metadata: &domain.ServicePlanMetadata{
								DisplayName: "small",
							},
						},
						{
							ID:          "1-2",
							Name:        "small1-2",
							Description: "testservice-small plan description",
							Free:        integration.BoolPtr(false),
							Bindable:    integration.BoolPtr(false),
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

	m, logger, cp, err := integration.SetupManager(t)
	require.NoError(t, err, "unable to setup integration test manager")
	defer m.Cleanup()

	bAPI, err := New(cp, logger)
	require.NoError(t, err, "unable to setup brokerapi")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, integration.CreateObjects(tt.ctx, tt.resources)(m.GetClient()))
			defer func() {
				require.NoError(t, integration.RemoveObjects(tt.ctx, tt.resources)(m.GetClient()))
			}()

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
	type args struct {
		ctx          context.Context
		instanceID   string
		details      domain.ProvisionDetails
		asyncAllowed bool
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.ProvisionedServiceSpec
		wantErr   error
		resources func() []runtime.Object
	}{
		{
			name: "requires async",
			args: args{
				ctx:          ctx,
				instanceID:   "1",
				details:      domain.ProvisionDetails{},
				asyncAllowed: false,
			},
			want: nil,
			resources: func() []runtime.Object {
				return nil
			},
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
			want: nil,
			resources: func() []runtime.Object {
				return nil
			},
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
			resources: func() []runtime.Object {
				return []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					integration.NewTestServicePlan("1", "1-1", crossplane.RedisService).Composition,
				}
			},
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
			resources: func() []runtime.Object {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				return []runtime.Object{
					service,
					servicePlan.Composition,
					integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", ""),
				}
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
			resources: func() []runtime.Object {
				return []runtime.Object{
					integration.NewTestService("1", crossplane.MariaDBService),
					integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService).Composition,
				}
			},
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
			resources: func() []runtime.Object {
				return []runtime.Object{
					integration.NewTestService("1", crossplane.MariaDBService),
					integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService).Composition,
					integration.NewTestService("2", crossplane.MariaDBDatabaseService),
					integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService).Composition,
				}
			},
			want:    &domain.ProvisionedServiceSpec{IsAsync: true},
			wantErr: nil,
		},
	}
	m, logger, cp, err := integration.SetupManager(t)
	require.NoError(t, err, "unable to setup integration test manager")
	defer m.Cleanup()

	bAPI, err := New(cp, logger)
	require.NoError(t, err, "unable to setup brokerapi")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NoError(t, integration.CreateObjects(tt.args.ctx, tt.resources())(m.GetClient()))
			defer func() {
				require.NoError(t, integration.RemoveObjects(tt.args.ctx, tt.resources())(m.GetClient()))
			}()

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
	type args struct {
		ctx          context.Context
		instanceID   string
		details      domain.DeprovisionDetails
		asyncAllowed bool
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name          string
		args          args
		want          *domain.DeprovisionServiceSpec
		wantErr       error
		resources     func() []runtime.Object
		customCheckFn integration.CustomCheckFunc
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
			resources: func() []runtime.Object {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				return []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					servicePlan.Composition,
				}
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
			resources: func() []runtime.Object {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				return []runtime.Object{
					service,
					servicePlan.Composition,
					integration.NewTestInstance("1", servicePlan, crossplane.RedisService, "", ""),
				}
			},
			customCheckFn: func(t *testing.T, c client.Client) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				_, err := integration.GetInstance(ctx, c, servicePlan, "1")
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
			resources: func() []runtime.Object {
				service := integration.NewTestService("1", crossplane.MariaDBService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				mdbs := integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService)

				dbInstance := integration.NewTestInstance("2", mdbs, crossplane.MariaDBDatabaseService, "", "1")

				return []runtime.Object{
					service,
					servicePlan.Composition,
					integration.NewTestService("2", crossplane.MariaDBDatabaseService),
					mdbs.Composition,
					integration.NewTestInstance("1", servicePlan, crossplane.MariaDBService, "", ""),
					dbInstance,
				}
			},
			want:    nil,
			wantErr: errors.New(`instance is still in use by "2" (correlation-id: "corrid")`),
		},
	}
	m, logger, cp, err := integration.SetupManager(t)
	require.NoError(t, err, "unable to setup integration test manager")
	defer m.Cleanup()

	bAPI, err := New(cp, logger)
	require.NoError(t, err, "unable to setup brokerapi")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := tt.resources()
			require.NoError(t, integration.CreateObjects(tt.args.ctx, objs)(m.GetClient()))
			defer func() {
				// if wantErr == nil, the instance is gone already and would error when trying to remove it again.
				if tt.wantErr == nil {
					objs = objs[:len(objs)-1]
				}
				require.NoError(t, integration.RemoveObjects(tt.args.ctx, objs)(m.GetClient()))
			}()

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
	type args struct {
		ctx        context.Context
		instanceID string
		details    domain.PollDetails
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.LastOperation
		wantErr   error
		resources func() (func(c client.Client) error, []runtime.Object)
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				return nil, []runtime.Object{
					service,
					servicePlan.Composition,
					integration.NewTestInstance("1", servicePlan, crossplane.RedisService, "", ""),
				}
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				instance := integration.NewTestInstance("1", servicePlan, crossplane.RedisService, "", "")
				objs := []runtime.Object{
					service,
					servicePlan.Composition,
					instance,
				}
				return func(c client.Client) error {
					return integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonCreating)
				}, objs
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				instance := integration.NewTestInstance("1", servicePlan, crossplane.RedisService, "", "")
				objs := []runtime.Object{
					service,
					servicePlan.Composition,
					instance,
				}
				return func(c client.Client) error {
					return integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
				}, objs
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				instance := integration.NewTestInstance("1", servicePlan, crossplane.RedisService, "", "")
				objs := []runtime.Object{
					service,
					servicePlan.Composition,
					instance,
				}
				return func(c client.Client) error {
					return integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonUnavailable)
				}, objs
			},
			want: &domain.LastOperation{
				Description: string(xrv1.ReasonUnavailable),
				State:       domain.Failed,
			},
			wantErr: nil,
		},
	}
	m, logger, cp, err := integration.SetupManager(t)
	require.NoError(t, err, "unable to setup integration test manager")
	defer m.Cleanup()

	bAPI, err := New(cp, logger)
	require.NoError(t, err, "unable to setup brokerapi")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, objs := tt.resources()
			require.NoError(t, integration.CreateObjects(tt.args.ctx, objs)(m.GetClient()))
			if fn != nil {
				require.NoError(t, fn(m.GetClient()))
			}

			defer func() {
				require.NoError(t, integration.RemoveObjects(tt.args.ctx, objs)(m.GetClient()))
			}()

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
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		details    domain.BindDetails
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name               string
		args               args
		want               *domain.Binding
		wantComparisonFunc assert.ComparisonAssertionFunc
		wantErr            error
		resources          func() (func(c client.Client) error, []runtime.Object)
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				return nil, []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					servicePlan.Composition,
					integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", ""),
				}
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")
				objs := []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					servicePlan.Composition,
					instance,
					integration.NewTestSecret(integration.TestNamespace, "1-1-1", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
						xrv1.ResourceCredentialsSecretEndpointKey: "localhost",
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
						"sentinelPort": "21234",
					}),
				}
				return func(c client.Client) error {
					return integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
				}, objs
			},
			want: &domain.Binding{
				IsAsync: false,
				Credentials: crossplane.Credentials{
					"host":     "localhost",
					"master":   "redis://1-1-1",
					"password": "supersecret",
					"port":     1234,
					"sentinels": []crossplane.Credentials{
						{
							"host": "localhost",
							"port": 21234,
						},
					},
					"servers": []crossplane.Credentials{
						{
							"host": "localhost",
							"port": 1234,
						},
					},
				},
			},
			wantComparisonFunc: assert.Equal,
			wantErr:            nil,
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", "")
				objs := []runtime.Object{
					integration.NewTestService("1", crossplane.MariaDBService),
					servicePlan.Composition,
					instance,
					integration.NewTestSecret(integration.TestNamespace, "1-1-1", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
						xrv1.ResourceCredentialsSecretEndpointKey: "localhost",
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
					}),
				}
				return func(c client.Client) error {
					return integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
				}, objs
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", "")
				objs := []runtime.Object{
					integration.NewTestService("1", crossplane.MariaDBService),
					servicePlan.Composition,
					instance,
					integration.NewTestSecret(integration.TestNamespace, "1-1-1", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
					}),
				}
				return func(c client.Client) error {
					return integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
				}, objs
			},
			want:    nil,
			wantErr: errors.New(`FinishProvision deactivated until proper solution in place. Retrieving Endpoint needs implementation. (correlation-id: "corrid")`),
		},
		{
			name: "creates a mariadb instance and binds a database instance to it",
			args: args{
				ctx:        ctx,
				instanceID: "1-2-1",
				bindingID:  "2",
				details: domain.BindDetails{
					PlanID:    "2-1",
					ServiceID: "2",
				},
			},
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", "")
				dbServicePlan := integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService)
				dbInstance := integration.NewTestInstance("1-2-1", dbServicePlan, crossplane.MariaDBDatabaseService, "", "1-1-1")
				objs := []runtime.Object{
					integration.NewTestService("1", crossplane.MariaDBService),
					integration.NewTestService("2", crossplane.MariaDBDatabaseService),
					servicePlan.Composition,
					instance,
					dbServicePlan.Composition,
					dbInstance,
					integration.NewTestSecret(integration.TestNamespace, "1-1-1", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
						xrv1.ResourceCredentialsSecretEndpointKey: "localhost",
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
					}),
				}
				return func(c client.Client) error {
					if err := integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable); err != nil {
						return err
					}
					return integration.UpdateInstanceConditions(ctx, c, dbServicePlan, dbInstance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
				}, objs
			},
			want: &domain.Binding{
				IsAsync: false,
				Credentials: crossplane.Credentials{
					"host":         "localhost",
					"hostname":     "localhost",
					"port":         int32(1234),
					"name":         "1-2-1",
					"database":     "1-2-1",
					"user":         nil,
					"password":     "***",
					"database_uri": "***",
					"uri":          "***",
					"jdbcUrl":      "***",
				},
			},
			wantComparisonFunc: func(t assert.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
				want := expected.(domain.Binding)
				got := actual.(domain.Binding)

				assert.Equal(t, want.IsAsync, got.IsAsync)

				wantCreds := want.Credentials.(crossplane.Credentials)
				gotCreds := got.Credentials.(crossplane.Credentials)

				assert.Equal(t, len(wantCreds), len(gotCreds))
				for k, v := range wantCreds {
					if v == "***" {
						assert.Contains(t, gotCreds, k, k)
					} else {
						assert.Equal(t, v, gotCreds[k], k)
					}
				}
				return true

			},
			wantErr: nil,
		},
	}

	m, logger, cp, err := integration.SetupManager(t)
	require.NoError(t, err, "unable to setup integration test manager")
	defer m.Cleanup()

	bAPI, err := New(cp, logger)
	require.NoError(t, err, "unable to setup brokerapi")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, objs := tt.resources()
			require.NoError(t, integration.CreateObjects(tt.args.ctx, objs)(m.GetClient()))
			defer func() {
				require.NoError(t, integration.RemoveObjects(tt.args.ctx, objs)(m.GetClient()))
			}()
			if fn != nil {
				require.NoError(t, fn(m.GetClient()))
			}

			got, err := bAPI.Bind(tt.args.ctx, tt.args.instanceID, tt.args.bindingID, tt.args.details, false)
			if tt.wantErr != nil {
				assert.EqualError(t, err, tt.wantErr.Error())
				return
			}

			assert.NoError(t, err)
			tt.wantComparisonFunc(t, *tt.want, got)
		})
	}
}

func TestBrokerAPI_GetBinding(t *testing.T) {
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.GetBindingSpec
		wantErr   error
		resources func() (func(c client.Client) error, []runtime.Object)
	}{
		{
			name: "requires instance to be ready before getting a binding",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
			},
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				objs := []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					servicePlan.Composition,
					integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", ""),
				}
				return nil, objs
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")
				objs := []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					integration.NewTestServicePlan("1", "1-2", crossplane.RedisService).Composition,
					servicePlan.Composition,
					instance,
					integration.NewTestSecret(integration.TestNamespace, "1-1-1", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
						xrv1.ResourceCredentialsSecretEndpointKey: "localhost",
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
						"sentinelPort": "21234",
					}),
				}
				return func(c client.Client) error {
					return integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
				}, objs
			},
			want: &domain.GetBindingSpec{
				Credentials: crossplane.Credentials{
					"host":     "localhost",
					"master":   "redis://1-1-1",
					"password": "supersecret",
					"port":     1234,
					"sentinels": []crossplane.Credentials{
						{
							"host": "localhost",
							"port": 21234,
						},
					},
					"servers": []crossplane.Credentials{
						{
							"host": "localhost",
							"port": 1234,
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	m, logger, cp, err := integration.SetupManager(t)
	require.NoError(t, err, "unable to setup integration test manager")
	defer m.Cleanup()

	bAPI, err := New(cp, logger)
	require.NoError(t, err, "unable to setup brokerapi")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, objs := tt.resources()
			require.NoError(t, integration.CreateObjects(tt.args.ctx, objs)(m.GetClient()))
			defer func() {
				require.NoError(t, integration.RemoveObjects(tt.args.ctx, objs)(m.GetClient()))
			}()
			if fn != nil {
				require.NoError(t, fn(m.GetClient()))
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
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.GetInstanceDetailsSpec
		wantErr   error
		resources func() (func(c client.Client) error, []runtime.Object)
	}{
		{
			name: "gets an instance without parameters",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
			},
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")

				return nil, []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					integration.NewTestServicePlan("1", "1-2", crossplane.RedisService).Composition,
					servicePlan.Composition,
					instance,
				}
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBDatabaseService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBDatabaseService, "", "1")

				return nil, []runtime.Object{
					integration.NewTestService("1", crossplane.MariaDBDatabaseService),
					integration.NewTestServicePlan("1", "1-2", crossplane.MariaDBDatabaseService).Composition,
					servicePlan.Composition,
					instance,
				}
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
	m, logger, cp, err := integration.SetupManager(t)
	require.NoError(t, err, "unable to setup integration test manager")
	defer m.Cleanup()

	bAPI, err := New(cp, logger)
	require.NoError(t, err, "unable to setup brokerapi")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, objs := tt.resources()
			require.NoError(t, integration.CreateObjects(tt.args.ctx, objs)(m.GetClient()))
			defer func() {
				require.NoError(t, integration.RemoveObjects(tt.args.ctx, objs)(m.GetClient()))
			}()
			if fn != nil {
				require.NoError(t, fn(m.GetClient()))
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
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		details    domain.UpdateDetails
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.UpdateServiceSpec
		wantErr   error
		resources func() (func(c client.Client) error, []runtime.Object)
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					integration.NewTestService("2", crossplane.RedisService),
					integration.NewTestServicePlan("1", "1-2", crossplane.RedisService).Composition,
					integration.NewTestServicePlan("2", "2-1", crossplane.RedisService).Composition,
					servicePlan.Composition,
					instance,
				}
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlanWithSize("1", "1-1", crossplane.RedisService, "small", "standard")
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					integration.NewTestService("2", crossplane.RedisService),
					integration.NewTestServicePlanWithSize("1", "1-2", crossplane.RedisService, "large", "standard").Composition,
					servicePlan.Composition,
					instance,
				}
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlanWithSize("1", "1-1", crossplane.RedisService, "small", "standard")
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					integration.NewTestService("2", crossplane.RedisService),
					integration.NewTestServicePlanWithSize("1", "1-2", crossplane.RedisService, "small-premium", "premium").Composition,
					servicePlan.Composition,
					instance,
				}
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlanWithSize("1", "1-1", crossplane.RedisService, "small", "standard")
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					integration.NewTestService("2", crossplane.RedisService),
					integration.NewTestServicePlanWithSize("1", "1-2", crossplane.RedisService, "small-premium", "premium").Composition,
					servicePlan.Composition,
					instance,
				}
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlanWithSize("1", "1-1", crossplane.RedisService, "small-premium", "premium")
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					integration.NewTestService("2", crossplane.RedisService),
					integration.NewTestServicePlanWithSize("1", "1-2", crossplane.RedisService, "small", "standard").Composition,
					servicePlan.Composition,
					instance,
				}
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlanWithSize("1", "1-1", crossplane.RedisService, "super-large", "standard")
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					integration.NewTestService("2", crossplane.RedisService),
					integration.NewTestServicePlanWithSize("1", "1-2", crossplane.RedisService, "super-large-premium", "premium").Composition,
					servicePlan.Composition,
					instance,
				}
			},
			want:    &domain.UpdateServiceSpec{},
			wantErr: nil,
		},
	}
	m, logger, cp, err := integration.SetupManager(t)
	require.NoError(t, err, "unable to setup integration test manager")
	defer m.Cleanup()

	bAPI, err := New(cp, logger)
	require.NoError(t, err, "unable to setup brokerapi")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, objs := tt.resources()
			require.NoError(t, integration.CreateObjects(tt.args.ctx, objs)(m.GetClient()))
			defer func() {
				require.NoError(t, integration.RemoveObjects(tt.args.ctx, objs)(m.GetClient()))
			}()
			if fn != nil {
				require.NoError(t, fn(m.GetClient()))
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
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		details    domain.UnbindDetails
	}
	ctx := context.WithValue(context.TODO(), middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.UnbindSpec
		wantErr   error
		resources func() (func(c client.Client) error, []runtime.Object)
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				return nil, []runtime.Object{
					integration.NewTestService("1", crossplane.RedisService),
					servicePlan.Composition,
					integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", ""),
				}
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
			resources: func() (func(c client.Client) error, []runtime.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBDatabaseService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBDatabaseService, "", "1")
				objs := []runtime.Object{
					integration.NewTestService("1", crossplane.MariaDBDatabaseService),
					servicePlan.Composition,
					instance,
					integration.NewTestMariaDBUserInstance("1-1-1", "binding-1"),
					integration.NewTestSecret(integration.TestNamespace, "binding-1-password", map[string]string{
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
					}),
				}
				return func(c client.Client) error {
					return integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
				}, objs
			},
			want: &domain.UnbindSpec{
				IsAsync: false,
			},
			wantErr: nil,
		},
	}
	m, logger, cp, err := integration.SetupManager(t)
	require.NoError(t, err, "unable to setup integration test manager")
	defer m.Cleanup()

	bAPI, err := New(cp, logger)
	require.NoError(t, err, "unable to setup brokerapi")

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, objs := tt.resources()
			require.NoError(t, integration.CreateObjects(tt.args.ctx, objs)(m.GetClient()))
			defer func() {
				// if wantErr == nil, secret must be gone and would error here if we still would try to remove it again
				if tt.wantErr == nil {
					objs = objs[:len(objs)-1]
				}
				require.NoError(t, integration.RemoveObjects(tt.args.ctx, objs)(m.GetClient()))
			}()
			if fn != nil {
				require.NoError(t, fn(m.GetClient()))
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
