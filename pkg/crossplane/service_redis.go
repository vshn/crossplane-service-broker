package crossplane

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"code.cloudfoundry.org/lager"
	xrv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/pivotal-cf/brokerapi/v8/domain/apiresponses"
)

var _ ProvisionValidater = &RedisServiceBinder{}

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

	caCert, ok := s.Data["ca.crt"]
	if !ok {
		// Only redis 7 has that field.
		// So we don't fail if it doesn't exist.
		caCert = []byte("")
	}

	cn := rsb.instance.GetClusterName()
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
		"ca.crt": string(caCert),
	}
	if rsb.cp.config.EnableMetrics {
		creds["metricsEndpoints"] = []string{
			fmt.Sprintf("http://%s.%s.%s", rsb.instance.ID(), cn, rsb.cp.config.MetricsDomain),
			fmt.Sprintf("http://%s.%s.%s/redis/0", rsb.instance.ID(), cn, rsb.cp.config.MetricsDomain),
			fmt.Sprintf("http://%s.%s.%s/redis/1", rsb.instance.ID(), cn, rsb.cp.config.MetricsDomain),
			fmt.Sprintf("http://%s.%s.%s/redis/2", rsb.instance.ID(), cn, rsb.cp.config.MetricsDomain),
		}
	}

	return creds, nil
}

// ValidateProvisionParams doesn't currently validate anything, it will simply take the params and convert them to
// a map. This is because there are multiple Redis implementations, one has parameters and the other doesn't.
func (rsb *RedisServiceBinder) ValidateProvisionParams(_ context.Context, params json.RawMessage) (map[string]interface{}, error) {
	validatedParams := map[string]any{}

	err := json.Unmarshal(params, &validatedParams)
	if err != nil {
		return validatedParams, fmt.Errorf("cannot unmarshal parameters: %w", err)
	}

	return validatedParams, nil
}
