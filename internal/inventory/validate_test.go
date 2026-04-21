package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func baseInv() *Inventory {
	return &Inventory{
		Cluster: Cluster{
			Name:       "c",
			InstallDir: "/opt/hadoop-cli",
			DataDir:    "/data/hadoop-cli",
			User:       "hadoop",
			JavaHome:   "/j",
			Components: []string{"zookeeper", "hdfs", "hbase"},
		},
		Versions: Versions{Hadoop: "3.3.6", ZooKeeper: "3.8.4", HBase: "2.5.8"},
		SSH:      SSH{Port: 22, User: "hadoop", PrivateKey: "~/.ssh/id_rsa", Parallelism: 8},
		Hosts: []Host{
			{Name: "n1", Address: "10.0.0.1"},
			{Name: "n2", Address: "10.0.0.2"},
			{Name: "n3", Address: "10.0.0.3"},
		},
		Roles: Roles{
			NameNode:     []string{"n1"},
			DataNode:     []string{"n1", "n2", "n3"},
			ZooKeeper:    []string{"n1", "n2", "n3"},
			HBaseMaster:  []string{"n1"},
			RegionServer: []string{"n1", "n2", "n3"},
		},
	}
}

func TestValidate_OK(t *testing.T) {
	require.NoError(t, Validate(baseInv()))
}

func TestValidate_RejectsMultipleNameNodes(t *testing.T) {
	inv := baseInv()
	inv.Roles.NameNode = []string{"n1", "n2"}
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "namenode")
}

func TestValidate_RejectsEvenZooKeeperCount(t *testing.T) {
	inv := baseInv()
	inv.Roles.ZooKeeper = []string{"n1", "n2"}
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "odd")
}

func TestValidate_RejectsUnknownHostRef(t *testing.T) {
	inv := baseInv()
	inv.Roles.RegionServer = []string{"n1", "ghost"}
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ghost")
}

func TestValidate_RejectsRelativePaths(t *testing.T) {
	inv := baseInv()
	inv.Cluster.InstallDir = "opt/hadoop-cli"
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "install_dir")
}

func TestValidate_RejectsUnsupportedVersion(t *testing.T) {
	inv := baseInv()
	inv.Versions.HBase = "1.0.0"
	err := Validate(inv)
	require.Error(t, err)
}

func zkOnlyInv() *Inventory {
	return &Inventory{
		Cluster: Cluster{
			Name:       "zk",
			InstallDir: "/opt/hadoop-cli",
			DataDir:    "/data/hadoop-cli",
			User:       "hadoop",
			JavaHome:   "/j",
			Components: []string{"zookeeper"},
		},
		Versions: Versions{ZooKeeper: "3.8.4"},
		SSH:      SSH{Port: 22, User: "hadoop", PrivateKey: "~/.ssh/id_rsa", Parallelism: 8},
		Hosts: []Host{
			{Name: "n1", Address: "10.0.0.1"},
			{Name: "n2", Address: "10.0.0.2"},
			{Name: "n3", Address: "10.0.0.3"},
		},
		Roles: Roles{
			ZooKeeper: []string{"n1", "n2", "n3"},
		},
	}
}

func TestValidate_ZKOnly_OK(t *testing.T) {
	require.NoError(t, Validate(zkOnlyInv()))
}

func TestValidate_ZKOnly_IgnoresMissingHadoopAndHBaseVersions(t *testing.T) {
	inv := zkOnlyInv()
	require.Empty(t, inv.Versions.Hadoop)
	require.Empty(t, inv.Versions.HBase)
	require.NoError(t, Validate(inv))
}

func TestValidate_ZKOnly_RejectsEvenZooKeeperCount(t *testing.T) {
	inv := zkOnlyInv()
	inv.Roles.ZooKeeper = []string{"n1", "n2"}
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "odd")
}

func TestValidate_RejectsUnknownComponent(t *testing.T) {
	inv := zkOnlyInv()
	inv.Cluster.Components = []string{"zookeeper", "kafka"}
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown component")
}

func TestValidate_RejectsEmptyComponents(t *testing.T) {
	inv := zkOnlyInv()
	inv.Cluster.Components = nil
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not be empty")
}

func TestValidate_HDFSOnly_OK(t *testing.T) {
	inv := baseInv()
	inv.Cluster.Components = []string{"hdfs"}
	inv.Versions.ZooKeeper = "" // not required
	inv.Versions.HBase = ""     // not required
	inv.Roles.ZooKeeper = nil
	inv.Roles.HBaseMaster = nil
	inv.Roles.RegionServer = nil
	require.NoError(t, Validate(inv))
}

func TestValidate_HBaseRequiresZooKeeper(t *testing.T) {
	inv := baseInv()
	inv.Cluster.Components = []string{"hdfs", "hbase"}
	inv.Versions.ZooKeeper = ""
	inv.Roles.ZooKeeper = nil
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "hbase requires zookeeper")
}

func TestValidate_HBaseWithoutHDFSRequiresRootDir(t *testing.T) {
	inv := baseInv()
	inv.Cluster.Components = []string{"zookeeper", "hbase"}
	inv.Versions.Hadoop = "" // HDFS not included
	inv.Roles.NameNode = nil
	inv.Roles.DataNode = nil
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "root_dir")
}

func TestValidate_HBaseWithoutHDFSAcceptsExternalRootDir(t *testing.T) {
	inv := baseInv()
	inv.Cluster.Components = []string{"zookeeper", "hbase"}
	inv.Versions.Hadoop = ""
	inv.Roles.NameNode = nil
	inv.Roles.DataNode = nil
	inv.Overrides.HBase.RootDir = "hdfs://external-nn:8020/hbase"
	require.NoError(t, Validate(inv))
}
