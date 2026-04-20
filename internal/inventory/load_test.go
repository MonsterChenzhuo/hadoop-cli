package inventory

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_ParsesValidInventory(t *testing.T) {
	inv, err := Load(filepath.Join("testdata", "valid-3-node.yaml"))
	require.NoError(t, err)

	require.Equal(t, "hbase-dev", inv.Cluster.Name)
	require.Equal(t, "/opt/hadoop-cli", inv.Cluster.InstallDir)
	require.Equal(t, "3.3.6", inv.Versions.Hadoop)
	require.Equal(t, 8, inv.SSH.Parallelism)
	require.Len(t, inv.Hosts, 3)
	require.Equal(t, []string{"node1"}, inv.Roles.NameNode)
	require.Equal(t, []string{"node1", "node2", "node3"}, inv.Roles.ZooKeeper)
	require.Equal(t, 2, inv.Overrides.HDFS.Replication)
	require.Equal(t, "2g", inv.Overrides.HBase.RegionServerHeap)
}

func TestLoad_AppliesDefaultsWhenUnset(t *testing.T) {
	inv, err := LoadBytes([]byte(`
cluster:
  name: demo
  install_dir: /opt/hadoop-cli
  data_dir: /data/hadoop-cli
  user: hadoop
  java_home: /usr/lib/jvm/java-11
versions: { hadoop: 3.3.6, zookeeper: 3.8.4, hbase: 2.5.8 }
ssh: { user: hadoop, private_key: ~/.ssh/id_rsa }
hosts:
  - { name: n1, address: 127.0.0.1 }
roles:
  namenode: [n1]
  datanode: [n1]
  zookeeper: [n1]
  hbase_master: [n1]
  regionserver: [n1]
`))
	require.NoError(t, err)
	require.Equal(t, 22, inv.SSH.Port)
	require.Equal(t, 8, inv.SSH.Parallelism)
	require.Equal(t, 3, inv.Overrides.HDFS.Replication)
	require.Equal(t, 2181, inv.Overrides.ZooKeeper.ClientPort)
}

func TestLoad_FailsOnUnknownField(t *testing.T) {
	_, err := LoadBytes([]byte(`cluster: { name: x, install_dir: /a, data_dir: /b, user: u, java_home: /j }
this_field_does_not_exist: 1
`))
	require.Error(t, err)
}
