package crossplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// errNotImplemented is the error returned for not implmemented functions
	errNotImplemented = apiresponses.NewFailureResponseBuilder(
		errors.New("not implemented"),
		http.StatusNotImplemented,
		"not-implemented").
		WithErrorKey("NotImplemented").
		Build()

	mariaDBGroupVersionKind = schema.GroupVersionKind{
		Group:   "syn.tools",
		Version: "v1alpha1",
		Kind:    "CompositeMariaDBInstance",
	}
)

// MariadbServiceBinder defines a specific Mariadb service with enough data to retrieve connection credentials.
type MariadbServiceBinder struct {
	id     string
	cp     *Crossplane
	logger lager.Logger
}

// NewMariadbServiceBinder instantiates a Mariadb service instance based on the given CompositeMariadbInstance.
func NewMariadbServiceBinder(c *Crossplane, id string, logger lager.Logger) *MariadbServiceBinder {
	return &MariadbServiceBinder{
		id:     id,
		cp:     c,
		logger: logger,
	}
}

// Bind on a MariaDB instance is not supported - only a database referencing an instance can be bound.
func (msb MariadbServiceBinder) Bind(_ context.Context, _ string) (Credentials, error) {
	return nil, apiresponses.NewFailureResponseBuilder(
		fmt.Errorf("service MariaDB Galera Cluster is not bindable. "+
			"You can create a bindable database on this cluster using "+
			"cf create-service mariadb-k8s-database default my-mariadb-db -c '{\"%s\": %q}'", instanceParamsParentReferenceName, msb.id),
		http.StatusUnprocessableEntity,
		"binding-not-supported",
	).WithErrorKey("BindingNotSupported").Build()
}

// Unbind on a MariaDB instance is not supported - only a database referencing an instance can be bound.
func (msb MariadbServiceBinder) Unbind(_ context.Context, _ string) error {
	return errNotImplemented
}

// Deprovisionable checks if no DBs exist for this instance anymore.
func (msb MariadbServiceBinder) Deprovisionable(ctx context.Context) error {
	instanceList := &unstructured.UnstructuredList{}
	instanceList.SetGroupVersionKind(mariaDBDatabaseGroupVersionKind)
	if err := msb.cp.client.List(ctx, instanceList, client.MatchingLabels{
		ParentIDLabel: msb.id,
	}); err != nil {
		return err
	}
	if len(instanceList.Items) > 0 {
		var instances []string
		for _, instance := range instanceList.Items {
			instances = append(instances, instance.GetName())
		}
		return apiresponses.NewFailureResponseBuilder(
			fmt.Errorf("instance is still in use by %q", strings.Join(instances, ", ")),
			http.StatusUnprocessableEntity,
			"deprovision-instance-in-use",
		).WithErrorKey("InUseError").Build()
	}
	return nil
}

// GetBinding is not implemented.
func (msb MariadbServiceBinder) GetBinding(_ context.Context, _ string) (Credentials, error) {
	return nil, errNotImplemented
}

// FinishProvision is called after a successful provisioning
// FIXME(mw): FinishProvision might be needed, but probably not.
func (msb MariadbServiceBinder) FinishProvision(ctx context.Context) error {
	s, err := msb.cp.getCredentials(ctx, msb.id)
	if err != nil {
		return err
	}

	// having an endpoint in the secret means fetching the endpoint via HaProxy
	// has been successfully implemented using provider-helm.
	if s.Data[xrv1.ResourceCredentialsSecretEndpointKey] != nil {
		return nil
	}
	return errors.New("finishProvision deactivated until proper solution in place. Retrieving Endpoint needs implementation")
}