package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecuteShowsHelp(t *testing.T) {
	cmd := newRootCmd("dev")
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "websudo")
	require.Contains(t, buf.String(), "serve")
}

func TestExecuteWrapsErrors(t *testing.T) {
	err := Execute("dev")
	require.NoError(t, err)
}
