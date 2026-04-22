package hbaseops

import (
	"context"
	"strings"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/stretchr/testify/require"
)

func TestExportSnapshot_DispatchesToMaster(t *testing.T) {
	fe := &fakeExec{}
	runner := orchestrator.NewRunner(fe, 1)
	res, err := ExportSnapshot(context.Background(), runner, invFull(), ExportOptions{
		Name:   "snap1",
		CopyTo: "hdfs://h:8020/hbase",
	}, "")
	require.NoError(t, err)
	require.True(t, res.OK)
	require.Equal(t, "m1", fe.seenHost)
	require.Contains(t, fe.seenCmd, "-snapshot snap1")
}

func TestExportSnapshot_HostOverride(t *testing.T) {
	fe := &fakeExec{}
	runner := orchestrator.NewRunner(fe, 1)
	_, err := ExportSnapshot(context.Background(), runner, invFull(), ExportOptions{
		Name: "snap1", CopyTo: "hdfs://h:8020/hbase",
	}, "rs1")
	require.NoError(t, err)
	require.Equal(t, "rs1", fe.seenHost)
}

func destInv(nn string, rpc int) *inventory.Inventory {
	inv := invWithHosts([]string{"nm"}, []string{"rs1"})
	inv.Cluster = inventory.Cluster{InstallDir: "/opt/hadoop", JavaHome: "/j"}
	inv.Roles.NameNode = []string{nn}
	inv.Overrides.HDFS.NameNodeRPC = rpc
	return inv
}

func TestDeriveCopyToFromInventory_UsesDefaultRPC(t *testing.T) {
	inv := destInv("node9", 0)
	url, err := DeriveCopyToFromInventory(inv)
	require.NoError(t, err)
	require.Equal(t, "hdfs://node9:8020/hbase", url)
}

func TestDeriveCopyToFromInventory_CustomRPC(t *testing.T) {
	inv := destInv("node9", 9000)
	url, err := DeriveCopyToFromInventory(inv)
	require.NoError(t, err)
	require.Equal(t, "hdfs://node9:9000/hbase", url)
}

func TestDeriveCopyToFromInventory_RequiresExactlyOneNameNode(t *testing.T) {
	inv := destInv("node9", 0)
	inv.Roles.NameNode = nil
	_, err := DeriveCopyToFromInventory(inv)
	require.Error(t, err)

	inv.Roles.NameNode = []string{"a", "b"}
	_, err = DeriveCopyToFromInventory(inv)
	require.Error(t, err)
}

func TestBuildExportCommand_Minimal(t *testing.T) {
	cmd, err := BuildExportCommand(invFull(), ExportOptions{
		Name:   "snap1",
		CopyTo: "hdfs://h:8020/hbase",
	})
	require.NoError(t, err)
	require.Contains(t, cmd, "export JAVA_HOME=/usr/lib/jvm/java-11")
	require.Contains(t, cmd, "/opt/hadoop/hbase/bin/hbase org.apache.hadoop.hbase.snapshot.ExportSnapshot")
	require.Contains(t, cmd, "-snapshot snap1")
	require.Contains(t, cmd, "-copy-to hdfs://h:8020/hbase")
	require.NotContains(t, cmd, "-mappers")
	require.NotContains(t, cmd, "-bandwidth")
	require.NotContains(t, cmd, "-overwrite")
}

func TestBuildExportCommand_AllFlags(t *testing.T) {
	mappers := 0
	cmd, err := BuildExportCommand(invFull(), ExportOptions{
		Name:      "snap1",
		CopyTo:    "hdfs://h:8020/hbase",
		Mappers:   &mappers,
		Bandwidth: 50,
		Overwrite: true,
		ExtraArgs: "-D foo=bar",
	})
	require.NoError(t, err)
	require.Contains(t, cmd, "-mappers 0")
	require.Contains(t, cmd, "-bandwidth 50")
	require.Contains(t, cmd, "-overwrite")
	require.Contains(t, cmd, "-D foo=bar")
}

func TestBuildExportCommand_ShellQuotesValues(t *testing.T) {
	_, err := BuildExportCommand(invFull(), ExportOptions{
		Name:   "sn ap",
		CopyTo: "hdfs://h:8020/a b",
	})
	require.Error(t, err, "spaces in identifier-like fields should be rejected before quoting")
	require.Contains(t, strings.ToLower(err.Error()), "name")
}

func TestBuildExportCommand_RejectsEmptyName(t *testing.T) {
	_, err := BuildExportCommand(invFull(), ExportOptions{Name: "", CopyTo: "hdfs://h/x"})
	require.Error(t, err)
}

func TestBuildExportCommand_RejectsNonHDFSCopyTo(t *testing.T) {
	_, err := BuildExportCommand(invFull(), ExportOptions{Name: "s", CopyTo: "/local"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "hdfs://")
}

func TestBuildExportCommand_RejectsShellMetacharInCopyTo(t *testing.T) {
	_, err := BuildExportCommand(invFull(), ExportOptions{
		Name: "s", CopyTo: "hdfs://h/x; rm -rf /",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsafe shell metacharacter")
}

func TestBuildExportCommand_RejectsSpaceInCopyTo(t *testing.T) {
	_, err := BuildExportCommand(invFull(), ExportOptions{
		Name: "s", CopyTo: "hdfs://h/a b",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsafe shell metacharacter")
}

func TestBuildExportCommand_RejectsNegativeMappers(t *testing.T) {
	neg := -1
	_, err := BuildExportCommand(invFull(), ExportOptions{
		Name: "s", CopyTo: "hdfs://h/x", Mappers: &neg,
	})
	require.Error(t, err)
}
