// This file is based on code initially published at
// https://github.com/pivotal-cf/brokerapi/blob/main/auth/auth.go.

package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
)

// Basic represents a mux middleware that, given a http.Request, checks whether the Authorization header
// contains any of the given Credentials.
type Basic struct {
	Credentials []Credential
}

// Credential represents one user's ``username and password'' combination.
type Credential struct {
	username []byte
	password []byte
}

// UserPropertyName allows to query the HTTP context for the current user's name.
// See Context() on http.Request.
//
//   func(w http.ResponseWriter, r *http.Request) {
//     user := r.Context().Value(auth.UserPropertyName);
//     fmt.Fprintf(w, "This is an authenticated request")
//     fmt.Fprintf(w, "User name: '%s'\n", user)
//   }
const UserPropertyName contextKey = "user"

// NewCredential turns a username and a password into a Credential.
// The username and the password are hashed using sha256.Sum256,
// so that they can not be extracted from memory or a core dump or likewise.
func NewCredential(username, password string) Credential {
	u := sha256.Sum256([]byte(username))
	p := sha256.Sum256([]byte(password))
	return Credential{username: u[:], password: p[:]}
}

// SingleCredential is a short-hand function to create a list of Credentials with just one Credential in it.
func SingleCredential(username, password string) []Credential {
	return []Credential{NewCredential(username, password)}
}

// Handler represents a mux.MiddlewareFunc
func (b Basic) Handler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r, ok := b.authorized(r)
		if !ok {
			failedAuthenticationCounter.Inc()
			w.Header().Set("WWW-Authenticate", "Basic")
			http.Error(w, "Not Authorized", http.StatusUnauthorized)
			return
		}

		handler.ServeHTTP(w, r)
	})
}

// Authorized returns true if the given request's Authorization header contains any of the configured Credentials.
// If true, the context.Context of the returned http.Request is modified and the username of the user
// can be fetched using the key UserPropertyName.
func (b Basic) authorized(r *http.Request) (*http.Request, bool) {
	username, password, isOk := r.BasicAuth()
	if isOk {
		u := sha256.Sum256([]byte(username))
		p := sha256.Sum256([]byte(password))
		for _, c := range b.Credentials {
			if c.isAuthorized(u, p) {
				ctx := context.WithValue(r.Context(), UserPropertyName, username)
				return r.WithContext(ctx), true
			}
		}
	}
	return r, false
}

func (c Credential) isAuthorized(uChecksum [32]byte, pChecksum [32]byte) bool {
	return subtle.ConstantTimeCompare(c.username, uChecksum[:]) == 1 &&
		subtle.ConstantTimeCompare(c.password, pChecksum[:]) == 1
}
