// +build integration

package brokerapi

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"github.com/pascaldekloe/jwt"
	"github.com/pivotal-cf/brokerapi/v8/domain"
	"github.com/pivotal-cf/brokerapi/v8/domain/apiresponses"
	"github.com/pivotal-cf/brokerapi/v8/middlewares"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/vshn/crossplane-service-broker/pkg/api/auth"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	cintegration "github.com/crossplane/crossplane-runtime/pkg/test/integration"
	"github.com/vshn/crossplane-service-broker/pkg/crossplane"
	"github.com/vshn/crossplane-service-broker/pkg/integration"
)

type EnvTestSuite struct {
	suite.Suite
	Ctx        context.Context
	Logger     lager.Logger
	Manager    *cintegration.Manager
	Crossplane *crossplane.Crossplane
}

func Test_BrokerAPI(t *testing.T) {
	suite.Run(t, new(EnvTestSuite))
}

func (ts *EnvTestSuite) SetupSuite() {
	m, logger, cp, err := integration.SetupManager(ts.T())
	ts.Require().NoError(err, "unable to setup integration test manager")

	ts.Logger = logger
	ts.Manager = m
	ts.Crossplane = cp
	ts.Ctx = context.Background()
}

func (ts *EnvTestSuite) TearDownSuite() {
	_ = ts.Manager.Cleanup()
}

func (ts *EnvTestSuite) givenContext() context.Context {
	ctx := context.WithValue(ts.Ctx, middlewares.CorrelationIDKey, "corrid")
	ctx = context.WithValue(ctx, auth.AuthenticationMethodPropertyName, auth.AuthenticationMethodBearerToken)
	ctx = context.WithValue(ctx, auth.TokenPropertyName, &jwt.Claims{Set: map[string]interface{}{"sub": "username"}})
	return ctx
}

func (ts *EnvTestSuite) TestBrokerAPI_Services() {
	tests := []struct {
		name      string
		want      []domain.Service
		wantErr   bool
		resources []client.Object
	}{
		{
			name: "returns the catalog successfully",
			resources: []client.Object{
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

	bAPI := New(ts.Crossplane, ts.Logger)

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			ts.Require().NoError(integration.CreateObjects(ts.Ctx, tt.resources)(ts.Manager.GetClient()))

			got, err := bAPI.Services(ts.Ctx)
			if tt.wantErr {
				ts.Assert().Error(err)
				return
			}

			ts.Assert().NoError(err)
			ts.Assert().Equal(tt.want, got)

			ts.Require().NoError(integration.RemoveObjects(ts.Ctx, tt.resources)(ts.Manager.GetClient()))
		})
	}
}

func (ts *EnvTestSuite) TestBrokerAPI_Provision() {
	type args struct {
		ctx          context.Context
		instanceID   string
		details      domain.ProvisionDetails
		asyncAllowed bool
	}

	tests := []struct {
		name      string
		args      args
		want      *domain.ProvisionedServiceSpec
		wantErr   error
		resources func() []client.Object
	}{
		{
			name: "requires async",
			args: args{
				ctx:          ts.givenContext(),
				instanceID:   "1",
				details:      domain.ProvisionDetails{},
				asyncAllowed: false,
			},
			want: nil,
			resources: func() []client.Object {
				return nil
			},
			wantErr: errors.New(`This service plan requires client support for asynchronous service operations. (correlation-id: "corrid")`),
		},
		{
			name: "specified plan must exist",
			args: args{
				ctx:        ts.givenContext(),
				instanceID: "1",
				details: domain.ProvisionDetails{
					PlanID: "1-1",
				},
				asyncAllowed: true,
			},
			want: nil,
			resources: func() []client.Object {
				return nil
			},
			wantErr: errors.New(`compositions.apiextensions.crossplane.io "1-1" not found (correlation-id: "corrid")`),
		},
		{
			name: "creates a redis instance",
			args: args{
				ctx:        ts.givenContext(),
				instanceID: "1",
				details: domain.ProvisionDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
				asyncAllowed: true,
			},
			resources: func() []client.Object {
				return []client.Object{
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
				ctx:        ts.givenContext(),
				instanceID: "1",
				details: domain.ProvisionDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
				asyncAllowed: true,
			},
			resources: func() []client.Object {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				return []client.Object{
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
				ctx:        ts.givenContext(),
				instanceID: "1",
				details: domain.ProvisionDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
				asyncAllowed: true,
			},
			resources: func() []client.Object {
				return []client.Object{
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
				ctx:        ts.givenContext(),
				instanceID: "2",
				details: domain.ProvisionDetails{
					PlanID:        "2-1",
					ServiceID:     "2",
					RawParameters: json.RawMessage(`{"parent_reference": "1"}`),
				},
				asyncAllowed: true,
			},
			resources: func() []client.Object {
				return []client.Object{
					integration.NewTestService("1", crossplane.MariaDBService),
					integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService).Composition,
					integration.NewTestService("2", crossplane.MariaDBDatabaseService),
					integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService).Composition,
				}
			},
			want:    &domain.ProvisionedServiceSpec{IsAsync: true},
			wantErr: nil,
		},
		{
			name: "creates a mariadb database instance referencing inexistent parent",
			args: args{
				ctx:        ts.givenContext(),
				instanceID: "3",
				details: domain.ProvisionDetails{
					PlanID:        "2-1",
					ServiceID:     "2",
					RawParameters: json.RawMessage(`{"parent_reference": "non-existent"}`),
				},
				asyncAllowed: true,
			},
			resources: func() []client.Object {
				return []client.Object{
					integration.NewTestService("1", crossplane.MariaDBService),
					integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService).Composition,
					integration.NewTestService("2", crossplane.MariaDBDatabaseService),
					integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService).Composition,
				}
			},
			want:    nil,
			wantErr: errors.New(`valid "parent_reference" required: compositemariadbinstances.syn.tools "non-existent" not found (correlation-id: "corrid")`),
		},
	}

	bAPI := New(ts.Crossplane, ts.Logger)

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			ts.Require().NoError(integration.CreateObjects(tt.args.ctx, tt.resources())(ts.Manager.GetClient()))
			defer func() {
				ts.Require().NoError(integration.RemoveObjects(tt.args.ctx, tt.resources())(ts.Manager.GetClient()))
			}()

			got, err := bAPI.Provision(tt.args.ctx, tt.args.instanceID, tt.args.details, tt.args.asyncAllowed)
			if tt.wantErr != nil {
				ts.Assert().EqualError(err, tt.wantErr.Error())
				return
			}

			ts.Assert().NoError(err)
			ts.Assert().Equal(*tt.want, got)
		})
	}
}

func (ts *EnvTestSuite) TestBrokerAPI_Deprovision() {
	type args struct {
		ctx          context.Context
		instanceID   string
		details      domain.DeprovisionDetails
		asyncAllowed bool
	}
	tests := []struct {
		name          string
		args          args
		want          *domain.DeprovisionServiceSpec
		wantErr       error
		resources     func() []client.Object
		customCheckFn func(t *testing.T, c client.Client)
	}{
		{
			name: "requires instance to exist before deprovisioning",
			args: args{
				ctx:        ts.givenContext(),
				instanceID: "1-1-1",
				details: domain.DeprovisionDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			resources: func() []client.Object {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				return []client.Object{
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
				ctx:        ts.givenContext(),
				instanceID: "1",
				details: domain.DeprovisionDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
				asyncAllowed: true,
			},
			resources: func() []client.Object {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				return []client.Object{
					service,
					servicePlan.Composition,
					integration.NewTestInstance("1", servicePlan, crossplane.RedisService, "", ""),
				}
			},
			customCheckFn: func(t *testing.T, c client.Client) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				_, err := integration.GetInstance(ts.givenContext(), c, servicePlan, "1")
				ts.Assert().EqualError(err, `compositeredisinstances.syn.tools "1" not found`)
			},
			want:    &domain.DeprovisionServiceSpec{IsAsync: false},
			wantErr: nil,
		},
		{
			name: "prevents removing instance if Deprovisionable returns an error",
			args: args{
				ctx:        ts.givenContext(),
				instanceID: "1",
				details: domain.DeprovisionDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
				asyncAllowed: true,
			},
			resources: func() []client.Object {
				service := integration.NewTestService("1", crossplane.MariaDBService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				mdbs := integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService)

				dbInstance := integration.NewTestInstance("2", mdbs, crossplane.MariaDBDatabaseService, "", "1")

				return []client.Object{
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

	bAPI := New(ts.Crossplane, ts.Logger)

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			objs := tt.resources()
			ts.Require().NoError(integration.CreateObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			defer func() {
				// if wantErr == nil, the instance is gone already and would error when trying to remove it again.
				if tt.wantErr == nil {
					objs = objs[:len(objs)-1]
				}
				ts.Require().NoError(integration.RemoveObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			}()

			got, err := bAPI.Deprovision(tt.args.ctx, tt.args.instanceID, tt.args.details, tt.args.asyncAllowed)
			if tt.wantErr != nil {
				ts.Assert().EqualError(err, tt.wantErr.Error())
				return
			}

			ts.Assert().NoError(err)
			ts.Assert().Equal(*tt.want, got)

			if tt.customCheckFn != nil {
				tt.customCheckFn(ts.T(), ts.Manager.GetClient())
			}
		})
	}
}

func (ts *EnvTestSuite) TestBrokerAPI_LastOperation() {
	type args struct {
		ctx        context.Context
		instanceID string
		details    domain.PollDetails
	}
	ctx := context.WithValue(ts.Ctx, middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.LastOperation
		wantErr   error
		resources func() (func(c client.Client) error, []client.Object)
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
			resources: func() (func(c client.Client) error, []client.Object) {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				return nil, []client.Object{
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
			resources: func() (func(c client.Client) error, []client.Object) {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				instance := integration.NewTestInstance("1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
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
			resources: func() (func(c client.Client) error, []client.Object) {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				instance := integration.NewTestInstance("1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
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
			resources: func() (func(c client.Client) error, []client.Object) {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				instance := integration.NewTestInstance("1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
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
	bAPI := New(ts.Crossplane, ts.Logger)

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			fn, objs := tt.resources()
			ts.Require().NoError(integration.CreateObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			if fn != nil {
				ts.Require().NoError(fn(ts.Manager.GetClient()))
			}

			defer func() {
				ts.Require().NoError(integration.RemoveObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			}()

			got, err := bAPI.LastOperation(tt.args.ctx, tt.args.instanceID, tt.args.details)
			if tt.wantErr != nil {
				ts.Assert().EqualError(err, tt.wantErr.Error())
				return
			}

			ts.Assert().NoError(err)
			ts.Assert().Equal(*tt.want, got)
		})
	}
}

func (ts *EnvTestSuite) TestBrokerAPI_LastBindingOperation() {
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		details    domain.PollDetails
	}
	ctx := context.WithValue(ts.Ctx, middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.LastOperation
		wantErr   error
		resources func() (func(c client.Client) error, []client.Object)
	}{
		{
			name: "inexistent redis binding fails",
			args: args{
				ctx:        ctx,
				instanceID: "inexistent-name",
				bindingID:  "binding-1",
				details: domain.PollDetails{
					PlanID: "1-1",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				instance := integration.NewTestInstance("instance-1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
					service,
					servicePlan.Composition,
					instance,
					integration.NewTestSecret(integration.TestNamespace, "instance-1", map[string]string{
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
			want:    nil,
			wantErr: apiresponses.ErrInstanceDoesNotExist.AppendErrorMessage(`(correlation-id: "corrid")`),
		},
		{
			name: "redis binding returns succeeded",
			args: args{
				ctx:        ctx,
				instanceID: "instance-1",
				bindingID:  "binding-1",
				details: domain.PollDetails{
					PlanID: "1-1",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				service := integration.NewTestService("1", crossplane.RedisService)
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)

				instance := integration.NewTestInstance("instance-1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
					service,
					servicePlan.Composition,
					instance,
					integration.NewTestSecret(integration.TestNamespace, "instance-1", map[string]string{
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
			want: &domain.LastOperation{
				State: domain.Succeeded,
			},
			wantErr: nil,
		},
		{
			name: "Unready MariaDB binding returns in progress",
			args: args{
				ctx:        ctx,
				instanceID: "instance-1",
				bindingID:  "binding-1",
				details: domain.PollDetails{
					PlanID: "2-1",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				dbServicePlan := integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService)
				objs := []client.Object{
					integration.NewTestService("1", crossplane.MariaDBService),
					integration.NewTestService("2", crossplane.MariaDBDatabaseService),
					servicePlan.Composition,
					integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", ""),
					dbServicePlan.Composition,
					integration.NewTestInstance("instance-1", dbServicePlan, crossplane.MariaDBDatabaseService, "", "1-1-1"),
					integration.NewTestMariaDBUserInstance("instance-1", "binding-1"),
				}
				return nil, objs
			},
			want: &domain.LastOperation{
				State: domain.InProgress,
			},
			wantErr: nil,
		},
		{
			name: "Ready MariaDB binding returns ready",
			args: args{
				ctx:        ctx,
				instanceID: "instance-1",
				bindingID:  "binding-1",
				details: domain.PollDetails{
					PlanID: "2-1",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				dbServicePlan := integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService)
				userInstance := integration.NewTestMariaDBUserInstance("instance-1", "binding-1")
				objs := []client.Object{
					integration.NewTestService("1", crossplane.MariaDBService),
					integration.NewTestService("2", crossplane.MariaDBDatabaseService),
					servicePlan.Composition,
					integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", ""),
					dbServicePlan.Composition,
					integration.NewTestInstance("instance-1", dbServicePlan, crossplane.MariaDBDatabaseService, "", "1-1-1"),
					userInstance,
					integration.NewTestSecret(integration.TestNamespace, "binding-1", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
						xrv1.ResourceCredentialsSecretEndpointKey: "localhost",
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
					}),
				}
				return func(c client.Client) error {
					userPlan := &crossplane.Plan{
						Composition: &xv1.Composition{
							Spec: xv1.CompositionSpec{
								CompositeTypeRef: xv1.TypeReference{
									APIVersion: "syn.tools/v1alpha1",
									Kind:       "CompositeMariaDBUserInstance",
								},
							},
						},
					}
					return integration.UpdateInstanceConditions(ctx, c, userPlan, userInstance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
				}, objs
			},
			want: &domain.LastOperation{
				State: domain.Succeeded,
			},
			wantErr: nil,
		},
		{
			name: "inexistent MariaDB binding returns failed",
			args: args{
				ctx:        ctx,
				instanceID: "instance-1",
				bindingID:  "inexistent-binding",
				details: domain.PollDetails{
					PlanID: "2-1",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				dbServicePlan := integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService)
				objs := []client.Object{
					integration.NewTestService("1", crossplane.MariaDBService),
					integration.NewTestService("2", crossplane.MariaDBDatabaseService),
					servicePlan.Composition,
					integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", ""),
					dbServicePlan.Composition,
					integration.NewTestInstance("instance-1", dbServicePlan, crossplane.MariaDBDatabaseService, "", "1-1-1"),
					integration.NewTestMariaDBUserInstance("instance-1", "binding-1"),
				}
				return nil, objs
			},
			want:    nil,
			wantErr: apiresponses.ErrBindingDoesNotExist.AppendErrorMessage(`(correlation-id: "corrid")`),
		},
	}
	bAPI := New(ts.Crossplane, ts.Logger)

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			fn, objs := tt.resources()
			ts.Require().NoError(integration.CreateObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			if fn != nil {
				ts.Require().NoError(fn(ts.Manager.GetClient()))
			}

			defer func() {
				ts.Require().NoError(integration.RemoveObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			}()

			got, err := bAPI.LastBindingOperation(tt.args.ctx, tt.args.instanceID, tt.args.bindingID, tt.args.details)
			if tt.wantErr != nil {
				ts.Assert().EqualError(err, tt.wantErr.Error())
				return
			}

			ts.Assert().NoError(err)
			ts.Assert().Equal(*tt.want, got)
		})
	}
}

func (ts *EnvTestSuite) TestBrokerAPI_Bind() {
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		details    domain.BindDetails
		async      bool
	}
	ctx := context.WithValue(ts.Ctx, middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name               string
		args               args
		want               *domain.Binding
		wantComparisonFunc assert.ComparisonAssertionFunc
		wantErr            error
		resources          func() (func(c client.Client) error, []client.Object)
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				return nil, []client.Object{
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
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
			name: "creates a redis instance with monitoring endpoints and binds it",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.BindDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
					integration.NewTestService("1", crossplane.RedisService),
					servicePlan.Composition,
					instance,
					integration.NewTestSecret(integration.TestNamespace, "1-1-1", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
						xrv1.ResourceCredentialsSecretEndpointKey: "localhost",
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
						"sentinelPort":   "21234",
						"monitoringPort": "25197",
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
					"metrics": []string{
						"http://localhost:25197/metrics/haproxy-0",
						"http://localhost:25197/metrics/haproxy-1",
						"http://localhost:25197/metrics/redis-0",
						"http://localhost:25197/metrics/redis-1",
						"http://localhost:25197/metrics/redis-2",
					},
				},
			},
			wantComparisonFunc: assert.Equal,
			wantErr:            nil,
		},
		{
			name: "creates a redis instance and binds it asynchronously",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				async:      true,
				details: domain.BindDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
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
			want:               nil,
			wantComparisonFunc: nil,
			wantErr:            nil,
		},
		{
			name: "creates a redis instance and tries to bind it while the endpoint is not ready",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.BindDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
					integration.NewTestService("1", crossplane.RedisService),
					servicePlan.Composition,
					instance,
					integration.NewTestSecret(integration.TestNamespace, "1-1-1", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
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
			wantComparisonFunc: nil,
			wantErr:            errors.New(`binding cannot be fetched (correlation-id: "corrid")`),
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", "")
				objs := []client.Object{
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
			name: "creates a mariadb instance with metrics endpoints and binds a database instance to it",
			args: args{
				ctx:        ctx,
				instanceID: "1-2-1",
				bindingID:  "4",
				details: domain.BindDetails{
					PlanID:    "2-1",
					ServiceID: "2",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", "")
				dbServicePlan := integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService)
				dbInstance := integration.NewTestInstance("1-2-1", dbServicePlan, crossplane.MariaDBDatabaseService, "", "1-1-1")
				objs := []client.Object{
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
						"monitoringPort":                          "25197",
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
					"metrics": []string{
						"http://localhost:25197/metrics/haproxy-0",
						"http://localhost:25197/metrics/haproxy-1",
						"http://localhost:25197/metrics/mariadb-0",
						"http://localhost:25197/metrics/mariadb-1",
						"http://localhost:25197/metrics/mariadb-2",
					},
				},
			},
			wantComparisonFunc: func(t assert.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
				want := expected.(domain.Binding)
				got := actual.(domain.Binding)

				ts.Assert().Equal(want.IsAsync, got.IsAsync, "asynchronous does not match")

				wantCreds := want.Credentials.(crossplane.Credentials)
				gotCreds := got.Credentials.(crossplane.Credentials)

				ts.Assert().Equal(len(wantCreds), len(gotCreds), "number of credentials should be equal")
				for k, v := range wantCreds {
					if v == "***" {
						assert.Contains(t, gotCreds, k, k)
					} else {
						ts.Assert().Equal(v, gotCreds[k], k)
					}
				}
				return true

			},
			wantErr: nil,
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", "")
				dbServicePlan := integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService)
				dbInstance := integration.NewTestInstance("1-2-1", dbServicePlan, crossplane.MariaDBDatabaseService, "", "1-1-1")
				objs := []client.Object{
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

				ts.Assert().Equal(want.IsAsync, got.IsAsync)

				wantCreds := want.Credentials.(crossplane.Credentials)
				gotCreds := got.Credentials.(crossplane.Credentials)

				ts.Assert().Equal(len(wantCreds), len(gotCreds))
				for k, v := range wantCreds {
					if v == "***" {
						assert.Contains(t, gotCreds, k, k)
					} else {
						ts.Assert().Equal(v, gotCreds[k], k)
					}
				}
				return true

			},
			wantErr: nil,
		},
		{
			name: "creates a mariadb instance and binds a database instance to it asynchronously",
			args: args{
				ctx:        ctx,
				instanceID: "1-2-1",
				bindingID:  "3",
				async:      true,
				details: domain.BindDetails{
					PlanID:    "2-1",
					ServiceID: "2",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBService, "", "")
				dbServicePlan := integration.NewTestServicePlan("2", "2-1", crossplane.MariaDBDatabaseService)
				dbInstance := integration.NewTestInstance("1-2-1", dbServicePlan, crossplane.MariaDBDatabaseService, "", "1-1-1")
				objs := []client.Object{
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
			want:               nil,
			wantComparisonFunc: nil,
			wantErr:            nil,
		},
	}

	bAPI := New(ts.Crossplane, ts.Logger)

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			fn, objs := tt.resources()
			ts.Require().NoError(integration.CreateObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			defer func() {
				ts.Require().NoError(integration.RemoveObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			}()
			if fn != nil {
				ts.Require().NoError(fn(ts.Manager.GetClient()))
			}

			got, err := bAPI.Bind(tt.args.ctx, tt.args.instanceID, tt.args.bindingID, tt.args.details, tt.args.async)
			if tt.wantErr != nil {
				ts.Assert().EqualError(err, tt.wantErr.Error())
				return
			}

			ts.Assert().NoError(err)
			ts.Assert().Equal(got.IsAsync, tt.args.async)
			if tt.wantComparisonFunc != nil && tt.want != nil {
				tt.wantComparisonFunc(ts.T(), *tt.want, got)
			}
		})
	}
}

func (ts *EnvTestSuite) TestBrokerAPI_GetBinding() {
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		planID     string
	}
	ctx := context.WithValue(ts.Ctx, middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.GetBindingSpec
		wantErr   error
		resources func() (func(c client.Client) error, []client.Object)
	}{
		{
			name: "requires instance to be ready before getting a binding",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				planID:     "1-1",
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				objs := []client.Object{
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
			name: "requires secret to be ready before getting a binding",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				planID:     "1-1",
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
					integration.NewTestService("1", crossplane.RedisService),
					integration.NewTestServicePlan("1", "1-2", crossplane.RedisService).Composition,
					servicePlan.Composition,
					instance,
					integration.NewTestSecret(integration.TestNamespace, "1-1-1", map[string]string{
						xrv1.ResourceCredentialsSecretPortKey:     "1234",
						xrv1.ResourceCredentialsSecretPasswordKey: "supersecret",
						"sentinelPort": "21234",
					}),
				}
				return func(c client.Client) error {
					return integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
				}, objs
			},
			want:    nil,
			wantErr: apiresponses.ErrBindingNotFound.AppendErrorMessage(`(correlation-id: "corrid")`),
		},
		{
			name: "creates a redis instance and gets the binding",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				planID:     "1-1",
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")
				objs := []client.Object{
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

	bAPI := New(ts.Crossplane, ts.Logger)

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			fn, objs := tt.resources()
			ts.Require().NoError(integration.CreateObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			defer func() {
				ts.Require().NoError(integration.RemoveObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			}()
			if fn != nil {
				ts.Require().NoError(fn(ts.Manager.GetClient()))
			}

			got, err := bAPI.GetBinding(
				tt.args.ctx,
				tt.args.instanceID,
				tt.args.bindingID,
				domain.FetchBindingDetails{
					PlanID: tt.args.planID,
				},
			)
			if tt.wantErr != nil {
				ts.Assert().EqualError(err, tt.wantErr.Error())
				return
			}

			ts.Assert().NoError(err)
			ts.Assert().Equal(*tt.want, got)
		})
	}
}

func (ts *EnvTestSuite) TestBrokerAPI_GetInstance() {
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		planID     string
	}
	ctx := context.WithValue(ts.Ctx, middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.GetInstanceDetailsSpec
		wantErr   error
		resources func() (func(c client.Client) error, []client.Object)
	}{
		{
			name: "gets an instance without parameters",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				planID:     "1-1",
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", "")

				return nil, []client.Object{
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
				planID:     "1-1",
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBDatabaseService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBDatabaseService, "", "1")

				return nil, []client.Object{
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

	bAPI := New(ts.Crossplane, ts.Logger)

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			fn, objs := tt.resources()
			ts.Require().NoError(integration.CreateObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			defer func() {
				ts.Require().NoError(integration.RemoveObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			}()
			if fn != nil {
				ts.Require().NoError(fn(ts.Manager.GetClient()))
			}

			got, err := bAPI.GetInstance(tt.args.ctx, tt.args.instanceID, domain.FetchInstanceDetails{
				PlanID: tt.args.planID,
			})
			if tt.wantErr != nil {
				ts.Assert().EqualError(err, tt.wantErr.Error())
				return
			}

			ts.Assert().NoError(err)
			ts.Assert().Equal(*tt.want, got)
		})
	}
}

func (ts *EnvTestSuite) TestBrokerAPI_Update() {
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		details    domain.UpdateDetails
	}
	ctx := context.WithValue(ts.Ctx, middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.UpdateServiceSpec
		wantErr   error
		resources func() (func(c client.Client) error, []client.Object)
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []client.Object{
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlanWithSize("1", "1-1", crossplane.RedisService, "small", "standard")
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []client.Object{
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlanWithSize("1", "1-1", crossplane.RedisService, "small", "standard")
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []client.Object{
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlanWithSize("1", "1-1", crossplane.RedisService, "small", "standard")
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []client.Object{
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlanWithSize("1", "1-1", crossplane.RedisService, "small-premium", "premium")
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []client.Object{
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlanWithSize("1", "1-1", crossplane.RedisService, "super-large", "standard")
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "1", "")

				return nil, []client.Object{
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

	bAPI := New(ts.Crossplane, ts.Logger)

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			fn, objs := tt.resources()
			ts.Require().NoError(integration.CreateObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			defer func() {
				ts.Require().NoError(integration.RemoveObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			}()
			if fn != nil {
				ts.Require().NoError(fn(ts.Manager.GetClient()))
			}

			got, err := bAPI.Update(tt.args.ctx, tt.args.instanceID, tt.args.details, false)
			if tt.wantErr != nil {
				ts.Assert().EqualError(err, tt.wantErr.Error())
				return
			}

			ts.Assert().NoError(err)
			ts.Assert().Equal(*tt.want, got)
		})
	}
}

func (ts *EnvTestSuite) TestBrokerAPI_Unbind() {
	type args struct {
		ctx        context.Context
		instanceID string
		bindingID  string
		details    domain.UnbindDetails
	}
	ctx := context.WithValue(ts.Ctx, middlewares.CorrelationIDKey, "corrid")

	tests := []struct {
		name      string
		args      args
		want      *domain.UnbindSpec
		wantErr   error
		resources func() (func(c client.Client) error, []client.Object)
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.RedisService)
				return nil, []client.Object{
					integration.NewTestService("1", crossplane.RedisService),
					servicePlan.Composition,
					integration.NewTestInstance("1-1-1", servicePlan, crossplane.RedisService, "", ""),
				}
			},
			want:    nil,
			wantErr: errors.New(`instance is being updated and cannot be retrieved (correlation-id: "corrid")`),
		},
		{
			name: "unbind inexistent binding",
			args: args{
				ctx:        ctx,
				instanceID: "1-1-1",
				bindingID:  "1",
				details: domain.UnbindDetails{
					PlanID:    "1-1",
					ServiceID: "1",
				},
			},
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBDatabaseService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBDatabaseService, "", "1")
				objs := []client.Object{
					servicePlan.Composition,
					instance,
				}
				return func(c client.Client) error {
					return integration.UpdateInstanceConditions(ctx, c, servicePlan, instance, xrv1.TypeReady, corev1.ConditionTrue, xrv1.ReasonAvailable)
				}, objs
			},
			want:    nil,
			wantErr: apiresponses.ErrBindingDoesNotExist.AppendErrorMessage(`(correlation-id: "corrid")`),
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
			resources: func() (func(c client.Client) error, []client.Object) {
				servicePlan := integration.NewTestServicePlan("1", "1-1", crossplane.MariaDBDatabaseService)
				instance := integration.NewTestInstance("1-1-1", servicePlan, crossplane.MariaDBDatabaseService, "", "1")
				objs := []client.Object{
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

	bAPI := New(ts.Crossplane, ts.Logger)

	for _, tt := range tests {
		ts.Run(tt.name, func() {
			fn, objs := tt.resources()
			ts.Require().NoError(integration.CreateObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			defer func() {
				// if wantErr == nil, secret must be gone and would error here if we still would try to remove it again
				if tt.wantErr == nil {
					objs = objs[:len(objs)-1]
				}
				ts.Require().NoError(integration.RemoveObjects(tt.args.ctx, objs)(ts.Manager.GetClient()))
			}()
			if fn != nil {
				ts.Require().NoError(fn(ts.Manager.GetClient()))
			}

			got, err := bAPI.Unbind(tt.args.ctx, tt.args.instanceID, tt.args.bindingID, tt.args.details, false)
			if tt.wantErr != nil {
				ts.Assert().EqualError(err, tt.wantErr.Error())
				return
			}

			ts.Assert().NoError(err)
			ts.Assert().Equal(*tt.want, got)
		})
	}
}
