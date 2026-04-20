package zookeeper

import (
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

// MyIDFor returns the 1-based ordinal of `host` inside roles.zookeeper, or 0 if missing.
func MyIDFor(inv *inventory.Inventory, host string) int {
	for i, h := range inv.Roles.ZooKeeper {
		if h == host {
			return i + 1
		}
	}
	return 0
}

func DataDir(inv *inventory.Inventory) string {
	return inv.Cluster.DataDir + "/zookeeper"
}

func LogDir(inv *inventory.Inventory) string {
	return inv.Cluster.DataDir + "/zookeeper/logs"
}

func Home(inv *inventory.Inventory) string {
	return fmt.Sprintf("%s/zookeeper", inv.Cluster.InstallDir)
}

func RenderZooCfg(inv *inventory.Inventory) (string, error) {
	zk := inv.Overrides.ZooKeeper
	var b strings.Builder
	fmt.Fprintf(&b, "tickTime=%d\n", zk.TickTime)
	fmt.Fprintf(&b, "initLimit=%d\n", zk.InitLimit)
	fmt.Fprintf(&b, "syncLimit=%d\n", zk.SyncLimit)
	fmt.Fprintf(&b, "dataDir=%s\n", DataDir(inv))
	fmt.Fprintf(&b, "dataLogDir=%s\n", LogDir(inv))
	fmt.Fprintf(&b, "clientPort=%d\n", zk.ClientPort)
	fmt.Fprintf(&b, "4lw.commands.whitelist=*\n")
	fmt.Fprintf(&b, "admin.enableServer=false\n")
	for i, hostName := range inv.Roles.ZooKeeper {
		h, ok := inv.HostByName(hostName)
		if !ok {
			return "", fmt.Errorf("unknown zookeeper host %q", hostName)
		}
		fmt.Fprintf(&b, "server.%d=%s:2888:3888\n", i+1, h.Address)
	}
	return b.String(), nil
}

func RenderEnv(inv *inventory.Inventory) (string, error) {
	return fmt.Sprintf(`export JAVA_HOME=%s
export ZOOCFGDIR=%s/conf
export ZOO_LOG_DIR=%s
export ZK_SERVER_HEAP=%s
export SERVER_JVMFLAGS="-Xmx%sm"
`, inv.Cluster.JavaHome, Home(inv), LogDir(inv), "512", "512"), nil
}
