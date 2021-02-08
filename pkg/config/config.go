package config

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Config struct {
	Kubeconfig     string
	ServiceIDs     []string
	ListenAddr     string
	Username       string
	Password       string
	Namespace      string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxHeaderBytes int
}

func ReadConfig(getEnv func(string) string) (*Config, error) {
	cfg := Config{
		Kubeconfig: getEnv("KUBECONFIG"),
		ServiceIDs: strings.Split(getEnv("OSB_SERVICE_IDS"), ","),
		Username:   getEnv("OSB_USERNAME"),
		Password:   getEnv("OSB_PASSWORD"),
		Namespace:  getEnv("OSB_NAMESPACE"),
		ListenAddr: getEnv("OSB_HTTP_LISTEN_ADDR"),
	}

	for i := range cfg.ServiceIDs {
		cfg.ServiceIDs[i] = strings.TrimSpace(cfg.ServiceIDs[i])
		if len(cfg.ServiceIDs[i]) == 0 {
			return nil, errors.New("OSB_SERVICE_IDS is required")
		}
	}

	if cfg.Username == "" {
		return nil, errors.New("OSB_USERNAME is required")
	}
	if cfg.Password == "" {
		return nil, errors.New("OSB_PASSWORD is required")
	}

	if cfg.Namespace == "" {
		return nil, errors.New("OSB_NAMESPACE is required")
	}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}

	rt, err := time.ParseDuration(getEnv("OSB_HTTP_READ_TIMEOUT"))
	if err != nil {
		rt = 180 * time.Second
	}
	cfg.ReadTimeout = rt

	wt, err := time.ParseDuration(getEnv("OSB_HTTP_WRITE_TIMEOUT"))
	if err != nil {
		wt = 180 * time.Second
	}
	cfg.WriteTimeout = wt

	mhb, err := strconv.Atoi(getEnv("OSB_HTTP_MAX_HEADER_BYTES"))
	if err != nil {
		mhb = 1 << 20 // 1 MB
	}
	cfg.MaxHeaderBytes = mhb

	return &cfg, nil
}

func loadRESTConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
