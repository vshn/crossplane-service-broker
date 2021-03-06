// Package api exposes an http.Handler which registers all routes necessary to act as an OSB-compatible router.
// It's the main entrypoint to the public API of crossplace-service-broker.
package api

import (
	"io"
	"net/http"

	"code.cloudfoundry.org/lager"
	"github.com/gorilla/mux"
	"github.com/pascaldekloe/jwt"
	"github.com/pivotal-cf/brokerapi/v8"
	"github.com/pivotal-cf/brokerapi/v8/domain"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/vshn/crossplane-service-broker/pkg/api/auth"
	"github.com/vshn/crossplane-service-broker/pkg/reqcontext"
)

// API is a http.Handler
type API struct {
	r      *mux.Router
	logger lager.Logger
}

// New creates a new API
func New(sb domain.ServiceBroker, brokerCredentials []auth.Credential, jwtSigningKeys *jwt.KeyRegister, logger lager.Logger) *API {
	rootRouter := mux.NewRouter()

	rootRouter.
		HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, `{"status": "ok"}`)
		}).
		Methods(http.MethodGet)

	rootRouter.
		Handle("/metrics", promhttp.Handler()).
		Methods(http.MethodGet)

	serviceBrokerAuthMiddleware := auth.New(brokerCredentials, jwtSigningKeys)
	sbRoutes := brokerapi.NewWithCustomAuth(sb, logger, serviceBrokerAuthMiddleware.Handler)

	sbRouter := sbRoutes.(*mux.Router)
	sbRouter.Use(LoggerMiddleware(logger))

	rootRouter.NewRoute().Handler(sbRoutes)

	return &API{rootRouter, logger}
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
			if a := headers.Get("Authorization"); a != "" {
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
