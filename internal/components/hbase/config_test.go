package hbase

import (
	"strings"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/stretchr/testify/require"
)

func fixture() *inventory.Inventory {
	return &inventory.Inventory{
		Cluster:  inventory.Cluster{InstallDir: "/opt/hadoop-cli", DataDir: "/data/hadoop-cli", JavaHome: "/j"},
		Versions: inventory.Versions{HBase: "2.5.8"},
		Hosts: []inventory.Host{
			{Name: "n1", Address: "10.0.0.1"},
			{Name: "n2", Address: "10.0.0.2"},
			{Name: "n3", Address: "10.0.0.3"},
		},
		Roles: inventory.Roles{
			NameNode:     []string{"n1"},
			ZooKeeper:    []string{"n1", "n2", "n3"},
			HBaseMaster:  []string{"n1"},
			RegionServer: []string{"n1", "n2", "n3"},
		},
		Overrides: inventory.Overrides{
			HDFS:      inventory.HDFSOverrides{NameNodeRPC: 8020},
			ZooKeeper: inventory.ZKOverrides{ClientPort: 2181},
			HBase:     inventory.HBaseOverrides{MasterHeap: "1g", RegionServerHeap: "1g"},
		},
	}
}

func TestRenderHBaseSite_UsesDerivedRootDirAndZKQuorum(t *testing.T) {
	s, err := RenderHBaseSite(fixture())
	require.NoError(t, err)
	require.Contains(t, s, "<name>hbase.rootdir</name>")
	require.Contains(t, s, "<value>hdfs://10.0.0.1:8020/hbase</value>")
	require.Contains(t, s, "<name>hbase.zookeeper.quorum</name>")
	require.Contains(t, s, "<value>10.0.0.1,10.0.0.2,10.0.0.3</value>")
	require.Contains(t, s, "<name>hbase.cluster.distributed</name>")
	require.Contains(t, s, "<value>true</value>")
}

func TestRenderHBaseSite_HonorsExplicitRootDir(t *testing.T) {
	inv := fixture()
	inv.Overrides.HBase.RootDir = "hdfs://custom:9000/h"
	s, err := RenderHBaseSite(inv)
	require.NoError(t, err)
	require.Contains(t, s, "<value>hdfs://custom:9000/h</value>")
}

func TestRenderRegionServers_HasOneLinePerHost(t *testing.T) {
	s := RenderRegionServers(fixture())
	require.Equal(t, 3, strings.Count(s, "\n"))
}
