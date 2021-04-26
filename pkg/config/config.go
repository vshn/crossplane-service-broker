package config

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	http "github.com/hashicorp/go-cleanhttp"
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
type keyLoadingFun func(keys jwt.KeyRegister, content []byte) (int, error)

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
	err := loadKeysFromPath(getEnv, keys, "OSB_JWT_KEYS_JWK_URL", loadJWK)
	if err != nil {
		return err
	}
	err = loadKeysFromPath(getEnv, keys, "OSB_JWT_KEYS_PEM_URL", loadPEM)
	return err
}

func loadKeysFromPath(getEnv GetEnv, keys jwt.KeyRegister, envVarName string, loadFunc keyLoadingFun) error {
	envVarValue := getEnv(envVarName)
	if envVarValue == "" {
		return nil
	}

	content, err := loadContentFromPath(envVarValue)
	if err != nil {
		return fmt.Errorf("unable to load keys from '%s' (defined in %s): %w", envVarValue, envVarName, err)
	}

	_, err = loadFunc(keys, content)
	if err != nil {
		return err
	}
	return nil
}

func loadContentFromPath(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("an empty path is not allowed")
	}

	urlOfPath, err := url.Parse(path)
	if err != nil {
		return nil, fmt.Errorf("the value '%s' can't be parsed as url: %w", path, err)
	}

	var content []byte
	switch {
	case urlOfPath.Scheme == "https":
		content, err = loadContentFromHTTP(urlOfPath)
	case urlOfPath.Scheme == "file":
		content, err = loadContentFromFile(urlOfPath)
	case urlOfPath.Scheme == "http":
		return nil, fmt.Errorf("the scheme '%s' of '%s' is not supported. Did you mean 'https' instead of 'http'? Supported schemes are 'https' and 'file'", urlOfPath.Scheme, path)
	default:
		return nil, fmt.Errorf("the scheme '%s' of '%s' is not supported. Supported schemes are 'https' and 'file'", urlOfPath.Scheme, path)
	}
	if err != nil {
		return nil, fmt.Errorf("content can't be loaded from '%s': %w", urlOfPath, err)
	}

	return content, nil
}

func loadContentFromHTTP(url *url.URL) ([]byte, error) {
	client := http.DefaultClient()
	urlStr := url.String()
	response, err := client.Get(urlStr)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to '%s': '%w", urlStr, err)
	}
	defer response.Body.Close()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response from '%s': %w", urlStr, err)
	}
	return content, nil
}

func loadContentFromFile(fileURL *url.URL) ([]byte, error) {
	filePath := fileURL.Path
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("unable to read file '%s': %w", filePath, err)
	}
	return content, nil
}

func loadJWK(keys jwt.KeyRegister, content []byte) (int, error) {
	return keys.LoadJWK(content)
}

func loadPEM(keys jwt.KeyRegister, content []byte) (int, error) {
	return keys.LoadPEM(content, []byte{})
}
