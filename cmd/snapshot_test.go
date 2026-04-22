package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoot_RegistersSnapshotCommand(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())
	require.Contains(t, buf.String(), "snapshot")
}

func TestSnapshot_HelpListsRequiredFlags(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"snapshot", "--help"})
	require.NoError(t, root.Execute())
	help := buf.String()
	require.Contains(t, help, "--table")
	require.Contains(t, help, "--name")
	require.Contains(t, help, "--skip-flush")
	require.Contains(t, help, "--on")
}
