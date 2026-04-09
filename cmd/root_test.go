package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootCommandOutput(t *testing.T) {
	cmd := newRootCmd("")
	b := bytes.NewBufferString("")

	cmd.SetArgs([]string{"-h"})
	cmd.SetOut(b)

	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, b.String(), "websudo")
	require.Contains(t, b.String(), "serve")
}
