package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_ReadConfig(t *testing.T) {
	tt := map[string]struct {
		env    map[string]string
		config *Config
		err    string
	}{
		"serviceIDs required": {
			env:    map[string]string{},
			config: nil,
			err:    "OSB_SERVICE_IDS is required",
		},
		"username required": {
			env: map[string]string{
				"OSB_SERVICE_IDS": "1,2,3",
			},
			config: nil,
			err:    "OSB_USERNAME is required",
		},
		"password required": {
			env: map[string]string{
				"OSB_SERVICE_IDS": "1,2,3",
				"OSB_USERNAME":    "user",
			},
			config: nil,
			err:    "OSB_PASSWORD is required",
		},
		"namespace required": {
			env: map[string]string{
				"OSB_SERVICE_IDS": "1,2,3",
				"OSB_USERNAME":    "user",
				"OSB_PASSWORD":    "pw",
			},
			config: nil,
			err:    "OSB_NAMESPACE is required",
		},
		"defaults configured": {
			env: map[string]string{
				"OSB_SERVICE_IDS": "1,2,3",
				"OSB_USERNAME":    "user",
				"OSB_PASSWORD":    "pw",
				"OSB_NAMESPACE":   "test",
			},
			config: &Config{
				ServiceIDs:     []string{"1", "2", "3"},
				ListenAddr:     ":8080",
				Username:       "user",
				Password:       "pw",
				Namespace:      "test",
				ReadTimeout:    180 * time.Second,
				WriteTimeout:   180 * time.Second,
				MaxHeaderBytes: 1 << 20,
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
