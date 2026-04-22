package hbaseops

import (
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

// PickHost returns the host to run the hbase command on.
// If override is empty, it defaults to the first HBase master.
// If override is set, it must appear in the inventory's role hosts.
func PickHost(inv *inventory.Inventory, override string) (string, error) {
	if override != "" {
		for _, h := range inv.AllRoleHosts() {
			if h == override {
				return override, nil
			}
		}
		return "", fmt.Errorf("--on host %q is not in the inventory", override)
	}
	if len(inv.Roles.HBaseMaster) == 0 {
		return "", fmt.Errorf("inventory has no roles.hbase_master; pass --on <host> or fix the inventory")
	}
	return inv.Roles.HBaseMaster[0], nil
}
