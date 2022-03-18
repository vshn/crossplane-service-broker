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
	Kubeconfig         string
	ServiceIDs         []string
	ListenAddr         string
	Username           string
	Password           string
	UsernameClaim      string
	JWKeyRegister      *jwt.KeyRegister
	Namespace          string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	MaxHeaderBytes     int
	PlanUpdateSizeRule string
	PlanUpdateSLARule  string
	EnableMetrics      bool
	MetricsDomain      string
}

// GetEnv is an interface that allows to get variables from the environment
type GetEnv func(string) string

type keyLoadingFun func(keys *jwt.KeyRegister, content []byte) (int, error)

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

	// EnvPlanUpdateSLA is a set of `|` seprated white-list rules for SLA changes
	EnvPlanUpdateSLA = "OSB_PLAN_UPDATE_SLA_RULES"
	// EnvPlanUpdateSize is a set of `|` seprated white-list rules for plan size changes
	EnvPlanUpdateSize = "OSB_PLAN_UPDATE_SIZE_RULES"

	// EnvEnableMetrics defines if metrics endpoints are returned.
	EnvEnableMetrics = "ENABLE_METRICS"
	// EnvMetricsDomain sets domain name for the metrics endpoints.
	EnvMetricsDomain = "METRICS_DOMAIN"

	defaultHTTPTimeout        = 3 * time.Minute
	defaultHTTPMaxHeaderBytes = 1 << 20 // 1 MB
	defaultHTTPListenAddr     = ":8080"
	defaultUsernameClaim      = "sub"
	defaultSLAUpdateRules     = "standard>premium|premium>standard"
	defaultEnableMetrics      = false
)

// ReadConfig reads env variables using the passed function.
func ReadConfig(getEnv GetEnv) (*Config, error) {
	cfg := Config{
		Kubeconfig:         getEnv(EnvKubeconfig),
		Username:           getEnv(EnvUsername),
		Password:           getEnv(EnvPassword),
		UsernameClaim:      getEnv(EnvUsernameClaim),
		Namespace:          getEnv(EnvNamespace),
		ListenAddr:         getEnv(EnvHTTPListenAddr),
		JWKeyRegister:      &jwt.KeyRegister{},
		PlanUpdateSizeRule: getEnv(EnvPlanUpdateSize),
		PlanUpdateSLARule:  getEnv(EnvPlanUpdateSLA),
		MetricsDomain:      getEnv(EnvMetricsDomain),
	}

	if cfg.PlanUpdateSLARule == "" {
		cfg.PlanUpdateSLARule = defaultSLAUpdateRules
	}

	ids, err := getServiceIDs(getEnv)
	if err != nil {
		return nil, err
	}
	cfg.ServiceIDs = ids

	rt, err := getTimeout(getEnv, EnvHTTPReadTimeout)
	if err != nil {
		return nil, err
	}
	cfg.ReadTimeout = rt

	wt, err := getTimeout(getEnv, EnvHTTPWriteTimeout)
	if err != nil {
		return nil, err
	}
	cfg.WriteTimeout = wt

	bytes, err := getHTTPMaxHeaderBytes(getEnv)
	if err != nil {
		return nil, err
	}
	cfg.MaxHeaderBytes = bytes

	err = loadJWTSigningKeys(getEnv, cfg.JWKeyRegister)
	if err != nil {
		return nil, err
	}

	err = ensureRequiredSettings(cfg)
	if err != nil {
		return nil, err
	}

	enableMetrics, err := getEnableMetrics(getEnv)
	if err != nil {
		return nil, err
	}
	cfg.EnableMetrics = enableMetrics

	metricsDomain, err := getMetricsDomain(getEnv, cfg.EnableMetrics)
	if err != nil {
		return nil, err
	}
	cfg.MetricsDomain = metricsDomain

	setDefaults(&cfg)

	return &cfg, nil
}

func setDefaults(cfg *Config) {
	if cfg.UsernameClaim == "" {
		cfg.UsernameClaim = defaultUsernameClaim
	}

	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultHTTPListenAddr
	}
}

func ensureRequiredSettings(cfg Config) error {
	if cfg.Username == "" {
		return fmt.Errorf("%s is required, but was not defined or is empty", EnvUsername)
	}
	if cfg.Password == "" {
		return fmt.Errorf("%s is required, but was not defined or is empty", EnvPassword)
	}
	if cfg.Namespace == "" {
		return fmt.Errorf("%s is required, but was not defined or is empty", EnvNamespace)
	}
	if cfg.EnableMetrics == true && cfg.MetricsDomain == "" {
		return fmt.Errorf("%s is required, but was not defined or is empty", EnvMetricsDomain)
	}
	return nil
}

func getHTTPMaxHeaderBytes(getEnv GetEnv) (int, error) {
	var bytes int
	httpMaxHeaderBytes := getEnv(EnvHTTPMaxHeaderBytes)
	if httpMaxHeaderBytes == "" {
		bytes = defaultHTTPMaxHeaderBytes
	} else {
		mhb, err := strconv.Atoi(httpMaxHeaderBytes)
		if err != nil {
			return 0, fmt.Errorf("%s is set to '%s', but a number was expected: %w", EnvHTTPMaxHeaderBytes, httpMaxHeaderBytes, err)
		}
		bytes = mhb
	}
	return bytes, nil
}

func getServiceIDs(getEnv GetEnv) ([]string, error) {
	var ids []string
	for _, s := range strings.Split(getEnv(EnvServiceIDs), ",") {
		s = strings.TrimSpace(s)
		if s != "" {
			ids = append(ids, s)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%s is required, but was not defined or is empty", EnvServiceIDs)
	}
	return ids, nil
}

func getTimeout(getEnv GetEnv, timeoutName string) (time.Duration, error) {
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

func getEnableMetrics(getEnv GetEnv) (bool, error) {
	enableMetrics := getEnv(EnvEnableMetrics)
	if enableMetrics == "" {
		return defaultEnableMetrics, nil
	}
	metricsEnabled, err := strconv.ParseBool(enableMetrics)
	if err != nil {
		return false, fmt.Errorf("%s is set to '%t', but a boolean was expected: %w", EnvEnableMetrics, metricsEnabled, err)
	}
	return metricsEnabled, nil
}

func getMetricsDomain(GetEnv GetEnv, enableMetrics bool) (string, error) {
	if !enableMetrics {
		return "", nil
	}
	metricsDomain := GetEnv(EnvMetricsDomain)
	if metricsDomain == "" {
		return "", fmt.Errorf("%s is set to true, but %s is empty", EnvEnableMetrics, EnvMetricsDomain)
	}
	return metricsDomain, nil
}

func loadJWTSigningKeys(getEnv GetEnv, keys *jwt.KeyRegister) error {
	err := loadKeysFromPath(getEnv, keys, EnvJWTKeyJWKURL, loadJWK)
	if err != nil {
		return err
	}
	err = loadKeysFromPath(getEnv, keys, EnvJWTKeyPEMURL, loadPEM)
	return err
}

func loadKeysFromPath(getEnv GetEnv, keys *jwt.KeyRegister, envVarName string, loadFunc keyLoadingFun) error {
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

func loadJWK(keys *jwt.KeyRegister, content []byte) (int, error) {
	return keys.LoadJWK(content)
}

func loadPEM(keys *jwt.KeyRegister, content []byte) (int, error) {
	return keys.LoadPEM(content, []byte{})
}
