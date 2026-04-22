package hbaseops

import (
	"strings"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/stretchr/testify/require"
)

func invFull() *inventory.Inventory {
	inv := invWithHosts([]string{"m1"}, []string{"rs1"})
	inv.Cluster = inventory.Cluster{
		InstallDir: "/opt/hadoop",
		JavaHome:   "/usr/lib/jvm/java-11",
	}
	return inv
}

func TestBuildSnapshotScript_OnlineSnapshot(t *testing.T) {
	script, err := BuildSnapshotScript(invFull(), SnapshotOptions{
		Table: "rta:tag_by_uid",
		Name:  "rta_tag_by_uid_1030",
	})
	require.NoError(t, err)
	require.Contains(t, script, "export JAVA_HOME=/usr/lib/jvm/java-11")
	require.Contains(t, script, "/opt/hadoop/hbase/bin/hbase shell -n")
	require.Contains(t, script, "snapshot 'rta:tag_by_uid','rta_tag_by_uid_1030'")
	require.NotContains(t, script, "SKIP_FLUSH")
}

func TestBuildSnapshotScript_SkipFlush(t *testing.T) {
	script, err := BuildSnapshotScript(invFull(), SnapshotOptions{
		Table:     "t",
		Name:      "s",
		SkipFlush: true,
	})
	require.NoError(t, err)
	require.Contains(t, script, "snapshot 't','s', {SKIP_FLUSH => true}")
}

func TestBuildSnapshotScript_RejectsEmptyTable(t *testing.T) {
	_, err := BuildSnapshotScript(invFull(), SnapshotOptions{Table: "", Name: "s"})
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "table")
}

func TestBuildSnapshotScript_RejectsEmptyName(t *testing.T) {
	_, err := BuildSnapshotScript(invFull(), SnapshotOptions{Table: "t", Name: ""})
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "name")
}

func TestBuildSnapshotScript_RejectsQuoteInName(t *testing.T) {
	_, err := BuildSnapshotScript(invFull(), SnapshotOptions{Table: "t", Name: "x'y"})
	require.Error(t, err)
}

func TestBuildSnapshotScript_RejectsNewlineInTable(t *testing.T) {
	_, err := BuildSnapshotScript(invFull(), SnapshotOptions{Table: "t\nDROP", Name: "s"})
	require.Error(t, err)
}
