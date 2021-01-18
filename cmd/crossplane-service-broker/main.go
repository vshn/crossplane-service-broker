package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"code.cloudfoundry.org/lager"
	"github.com/gorilla/mux"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/vshn/crossplane-service-broker/pkg/api"
	"github.com/vshn/crossplane-service-broker/pkg/brokerapi"
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
	cfg, err := readAppConfig(os.Getenv)
	if err != nil {
		return fmt.Errorf("unable to read app env: %w", err)
	}

	router := mux.NewRouter()

	config, err := loadRESTConfig(cfg)
	if err != nil {
		return err
	}

	b, err := brokerapi.New(cfg.serviceIDs, cfg.namespace, config, logger.WithData(lager.Data{"component": "brokerapi"}))
	if err != nil {
		return err
	}

	a := api.New(b, cfg.username, cfg.password, logger.WithData(lager.Data{"component": "api"}))
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

func loadRESTConfig(cfg *appConfig) (*rest.Config, error) {
	if cfg.kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", cfg.kubeconfig)
	}
	return rest.InClusterConfig()
}

type appConfig struct {
	kubeconfig     string
	serviceIDs     []string
	listenAddr     string
	username       string
	password       string
	namespace      string
	readTimeout    time.Duration
	writeTimeout   time.Duration
	maxHeaderBytes int
}

func readAppConfig(getEnv func(string) string) (*appConfig, error) {
	cfg := appConfig{
		kubeconfig: getEnv("KUBECONFIG"),
		serviceIDs: strings.Split(getEnv("OSB_SERVICE_IDS"), ","),
		username:   getEnv("OSB_USERNAME"),
		password:   getEnv("OSB_PASSWORD"),
		namespace:  getEnv("OSB_NAMESPACE"),
		listenAddr: getEnv("OSB_HTTP_LISTEN_ADDR"),
	}

	for i := range cfg.serviceIDs {
		cfg.serviceIDs[i] = strings.TrimSpace(cfg.serviceIDs[i])
		if len(cfg.serviceIDs[i]) == 0 {
			return nil, errors.New("OSB_SERVICE_IDS is required")
		}
	}

	if cfg.username == "" {
		return nil, errors.New("OSB_USERNAME is required")
	}
	if cfg.password == "" {
		return nil, errors.New("OSB_PASSWORD is required")
	}

	if cfg.namespace == "" {
		return nil, errors.New("OSB_NAMESPACE is required")
	}

	if cfg.listenAddr == "" {
		cfg.listenAddr = ":8080"
	}

	rt, err := time.ParseDuration(getEnv("OSB_HTTP_READ_TIMEOUT"))
	if err != nil {
		rt = 180 * time.Second
	}
	cfg.readTimeout = rt

	wt, err := time.ParseDuration(getEnv("OSB_HTTP_WRITE_TIMEOUT"))
	if err != nil {
		wt = 180 * time.Second
	}
	cfg.writeTimeout = wt

	mhb, err := strconv.Atoi(getEnv("OSB_HTTP_MAX_HEADER_BYTES"))
	if err != nil {
		mhb = 1 << 20 // 1 MB
	}
	cfg.maxHeaderBytes = mhb

	return &cfg, nil
}
