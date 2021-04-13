package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pascaldekloe/jwt"
)

// Config contains all configuration values.
type Config struct {
	Kubeconfig     string
	ServiceIDs     []string
	ListenAddr     string
	Username       string
	Password       string
	JWKeyRegister  jwt.KeyRegister
	Namespace      string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxHeaderBytes int
}

// GetEnv is an interface that allows to get variables from the environment
type GetEnv func(string) string
type keyLoadingFun func(keys jwt.KeyRegister, file []byte) (int, error)

// ReadConfig reads env variables using the passed function.
func ReadConfig(getEnv GetEnv) (*Config, error) {
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
		rt = 3 * time.Minute
	}
	cfg.ReadTimeout = rt

	wt, err := time.ParseDuration(getEnv("OSB_HTTP_WRITE_TIMEOUT"))
	if err != nil {
		wt = 3 * time.Minute
	}
	cfg.WriteTimeout = wt

	mhb, err := strconv.Atoi(getEnv("OSB_HTTP_MAX_HEADER_BYTES"))
	if err != nil {
		mhb = 1 << 20 // 1 MB
	}
	cfg.MaxHeaderBytes = mhb

	err = loadJWTSigningKeys(getEnv, cfg.JWKeyRegister)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func loadJWTSigningKeys(getEnv GetEnv, keys jwt.KeyRegister) error {
	err := loadKeysFromFile(getEnv, keys, "OSB_JWT_KEYS_JWK_PATH", loadJWK)
	if err != nil {
		return err
	}

	err = loadKeysFromFile(getEnv, keys, "OSB_JWT_KEYS_PEM_PATH", loadPEM)
	if err != nil {
		return err
	}
	return nil
}

func loadKeysFromFile(getEnv GetEnv, keys jwt.KeyRegister, envVarName string, loadFunc keyLoadingFun) error {
	jwkPath := getEnv(envVarName)
	if jwkPath != "" {
		file, err := os.ReadFile(jwkPath)
		if err != nil {
			return fmt.Errorf("%s is set to '%s', but: %w", envVarName, jwkPath, err)
		}
		_, err = loadFunc(keys, file)
		if err != nil {
			return fmt.Errorf("unable to parse %s '%s': %w", envVarName, jwkPath, err)
		}
	}
	return nil
}

func loadJWK(keys jwt.KeyRegister, file []byte) (int, error) {
	return keys.LoadJWK(file)
}

func loadPEM(keys jwt.KeyRegister, file []byte) (int, error) {
	return keys.LoadPEM(file, []byte{})
}
