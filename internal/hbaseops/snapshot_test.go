package hbaseops

import (
	"context"
	"strings"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/stretchr/testify/require"
)

type fakeExec struct {
	seenHost string
	seenCmd  string
	result   orchestrator.Result
}

func (f *fakeExec) Execute(_ context.Context, host string, t orchestrator.Task) orchestrator.Result {
	f.seenHost = host
	f.seenCmd = t.Cmd
	if f.result.Host == "" {
		f.result.Host = host
		f.result.OK = true
	}
	return f.result
}

func TestSnapshot_DispatchesToMaster(t *testing.T) {
	fe := &fakeExec{}
	runner := orchestrator.NewRunner(fe, 1)
	res, err := Snapshot(context.Background(), runner, invFull(), SnapshotOptions{
		Table: "t", Name: "s",
	}, "")
	require.NoError(t, err)
	require.True(t, res.OK)
	require.Equal(t, "m1", fe.seenHost)
	require.Contains(t, fe.seenCmd, "snapshot 't','s'")
}

func TestSnapshot_OverrideHostWins(t *testing.T) {
	fe := &fakeExec{}
	runner := orchestrator.NewRunner(fe, 1)
	_, err := Snapshot(context.Background(), runner, invFull(), SnapshotOptions{
		Table: "t", Name: "s",
	}, "rs1")
	require.NoError(t, err)
	require.Equal(t, "rs1", fe.seenHost)
}

func TestSnapshot_ValidationErrorsPropagate(t *testing.T) {
	fe := &fakeExec{}
	runner := orchestrator.NewRunner(fe, 1)
	_, err := Snapshot(context.Background(), runner, invFull(), SnapshotOptions{
		Table: "", Name: "s",
	}, "")
	require.Error(t, err)
	require.Empty(t, fe.seenHost, "should reject before SSH")
}

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
