package zookeeper

import (
	"strings"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/stretchr/testify/require"
)

func fixture() *inventory.Inventory {
	return &inventory.Inventory{
		Cluster:  inventory.Cluster{InstallDir: "/opt/hadoop-cli", DataDir: "/data/hadoop-cli", JavaHome: "/j"},
		Versions: inventory.Versions{ZooKeeper: "3.8.4"},
		Hosts: []inventory.Host{
			{Name: "n1", Address: "10.0.0.1"},
			{Name: "n2", Address: "10.0.0.2"},
			{Name: "n3", Address: "10.0.0.3"},
		},
		Roles: inventory.Roles{ZooKeeper: []string{"n1", "n2", "n3"}},
		Overrides: inventory.Overrides{ZooKeeper: inventory.ZKOverrides{
			ClientPort: 2181, TickTime: 2000, InitLimit: 10, SyncLimit: 5,
		}},
	}
}

func TestRenderZooCfg_ContainsServerLinesAndDataDir(t *testing.T) {
	cfg, err := RenderZooCfg(fixture())
	require.NoError(t, err)
	require.Contains(t, cfg, "dataDir=/data/hadoop-cli/zookeeper")
	require.Contains(t, cfg, "clientPort=2181")
	require.Contains(t, cfg, "server.1=10.0.0.1:2888:3888")
	require.Contains(t, cfg, "server.2=10.0.0.2:2888:3888")
	require.Contains(t, cfg, "server.3=10.0.0.3:2888:3888")
}

func TestMyIDFor_IsOrdinalStartingAt1(t *testing.T) {
	inv := fixture()
	require.Equal(t, 1, MyIDFor(inv, "n1"))
	require.Equal(t, 2, MyIDFor(inv, "n2"))
	require.Equal(t, 3, MyIDFor(inv, "n3"))
	require.Equal(t, 0, MyIDFor(inv, "missing"))
}

func TestRenderEnv_SetsJAVAHOMEAndHeap(t *testing.T) {
	env, err := RenderEnv(fixture())
	require.NoError(t, err)
	require.Contains(t, env, "JAVA_HOME=/j")
	require.True(t, strings.Contains(env, "ZK_SERVER_HEAP") || strings.Contains(env, "SERVER_JVMFLAGS"))
}
