package crossplane

import (
	"context"
	"fmt"
	"strconv"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pivotal-cf/brokerapi/v7/domain/apiresponses"
)

// RedisServiceBinder defines a specific redis service with enough data to retrieve connection credentials.
type RedisServiceBinder struct {
	serviceBinder
}

// NewRedisServiceBinder instantiates a redis service instance based on the given CompositeRedisInstance.
func NewRedisServiceBinder(c *Crossplane, instance *Instance, logger lager.Logger) *RedisServiceBinder {
	return &RedisServiceBinder{
		serviceBinder: serviceBinder{
			instance: instance,
			cp:       c,
			logger:   logger,
		},
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
func (rsb RedisServiceBinder) GetBinding(ctx context.Context, bindingID string) (Credentials, error) {
	s, err := rsb.cp.GetConnectionDetails(ctx, rsb.instance.Composite)
	if err != nil {
		return nil, err
	}

	endpointBytes, ok := s.Data[xrv1.ResourceCredentialsSecretEndpointKey]
	if !ok {
		return nil, apiresponses.ErrBindingNotFound
	}
	endpoint := string(endpointBytes)
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
		"master":   fmt.Sprintf("redis://%s", rsb.instance.ID()),
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
