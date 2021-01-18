package crossplane

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"code.cloudfoundry.org/lager"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	corev1 "k8s.io/api/core/v1"
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

// Bind is not implemented.
func (msb MariadbServiceBinder) Bind(_ context.Context, _ string) (Credentials, error) {
	return nil, apiresponses.NewFailureResponseBuilder(
		fmt.Errorf("service MariaDB Galera Cluster is not bindable. "+
			"You can create a bindable database on this cluster using "+
			"cf create-service mariadb-k8s-database default my-mariadb-db -c '{\"parent_reference\": %q}'", msb.instanceID),
		http.StatusUnprocessableEntity,
		"binding-not-supported",
	).WithErrorKey("BindingNotSupported").Build()
}

// FIXME(mw): FinishProvision might be needed, but probably not.
func (msb MariadbServiceBinder) FinishProvision(ctx context.Context) error {
	return errors.New("FinishProvision deactivated until proper solution in place. Retrieving Endpoint needs implementation.")
}
