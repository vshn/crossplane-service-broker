package crossplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	serviceMariadb = "mariadb-k8s"
)

// MariadbServiceBinder defines a specific Mariadb service with enough data to retrieve connection credentials.
type MariadbServiceBinder struct {
	instanceID     string
	instanceLabels *Labels
	resources      []corev1.ObjectReference
	cp             *Crossplane
	logger         lager.Logger
}

// NewMariadbServiceBinder instantiates a Mariadb service instance based on the given CompositeMariadbInstance.
func NewMariadbServiceBinder(c *Crossplane, instance *Instance, logger lager.Logger) *MariadbServiceBinder {
	return &MariadbServiceBinder{
		instanceID:     instance.Composite.GetName(),
		instanceLabels: instance.Labels,
		resources:      instance.Composite.GetResourceReferences(),
		cp:             c,
		logger:         logger,
	}
}

// Bind on a MariaDB instance doesn't work - only a database referencing an instance can be bound.
func (msb MariadbServiceBinder) Bind(_ context.Context, _ string) (Credentials, error) {
	return nil, apiresponses.NewFailureResponseBuilder(
		fmt.Errorf("service MariaDB Galera Cluster is not bindable. "+
			"You can create a bindable database on this cluster using "+
			"cf create-service mariadb-k8s-database default my-mariadb-db -c '{\"parent_reference\": %q}'", msb.instanceID),
		http.StatusUnprocessableEntity,
		"binding-not-supported",
	).WithErrorKey("BindingNotSupported").Build()
}

// Deprovision removes the downstream namespace and checks if no DBs exist for this instance anymore.
func (msb MariadbServiceBinder) Deprovision(ctx context.Context) error {
	instanceList := &unstructured.UnstructuredList{}
	instanceList.SetGroupVersionKind(groupVersionKind)
	instanceList.SetKind("CompositeMariaDBDatabaseInstanceList")
	if err := msb.cp.client.List(ctx, instanceList, client.MatchingLabels{
		ParentIDLabel: msb.instanceID,
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
	return markNamespaceDeleted(ctx, msb.cp, msb.instanceID, msb.resources)
}

// FIXME(mw): FinishProvision might be needed, but probably not.
func (msb MariadbServiceBinder) FinishProvision(ctx context.Context) error {
	return errors.New("FinishProvision deactivated until proper solution in place. Retrieving Endpoint needs implementation.")
}
