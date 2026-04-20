package hdfs

import (
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/render"
)

func Home(inv *inventory.Inventory) string {
	return inv.Cluster.InstallDir + "/hadoop"
}

func NNDir(inv *inventory.Inventory) string  { return inv.Cluster.DataDir + "/hdfs/nn" }
func DNDir(inv *inventory.Inventory) string  { return inv.Cluster.DataDir + "/hdfs/dn" }
func LogDir(inv *inventory.Inventory) string { return inv.Cluster.DataDir + "/hdfs/logs" }

func nameNodeAddress(inv *inventory.Inventory) (string, error) {
	if len(inv.Roles.NameNode) != 1 {
		return "", fmt.Errorf("expected exactly 1 namenode, got %d", len(inv.Roles.NameNode))
	}
	h, ok := inv.HostByName(inv.Roles.NameNode[0])
	if !ok {
		return "", fmt.Errorf("namenode host %q not in hosts list", inv.Roles.NameNode[0])
	}
	return h.Address, nil
}

func RenderCoreSite(inv *inventory.Inventory) (string, error) {
	nn, err := nameNodeAddress(inv)
	if err != nil {
		return "", err
	}
	return render.XMLSite([]render.Property{
		{Name: "fs.defaultFS", Value: fmt.Sprintf("hdfs://%s:%d", nn, inv.Overrides.HDFS.NameNodeRPC)},
		{Name: "hadoop.tmp.dir", Value: inv.Cluster.DataDir + "/hdfs/tmp"},
	})
}

func RenderHDFSSite(inv *inventory.Inventory) (string, error) {
	return render.XMLSite([]render.Property{
		{Name: "dfs.replication", Value: fmt.Sprintf("%d", inv.Overrides.HDFS.Replication)},
		{Name: "dfs.namenode.name.dir", Value: "file://" + NNDir(inv)},
		{Name: "dfs.datanode.data.dir", Value: "file://" + DNDir(inv)},
		{Name: "dfs.namenode.http-address", Value: fmt.Sprintf("0.0.0.0:%d", inv.Overrides.HDFS.NameNodeHTTP)},
		{Name: "dfs.permissions.enabled", Value: "false"},
	})
}

func RenderWorkers(inv *inventory.Inventory) string {
	var b strings.Builder
	for _, name := range inv.Roles.DataNode {
		if h, ok := inv.HostByName(name); ok {
			b.WriteString(h.Address)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func RenderHadoopEnv(inv *inventory.Inventory) string {
	return fmt.Sprintf(`export JAVA_HOME=%s
export HADOOP_LOG_DIR=%s
export HDFS_NAMENODE_OPTS="-Xmx%s"
export HDFS_DATANODE_OPTS="-Xmx%s"
`, inv.Cluster.JavaHome, LogDir(inv), inv.Overrides.HDFS.NameNodeHeap, inv.Overrides.HDFS.DataNodeHeap)
}
