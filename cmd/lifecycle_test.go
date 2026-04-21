package cmd

import (
	"bytes"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
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

func TestComponentsForInv_ZKOnly(t *testing.T) {
	inv := &inventory.Inventory{Cluster: inventory.Cluster{Components: []string{"zookeeper"}}}
	comps, err := componentsForInv(inv, "", false, false)
	require.NoError(t, err)
	require.Len(t, comps, 1)
	require.Equal(t, "zookeeper", comps[0].Name())
}

func TestComponentsForInv_RejectsFilterOutsideInventory(t *testing.T) {
	inv := &inventory.Inventory{Cluster: inventory.Cluster{Components: []string{"zookeeper"}}}
	_, err := componentsForInv(inv, "hbase", false, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "hbase")
}

func TestComponentsForInv_FullStackReverse(t *testing.T) {
	inv := &inventory.Inventory{Cluster: inventory.Cluster{Components: []string{"zookeeper", "hdfs", "hbase"}}}
	comps, err := componentsForInv(inv, "", true, false)
	require.NoError(t, err)
	require.Len(t, comps, 3)
	require.Equal(t, "hbase", comps[0].Name())
	require.Equal(t, "zookeeper", comps[2].Name())
}
