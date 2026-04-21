package preflight

import (
	"context"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/stretchr/testify/require"
)

type fakeExec struct {
	byCmd map[string]orchestrator.Result
}

func (f *fakeExec) Execute(_ context.Context, host string, task orchestrator.Task) orchestrator.Result {
	if r, ok := f.byCmd[task.Name]; ok {
		r.Host = host
		return r
	}
	return orchestrator.Result{Host: host, OK: true}
}

func baseInv() *inventory.Inventory {
	return &inventory.Inventory{
		Cluster: inventory.Cluster{
			DataDir:    "/data/hadoop-cli",
			JavaHome:   "/j",
			Components: []string{"zookeeper", "hdfs", "hbase"},
		},
		Hosts: []inventory.Host{
			{Name: "n1", Address: "10.0.0.1"},
		},
		Roles: inventory.Roles{
			NameNode: []string{"n1"}, DataNode: []string{"n1"},
			ZooKeeper: []string{"n1"}, HBaseMaster: []string{"n1"}, RegionServer: []string{"n1"},
		},
		Overrides: inventory.Overrides{
			HDFS:      inventory.HDFSOverrides{NameNodeRPC: 8020, NameNodeHTTP: 9870},
			ZooKeeper: inventory.ZKOverrides{ClientPort: 2181},
			HBase:     inventory.HBaseOverrides{MasterPort: 16000, MasterInfoPort: 16010, RSPort: 16020, RSInfoPort: 16030},
		},
	}
}

func TestRun_PassesWhenAllChecksOK(t *testing.T) {
	fe := &fakeExec{byCmd: map[string]orchestrator.Result{
		"preflight-jdk":   {OK: true, Stdout: "java version \"11.0.1\""},
		"preflight-ports": {OK: true},
		"preflight-disk":  {OK: true, Stdout: "20G"},
		"preflight-clock": {OK: true, Stdout: "0"},
	}}
	runner := orchestrator.NewRunner(fe, 2)
	rep, err := Run(context.Background(), baseInv(), runner)
	require.NoError(t, err)
	require.True(t, rep.OK)
}

func TestPortsToCheck_ZKOnly(t *testing.T) {
	inv := baseInv()
	inv.Cluster.Components = []string{"zookeeper"}
	require.Equal(t, []int{2181}, portsToCheck(inv))
}

func TestPortsToCheck_FullStack(t *testing.T) {
	require.Equal(t,
		[]int{2181, 8020, 9870, 16000, 16010, 16020, 16030},
		portsToCheck(baseInv()),
	)
}

func TestRun_FailsWhenJDKMissing(t *testing.T) {
	fe := &fakeExec{byCmd: map[string]orchestrator.Result{
		"preflight-jdk": {OK: false, Stderr: "java: not found", ExitCode: 127},
	}}
	runner := orchestrator.NewRunner(fe, 2)
	rep, err := Run(context.Background(), baseInv(), runner)
	require.Error(t, err)
	require.False(t, rep.OK)
	require.Contains(t, err.Error(), "PREFLIGHT_JDK_MISSING")
}
