package crossplane

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/password"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	"github.com/pivotal-cf/brokerapi/v8/domain/apiresponses"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	planName   = "mariadb-user"
	secretName = "%s-password"

	// instanceParamsParentReferenceName is the name of an instance's parent reference parameter
	instanceParamsParentReferenceName = "parent_reference"
	// instanceSpecParamsParentReferencePath is the path to an instance's parent reference parameter
	instanceSpecParamsParentReferencePath = instanceSpecParamsPath + "." + instanceParamsParentReferenceName
)

var (
	mariaDBUserGroupVersionKind = schema.GroupVersionKind{
		Group:   "syn.tools",
		Version: "v1alpha1",
		Kind:    "CompositeMariaDBUserInstance",
	}
	mariaDBDatabaseGroupVersionKind = schema.GroupVersionKind{
		Group:   "syn.tools",
		Version: "v1alpha1",
		Kind:    "CompositeMariaDBDatabaseInstanceList",
	}
)

// MariadbDatabaseServiceBinder defines a specific Mariadb service with enough data to retrieve connection credentials.
type MariadbDatabaseServiceBinder struct {
	serviceBinder
}

// NewMariadbDatabaseServiceBinder instantiates a Mariadb service instance based on the given CompositeMariadbInstance.
func NewMariadbDatabaseServiceBinder(c *Crossplane, instance *Instance, logger lager.Logger) *MariadbDatabaseServiceBinder {
	return &MariadbDatabaseServiceBinder{
		serviceBinder: serviceBinder{
			instance: instance,
			cp:       c,
			logger:   logger,
		},
	}
}

// Bind creates a MariaDB binding composite.
func (msb MariadbDatabaseServiceBinder) Bind(ctx context.Context, bindingID string) (Credentials, error) {
	parentRef, err := msb.instance.ParentReference()
	if err != nil {
		return nil, err
	}

	cmp := composite.New(composite.WithGroupVersionKind(mariaDBGroupVersionKind))
	if err := msb.cp.client.Get(ctx, types.NamespacedName{Name: parentRef}, cmp); err != nil {
		return nil, fmt.Errorf("could not get parent Galera cluster: %w", err)
	}

	pw, err := msb.createBinding(
		ctx,
		bindingID,
		msb.instance.ID(),
		parentRef,
	)
	if err != nil {
		return nil, err
	}

	// In order to directly return the credentials we need to get the IP/port for this instance.
	secret, err := msb.cp.GetConnectionDetails(ctx, cmp)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, ErrInstanceNotReady
		}
		return nil, err
	}

	endpoint, err := mapMariadbEndpoint(secret.Data)
	if err != nil {
		return nil, err
	}

	mp := ""
	mbyte, ok := secret.Data[MetricsPortKey]
	if ok {
		mp = string(mbyte)
	}

	creds := createCredentials(endpoint, bindingID, pw, msb.instance.ID(), mp)

	return creds, nil
}

// Unbind deletes the created User and Grant.
func (msb MariadbDatabaseServiceBinder) Unbind(ctx context.Context, bindingID string) error {
	cmp := composite.New(composite.WithGroupVersionKind(mariaDBUserGroupVersionKind))
	cmp.SetName(bindingID)
	if err := msb.cp.client.Delete(ctx, cmp, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		return err
	}

	// TODO: figure out a better way to delete the password secret
	//       option a) use Watch on resourceRefs of composite and wait until User/Grant are both deleted
	//       option b) https://github.com/crossplane/crossplane/issues/1612 is implemented by crossplane
	//                 and wait for the composite to disappear before removing the secret
	// If we delete the secret too quickly, the provider-sql can't deprovision the user
	time.Sleep(5 * time.Second)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(secretName, bindingID),
			Namespace: msb.cp.config.Namespace,
		},
	}
	return msb.cp.client.Delete(ctx, secret)
}

// Deprovisionable always returns nil for MariadbDatabase instances.
func (msb MariadbDatabaseServiceBinder) Deprovisionable(_ context.Context) error {
	return nil
}

// GetBinding returns credentials for MariaDB
func (msb MariadbDatabaseServiceBinder) GetBinding(ctx context.Context, bindingID string) (Credentials, error) {
	cmp := composite.New(composite.WithGroupVersionKind(mariaDBUserGroupVersionKind))
	if err := msb.cp.client.Get(ctx, types.NamespacedName{Name: bindingID}, cmp); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, apiresponses.ErrBindingDoesNotExist
		}
		return nil, fmt.Errorf("could not get binding: %w", err)
	}

	if cmp.GetCondition(xrv1.TypeReady).Status != corev1.ConditionTrue {
		return nil, ErrBindingNotReady
	}
	secret, err := msb.cp.GetConnectionDetails(ctx, cmp)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, ErrBindingNotReady
		}
		return nil, err
	}

	endpoint, err := mapMariadbEndpoint(secret.Data)
	if err != nil {
		return nil, err
	}

	mp := ""
	mbyte, ok := secret.Data[MetricsPortKey]
	if ok {
		mp = string(mbyte)
	}

	pw := string(secret.Data[xrv1.ResourceCredentialsSecretPasswordKey])
	creds := createCredentials(endpoint, bindingID, pw, msb.instance.ID(), mp)

	return creds, nil
}

// ValidateProvisionParams ensures the passed parent reference is an existing mariadb instance.
func (msb MariadbDatabaseServiceBinder) ValidateProvisionParams(ctx context.Context, params json.RawMessage) (map[string]interface{}, error) {
	paramsMap := map[string]interface{}{}
	if err := json.Unmarshal(params, &paramsMap); err != nil {
		return nil, err
	}
	parentRef, err := getParentRef(paramsMap)
	if err != nil {
		return nil, err
	}

	cmp := composite.New(composite.WithGroupVersionKind(mariaDBGroupVersionKind))
	if err := msb.cp.client.Get(ctx, types.NamespacedName{Name: parentRef}, cmp); err != nil {
		return nil, fmt.Errorf("valid %q required: %w", instanceParamsParentReferenceName, err)
	}
	return paramsMap, nil
}

func (msb MariadbDatabaseServiceBinder) createBinding(ctx context.Context, bindingID, instanceID, parentReference string) (string, error) {
	pw, err := password.Generate()
	if err != nil {
		return "", err
	}

	labels := map[string]string{
		InstanceIDLabel: instanceID,
		ParentIDLabel:   parentReference,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(secretName, bindingID),
			Namespace: msb.cp.config.Namespace,
			Labels:    labels,
		},
		Data: map[string][]byte{
			xrv1.ResourceCredentialsSecretPasswordKey: []byte(pw),
		},
	}
	err = msb.cp.client.Create(ctx, secret)
	if err != nil {
		return "", err
	}

	cmp := composite.New(composite.WithGroupVersionKind(mariaDBUserGroupVersionKind))
	cmp.SetName(bindingID)
	cmp.SetLabels(labels)
	cmp.SetCompositionReference(&corev1.ObjectReference{
		Name: planName,
	})
	if err := fieldpath.Pave(cmp.Object).SetValue(instanceSpecParamsParentReferencePath, parentReference); err != nil {
		return "", err
	}

	msb.logger.Debug("create-binding", lager.Data{"instance": cmp})
	err = msb.cp.client.Create(ctx, cmp)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return "", err
	}
	return string(secret.Data[xrv1.ResourceCredentialsSecretPasswordKey]), nil
}

func mapMariadbEndpoint(data map[string][]byte) (*Endpoint, error) {
	hostBytes, ok := data[xrv1.ResourceCredentialsSecretEndpointKey]
	if !ok {
		return nil, apiresponses.ErrBindingNotFound
	}
	host := string(hostBytes)
	port, err := strconv.Atoi(string(data[xrv1.ResourceCredentialsSecretPortKey]))
	if err != nil {
		return nil, err
	}
	return &Endpoint{
		Host:     host,
		Port:     int32(port),
		Protocol: "tcp",
	}, nil
}

func createCredentials(endpoint *Endpoint, username, password, database, metricsPort string) Credentials {
	uri := fmt.Sprintf("mysql://%s:%s@%s:%d/%s?reconnect=true", username, password, endpoint.Host, endpoint.Port, database)

	creds := Credentials{
		"host":                                endpoint.Host,
		"hostname":                            endpoint.Host,
		xrv1.ResourceCredentialsSecretPortKey: endpoint.Port,
		"name":                                database,
		"database":                            database,
		xrv1.ResourceCredentialsSecretUserKey: username,
		xrv1.ResourceCredentialsSecretPasswordKey: password,
		"database_uri": uri,
		"uri":          uri,
		"jdbcUrl":      fmt.Sprintf("jdbc:mysql://%s:%d/%s?user=%s&password=%s", endpoint.Host, endpoint.Port, database, username, password),
	}

	if metricsPort != "" {
		creds["metrics"] = []string{
			fmt.Sprintf("http://%s:%s/metrics/mariadb-0", endpoint.Host, metricsPort),
			fmt.Sprintf("http://%s:%s/metrics/mariadb-1", endpoint.Host, metricsPort),
			fmt.Sprintf("http://%s:%s/metrics/mariadb-2", endpoint.Host, metricsPort),
		}
	}

	return creds
}
