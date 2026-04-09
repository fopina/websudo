package server

import (
	"context"
	"testing"

	"github.com/fopina/websudo/internal/config"
	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	srv := New(&config.Config{Services: map[string]config.Service{"github": {BaseURL: "https://api.github.com"}}})
	require.NoError(t, srv.Run(context.Background()))
}
