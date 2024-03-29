package config

import (
	"testing"

	"github.com/pascaldekloe/jwt"
	"github.com/stretchr/testify/assert"
)

func Test_ReadConfig(t *testing.T) {
	tt := map[string]struct {
		env    map[string]string
		config *Config
		err    string
	}{
		"serviceIDs required": {
			env: map[string]string{
				EnvUsername:  "user",
				EnvPassword:  "password",
				EnvNamespace: "namespace",
			},
			config: nil,
			err:    "OSB_SERVICE_IDS is required, but was not defined or is empty",
		},
		"empty serviceIDs given": {
			env: map[string]string{
				EnvUsername:   "user",
				EnvPassword:   "password",
				EnvNamespace:  "namespace",
				EnvServiceIDs: ",,,",
			},
			config: nil,
			err:    "OSB_SERVICE_IDS is required, but was not defined or is empty",
		},
		"username required": {
			env: map[string]string{
				EnvServiceIDs: "1,2,3",
			},
			config: nil,
			err:    "OSB_USERNAME is required, but was not defined or is empty",
		},
		"password required": {
			env: map[string]string{
				EnvServiceIDs: "1,2,3",
				EnvUsername:   "user",
			},
			config: nil,
			err:    "OSB_PASSWORD is required, but was not defined or is empty",
		},
		"namespace required": {
			env: map[string]string{
				EnvServiceIDs: "1,2,3",
				EnvUsername:   "user",
				EnvPassword:   "pw",
			},
			config: nil,
			err:    "OSB_NAMESPACE is required, but was not defined or is empty",
		},
		"metrics domain required": {
			env: map[string]string{
				EnvServiceIDs:    "1,2,3",
				EnvUsername:      "user",
				EnvPassword:      "pw",
				EnvNamespace:     "test",
				EnvEnableMetrics: "true",
			},
			config: nil,
			err:    "ENABLE_METRICS is set to true, but METRICS_DOMAIN is empty",
		},
		"username claim given": {
			env: map[string]string{
				EnvServiceIDs:    "1,2,3",
				EnvUsername:      "user",
				EnvPassword:      "pw",
				EnvNamespace:     "test",
				EnvUsernameClaim: "different than default",
			},
			config: &Config{
				ServiceIDs:        []string{"1", "2", "3"},
				ListenAddr:        ":8080",
				Username:          "user",
				Password:          "pw",
				UsernameClaim:     "different than default",
				Namespace:         "test",
				ReadTimeout:       defaultHTTPTimeout,
				WriteTimeout:      defaultHTTPTimeout,
				MaxHeaderBytes:    defaultHTTPMaxHeaderBytes,
				JWKeyRegister:     &jwt.KeyRegister{},
				PlanUpdateSLARule: defaultSLAUpdateRules,
			},
			err: "",
		},
		"metrics domain given": {
			env: map[string]string{
				EnvServiceIDs:    "1,2,3",
				EnvUsername:      "user",
				EnvPassword:      "pw",
				EnvNamespace:     "test",
				EnvEnableMetrics: "true",
				EnvMetricsDomain: "example.tld",
			},
			config: &Config{
				ServiceIDs:        []string{"1", "2", "3"},
				ListenAddr:        ":8080",
				Username:          "user",
				Password:          "pw",
				UsernameClaim:     defaultUsernameClaim,
				Namespace:         "test",
				ReadTimeout:       defaultHTTPTimeout,
				WriteTimeout:      defaultHTTPTimeout,
				MaxHeaderBytes:    defaultHTTPMaxHeaderBytes,
				JWKeyRegister:     &jwt.KeyRegister{},
				PlanUpdateSLARule: defaultSLAUpdateRules,
				EnableMetrics:     true,
				MetricsDomain:     "example.tld",
			},
			err: "",
		},
		"defaults configured": {
			env: map[string]string{
				EnvServiceIDs: "1,2,3",
				EnvUsername:   "user",
				EnvPassword:   "pw",
				EnvNamespace:  "test",
			},
			config: &Config{
				ServiceIDs:        []string{"1", "2", "3"},
				ListenAddr:        defaultHTTPListenAddr,
				Username:          "user",
				Password:          "pw",
				UsernameClaim:     defaultUsernameClaim,
				Namespace:         "test",
				ReadTimeout:       defaultHTTPTimeout,
				WriteTimeout:      defaultHTTPTimeout,
				MaxHeaderBytes:    defaultHTTPMaxHeaderBytes,
				JWKeyRegister:     &jwt.KeyRegister{},
				PlanUpdateSLARule: defaultSLAUpdateRules,
				EnableMetrics:     defaultEnableMetrics,
			},
			err: "",
		},
	}

	for name, v := range tt {
		t.Run(name, func(t *testing.T) {
			cfg, err := ReadConfig(func(key string) string {
				return v.env[key]
			})
			if v.err != "" {
				assert.Nil(t, cfg)
				assert.EqualError(t, err, v.err)
				return
			}
			assert.Empty(t, err)
			assert.Equal(t, v.config, cfg)
		})
	}
}
