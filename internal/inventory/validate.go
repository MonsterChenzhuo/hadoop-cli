package inventory

import (
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/errs"
)

var supportedVersions = struct {
	Hadoop    []string
	ZooKeeper []string
	HBase     []string
}{
	Hadoop:    []string{"3.3.4", "3.3.5", "3.3.6"},
	ZooKeeper: []string{"3.7.2", "3.8.3", "3.8.4"},
	HBase:     []string{"2.5.5", "2.5.7", "2.5.8"},
}

func Validate(inv *Inventory) error {
	var msgs []string
	add := func(s string) { msgs = append(msgs, s) }

	if !strings.HasPrefix(inv.Cluster.InstallDir, "/") {
		add("cluster.install_dir must be an absolute path")
	}
	if !strings.HasPrefix(inv.Cluster.DataDir, "/") {
		add("cluster.data_dir must be an absolute path")
	}
	if inv.Cluster.Name == "" {
		add("cluster.name is required")
	}
	if inv.Cluster.User == "" {
		add("cluster.user is required")
	}
	if inv.SSH.User == "" {
		add("ssh.user is required")
	}
	if inv.SSH.PrivateKey == "" {
		add("ssh.private_key is required")
	}

	if !contains(supportedVersions.Hadoop, inv.Versions.Hadoop) {
		add(fmt.Sprintf("unsupported hadoop version %q; supported: %s",
			inv.Versions.Hadoop, strings.Join(supportedVersions.Hadoop, ", ")))
	}
	if !contains(supportedVersions.ZooKeeper, inv.Versions.ZooKeeper) {
		add(fmt.Sprintf("unsupported zookeeper version %q; supported: %s",
			inv.Versions.ZooKeeper, strings.Join(supportedVersions.ZooKeeper, ", ")))
	}
	if !contains(supportedVersions.HBase, inv.Versions.HBase) {
		add(fmt.Sprintf("unsupported hbase version %q; supported: %s",
			inv.Versions.HBase, strings.Join(supportedVersions.HBase, ", ")))
	}

	if len(inv.Roles.NameNode) != 1 {
		add(fmt.Sprintf("roles.namenode must have exactly 1 host (v1 single-NN); got %d", len(inv.Roles.NameNode)))
	}
	if n := len(inv.Roles.ZooKeeper); n == 0 || n%2 == 0 {
		add(fmt.Sprintf("roles.zookeeper must have an odd number of hosts (1,3,5); got %d", n))
	}
	if len(inv.Roles.DataNode) == 0 {
		add("roles.datanode must not be empty")
	}
	if len(inv.Roles.HBaseMaster) == 0 {
		add("roles.hbase_master must not be empty")
	}
	if len(inv.Roles.RegionServer) == 0 {
		add("roles.regionserver must not be empty")
	}

	hostNames := map[string]bool{}
	for _, h := range inv.Hosts {
		if h.Name == "" || h.Address == "" {
			add(fmt.Sprintf("hosts entry missing name or address: %+v", h))
			continue
		}
		if hostNames[h.Name] {
			add(fmt.Sprintf("duplicate host name %q", h.Name))
		}
		hostNames[h.Name] = true
	}
	for role, list := range map[string][]string{
		"namenode":     inv.Roles.NameNode,
		"datanode":     inv.Roles.DataNode,
		"zookeeper":    inv.Roles.ZooKeeper,
		"hbase_master": inv.Roles.HBaseMaster,
		"regionserver": inv.Roles.RegionServer,
	} {
		for _, name := range list {
			if !hostNames[name] {
				add(fmt.Sprintf("roles.%s references unknown host %q", role, name))
			}
		}
	}

	if len(msgs) > 0 {
		return errs.New(errs.CodeInventoryInvalid, "", strings.Join(msgs, "; "))
	}
	return nil
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
