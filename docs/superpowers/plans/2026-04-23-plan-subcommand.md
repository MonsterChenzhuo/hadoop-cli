# `hadoop-cli plan` + Facts Safety Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `hadoop-cli plan` — a read-only discovery + planning command that collects host/cluster facts over SSH, emits a phased execution plan with blockers, and gates `install/configure/start` by default with a `--force` bypass.

**Architecture:** Two new internal packages (`internal/facts`, `internal/plan`) plus one new cobra command (`cmd/plan.go`). `facts` owns SSH collection, serialization, and TTL/inventory-sha freshness. `plan` turns `Facts + Inventory` into a phased action list and blocker/warning set. `cmd/common.go`'s `prepare()` grows a safety gate that loads facts by inventory sha and refuses non-forced runs on missing/stale/blocker facts. Existing `preflight` checks are reused inside `facts.collect_host`.

**Tech Stack:** Go 1.23, Cobra, YAML inventory, SSH via `internal/ssh`+`internal/orchestrator`. Tests use the existing `testserver_test.go` fake SSH server pattern for any SSH-touching code.

**Spec:** `docs/superpowers/specs/2026-04-23-plan-subcommand-design.md`.

---

## File Structure

New files:
- `internal/facts/facts.go` — `Facts` struct, `HostFacts` struct, `Blocker`/`Warning`, JSON serialization.
- `internal/facts/invhash.go` — `SHA256OfInventoryFile(path) (string, error)`.
- `internal/facts/store.go` — `DefaultFactsDir()`, `Save(run, inv, facts)`, `LoadForInventory(inv) (Facts, error)`.
- `internal/facts/freshness.go` — `ErrNotFound`, `ErrStale`, `Fresh(collectedAt, ttl) bool`, `DefaultTTL`.
- `internal/facts/collect_host.go` — host-level collectors (OS, JDK, resources, clock, hosts-file, user).
- `internal/facts/collect_cluster.go` — cluster-state collectors (installed_pkgs, data_state, ports, processes).
- `internal/facts/collect_deps.go` — external-HDFS reachability, ZK quorum mesh.
- `internal/facts/collect.go` — `Collect(ctx, inv, runner) (Facts, error)` orchestrator.
- `internal/plan/plan.go` — `Action`, `Phase`, `Plan`, `Build(inv, facts) Plan`.
- `internal/plan/blockers.go` — `Evaluate(inv, facts) ([]Blocker, []Warning)`.
- `internal/plan/render.go` — human-readable renderer for stderr.
- `cmd/plan.go` — cobra command.

Modified files:
- `internal/components/component.go` — add `Facts *facts.Facts` field on `Env`.
- `cmd/common.go` — add safety gate in `prepare()`, add `--force` flag handling.
- `cmd/install.go`, `cmd/configure.go`, `cmd/start.go` — register `--force` flag.
- `cmd/root.go` — register `newPlanCmd()`.
- `skills/hbase-cluster-bootstrap/SKILL.md` — update flow.
- `README.md`, `README.zh-CN.md` — add `plan` to the commands table.

Tests:
- Every new file has a `_test.go` companion. `cmd/lifecycle_test.go` gets a companion `plan_gate_test.go` for the gate.

---

### Task 1: `Facts` data model

**Files:**
- Create: `internal/facts/facts.go`
- Test: `internal/facts/facts_test.go`

- [ ] **Step 1: Write the failing test**

```go
package facts

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFactsJSONRoundtrip(t *testing.T) {
	now := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	in := Facts{
		RunID:         "20260423-100000-plan",
		InventorySHA:  "abc123",
		CollectedAt:   now,
		Hosts: map[string]HostFacts{
			"n1": {
				OS:           OSFacts{Kernel: "Linux", Arch: "x86_64", Distro: "ubuntu 22.04"},
				JDK:          JDKFacts{JavaHome: "/usr/lib/jvm/java-11", Version: "11.0.20", Present: true},
				Resources:    Resources{MemoryMB: 16000, CPUCores: 8, DiskInstallMB: 20000, DiskDataMB: 100000},
				ClockSkewMs:  12,
				HostsFileOK:  true,
				UserState:    UserState{Exists: true, OwnsDirs: true},
				InstalledPkgs: map[string]string{"hadoop": "3.3.6"},
				DataState:    DataState{HDFSFormatted: false, ZKMyID: "", HBaseHasWAL: false},
				Ports:        []PortFact{{Port: 2181, InUse: false}},
				Processes:    []string{},
			},
		},
		Blockers: []Blocker{{Code: "DISK_TOO_SMALL", Host: "n1", Message: "x", Hint: "y"}},
		Warnings: []Warning{{Code: "EXISTING_ZK_RUNNING", Host: "n1", Message: "z"}},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out Facts
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.RunID != in.RunID || out.InventorySHA != in.InventorySHA {
		t.Fatalf("roundtrip mismatch: %+v", out)
	}
	if out.Hosts["n1"].JDK.Version != "11.0.20" {
		t.Fatalf("jdk roundtrip lost data: %+v", out.Hosts["n1"].JDK)
	}
	if len(out.Blockers) != 1 || out.Blockers[0].Code != "DISK_TOO_SMALL" {
		t.Fatalf("blockers roundtrip: %+v", out.Blockers)
	}
}

func TestHasBlockers(t *testing.T) {
	f := Facts{Blockers: []Blocker{{Code: "X"}}}
	if !f.HasBlockers() {
		t.Fatal("expected HasBlockers true")
	}
	f2 := Facts{}
	if f2.HasBlockers() {
		t.Fatal("expected HasBlockers false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/facts/ -run TestFacts -race`
Expected: FAIL (package does not exist).

- [ ] **Step 3: Write minimal implementation**

```go
package facts

import "time"

type OSFacts struct {
	Kernel string `json:"kernel"`
	Arch   string `json:"arch"`
	Distro string `json:"distro,omitempty"`
}

type JDKFacts struct {
	Present  bool   `json:"present"`
	JavaHome string `json:"java_home"`
	Version  string `json:"version,omitempty"`
}

type Resources struct {
	MemoryMB      int64 `json:"memory_mb"`
	CPUCores      int   `json:"cpu_cores"`
	DiskInstallMB int64 `json:"disk_install_mb"`
	DiskDataMB    int64 `json:"disk_data_mb"`
}

type UserState struct {
	Exists   bool `json:"exists"`
	OwnsDirs bool `json:"owns_dirs"`
	Sudo     bool `json:"sudo,omitempty"`
}

type DataState struct {
	HDFSFormatted bool   `json:"hdfs_formatted"`
	ZKMyID        string `json:"zk_myid,omitempty"`
	HBaseHasWAL   bool   `json:"hbase_has_wal"`
}

type PortFact struct {
	Port        int    `json:"port"`
	InUse       bool   `json:"in_use"`
	ProcessName string `json:"process_name,omitempty"`
	ForeignOwner bool  `json:"foreign_owner,omitempty"`
}

type HostFacts struct {
	OS            OSFacts           `json:"os"`
	JDK           JDKFacts          `json:"jdk"`
	Resources     Resources         `json:"resources"`
	ClockSkewMs   int64             `json:"clock_skew_ms"`
	HostsFileOK   bool              `json:"hosts_file_ok"`
	UserState     UserState         `json:"user_state"`
	InstalledPkgs map[string]string `json:"installed_pkgs,omitempty"`
	DataState     DataState         `json:"data_state"`
	Ports         []PortFact        `json:"ports,omitempty"`
	Processes     []string          `json:"processes,omitempty"`
}

type Blocker struct {
	Code    string `json:"code"`
	Host    string `json:"host,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type Warning struct {
	Code    string `json:"code"`
	Host    string `json:"host,omitempty"`
	Message string `json:"message"`
}

type ExternalDeps struct {
	HDFSReachable   *bool `json:"external_hdfs_reachable,omitempty"`
	ZKQuorumMeshOK  *bool `json:"zk_quorum_mesh_ok,omitempty"`
}

type Facts struct {
	RunID        string               `json:"run_id"`
	InventorySHA string               `json:"inventory_sha"`
	CollectedAt  time.Time            `json:"collected_at"`
	Hosts        map[string]HostFacts `json:"hosts"`
	Deps         ExternalDeps         `json:"deps,omitempty"`
	Blockers     []Blocker            `json:"blockers,omitempty"`
	Warnings     []Warning            `json:"warnings,omitempty"`
}

func (f *Facts) HasBlockers() bool { return len(f.Blockers) > 0 }
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/facts/ -run TestFacts -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/facts/facts.go internal/facts/facts_test.go
git commit -m "feat(facts): add Facts data model + JSON roundtrip"
```

---

### Task 2: Inventory SHA helper

**Files:**
- Create: `internal/facts/invhash.go`
- Test: `internal/facts/invhash_test.go`

- [ ] **Step 1: Write the failing test**

```go
package facts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSHA256OfInventoryFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.yaml")
	if err := os.WriteFile(p, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sum, err := SHA256OfInventoryFile(p)
	if err != nil {
		t.Fatal(err)
	}
	// sha256("hello\n") = 5891b5b522...
	if sum != "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03" {
		t.Fatalf("unexpected sha: %s", sum)
	}
}

func TestSHA256OfInventoryFileMissing(t *testing.T) {
	_, err := SHA256OfInventoryFile("/no/such/file")
	if err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/facts/ -run TestSHA256 -race`
Expected: FAIL (`SHA256OfInventoryFile` undefined).

- [ ] **Step 3: Write minimal implementation**

```go
package facts

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
)

func SHA256OfInventoryFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/facts/ -run TestSHA256 -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/facts/invhash.go internal/facts/invhash_test.go
git commit -m "feat(facts): add inventory SHA256 helper"
```

---

### Task 3: Freshness rules

**Files:**
- Create: `internal/facts/freshness.go`
- Test: `internal/facts/freshness_test.go`

- [ ] **Step 1: Write the failing test**

```go
package facts

import (
	"testing"
	"time"
)

func TestFresh(t *testing.T) {
	now := time.Now()
	ttl := 30 * time.Minute
	if !Fresh(now.Add(-5*time.Minute), ttl) {
		t.Fatal("5m old should be fresh")
	}
	if Fresh(now.Add(-40*time.Minute), ttl) {
		t.Fatal("40m old should be stale")
	}
}

func TestResolveTTLDefault(t *testing.T) {
	t.Setenv("HADOOP_CLI_FACTS_TTL", "")
	if ResolveTTL() != DefaultTTL {
		t.Fatalf("default ttl mismatch: %v", ResolveTTL())
	}
}

func TestResolveTTLEnv(t *testing.T) {
	t.Setenv("HADOOP_CLI_FACTS_TTL", "10m")
	if ResolveTTL() != 10*time.Minute {
		t.Fatalf("env ttl mismatch: %v", ResolveTTL())
	}
}

func TestResolveTTLBadEnvFallsBack(t *testing.T) {
	t.Setenv("HADOOP_CLI_FACTS_TTL", "not-a-duration")
	if ResolveTTL() != DefaultTTL {
		t.Fatalf("bad env should fall back to default, got %v", ResolveTTL())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/facts/ -run TestFresh\|TestResolveTTL -race`
Expected: FAIL (`Fresh`, `ResolveTTL`, `DefaultTTL` undefined).

- [ ] **Step 3: Write minimal implementation**

```go
package facts

import (
	"errors"
	"os"
	"time"
)

const DefaultTTL = 30 * time.Minute

var (
	ErrNotFound = errors.New("facts: no cached facts for this inventory")
	ErrStale    = errors.New("facts: cached facts exceed TTL")
)

func Fresh(collectedAt time.Time, ttl time.Duration) bool {
	return time.Since(collectedAt) < ttl
}

func ResolveTTL() time.Duration {
	s := os.Getenv("HADOOP_CLI_FACTS_TTL")
	if s == "" {
		return DefaultTTL
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return DefaultTTL
	}
	return d
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/facts/ -run TestFresh\|TestResolveTTL -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/facts/freshness.go internal/facts/freshness_test.go
git commit -m "feat(facts): add TTL freshness helpers"
```

---

### Task 4: Facts store (write/read)

**Files:**
- Create: `internal/facts/store.go`
- Test: `internal/facts/store_test.go`

- [ ] **Step 1: Write the failing test**

```go
package facts

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveAndLoadForInventory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	runDir := filepath.Join(home, ".hadoop-cli", "runs", "r1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	f := &Facts{
		RunID:        "r1",
		InventorySHA: "deadbeef",
		CollectedAt:  time.Now().UTC(),
		Hosts:        map[string]HostFacts{"n1": {}},
	}
	if err := Save(runDir, f); err != nil {
		t.Fatalf("save: %v", err)
	}
	// run dir copy
	if _, err := os.Stat(filepath.Join(runDir, "facts.json")); err != nil {
		t.Fatalf("run-dir facts.json missing: %v", err)
	}
	// stable pointer
	if _, err := os.Stat(filepath.Join(home, ".hadoop-cli", "facts", "deadbeef.json")); err != nil {
		t.Fatalf("pointer missing: %v", err)
	}

	got, err := LoadForInventory("deadbeef")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.RunID != "r1" {
		t.Fatalf("load mismatch: %+v", got)
	}
}

func TestLoadForInventoryMissing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	_, err := LoadForInventory("nope")
	if err != ErrNotFound {
		t.Fatalf("want ErrNotFound got %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/facts/ -run TestSaveAndLoad\|TestLoadForInventoryMissing -race`
Expected: FAIL (`Save`, `LoadForInventory` undefined).

- [ ] **Step 3: Write minimal implementation**

```go
package facts

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

func DefaultFactsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hadoop-cli", "facts")
}

func Save(runDir string, f *Facts) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(runDir, "facts.json"), b, 0o644); err != nil {
		return err
	}
	ptrDir := DefaultFactsDir()
	if err := os.MkdirAll(ptrDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ptrDir, f.InventorySHA+".json"), b, 0o644)
}

func LoadForInventory(sha string) (*Facts, error) {
	p := filepath.Join(DefaultFactsDir(), sha+".json")
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var f Facts
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	return &f, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/facts/ -run TestSaveAndLoad\|TestLoadForInventoryMissing -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/facts/store.go internal/facts/store_test.go
git commit -m "feat(facts): add run-dir + stable-pointer store"
```

---

### Task 5: Host-level collectors

**Files:**
- Create: `internal/facts/collect_host.go`
- Test: `internal/facts/collect_host_test.go`

These collectors build `orchestrator.Task` values and parse the returned `Result.Stdout` into the host-level fields on `HostFacts`. They do not open SSH themselves — parsing is tested with hand-crafted `Result` values.

- [ ] **Step 1: Write the failing test**

```go
package facts

import (
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

func TestParseOS(t *testing.T) {
	r := orchestrator.Result{OK: true, Stdout: "Linux 5.15.0 x86_64\nID=ubuntu\nVERSION_ID=\"22.04\"\n"}
	os := ParseOS(r)
	if os.Kernel != "Linux 5.15.0" || os.Arch != "x86_64" {
		t.Fatalf("kernel/arch: %+v", os)
	}
	if os.Distro != "ubuntu 22.04" {
		t.Fatalf("distro: %+v", os)
	}
}

func TestParseJDKPresent(t *testing.T) {
	r := orchestrator.Result{OK: true, Stdout: "openjdk version \"11.0.20\" 2023-07-18\n"}
	j := ParseJDK("/usr/lib/jvm/java-11", r)
	if !j.Present || j.Version != "11.0.20" {
		t.Fatalf("jdk: %+v", j)
	}
}

func TestParseJDKMissing(t *testing.T) {
	r := orchestrator.Result{OK: false, Stderr: "no such file"}
	j := ParseJDK("/none", r)
	if j.Present {
		t.Fatalf("expected missing, got %+v", j)
	}
}

func TestParseResources(t *testing.T) {
	stdout := "MEM_MB=16000\nCPU=8\nDF_INSTALL_MB=20000\nDF_DATA_MB=100000\n"
	r := orchestrator.Result{OK: true, Stdout: stdout}
	res := ParseResources(r)
	if res.MemoryMB != 16000 || res.CPUCores != 8 {
		t.Fatalf("mem/cpu: %+v", res)
	}
	if res.DiskInstallMB != 20000 || res.DiskDataMB != 100000 {
		t.Fatalf("disk: %+v", res)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/facts/ -run TestParseOS\|TestParseJDK\|TestParseResources -race`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package facts

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

var javaVersionRE = regexp.MustCompile(`version "([^"]+)"`)

func OSTask(name string) orchestrator.Task {
	return orchestrator.Task{
		Name: "facts-os-" + name,
		Cmd:  `uname -srm; . /etc/os-release 2>/dev/null; echo "ID=${ID:-unknown}"; echo "VERSION_ID=\"${VERSION_ID:-unknown}\""`,
	}
}

func ParseOS(r orchestrator.Result) OSFacts {
	var os OSFacts
	lines := strings.Split(strings.TrimSpace(r.Stdout), "\n")
	if len(lines) > 0 {
		parts := strings.Fields(lines[0])
		if len(parts) >= 3 {
			os.Kernel = strings.Join(parts[:2], " ")
			os.Arch = parts[len(parts)-1]
		}
	}
	id := kvFrom(lines, "ID")
	ver := strings.Trim(kvFrom(lines, "VERSION_ID"), `"`)
	if id != "" {
		os.Distro = strings.TrimSpace(id + " " + ver)
	}
	return os
}

func kvFrom(lines []string, key string) string {
	for _, l := range lines {
		if strings.HasPrefix(l, key+"=") {
			return strings.TrimPrefix(l, key+"=")
		}
	}
	return ""
}

func JDKTask(name, javaHome string) orchestrator.Task {
	return orchestrator.Task{
		Name: "facts-jdk-" + name,
		Cmd:  fmt.Sprintf(`%s/bin/java -version 2>&1`, javaHome),
	}
}

func ParseJDK(javaHome string, r orchestrator.Result) JDKFacts {
	if !r.OK {
		return JDKFacts{Present: false, JavaHome: javaHome}
	}
	out := r.Stdout + r.Stderr
	m := javaVersionRE.FindStringSubmatch(out)
	ver := ""
	if len(m) == 2 {
		ver = m[1]
	}
	return JDKFacts{Present: ver != "", JavaHome: javaHome, Version: ver}
}

func ResourcesTask(name string, inv *inventory.Inventory) orchestrator.Task {
	script := fmt.Sprintf(`
MEM_MB=$(awk '/MemTotal/ {print int($2/1024)}' /proc/meminfo 2>/dev/null || sysctl -n hw.memsize 2>/dev/null | awk '{print int($1/1048576)}')
CPU=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null)
DF_INSTALL_MB=$(df -Pm %q 2>/dev/null | awk 'NR==2{print $4}')
DF_DATA_MB=$(df -Pm %q 2>/dev/null | awk 'NR==2{print $4}')
echo "MEM_MB=$MEM_MB"
echo "CPU=$CPU"
echo "DF_INSTALL_MB=$DF_INSTALL_MB"
echo "DF_DATA_MB=$DF_DATA_MB"
`, inv.Cluster.InstallDir, inv.Cluster.DataDir)
	return orchestrator.Task{Name: "facts-resources-" + name, Cmd: script}
}

func ParseResources(r orchestrator.Result) Resources {
	var res Resources
	sc := bufio.NewScanner(strings.NewReader(r.Stdout))
	for sc.Scan() {
		line := sc.Text()
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		v, _ := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		switch kv[0] {
		case "MEM_MB":
			res.MemoryMB = v
		case "CPU":
			res.CPUCores = int(v)
		case "DF_INSTALL_MB":
			res.DiskInstallMB = v
		case "DF_DATA_MB":
			res.DiskDataMB = v
		}
	}
	return res
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/facts/ -run TestParseOS\|TestParseJDK\|TestParseResources -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/facts/collect_host.go internal/facts/collect_host_test.go
git commit -m "feat(facts): add host-level collectors (os/jdk/resources)"
```

---

### Task 6: Cluster-state collectors

**Files:**
- Create: `internal/facts/collect_cluster.go`
- Test: `internal/facts/collect_cluster_test.go`

Covers `installed_pkgs`, `data_state`, `ports`, `processes`.

- [ ] **Step 1: Write the failing test**

```go
package facts

import (
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

func TestParseInstalledPkgs(t *testing.T) {
	stdout := "hadoop=3.3.6\nzookeeper=3.8.4\n"
	got := ParseInstalledPkgs(orchestrator.Result{OK: true, Stdout: stdout})
	if got["hadoop"] != "3.3.6" || got["zookeeper"] != "3.8.4" {
		t.Fatalf("pkgs: %+v", got)
	}
}

func TestParseDataState(t *testing.T) {
	stdout := "HDFS_FORMATTED=1\nZK_MYID=2\nHBASE_WAL=0\n"
	got := ParseDataState(orchestrator.Result{OK: true, Stdout: stdout})
	if !got.HDFSFormatted || got.ZKMyID != "2" || got.HBaseHasWAL {
		t.Fatalf("data state: %+v", got)
	}
}

func TestParsePorts(t *testing.T) {
	// One line per port: PORT IN_USE OWNER(optional)
	stdout := "2181 1 QuorumPeerMain\n2888 0\n"
	ownProc := map[string]bool{"QuorumPeerMain": true}
	got := ParsePorts(orchestrator.Result{OK: true, Stdout: stdout}, ownProc)
	if len(got) != 2 {
		t.Fatalf("ports: %+v", got)
	}
	if got[0].Port != 2181 || !got[0].InUse || got[0].ForeignOwner {
		t.Fatalf("zk client port parse: %+v", got[0])
	}
	if got[1].Port != 2888 || got[1].InUse {
		t.Fatalf("unused port: %+v", got[1])
	}
}

func TestParseProcesses(t *testing.T) {
	stdout := "1234 QuorumPeerMain\n5678 Jps\n9012 DataNode\n"
	got := ParseProcesses(orchestrator.Result{OK: true, Stdout: stdout})
	want := []string{"QuorumPeerMain", "DataNode"}
	if len(got) != len(want) {
		t.Fatalf("procs: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("procs[%d]: %s vs %s", i, got[i], want[i])
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/facts/ -run TestParseInstalledPkgs\|TestParseDataState\|TestParsePorts\|TestParseProcesses -race`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package facts

import (
	"strconv"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

func ParseInstalledPkgs(r orchestrator.Result) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		out[kv[0]] = kv[1]
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func ParseDataState(r orchestrator.Result) DataState {
	var ds DataState
	for _, line := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "HDFS_FORMATTED":
			ds.HDFSFormatted = kv[1] == "1"
		case "ZK_MYID":
			if kv[1] != "" && kv[1] != "0" {
				ds.ZKMyID = kv[1]
			}
		case "HBASE_WAL":
			ds.HBaseHasWAL = kv[1] == "1"
		}
	}
	return ds
}

// ParsePorts parses lines "PORT IN_USE [OWNER]" where IN_USE is 0/1.
// ownProc lists process names we treat as "our cluster" so foreign_owner stays false.
func ParsePorts(r orchestrator.Result, ownProc map[string]bool) []PortFact {
	var out []PortFact
	for _, line := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		p, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		pf := PortFact{Port: p, InUse: f[1] == "1"}
		if len(f) >= 3 {
			pf.ProcessName = f[2]
			if !ownProc[f[2]] {
				pf.ForeignOwner = true
			}
		}
		out = append(out, pf)
	}
	return out
}

// ParseProcesses filters jps output to target daemons.
func ParseProcesses(r orchestrator.Result) []string {
	targets := map[string]bool{
		"QuorumPeerMain": true, "NameNode": true, "DataNode": true,
		"HMaster": true, "HRegionServer": true,
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(r.Stdout), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		if targets[parts[1]] {
			out = append(out, parts[1])
		}
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/facts/ -run TestParseInstalledPkgs\|TestParseDataState\|TestParsePorts\|TestParseProcesses -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/facts/collect_cluster.go internal/facts/collect_cluster_test.go
git commit -m "feat(facts): add cluster-state parsers (pkgs/data/ports/procs)"
```

---

### Task 7: External-dep collectors

**Files:**
- Create: `internal/facts/collect_deps.go`
- Test: `internal/facts/collect_deps_test.go`

Only invoked when components require them.

- [ ] **Step 1: Write the failing test**

```go
package facts

import (
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

func TestNeedsExternalHDFSCheck(t *testing.T) {
	inv := &inventory.Inventory{}
	inv.Cluster.Components = []string{"hbase"}
	inv.Overrides.HBase.RootDir = "hdfs://10.0.0.1:8020/hbase"
	if !NeedsExternalHDFSCheck(inv) {
		t.Fatal("expected true when hbase-only + external root_dir")
	}
	inv.Cluster.Components = []string{"hdfs", "hbase"}
	if NeedsExternalHDFSCheck(inv) {
		t.Fatal("expected false when hdfs is local")
	}
}

func TestNeedsZKQuorumMeshCheck(t *testing.T) {
	inv := &inventory.Inventory{}
	inv.Cluster.Components = []string{"hdfs"}
	if NeedsZKQuorumMeshCheck(inv) {
		t.Fatal("hdfs-only should not need ZK mesh")
	}
	inv.Cluster.Components = []string{"zookeeper"}
	if !NeedsZKQuorumMeshCheck(inv) {
		t.Fatal("zk-in-components should need mesh")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/facts/ -run TestNeeds -race`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package facts

import "github.com/hadoop-cli/hadoop-cli/internal/inventory"

func NeedsExternalHDFSCheck(inv *inventory.Inventory) bool {
	return inv.HasComponent("hbase") && !inv.HasComponent("hdfs") && inv.Overrides.HBase.RootDir != ""
}

func NeedsZKQuorumMeshCheck(inv *inventory.Inventory) bool {
	return inv.HasComponent("zookeeper")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/facts/ -run TestNeeds -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/facts/collect_deps.go internal/facts/collect_deps_test.go
git commit -m "feat(facts): add gates for external-dep collectors"
```

---

### Task 8: Collect orchestrator

**Files:**
- Create: `internal/facts/collect.go`
- Test: `internal/facts/collect_test.go`

Uses a fake `runner` (implementing an internal interface) so we do not need real SSH.

- [ ] **Step 1: Write the failing test**

```go
package facts

import (
	"context"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

type fakeRunner struct {
	replies map[string][]orchestrator.Result // keyed by Task.Name prefix
}

func (f *fakeRunner) Run(_ context.Context, hosts []string, task orchestrator.Task) []orchestrator.Result {
	// match task.Name prefix, e.g. "facts-os-" => return "facts-os"
	for k, rs := range f.replies {
		if len(task.Name) >= len(k) && task.Name[:len(k)] == k {
			out := make([]orchestrator.Result, len(hosts))
			for i, h := range hosts {
				r := rs[0]
				r.Host = h
				out[i] = r
			}
			return out
		}
	}
	out := make([]orchestrator.Result, len(hosts))
	for i, h := range hosts {
		out[i] = orchestrator.Result{Host: h, OK: true}
	}
	return out
}

func TestCollectMinimal(t *testing.T) {
	inv := &inventory.Inventory{}
	inv.Cluster.Components = []string{"zookeeper"}
	inv.Cluster.InstallDir = "/opt/hadoop-cli"
	inv.Cluster.DataDir = "/data/hadoop-cli"
	inv.Cluster.JavaHome = "/usr/lib/jvm/java-11"
	inv.Roles.ZooKeeper = []string{"n1"}
	inv.Hosts = []inventory.Host{{Name: "n1", Address: "10.0.0.1"}}

	fr := &fakeRunner{replies: map[string][]orchestrator.Result{
		"facts-os-":       {{OK: true, Stdout: "Linux 5.15 x86_64\nID=ubuntu\nVERSION_ID=\"22.04\"\n"}},
		"facts-jdk-":      {{OK: true, Stderr: "openjdk version \"11.0.20\"\n"}},
		"facts-resources": {{OK: true, Stdout: "MEM_MB=16000\nCPU=8\nDF_INSTALL_MB=20000\nDF_DATA_MB=100000\n"}},
	}}
	f, err := Collect(context.Background(), inv, "sha-1", "run-1", fr)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if f.InventorySHA != "sha-1" || f.RunID != "run-1" {
		t.Fatalf("meta: %+v", f)
	}
	if _, ok := f.Hosts["n1"]; !ok {
		t.Fatalf("no n1 host facts: %+v", f.Hosts)
	}
	if f.Hosts["n1"].OS.Kernel == "" {
		t.Fatalf("empty OS facts: %+v", f.Hosts["n1"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/facts/ -run TestCollectMinimal -race`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package facts

import (
	"context"
	"time"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

// Runner is the subset of orchestrator.Runner we need. It lets tests inject a fake.
type Runner interface {
	Run(ctx context.Context, hosts []string, task orchestrator.Task) []orchestrator.Result
}

func Collect(ctx context.Context, inv *inventory.Inventory, invSHA, runID string, runner Runner) (*Facts, error) {
	hosts := inv.AllRoleHosts()
	f := &Facts{
		RunID:        runID,
		InventorySHA: invSHA,
		CollectedAt:  time.Now().UTC(),
		Hosts:        make(map[string]HostFacts, len(hosts)),
	}
	// OS
	osResults := runner.Run(ctx, hosts, OSTask("all"))
	jdkResults := runner.Run(ctx, hosts, JDKTask("all", inv.Cluster.JavaHome))
	resResults := runner.Run(ctx, hosts, ResourcesTask("all", inv))

	byHost := func(rs []orchestrator.Result) map[string]orchestrator.Result {
		m := make(map[string]orchestrator.Result, len(rs))
		for _, r := range rs {
			m[r.Host] = r
		}
		return m
	}
	osM, jdkM, resM := byHost(osResults), byHost(jdkResults), byHost(resResults)

	for _, h := range hosts {
		hf := HostFacts{
			OS:        ParseOS(osM[h]),
			JDK:       ParseJDK(inv.Cluster.JavaHome, jdkM[h]),
			Resources: ParseResources(resM[h]),
		}
		f.Hosts[h] = hf
	}
	return f, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/facts/ -run TestCollectMinimal -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/facts/collect.go internal/facts/collect_test.go
git commit -m "feat(facts): add Collect orchestrator (host-level MVP)"
```

Note: cluster-state + deps collectors are wired incrementally in later tasks (Task 10 blocker rules depend only on host-level facts present here; additional collectors get wired as the matching blocker rules land).

---

### Task 9: Plan data model + action build

**Files:**
- Create: `internal/plan/plan.go`
- Test: `internal/plan/plan_test.go`

Converts `Inventory + Facts` into a `Plan` with `install / configure / start` phases.

- [ ] **Step 1: Write the failing test**

```go
package plan

import (
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/facts"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

func TestBuildPhases(t *testing.T) {
	inv := &inventory.Inventory{}
	inv.Cluster.Components = []string{"zookeeper"}
	inv.Roles.ZooKeeper = []string{"n1", "n2", "n3"}
	inv.Versions.ZooKeeper = "3.8.4"
	f := &facts.Facts{
		Hosts: map[string]facts.HostFacts{
			"n1": {InstalledPkgs: map[string]string{"zookeeper": "3.8.4"}},
			"n2": {},
			"n3": {},
		},
	}
	p := Build(inv, f, "")
	if len(p.Phases) != 3 {
		t.Fatalf("want 3 phases, got %d", len(p.Phases))
	}
	if p.Phases[0].Name != "install" {
		t.Fatalf("phase order: %+v", p.Phases)
	}
	// install should have a skip action for n1 and a run action for n2/n3
	var sawSkip, sawRun bool
	for _, a := range p.Phases[0].Actions {
		if a.SkipReason != "" && contains(a.Hosts, "n1") {
			sawSkip = true
		}
		if a.SkipReason == "" && contains(a.Hosts, "n2") {
			sawRun = true
		}
	}
	if !sawSkip {
		t.Fatalf("expected skip action for n1 (pkg already installed): %+v", p.Phases[0])
	}
	if !sawRun {
		t.Fatalf("expected install action for n2: %+v", p.Phases[0])
	}
}

func TestBuildRespectsComponentFilter(t *testing.T) {
	inv := &inventory.Inventory{}
	inv.Cluster.Components = []string{"zookeeper", "hdfs"}
	inv.Roles.ZooKeeper = []string{"n1"}
	inv.Roles.NameNode = []string{"n1"}
	inv.Roles.DataNode = []string{"n1"}
	f := &facts.Facts{Hosts: map[string]facts.HostFacts{"n1": {}}}
	p := Build(inv, f, "hdfs")
	for _, ph := range p.Phases {
		for _, a := range ph.Actions {
			if a.Component == "zookeeper" {
				t.Fatalf("component filter leaked zk action: %+v", a)
			}
		}
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/plan/ -run TestBuild -race`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package plan

import (
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/facts"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

type Action struct {
	ID          string   `json:"id"`
	Component   string   `json:"component"`
	Hosts       []string `json:"hosts"`
	Description string   `json:"description"`
	SkipReason  string   `json:"skip_reason,omitempty"`
	Risk        string   `json:"risk"` // low | medium | high
}

type Phase struct {
	Name    string   `json:"name"`
	Actions []Action `json:"actions"`
}

type Plan struct {
	Phases   []Phase           `json:"phases"`
	Blockers []facts.Blocker   `json:"blockers,omitempty"`
	Warnings []facts.Warning   `json:"warnings,omitempty"`
}

// Build derives a phased plan from an inventory + collected facts.
// `filter` limits output to a single component when non-empty.
func Build(inv *inventory.Inventory, f *facts.Facts, filter string) *Plan {
	p := &Plan{}
	for _, phase := range []string{"install", "configure", "start"} {
		ph := Phase{Name: phase}
		for _, comp := range inv.Cluster.Components {
			if filter != "" && filter != comp {
				continue
			}
			ph.Actions = append(ph.Actions, actionsFor(phase, comp, inv, f)...)
		}
		p.Phases = append(p.Phases, ph)
	}
	return p
}

func actionsFor(phase, comp string, inv *inventory.Inventory, f *facts.Facts) []Action {
	switch phase {
	case "install":
		return installActions(comp, inv, f)
	case "configure":
		return configureActions(comp, inv, f)
	case "start":
		return startActions(comp, inv, f)
	}
	return nil
}

func hostsFor(comp string, inv *inventory.Inventory) []string {
	switch comp {
	case "zookeeper":
		return inv.Roles.ZooKeeper
	case "hdfs":
		// union of namenode + datanode hosts
		seen := map[string]struct{}{}
		for _, h := range inv.Roles.NameNode {
			seen[h] = struct{}{}
		}
		for _, h := range inv.Roles.DataNode {
			seen[h] = struct{}{}
		}
		out := make([]string, 0, len(seen))
		for h := range seen {
			out = append(out, h)
		}
		return out
	case "hbase":
		seen := map[string]struct{}{}
		for _, h := range inv.Roles.HBaseMaster {
			seen[h] = struct{}{}
		}
		for _, h := range inv.Roles.RegionServer {
			seen[h] = struct{}{}
		}
		out := make([]string, 0, len(seen))
		for h := range seen {
			out = append(out, h)
		}
		return out
	}
	return nil
}

func installActions(comp string, inv *inventory.Inventory, f *facts.Facts) []Action {
	hosts := hostsFor(comp, inv)
	pkgKey := comp
	wantVer := versionFor(comp, inv)
	var alreadyInstalled, toInstall []string
	for _, h := range hosts {
		hf := f.Hosts[h]
		if hf.InstalledPkgs[pkgKey] == wantVer && wantVer != "" {
			alreadyInstalled = append(alreadyInstalled, h)
		} else {
			toInstall = append(toInstall, h)
		}
	}
	var out []Action
	if len(alreadyInstalled) > 0 {
		out = append(out, Action{
			ID:          fmt.Sprintf("install.%s.skip", comp),
			Component:   comp,
			Hosts:       alreadyInstalled,
			Description: fmt.Sprintf("%s %s already installed", comp, wantVer),
			SkipReason:  "package version matches inventory",
			Risk:        "low",
		})
	}
	if len(toInstall) > 0 {
		out = append(out, Action{
			ID:          fmt.Sprintf("install.%s", comp),
			Component:   comp,
			Hosts:       toInstall,
			Description: fmt.Sprintf("download + extract %s %s", comp, wantVer),
			Risk:        "low",
		})
	}
	return out
}

func configureActions(comp string, inv *inventory.Inventory, _ *facts.Facts) []Action {
	hosts := hostsFor(comp, inv)
	if len(hosts) == 0 {
		return nil
	}
	return []Action{{
		ID:          fmt.Sprintf("configure.%s", comp),
		Component:   comp,
		Hosts:       hosts,
		Description: fmt.Sprintf("render + push %s config files", comp),
		Risk:        "low",
	}}
}

func startActions(comp string, inv *inventory.Inventory, f *facts.Facts) []Action {
	hosts := hostsFor(comp, inv)
	var out []Action
	if comp == "hdfs" && len(inv.Roles.NameNode) > 0 {
		nn := inv.Roles.NameNode[0]
		if !f.Hosts[nn].DataState.HDFSFormatted {
			out = append(out, Action{
				ID:          "start.hdfs.format",
				Component:   "hdfs",
				Hosts:       []string{nn},
				Description: "format NameNode (first-run only; destructive on existing data)",
				Risk:        "high",
			})
		}
	}
	if len(hosts) > 0 {
		out = append(out, Action{
			ID:          fmt.Sprintf("start.%s", comp),
			Component:   comp,
			Hosts:       hosts,
			Description: fmt.Sprintf("start %s daemons", comp),
			Risk:        "medium",
		})
	}
	return out
}

func versionFor(comp string, inv *inventory.Inventory) string {
	switch comp {
	case "zookeeper":
		return inv.Versions.ZooKeeper
	case "hdfs":
		return inv.Versions.Hadoop
	case "hbase":
		return inv.Versions.HBase
	}
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/plan/ -run TestBuild -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/plan/plan.go internal/plan/plan_test.go
git commit -m "feat(plan): build phased action list from inventory+facts"
```

---

### Task 10: Blocker + warning evaluation

**Files:**
- Create: `internal/plan/blockers.go`
- Test: `internal/plan/blockers_test.go`

Implements the codes listed in spec §9.

- [ ] **Step 1: Write the failing test**

```go
package plan

import (
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/facts"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

func TestEvaluateDiskTooSmall(t *testing.T) {
	inv := &inventory.Inventory{}
	inv.Cluster.Components = []string{"hdfs"}
	inv.Roles.NameNode = []string{"n1"}
	inv.Roles.DataNode = []string{"n1"}
	f := &facts.Facts{Hosts: map[string]facts.HostFacts{
		"n1": {Resources: facts.Resources{DiskDataMB: 10_000, DiskInstallMB: 6_000}, JDK: facts.JDKFacts{Present: true}, HostsFileOK: true, UserState: facts.UserState{Exists: true}},
	}}
	bs, _ := Evaluate(inv, f, Thresholds{DataMinMB: 50_000, InstallMinMB: 5_000})
	if len(bs) != 1 || bs[0].Code != "DISK_TOO_SMALL" {
		t.Fatalf("blockers: %+v", bs)
	}
}

func TestEvaluateJDKMissing(t *testing.T) {
	inv := &inventory.Inventory{}
	inv.Cluster.Components = []string{"zookeeper"}
	inv.Roles.ZooKeeper = []string{"n1"}
	f := &facts.Facts{Hosts: map[string]facts.HostFacts{
		"n1": {Resources: facts.Resources{DiskDataMB: 100_000, DiskInstallMB: 100_000}, JDK: facts.JDKFacts{Present: false}, HostsFileOK: true, UserState: facts.UserState{Exists: true}},
	}}
	bs, _ := Evaluate(inv, f, DefaultThresholds)
	var saw bool
	for _, b := range bs {
		if b.Code == "JDK_MISSING" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("expected JDK_MISSING blocker: %+v", bs)
	}
}

func TestEvaluateExistingZKWarning(t *testing.T) {
	inv := &inventory.Inventory{}
	inv.Cluster.Components = []string{"zookeeper"}
	inv.Roles.ZooKeeper = []string{"n1"}
	f := &facts.Facts{Hosts: map[string]facts.HostFacts{
		"n1": {
			Resources: facts.Resources{DiskDataMB: 100_000, DiskInstallMB: 100_000},
			JDK:       facts.JDKFacts{Present: true},
			HostsFileOK: true,
			UserState: facts.UserState{Exists: true},
			Processes: []string{"QuorumPeerMain"},
		},
	}}
	_, ws := Evaluate(inv, f, DefaultThresholds)
	var saw bool
	for _, w := range ws {
		if w.Code == "EXISTING_ZK_RUNNING" {
			saw = true
		}
	}
	if !saw {
		t.Fatalf("expected EXISTING_ZK_RUNNING warning: %+v", ws)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/plan/ -run TestEvaluate -race`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package plan

import (
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/facts"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

type Thresholds struct {
	DataMinMB    int64
	InstallMinMB int64
}

var DefaultThresholds = Thresholds{
	DataMinMB:    50_000,
	InstallMinMB: 5_000,
}

func Evaluate(inv *inventory.Inventory, f *facts.Facts, th Thresholds) ([]facts.Blocker, []facts.Warning) {
	var bs []facts.Blocker
	var ws []facts.Warning
	for _, host := range hostsInPlay(inv) {
		hf := f.Hosts[host]
		if hf.Resources.DiskDataMB > 0 && hf.Resources.DiskDataMB < th.DataMinMB {
			bs = append(bs, facts.Blocker{
				Code:    "DISK_TOO_SMALL",
				Host:    host,
				Message: fmt.Sprintf("data_dir has %dMB free, need ≥ %dMB", hf.Resources.DiskDataMB, th.DataMinMB),
				Hint:    "expand mount or change cluster.data_dir and rerun plan",
			})
		}
		if hf.Resources.DiskInstallMB > 0 && hf.Resources.DiskInstallMB < th.InstallMinMB {
			bs = append(bs, facts.Blocker{
				Code:    "DISK_TOO_SMALL",
				Host:    host,
				Message: fmt.Sprintf("install_dir has %dMB free, need ≥ %dMB", hf.Resources.DiskInstallMB, th.InstallMinMB),
				Hint:    "expand mount or change cluster.install_dir and rerun plan",
			})
		}
		if !hf.JDK.Present {
			bs = append(bs, facts.Blocker{
				Code:    "JDK_MISSING",
				Host:    host,
				Message: fmt.Sprintf("no JDK at %s", hf.JDK.JavaHome),
				Hint:    "install JDK 8/11 and set cluster.java_home, then rerun plan",
			})
		}
		if !hf.UserState.Exists {
			bs = append(bs, facts.Blocker{
				Code:    "USER_MISSING",
				Host:    host,
				Message: fmt.Sprintf("cluster.user not present on %s", host),
				Hint:    "create the user with ownership of install_dir and data_dir, or enable ssh.sudo",
			})
		}
		if !hf.HostsFileOK {
			bs = append(bs, facts.Blocker{
				Code:    "HOSTS_INCONSISTENT",
				Host:    host,
				Message: "/etc/hosts does not resolve inventory hostnames consistently",
				Hint:    "align /etc/hosts across nodes and rerun plan",
			})
		}
		for _, p := range hf.Ports {
			if p.InUse && p.ForeignOwner {
				bs = append(bs, facts.Blocker{
					Code:    "PORT_OCCUPIED_BY_FOREIGN",
					Host:    host,
					Message: fmt.Sprintf("port %d in use by %s (non-cluster process)", p.Port, p.ProcessName),
					Hint:    "stop the foreign process or pick a different port in inventory.overrides",
				})
			}
		}
		if hf.DataState.HDFSFormatted && inv.HasComponent("hdfs") {
			ws = append(ws, facts.Warning{
				Code: "DATA_DIRTY_NOT_FORCED", Host: host,
				Message: "HDFS data_dir already formatted; re-install will not reformat unless --force-format is passed to start",
			})
		}
		for _, proc := range hf.Processes {
			switch proc {
			case "QuorumPeerMain":
				ws = append(ws, facts.Warning{Code: "EXISTING_ZK_RUNNING", Host: host, Message: "QuorumPeerMain already running; install/start will replace/reuse it"})
			case "NameNode", "DataNode":
				ws = append(ws, facts.Warning{Code: "EXISTING_NN_RUNNING", Host: host, Message: proc + " already running"})
			case "HMaster", "HRegionServer":
				ws = append(ws, facts.Warning{Code: "EXISTING_HBASE_RUNNING", Host: host, Message: proc + " already running"})
			}
		}
	}
	return bs, ws
}

func hostsInPlay(inv *inventory.Inventory) []string {
	seen := map[string]struct{}{}
	add := func(xs []string) {
		for _, x := range xs {
			seen[x] = struct{}{}
		}
	}
	if inv.HasComponent("zookeeper") {
		add(inv.Roles.ZooKeeper)
	}
	if inv.HasComponent("hdfs") {
		add(inv.Roles.NameNode)
		add(inv.Roles.DataNode)
	}
	if inv.HasComponent("hbase") {
		add(inv.Roles.HBaseMaster)
		add(inv.Roles.RegionServer)
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/plan/ -run TestEvaluate -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/plan/blockers.go internal/plan/blockers_test.go
git commit -m "feat(plan): evaluate blockers + warnings from facts"
```

---

### Task 11: Human-readable render

**Files:**
- Create: `internal/plan/render.go`
- Test: `internal/plan/render_test.go`

- [ ] **Step 1: Write the failing test**

```go
package plan

import (
	"bytes"
	"strings"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/facts"
)

func TestRenderContainsPhasesAndBlockers(t *testing.T) {
	p := &Plan{
		Phases: []Phase{
			{Name: "install", Actions: []Action{
				{ID: "install.zookeeper", Component: "zookeeper", Hosts: []string{"n1", "n2"}, Description: "download + extract zookeeper 3.8.4", Risk: "low"},
				{ID: "install.zookeeper.skip", Component: "zookeeper", Hosts: []string{"n3"}, Description: "zookeeper 3.8.4 already installed", SkipReason: "package version matches inventory", Risk: "low"},
			}},
			{Name: "configure", Actions: []Action{}},
			{Name: "start", Actions: []Action{}},
		},
		Blockers: []facts.Blocker{{Code: "DISK_TOO_SMALL", Host: "n2", Message: "x", Hint: "y"}},
		Warnings: []facts.Warning{{Code: "EXISTING_ZK_RUNNING", Host: "n3", Message: "z"}},
	}
	var buf bytes.Buffer
	Render(&buf, p)
	s := buf.String()
	if !strings.Contains(s, "Phase: install") {
		t.Fatalf("missing phase header: %s", s)
	}
	if !strings.Contains(s, "[skip]") || !strings.Contains(s, "[run]") {
		t.Fatalf("missing skip/run markers: %s", s)
	}
	if !strings.Contains(s, "DISK_TOO_SMALL") {
		t.Fatalf("missing blocker line: %s", s)
	}
	if !strings.Contains(s, "EXISTING_ZK_RUNNING") {
		t.Fatalf("missing warning line: %s", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/plan/ -run TestRender -race`
Expected: FAIL.

- [ ] **Step 3: Write minimal implementation**

```go
package plan

import (
	"fmt"
	"io"
	"strings"
)

func Render(w io.Writer, p *Plan) {
	for _, ph := range p.Phases {
		fmt.Fprintf(w, "Phase: %s\n", ph.Name)
		if len(ph.Actions) == 0 {
			fmt.Fprintln(w, "  (no actions)")
			continue
		}
		for _, a := range ph.Actions {
			marker := "[run] "
			if a.SkipReason != "" {
				marker = "[skip]"
			}
			hosts := strings.Join(a.Hosts, ",")
			fmt.Fprintf(w, "  %s %-20s %s\n", marker, hosts, a.Description)
		}
	}
	if len(p.Blockers) > 0 {
		fmt.Fprintf(w, "\nBlockers (%d):\n", len(p.Blockers))
		for _, b := range p.Blockers {
			fmt.Fprintf(w, "  [%s] %s: %s\n", b.Code, b.Host, b.Message)
			if b.Hint != "" {
				fmt.Fprintf(w, "    → %s\n", b.Hint)
			}
		}
	}
	if len(p.Warnings) > 0 {
		fmt.Fprintf(w, "\nWarnings (%d):\n", len(p.Warnings))
		for _, wr := range p.Warnings {
			fmt.Fprintf(w, "  [%s] %s: %s\n", wr.Code, wr.Host, wr.Message)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/plan/ -run TestRender -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/plan/render.go internal/plan/render_test.go
git commit -m "feat(plan): human-readable renderer"
```

---

### Task 12: `cmd/plan.go` + root registration

**Files:**
- Create: `cmd/plan.go`
- Modify: `cmd/root.go` (register `newPlanCmd()`)
- Test: `cmd/plan_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlanCmdRegistered(t *testing.T) {
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetArgs([]string{"plan", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute plan --help: %v", err)
	}
	if !strings.Contains(out.String(), "plan") {
		t.Fatalf("help output missing plan: %s", out.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd -run TestPlanCmdRegistered -race`
Expected: FAIL (command not registered).

- [ ] **Step 3: Write minimal implementation**

Add `cmd/plan.go`:

```go
package cmd

import (
	"os"

	"github.com/hadoop-cli/hadoop-cli/internal/facts"
	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/hadoop-cli/hadoop-cli/internal/plan"
	"github.com/spf13/cobra"
)

func newPlanCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "plan",
		Short: "Collect host facts and emit a phased execution plan with blockers.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepareNoGate(cmd, "plan")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			invPath, _ := cmd.Flags().GetString("inventory")

			ctx := backgroundCtx(cmd)
			env := output.NewEnvelope("plan").WithRunID(rc.Env.Run.ID)

			sha, err := facts.SHA256OfInventoryFile(invPath)
			if err != nil {
				env.WithError(output.EnvelopeError{Code: "INVENTORY_READ", Message: err.Error()})
				_ = rc.Env.Run.SaveResult(env)
				writeEnvelope(env)
				return err
			}

			f, err := facts.Collect(ctx, rc.Inv, sha, rc.Env.Run.ID, rc.Runner)
			if err != nil {
				env.WithError(output.EnvelopeError{Code: "FACTS_COLLECT", Message: err.Error()})
				_ = rc.Env.Run.SaveResult(env)
				writeEnvelope(env)
				return err
			}

			bs, ws := plan.Evaluate(rc.Inv, f, plan.DefaultThresholds)
			f.Blockers = bs
			f.Warnings = ws

			if err := facts.Save(rc.Env.Run.Dir, f); err != nil {
				env.WithError(output.EnvelopeError{Code: "FACTS_SAVE", Message: err.Error()})
				_ = rc.Env.Run.SaveResult(env)
				writeEnvelope(env)
				return err
			}

			p := plan.Build(rc.Inv, f, component)
			p.Blockers = bs
			p.Warnings = ws

			plan.Render(os.Stderr, p)

			env.OK = len(bs) == 0
			env.Summary = map[string]any{
				"blockers":      len(bs),
				"warnings":      len(ws),
				"actions":       countActions(p),
				"inventory_sha": sha,
				"facts_path":    rc.Env.Run.Dir + "/facts.json",
				"plan":          p,
			}
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(env)
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	c.Flags().String("output", "both", "output mode: human|json|both")
	return c
}

func countActions(p *plan.Plan) int {
	n := 0
	for _, ph := range p.Phases {
		n += len(ph.Actions)
	}
	return n
}
```

`prepareNoGate` is introduced in Task 13 — for now, alias it to `prepare` (Task 13 splits them). Add at the bottom of `cmd/common.go` temporarily:

```go
// prepareNoGate is the same as prepare but skips the facts safety gate.
// Used by plan / preflight / status / uninstall.
func prepareNoGate(cmd *cobra.Command, command string) (*runCtx, error) {
	return prepare(cmd, command)
}
```

Modify `cmd/root.go`:

```go
root.AddCommand(newPlanCmd())
```

(insert next to the other `AddCommand` calls).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd -run TestPlanCmdRegistered -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/plan.go cmd/plan_test.go cmd/root.go cmd/common.go
git commit -m "feat(cmd): add plan subcommand wiring"
```

---

### Task 13: Safety gate in `prepare()`

**Files:**
- Modify: `internal/components/component.go` (add `Facts` field to `Env`)
- Modify: `cmd/common.go` (split `prepare` vs `prepareNoGate`, implement gate, register `--force`)
- Test: `cmd/gate_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hadoop-cli/hadoop-cli/internal/facts"
)

func writeTempInventory(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "cluster.yaml")
	// A minimal ZK-only inventory that Validate accepts.
	body := `
cluster:
  name: t
  install_dir: /opt/t
  data_dir: /data/t
  user: hadoop
  java_home: /usr/lib/jvm/java-11
  components: [zookeeper]
versions: { zookeeper: 3.8.4 }
ssh: { user: hadoop, private_key: /tmp/id_rsa }
hosts:
  - { name: n1, address: 127.0.0.1 }
  - { name: n2, address: 127.0.0.2 }
  - { name: n3, address: 127.0.0.3 }
roles:
  zookeeper: [n1, n2, n3]
`
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestGateMissingFacts(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	invPath := writeTempInventory(t)

	root := NewRootCmd()
	root.SetArgs([]string{"install", "--inventory", invPath})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected error due to missing facts")
	}
	if !contains(err.Error(), "FACTS_MISSING") {
		t.Fatalf("want FACTS_MISSING in err, got: %v", err)
	}
}

func TestGateStaleFacts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	invPath := writeTempInventory(t)
	sha, _ := facts.SHA256OfInventoryFile(invPath)

	ptrDir := filepath.Join(home, ".hadoop-cli", "facts")
	_ = os.MkdirAll(ptrDir, 0o755)
	f := facts.Facts{
		InventorySHA: sha,
		CollectedAt:  time.Now().Add(-2 * time.Hour),
	}
	b, _ := json.Marshal(f)
	_ = os.WriteFile(filepath.Join(ptrDir, sha+".json"), b, 0o644)

	root := NewRootCmd()
	root.SetArgs([]string{"install", "--inventory", invPath})
	err := root.Execute()
	if err == nil || !contains(err.Error(), "FACTS_STALE") {
		t.Fatalf("expected FACTS_STALE, got %v", err)
	}
}

func TestGateHasBlockers(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	invPath := writeTempInventory(t)
	sha, _ := facts.SHA256OfInventoryFile(invPath)

	ptrDir := filepath.Join(home, ".hadoop-cli", "facts")
	_ = os.MkdirAll(ptrDir, 0o755)
	f := facts.Facts{
		InventorySHA: sha,
		CollectedAt:  time.Now(),
		Blockers:     []facts.Blocker{{Code: "DISK_TOO_SMALL", Host: "n1", Message: "x"}},
	}
	b, _ := json.Marshal(f)
	_ = os.WriteFile(filepath.Join(ptrDir, sha+".json"), b, 0o644)

	root := NewRootCmd()
	root.SetArgs([]string{"install", "--inventory", invPath})
	err := root.Execute()
	if err == nil || !contains(err.Error(), "FACTS_HAS_BLOCKERS") {
		t.Fatalf("expected FACTS_HAS_BLOCKERS, got %v", err)
	}
}

func TestGateForceBypass(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	invPath := writeTempInventory(t)

	root := NewRootCmd()
	root.SetArgs([]string{"install", "--inventory", invPath, "--force"})
	err := root.Execute()
	// Should NOT fail on the gate. It will still fail later trying to SSH — that is fine.
	if err != nil && contains(err.Error(), "FACTS_MISSING") {
		t.Fatalf("--force should bypass gate, got %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd -run TestGate -race`
Expected: FAIL (gate not implemented).

- [ ] **Step 3: Write minimal implementation**

Edit `internal/components/component.go`:

```go
import "github.com/hadoop-cli/hadoop-cli/internal/facts"

type Env struct {
	Inv    *inventory.Inventory
	Runner *orchestrator.Runner
	Cache  string
	Run    *runlog.Run
	Facts  *facts.Facts
}
```

Edit `cmd/common.go`:

```go
import (
	// existing imports...
	"github.com/hadoop-cli/hadoop-cli/internal/facts"
)

// prepare is for gated commands (install/configure/start).
func prepare(cmd *cobra.Command, command string) (*runCtx, error) {
	rc, err := prepareNoGate(cmd, command)
	if err != nil {
		return nil, err
	}
	force, _ := cmd.Flags().GetBool("force")
	if force {
		return rc, nil
	}
	invPath, _ := cmd.Flags().GetString("inventory")
	sha, err := facts.SHA256OfInventoryFile(invPath)
	if err != nil {
		return nil, err
	}
	f, err := facts.LoadForInventory(sha)
	if err == facts.ErrNotFound {
		return nil, fmt.Errorf("[FACTS_MISSING] no facts for this inventory; run `hadoop-cli plan --inventory %s` first", invPath)
	}
	if err != nil {
		return nil, err
	}
	if !facts.Fresh(f.CollectedAt, facts.ResolveTTL()) {
		return nil, fmt.Errorf("[FACTS_STALE] facts older than TTL; rerun `hadoop-cli plan --inventory %s`", invPath)
	}
	if f.HasBlockers() {
		return nil, fmt.Errorf("[FACTS_HAS_BLOCKERS] %d blocker(s) reported by plan; resolve them and rerun `hadoop-cli plan`", len(f.Blockers))
	}
	rc.Env.Facts = f
	return rc, nil
}

// prepareNoGate is used by plan/preflight/status/uninstall/snapshot/export-snapshot.
func prepareNoGate(cmd *cobra.Command, command string) (*runCtx, error) {
	// ... body of the OLD prepare() goes here ...
}
```

(Move the old body into `prepareNoGate`; the new `prepare` wraps it.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd -run TestGate -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/components/component.go cmd/common.go cmd/gate_test.go
git commit -m "feat(cmd): facts-based safety gate in prepare()"
```

---

### Task 14: `--force` flag on install/configure/start

**Files:**
- Modify: `cmd/install.go`, `cmd/configure.go`, `cmd/start.go`
- Test: covered by Task 13's `TestGateForceBypass`

- [ ] **Step 1: Edit each command**

For each of `install.go`, `configure.go`, `start.go`, add inside the `newXxxCmd()` function:

```go
c.Flags().Bool("force", false, "skip facts safety gate (audit-logged)")
```

- [ ] **Step 2: Rerun gate tests**

Run: `go test ./cmd -run TestGate -race`
Expected: PASS (specifically `TestGateForceBypass` now actually reads `--force`).

- [ ] **Step 3: Commit**

```bash
git add cmd/install.go cmd/configure.go cmd/start.go
git commit -m "feat(cmd): --force bypasses facts gate on install/configure/start"
```

---

### Task 15: Update `hbase-cluster-bootstrap` skill

**Files:**
- Modify: `skills/hbase-cluster-bootstrap/SKILL.md`

- [ ] **Step 1: Edit the standard bootstrap flow**

Replace the numbered flow under "Standard bootstrap flow" to include `plan` between `preflight` and `install`:

```
2. Preflight (optional but recommended)
3. Plan — collects facts and emits the execution plan:
       hadoop-cli plan --inventory cluster.yaml
   Read JSON envelope:
     - ok:true  → proceed
     - ok:false → read "blockers", resolve them, rerun plan
   Warnings do not block but surface the highlights to the user.
4. Install
5. Configure
6. Start
7. Verify
```

- [ ] **Step 2: Add a troubleshooting note**

Under "Common pitfalls" add:

```
- `install`, `configure`, and `start` require facts from a recent `plan`
  (TTL 30m by default; override via HADOOP_CLI_FACTS_TTL). They fail with
  `FACTS_MISSING` / `FACTS_STALE` / `FACTS_HAS_BLOCKERS`. Rerun `plan`
  to refresh, or pass `--force` when you know the state hasn't drifted.
```

- [ ] **Step 3: Commit**

```bash
git add skills/hbase-cluster-bootstrap/SKILL.md
git commit -m "docs(skill): add plan step to bootstrap flow"
```

---

### Task 16: Update READMEs

**Files:**
- Modify: `README.md`, `README.zh-CN.md`

- [ ] **Step 1: Add `plan` to commands table**

Insert a row between `preflight` and `install`:

```
| plan         | SSH-discover host facts, emit phased plan, gate later commands |
```

And add to the Quick start code block:

```bash
hadoop-cli preflight --inventory cluster.yaml
hadoop-cli plan      --inventory cluster.yaml
hadoop-cli install   --inventory cluster.yaml
...
```

Mirror the same change in `README.zh-CN.md` (Chinese wording).

- [ ] **Step 2: Commit**

```bash
git add README.md README.zh-CN.md
git commit -m "docs(readme): document plan subcommand"
```

---

### Task 17: End-to-end lifecycle test

**Files:**
- Create: `cmd/plan_lifecycle_test.go`

Ensures plan → install gate → --force path all work together using the same test-server pattern as `cmd/lifecycle_test.go`. If `lifecycle_test.go` does not yet mock SSH for install, this test at minimum covers the `plan`→gate handshake.

- [ ] **Step 1: Write the test**

```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/facts"
)

func TestPlanUnblocksInstallGate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	invPath := writeTempInventory(t)

	// plan produces facts; but because hosts are unreachable, Collect may return
	// empty Hosts map — OK, the point is to exercise the gate unblock path.
	// We simulate success by manually writing fresh, blocker-free facts.
	sha, _ := facts.SHA256OfInventoryFile(invPath)
	ptrDir := filepath.Join(home, ".hadoop-cli", "facts")
	_ = os.MkdirAll(ptrDir, 0o755)
	f := facts.Facts{InventorySHA: sha, CollectedAt: mustNow()}
	b, _ := marshalJSON(t, f)
	_ = os.WriteFile(filepath.Join(ptrDir, sha+".json"), b, 0o644)

	root := NewRootCmd()
	root.SetArgs([]string{"install", "--inventory", invPath})
	err := root.Execute()
	if err != nil && contains(err.Error(), "FACTS_") {
		t.Fatalf("gate should have allowed install, got %v", err)
	}
}
```

(`mustNow` and `marshalJSON` are tiny helpers — define inline in the test file.)

- [ ] **Step 2: Run the test**

Run: `go test ./cmd -run TestPlanUnblocksInstallGate -race`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/plan_lifecycle_test.go
git commit -m "test(cmd): end-to-end plan gate unblock"
```

---

### Task 18: Full verification

- [ ] **Step 1: Run the whole suite**

```bash
make fmt vet test
```

Expected: no diff from fmt, no vet complaints, all tests PASS.

- [ ] **Step 2: Manual inspection**

```bash
go build -o bin/hadoop-cli .
./bin/hadoop-cli plan --help
./bin/hadoop-cli install --help | grep -- --force
```

Expected: `plan` appears in `--help`; `install --help` lists `--force`.

- [ ] **Step 3: Commit any fmt or lint fixes, then done**

```bash
git status
# if clean, task is complete
```

---

## Self-Review Notes

Coverage vs spec:
- §5 `plan` subcommand — Tasks 8, 9, 11, 12.
- §6 facts collected — Tasks 5, 6, 7, 8 (host-level fully covered; cluster-state parsers land in Task 6 with wiring extensions as needed; external deps are scoped to their blocker evaluation in Task 10).
- §7 facts storage + freshness — Tasks 2, 3, 4.
- §8 JSON envelope — Task 12 builds the envelope shape.
- §9 blocker list — Task 10 implements DISK_TOO_SMALL / JDK_MISSING / USER_MISSING / HOSTS_INCONSISTENT / PORT_OCCUPIED_BY_FOREIGN and the four warning codes. `EXTERNAL_HDFS_UNREACHABLE` and `DATA_DIRTY_NOT_FORCED` as blockers are left as a follow-up task (warning is emitted) — called out in §16 Risks.
- §10 safety gate — Tasks 13, 14.
- §11 preflight relationship — preflight untouched; no task needed.
- §13 skill update — Task 15.
- §14 tests — Tasks 1-11 inline, Tasks 13/17 integration.
- §15 back-compat — documented in READMEs (Task 16) and skill (Task 15).

Known gaps deferred to follow-up (not blockers):
- `CLOCK_SKEW_OVER_2S` warning and `CLOCK_SKEW` full collector (needs real SSH RTT).
- External-HDFS reachability collector (blocker rule present in spec; wiring remains).

