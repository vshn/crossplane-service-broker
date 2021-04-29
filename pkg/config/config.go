package config

import (
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
	UsernameClaim  string
	JWKeyRegister  jwt.KeyRegister
	Namespace      string
	ReadTimeout    time.Duration
	WriteTimeout   time.Duration
	MaxHeaderBytes int
}

// GetEnv is an interface that allows to get variables from the environment
type GetEnv func(string) string

type keyLoadingFun func(keys jwt.KeyRegister, content []byte) (int, error)

const (
	// EnvKubeconfig is used to configure the internal K8s client
	EnvKubeconfig = "KUBECONFIG"

	// EnvNamespace defines the K8s namespace that is used to keep the state of the service broker.
	EnvNamespace = "OSB_NAMESPACE"

	// EnvServiceIDs is a comma-separated list (no whitespace after comma!) of service ids available in the cluster
	EnvServiceIDs = "OSB_SERVICE_IDS"

	// EnvUsername defines the username to use when connecting to this service broker
	// when no JWT Bearer Token is presented.
	EnvUsername = "OSB_USERNAME"

	// EnvPassword defines the password to use when connecting to this service broker
	// when no JWT Bearer Token is presented.
	EnvPassword = "OSB_PASSWORD"

	// EnvUsernameClaim defines the name of the claim which is considered to be the username of the request principal
	// whenever a JWT Bearer Token is presented.
	EnvUsernameClaim = "OSB_USERNAME_CLAIM"

	// EnvHTTPListenAddr defines which port to listen on for HTTP requests.
	EnvHTTPListenAddr = "OSB_HTTP_LISTEN_ADDR"

	// EnvHTTPReadTimeout sets a read timeout for HTTP requests.
	EnvHTTPReadTimeout = "OSB_HTTP_READ_TIMEOUT"

	// EnvHTTPWriteTimeout sets a write timeout for HTTP requests.
	EnvHTTPWriteTimeout = "OSB_HTTP_WRITE_TIMEOUT"

	// EnvHTTPMaxHeaderBytes sets the maximum header size for HTTP requests.
	EnvHTTPMaxHeaderBytes = "OSB_HTTP_MAX_HEADER_BYTES"

	// EnvJWTKeyJWKURL sets the URL of a JWK file, which is used to validate the signatures of the JWT Bearer Tokens.
	EnvJWTKeyJWKURL = "OSB_JWT_KEYS_JWK_URL"

	// EnvJWTKeyPEMURL sets the URL of a PEM file, which is used to validate the signatures of the JWT Bearer Tokens.
	EnvJWTKeyPEMURL = "OSB_JWT_KEYS_PEM_URL"

	defaultHTTPTimeout        = 3 * time.Minute
	defaultHTTPMaxHeaderBytes = 1 << 20 // 1 MB
	defaultHTTPListenAddr     = ":8080"
	defaultUsernameClaim      = "sub"
)

// ReadConfig reads env variables using the passed function.
func ReadConfig(getEnv GetEnv) (*Config, error) {
	cfg := Config{
		Kubeconfig:    getEnv(EnvKubeconfig),
		ServiceIDs:    strings.Split(getEnv(EnvServiceIDs), ","),
		Username:      getEnv(EnvUsername),
		Password:      getEnv(EnvPassword),
		UsernameClaim: getEnv(EnvUsernameClaim),
		Namespace:     getEnv(EnvNamespace),
		ListenAddr:    getEnv(EnvHTTPListenAddr),
	}

	for i := range cfg.ServiceIDs {
		cfg.ServiceIDs[i] = strings.TrimSpace(cfg.ServiceIDs[i])
		if len(cfg.ServiceIDs[i]) == 0 {
			return nil, fmt.Errorf("%s is required, but was not defined or is empty", EnvServiceIDs)
		}
	}

	if cfg.Username == "" {
		return nil, fmt.Errorf("%s is required, but was not defined or is empty", EnvUsername)
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("%s is required, but was not defined or is empty", EnvPassword)
	}
	if cfg.UsernameClaim == "" {
		cfg.UsernameClaim = defaultUsernameClaim
	}

	if cfg.Namespace == "" {
		return nil, fmt.Errorf("%s is required, but was not defined or is empty", EnvNamespace)
	}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultHTTPListenAddr
	}

	rt, err := getTimeoutFromEnv(getEnv, EnvHTTPReadTimeout)
	if err != nil {
		return nil, err
	}
	cfg.ReadTimeout = rt

	wt, err := getTimeoutFromEnv(getEnv, EnvHTTPWriteTimeout)
	if err != nil {
		return nil, err
	}
	cfg.WriteTimeout = wt

	httpMaxHeaderBytes := getEnv(EnvHTTPMaxHeaderBytes)
	if httpMaxHeaderBytes == "" {
		cfg.MaxHeaderBytes = defaultHTTPMaxHeaderBytes
	} else {
		mhb, err := strconv.Atoi(httpMaxHeaderBytes)
		if err != nil {
			return nil, fmt.Errorf("%s is set to '%s', but a number was expected: %w", EnvHTTPMaxHeaderBytes, httpMaxHeaderBytes, err)
		}
		cfg.MaxHeaderBytes = mhb
	}

	err = loadJWTSigningKeys(getEnv, cfg.JWKeyRegister)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func getTimeoutFromEnv(getEnv GetEnv, timeoutName string) (time.Duration, error) {
	timeout := getEnv(timeoutName)
	if timeout == "" {
		return defaultHTTPTimeout, nil
	}

	wt, err := time.ParseDuration(timeout)
	if err != nil {
		return 0, fmt.Errorf("%s is set to '%s', but that is not a valid time format: %w", timeoutName, timeout, err)
	}
	return wt, err
}

func loadJWTSigningKeys(getEnv GetEnv, keys jwt.KeyRegister) error {
	err := loadKeysFromPath(getEnv, keys, EnvJWTKeyJWKURL, loadJWK)
	if err != nil {
		return err
	}
	err = loadKeysFromPath(getEnv, keys, EnvJWTKeyPEMURL, loadPEM)
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
