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
	// but Cobra only renders that header when subcommands exist. Subcommands
	// are introduced in Task 16; until then we assert on "Usage:" which is
	// always present in the help output.
	require.Contains(t, buf.String(), "Usage:")
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
