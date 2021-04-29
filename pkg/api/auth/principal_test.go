package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vshn/crossplane-service-broker/pkg/config"

	"github.com/pascaldekloe/jwt"
)

var emptyEnv = map[string]string{
	config.EnvServiceIDs: "test",
	config.EnvUsername:   "test",
	config.EnvPassword:   "test",
	config.EnvNamespace:  "test",
}

func Test_BasicPrincipal(t *testing.T) {
	cfg := givenConfiguration(t, emptyEnv)
	givenUsername := "expectedUsername"
	ctx := givenAuthenticatedContext(AuthenticationMethodBasicAuth, UserPropertyName, givenUsername)

	actualPrincipal, err := PrincipalFromContext(ctx, cfg)

	assert.NoError(t, err)
	assert.Equal(t, givenUsername, string(actualPrincipal))
}

func Test_BearerTokenPrincipal(t *testing.T) {
	cfg := givenConfiguration(t, emptyEnv)
	givenUsername := "expectedUsername"
	givenToken := jwt.Claims{
		Set: map[string]interface{}{
			cfg.UsernameClaim: givenUsername,
		},
	}
	ctx := givenAuthenticatedContext(AuthenticationMethodBearerToken, TokenPropertyName, &givenToken)

	actualPrincipal, err := PrincipalFromContext(ctx, cfg)

	assert.NoError(t, err)
	assert.Equal(t, givenUsername, string(actualPrincipal))
}

func givenConfiguration(t *testing.T, env map[string]string) *config.Config {
	cfg, err := config.ReadConfig(func(s string) string {
		return env[s]
	})
	require.NoError(t, err)
	return cfg
}

func givenAuthenticatedContext(authMethod interface{}, expectedPrincipalKey interface{}, expectedPrincipalValue interface{}) context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, AuthenticationMethodPropertyName, authMethod)
	ctx = context.WithValue(ctx, expectedPrincipalKey, expectedPrincipalValue)
	return ctx
}
