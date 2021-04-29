package auth

import (
	"fmt"
	"net/http"

	"github.com/pascaldekloe/jwt"
	"github.com/vshn/crossplane-service-broker/pkg/config"
)

// Principal represents the entity (person or system) in who's name the current request is made.
// It does not represent the _origin_ of the request.
// For example, a system might send a request in the name of a person.
type Principal struct {
	Name string
}

// FromRequest extracts the Principal on who's name the given http.Request was made based on the information
// that was retrieved with the http.Request in the Authorization HTTP header.
func FromRequest(cfg *config.Config, req *http.Request) (Principal, error) {
	authenticationMethod := req.Context().Value(AuthenticationMethodPropertyName)
	if authenticationMethod == nil {
		return Principal{}, fmt.Errorf("likely an unauthorized request")
	}

	authenticationMethodStr, ok := authenticationMethod.(string)
	if !ok {
		return Principal{}, fmt.Errorf("can't convert authorization method to string")
	}

	switch {
	case authenticationMethodStr == AuthorizationMethodBasicAuth:
		return getPrincipalFromBasicAuth(req)
	case authenticationMethodStr == AuthorizationMethodBearerToken:
		return getPrincipalFromBearerToken(cfg, req)
	default:
		return Principal{}, fmt.Errorf("unknown authorication format '%s'", authenticationMethodStr)
	}
}

// getPrincipalFromBearerToken extracts the Principal on who's name the given http.Request was made based on the
// Bearer token that was retrieved with the http.Request in the Authorization HTTP header.
func getPrincipalFromBearerToken(cfg *config.Config, req *http.Request) (Principal, error) {
	token := req.Context().Value(TokenPropertyName)
	if token == nil {
		return Principal{}, fmt.Errorf("principal token not set")
	}

	claim, ok := token.(*jwt.Claims)
	if !ok {
		return Principal{}, fmt.Errorf("principal token is invalid")
	}

	username, ok := claim.String(cfg.UsernameClaim)
	if !ok {
		return Principal{}, fmt.Errorf("principal token contains no claim named '%s'", cfg.UsernameClaim)
	}

	return Principal{Name: username}, nil
}

// getPrincipalFromBasicAuth extracts the Principal on who's name the given http.Request was made based on the
// Basic auth username that was retrieved with the http.Request in the Authorization HTTP header.
func getPrincipalFromBasicAuth(req *http.Request) (Principal, error) {
	userName := req.Context().Value(UserPropertyName)
	if userName == nil {
		return Principal{}, fmt.Errorf("principal not set")
	}

	userNameStr, ok := userName.(string)
	if !ok {
		return Principal{}, fmt.Errorf("principal is not a string")
	}
	return Principal{Name: userNameStr}, nil
}
