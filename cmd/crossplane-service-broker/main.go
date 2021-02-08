package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/gorilla/mux"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/vshn/crossplane-service-broker/pkg/api"
	"github.com/vshn/crossplane-service-broker/pkg/brokerapi"
	"github.com/vshn/crossplane-service-broker/pkg/config"
)

const (
	exitCodeErr = 1
)

func main() {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)

	logger := lager.NewLogger("crossplane-service-broker")
	logger.RegisterSink(lager.NewPrettySink(os.Stdout, lager.DEBUG))

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	defer func() {
		signal.Stop(signalChan)
		cancel()
	}()

	if err := run(signalChan, logger); err != nil {
		logger.Error("application  run failed", err)
		os.Exit(exitCodeErr)
	}
}

func run(signalChan chan os.Signal, logger lager.Logger) error {
	cfg, err := config.ReadConfig(os.Getenv)
	if err != nil {
		return fmt.Errorf("unable to read app env: %w", err)
	}
	rConfig, err := clientcmd.BuildConfigFromFlags("", cfg.Kubeconfig)
	if err != nil {
		return fmt.Errorf("unable to load k8s REST config: %w", err)
	}

	router := mux.NewRouter()

	b, err := brokerapi.New(cfg.ServiceIDs, cfg.Namespace, rConfig, logger.WithData(lager.Data{"component": "brokerapi"}))
	if err != nil {
		return err
	}

	a := api.New(b, cfg.Username, cfg.Password, logger.WithData(lager.Data{"component": "api"}))
	router.NewRoute().Handler(a)

	srv := http.Server{
		Addr:           cfg.ListenAddr,
		Handler:        router,
		ReadTimeout:    cfg.ReadTimeout,
		WriteTimeout:   cfg.WriteTimeout,
		MaxHeaderBytes: cfg.MaxHeaderBytes,
	}

	go func() {
		logger.Info("server start")
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", err)
			signalChan <- syscall.SIGABRT
		}
		logger.Info("server shut down")
	}()

	sig := <-signalChan
	if sig == syscall.SIGABRT {
		return errors.New("unable to start server")
	}

	logger.Info("shutting down server", lager.Data{"signal": sig.String()})

	graceCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(graceCtx)
}
