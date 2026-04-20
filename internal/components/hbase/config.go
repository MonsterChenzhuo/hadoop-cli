package hbase

import (
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/render"
)

func Home(inv *inventory.Inventory) string      { return inv.Cluster.InstallDir + "/hbase" }
func LogDir(inv *inventory.Inventory) string    { return inv.Cluster.DataDir + "/hbase/logs" }
func PidDir(inv *inventory.Inventory) string    { return inv.Cluster.DataDir + "/hbase/pids" }
func ZKDataDir(inv *inventory.Inventory) string { return inv.Cluster.DataDir + "/hbase/zookeeper" }

func rootDir(inv *inventory.Inventory) (string, error) {
	if inv.Overrides.HBase.RootDir != "" {
		return inv.Overrides.HBase.RootDir, nil
	}
	if len(inv.Roles.NameNode) != 1 {
		return "", fmt.Errorf("cannot derive hbase.rootdir without single namenode")
	}
	h, ok := inv.HostByName(inv.Roles.NameNode[0])
	if !ok {
		return "", fmt.Errorf("namenode host not found")
	}
	return fmt.Sprintf("hdfs://%s:%d/hbase", h.Address, inv.Overrides.HDFS.NameNodeRPC), nil
}

func zkQuorum(inv *inventory.Inventory) string {
	addrs := make([]string, 0, len(inv.Roles.ZooKeeper))
	for _, name := range inv.Roles.ZooKeeper {
		if h, ok := inv.HostByName(name); ok {
			addrs = append(addrs, h.Address)
		}
	}
	return strings.Join(addrs, ",")
}

func RenderHBaseSite(inv *inventory.Inventory) (string, error) {
	root, err := rootDir(inv)
	if err != nil {
		return "", err
	}
	return render.XMLSite([]render.Property{
		{Name: "hbase.rootdir", Value: root},
		{Name: "hbase.cluster.distributed", Value: "true"},
		{Name: "hbase.zookeeper.quorum", Value: zkQuorum(inv)},
		{Name: "hbase.zookeeper.property.clientPort", Value: fmt.Sprintf("%d", inv.Overrides.ZooKeeper.ClientPort)},
		{Name: "hbase.unsafe.stream.capability.enforce", Value: "false"},
		{Name: "hbase.master.port", Value: fmt.Sprintf("%d", inv.Overrides.HBase.MasterPort)},
		{Name: "hbase.master.info.port", Value: fmt.Sprintf("%d", inv.Overrides.HBase.MasterInfoPort)},
		{Name: "hbase.regionserver.port", Value: fmt.Sprintf("%d", inv.Overrides.HBase.RSPort)},
		{Name: "hbase.regionserver.info.port", Value: fmt.Sprintf("%d", inv.Overrides.HBase.RSInfoPort)},
	})
}

func RenderRegionServers(inv *inventory.Inventory) string {
	var b strings.Builder
	for _, name := range inv.Roles.RegionServer {
		if h, ok := inv.HostByName(name); ok {
			b.WriteString(h.Address)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func RenderBackupMasters(_ *inventory.Inventory) string { return "" }

func RenderHBaseEnv(inv *inventory.Inventory) string {
	return fmt.Sprintf(`export JAVA_HOME=%s
export HBASE_LOG_DIR=%s
export HBASE_PID_DIR=%s
export HBASE_MANAGES_ZK=false
export HBASE_HEAPSIZE=%s
export HBASE_MASTER_OPTS="-Xmx%s"
export HBASE_REGIONSERVER_OPTS="-Xmx%s"
`, inv.Cluster.JavaHome, LogDir(inv), PidDir(inv),
		inv.Overrides.HBase.MasterHeap,
		inv.Overrides.HBase.MasterHeap,
		inv.Overrides.HBase.RegionServerHeap,
	)
}
