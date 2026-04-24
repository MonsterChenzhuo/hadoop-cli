# Inventory Default Lookup Chain — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users omit `--inventory` when there's a single obvious cluster file — resolve it from a fixed lookup chain instead, so `hadoop-cli status` works without arguments.

**Architecture:** Add a `inventory.Resolve(flag)` function that returns the first inventory path hit in the order `flag → $HADOOPCLI_INVENTORY → ./cluster.yaml → ~/.hadoop-cli/cluster.yaml`, along with a human-readable source label. Change the root command's `--inventory` flag default from the magic string `"cluster.yaml"` to `""` so `prepare()` can tell "user didn't pass one" from "user explicitly said cluster.yaml", call the resolver in `prepare()`, emit the resolved path to stderr, and echo it back in the JSON envelope as `inventory_path`. The `--to-inventory` flag on `export-snapshot` stays explicit — it's a second inventory, not the primary one, so the resolver does not apply.

**Tech Stack:** Go 1.25, `cobra`, `testify`, YAML inventory under `~/.hadoop-cli/`.

**Non-goals:**
- No `config use/list/current` context-switching command.
- No `hadoop-cli init` wizard.
- No change to `--to-inventory` semantics in `export-snapshot`.
- No change to `inventory.Load` / `Validate`.

---

## File Structure

**New:**
- `internal/inventory/resolve.go` — pure resolver: `Resolve(flag string) (path, source string, err error)`.
- `internal/inventory/resolve_test.go` — unit tests covering each resolution rung.

**Modified:**
- `cmd/root.go:19` — flag default `"cluster.yaml"` → `""`, help text updated.
- `cmd/common.go:30-40` — `prepare()` calls `inventory.Resolve`, stores path on `runCtx`, prints "using inventory: <path>" to stderr.
- `cmd/common.go` (new field on `runCtx`) — `InventoryPath string` so subcommands can thread it into the envelope.
- `cmd/preflight.go`, `install.go`, `configure.go`, `start.go`, `stop.go`, `status.go`, `uninstall.go`, `snapshot.go`, `export_snapshot.go` — populate `env.InventoryPath` via a new `envelopeFor(ctx)` helper in `common.go` (don't hand-edit every file; centralize once).
- `internal/output/envelope.go` — add `InventoryPath string \`json:"inventory_path,omitempty"\`` field.
- `README.md`, `README.zh-CN.md` — document the lookup chain and drop `--inventory cluster.yaml` from the quick-start examples.
- `skills/hbase-cluster-bootstrap/SKILL.md`, `skills/hbase-cluster-ops/SKILL.md` — drop `--inventory cluster.yaml` from examples where the default resolves correctly; keep it in multi-cluster flows (e.g., `export-snapshot`).
- `CLAUDE.md` — one-line mention under "Inventory" that a default lookup chain exists.

---

## Task 1: Resolver core + tests

**Files:**
- Create: `internal/inventory/resolve.go`
- Create: `internal/inventory/resolve_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/inventory/resolve_test.go
package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolve_FlagWins(t *testing.T) {
	t.Setenv("HADOOPCLI_INVENTORY", "/env/path.yaml")
	t.Setenv("HOME", t.TempDir())

	path, src, err := Resolve("explicit.yaml")
	require.NoError(t, err)
	require.Equal(t, "explicit.yaml", path)
	require.Equal(t, "flag", src)
}

func TestResolve_EnvOverridesCWDAndHome(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cluster.yaml"), []byte("x"), 0o644))
	t.Setenv("HADOOPCLI_INVENTORY", "/env/path.yaml")

	path, src, err := Resolve("")
	require.NoError(t, err)
	require.Equal(t, "/env/path.yaml", path)
	require.Equal(t, "env:HADOOPCLI_INVENTORY", src)
}

func TestResolve_CWDBeatsHome(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	t.Setenv("HOME", home)
	t.Chdir(cwd)

	require.NoError(t, os.MkdirAll(filepath.Join(home, ".hadoop-cli"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, ".hadoop-cli", "cluster.yaml"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(cwd, "cluster.yaml"), []byte("x"), 0o644))

	path, src, err := Resolve("")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(cwd, "cluster.yaml"), path)
	require.Equal(t, "cwd", src)
}

func TestResolve_HomeFallback(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir() // no cluster.yaml here
	t.Setenv("HOME", home)
	t.Chdir(cwd)

	want := filepath.Join(home, ".hadoop-cli", "cluster.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(want), 0o755))
	require.NoError(t, os.WriteFile(want, []byte("x"), 0o644))

	path, src, err := Resolve("")
	require.NoError(t, err)
	require.Equal(t, want, path)
	require.Equal(t, "home", src)
}

func TestResolve_NothingFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv("HADOOPCLI_INVENTORY", "")

	_, _, err := Resolve("")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no inventory found")
	require.Contains(t, err.Error(), "HADOOPCLI_INVENTORY")
	require.Contains(t, err.Error(), "./cluster.yaml")
	require.Contains(t, err.Error(), "~/.hadoop-cli/cluster.yaml")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/inventory -run TestResolve -race -v`
Expected: FAIL — `Resolve` undefined.

- [ ] **Step 3: Write the resolver**

```go
// internal/inventory/resolve.go
package inventory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// EnvVar names the environment variable consulted by Resolve.
const EnvVar = "HADOOPCLI_INVENTORY"

// DefaultFile is the file name looked up in the CWD and under ~/.hadoop-cli.
const DefaultFile = "cluster.yaml"

// HomeDir is the per-user state directory where hadoop-cli parks defaults.
const HomeDir = ".hadoop-cli"

// Resolve returns the inventory path to use. Lookup order:
//  1. flag (if non-empty)
//  2. $HADOOPCLI_INVENTORY
//  3. ./cluster.yaml
//  4. ~/.hadoop-cli/cluster.yaml
//
// The second return value is a short label identifying which rung matched
// ("flag", "env:HADOOPCLI_INVENTORY", "cwd", "home"). When nothing is found,
// the error lists every rung that was tried so users know how to fix it.
func Resolve(flag string) (string, string, error) {
	if flag != "" {
		return flag, "flag", nil
	}

	if env := os.Getenv(EnvVar); env != "" {
		return env, "env:" + EnvVar, nil
	}

	cwd, err := os.Getwd()
	if err == nil {
		candidate := filepath.Join(cwd, DefaultFile)
		if fileExists(candidate) {
			return candidate, "cwd", nil
		}
	}

	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		candidate := filepath.Join(home, HomeDir, DefaultFile)
		if fileExists(candidate) {
			return candidate, "home", nil
		}
	}

	return "", "", errors.New("no inventory found; tried --inventory flag, $" + EnvVar +
		", ./" + DefaultFile + ", ~/" + HomeDir + "/" + DefaultFile +
		fmt.Sprintf(" (cwd=%s home=%s)", cwd, home))
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/inventory -run TestResolve -race -v`
Expected: PASS — all five subtests green.

- [ ] **Step 5: Commit**

```bash
git add internal/inventory/resolve.go internal/inventory/resolve_test.go
git commit -m "feat(inventory): default lookup chain (flag > env > cwd > home)"
```

---

## Task 2: Envelope gets an `inventory_path` field

**Files:**
- Modify: `internal/output/envelope.go:22-29`
- Test: `internal/output/envelope_test.go` (create if missing)

- [ ] **Step 1: Check whether a test file exists**

Run: `ls internal/output/envelope_test.go 2>/dev/null || echo missing`
If missing, create a new file; otherwise append.

- [ ] **Step 2: Write the failing test**

```go
// internal/output/envelope_test.go
package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvelope_InventoryPath(t *testing.T) {
	env := NewEnvelope("status")
	env.InventoryPath = "/tmp/cluster.yaml"

	buf := &bytes.Buffer{}
	require.NoError(t, env.Write(buf))

	var got map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Equal(t, "/tmp/cluster.yaml", got["inventory_path"])
}

func TestEnvelope_InventoryPathOmittedWhenEmpty(t *testing.T) {
	env := NewEnvelope("status")

	buf := &bytes.Buffer{}
	require.NoError(t, env.Write(buf))

	require.NotContains(t, buf.String(), "inventory_path")
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/output -run TestEnvelope_InventoryPath -race -v`
Expected: FAIL — `InventoryPath` field unknown.

- [ ] **Step 4: Add the field**

Edit `internal/output/envelope.go` — in the `Envelope` struct, after `RunID`:

```go
type Envelope struct {
	Command       string         `json:"command"`
	OK            bool           `json:"ok"`
	Summary       map[string]any `json:"summary,omitempty"`
	Hosts         []HostResult   `json:"hosts,omitempty"`
	Error         *EnvelopeError `json:"error,omitempty"`
	RunID         string         `json:"run_id,omitempty"`
	InventoryPath string         `json:"inventory_path,omitempty"`
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/output -race -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/output/envelope.go internal/output/envelope_test.go
git commit -m "feat(output): add inventory_path to envelope"
```

---

## Task 3: Wire resolver into `prepare()` + stderr breadcrumb

**Files:**
- Modify: `cmd/root.go:19`
- Modify: `cmd/common.go:21-66`
- Test: `cmd/common_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `cmd/common_test.go`:

```go
package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// captureStderr replaces os.Stderr during fn and returns whatever was written.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		buf := &bytes.Buffer{}
		_, _ = io.Copy(buf, r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stderr = orig
	return <-done
}

func TestPrepare_UsesResolvedInventoryAndAnnounces(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("HOME", t.TempDir())

	// Minimal valid ZK-only inventory.
	yaml := `cluster:
  name: test
  install_dir: /opt/hadoop-cli
  data_dir: /data/hadoop-cli
  user: hadoop
  java_home: /usr/lib/jvm/java-11
  components: [zookeeper]
versions:
  zookeeper: 3.8.4
ssh:
  user: hadoop
  private_key: /tmp/id_rsa
hosts:
  - { name: n1, address: 10.0.0.1 }
  - { name: n2, address: 10.0.0.2 }
  - { name: n3, address: 10.0.0.3 }
roles:
  zookeeper: [n1, n2, n3]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "cluster.yaml"), []byte(yaml), 0o644))

	root := NewRootCmd()
	root.SetArgs([]string{"status"}) // no --inventory
	var ctx *runCtx
	var prepErr error
	stderr := captureStderr(t, func() {
		// Swap the status RunE to capture runCtx without actually executing.
		for _, sub := range root.Commands() {
			if sub.Name() == "status" {
				sub.RunE = func(cmd *cobra.Command, _ []string) error {
					ctx, prepErr = prepare(cmd, "status")
					return prepErr
				}
				break
			}
		}
		_ = root.Execute()
	})

	require.NoError(t, prepErr)
	require.NotNil(t, ctx)
	require.Equal(t, filepath.Join(dir, "cluster.yaml"), ctx.InventoryPath)
	require.True(t, strings.Contains(stderr, "using inventory:"), "stderr should announce resolved path; got: %q", stderr)
	require.True(t, strings.Contains(stderr, "cluster.yaml"), "stderr should mention cluster.yaml; got: %q", stderr)
}

func TestPrepare_MissingInventoryErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("HADOOPCLI_INVENTORY", "")

	root := NewRootCmd()
	root.SetArgs([]string{"status"})
	var prepErr error
	for _, sub := range root.Commands() {
		if sub.Name() == "status" {
			sub.RunE = func(cmd *cobra.Command, _ []string) error {
				_, prepErr = prepare(cmd, "status")
				return prepErr
			}
			break
		}
	}
	_ = root.Execute()
	require.Error(t, prepErr)
	require.Contains(t, prepErr.Error(), "no inventory found")
}
```

Note: this test imports `cobra`; add the import at the top:

```go
import (
	// ... existing
	"github.com/spf13/cobra"
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd -run TestPrepare -race -v`
Expected: FAIL — current `prepare()` sees default `"cluster.yaml"` and never resolves; `ctx.InventoryPath` field doesn't exist yet; stderr breadcrumb missing.

- [ ] **Step 3: Change the flag default**

Edit `cmd/root.go:19`:

```go
root.PersistentFlags().String("inventory", "", "path to cluster inventory YAML (default: $HADOOPCLI_INVENTORY, ./cluster.yaml, ~/.hadoop-cli/cluster.yaml)")
```

- [ ] **Step 4: Update `runCtx` and `prepare()` to use the resolver**

Edit `cmd/common.go`. Add `InventoryPath` to `runCtx`:

```go
type runCtx struct {
	Inv           *inventory.Inventory
	InventoryPath string
	Runner        *orchestrator.Runner
	Pool          *sshx.Pool
	Env           components.Env
	Command       string
	Progress      *output.Progress
}
```

Rewrite the top of `prepare()`:

```go
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
```

- [ ] **Step 5: Run tests**

Run: `go test ./cmd -run TestPrepare -race -v`
Expected: PASS — both subtests green.

- [ ] **Step 6: Run existing cmd tests to confirm no regression**

Run: `go test ./cmd -race`
Expected: PASS — `TestRootCommand_ShowsHelpByDefault`, `TestRootCommand_HasVersion`, `TestLifecycle*`, `TestExportSnapshot*` still green.

- [ ] **Step 7: Commit**

```bash
git add cmd/root.go cmd/common.go cmd/common_test.go
git commit -m "feat(cmd): resolve --inventory via lookup chain and announce on stderr"
```

---

## Task 4: Every envelope carries `inventory_path`

**Files:**
- Modify: `cmd/common.go` — add an `envelopeFor` helper.
- Modify: `cmd/preflight.go`, `install.go`, `configure.go`, `start.go`, `stop.go`, `status.go`, `uninstall.go`, `snapshot.go`, `export_snapshot.go` — replace `output.NewEnvelope("<name>")` with `ctx.envelope("<name>")`.

- [ ] **Step 1: Write the failing test**

Append to `cmd/common_test.go`:

```go
func TestRunCtx_EnvelopeCarriesInventoryPath(t *testing.T) {
	ctx := &runCtx{InventoryPath: "/tmp/cluster.yaml", Command: "status"}
	env := ctx.envelope("status")
	require.Equal(t, "/tmp/cluster.yaml", env.InventoryPath)
	require.Equal(t, "status", env.Command)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd -run TestRunCtx_EnvelopeCarriesInventoryPath -race -v`
Expected: FAIL — `ctx.envelope` undefined.

- [ ] **Step 3: Add the helper**

Append to `cmd/common.go`:

```go
// envelope returns a new output.Envelope pre-populated with fields that are
// uniform across subcommands (currently: the resolved inventory path).
func (c *runCtx) envelope(command string) *output.Envelope {
	e := output.NewEnvelope(command)
	e.InventoryPath = c.InventoryPath
	return e
}
```

- [ ] **Step 4: Run the new test alone**

Run: `go test ./cmd -run TestRunCtx_EnvelopeCarriesInventoryPath -race -v`
Expected: PASS.

- [ ] **Step 5: Find every call site to migrate**

Run: `grep -n 'output.NewEnvelope' cmd/*.go`
Expected output lists each subcommand once (preflight, install, configure, start, stop, status, uninstall, snapshot, export_snapshot — nine files).

- [ ] **Step 6: Migrate each call site**

For each matching file, replace

```go
env := output.NewEnvelope("<name>")
```

with

```go
env := rc.envelope("<name>")
```

using whatever local variable already holds the `*runCtx` (usually `rc`). The import of `output` stays because `aggregate()`, `HostResult`, etc. are still referenced.

If a file builds the envelope before `prepare()` (e.g., to report a prep error), wrap that branch to fall back to `output.NewEnvelope(command)` — inventory path isn't known yet in that path, which is correct.

- [ ] **Step 7: Run the full test suite**

Run: `go test ./... -race`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add cmd/common.go cmd/common_test.go cmd/preflight.go cmd/install.go cmd/configure.go cmd/start.go cmd/stop.go cmd/status.go cmd/uninstall.go cmd/snapshot.go cmd/export_snapshot.go
git commit -m "feat(cmd): record resolved inventory path in every envelope"
```

---

## Task 5: Documentation & skills

**Files:**
- Modify: `README.md` — Quick Start block; add lookup-chain note.
- Modify: `README.zh-CN.md` — mirror the English change.
- Modify: `skills/hbase-cluster-ops/SKILL.md:17-25` — drop `--inventory cluster.yaml` from examples where the lookup chain covers it; retain `--inventory` on `export-snapshot --to-inventory` cases (two inventories).
- Modify: `skills/hbase-cluster-bootstrap/SKILL.md:55-60` (and similar) — drop `--inventory cluster.yaml` from preflight example.
- Modify: `CLAUDE.md` — add a single line under existing inventory mentions.

- [ ] **Step 1: Update the English README**

In `README.md`, inside the **Quick Start** section (where the five-command bootstrap block lives), replace the block with:

````markdown
1. **Write `cluster.yaml`** — pick an example from [`skills/hbase-cluster-bootstrap/references/examples/`](./skills/hbase-cluster-bootstrap/references/examples/). Save it in the current directory or as `~/.hadoop-cli/cluster.yaml` and you can skip `--inventory` on every command.
2. **Verify SSH reachability** — `ssh -i ~/.ssh/id_rsa hadoop@node1 true` on every node listed in `hosts:`.
3. **Bootstrap the cluster**:

   ```bash
   hadoop-cli preflight    # JDK / port / disk / clock checks
   hadoop-cli install      # download, distribute, extract tarballs
   hadoop-cli configure    # render and push config files
   hadoop-cli start        # ZK → HDFS → HBase in order
   hadoop-cli status       # process presence on every host
   ```

Inventory is resolved in this order: `--inventory <path>` → `$HADOOPCLI_INVENTORY` → `./cluster.yaml` → `~/.hadoop-cli/cluster.yaml`. The resolved path is printed to stderr (`using inventory: …`) and echoed in every JSON envelope as `inventory_path`.
````

- [ ] **Step 2: Mirror the change in `README.zh-CN.md`**

In the **快速开始** section, replace the block with:

````markdown
1. **写一份 `cluster.yaml`** — 参考 [`skills/hbase-cluster-bootstrap/references/examples/`](./skills/hbase-cluster-bootstrap/references/examples/) 下的示例。把它放在当前目录，或保存为 `~/.hadoop-cli/cluster.yaml`，之后所有命令都不需要再带 `--inventory`。
2. **确认 SSH 可达** — 对每个 `hosts:` 下的节点执行 `ssh -i ~/.ssh/id_rsa hadoop@node1 true`。
3. **引导集群**：

   ```bash
   hadoop-cli preflight    # JDK / 端口 / 磁盘 / 时钟检查
   hadoop-cli install      # 下载、分发、解压 tarball
   hadoop-cli configure    # 渲染并推送配置文件
   hadoop-cli start        # 按 ZK → HDFS → HBase 顺序启动
   hadoop-cli status       # 在每台主机上检查进程
   ```

inventory 的查找顺序：`--inventory <path>` → `$HADOOPCLI_INVENTORY` → `./cluster.yaml` → `~/.hadoop-cli/cluster.yaml`。解析结果会在 stderr 打印一行 `using inventory: …`，并回填到 JSON envelope 的 `inventory_path` 字段。
````

- [ ] **Step 3: Simplify examples in `skills/hbase-cluster-ops/SKILL.md`**

Edit lines 17-22:

```markdown
| Check health               | `hadoop-cli status`                                                     |
| Stop the cluster           | `hadoop-cli stop`                                                       |
| Start it again             | `hadoop-cli start`                                                      |
| Remove the install         | `hadoop-cli uninstall`                                                  |
| Nuke install AND data      | `hadoop-cli uninstall --purge-data` (DESTRUCTIVE — confirm with the user first) |
```

Edit the snapshot row (line 23):

```markdown
| 打快照 / take a snapshot           | `hadoop-cli snapshot --table <ns:t> --name <snap>` |
```

Keep export-snapshot rows untouched — they use two inventories where `--inventory src.yaml --to-inventory dst.yaml` remains clearer than relying on the chain.

Add one sentence under the table:

```markdown
> The inventory is resolved from `$HADOOPCLI_INVENTORY`, `./cluster.yaml`, or `~/.hadoop-cli/cluster.yaml` unless `--inventory <path>` is passed. Pass `--inventory` explicitly when running against a non-default cluster or from an unrelated CWD.
```

- [ ] **Step 4: Simplify `skills/hbase-cluster-bootstrap/SKILL.md`**

Replace the preflight example (around line 55) with `hadoop-cli preflight` (no `--inventory`) and add a one-line note:

```markdown
> Place the generated `cluster.yaml` in the CWD or save it as `~/.hadoop-cli/cluster.yaml` so the remaining commands resolve it automatically.
```

- [ ] **Step 5: Extend `CLAUDE.md`**

Under the "What this project is" or "Architecture" section, add a bullet:

```markdown
- Inventory resolution: `--inventory` is no longer required. `inventory.Resolve` (`internal/inventory/resolve.go`) checks, in order, the flag, `$HADOOPCLI_INVENTORY`, `./cluster.yaml`, and `~/.hadoop-cli/cluster.yaml`. Subcommands announce the resolved path on stderr and set `inventory_path` on the envelope. `--to-inventory` on `export-snapshot` stays explicit (two inventories).
```

- [ ] **Step 6: Commit**

```bash
git add README.md README.zh-CN.md skills/hbase-cluster-bootstrap/SKILL.md skills/hbase-cluster-ops/SKILL.md CLAUDE.md
git commit -m "docs: document inventory lookup chain and simplify examples"
```

---

## Task 6: Verification & push

- [ ] **Step 1: Fmt / vet / test gate**

Run: `make fmt vet test`
Expected: PASS — no diff from fmt, no vet complaints, all tests green.

- [ ] **Step 2: Manual smoke — stderr breadcrumb**

Build once: `make build`

Run: `./bin/hadoop-cli preflight` from a directory containing a minimal `cluster.yaml` (the one used in the unit test is fine).
Expected stderr begins with `using inventory: <cwd>/cluster.yaml (cwd)`.

Run: `./bin/hadoop-cli preflight --inventory ./cluster.yaml`
Expected stderr begins with `using inventory: ./cluster.yaml (flag)`.

Run from a directory with no `cluster.yaml` and `HOME` pointing at an empty dir:
`HOME=/tmp HADOOPCLI_INVENTORY= ./bin/hadoop-cli preflight`
Expected: exit 1; stderr / envelope error message mentions all four rungs.

- [ ] **Step 3: Push**

```bash
git push origin main
```

Expected: release workflow auto-tags `v0.1.4` (or the next patch) and ships the new binary through GoReleaser.
