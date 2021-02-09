package crossplane

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	corev1 "k8s.io/api/core/v1"
)

// RedisServiceBinder defines a specific redis service with enough data to retrieve connection credentials.
type RedisServiceBinder struct {
	instanceID   string
	resourceRefs []corev1.ObjectReference
	cp           *Crossplane
	logger       lager.Logger
}

// NewRedisServiceBinder instantiates a redis service instance based on the given CompositeRedisInstance.
func NewRedisServiceBinder(c *Crossplane, id string, resourceRefs []corev1.ObjectReference, logger lager.Logger) *RedisServiceBinder {
	return &RedisServiceBinder{
		instanceID:   id,
		resourceRefs: resourceRefs,
		cp:           c,
		logger:       logger,
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

// Deprovisionable returns always nil for redis instances.
func (rsb RedisServiceBinder) Deprovisionable(ctx context.Context) error {
	return nil
}

// GetBinding always returns the same credentials for Redis
func (rsb RedisServiceBinder) GetBinding(ctx context.Context, _ string) (Credentials, error) {
	secrets := findResourceRefs(rsb.resourceRefs, "Secret")
	if len(secrets) != 1 {
		return nil, errors.New("resourceRef contains more than one secret")
	}
	s, err := rsb.cp.getCredentials(ctx, secrets[0].Name)
	if err != nil {
		return nil, err
	}

	endpoint := string(s.Data[xrv1.ResourceCredentialsSecretEndpointKey])
	port, err := strconv.Atoi(string(s.Data[xrv1.ResourceCredentialsSecretPortKey]))
	if err != nil {
		return nil, err
	}
	sentinelPort, err := strconv.Atoi(string(s.Data["sentinelPort"]))
	if err != nil {
		return nil, err
	}
	creds := Credentials{
		"password": string(s.Data[xrv1.ResourceCredentialsSecretPasswordKey]),
		"host":     endpoint,
		"port":     port,
		"master":   fmt.Sprintf("redis://%s", rsb.instanceID),
		"sentinels": []Credentials{
			{
				"host": endpoint,
				"port": sentinelPort,
			},
		},
		"servers": []Credentials{
			{
				"host": endpoint,
				"port": port,
			},
		},
	}

	return creds, nil
}
