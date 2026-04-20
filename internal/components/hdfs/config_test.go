package hdfs

import (
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/stretchr/testify/require"
)

func fixture() *inventory.Inventory {
	return &inventory.Inventory{
		Cluster:  inventory.Cluster{InstallDir: "/opt/hadoop-cli", DataDir: "/data/hadoop-cli", JavaHome: "/j", User: "hadoop"},
		Versions: inventory.Versions{Hadoop: "3.3.6"},
		Hosts: []inventory.Host{
			{Name: "n1", Address: "10.0.0.1"},
			{Name: "n2", Address: "10.0.0.2"},
		},
		Roles: inventory.Roles{NameNode: []string{"n1"}, DataNode: []string{"n1", "n2"}},
		Overrides: inventory.Overrides{HDFS: inventory.HDFSOverrides{
			Replication: 2, NameNodeHeap: "1g", DataNodeHeap: "1g",
			NameNodeRPC: 8020, NameNodeHTTP: 9870,
		}},
	}
}

func TestRenderCoreSite_UsesNameNodeAddress(t *testing.T) {
	s, err := RenderCoreSite(fixture())
	require.NoError(t, err)
	require.Contains(t, s, "<name>fs.defaultFS</name>")
	require.Contains(t, s, "<value>hdfs://10.0.0.1:8020</value>")
}

func TestRenderHDFSSite_SetsReplicationAndDirs(t *testing.T) {
	s, err := RenderHDFSSite(fixture())
	require.NoError(t, err)
	require.Contains(t, s, "<name>dfs.replication</name>")
	require.Contains(t, s, "<value>2</value>")
	require.Contains(t, s, "/data/hadoop-cli/hdfs/nn")
	require.Contains(t, s, "/data/hadoop-cli/hdfs/dn")
}

func TestRenderWorkers_ListsDataNodes(t *testing.T) {
	s := RenderWorkers(fixture())
	require.Equal(t, "10.0.0.1\n10.0.0.2\n", s)
}
