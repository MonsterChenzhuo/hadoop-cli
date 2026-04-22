# HBase Snapshot & ExportSnapshot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `hadoop-cli snapshot` and `hadoop-cli export-snapshot` subcommands that take an online HBase snapshot and sync it to a remote HDFS.

**Architecture:** New `internal/hbaseops` package with pure command-building plus a thin runner wrapper; two cobra subcommands wire CLI flags to the package. The existing `components.Component` interface is untouched. Docs are bilingual (English + 中文).

**Tech Stack:** Go, cobra, stretchr/testify, existing `orchestrator.Runner` / `SSHExecutor` / `inventory` / `components/hbase` packages.

---

## File structure

Created:

- `internal/hbaseops/host.go` — host selection (`PickHost`).
- `internal/hbaseops/host_test.go`
- `internal/hbaseops/snapshot.go` — `SnapshotOptions`, `BuildSnapshotScript`, `Snapshot`.
- `internal/hbaseops/snapshot_test.go`
- `internal/hbaseops/export.go` — `ExportOptions`, `DeriveCopyToFromInventory`, `BuildExportCommand`, `ExportSnapshot`.
- `internal/hbaseops/export_test.go`
- `cmd/snapshot.go`
- `cmd/snapshot_test.go`
- `cmd/export_snapshot.go`
- `cmd/export_snapshot_test.go`
- `docs/snapshot.md`
- `docs/snapshot.zh-CN.md`

Modified:

- `cmd/root.go` — register two new commands.
- `README.md` — add rows to the Commands table + new Quick start subsection.
- `skills/hbase-cluster-ops/SKILL.md` — add bilingual Snapshot section.

---

## Task 1: Host selection helper

**Files:**
- Create: `internal/hbaseops/host.go`
- Test: `internal/hbaseops/host_test.go`

Responsibility: given an inventory and an optional `--on` override, return the host to SSH into (default: first HBase master) or an error.

- [ ] **Step 1: Write failing tests**

```go
// internal/hbaseops/host_test.go
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
```

- [ ] **Step 2: Run tests, expect failure**

Run: `go test ./internal/hbaseops/ -run TestPickHost -v`
Expected: FAIL (package missing / `PickHost` undefined).

- [ ] **Step 3: Implement `PickHost`**

```go
// internal/hbaseops/host.go
package hbaseops

import (
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

// PickHost returns the host to run the hbase command on.
// If override is empty, it defaults to the first HBase master.
// If override is set, it must appear in the inventory's role hosts.
func PickHost(inv *inventory.Inventory, override string) (string, error) {
	if override != "" {
		for _, h := range inv.AllRoleHosts() {
			if h == override {
				return override, nil
			}
		}
		return "", fmt.Errorf("--on host %q is not in the inventory", override)
	}
	if len(inv.Roles.HBaseMaster) == 0 {
		return "", fmt.Errorf("inventory has no roles.hbase_master; pass --on <host> or fix the inventory")
	}
	return inv.Roles.HBaseMaster[0], nil
}
```

- [ ] **Step 4: Run tests, expect pass**

Run: `go test ./internal/hbaseops/ -run TestPickHost -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hbaseops/host.go internal/hbaseops/host_test.go
git commit -m "feat(hbaseops): add PickHost for snapshot/export target selection"
```

---

## Task 2: Snapshot script builder

**Files:**
- Create: `internal/hbaseops/snapshot.go`
- Test: `internal/hbaseops/snapshot_test.go`

Responsibility: pure function that validates inputs and builds the shell script to run on the HBase master.

- [ ] **Step 1: Write failing tests**

```go
// internal/hbaseops/snapshot_test.go
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
```

- [ ] **Step 2: Run tests, expect failure**

Run: `go test ./internal/hbaseops/ -run TestBuildSnapshotScript -v`
Expected: FAIL (undefined `BuildSnapshotScript`, `SnapshotOptions`).

- [ ] **Step 3: Implement**

```go
// internal/hbaseops/snapshot.go
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
```

- [ ] **Step 4: Run tests, expect pass**

Run: `go test ./internal/hbaseops/ -run TestBuildSnapshotScript -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hbaseops/snapshot.go internal/hbaseops/snapshot_test.go
git commit -m "feat(hbaseops): add BuildSnapshotScript with injection guard"
```

---

## Task 3: ExportSnapshot command builder

**Files:**
- Create: `internal/hbaseops/export.go`
- Test: `internal/hbaseops/export_test.go`

Responsibility: pure helpers that (a) derive a `hdfs://...` copyTo from a destination inventory and (b) build the `hbase ExportSnapshot` command.

- [ ] **Step 1: Write failing tests**

```go
// internal/hbaseops/export_test.go
package hbaseops

import (
	"strings"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/stretchr/testify/require"
)

func destInv(nn string, rpc int) *inventory.Inventory {
	inv := invWithHosts([]string{"nm"}, []string{"rs1"})
	inv.Cluster = inventory.Cluster{InstallDir: "/opt/hadoop", JavaHome: "/j"}
	inv.Roles.NameNode = []string{nn}
	inv.Overrides.HDFS.NameNodeRPC = rpc
	return inv
}

func TestDeriveCopyToFromInventory_UsesDefaultRPC(t *testing.T) {
	inv := destInv("node9", 0) // zero means "not set"; default is 8020
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
	require.Contains(t, cmd, "export JAVA_HOME=/j")
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

func TestBuildExportCommand_RejectsNegativeMappers(t *testing.T) {
	neg := -1
	_, err := BuildExportCommand(invFull(), ExportOptions{
		Name: "s", CopyTo: "hdfs://h/x", Mappers: &neg,
	})
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests, expect failure**

Run: `go test ./internal/hbaseops/ -run "TestDeriveCopyToFromInventory|TestBuildExportCommand" -v`
Expected: FAIL (undefined symbols).

- [ ] **Step 3: Implement**

```go
// internal/hbaseops/export.go
package hbaseops

import (
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/components/hbase"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
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
		parts = append(parts, opts.ExtraArgs)
	}

	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s
`, inv.Cluster.JavaHome, strings.Join(parts, " "))
	return script, nil
}
```

- [ ] **Step 4: Run tests, expect pass**

Run: `go test ./internal/hbaseops/ -run "TestDeriveCopyToFromInventory|TestBuildExportCommand" -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/hbaseops/export.go internal/hbaseops/export_test.go
git commit -m "feat(hbaseops): add BuildExportCommand and DeriveCopyToFromInventory"
```

---

## Task 4: Runner wrappers (Snapshot / ExportSnapshot functions)

**Files:**
- Modify: `internal/hbaseops/snapshot.go`
- Modify: `internal/hbaseops/export.go`
- Test: `internal/hbaseops/snapshot_test.go`
- Test: `internal/hbaseops/export_test.go`

Responsibility: thin wrappers that pick the host, build the script, and dispatch via `orchestrator.Runner`. Tested with a fake executor.

- [ ] **Step 1: Add failing tests using a fake executor**

Append to `internal/hbaseops/snapshot_test.go`:

```go
import (
	"context"
	// existing imports plus:
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
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
```

Append to `internal/hbaseops/export_test.go`:

```go
import (
	"context"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
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
```

- [ ] **Step 2: Run tests, expect failure**

Run: `go test ./internal/hbaseops/ -v`
Expected: FAIL (`Snapshot`, `ExportSnapshot` undefined).

- [ ] **Step 3: Implement wrappers**

Append to `internal/hbaseops/snapshot.go`:

```go
import (
	"context"
	// existing imports plus:
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

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
```

Append to `internal/hbaseops/export.go`:

```go
import (
	"context"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

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
```

(Merge the new imports into each file's existing `import (...)` block.)

- [ ] **Step 4: Run tests, expect pass**

Run: `go test ./internal/hbaseops/ -v`
Expected: PASS (all tests).

- [ ] **Step 5: Commit**

```bash
git add internal/hbaseops/snapshot.go internal/hbaseops/snapshot_test.go \
        internal/hbaseops/export.go internal/hbaseops/export_test.go
git commit -m "feat(hbaseops): add Snapshot/ExportSnapshot runner wrappers"
```

---

## Task 5: `hadoop-cli snapshot` cobra command

**Files:**
- Create: `cmd/snapshot.go`
- Create: `cmd/snapshot_test.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: Write failing test for command registration**

```go
// cmd/snapshot_test.go
package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoot_RegistersSnapshotCommand(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())
	require.Contains(t, buf.String(), "snapshot")
}

func TestSnapshot_HelpListsRequiredFlags(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"snapshot", "--help"})
	require.NoError(t, root.Execute())
	help := buf.String()
	require.Contains(t, help, "--table")
	require.Contains(t, help, "--name")
	require.Contains(t, help, "--skip-flush")
	require.Contains(t, help, "--on")
}
```

- [ ] **Step 2: Run test, expect failure**

Run: `go test ./cmd/ -run "Snapshot|RegistersSnapshot" -v`
Expected: FAIL (`snapshot` not in help).

- [ ] **Step 3: Implement command**

```go
// cmd/snapshot.go
package cmd

import (
	"github.com/hadoop-cli/hadoop-cli/internal/hbaseops"
	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newSnapshotCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "snapshot",
		Short: "Take an online HBase snapshot via hbase shell.",
		Long:  "Connects to an HBase master over SSH and runs `snapshot '<table>','<name>'` in hbase shell. Runs online by default; pass --skip-flush to skip the memstore flush.",
		Example: `  # English: snapshot the users table.
  # 中文: 对 users 表做快照。
  hadoop-cli snapshot --inventory cluster.yaml --table default:users --name users_20260422`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "snapshot")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()

			table, _ := cmd.Flags().GetString("table")
			name, _ := cmd.Flags().GetString("name")
			skipFlush, _ := cmd.Flags().GetBool("skip-flush")
			onHost, _ := cmd.Flags().GetString("on")

			ctx := backgroundCtx(cmd)
			res, err := hbaseops.Snapshot(ctx, rc.Runner, rc.Inv, hbaseops.SnapshotOptions{
				Table:     table,
				Name:      name,
				SkipFlush: skipFlush,
			}, onHost)
			if err != nil {
				return err
			}
			env := output.NewEnvelope("snapshot").WithRunID(rc.Env.Run.ID)
			aggregateOne(env, res)
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(env)
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("table", "", "HBase table name, e.g. namespace:table (required)")
	c.Flags().String("name", "", "snapshot name (required)")
	c.Flags().Bool("skip-flush", false, "do not flush memstore before snapshotting")
	c.Flags().String("on", "", "host to run hbase shell on (default: first hbase_master)")
	_ = c.MarkFlagRequired("table")
	_ = c.MarkFlagRequired("name")
	return c
}
```

Add a helper in `cmd/common.go` (modify the file) so both new commands share the single-result aggregation:

```go
// Append to cmd/common.go
func aggregateOne(env *output.Envelope, r orchestrator.Result) {
	aggregate(env, []orchestrator.Result{r})
}
```

Register in `cmd/root.go`:

```go
// Modify cmd/root.go NewRootCmd(): add after the existing AddCommand calls
root.AddCommand(newSnapshotCmd())
```

- [ ] **Step 4: Run tests, expect pass**

Run: `go test ./cmd/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/snapshot.go cmd/snapshot_test.go cmd/common.go cmd/root.go
git commit -m "feat(cmd): add snapshot subcommand"
```

---

## Task 6: `hadoop-cli export-snapshot` cobra command

**Files:**
- Create: `cmd/export_snapshot.go`
- Create: `cmd/export_snapshot_test.go`
- Modify: `cmd/root.go`

- [ ] **Step 1: Write failing tests**

```go
// cmd/export_snapshot_test.go
package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoot_RegistersExportSnapshotCommand(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())
	require.Contains(t, buf.String(), "export-snapshot")
}

func TestExportSnapshot_HelpListsFlags(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"export-snapshot", "--help"})
	require.NoError(t, root.Execute())
	help := buf.String()
	for _, s := range []string{"--name", "--to", "--to-inventory", "--mappers", "--bandwidth", "--overwrite", "--extra-args", "--on"} {
		require.Containsf(t, help, s, "help should mention %s", s)
	}
}

func TestExportSnapshot_RejectsToAndToInventoryTogether(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{
		"export-snapshot",
		"--name", "s",
		"--to", "hdfs://a/b",
		"--to-inventory", "dst.yaml",
	})
	err := root.Execute()
	require.Error(t, err)
}

func TestExportSnapshot_RejectsNeitherToNorToInventory(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"export-snapshot", "--name", "s"})
	err := root.Execute()
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests, expect failure**

Run: `go test ./cmd/ -run "ExportSnapshot|RegistersExportSnapshot" -v`
Expected: FAIL.

- [ ] **Step 3: Implement command**

```go
// cmd/export_snapshot.go
package cmd

import (
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/hbaseops"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newExportSnapshotCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "export-snapshot",
		Short: "Copy an HBase snapshot to a remote HDFS via hbase ExportSnapshot.",
		Long:  "Runs `hbase org.apache.hadoop.hbase.snapshot.ExportSnapshot` on an HBase master. Without YARN the job falls back to LocalJobRunner; tune with --mappers / --bandwidth for larger snapshots.",
		Example: `  # English: export with a literal HDFS URL.
  # 中文: 直接用 HDFS URL 同步。
  hadoop-cli export-snapshot --inventory cluster.yaml \
    --name rta_tag_by_uid_1030 --to hdfs://10.57.1.211:8020/hbase

  # English: derive the URL from the destination cluster.yaml.
  # 中文: 从目标集群 inventory 推导 URL。
  hadoop-cli export-snapshot --inventory src.yaml \
    --name rta_tag_by_uid_1030 --to-inventory dst.yaml`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "export-snapshot")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()

			name, _ := cmd.Flags().GetString("name")
			to, _ := cmd.Flags().GetString("to")
			toInv, _ := cmd.Flags().GetString("to-inventory")
			bandwidth, _ := cmd.Flags().GetInt("bandwidth")
			overwrite, _ := cmd.Flags().GetBool("overwrite")
			extra, _ := cmd.Flags().GetString("extra-args")
			onHost, _ := cmd.Flags().GetString("on")

			opts := hbaseops.ExportOptions{
				Name:      name,
				Bandwidth: bandwidth,
				Overwrite: overwrite,
				ExtraArgs: extra,
			}
			if cmd.Flags().Changed("mappers") {
				m, _ := cmd.Flags().GetInt("mappers")
				opts.Mappers = &m
			}

			switch {
			case to != "" && toInv != "":
				return fmt.Errorf("--to and --to-inventory are mutually exclusive")
			case to == "" && toInv == "":
				return fmt.Errorf("one of --to or --to-inventory is required")
			case to != "":
				opts.CopyTo = to
			default:
				dst, err := inventory.Load(toInv)
				if err != nil {
					return fmt.Errorf("load --to-inventory: %w", err)
				}
				url, err := hbaseops.DeriveCopyToFromInventory(dst)
				if err != nil {
					return err
				}
				opts.CopyTo = url
			}

			ctx := backgroundCtx(cmd)
			res, err := hbaseops.ExportSnapshot(ctx, rc.Runner, rc.Inv, opts, onHost)
			if err != nil {
				return err
			}
			env := output.NewEnvelope("export-snapshot").WithRunID(rc.Env.Run.ID)
			aggregateOne(env, res)
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(env)
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("name", "", "snapshot name (required)")
	c.Flags().String("to", "", "destination HDFS URL, e.g. hdfs://nn:8020/hbase")
	c.Flags().String("to-inventory", "", "path to a destination cluster.yaml; derives hdfs://<nn>:<rpc>/hbase")
	c.Flags().Int("mappers", 0, "number of mappers (pass 0 for LocalJobRunner); unset = HBase default")
	c.Flags().Int("bandwidth", 0, "per-mapper bandwidth limit in MB/s (0 = unlimited)")
	c.Flags().Bool("overwrite", false, "overwrite destination snapshot if it already exists")
	c.Flags().String("extra-args", "", "raw args appended to the hbase ExportSnapshot command")
	c.Flags().String("on", "", "host to run on (default: first hbase_master)")
	_ = c.MarkFlagRequired("name")
	return c
}
```

Register in `cmd/root.go`:

```go
// add after newSnapshotCmd registration
root.AddCommand(newExportSnapshotCmd())
```

- [ ] **Step 4: Run tests, expect pass**

Run: `go test ./cmd/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/export_snapshot.go cmd/export_snapshot_test.go cmd/root.go
git commit -m "feat(cmd): add export-snapshot subcommand"
```

---

## Task 7: Build + whole-repo tests

- [ ] **Step 1: Build**

Run: `make build`
Expected: binary built at `bin/hadoop-cli` with no errors.

- [ ] **Step 2: Run all tests**

Run: `go test ./...`
Expected: PASS across every package.

- [ ] **Step 3: Commit if any formatting touch-ups**

```bash
git status
# if gofmt produced changes (it shouldn't, but just in case)
git add -u && git commit -m "chore: gofmt"
```

---

## Task 8: Bilingual documentation

**Files:**
- Create: `docs/snapshot.md`
- Create: `docs/snapshot.zh-CN.md`
- Modify: `README.md`

- [ ] **Step 1: Add a row and subsection to `README.md`**

Modify the Commands table in `README.md` — add these two rows to the existing `| Command | What it does |` block:

```markdown
| snapshot        | Take an online HBase snapshot via hbase shell |
| export-snapshot | Sync a snapshot to a remote HDFS via hbase ExportSnapshot |
```

Then, after the `## Commands` section, append:

````markdown
## Snapshot & sync / 快照与同步

```bash
# Create a snapshot / 创建快照
hadoop-cli snapshot --inventory cluster.yaml \
    --table rta:tag_by_uid --name rta_tag_by_uid_1030

# Export to a remote HDFS URL / 同步到远端 HDFS
hadoop-cli export-snapshot --inventory cluster.yaml \
    --name rta_tag_by_uid_1030 --to hdfs://10.57.1.211:8020/hbase

# Export using the destination cluster.yaml / 用目标集群 inventory 推导地址
hadoop-cli export-snapshot --inventory src.yaml \
    --name rta_tag_by_uid_1030 --to-inventory dst.yaml
```

See [docs/snapshot.md](docs/snapshot.md) (English) or
[docs/snapshot.zh-CN.md](docs/snapshot.zh-CN.md) (中文) for full details.
````

- [ ] **Step 2: Create `docs/snapshot.md` (English)**

````markdown
# HBase Snapshot & ExportSnapshot

> 中文版: [snapshot.zh-CN.md](snapshot.zh-CN.md)

`hadoop-cli` wraps two common HBase snapshot operations so Claude Code can
drive them from one user request:

1. `hadoop-cli snapshot` — take an online snapshot of a table.
2. `hadoop-cli export-snapshot` — copy a snapshot to a remote HDFS.

Both commands SSH into an HBase master (first entry of
`roles.hbase_master`) and run the corresponding `bin/hbase` call there.

## `hadoop-cli snapshot`

| Flag          | Required | Description                                                                 |
|---------------|----------|-----------------------------------------------------------------------------|
| `--table`     | yes      | HBase table, e.g. `rta:tag_by_uid`.                                         |
| `--name`      | yes      | Snapshot name (must not contain `'` or newline).                            |
| `--skip-flush`| no       | Pass `{SKIP_FLUSH => true}` to avoid flushing memstore.                     |
| `--on`        | no       | Run on a specific host instead of the first `hbase_master`.                 |

Example:

```bash
hadoop-cli snapshot --inventory cluster.yaml \
    --table rta:tag_by_uid --name rta_tag_by_uid_1030
```

Runs:

```
echo "snapshot 'rta:tag_by_uid','rta_tag_by_uid_1030'" | $HBASE_HOME/bin/hbase shell -n
```

## `hadoop-cli export-snapshot`

| Flag              | Required | Description                                                                 |
|-------------------|----------|-----------------------------------------------------------------------------|
| `--name`          | yes      | Snapshot name to copy.                                                      |
| `--to`            | one of   | Destination HDFS URL, must start with `hdfs://`.                            |
| `--to-inventory`  | one of   | Path to another `cluster.yaml`; derives `hdfs://<nn>:<rpc>/hbase` from it.  |
| `--mappers`       | no       | Number of mappers. Omit for HBase default; `0` for LocalJobRunner.          |
| `--bandwidth`     | no       | Per-mapper MB/s cap. `0` means unlimited.                                   |
| `--overwrite`     | no       | Overwrite existing snapshot at the destination.                             |
| `--extra-args`    | no       | Raw args appended to the command.                                           |
| `--on`            | no       | Host override, same as `snapshot`.                                          |

`--to` and `--to-inventory` are mutually exclusive; exactly one is required.

Examples:

```bash
# URL mode — matches the raw hbase invocation.
hadoop-cli export-snapshot --inventory cluster.yaml \
    --name rta_tag_by_uid_1030 --to hdfs://10.57.1.211:8020/hbase

# Inventory mode — derives the URL from dst.yaml's roles.namenode and
# overrides.hdfs.namenode_rpc_port (default 8020).
hadoop-cli export-snapshot --inventory src.yaml \
    --name rta_tag_by_uid_1030 --to-inventory dst.yaml
```

### Flag mapping to native `ExportSnapshot`

| hadoop-cli flag   | native `hbase ExportSnapshot` flag |
|-------------------|-------------------------------------|
| `--name`          | `-snapshot`                         |
| `--to` / `--to-inventory` | `-copy-to`                  |
| `--mappers N`     | `-mappers N`                        |
| `--bandwidth N`   | `-bandwidth N`                      |
| `--overwrite`     | `-overwrite`                        |
| `--extra-args`    | appended raw                        |

## Performance note: LocalJobRunner

This CLI provisions clusters without YARN. `ExportSnapshot` will fall back
to Hadoop's `LocalJobRunner`, which copies in-process and is fine for small
or medium snapshots but slow for large ones. Tune `--mappers` / `--bandwidth`
or attach a YARN cluster manually via `--extra-args "-D ..."` for large jobs.

## Common errors

- `--to must start with hdfs://` — you passed a local path. Use
  `--to hdfs://...` or `--to-inventory <yaml>`.
- `destination inventory must have exactly 1 roles.namenode` — the target
  `cluster.yaml` has multiple NameNodes (HA) or zero; HA isn't supported
  here yet, use `--to` directly.
- `--on host "X" is not in the inventory` — typo in the hostname; it must
  match a name declared under `hosts:` in the source inventory.
- `Snapshot 'X' already exists` (from HBase) — pick a new name or
  re-run with `--overwrite` (export only) after deleting via `hbase shell`.
````

- [ ] **Step 3: Create `docs/snapshot.zh-CN.md` (中文)**

````markdown
# HBase 快照与同步

> English: [snapshot.md](snapshot.md)

`hadoop-cli` 封装了两个常用的 HBase 快照动作，让 Claude Code 在一次请求里
就能完成：

1. `hadoop-cli snapshot` — 对一张表做在线快照。
2. `hadoop-cli export-snapshot` — 把快照同步到远端 HDFS。

两条命令都会 SSH 到 HBase master（`roles.hbase_master` 的第 1 台），在那
里调用对应的 `bin/hbase`。

## `hadoop-cli snapshot`

| Flag          | 必填 | 说明                                                         |
|---------------|------|--------------------------------------------------------------|
| `--table`     | 是   | HBase 表名，例如 `rta:tag_by_uid`。                          |
| `--name`      | 是   | 快照名（不能包含 `'` 或换行）。                              |
| `--skip-flush`| 否   | 追加 `{SKIP_FLUSH => true}`，跳过 memstore flush。           |
| `--on`        | 否   | 指定执行节点，默认 `roles.hbase_master[0]`。                 |

示例：

```bash
hadoop-cli snapshot --inventory cluster.yaml \
    --table rta:tag_by_uid --name rta_tag_by_uid_1030
```

实际执行：

```
echo "snapshot 'rta:tag_by_uid','rta_tag_by_uid_1030'" | $HBASE_HOME/bin/hbase shell -n
```

## `hadoop-cli export-snapshot`

| Flag              | 必填 | 说明                                                                |
|-------------------|------|---------------------------------------------------------------------|
| `--name`          | 是   | 要同步的快照名。                                                    |
| `--to`            | 二选一 | 目标 HDFS URL，必须以 `hdfs://` 开头。                            |
| `--to-inventory`  | 二选一 | 目标集群的 `cluster.yaml`；从中推导 `hdfs://<nn>:<rpc>/hbase`。   |
| `--mappers`       | 否   | mapper 数量。不传 = 用 HBase 默认；`0` = LocalJobRunner。           |
| `--bandwidth`     | 否   | 单个 mapper 的 MB/s 限速，`0` 表示不限速。                          |
| `--overwrite`     | 否   | 覆盖目标已有快照。                                                  |
| `--extra-args`    | 否   | 原样追加到命令末尾。                                                |
| `--on`            | 否   | 指定执行节点，同 `snapshot`。                                       |

`--to` 与 `--to-inventory` 互斥，必须二选一。

示例：

```bash
# URL 模式 —— 等价于直接调 hbase 原生命令
hadoop-cli export-snapshot --inventory cluster.yaml \
    --name rta_tag_by_uid_1030 --to hdfs://10.57.1.211:8020/hbase

# Inventory 模式 —— 从 dst.yaml 的 roles.namenode 和
# overrides.hdfs.namenode_rpc_port（默认 8020）推导 URL
hadoop-cli export-snapshot --inventory src.yaml \
    --name rta_tag_by_uid_1030 --to-inventory dst.yaml
```

### 参数映射到原生 `ExportSnapshot`

| hadoop-cli 参数   | 原生 `hbase ExportSnapshot` 参数  |
|-------------------|-----------------------------------|
| `--name`          | `-snapshot`                       |
| `--to` / `--to-inventory` | `-copy-to`                |
| `--mappers N`     | `-mappers N`                      |
| `--bandwidth N`   | `-bandwidth N`                    |
| `--overwrite`     | `-overwrite`                      |
| `--extra-args`    | 原样追加                          |

## 性能说明：LocalJobRunner

本 CLI 搭出来的集群不含 YARN，`ExportSnapshot` 会自动退回 Hadoop 的
`LocalJobRunner` 进程内单机拷贝。小中快照够用，大快照会慢。需要时用
`--mappers` / `--bandwidth` 调参，或通过 `--extra-args "-D ..."` 接入外部
YARN。

## 常见错误

- `--to must start with hdfs://` —— 传了本地路径。请用 `--to hdfs://...`
  或 `--to-inventory <yaml>`。
- `destination inventory must have exactly 1 roles.namenode` —— 目标
  `cluster.yaml` 的 NameNode 数量不是 1（可能是 HA）。HA 暂不支持，改用
  `--to` 直接传 URL。
- `--on host "X" is not in the inventory` —— 主机名拼错；必须匹配源
  inventory `hosts:` 里声明过的名字。
- HBase 报 `Snapshot 'X' already exists` —— 换名字，或先手工 `hbase shell`
  删除后再跑；export 可加 `--overwrite`。
````

- [ ] **Step 4: Verify markdown renders sanely**

Run: `grep -E "^#|^\|" docs/snapshot.md docs/snapshot.zh-CN.md | head`
Expected: headings and table lines appear.

- [ ] **Step 5: Commit**

```bash
git add README.md docs/snapshot.md docs/snapshot.zh-CN.md
git commit -m "docs: bilingual guide for snapshot and export-snapshot"
```

---

## Task 9: Update `hbase-cluster-ops` skill

**Files:**
- Modify: `skills/hbase-cluster-ops/SKILL.md`

- [ ] **Step 1: Add two rows to the Commands table and a new section**

Modify `skills/hbase-cluster-ops/SKILL.md`. In the `## Commands you run here` table, add these rows before the `## Rules of engagement` heading:

```markdown
| 打快照 / take a snapshot           | `hadoop-cli snapshot --inventory cluster.yaml --table <ns:t> --name <snap>` |
| 同步快照到 B 集群 / sync snapshot   | `hadoop-cli export-snapshot --inventory cluster.yaml --name <snap> --to hdfs://<nn>:8020/hbase` |
| 同步到 B 集群 (已有 inventory)     | `hadoop-cli export-snapshot --inventory src.yaml --name <snap> --to-inventory dst.yaml` |
```

Then append this section after `## Rules of engagement`:

```markdown
## 快照 / Snapshots

用户说"给 X 表打个快照"/"把这个快照同步到集群 B" → 用
`hadoop-cli snapshot` 和 `hadoop-cli export-snapshot`。

- 用户只给一个 HDFS 地址 → `--to hdfs://...`.
- 用户提到目标集群的 `cluster.yaml` 路径 → `--to-inventory <path>`.
- 默认 `roles.hbase_master[0]` 执行；如果用户说"在 nodeX 上跑"再用
  `--on nodeX`。
- 目标集群必须只有 1 个 NameNode（当前单 NN 形态）。
- Cluster has no YARN; large snapshots will be slow under LocalJobRunner.
  Warn the user and offer `--mappers` / `--bandwidth` tuning.
```

- [ ] **Step 2: Commit**

```bash
git add skills/hbase-cluster-ops/SKILL.md
git commit -m "docs(skill): hbase-cluster-ops covers snapshot + export-snapshot"
```

---

## Done criteria

- `go test ./...` passes.
- `make build` produces a binary that exposes `snapshot` and
  `export-snapshot` in `--help`.
- `docs/snapshot.md` and `docs/snapshot.zh-CN.md` both render.
- `skills/hbase-cluster-ops/SKILL.md` lists the two new commands.
