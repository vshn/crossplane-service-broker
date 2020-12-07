// Package API exposes an http.Handler which registers all routes necessary to act as an OSB-compatible router.
// It's the main entrypoint to the public API of crossplace-service-broker.
package api

import (
	"net/http"

	"code.cloudfoundry.org/lager"
	"github.com/gorilla/mux"
)

type API struct {
	r  *mux.Router
	logger lager.Logger
}

func New(logger lager.Logger) *API {
	router := mux.NewRouter()

	router.HandleFunc("/healthz", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(200)
	})

	return &API{router, logger}
}

func (a *API) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	a.r.ServeHTTP(w, req)
}
