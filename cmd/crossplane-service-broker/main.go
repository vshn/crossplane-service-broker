package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/gorilla/mux"

	"github.com/vshn/crossplane-service-broker/pkg/api"
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

	if err := run(ctx, signalChan, logger); err != nil {
		logger.Error("application  run failed", err)
		os.Exit(exitCodeErr)
	}
}

func run(ctx context.Context, signalChan chan os.Signal, logger lager.Logger) error {
	cfg, err := readAppConfig()
	if err != nil {
		return fmt.Errorf("unable to read app env: %w", err)
	}

	router := mux.NewRouter()

	a := api.New(logger.WithData(lager.Data{"component": "api"}))
	router.NewRoute().Handler(a)

	srv := http.Server{
		Addr:           cfg.listenAddr,
		Handler:        router,
		ReadTimeout:    cfg.readTimeout,
		WriteTimeout:   cfg.writeTimeout,
		MaxHeaderBytes: cfg.maxHeaderBytes,
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

type appConfig struct {
	listenAddr     string
	readTimeout    time.Duration
	writeTimeout   time.Duration
	maxHeaderBytes int
}

func readAppConfig() (*appConfig, error) {
	cfg := appConfig{
		listenAddr: os.Getenv("OSB_HTTP_LISTEN_ADDR"),
	}

	if cfg.listenAddr == "" {
		cfg.listenAddr = ":8080"
	}

	rt, err := time.ParseDuration(os.Getenv("OSB_HTTP_READ_TIMEOUT"))
	if err != nil {
		rt = 180 * time.Second
	}
	cfg.readTimeout = rt

	wt, err := time.ParseDuration(os.Getenv("OSB_HTTP_WRITE_TIMEOUT"))
	if err != nil {
		wt = 180 * time.Second
	}
	cfg.readTimeout = wt

	mhb, err := strconv.Atoi(os.Getenv("OSB_HTTP_MAX_HEADER_BYTES"))
	if err != nil {
		mhb = 1 << 20 // 1 MB
	}
	cfg.maxHeaderBytes = mhb

	return &cfg, nil
}

