package auth

import (
	"fmt"
	"net/http"

	"github.com/pascaldekloe/jwt"
)

// Principal represents the entity (person or system) in who's name the current request is made.
// It does not represent the _origin_ of the request.
// For example, a system might send a request in the name of a person.
type Principal struct {
	Name string
}

// GetPrincipal extracts the Principal on who's name the given http.Request was made based on the information
// that was retrieved with the http.Request in the Authorization HTTP header.
func GetPrincipal(req *http.Request) (Principal, error) {
	authenticationMethod := req.Context().Value(AuthenticationMethodPropertyName)
	if authenticationMethod == nil {
		return Principal{}, fmt.Errorf("likely an unauthorized request")
	}

	authenticationMethodStr, ok := authenticationMethod.(string)
	if !ok {
		return Principal{}, fmt.Errorf("can't convert authorization method to string")
	}

	var reader principalReader
	switch {
	case authenticationMethodStr == AuthorizationMethodBasicAuth:
		reader = getPrincipalFromBasicAuth
	case authenticationMethodStr == AuthorizationMethodBearerToken:
		reader = getPrincipalFromBearerToken
	default:
		return Principal{}, fmt.Errorf("unknown authorication format '%s'", authenticationMethodStr)
	}

	return reader(req)
}

// getPrincipalFromBearerToken extracts the Principal on who's name the given http.Request was made based on the
// Bearer token that was retrieved with the http.Request in the Authorization HTTP header.
func getPrincipalFromBearerToken(req *http.Request) (Principal, error) {
	token := req.Context().Value(TokenPropertyName)
	if token == nil {
		return Principal{}, fmt.Errorf("principal token not set")
	}

	claim, ok := token.(*jwt.Claims)
	if !ok {
		return Principal{}, fmt.Errorf("principal token is invalid")
	}

	claimName := "username" // TODO move hardcoded to configuration
	username, ok := claim.String(claimName)
	if !ok {
		return Principal{}, fmt.Errorf("principal token contains no claim named '%s'", claimName)
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
