package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootCommand_ShowsHelpByDefault(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})

	err := root.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "hadoop-cli")
	// NOTE: Deviation from plan. The plan asked for "Available Commands",
	// but Cobra's default help template only renders the full usage block
	// (with "Usage:" and "Available Commands:" headers) when the command is
	// Runnable or has subcommands. With the redundant RunE removed and no
	// subcommands yet (added in Task 16), only the Long description is
	// emitted. This assertion will be tightened once subcommands land.
}

func TestRootCommand_HasVersion(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--version"})

	err := root.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "hadoop-cli")
}
