package crossplane

import (
	"context"
	"fmt"
	"strconv"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pivotal-cf/brokerapi/v8/domain/apiresponses"
)

// SentinelPortKey is the key in the connection secret that contains the port to Redis Sentinel
const SentinelPortKey = "sentinelPort"

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
	sentinelPort, err := strconv.Atoi(string(s.Data[SentinelPortKey]))
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
	mp := string(s.Data[MetricsPortKey])
	if mp != "" {
		creds["metrics"] = []string{
			fmt.Sprintf("http://%s:%s/metrics/redis-0/metrics", endpoint, mp),
			fmt.Sprintf("http://%s:%s/metrics/redis-1/metrics", endpoint, mp),
			fmt.Sprintf("http://%s:%s/metrics/redis-2/metrics", endpoint, mp),
		}
	}

	return creds, nil
}
