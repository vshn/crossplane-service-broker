package crossplane

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/fieldpath"
	"github.com/crossplane/crossplane-runtime/pkg/password"
	"github.com/crossplane/crossplane-runtime/pkg/resource/unstructured/composite"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
	groupVersionKind = schema.GroupVersionKind{
		Group:   "syn.tools",
		Version: "v1alpha1",
		Kind:    "CompositeMariaDBUserInstance",
	}
)

// MariadbDatabaseServiceBinder defines a specific Mariadb service with enough data to retrieve connection credentials.
type MariadbDatabaseServiceBinder struct {
	instance  *Instance
	resources []corev1.ObjectReference
	cp        *Crossplane
	logger    lager.Logger
}

// NewMariadbDatabaseServiceBinder instantiates a Mariadb service instance based on the given CompositeMariadbInstance.
func NewMariadbDatabaseServiceBinder(c *Crossplane, instance *Instance, logger lager.Logger) *MariadbDatabaseServiceBinder {
	return &MariadbDatabaseServiceBinder{
		instance:  instance,
		resources: instance.Composite.GetResourceReferences(),
		cp:        c,
		logger:    logger,
	}
}

// Bind creates a MariaDB binding composite.
func (msb MariadbDatabaseServiceBinder) Bind(ctx context.Context, bindingID string) (Credentials, error) {
	parentRef, err := msb.retrieveDBInstance()
	if err != nil {
		return nil, err
	}

	pw, err := msb.createBinding(
		ctx,
		bindingID,
		msb.instance.Labels.InstanceID,
		parentRef,
	)
	if err != nil {
		return nil, err
	}

	// In order to directly return the credentials we need to get the IP/port for this instance.
	secret, err := msb.cp.getCredentials(ctx, parentRef)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err = ErrInstanceNotReady
		}
		return nil, err
	}

	endpoint, err := mapMariadbEndpoint(secret.Data)
	if err != nil {
		return nil, err
	}

	creds := createCredentials(endpoint, bindingID, pw, msb.instance.Composite.GetName())

	return creds, nil
}

// Unbind deletes the created User and Grant.
func (msb MariadbDatabaseServiceBinder) Unbind(ctx context.Context, bindingID string) error {
	cmp := composite.New(composite.WithGroupVersionKind(groupVersionKind))
	cmp.SetName(bindingID)
	if err := msb.cp.client.Delete(ctx, cmp, client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil {
		return err
	}

	// TODO: figure out a better way to delete the password secret
	//       option a) use Watch on resourceRefs of composite and wait until User/Grant are both deleted
	//       option b) https://github.com/crossplane/crossplane/issues/1612 is implemented by crossplane
	// If we delete the secret too quickly, the provider-sql can't deprovision the user
	time.Sleep(5 * time.Second)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf(secretName, bindingID),
			Namespace: msb.cp.namespace,
		},
	}
	return msb.cp.client.Delete(ctx, secret)
}

// Deprovisionable always returns nil for MariadbDatabase instances.
func (msb MariadbDatabaseServiceBinder) Deprovisionable(ctx context.Context) error {
	return nil
}

// GetBinding returns credentials for MariaDB
func (msb MariadbDatabaseServiceBinder) GetBinding(ctx context.Context, bindingID string) (Credentials, error) {
	secret, err := msb.cp.getCredentials(ctx, bindingID)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			err = ErrInstanceNotReady
		}
		return nil, err
	}

	endpoint, err := mapMariadbEndpoint(secret.Data)
	if err != nil {
		return nil, err
	}

	pw := string(secret.Data[xrv1.ResourceCredentialsSecretPasswordKey])
	creds := createCredentials(endpoint, bindingID, pw, msb.instance.Composite.GetName())

	return creds, nil
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
			Namespace: msb.cp.namespace,
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

	cmp := composite.New(composite.WithGroupVersionKind(groupVersionKind))
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

func (msb MariadbDatabaseServiceBinder) retrieveDBInstance() (string, error) {
	parentReference, err := fieldpath.Pave(msb.instance.Composite.Object).GetString(instanceSpecParamsParentReferencePath)
	if err != nil {
		return "", err
	}
	return parentReference, nil
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

func createCredentials(endpoint *Endpoint, username, password, database string) Credentials {
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

	return creds
}
