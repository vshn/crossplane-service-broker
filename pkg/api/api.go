// Package API exposes an http.Handler which registers all routes necessary to act as an OSB-compatible router.
// It's the main entrypoint to the public API of crossplace-service-broker.
package api

import (
	"io"
	"net/http"

	"code.cloudfoundry.org/lager"
	"github.com/gorilla/mux"
	"github.com/pivotal-cf/brokerapi/v7"
	"github.com/pivotal-cf/brokerapi/v7/domain"
	"github.com/pivotal-cf/brokerapi/v7/middlewares"
)

type API struct {
	r      *mux.Router
	logger lager.Logger
}

func New(sb domain.ServiceBroker, username, password string, logger lager.Logger) *API {
	router := mux.NewRouter()

	router.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"status": "ok"}`)
	}).Methods(http.MethodGet)

	osbRoutes := brokerapi.New(sb, logger, brokerapi.BrokerCredentials{Username: username, Password: password})

	router.Use(loggerMiddleware(logger))

	router.NewRoute().Handler(osbRoutes)

	return &API{router, logger}
}

func (a *API) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	a.r.ServeHTTP(w, req)
}

func loggerMiddleware(logger lager.Logger) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			id, ok := req.Context().Value(middlewares.CorrelationIDKey).(string)
			if !ok {
				id = "unknown"
			}
			headers := req.Header.Clone()
			if auth := headers.Get("Authorization"); auth != "" {
				headers.Set("Authorization", "****")
			}
			logger.WithData(lager.Data{
				"correlation-id": id,
				"headers":        headers,
				"URI":            req.RequestURI,
				"method":         req.Method,
			}).Debug("debug-headers")
			next.ServeHTTP(w, req)
		})
	}
}
