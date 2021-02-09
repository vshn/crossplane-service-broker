// Package api exposes an http.Handler which registers all routes necessary to act as an OSB-compatible router.
// It's the main entrypoint to the public API of crossplace-service-broker.
package api

import (
	"io"
	"net/http"

	"code.cloudfoundry.org/lager"
	"github.com/gorilla/mux"
	"github.com/pivotal-cf/brokerapi/v7"
	"github.com/pivotal-cf/brokerapi/v7/domain"

	"github.com/vshn/crossplane-service-broker/pkg/reqcontext"
)

// API is a http.Handler
type API struct {
	r      *mux.Router
	logger lager.Logger
}

// New creates a new API
func New(sb domain.ServiceBroker, username, password string, logger lager.Logger) *API {
	router := mux.NewRouter()

	router.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"status": "ok"}`)
	}).Methods(http.MethodGet)

	osbRoutes := brokerapi.New(sb, logger, brokerapi.BrokerCredentials{Username: username, Password: password})

	osbRoutes.(*mux.Router).Use(LoggerMiddleware(logger))

	router.NewRoute().Handler(osbRoutes)

	return &API{router, logger}
}

func (a *API) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	a.r.ServeHTTP(w, req)
}

// LoggerMiddleware logs a debug log with headers.
func LoggerMiddleware(logger lager.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			rctx := reqcontext.NewReqContext(req.Context(), logger, nil)

			headers := req.Header.Clone()
			if auth := headers.Get("Authorization"); auth != "" {
				headers.Set("Authorization", "****")
			}
			rctx.Logger.Debug("debug-headers", lager.Data{
				"headers": headers,
				"URI":     req.RequestURI,
				"method":  req.Method,
			})
			next.ServeHTTP(w, req)
		})
	}
}
