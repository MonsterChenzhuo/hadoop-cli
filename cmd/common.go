package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/hadoop-cli/hadoop-cli/internal/components"
	"github.com/hadoop-cli/hadoop-cli/internal/components/hbase"
	"github.com/hadoop-cli/hadoop-cli/internal/components/hdfs"
	"github.com/hadoop-cli/hadoop-cli/internal/components/zookeeper"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/hadoop-cli/hadoop-cli/internal/packages"
	"github.com/hadoop-cli/hadoop-cli/internal/runlog"
	sshx "github.com/hadoop-cli/hadoop-cli/internal/ssh"
	"github.com/spf13/cobra"
)

type runCtx struct {
	Inv           *inventory.Inventory
	InventoryPath string
	Runner        *orchestrator.Runner
	Pool          *sshx.Pool
	Env           components.Env
	Command       string
	Progress      *output.Progress
}

// envelope returns a new output.Envelope pre-populated with fields uniform
// across subcommands (currently: the resolved inventory path).
func (c *runCtx) envelope(command string) *output.Envelope {
	e := output.NewEnvelope(command)
	e.InventoryPath = c.InventoryPath
	return e
}

func prepare(cmd *cobra.Command, command string) (*runCtx, error) {
	invFlag, _ := cmd.Flags().GetString("inventory")
	noColor, _ := cmd.Flags().GetBool("no-color")

	invPath, source, err := inventory.Resolve(invFlag)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "using inventory: %s (%s)\n", invPath, source)

	inv, err := inventory.Load(invPath)
	if err != nil {
		return nil, err
	}
	if err := inventory.Validate(inv); err != nil {
		return nil, err
	}

	pool := sshx.NewPool(inv)
	exec := &orchestrator.SSHExecutor{Pool: pool}
	runner := orchestrator.NewRunner(exec, inv.SSH.Parallelism)

	run, err := runlog.New(runlog.DefaultRoot(), command)
	if err != nil {
		return nil, err
	}

	env := components.Env{
		Inv:    inv,
		Runner: runner,
		Cache:  packages.DefaultCacheDir(),
		Run:    run,
	}

	return &runCtx{
		Inv:           inv,
		InventoryPath: invPath,
		Runner:        runner,
		Pool:          pool,
		Env:           env,
		Command:       command,
		Progress:      output.NewProgress(os.Stderr, noColor),
	}, nil
}

func registry(forceFormat bool) map[string]components.Component {
	return map[string]components.Component{
		"zookeeper": zookeeper.ZooKeeper{},
		"hdfs":      hdfs.HDFS{ForceFormat: forceFormat},
		"hbase":     hbase.HBase{},
	}
}

// componentsForInv returns the components to act on, intersected with the
// components declared in the inventory and honoring --component filter and
// direction. If filter names a component not in the inventory, it returns an
// error so callers can surface a clear message instead of silently no-oping.
func componentsForInv(inv *inventory.Inventory, filter string, reverse bool, forceFormat bool) ([]components.Component, error) {
	reg := registry(forceFormat)
	order := components.Ordered()
	if reverse {
		order = components.ReverseOrdered()
	}
	if filter != "" && !inv.HasComponent(filter) {
		return nil, fmt.Errorf("--component %q is not declared in cluster.components %v", filter, inv.Cluster.Components)
	}
	var out []components.Component
	for _, name := range order {
		if !inv.HasComponent(name) {
			continue
		}
		if filter != "" && filter != name {
			continue
		}
		out = append(out, reg[name])
	}
	return out, nil
}

func aggregateOne(env *output.Envelope, r orchestrator.Result) {
	aggregate(env, []orchestrator.Result{r})
}

func aggregate(env *output.Envelope, results []orchestrator.Result) {
	for _, r := range results {
		host := output.HostResult{
			Host:      r.Host,
			OK:        r.OK,
			ElapsedMs: r.Elapsed.Milliseconds(),
		}
		if r.Err != nil {
			host.Message = r.Err.Error()
		} else if !r.OK {
			host.Message = fmt.Sprintf("exit=%d stderr=%s", r.ExitCode, r.Stderr)
		}
		env.AddHost(host)
	}
}

func allOK(rs []orchestrator.Result) bool {
	for _, r := range rs {
		if !r.OK {
			return false
		}
	}
	return true
}

func writeEnvelope(env *output.Envelope) {
	_ = env.Write(os.Stdout)
}

func errFromEnvelope(e *output.Envelope) error {
	if e.Error != nil {
		return fmt.Errorf("[%s] %s", e.Error.Code, e.Error.Message)
	}
	for _, h := range e.Hosts {
		if !h.OK {
			return fmt.Errorf("[%s] %s", h.Host, h.Message)
		}
	}
	return fmt.Errorf("command failed")
}

// backgroundCtx returns a fresh context for command execution; keeps cmd files tidy.
func backgroundCtx(_ *cobra.Command) context.Context { return context.Background() }
