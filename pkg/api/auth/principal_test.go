package auth

import (
	"bytes"
	"context"
	"net/http"
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
	req := givenAuthenticatedRequest(t, AuthorizationMethodBasicAuth, UserPropertyName, givenUsername)

	actualPrincipal, err := FromRequest(cfg, req)

	assert.NoError(t, err)
	assert.Equal(t, givenUsername, actualPrincipal.Name)
}

func Test_BearerTokenPrincipal(t *testing.T) {
	cfg := givenConfiguration(t, emptyEnv)
	givenUsername := "expectedUsername"
	givenToken := jwt.Claims{
		Set: map[string]interface{}{
			cfg.UsernameClaim: givenUsername,
		},
	}
	req := givenAuthenticatedRequest(t, AuthorizationMethodBearerToken, TokenPropertyName, &givenToken)

	actualPrincipal, err := FromRequest(cfg, req)

	assert.NoError(t, err)
	assert.Equal(t, givenUsername, actualPrincipal.Name)
}

func givenConfiguration(t *testing.T, env map[string]string) *config.Config {
	cfg, err := config.ReadConfig(func(s string) string {
		return env[s]
	})
	require.NoError(t, err)
	return cfg
}

func givenAuthenticatedRequest(t *testing.T, authMethod interface{}, expectedPrincipalKey interface{}, expectedPrincipalValue interface{}) *http.Request {
	req, err := http.NewRequest("GET", "/whatever", &bytes.Buffer{})
	require.NoError(t, err)
	req = req.WithContext(context.WithValue(req.Context(), AuthenticationMethodPropertyName, authMethod))
	req = req.WithContext(context.WithValue(req.Context(), expectedPrincipalKey, expectedPrincipalValue))
	return req
}
