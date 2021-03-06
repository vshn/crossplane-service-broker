package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/pascaldekloe/jwt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type authenticationHandler interface {
	Handler(handler http.Handler) http.Handler
}

// AuthenticationMiddleware represents a mux middleware which, given an http.Request, can check whether
// it's Authorization header contains a valid BearerToken or – if not – valid Basic Auth information.
type AuthenticationMiddleware struct {
	bearerTokenAuth authenticationHandler
	basicAuth       authenticationHandler
}

type contextKey string

const (
	// AuthenticationMethodPropertyName allows to query the HTTP context for the authentication method used.
	// See AuthenticationMethodBearerToken and AuthenticationMethodBasicAuth,
	// as well as Context() on http.Request.
	AuthenticationMethodPropertyName contextKey = "authentication-method"

	// AuthenticationMethodBearerToken should be used to compare with the value returned by AuthenticationMethodPropertyName.
	//
	//   func(w http.ResponseWriter, r *http.Request) {
	//     method := r.Context().Value(auth.UserPropertyName);
	//     if method == auth.AuthenticationMethodBearerToken {
	//       // Bearer Token auth
	//     }
	//   }
	AuthenticationMethodBearerToken = "Bearer"

	// AuthenticationMethodBasicAuth should be used to compare with the value returned by AuthenticationMethodPropertyName:
	//
	//   func(w http.ResponseWriter, r *http.Request) {
	//     method := r.Context().Value(auth.UserPropertyName);
	//     if method == auth.AuthenticationMethodBasicAuth {
	//       // Basic auth
	//     }
	//   }
	AuthenticationMethodBasicAuth = "Basic"
)

var (
	totalRequestCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "osb_total_broker_api_requests_total",
		Help: "The total number of processed service broker api requests, not including healthz and metrics requests.",
	})
	failedAuthenticationCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "osb_failed_authentication_attempts_total",
		Help: "The total number of failed authentication attempts.",
	})
)

// New returns a new and completely initialized AuthenticationMiddleware.
func New(credentials []Credential, keys *jwt.KeyRegister) AuthenticationMiddleware {
	bearerToken := &BearerToken{
		Keys: keys,
	}
	basic := Basic{
		Credentials: credentials,
	}

	return AuthenticationMiddleware{
		bearerTokenAuth: bearerToken,
		basicAuth:       basic,
	}
}

// Handler represents a mux.MiddlewareFunc
func (a AuthenticationMiddleware) Handler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		totalRequestCounter.Inc()

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			unauthorized(w)
			return
		}

		authHeaderParts := strings.Fields(authHeader)
		if len(authHeaderParts) < 2 {
			badRequest(w)
			return
		}

		authMethod := authHeaderParts[0]
		switch {
		case authMethod == "Bearer":
			a.handleBearerToken(w, r, handler)
			return
		case authMethod == "Basic":
			a.handleBasicAuth(w, r, handler)
			return
		default:
			badRequest(w)
			return
		}
	})
}

func (a AuthenticationMiddleware) handleBasicAuth(w http.ResponseWriter, r *http.Request, handler http.Handler) {
	ctx := context.WithValue(r.Context(), AuthenticationMethodPropertyName, AuthenticationMethodBasicAuth)
	a.basicAuth.Handler(handler).ServeHTTP(w, r.WithContext(ctx))
}

func (a AuthenticationMiddleware) handleBearerToken(w http.ResponseWriter, r *http.Request, handler http.Handler) {
	ctx := context.WithValue(r.Context(), AuthenticationMethodPropertyName, AuthenticationMethodBearerToken)
	a.bearerTokenAuth.Handler(handler).ServeHTTP(w, r.WithContext(ctx))
}

func unauthorized(w http.ResponseWriter) {
	failedAuthenticationCounter.Inc()
	w.Header().Set("WWW-Authenticate", "Basic realm=\"Crossplane Service Broker\", charset=\"UTF-8\"")
	http.Error(w, "Not Authorized", http.StatusUnauthorized)
}

func badRequest(w http.ResponseWriter) {
	http.Error(w, "Unable to determine Authorization method, supported are 'Basic' and 'Bearer'.", http.StatusBadRequest)
}
