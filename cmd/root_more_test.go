package cmd

import (
	"bytes"
	"os"
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

func TestExecuteUsesProcessArgs(t *testing.T) {
	err := Execute("dev")
	require.NoError(t, err)
}

func TestExecuteWrapsErrors(t *testing.T) {
	args := os.Args
	t.Cleanup(func() {
		os.Args = args
	})
	os.Args = []string{"websudo", "unknown-command"}

	err := Execute("dev")
	require.ErrorContains(t, err, "error executing root command")
	require.ErrorContains(t, err, "unknown command")
}
