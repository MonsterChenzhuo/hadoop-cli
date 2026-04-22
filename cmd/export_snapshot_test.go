package cmd

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestExportSnapshot_RejectsInconsistentToInventory(t *testing.T) {
	dir := t.TempDir()

	srcYAML := `cluster:
  name: hbase-dev
  install_dir: /opt/hadoop-cli
  data_dir: /data/hadoop-cli
  user: hadoop
  java_home: /usr/lib/jvm/java-11
versions:
  hadoop: 3.3.6
  zookeeper: 3.8.4
  hbase: 2.5.8
ssh:
  port: 22
  user: hadoop
  private_key: ~/.ssh/id_rsa
  parallelism: 8
  sudo: false
hosts:
  - { name: node1, address: 10.0.0.11 }
  - { name: node2, address: 10.0.0.12 }
  - { name: node3, address: 10.0.0.13 }
roles:
  namenode:     [node1]
  datanode:     [node1, node2, node3]
  zookeeper:    [node1, node2, node3]
  hbase_master: [node1]
  regionserver: [node1, node2, node3]
`
	// Destination names a NameNode host "ghost" that does not appear in hosts:
	dstYAML := `cluster:
  name: hbase-dst
  install_dir: /opt/hadoop-cli
  data_dir: /data/hadoop-cli
  user: hadoop
  java_home: /usr/lib/jvm/java-11
hosts:
  - { name: nn1, address: 10.1.0.11 }
roles:
  namenode: [ghost]
`
	srcPath := filepath.Join(dir, "src.yaml")
	dstPath := filepath.Join(dir, "dst.yaml")
	require.NoError(t, os.WriteFile(srcPath, []byte(srcYAML), 0o600))
	require.NoError(t, os.WriteFile(dstPath, []byte(dstYAML), 0o600))

	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{
		"export-snapshot",
		"--inventory", srcPath,
		"--name", "snap1",
		"--to-inventory", dstPath,
	})
	err := root.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "ghost")
}
