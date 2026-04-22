package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoot_RegistersExportSnapshotCommand(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())
	require.Contains(t, buf.String(), "export-snapshot")
}

func TestExportSnapshot_HelpListsFlags(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"export-snapshot", "--help"})
	require.NoError(t, root.Execute())
	help := buf.String()
	for _, s := range []string{"--name", "--to", "--to-inventory", "--mappers", "--bandwidth", "--overwrite", "--extra-args", "--on"} {
		require.Containsf(t, help, s, "help should mention %s", s)
	}
}

func TestExportSnapshot_RejectsToAndToInventoryTogether(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{
		"export-snapshot",
		"--name", "s",
		"--to", "hdfs://a/b",
		"--to-inventory", "dst.yaml",
	})
	err := root.Execute()
	require.Error(t, err)
}

func TestExportSnapshot_RejectsNeitherToNorToInventory(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"export-snapshot", "--name", "s"})
	err := root.Execute()
	require.Error(t, err)
}
