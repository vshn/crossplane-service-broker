package auth

import (
	"context"
	"fmt"

	"github.com/pascaldekloe/jwt"
	"github.com/vshn/crossplane-service-broker/pkg/config"
)

// Principal represents the entity (person or system) in who's name the current request is made.
// It does not represent the _origin_ of the request.
// For example, a system might send a request in the name of a person.
type Principal string

// PrincipalFromContext extracts the Principal on whose name the given http.Request was made based on the information
// that was retrieved with the http.Request in the Authorization HTTP header.
func PrincipalFromContext(ctx context.Context, cfg *config.Config) (Principal, error) {
	authenticationMethod := ctx.Value(AuthenticationMethodPropertyName)
	if authenticationMethod == nil {
		return "", fmt.Errorf("likely an unauthorized request")
	}

	authenticationMethodStr, ok := authenticationMethod.(string)
	if !ok {
		return "", fmt.Errorf("can't convert authorization method to string")
	}

	switch {
	case authenticationMethodStr == AuthenticationMethodBasicAuth:
		return getPrincipalFromBasicAuth(ctx)
	case authenticationMethodStr == AuthenticationMethodBearerToken:
		return getPrincipalFromBearerToken(ctx, cfg)
	default:
		return "", fmt.Errorf("unknown authorication format '%s'", authenticationMethodStr)
	}
}

// getPrincipalFromBearerToken extracts the Principal on whose name the given http.Request was made based on the
// Bearer token that was retrieved with the http.Request in the Authorization HTTP header.
func getPrincipalFromBearerToken(ctx context.Context, cfg *config.Config) (Principal, error) {
	token := ctx.Value(TokenPropertyName)
	if token == nil {
		return "", fmt.Errorf("principal token not set")
	}

	claim, ok := token.(*jwt.Claims)
	if !ok {
		return "", fmt.Errorf("principal token is invalid")
	}

	username, ok := claim.String(cfg.UsernameClaim)
	if !ok {
		return "", fmt.Errorf("principal token contains no claim named '%s'", cfg.UsernameClaim)
	}

	return Principal(username), nil
}

// getPrincipalFromBasicAuth extracts the Principal on whose name the given http.Request was made based on the
// Basic auth username that was retrieved with the http.Request in the Authorization HTTP header.
func getPrincipalFromBasicAuth(ctx context.Context) (Principal, error) {
	userName := ctx.Value(UserPropertyName)
	if userName == nil {
		return "", fmt.Errorf("principal not set")
	}

	userNameStr, ok := userName.(string)
	if !ok {
		return "", fmt.Errorf("principal is not a string")
	}
	return Principal(userNameStr), nil
}
