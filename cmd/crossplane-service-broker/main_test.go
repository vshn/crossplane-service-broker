package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_readAppConfig(t *testing.T) {
	tt := map[string]struct {
		env    map[string]string
		config *appConfig
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
		"defaults configured": {
			env: map[string]string{
				"OSB_SERVICE_IDS": "1,2,3",
				"OSB_USERNAME":    "user",
				"OSB_PASSWORD":    "pw",
			},
			config: &appConfig{
				serviceIDs:     []string{"1", "2", "3"},
				listenAddr:     ":8080",
				username:       "user",
				password:       "pw",
				readTimeout:    180 * time.Second,
				writeTimeout:   180 * time.Second,
				maxHeaderBytes: 1 << 20,
			},
			err: "",
		},
	}

	for name, v := range tt {
		t.Run(name, func(t *testing.T) {
			cfg, err := readAppConfig(func(key string) string {
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
