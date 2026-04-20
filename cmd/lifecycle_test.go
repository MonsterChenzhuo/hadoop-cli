package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoot_RegistersLifecycleCommands(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())
	for _, s := range []string{"preflight", "install", "configure", "start", "stop", "status", "uninstall"} {
		require.Containsf(t, buf.String(), s, "help should mention %s", s)
	}
}
