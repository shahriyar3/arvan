package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("APP_NAME", "test-gateway")
	t.Setenv("HTTP_PORT", "9090")

	cfg, err := Load()
	require.NoError(t, err)
	require.NotNil(t, cfg)

	assert.Equal(t, "test-gateway", cfg.App.Name)
	assert.Equal(t, 9090, cfg.HTTP.Port)
	assert.Equal(t, cfg.Database.PrimaryDSN, cfg.Database.ReplicaDSN)
	assert.Equal(t, "0.0.0.0:9090", cfg.HTTP.Addr())
}
