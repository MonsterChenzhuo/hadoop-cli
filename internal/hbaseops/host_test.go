package hbaseops

import (
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/stretchr/testify/require"
)

func invWithHosts(masters, rs []string) *inventory.Inventory {
	hosts := []inventory.Host{}
	seen := map[string]bool{}
	for _, n := range append(append([]string{}, masters...), rs...) {
		if seen[n] {
			continue
		}
		seen[n] = true
		hosts = append(hosts, inventory.Host{Name: n, Address: n})
	}
	return &inventory.Inventory{
		Hosts: hosts,
		Roles: inventory.Roles{HBaseMaster: masters, RegionServer: rs},
	}
}

func TestPickHost_DefaultsToFirstMaster(t *testing.T) {
	inv := invWithHosts([]string{"m1", "m2"}, []string{"rs1"})
	h, err := PickHost(inv, "")
	require.NoError(t, err)
	require.Equal(t, "m1", h)
}

func TestPickHost_NoMastersAndNoOverride(t *testing.T) {
	inv := invWithHosts(nil, []string{"rs1"})
	_, err := PickHost(inv, "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "hbase_master")
}

func TestPickHost_OverrideMustBeKnown(t *testing.T) {
	inv := invWithHosts([]string{"m1"}, []string{"rs1"})
	_, err := PickHost(inv, "stranger")
	require.Error(t, err)
	require.Contains(t, err.Error(), "stranger")
}

func TestPickHost_OverrideMatchesRegionServer(t *testing.T) {
	inv := invWithHosts([]string{"m1"}, []string{"rs1"})
	h, err := PickHost(inv, "rs1")
	require.NoError(t, err)
	require.Equal(t, "rs1", h)
}
