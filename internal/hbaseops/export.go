package hbaseops

import (
	"context"
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/components/hbase"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

type ExportOptions struct {
	Name      string
	CopyTo    string
	Mappers   *int
	Bandwidth int
	Overwrite bool
	ExtraArgs string
}

// DeriveCopyToFromInventory returns hdfs://<namenode>:<rpc>/hbase for the
// given destination inventory. Requires exactly one NameNode; uses the
// inventory's NameNode RPC override, defaulting to 8020 when unset.
func DeriveCopyToFromInventory(inv *inventory.Inventory) (string, error) {
	if n := len(inv.Roles.NameNode); n != 1 {
		return "", fmt.Errorf("destination inventory must have exactly 1 roles.namenode; got %d", n)
	}
	rpc := inv.Overrides.HDFS.NameNodeRPC
	// Fallback for inventories not loaded via inventory.Load (e.g. constructed in tests);
	// the production Load path fills this default to 8020 already.
	if rpc == 0 {
		rpc = 8020
	}
	return fmt.Sprintf("hdfs://%s:%d/hbase", inv.Roles.NameNode[0], rpc), nil
}

// BuildExportCommand returns a bash script that runs
// `hbase org.apache.hadoop.hbase.snapshot.ExportSnapshot` on the target host.
func BuildExportCommand(inv *inventory.Inventory, opts ExportOptions) (string, error) {
	if err := validateIdent("name", opts.Name); err != nil {
		return "", err
	}
	if !strings.HasPrefix(opts.CopyTo, "hdfs://") {
		return "", fmt.Errorf("--to must start with hdfs://, got %q", opts.CopyTo)
	}
	for _, r := range opts.CopyTo {
		switch r {
		case ' ', '\t', '\n', '\r', ';', '|', '`', '$', '"', '\'', '\\', '<', '>', '&', '(', ')':
			return "", fmt.Errorf("--to contains unsafe shell metacharacter %q", r)
		}
	}
	if opts.Mappers != nil && *opts.Mappers < 0 {
		return "", fmt.Errorf("--mappers must be >= 0, got %d", *opts.Mappers)
	}
	if opts.Bandwidth < 0 {
		return "", fmt.Errorf("--bandwidth must be >= 0, got %d", opts.Bandwidth)
	}

	parts := []string{
		fmt.Sprintf("%s/bin/hbase org.apache.hadoop.hbase.snapshot.ExportSnapshot", hbase.Home(inv)),
		fmt.Sprintf("-snapshot %s", opts.Name),
		fmt.Sprintf("-copy-to %s", opts.CopyTo),
	}
	if opts.Mappers != nil {
		parts = append(parts, fmt.Sprintf("-mappers %d", *opts.Mappers))
	}
	if opts.Bandwidth > 0 {
		parts = append(parts, fmt.Sprintf("-bandwidth %d", opts.Bandwidth))
	}
	if opts.Overwrite {
		parts = append(parts, "-overwrite")
	}
	if strings.TrimSpace(opts.ExtraArgs) != "" {
		// appended unquoted; caller is responsible for safety
		parts = append(parts, opts.ExtraArgs)
	}

	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s
`, inv.Cluster.JavaHome, strings.Join(parts, " "))
	return script, nil
}

// ExportSnapshot runs `hbase ExportSnapshot` on the target host.
func ExportSnapshot(ctx context.Context, runner *orchestrator.Runner, inv *inventory.Inventory, opts ExportOptions, onHost string) (orchestrator.Result, error) {
	host, err := PickHost(inv, onHost)
	if err != nil {
		return orchestrator.Result{}, err
	}
	script, err := BuildExportCommand(inv, opts)
	if err != nil {
		return orchestrator.Result{}, err
	}
	results := runner.Run(ctx, []string{host}, orchestrator.Task{
		Name: "hbase-export-snapshot",
		Cmd:  script,
	})
	return results[0], nil
}
