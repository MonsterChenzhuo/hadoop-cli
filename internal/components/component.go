package components

import (
	"context"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/hadoop-cli/hadoop-cli/internal/runlog"
)

type Env struct {
	Inv    *inventory.Inventory
	Runner *orchestrator.Runner
	Cache  string // local tarball cache dir
	Run    *runlog.Run
}

type Component interface {
	Name() string
	Hosts(inv *inventory.Inventory) []string
	Install(ctx context.Context, e Env) []orchestrator.Result
	Configure(ctx context.Context, e Env) []orchestrator.Result
	Start(ctx context.Context, e Env) []orchestrator.Result
	Stop(ctx context.Context, e Env) []orchestrator.Result
	Status(ctx context.Context, e Env) []orchestrator.Result
	Uninstall(ctx context.Context, e Env, purgeData bool) []orchestrator.Result
}

// Ordered returns components in forward dependency order (install/start).
func Ordered() []string { return []string{"zookeeper", "hdfs", "hbase"} }

// ReverseOrdered returns components in reverse order (stop/uninstall).
func ReverseOrdered() []string { return []string{"hbase", "hdfs", "zookeeper"} }
