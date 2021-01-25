package crossplane

import (
	"context"
	"errors"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
)

// RedisServiceBinder defines a specific redis service with enough data to retrieve connection credentials.
type RedisServiceBinder struct {
	instanceID string
	resources  []corev1.ObjectReference
	cp         *Crossplane
	logger     lager.Logger
}

// NewRedisServiceBinder instantiates a redis service instance based on the given CompositeRedisInstance.
func NewRedisServiceBinder(c *Crossplane, instance *Instance, logger lager.Logger) *RedisServiceBinder {
	return &RedisServiceBinder{
		instanceID: instance.Composite.GetName(),
		resources:  instance.Composite.GetResourceReferences(),
		cp:         c,
		logger:     logger,
	}
}

// Bind retrieves the necessary external IP, password and ports.
func (rsb RedisServiceBinder) Bind(ctx context.Context, bindingID string) (Credentials, error) {
	return rsb.GetBinding(ctx, bindingID)
}

// Unbind does nothing for redis bindings.
func (rsb RedisServiceBinder) Unbind(_ context.Context, _ string) error {
	return nil
}

// Deprovision removes the downstream namespace.
func (rsb RedisServiceBinder) Deprovision(ctx context.Context) error {
	return markNamespaceDeleted(ctx, rsb.cp, rsb.instanceID, rsb.resources)
}

// GetBinding always returns the same credentials for Redis
func (rsb RedisServiceBinder) GetBinding(ctx context.Context, _ string) (Credentials, error) {
	creds := make(Credentials)

	secrets := findResourceRefs(rsb.resources, "Secret")
	if len(secrets) != 1 {
		return nil, errors.New("resourceRef contains more than one secret")
	}
	sr := NewSecretResource(rsb.cp.namespace, secrets[0], rsb.cp)
	sc, err := sr.GetCredentials(ctx)
	if err != nil {
		return nil, err
	}
	creds[xrv1.ResourceCredentialsSecretPasswordKey] = sc.(*SecretCredentials).Password

	// FIXME(mw): adjust to updated helm-provider (to retrieve endpoint etc.)

	return creds, nil
}
