package hbaseops

import (
	"context"
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/components/hbase"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

type SnapshotOptions struct {
	Table     string
	Name      string
	SkipFlush bool
}

// BuildSnapshotScript returns a bash script that runs `hbase shell` on the
// target host to take a snapshot. The script exits non-zero on failure.
func BuildSnapshotScript(inv *inventory.Inventory, opts SnapshotOptions) (string, error) {
	if err := validateIdent("table", opts.Table); err != nil {
		return "", err
	}
	if err := validateIdent("name", opts.Name); err != nil {
		return "", err
	}
	cmd := fmt.Sprintf("snapshot '%s','%s'", opts.Table, opts.Name)
	if opts.SkipFlush {
		cmd += ", {SKIP_FLUSH => true}"
	}
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
echo "%s" | %s/bin/hbase shell -n
`, inv.Cluster.JavaHome, cmd, hbase.Home(inv))
	return script, nil
}

// Snapshot runs an hbase shell `snapshot` command on the target host.
func Snapshot(ctx context.Context, runner *orchestrator.Runner, inv *inventory.Inventory, opts SnapshotOptions, onHost string) (orchestrator.Result, error) {
	host, err := PickHost(inv, onHost)
	if err != nil {
		return orchestrator.Result{}, err
	}
	script, err := BuildSnapshotScript(inv, opts)
	if err != nil {
		return orchestrator.Result{}, err
	}
	results := runner.Run(ctx, []string{host}, orchestrator.Task{
		Name: "hbase-snapshot",
		Cmd:  script,
	})
	return results[0], nil
}

func validateIdent(field, v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if strings.ContainsAny(v, "'\n\r") {
		return fmt.Errorf("%s must not contain single quote or newline: %q", field, v)
	}
	if strings.ContainsAny(v, " \t") {
		return fmt.Errorf("%s must not contain whitespace: %q", field, v)
	}
	return nil
}
