package auth

import (
	"net/http"

	"github.com/pascaldekloe/jwt"
)

// BearerToken represents a mux middleware that, given a http.Request, checks whether the Authorization header
// contains any JWT tokens and validates them with the configured Keys.
type BearerToken struct {
	Keys *jwt.KeyRegister
}

// TokenPropertyName allows to query the HTTP context for the current user's JWT token.
// See Context() on http.Request.
//
//   func(w http.ResponseWriter, r *http.Request) {
//     claims := req.Context().Value(auth.TokenPropertyName).(*jwt.Claims)
//     if n, ok := claims.Number("deadline"); !ok {
//       fmt.Fprintln(w, "no deadline")
//     } else {
//       fmt.Fprintln(w, "deadline at", (*jwt.NumericTime)(&n))
//     }
//   }
const TokenPropertyName = "bearer-token"

// Handler represents a mux.MiddlewareFunc
func (t *BearerToken) Handler(handler http.Handler) http.Handler {
	if t.Keys == nil { // things to terribly wrong if Keys is nil
		t.Keys = &jwt.KeyRegister{}
	}

	return &jwt.Handler{
		Target:     handler,
		Keys:       t.Keys,
		ContextKey: TokenPropertyName,
	}
}
