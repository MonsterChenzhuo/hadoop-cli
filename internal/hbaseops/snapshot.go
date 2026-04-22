package hbaseops

import (
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/components/hbase"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
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

func validateIdent(field, v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if strings.ContainsAny(v, "'\n\r") {
		return fmt.Errorf("%s must not contain single quote or newline: %q", field, v)
	}
	return nil
}
