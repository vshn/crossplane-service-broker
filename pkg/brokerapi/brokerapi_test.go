// +build integration

package brokerapi

import (
	"context"
	"fmt"
	"os"
	"testing"

	"code.cloudfoundry.org/lager"
	"github.com/crossplane/crossplane-runtime/pkg/test/integration"
	xv1 "github.com/crossplane/crossplane/apis/apiextensions/v1"
	"github.com/go-logr/zapr"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vshn/crossplane-service-broker/internal/broker"
	"github.com/vshn/crossplane-service-broker/internal/crossplane"
)

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
		preRunFn func(c client.Client) error
	}{
		{
			name: "returns the catalog successfully",
			ctx:  context.TODO(),
			preRunFn: func(c client.Client) error {
				ctx := context.TODO()
				objs := []runtime.Object{
					&xv1.CompositeResourceDefinition{
						ObjectMeta: metav1.ObjectMeta{
							Name: "testservice",
							Labels: map[string]string{
								crossplane.ServiceIDLabel:   "1",
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
					},
					&xv1.Composition{
						ObjectMeta: metav1.ObjectMeta{
							Name: "testservice-small",
							Labels: map[string]string{
								crossplane.ServiceIDLabel: "1",
								crossplane.PlanNameLabel:  "small",
								crossplane.BindableLabel:  "true",
							},
							Annotations: map[string]string{
								crossplane.DescriptionAnnotation: "testservice-small plan description",
								crossplane.MetadataAnnotation:    `{"displayName": "small"}`,
							},
						},
						Spec: xv1.CompositionSpec{
							Resources: []xv1.ComposedTemplate{},
						},
					},
				}
				for _, obj := range objs {
					if err := c.Create(ctx, obj); err != nil {
						return err
					}
				}
				return nil
			},
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
							ID:          "testservice-small",
							Name:        "small",
							Description: "testservice-small plan description",
							Free:        boolPtr(false),
							Bindable:    boolPtr(true),
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

	if db := os.Getenv("DEBUG"); db != "" {
		zl, _ := zap.NewDevelopment()
		log.SetLogger(zapr.NewLogger(zl))
	}

	m, err := integration.New(nil,
		integration.WithCRDPaths("../../testdata/crds"),
	)
	if err != nil {
		assert.FailNow(t, fmt.Sprintf("unable to setup integration test manager: %s", err))
		return
	}

	m.Run()

	scheme := m.GetScheme()
	assert.NoError(t, xv1.AddToScheme(scheme))

	logger := lager.NewLogger("test")

	cp, err := crossplane.New([]string{"1", "2"}, m.GetConfig(), logger)
	if err != nil {
		assert.FailNow(t, fmt.Sprintf("unable to setup crossplane client: %s", err))
		return
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func boolPtr(b bool) *bool {
	return &b
}
