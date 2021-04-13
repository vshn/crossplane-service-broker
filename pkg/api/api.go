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
	"github.com/pivotal-cf/brokerapi/v8/middlewares"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/vshn/crossplane-service-broker/pkg/api/auth"
	"github.com/vshn/crossplane-service-broker/pkg/reqcontext"
)

// API is a http.Handler
type API struct {
	r      *mux.Router
	logger lager.Logger
}

var (
	totalRequestCounter = promauto.NewCounter(prometheus.CounterOpts{
		Name: "osb_total_broker_api_requests_total",
		Help: "The total number of processed service broker api requests, not including healthz and metrics requests.",
	})
)

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
	sbRoutes := NewBrokerAPI(sb, logger, serviceBrokerAuthMiddleware.Handler)

	sbRouter := sbRoutes.(*mux.Router)
	sbRouter.Use(LoggerMiddleware(logger))

	rootRouter.NewRoute().Handler(sbRoutes)

	return &API{rootRouter, logger}
}

// RequestCounterMiddleware increases the respective OpenMetrics counter by one for each request it is invoked.
func RequestCounterMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		totalRequestCounter.Inc()
		next.ServeHTTP(w, r)
	})
}

// NewBrokerAPI initializes the BrokerAPI.
// It's copied from [0] and then modified under the terms of it's Apache 2.0 license [1].
// See issue [2] whether this function can already be replaced by an upstream implementation.
//
// [0] https://github.com/pivotal-cf/brokerapi/blob/813290b2c03139287314af01af39deb355931e30/api.go#L33-L49
// [1] https://github.com/pivotal-cf/brokerapi/blob/813290b2c03139287314af01af39deb355931e30/api.go#L3-L14
// [2] https://github.com/pivotal-cf/brokerapi/issues/158
//goland:noinspection GoDeprecation
func NewBrokerAPI(serviceBroker brokerapi.ServiceBroker, logger lager.Logger, authMiddleware mux.MiddlewareFunc) http.Handler {
	router := mux.NewRouter()

	brokerapi.AttachRoutes(router, serviceBroker, logger)

	apiVersionMiddleware := middlewares.APIVersionMiddleware{LoggerFactory: logger}

	router.Use(middlewares.AddCorrelationIDToContext)
	router.Use(RequestCounterMiddleware)
	router.Use(authMiddleware)
	router.Use(middlewares.AddOriginatingIdentityToContext)
	router.Use(apiVersionMiddleware.ValidateAPIVersionHdr)
	router.Use(middlewares.AddInfoLocationToContext)
	router.Use(middlewares.AddRequestIdentityToContext)

	return router
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
