package auth

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pascaldekloe/jwt"
)

func Test_BasicPrincipal(t *testing.T) {
	givenUsername := "expectedUsername"
	req := givenAuthenticatedRequest(t, AuthorizationMethodBasicAuth, UserPropertyName, givenUsername)

	actualPrincipal, err := GetPrincipal(req)

	assert.NoError(t, err)
	assert.Equal(t, givenUsername, actualPrincipal.Name)
}

func Test_BearerTokenPrincipal(t *testing.T) {
	givenUsername := "expectedUsername"
	givenToken := jwt.Claims{
		Set: map[string]interface{}{
			"username": givenUsername, // TODO get/set claim name in configuration
		},
	}
	req := givenAuthenticatedRequest(t, AuthorizationMethodBearerToken, TokenPropertyName, &givenToken)

	actualPrincipal, err := GetPrincipal(req)

	assert.NoError(t, err)
	assert.Equal(t, givenUsername, actualPrincipal.Name)
}

func givenAuthenticatedRequest(t *testing.T, authMethod interface{}, expectedPrincipalKey interface{}, expectedPrincipalValue interface{}) *http.Request {
	req, err := http.NewRequest("GET", "/whatever", &bytes.Buffer{})
	require.NoError(t, err)
	req = req.WithContext(context.WithValue(req.Context(), AuthenticationMethodPropertyName, authMethod))
	req = req.WithContext(context.WithValue(req.Context(), expectedPrincipalKey, expectedPrincipalValue))
	return req
}
