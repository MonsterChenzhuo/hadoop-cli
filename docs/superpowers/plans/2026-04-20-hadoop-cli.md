# hadoop-cli Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a single-binary Go CLI (`hadoop-cli`) that bootstraps and manages an HBase cluster (HDFS single-NN + ZooKeeper + HBase) on a multi-node Linux/macOS environment via agentless SSH, with structured JSON output and two Claude Code skills that let Claude drive the whole lifecycle from one command.

**Architecture:** Control-machine-only static binary. Reads a YAML `cluster.yaml` inventory, concurrently SSHes to target nodes (crypto/ssh + sftp, goroutine pool), renders Hadoop/ZK/HBase configs from templates with built-in defaults, downloads Apache tarballs into a local cache (SHA-512), and orchestrates install/configure/start/stop/status/uninstall in a fixed dependency order (ZK → HDFS → HBase). All commands emit one JSON envelope on stdout and a stable error-code set for AI consumption.

**Tech Stack:** Go ≥ 1.23, `github.com/spf13/cobra`, `gopkg.in/yaml.v3`, `golang.org/x/crypto/ssh`, `github.com/pkg/sftp`, standard `text/template` + `encoding/xml`. Tests use stdlib `testing` + `github.com/stretchr/testify/require`.

**Reference spec:** `docs/superpowers/specs/2026-04-20-hadoop-cli-design.md`

---

## Conventions used in every task

- **TDD order:** test first → run and see it fail → minimal implementation → run and see it pass → commit.
- **Commit message style:** Conventional Commits in English (`feat:`, `fix:`, `test:`, `refactor:`, `docs:`, `chore:`).
- **Every commit footer:** `Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>`.
- **Run all tests before each commit:** `go test ./...`
- **Keep `gofmt -l .` empty and `go vet ./...` clean** — both run before commit.
- **Module path:** `github.com/hadoop-cli/hadoop-cli` (pick any; this plan uses that path — replace consistently if the user prefers another).

---

## Task 1: Project skeleton and root Cobra command

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `cmd/root.go`
- Create: `cmd/root_test.go`
- Create: `Makefile`
- Create: `.gitignore`
- Create: `.golangci.yml`

- [ ] **Step 1: Initialize Go module and commit baseline ignore files**

Run:
```bash
cd hadoop-cli
go mod init github.com/hadoop-cli/hadoop-cli
```

Create `.gitignore`:
```gitignore
/bin/
/dist/
*.test
*.out
.idea/
.vscode/
.DS_Store
```

Create `.golangci.yml`:
```yaml
version: "2"
run:
  timeout: 3m
linters:
  enable:
    - gofmt
    - govet
    - ineffassign
    - misspell
    - staticcheck
    - unused
```

- [ ] **Step 2: Write the root command failing test**

Create `cmd/root_test.go`:
```go
package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootCommand_ShowsHelpByDefault(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})

	err := root.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "hadoop-cli")
	require.Contains(t, buf.String(), "Available Commands")
}

func TestRootCommand_HasVersion(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--version"})

	err := root.Execute()
	require.NoError(t, err)
	require.Contains(t, buf.String(), "hadoop-cli")
}
```

Add dependencies:
```bash
go get github.com/spf13/cobra@latest
go get github.com/stretchr/testify@latest
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./cmd/...`
Expected: FAIL with `cmd/root.go` not found / `NewRootCmd` undefined.

- [ ] **Step 4: Implement root command**

Create `cmd/root.go`:
```go
package cmd

import (
	"github.com/spf13/cobra"
)

var Version = "0.1.0-dev"

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "hadoop-cli",
		Short:         "hadoop-cli bootstraps and manages HBase clusters (HDFS + ZooKeeper + HBase).",
		Long:          "hadoop-cli is a single-binary CLI that installs, configures, starts, stops, and uninstalls an HBase cluster over SSH, driven by a YAML inventory.",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().String("inventory", "cluster.yaml", "path to cluster inventory YAML")
	root.PersistentFlags().String("log-level", "info", "log level: debug|info|warn|error")
	root.PersistentFlags().Bool("no-color", false, "disable color in stderr progress output")
	return root
}
```

Create `main.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/hadoop-cli/hadoop-cli/cmd"
)

func main() {
	root := cmd.NewRootCmd()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run:
```bash
go mod tidy
go test ./... -race
go vet ./...
gofmt -l .
```

Expected: tests pass, no vet errors, gofmt prints nothing.

- [ ] **Step 6: Add Makefile and commit**

Create `Makefile`:
```makefile
GO ?= go
BIN := bin/hadoop-cli
PKG := ./...

.PHONY: all build test vet fmt lint tidy clean

all: fmt vet test build

build:
	$(GO) build -o $(BIN) .

test:
	$(GO) test $(PKG) -race

vet:
	$(GO) vet $(PKG)

fmt:
	$(GO) fmt $(PKG)
	@test -z "$$(gofmt -l .)" || (echo 'gofmt diff found'; gofmt -l .; exit 1)

tidy:
	$(GO) mod tidy

lint:
	$(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 run

clean:
	rm -rf bin dist
```

Commit:
```bash
git add .
git commit -m "$(cat <<'EOF'
feat: initial project skeleton with Cobra root command

Set up Go module, root command with --help/--version, Makefile targets,
gitignore and golangci config. This is the scaffold all subsequent tasks
build on.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: `internal/output` — structured JSON envelope

**Files:**
- Create: `internal/output/envelope.go`
- Create: `internal/output/envelope_test.go`
- Create: `internal/output/progress.go`
- Create: `internal/output/progress_test.go`

- [ ] **Step 1: Write envelope failing tests**

Create `internal/output/envelope_test.go`:
```go
package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEnvelope_SuccessMarshalsOkTrue(t *testing.T) {
	buf := &bytes.Buffer{}
	env := NewEnvelope("install").WithSummary(map[string]any{
		"hosts_total": 3,
		"hosts_ok":    3,
		"elapsed_ms":  1000,
	})
	env.AddHost(HostResult{Host: "node1", OK: true, ElapsedMs: 333})

	require.NoError(t, env.Write(buf))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Equal(t, "install", decoded["command"])
	require.Equal(t, true, decoded["ok"])
	require.NotNil(t, decoded["summary"])
	require.Len(t, decoded["hosts"], 1)
}

func TestEnvelope_FailureIncludesError(t *testing.T) {
	buf := &bytes.Buffer{}
	env := NewEnvelope("install").WithError(EnvelopeError{
		Code:    "SSH_AUTH_FAILED",
		Host:    "node2",
		Message: "public key authentication failed",
		Hint:    "check ssh.private_key in inventory",
	})

	require.NoError(t, env.Write(buf))

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	require.Equal(t, false, decoded["ok"])
	errObj := decoded["error"].(map[string]any)
	require.Equal(t, "SSH_AUTH_FAILED", errObj["code"])
	require.Equal(t, "node2", errObj["host"])
}
```

- [ ] **Step 2: Run test, see it fail**

Run: `go test ./internal/output/...`
Expected: FAIL — package not found.

- [ ] **Step 3: Implement envelope**

Create `internal/output/envelope.go`:
```go
package output

import (
	"encoding/json"
	"io"
)

type HostResult struct {
	Host      string `json:"host"`
	OK        bool   `json:"ok"`
	ElapsedMs int64  `json:"elapsed_ms"`
	Message   string `json:"message,omitempty"`
}

type EnvelopeError struct {
	Code    string `json:"code"`
	Host    string `json:"host,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

type Envelope struct {
	Command string         `json:"command"`
	OK      bool           `json:"ok"`
	Summary map[string]any `json:"summary,omitempty"`
	Hosts   []HostResult   `json:"hosts,omitempty"`
	Error   *EnvelopeError `json:"error,omitempty"`
	RunID   string         `json:"run_id,omitempty"`
}

func NewEnvelope(command string) *Envelope {
	return &Envelope{Command: command, OK: true}
}

func (e *Envelope) WithSummary(s map[string]any) *Envelope {
	e.Summary = s
	return e
}

func (e *Envelope) AddHost(r HostResult) *Envelope {
	if !r.OK {
		e.OK = false
	}
	e.Hosts = append(e.Hosts, r)
	return e
}

func (e *Envelope) WithError(err EnvelopeError) *Envelope {
	e.OK = false
	e.Error = &err
	return e
}

func (e *Envelope) WithRunID(id string) *Envelope {
	e.RunID = id
	return e
}

func (e *Envelope) Write(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(e)
}
```

- [ ] **Step 4: Write progress logger failing tests**

Create `internal/output/progress_test.go`:
```go
package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProgress_WritesHostPrefixedLine(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, false)
	p.Infof("node1", "extracting hbase-2.5.8-bin.tar.gz … ok (3.2s)")
	require.Contains(t, buf.String(), "[node1] extracting hbase-2.5.8-bin.tar.gz")
}

func TestProgress_ColorDisabled(t *testing.T) {
	buf := &bytes.Buffer{}
	p := NewProgress(buf, true)
	p.Errorf("node2", "boom")
	require.False(t, strings.Contains(buf.String(), "\x1b["))
}
```

- [ ] **Step 5: Run tests, see fail**

Run: `go test ./internal/output/...`
Expected: FAIL — `NewProgress` undefined.

- [ ] **Step 6: Implement progress logger**

Create `internal/output/progress.go`:
```go
package output

import (
	"fmt"
	"io"
	"sync"
)

type Progress struct {
	mu      sync.Mutex
	w       io.Writer
	noColor bool
}

func NewProgress(w io.Writer, noColor bool) *Progress {
	return &Progress{w: w, noColor: noColor}
}

func (p *Progress) Infof(host, format string, args ...any) {
	p.writef("", host, format, args...)
}

func (p *Progress) Warnf(host, format string, args ...any) {
	p.writef("WARN ", host, format, args...)
}

func (p *Progress) Errorf(host, format string, args ...any) {
	p.writef("ERROR ", host, format, args...)
}

func (p *Progress) writef(level, host, format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	prefix := level
	if host != "" {
		prefix = prefix + "[" + host + "] "
	}
	fmt.Fprintf(p.w, "%s%s\n", prefix, fmt.Sprintf(format, args...))
}
```

- [ ] **Step 7: Run tests, commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/output
git commit -m "$(cat <<'EOF'
feat(output): add structured JSON envelope and stderr progress logger

JSON envelope (stdout) is the single contract Claude parses; progress logger
(stderr) stays human-readable and does not pollute the pipe.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: `internal/errs` — stable error codes + hints

**Files:**
- Create: `internal/errs/errs.go`
- Create: `internal/errs/errs_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/errs/errs_test.go`:
```go
package errs

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_CarriesCodeAndHint(t *testing.T) {
	e := New(CodeSSHAuthFailed, "node2", "pubkey denied")
	var ce *CodedError
	require.True(t, errors.As(e, &ce))
	require.Equal(t, CodeSSHAuthFailed, ce.Code)
	require.Equal(t, "node2", ce.Host)
	require.Contains(t, ce.Error(), "pubkey denied")
	require.NotEmpty(t, ce.Hint)
}

func TestHintRegistry_CoversAllCodes(t *testing.T) {
	for _, c := range AllCodes() {
		require.NotEmptyf(t, HintFor(c), "missing hint for %s", c)
	}
}
```

- [ ] **Step 2: Run test, see fail**

Run: `go test ./internal/errs/...`
Expected: FAIL — undefined types.

- [ ] **Step 3: Implement errs package**

Create `internal/errs/errs.go`:
```go
package errs

import "fmt"

type Code string

const (
	CodeSSHConnectFailed            Code = "SSH_CONNECT_FAILED"
	CodeSSHAuthFailed               Code = "SSH_AUTH_FAILED"
	CodePreflightJDKMissing         Code = "PREFLIGHT_JDK_MISSING"
	CodePreflightPortBusy           Code = "PREFLIGHT_PORT_BUSY"
	CodePreflightHostUnresolvable   Code = "PREFLIGHT_HOSTNAME_UNRESOLVABLE"
	CodePreflightDiskLow            Code = "PREFLIGHT_DISK_LOW"
	CodePreflightClockSkew          Code = "PREFLIGHT_CLOCK_SKEW"
	CodeDownloadFailed              Code = "DOWNLOAD_FAILED"
	CodeDownloadChecksumMismatch    Code = "DOWNLOAD_CHECKSUM_MISMATCH"
	CodeConfigRenderFailed          Code = "CONFIG_RENDER_FAILED"
	CodeRemoteCommandFailed         Code = "REMOTE_COMMAND_FAILED"
	CodeTimeout                     Code = "TIMEOUT"
	CodeInventoryInvalid            Code = "INVENTORY_INVALID"
	CodeComponentNotReady           Code = "COMPONENT_NOT_READY"
)

var hints = map[Code]string{
	CodeSSHConnectFailed:          "verify host reachability (ping / nc) and ssh.port in inventory",
	CodeSSHAuthFailed:             "check ssh.private_key in inventory and run `ssh-copy-id` to the target host",
	CodePreflightJDKMissing:       "install JDK 8 or 11 on the host and set cluster.java_home to a valid path",
	CodePreflightPortBusy:         "free the listed port on the host (lsof -i :<port>) or change overrides to another port",
	CodePreflightHostUnresolvable: "add host entries to /etc/hosts on every node so names resolve consistently",
	CodePreflightDiskLow:          "free space under cluster.data_dir (need at least a few GB)",
	CodePreflightClockSkew:        "enable ntpd / chrony so all nodes are within 30s of each other",
	CodeDownloadFailed:            "check the control machine's outbound network or pre-populate ~/.hadoop-cli/cache",
	CodeDownloadChecksumMismatch:  "delete the cached tarball and rerun; the Apache mirror may be corrupt",
	CodeConfigRenderFailed:        "this is a bug in a template; file an issue with the render error message",
	CodeRemoteCommandFailed:       "inspect the run directory ~/.hadoop-cli/runs/<run-id>/<host>.stderr for the exact remote failure",
	CodeTimeout:                   "the task exceeded its deadline; rerun with --log-level debug and investigate the slow host",
	CodeInventoryInvalid:          "fix cluster.yaml per the message (schema is documented in hbase-cluster-bootstrap skill)",
	CodeComponentNotReady:         "wait for prerequisites (ZK quorum, NN live) or rerun `start` which retries",
}

type CodedError struct {
	Code    Code
	Host    string
	Message string
	Cause   error
}

func (e *CodedError) Error() string {
	if e.Host != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Code, e.Host, e.Message)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func (e *CodedError) Unwrap() error { return e.Cause }

func (e *CodedError) Hint() string { return HintFor(e.Code) }

func New(code Code, host, message string) *CodedError {
	return &CodedError{Code: code, Host: host, Message: message}
}

func Wrap(code Code, host string, cause error) *CodedError {
	return &CodedError{Code: code, Host: host, Message: cause.Error(), Cause: cause}
}

func HintFor(code Code) string {
	return hints[code]
}

func AllCodes() []Code {
	out := make([]Code, 0, len(hints))
	for k := range hints {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 4: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/errs
git commit -m "$(cat <<'EOF'
feat(errs): add stable error-code registry with per-code hints

Every remediable failure maps to a Code and a fixed Hint. Claude reads the
hint to pick the next action automatically.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: `internal/inventory` — YAML types and loader

**Files:**
- Create: `internal/inventory/types.go`
- Create: `internal/inventory/load.go`
- Create: `internal/inventory/load_test.go`
- Create: `internal/inventory/testdata/valid-3-node.yaml`

- [ ] **Step 1: Add yaml dependency**

Run:
```bash
go get gopkg.in/yaml.v3@latest
```

- [ ] **Step 2: Create golden fixture**

Create `internal/inventory/testdata/valid-3-node.yaml`:
```yaml
cluster:
  name: hbase-dev
  install_dir: /opt/hadoop-cli
  data_dir: /data/hadoop-cli
  user: hadoop
  java_home: /usr/lib/jvm/java-11
versions:
  hadoop: 3.3.6
  zookeeper: 3.8.4
  hbase: 2.5.8
ssh:
  port: 22
  user: hadoop
  private_key: ~/.ssh/id_rsa
  parallelism: 8
  sudo: false
hosts:
  - { name: node1, address: 10.0.0.11 }
  - { name: node2, address: 10.0.0.12 }
  - { name: node3, address: 10.0.0.13 }
roles:
  namenode:     [node1]
  datanode:     [node1, node2, node3]
  zookeeper:    [node1, node2, node3]
  hbase_master: [node1]
  regionserver: [node1, node2, node3]
overrides:
  hdfs:
    replication: 2
    namenode_heap: 1g
    datanode_heap: 1g
  zookeeper:
    client_port: 2181
    tick_time: 2000
  hbase:
    master_heap: 1g
    regionserver_heap: 2g
```

- [ ] **Step 3: Write failing loader test**

Create `internal/inventory/load_test.go`:
```go
package inventory

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoad_ParsesValidInventory(t *testing.T) {
	inv, err := Load(filepath.Join("testdata", "valid-3-node.yaml"))
	require.NoError(t, err)

	require.Equal(t, "hbase-dev", inv.Cluster.Name)
	require.Equal(t, "/opt/hadoop-cli", inv.Cluster.InstallDir)
	require.Equal(t, "3.3.6", inv.Versions.Hadoop)
	require.Equal(t, 8, inv.SSH.Parallelism)
	require.Len(t, inv.Hosts, 3)
	require.Equal(t, []string{"node1"}, inv.Roles.NameNode)
	require.Equal(t, []string{"node1", "node2", "node3"}, inv.Roles.ZooKeeper)
	require.Equal(t, 2, inv.Overrides.HDFS.Replication)
	require.Equal(t, "2g", inv.Overrides.HBase.RegionServerHeap)
}

func TestLoad_AppliesDefaultsWhenUnset(t *testing.T) {
	inv, err := LoadBytes([]byte(`
cluster:
  name: demo
  install_dir: /opt/hadoop-cli
  data_dir: /data/hadoop-cli
  user: hadoop
  java_home: /usr/lib/jvm/java-11
versions: { hadoop: 3.3.6, zookeeper: 3.8.4, hbase: 2.5.8 }
ssh: { user: hadoop, private_key: ~/.ssh/id_rsa }
hosts:
  - { name: n1, address: 127.0.0.1 }
roles:
  namenode: [n1]
  datanode: [n1]
  zookeeper: [n1]
  hbase_master: [n1]
  regionserver: [n1]
`))
	require.NoError(t, err)
	require.Equal(t, 22, inv.SSH.Port)
	require.Equal(t, 8, inv.SSH.Parallelism)
	require.Equal(t, 3, inv.Overrides.HDFS.Replication)
	require.Equal(t, 2181, inv.Overrides.ZooKeeper.ClientPort)
}

func TestLoad_FailsOnUnknownField(t *testing.T) {
	_, err := LoadBytes([]byte(`cluster: { name: x, install_dir: /a, data_dir: /b, user: u, java_home: /j }
this_field_does_not_exist: 1
`))
	require.Error(t, err)
}
```

- [ ] **Step 4: Run test, see fail**

Run: `go test ./internal/inventory/...`
Expected: FAIL — `Load` undefined.

- [ ] **Step 5: Implement types**

Create `internal/inventory/types.go`:
```go
package inventory

type Cluster struct {
	Name       string `yaml:"name"`
	InstallDir string `yaml:"install_dir"`
	DataDir    string `yaml:"data_dir"`
	User       string `yaml:"user"`
	JavaHome   string `yaml:"java_home"`
}

type Versions struct {
	Hadoop    string `yaml:"hadoop"`
	ZooKeeper string `yaml:"zookeeper"`
	HBase     string `yaml:"hbase"`
}

type SSH struct {
	Port        int    `yaml:"port"`
	User        string `yaml:"user"`
	PrivateKey  string `yaml:"private_key"`
	Parallelism int    `yaml:"parallelism"`
	Sudo        bool   `yaml:"sudo"`
}

type Host struct {
	Name    string `yaml:"name"`
	Address string `yaml:"address"`
}

type Roles struct {
	NameNode     []string `yaml:"namenode"`
	DataNode     []string `yaml:"datanode"`
	ZooKeeper    []string `yaml:"zookeeper"`
	HBaseMaster  []string `yaml:"hbase_master"`
	RegionServer []string `yaml:"regionserver"`
}

type HDFSOverrides struct {
	Replication  int    `yaml:"replication"`
	NameNodeHeap string `yaml:"namenode_heap"`
	DataNodeHeap string `yaml:"datanode_heap"`
	NameNodeRPC  int    `yaml:"namenode_rpc_port"`
	NameNodeHTTP int    `yaml:"namenode_http_port"`
}

type ZKOverrides struct {
	ClientPort int `yaml:"client_port"`
	TickTime   int `yaml:"tick_time"`
	InitLimit  int `yaml:"init_limit"`
	SyncLimit  int `yaml:"sync_limit"`
}

type HBaseOverrides struct {
	MasterHeap       string `yaml:"master_heap"`
	RegionServerHeap string `yaml:"regionserver_heap"`
	RootDir          string `yaml:"root_dir"`
	MasterPort       int    `yaml:"master_port"`
	MasterInfoPort   int    `yaml:"master_info_port"`
	RSPort           int    `yaml:"regionserver_port"`
	RSInfoPort       int    `yaml:"regionserver_info_port"`
}

type Overrides struct {
	HDFS      HDFSOverrides  `yaml:"hdfs"`
	ZooKeeper ZKOverrides    `yaml:"zookeeper"`
	HBase     HBaseOverrides `yaml:"hbase"`
}

type Inventory struct {
	Cluster   Cluster   `yaml:"cluster"`
	Versions  Versions  `yaml:"versions"`
	SSH       SSH       `yaml:"ssh"`
	Hosts     []Host    `yaml:"hosts"`
	Roles     Roles     `yaml:"roles"`
	Overrides Overrides `yaml:"overrides"`
}

func (i *Inventory) HostByName(name string) (Host, bool) {
	for _, h := range i.Hosts {
		if h.Name == name {
			return h, true
		}
	}
	return Host{}, false
}

func (i *Inventory) AllRoleHosts() []string {
	seen := map[string]struct{}{}
	add := func(xs []string) {
		for _, x := range xs {
			seen[x] = struct{}{}
		}
	}
	add(i.Roles.NameNode)
	add(i.Roles.DataNode)
	add(i.Roles.ZooKeeper)
	add(i.Roles.HBaseMaster)
	add(i.Roles.RegionServer)
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 6: Implement loader with defaults**

Create `internal/inventory/load.go`:
```go
package inventory

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Inventory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return LoadBytes(data)
}

func LoadBytes(data []byte) (*Inventory, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	inv := &Inventory{}
	if err := dec.Decode(inv); err != nil {
		return nil, fmt.Errorf("parse inventory: %w", err)
	}
	applyDefaults(inv)
	return inv, nil
}

func applyDefaults(inv *Inventory) {
	if inv.SSH.Port == 0 {
		inv.SSH.Port = 22
	}
	if inv.SSH.Parallelism == 0 {
		inv.SSH.Parallelism = 8
	}
	if inv.Overrides.HDFS.Replication == 0 {
		inv.Overrides.HDFS.Replication = 3
	}
	if inv.Overrides.HDFS.NameNodeHeap == "" {
		inv.Overrides.HDFS.NameNodeHeap = "1g"
	}
	if inv.Overrides.HDFS.DataNodeHeap == "" {
		inv.Overrides.HDFS.DataNodeHeap = "1g"
	}
	if inv.Overrides.HDFS.NameNodeRPC == 0 {
		inv.Overrides.HDFS.NameNodeRPC = 8020
	}
	if inv.Overrides.HDFS.NameNodeHTTP == 0 {
		inv.Overrides.HDFS.NameNodeHTTP = 9870
	}
	if inv.Overrides.ZooKeeper.ClientPort == 0 {
		inv.Overrides.ZooKeeper.ClientPort = 2181
	}
	if inv.Overrides.ZooKeeper.TickTime == 0 {
		inv.Overrides.ZooKeeper.TickTime = 2000
	}
	if inv.Overrides.ZooKeeper.InitLimit == 0 {
		inv.Overrides.ZooKeeper.InitLimit = 10
	}
	if inv.Overrides.ZooKeeper.SyncLimit == 0 {
		inv.Overrides.ZooKeeper.SyncLimit = 5
	}
	if inv.Overrides.HBase.MasterHeap == "" {
		inv.Overrides.HBase.MasterHeap = "1g"
	}
	if inv.Overrides.HBase.RegionServerHeap == "" {
		inv.Overrides.HBase.RegionServerHeap = "1g"
	}
	if inv.Overrides.HBase.MasterPort == 0 {
		inv.Overrides.HBase.MasterPort = 16000
	}
	if inv.Overrides.HBase.MasterInfoPort == 0 {
		inv.Overrides.HBase.MasterInfoPort = 16010
	}
	if inv.Overrides.HBase.RSPort == 0 {
		inv.Overrides.HBase.RSPort = 16020
	}
	if inv.Overrides.HBase.RSInfoPort == 0 {
		inv.Overrides.HBase.RSInfoPort = 16030
	}
}
```

- [ ] **Step 7: Run tests and commit**

```bash
go mod tidy
go test ./... -race
go vet ./...
gofmt -l .
git add internal/inventory go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(inventory): load cluster.yaml with strict field checks and defaults

Strict YAML decoding (KnownFields=true) rejects typos; baked-in defaults keep
minimal inventories working without boilerplate.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: `internal/inventory` — validation

**Files:**
- Create: `internal/inventory/validate.go`
- Create: `internal/inventory/validate_test.go`

- [ ] **Step 1: Write failing validation tests**

Create `internal/inventory/validate_test.go`:
```go
package inventory

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func baseInv() *Inventory {
	return &Inventory{
		Cluster:  Cluster{Name: "c", InstallDir: "/opt/hadoop-cli", DataDir: "/data/hadoop-cli", User: "hadoop", JavaHome: "/j"},
		Versions: Versions{Hadoop: "3.3.6", ZooKeeper: "3.8.4", HBase: "2.5.8"},
		SSH:      SSH{Port: 22, User: "hadoop", PrivateKey: "~/.ssh/id_rsa", Parallelism: 8},
		Hosts: []Host{
			{Name: "n1", Address: "10.0.0.1"},
			{Name: "n2", Address: "10.0.0.2"},
			{Name: "n3", Address: "10.0.0.3"},
		},
		Roles: Roles{
			NameNode:     []string{"n1"},
			DataNode:     []string{"n1", "n2", "n3"},
			ZooKeeper:    []string{"n1", "n2", "n3"},
			HBaseMaster:  []string{"n1"},
			RegionServer: []string{"n1", "n2", "n3"},
		},
	}
}

func TestValidate_OK(t *testing.T) {
	require.NoError(t, Validate(baseInv()))
}

func TestValidate_RejectsMultipleNameNodes(t *testing.T) {
	inv := baseInv()
	inv.Roles.NameNode = []string{"n1", "n2"}
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "namenode")
}

func TestValidate_RejectsEvenZooKeeperCount(t *testing.T) {
	inv := baseInv()
	inv.Roles.ZooKeeper = []string{"n1", "n2"}
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "odd")
}

func TestValidate_RejectsUnknownHostRef(t *testing.T) {
	inv := baseInv()
	inv.Roles.RegionServer = []string{"n1", "ghost"}
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "ghost")
}

func TestValidate_RejectsRelativePaths(t *testing.T) {
	inv := baseInv()
	inv.Cluster.InstallDir = "opt/hadoop-cli"
	err := Validate(inv)
	require.Error(t, err)
	require.Contains(t, err.Error(), "install_dir")
}

func TestValidate_RejectsUnsupportedVersion(t *testing.T) {
	inv := baseInv()
	inv.Versions.HBase = "1.0.0"
	err := Validate(inv)
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test, see fail**

Run: `go test ./internal/inventory/...`
Expected: FAIL — `Validate` undefined.

- [ ] **Step 3: Implement validator**

Create `internal/inventory/validate.go`:
```go
package inventory

import (
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/errs"
)

var supportedVersions = struct {
	Hadoop    []string
	ZooKeeper []string
	HBase     []string
}{
	Hadoop:    []string{"3.3.4", "3.3.5", "3.3.6"},
	ZooKeeper: []string{"3.7.2", "3.8.3", "3.8.4"},
	HBase:     []string{"2.5.5", "2.5.7", "2.5.8"},
}

func Validate(inv *Inventory) error {
	var msgs []string
	add := func(s string) { msgs = append(msgs, s) }

	if !strings.HasPrefix(inv.Cluster.InstallDir, "/") {
		add("cluster.install_dir must be an absolute path")
	}
	if !strings.HasPrefix(inv.Cluster.DataDir, "/") {
		add("cluster.data_dir must be an absolute path")
	}
	if inv.Cluster.Name == "" {
		add("cluster.name is required")
	}
	if inv.Cluster.User == "" {
		add("cluster.user is required")
	}
	if inv.SSH.User == "" {
		add("ssh.user is required")
	}
	if inv.SSH.PrivateKey == "" {
		add("ssh.private_key is required")
	}

	if !contains(supportedVersions.Hadoop, inv.Versions.Hadoop) {
		add(fmt.Sprintf("unsupported hadoop version %q; supported: %s",
			inv.Versions.Hadoop, strings.Join(supportedVersions.Hadoop, ", ")))
	}
	if !contains(supportedVersions.ZooKeeper, inv.Versions.ZooKeeper) {
		add(fmt.Sprintf("unsupported zookeeper version %q; supported: %s",
			inv.Versions.ZooKeeper, strings.Join(supportedVersions.ZooKeeper, ", ")))
	}
	if !contains(supportedVersions.HBase, inv.Versions.HBase) {
		add(fmt.Sprintf("unsupported hbase version %q; supported: %s",
			inv.Versions.HBase, strings.Join(supportedVersions.HBase, ", ")))
	}

	if len(inv.Roles.NameNode) != 1 {
		add(fmt.Sprintf("roles.namenode must have exactly 1 host (v1 single-NN); got %d", len(inv.Roles.NameNode)))
	}
	if n := len(inv.Roles.ZooKeeper); n == 0 || n%2 == 0 {
		add(fmt.Sprintf("roles.zookeeper must have an odd number of hosts (1,3,5); got %d", n))
	}
	if len(inv.Roles.DataNode) == 0 {
		add("roles.datanode must not be empty")
	}
	if len(inv.Roles.HBaseMaster) == 0 {
		add("roles.hbase_master must not be empty")
	}
	if len(inv.Roles.RegionServer) == 0 {
		add("roles.regionserver must not be empty")
	}

	hostNames := map[string]bool{}
	for _, h := range inv.Hosts {
		if h.Name == "" || h.Address == "" {
			add(fmt.Sprintf("hosts entry missing name or address: %+v", h))
			continue
		}
		if hostNames[h.Name] {
			add(fmt.Sprintf("duplicate host name %q", h.Name))
		}
		hostNames[h.Name] = true
	}
	for role, list := range map[string][]string{
		"namenode":     inv.Roles.NameNode,
		"datanode":     inv.Roles.DataNode,
		"zookeeper":    inv.Roles.ZooKeeper,
		"hbase_master": inv.Roles.HBaseMaster,
		"regionserver": inv.Roles.RegionServer,
	} {
		for _, name := range list {
			if !hostNames[name] {
				add(fmt.Sprintf("roles.%s references unknown host %q", role, name))
			}
		}
	}

	if len(msgs) > 0 {
		return errs.New(errs.CodeInventoryInvalid, "", strings.Join(msgs, "; "))
	}
	return nil
}

func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/inventory
git commit -m "$(cat <<'EOF'
feat(inventory): enforce single-NN, odd ZK count, version allowlist, host refs

Validation rejects the common misconfigurations listed in the design spec
and surfaces them as INVENTORY_INVALID with a concrete message.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: `internal/ssh` — SSH client with exec/sftp and in-process test server

**Files:**
- Create: `internal/ssh/client.go`
- Create: `internal/ssh/pool.go`
- Create: `internal/ssh/client_test.go`
- Create: `internal/ssh/testserver_test.go`

- [ ] **Step 1: Add dependencies**

```bash
go get golang.org/x/crypto/ssh@latest
go get github.com/pkg/sftp@latest
go get github.com/gliderlabs/ssh@latest
```

(`gliderlabs/ssh` is used only in tests to spin up an in-process SSH server.)

- [ ] **Step 2: Write test server helper (test file)**

Create `internal/ssh/testserver_test.go`:
```go
package ssh

import (
	"io"
	"net"
	"os/exec"
	"testing"

	gssh "github.com/gliderlabs/ssh"
	"github.com/stretchr/testify/require"
)

// startTestServer starts an in-process SSH server that runs each requested
// command with /bin/sh -c. Returns host:port and a stop func.
func startTestServer(t *testing.T) (string, func()) {
	t.Helper()
	lst, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	srv := &gssh.Server{
		Handler: func(s gssh.Session) {
			cmd := exec.Command("/bin/sh", "-c", s.RawCommand())
			stdout, _ := cmd.StdoutPipe()
			stderr, _ := cmd.StderrPipe()
			_ = cmd.Start()
			go io.Copy(s, stdout)
			go io.Copy(s.Stderr(), stderr)
			err := cmd.Wait()
			if exit, ok := err.(*exec.ExitError); ok {
				_ = s.Exit(exit.ExitCode())
				return
			}
			_ = s.Exit(0)
		},
		PasswordHandler: func(gssh.Context, string) bool { return true },
	}
	srv.SetOption(gssh.HostKeyFile("")) // random ephemeral key
	go func() { _ = srv.Serve(lst) }()
	return lst.Addr().String(), func() { _ = srv.Close() }
}
```

- [ ] **Step 3: Write failing client test**

Create `internal/ssh/client_test.go`:
```go
package ssh

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_ExecCapturesStdoutAndExitCode(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	c, err := Dial(Config{
		Host:     host,
		Port:     port,
		User:     "test",
		Password: "x",
		Timeout:  2 * time.Second,
	})
	require.NoError(t, err)
	defer c.Close()

	res, err := c.Exec(context.Background(), "echo hello && echo err 1>&2")
	require.NoError(t, err)
	require.Equal(t, 0, res.ExitCode)
	require.Contains(t, res.Stdout, "hello")
	require.Contains(t, res.Stderr, "err")
}

func TestClient_ExecPropagatesNonZeroExit(t *testing.T) {
	addr, stop := startTestServer(t)
	defer stop()

	host, portStr, _ := net.SplitHostPort(addr)
	port, _ := strconv.Atoi(portStr)

	c, err := Dial(Config{Host: host, Port: port, User: "t", Password: "x", Timeout: 2 * time.Second})
	require.NoError(t, err)
	defer c.Close()

	res, err := c.Exec(context.Background(), "exit 7")
	require.NoError(t, err) // Exec returns no error when process ran
	require.Equal(t, 7, res.ExitCode)
}
```

- [ ] **Step 4: Run tests, see fail**

Run: `go test ./internal/ssh/...`
Expected: FAIL — undefined `Dial`/`Config`.

- [ ] **Step 5: Implement SSH client**

Create `internal/ssh/client.go`:
```go
package ssh

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pkg/sftp"
	xssh "golang.org/x/crypto/ssh"
)

type Config struct {
	Host       string
	Port       int
	User       string
	PrivateKey string
	Password   string // only used in tests
	Timeout    time.Duration
}

type Client struct {
	conn *xssh.Client
	cfg  Config
}

type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func Dial(cfg Config) (*Client, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.Port == 0 {
		cfg.Port = 22
	}

	auths := []xssh.AuthMethod{}
	if cfg.PrivateKey != "" {
		path := cfg.PrivateKey
		if path[:1] == "~" {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[1:])
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read private key %s: %w", path, err)
		}
		signer, err := xssh.ParsePrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		auths = append(auths, xssh.PublicKeys(signer))
	}
	if cfg.Password != "" {
		auths = append(auths, xssh.Password(cfg.Password))
	}

	clientCfg := &xssh.ClientConfig{
		User:            cfg.User,
		Auth:            auths,
		HostKeyCallback: xssh.InsecureIgnoreHostKey(),
		Timeout:         cfg.Timeout,
	}
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	conn, err := xssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, cfg: cfg}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) Exec(ctx context.Context, cmd string) (*ExecResult, error) {
	sess, err := c.conn.NewSession()
	if err != nil {
		return nil, err
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()

	select {
	case <-ctx.Done():
		_ = sess.Signal(xssh.SIGKILL)
		return nil, ctx.Err()
	case err := <-done:
		exit := 0
		if err != nil {
			if xerr, ok := err.(*xssh.ExitError); ok {
				exit = xerr.ExitStatus()
			} else {
				return &ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}, err
			}
		}
		return &ExecResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: exit}, nil
	}
}

func (c *Client) Upload(ctx context.Context, local, remote string, mode os.FileMode) error {
	sc, err := sftp.NewClient(c.conn)
	if err != nil {
		return err
	}
	defer sc.Close()

	if err := sc.MkdirAll(filepath.Dir(remote)); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(remote), err)
	}
	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := sc.Create(remote)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return sc.Chmod(remote, mode)
}

func (c *Client) WriteFile(ctx context.Context, remote string, content []byte, mode os.FileMode) error {
	sc, err := sftp.NewClient(c.conn)
	if err != nil {
		return err
	}
	defer sc.Close()

	if err := sc.MkdirAll(filepath.Dir(remote)); err != nil {
		return err
	}
	dst, err := sc.Create(remote)
	if err != nil {
		return err
	}
	defer dst.Close()
	if _, err := dst.Write(content); err != nil {
		return err
	}
	return sc.Chmod(remote, mode)
}
```

- [ ] **Step 6: Implement connection pool**

Create `internal/ssh/pool.go`:
```go
package ssh

import (
	"sync"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

type Pool struct {
	mu      sync.Mutex
	clients map[string]*Client
	inv     *inventory.Inventory
}

func NewPool(inv *inventory.Inventory) *Pool {
	return &Pool{clients: map[string]*Client{}, inv: inv}
}

func (p *Pool) Get(hostName string) (*Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.clients[hostName]; ok {
		return c, nil
	}
	h, ok := p.inv.HostByName(hostName)
	if !ok {
		return nil, &UnknownHostError{Name: hostName}
	}
	c, err := Dial(Config{
		Host:       h.Address,
		Port:       p.inv.SSH.Port,
		User:       p.inv.SSH.User,
		PrivateKey: p.inv.SSH.PrivateKey,
	})
	if err != nil {
		return nil, err
	}
	p.clients[hostName] = c
	return c, nil
}

func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range p.clients {
		_ = c.Close()
	}
	p.clients = map[string]*Client{}
}

type UnknownHostError struct{ Name string }

func (e *UnknownHostError) Error() string { return "unknown host: " + e.Name }
```

- [ ] **Step 7: Run tests and commit**

```bash
go mod tidy
go test ./... -race
go vet ./...
gofmt -l .
git add internal/ssh go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(ssh): SSH client with exec/sftp and per-host connection pool

Uses crypto/ssh + pkg/sftp; tests spin up an in-process gliderlabs/ssh server
so CI doesn't need network access. Pool keeps one long connection per host.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: `internal/orchestrator` — parallel task runner

**Files:**
- Create: `internal/orchestrator/task.go`
- Create: `internal/orchestrator/runner.go`
- Create: `internal/orchestrator/runner_test.go`

- [ ] **Step 1: Write failing runner test**

Create `internal/orchestrator/runner_test.go`:
```go
package orchestrator

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type fakeExecutor struct {
	calls int32
	fn    func(ctx context.Context, host string, task Task) Result
}

func (f *fakeExecutor) Execute(ctx context.Context, host string, task Task) Result {
	atomic.AddInt32(&f.calls, 1)
	return f.fn(ctx, host, task)
}

func TestRunner_RunsInParallelAndAggregates(t *testing.T) {
	fe := &fakeExecutor{fn: func(ctx context.Context, host string, _ Task) Result {
		time.Sleep(50 * time.Millisecond)
		return Result{Host: host, OK: true}
	}}
	r := NewRunner(fe, 4)

	start := time.Now()
	results := r.Run(context.Background(), []string{"a", "b", "c", "d"}, Task{Name: "x"})
	elapsed := time.Since(start)

	require.Len(t, results, 4)
	for _, r := range results {
		require.True(t, r.OK)
	}
	require.Less(t, elapsed, 180*time.Millisecond, "should run in parallel")
	require.Equal(t, int32(4), atomic.LoadInt32(&fe.calls))
}

func TestRunner_OneFailureDoesNotAbortOthers(t *testing.T) {
	fe := &fakeExecutor{fn: func(_ context.Context, host string, _ Task) Result {
		if host == "b" {
			return Result{Host: host, OK: false, Err: errors.New("boom")}
		}
		return Result{Host: host, OK: true}
	}}
	r := NewRunner(fe, 2)
	results := r.Run(context.Background(), []string{"a", "b", "c"}, Task{Name: "x"})

	byHost := map[string]Result{}
	for _, r := range results {
		byHost[r.Host] = r
	}
	require.True(t, byHost["a"].OK)
	require.False(t, byHost["b"].OK)
	require.True(t, byHost["c"].OK)
}

func TestRunner_CancelPropagates(t *testing.T) {
	block := make(chan struct{})
	fe := &fakeExecutor{fn: func(ctx context.Context, host string, _ Task) Result {
		select {
		case <-block:
		case <-ctx.Done():
		}
		return Result{Host: host, OK: false, Err: ctx.Err()}
	}}
	ctx, cancel := context.WithCancel(context.Background())
	r := NewRunner(fe, 4)

	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	results := r.Run(ctx, []string{"a", "b"}, Task{Name: "x"})
	close(block)
	require.Len(t, results, 2)
}
```

- [ ] **Step 2: Run test, see fail**

Run: `go test ./internal/orchestrator/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement task/result types**

Create `internal/orchestrator/task.go`:
```go
package orchestrator

import (
	"os"
	"time"
)

type FileXfer struct {
	Local  string
	Remote string
	Mode   os.FileMode
}

type InlineFile struct {
	Remote  string
	Content []byte
	Mode    os.FileMode
}

type Task struct {
	Name    string        // human-readable for logs
	Cmd     string        // shell command to execute remotely
	Files   []FileXfer    // large files (tarballs) uploaded via sftp
	Inline  []InlineFile  // small rendered configs written inline
	Timeout time.Duration // 0 => runner default (5 min)
}

type Result struct {
	Host     string
	OK       bool
	Stdout   string
	Stderr   string
	ExitCode int
	Elapsed  time.Duration
	Err      error
}
```

- [ ] **Step 4: Implement runner**

Create `internal/orchestrator/runner.go`:
```go
package orchestrator

import (
	"context"
	"sync"
	"time"
)

type Executor interface {
	Execute(ctx context.Context, host string, task Task) Result
}

type Runner struct {
	exec        Executor
	parallelism int
	defaultTO   time.Duration
}

func NewRunner(exec Executor, parallelism int) *Runner {
	if parallelism <= 0 {
		parallelism = 4
	}
	return &Runner{exec: exec, parallelism: parallelism, defaultTO: 5 * time.Minute}
}

func (r *Runner) Run(ctx context.Context, hosts []string, task Task) []Result {
	if task.Timeout == 0 {
		task.Timeout = r.defaultTO
	}
	out := make([]Result, len(hosts))
	sem := make(chan struct{}, r.parallelism)
	var wg sync.WaitGroup

	for i, h := range hosts {
		i, h := i, h
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			subCtx, cancel := context.WithTimeout(ctx, task.Timeout)
			defer cancel()
			start := time.Now()
			res := r.exec.Execute(subCtx, h, task)
			res.Elapsed = time.Since(start)
			if res.Host == "" {
				res.Host = h
			}
			out[i] = res
		}()
	}
	wg.Wait()
	return out
}
```

- [ ] **Step 5: Add SSH-backed Executor adapter**

Create `internal/orchestrator/ssh_executor.go`:
```go
package orchestrator

import (
	"context"

	"github.com/hadoop-cli/hadoop-cli/internal/ssh"
)

type SSHExecutor struct {
	Pool *ssh.Pool
}

func (s *SSHExecutor) Execute(ctx context.Context, host string, task Task) Result {
	client, err := s.Pool.Get(host)
	if err != nil {
		return Result{Host: host, OK: false, Err: err}
	}

	for _, f := range task.Files {
		if err := client.Upload(ctx, f.Local, f.Remote, f.Mode); err != nil {
			return Result{Host: host, OK: false, Err: err}
		}
	}
	for _, f := range task.Inline {
		if err := client.WriteFile(ctx, f.Remote, f.Content, f.Mode); err != nil {
			return Result{Host: host, OK: false, Err: err}
		}
	}
	if task.Cmd == "" {
		return Result{Host: host, OK: true}
	}
	exec, err := client.Exec(ctx, task.Cmd)
	if err != nil {
		return Result{Host: host, OK: false, Err: err}
	}
	return Result{
		Host:     host,
		OK:       exec.ExitCode == 0,
		Stdout:   exec.Stdout,
		Stderr:   exec.Stderr,
		ExitCode: exec.ExitCode,
	}
}
```

- [ ] **Step 6: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/orchestrator
git commit -m "$(cat <<'EOF'
feat(orchestrator): parallel per-host runner with aggregated Results

Fan-out with bounded parallelism, per-task timeout, isolation (one host's
failure does not abort others). SSH-backed executor wires it to the pool.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: `internal/packages` — tarball download + SHA-512 cache

**Files:**
- Create: `internal/packages/registry.go`
- Create: `internal/packages/cache.go`
- Create: `internal/packages/cache_test.go`

- [ ] **Step 1: Write failing cache test**

Create `internal/packages/cache_test.go`:
```go
package packages

import (
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCache_DownloadsAndVerifiesChecksum(t *testing.T) {
	payload := []byte("fake-tarball-content")
	sum := sha512.Sum512(payload)
	hexSum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()

	dir := t.TempDir()
	c := NewCache(dir)
	spec := Spec{
		Name:     "hbase",
		Version:  "2.5.8",
		URL:      srv.URL + "/hbase-2.5.8-bin.tar.gz",
		Filename: "hbase-2.5.8-bin.tar.gz",
		SHA512:   hexSum,
	}
	path, err := c.Ensure(spec)
	require.NoError(t, err)
	require.FileExists(t, path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, payload, data)

	// second call hits cache, no error
	path2, err := c.Ensure(spec)
	require.NoError(t, err)
	require.Equal(t, path, path2)
}

func TestCache_RejectsChecksumMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("wrong"))
	}))
	defer srv.Close()

	c := NewCache(t.TempDir())
	_, err := c.Ensure(Spec{
		Name: "x", Version: "1", URL: srv.URL,
		Filename: "x.tgz", SHA512: "deadbeef",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "checksum")
}

func TestCache_PathIsDeterministic(t *testing.T) {
	c := NewCache("/tmp/hc")
	require.Equal(t, filepath.Join("/tmp/hc", "hbase-2.5.8-bin.tar.gz"),
		c.PathFor(Spec{Filename: "hbase-2.5.8-bin.tar.gz"}))
}
```

- [ ] **Step 2: Run test, see fail**

Run: `go test ./internal/packages/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement registry**

Create `internal/packages/registry.go`:
```go
package packages

import "fmt"

type Spec struct {
	Name     string // hadoop | zookeeper | hbase
	Version  string
	URL      string
	Filename string // filename on disk and inside remote $install_dir/.cache
	SHA512   string // hex-encoded
}

// Hardcoded checksums for supported versions. Update when adding a version.
// (Values are illustrative; the implementer must copy from Apache KEYS/CHECKSUM.)
var builtinChecksums = map[string]map[string]string{
	"hadoop": {
		"3.3.6": "PUT_REAL_SHA512_HERE_AT_IMPLEMENTATION_TIME",
	},
	"zookeeper": {
		"3.8.4": "PUT_REAL_SHA512_HERE_AT_IMPLEMENTATION_TIME",
	},
	"hbase": {
		"2.5.8": "PUT_REAL_SHA512_HERE_AT_IMPLEMENTATION_TIME",
	},
}

func HadoopSpec(version string) (Spec, error) {
	sum, ok := builtinChecksums["hadoop"][version]
	if !ok {
		return Spec{}, fmt.Errorf("no checksum registered for hadoop %s", version)
	}
	return Spec{
		Name:     "hadoop",
		Version:  version,
		URL:      fmt.Sprintf("https://archive.apache.org/dist/hadoop/common/hadoop-%s/hadoop-%s.tar.gz", version, version),
		Filename: fmt.Sprintf("hadoop-%s.tar.gz", version),
		SHA512:   sum,
	}, nil
}

func ZooKeeperSpec(version string) (Spec, error) {
	sum, ok := builtinChecksums["zookeeper"][version]
	if !ok {
		return Spec{}, fmt.Errorf("no checksum registered for zookeeper %s", version)
	}
	return Spec{
		Name:     "zookeeper",
		Version:  version,
		URL:      fmt.Sprintf("https://archive.apache.org/dist/zookeeper/zookeeper-%s/apache-zookeeper-%s-bin.tar.gz", version, version),
		Filename: fmt.Sprintf("apache-zookeeper-%s-bin.tar.gz", version),
		SHA512:   sum,
	}, nil
}

func HBaseSpec(version string) (Spec, error) {
	sum, ok := builtinChecksums["hbase"][version]
	if !ok {
		return Spec{}, fmt.Errorf("no checksum registered for hbase %s", version)
	}
	return Spec{
		Name:     "hbase",
		Version:  version,
		URL:      fmt.Sprintf("https://archive.apache.org/dist/hbase/%s/hbase-%s-bin.tar.gz", version, version),
		Filename: fmt.Sprintf("hbase-%s-bin.tar.gz", version),
		SHA512:   sum,
	}, nil
}
```

**NOTE to implementer:** The three `PUT_REAL_SHA512_HERE_AT_IMPLEMENTATION_TIME` strings must be replaced with the real SHA-512 values from the Apache mirror's `.sha512` files before the test suite in `cmd/` uses them. Keep the test fixture (`cache_test.go`) independent (it supplies its own checksum), so the build still passes during early development.

- [ ] **Step 4: Implement cache**

Create `internal/packages/cache.go`:
```go
package packages

import (
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/hadoop-cli/hadoop-cli/internal/errs"
)

type Cache struct {
	Dir string
}

func NewCache(dir string) *Cache {
	return &Cache{Dir: dir}
}

func DefaultCacheDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hadoop-cli", "cache")
}

func (c *Cache) PathFor(s Spec) string {
	return filepath.Join(c.Dir, s.Filename)
}

func (c *Cache) Ensure(s Spec) (string, error) {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return "", err
	}
	path := c.PathFor(s)
	if ok, _ := verify(path, s.SHA512); ok {
		return path, nil
	}

	tmp := path + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	resp, err := http.Get(s.URL)
	if err != nil {
		f.Close()
		os.Remove(tmp)
		return "", errs.Wrap(errs.CodeDownloadFailed, "", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		f.Close()
		os.Remove(tmp)
		return "", errs.New(errs.CodeDownloadFailed, "", fmt.Sprintf("HTTP %d from %s", resp.StatusCode, s.URL))
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", errs.Wrap(errs.CodeDownloadFailed, "", err)
	}
	f.Close()

	if ok, err := verify(tmp, s.SHA512); !ok {
		os.Remove(tmp)
		return "", errs.New(errs.CodeDownloadChecksumMismatch, "",
			fmt.Sprintf("SHA-512 mismatch for %s: %v", s.Filename, err))
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return path, nil
}

func verify(path, expectedHex string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expectedHex {
		return false, fmt.Errorf("expected %s, got %s", expectedHex, got)
	}
	return true, nil
}
```

- [ ] **Step 5: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/packages
git commit -m "$(cat <<'EOF'
feat(packages): local tarball cache with SHA-512 verification

Downloads into ~/.hadoop-cli/cache with atomic rename; rejects corrupted
mirrors via hardcoded per-version checksums.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: `internal/render` — config template engine

**Files:**
- Create: `internal/render/render.go`
- Create: `internal/render/render_test.go`
- Create: `internal/render/templates/` (filled by component tasks later)

- [ ] **Step 1: Write failing test**

Create `internal/render/render_test.go`:
```go
package render

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestXMLSite_RendersKeyValues(t *testing.T) {
	props := []Property{
		{Name: "fs.defaultFS", Value: "hdfs://node1:8020"},
		{Name: "dfs.replication", Value: "2"},
	}
	out, err := XMLSite(props)
	require.NoError(t, err)
	require.Contains(t, out, `<name>fs.defaultFS</name>`)
	require.Contains(t, out, `<value>hdfs://node1:8020</value>`)
	require.Contains(t, out, `<name>dfs.replication</name>`)
	require.Contains(t, out, `<value>2</value>`)
}

func TestXMLSite_EscapesSpecialChars(t *testing.T) {
	out, err := XMLSite([]Property{{Name: "k", Value: "<&>"}})
	require.NoError(t, err)
	require.Contains(t, out, "&lt;&amp;&gt;")
}

func TestRenderText_SubstitutesVars(t *testing.T) {
	out, err := RenderText("hello {{.Name}}", map[string]any{"Name": "world"})
	require.NoError(t, err)
	require.Equal(t, "hello world", out)
}
```

- [ ] **Step 2: Run test, see fail**

Run: `go test ./internal/render/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

Create `internal/render/render.go`:
```go
package render

import (
	"bytes"
	"encoding/xml"
	"text/template"

	"github.com/hadoop-cli/hadoop-cli/internal/errs"
)

type Property struct {
	Name  string `xml:"name"`
	Value string `xml:"value"`
}

type configuration struct {
	XMLName    xml.Name   `xml:"configuration"`
	Properties []Property `xml:"property"`
}

// XMLSite renders Hadoop-style *-site.xml files.
func XMLSite(props []Property) (string, error) {
	cfg := configuration{Properties: props}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return "", errs.Wrap(errs.CodeConfigRenderFailed, "", err)
	}
	buf.WriteString("\n")
	return buf.String(), nil
}

// RenderText renders a Go text/template against data.
func RenderText(tpl string, data any) (string, error) {
	t, err := template.New("t").Parse(tpl)
	if err != nil {
		return "", errs.Wrap(errs.CodeConfigRenderFailed, "", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", errs.Wrap(errs.CodeConfigRenderFailed, "", err)
	}
	return buf.String(), nil
}
```

- [ ] **Step 4: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/render
git commit -m "$(cat <<'EOF'
feat(render): XML site-file and text-template helpers for component configs

Wraps encoding/xml with the Hadoop configuration envelope and exposes a
text-template escape hatch for env.sh / workers / myid files.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: `internal/runlog` — per-run record directory

**Files:**
- Create: `internal/runlog/runlog.go`
- Create: `internal/runlog/runlog_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/runlog/runlog_test.go`:
```go
package runlog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew_CreatesRunDirAndID(t *testing.T) {
	root := t.TempDir()
	r, err := New(root, "install")
	require.NoError(t, err)
	require.NotEmpty(t, r.ID)
	require.DirExists(t, r.Dir)
	require.True(t, filepath.IsAbs(r.Dir))
}

func TestWriteFile_StoresUnderRun(t *testing.T) {
	r, err := New(t.TempDir(), "install")
	require.NoError(t, err)
	require.NoError(t, r.WriteFile("hosts/node1.stdout", []byte("ok")))
	b, err := os.ReadFile(filepath.Join(r.Dir, "hosts/node1.stdout"))
	require.NoError(t, err)
	require.Equal(t, "ok", string(b))
}

func TestSaveResult_WritesJSON(t *testing.T) {
	r, err := New(t.TempDir(), "install")
	require.NoError(t, err)
	require.NoError(t, r.SaveResult(map[string]any{"ok": true, "hosts": 3}))
	b, err := os.ReadFile(filepath.Join(r.Dir, "result.json"))
	require.NoError(t, err)
	require.Contains(t, string(b), `"ok": true`)
}
```

- [ ] **Step 2: Run test, see fail**

Run: `go test ./internal/runlog/...`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement**

Create `internal/runlog/runlog.go`:
```go
package runlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Run struct {
	ID  string
	Dir string
}

func DefaultRoot() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hadoop-cli", "runs")
}

func New(root, command string) (*Run, error) {
	id := fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102-150405"), command)
	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Run{ID: id, Dir: dir}, nil
}

func (r *Run) WriteFile(rel string, data []byte) error {
	full := filepath.Join(r.Dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, data, 0o644)
}

func (r *Run) SaveResult(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return r.WriteFile("result.json", b)
}
```

- [ ] **Step 4: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/runlog
git commit -m "$(cat <<'EOF'
feat(runlog): per-run directory under ~/.hadoop-cli/runs

Every command gets a timestamped run-id; per-host stdout/stderr, rendered
configs, and aggregated result.json land here for post-mortem.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: `internal/components` — shared Component interface

**Files:**
- Create: `internal/components/component.go`
- Create: `internal/components/component_test.go`

- [ ] **Step 1: Write failing interface test**

Create `internal/components/component_test.go`:
```go
package components

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNames_AreStableOrdered(t *testing.T) {
	require.Equal(t, []string{"zookeeper", "hdfs", "hbase"}, Ordered())
	require.Equal(t, []string{"hbase", "hdfs", "zookeeper"}, ReverseOrdered())
}
```

- [ ] **Step 2: Run test, see fail; then implement**

Run: `go test ./internal/components/...` → FAIL.

Create `internal/components/component.go`:
```go
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
```

- [ ] **Step 3: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/components
git commit -m "$(cat <<'EOF'
feat(components): introduce shared Component interface and ordering

Each component (zookeeper/hdfs/hbase) implements the same lifecycle methods;
cmd wiring iterates the ordered list for start/install and the reverse for
stop/uninstall.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: `internal/components/zookeeper`

**Files:**
- Create: `internal/components/zookeeper/zookeeper.go`
- Create: `internal/components/zookeeper/config.go`
- Create: `internal/components/zookeeper/config_test.go`

- [ ] **Step 1: Write failing config test**

Create `internal/components/zookeeper/config_test.go`:
```go
package zookeeper

import (
	"strings"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/stretchr/testify/require"
)

func fixture() *inventory.Inventory {
	return &inventory.Inventory{
		Cluster:  inventory.Cluster{InstallDir: "/opt/hadoop-cli", DataDir: "/data/hadoop-cli", JavaHome: "/j"},
		Versions: inventory.Versions{ZooKeeper: "3.8.4"},
		Hosts: []inventory.Host{
			{Name: "n1", Address: "10.0.0.1"},
			{Name: "n2", Address: "10.0.0.2"},
			{Name: "n3", Address: "10.0.0.3"},
		},
		Roles: inventory.Roles{ZooKeeper: []string{"n1", "n2", "n3"}},
		Overrides: inventory.Overrides{ZooKeeper: inventory.ZKOverrides{
			ClientPort: 2181, TickTime: 2000, InitLimit: 10, SyncLimit: 5,
		}},
	}
}

func TestRenderZooCfg_ContainsServerLinesAndDataDir(t *testing.T) {
	cfg, err := RenderZooCfg(fixture())
	require.NoError(t, err)
	require.Contains(t, cfg, "dataDir=/data/hadoop-cli/zookeeper")
	require.Contains(t, cfg, "clientPort=2181")
	require.Contains(t, cfg, "server.1=10.0.0.1:2888:3888")
	require.Contains(t, cfg, "server.2=10.0.0.2:2888:3888")
	require.Contains(t, cfg, "server.3=10.0.0.3:2888:3888")
}

func TestMyIDFor_IsOrdinalStartingAt1(t *testing.T) {
	inv := fixture()
	require.Equal(t, 1, MyIDFor(inv, "n1"))
	require.Equal(t, 2, MyIDFor(inv, "n2"))
	require.Equal(t, 3, MyIDFor(inv, "n3"))
	require.Equal(t, 0, MyIDFor(inv, "missing"))
}

func TestRenderEnv_SetsJAVAHOMEAndHeap(t *testing.T) {
	env, err := RenderEnv(fixture())
	require.NoError(t, err)
	require.Contains(t, env, "JAVA_HOME=/j")
	require.True(t, strings.Contains(env, "ZK_SERVER_HEAP") || strings.Contains(env, "SERVER_JVMFLAGS"))
}
```

- [ ] **Step 2: Run test, see fail**

Run: `go test ./internal/components/zookeeper/...`
Expected: FAIL.

- [ ] **Step 3: Implement config rendering**

Create `internal/components/zookeeper/config.go`:
```go
package zookeeper

import (
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
)

// MyIDFor returns the 1-based ordinal of `host` inside roles.zookeeper, or 0 if missing.
func MyIDFor(inv *inventory.Inventory, host string) int {
	for i, h := range inv.Roles.ZooKeeper {
		if h == host {
			return i + 1
		}
	}
	return 0
}

func DataDir(inv *inventory.Inventory) string {
	return inv.Cluster.DataDir + "/zookeeper"
}

func LogDir(inv *inventory.Inventory) string {
	return inv.Cluster.DataDir + "/zookeeper/logs"
}

func Home(inv *inventory.Inventory) string {
	return fmt.Sprintf("%s/zookeeper", inv.Cluster.InstallDir)
}

func RenderZooCfg(inv *inventory.Inventory) (string, error) {
	zk := inv.Overrides.ZooKeeper
	var b strings.Builder
	fmt.Fprintf(&b, "tickTime=%d\n", zk.TickTime)
	fmt.Fprintf(&b, "initLimit=%d\n", zk.InitLimit)
	fmt.Fprintf(&b, "syncLimit=%d\n", zk.SyncLimit)
	fmt.Fprintf(&b, "dataDir=%s\n", DataDir(inv))
	fmt.Fprintf(&b, "dataLogDir=%s\n", LogDir(inv))
	fmt.Fprintf(&b, "clientPort=%d\n", zk.ClientPort)
	fmt.Fprintf(&b, "4lw.commands.whitelist=*\n")
	fmt.Fprintf(&b, "admin.enableServer=false\n")
	for i, hostName := range inv.Roles.ZooKeeper {
		h, ok := inv.HostByName(hostName)
		if !ok {
			return "", fmt.Errorf("unknown zookeeper host %q", hostName)
		}
		fmt.Fprintf(&b, "server.%d=%s:2888:3888\n", i+1, h.Address)
	}
	return b.String(), nil
}

func RenderEnv(inv *inventory.Inventory) (string, error) {
	return fmt.Sprintf(`export JAVA_HOME=%s
export ZOOCFGDIR=%s/conf
export ZOO_LOG_DIR=%s
export ZK_SERVER_HEAP=%s
export SERVER_JVMFLAGS="-Xmx%sm"
`, inv.Cluster.JavaHome, Home(inv), LogDir(inv), "512", "512"), nil
}
```

- [ ] **Step 4: Implement component lifecycle**

Create `internal/components/zookeeper/zookeeper.go`:
```go
package zookeeper

import (
	"context"
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/components"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/hadoop-cli/hadoop-cli/internal/packages"
)

type ZooKeeper struct{}

func (ZooKeeper) Name() string { return "zookeeper" }

func (ZooKeeper) Hosts(inv *inventory.Inventory) []string { return inv.Roles.ZooKeeper }

func (ZooKeeper) Install(ctx context.Context, e components.Env) []orchestrator.Result {
	spec, err := packages.ZooKeeperSpec(e.Inv.Versions.ZooKeeper)
	if err != nil {
		return failAll(e.Inv.Roles.ZooKeeper, err)
	}
	cache := packages.NewCache(e.Cache)
	local, err := cache.Ensure(spec)
	if err != nil {
		return failAll(e.Inv.Roles.ZooKeeper, err)
	}
	remoteTarball := fmt.Sprintf("%s/.cache/%s", e.Inv.Cluster.InstallDir, spec.Filename)
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
mkdir -p %s %s %s
if [ -x %s/bin/zkServer.sh ]; then exit 0; fi
tar -xzf %s -C %s
mv %s/apache-zookeeper-%s-bin/* %s/
rmdir %s/apache-zookeeper-%s-bin
`,
		home, DataDir(e.Inv), LogDir(e.Inv),
		home,
		remoteTarball, e.Inv.Cluster.InstallDir,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.ZooKeeper, home,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.ZooKeeper,
	)

	task := orchestrator.Task{
		Name:  "zk-install",
		Cmd:   script,
		Files: []orchestrator.FileXfer{{Local: local, Remote: remoteTarball, Mode: 0o644}},
	}
	return e.Runner.Run(ctx, e.Inv.Roles.ZooKeeper, task)
}

func (ZooKeeper) Configure(ctx context.Context, e components.Env) []orchestrator.Result {
	zoo, err := RenderZooCfg(e.Inv)
	if err != nil {
		return failAll(e.Inv.Roles.ZooKeeper, err)
	}
	envSh, err := RenderEnv(e.Inv)
	if err != nil {
		return failAll(e.Inv.Roles.ZooKeeper, err)
	}
	home := Home(e.Inv)
	results := make([]orchestrator.Result, 0, len(e.Inv.Roles.ZooKeeper))
	// per-host because myid differs
	for _, host := range e.Inv.Roles.ZooKeeper {
		id := MyIDFor(e.Inv, host)
		task := orchestrator.Task{
			Name: "zk-configure",
			Cmd: fmt.Sprintf(`set -e
mkdir -p %s/conf %s
`, home, DataDir(e.Inv)),
			Inline: []orchestrator.InlineFile{
				{Remote: home + "/conf/zoo.cfg", Content: []byte(zoo), Mode: 0o644},
				{Remote: home + "/conf/zookeeper-env.sh", Content: []byte(envSh), Mode: 0o644},
				{Remote: DataDir(e.Inv) + "/myid", Content: []byte(fmt.Sprintf("%d\n", id)), Mode: 0o644},
			},
		}
		results = append(results, e.Runner.Run(ctx, []string{host}, task)...)
	}
	return results
}

func (ZooKeeper) Start(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
if %s/bin/zkServer.sh status >/dev/null 2>&1; then exit 0; fi
%s/bin/zkServer.sh start
`, e.Inv.Cluster.JavaHome, home, home)
	return e.Runner.Run(ctx, e.Inv.Roles.ZooKeeper, orchestrator.Task{Name: "zk-start", Cmd: script})
}

func (ZooKeeper) Stop(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/zkServer.sh stop || true
`, e.Inv.Cluster.JavaHome, home)
	return e.Runner.Run(ctx, e.Inv.Roles.ZooKeeper, orchestrator.Task{Name: "zk-stop", Cmd: script})
}

func (ZooKeeper) Status(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/zkServer.sh status
`, e.Inv.Cluster.JavaHome, home)
	return e.Runner.Run(ctx, e.Inv.Roles.ZooKeeper, orchestrator.Task{Name: "zk-status", Cmd: script})
}

func (ZooKeeper) Uninstall(ctx context.Context, e components.Env, purgeData bool) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/zkServer.sh stop || true
rm -rf %s
`, e.Inv.Cluster.JavaHome, home, home)
	if purgeData {
		script += fmt.Sprintf("rm -rf %s\n", DataDir(e.Inv))
	}
	return e.Runner.Run(ctx, e.Inv.Roles.ZooKeeper, orchestrator.Task{Name: "zk-uninstall", Cmd: script})
}

func failAll(hosts []string, err error) []orchestrator.Result {
	out := make([]orchestrator.Result, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, orchestrator.Result{Host: h, OK: false, Err: err})
	}
	return out
}
```

- [ ] **Step 5: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/components/zookeeper
git commit -m "$(cat <<'EOF'
feat(components/zookeeper): install/configure/start/stop/status/uninstall

Renders zoo.cfg (with server.N lines) and per-host myid, uploads the tarball
into $install_dir/.cache, and runs zkServer.sh under the configured JAVA_HOME.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: `internal/components/hdfs`

**Files:**
- Create: `internal/components/hdfs/hdfs.go`
- Create: `internal/components/hdfs/config.go`
- Create: `internal/components/hdfs/config_test.go`

- [ ] **Step 1: Write failing config test**

Create `internal/components/hdfs/config_test.go`:
```go
package hdfs

import (
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/stretchr/testify/require"
)

func fixture() *inventory.Inventory {
	return &inventory.Inventory{
		Cluster:  inventory.Cluster{InstallDir: "/opt/hadoop-cli", DataDir: "/data/hadoop-cli", JavaHome: "/j", User: "hadoop"},
		Versions: inventory.Versions{Hadoop: "3.3.6"},
		Hosts: []inventory.Host{
			{Name: "n1", Address: "10.0.0.1"},
			{Name: "n2", Address: "10.0.0.2"},
		},
		Roles: inventory.Roles{NameNode: []string{"n1"}, DataNode: []string{"n1", "n2"}},
		Overrides: inventory.Overrides{HDFS: inventory.HDFSOverrides{
			Replication: 2, NameNodeHeap: "1g", DataNodeHeap: "1g",
			NameNodeRPC: 8020, NameNodeHTTP: 9870,
		}},
	}
}

func TestRenderCoreSite_UsesNameNodeAddress(t *testing.T) {
	s, err := RenderCoreSite(fixture())
	require.NoError(t, err)
	require.Contains(t, s, "<name>fs.defaultFS</name>")
	require.Contains(t, s, "<value>hdfs://10.0.0.1:8020</value>")
}

func TestRenderHDFSSite_SetsReplicationAndDirs(t *testing.T) {
	s, err := RenderHDFSSite(fixture())
	require.NoError(t, err)
	require.Contains(t, s, "<name>dfs.replication</name>")
	require.Contains(t, s, "<value>2</value>")
	require.Contains(t, s, "/data/hadoop-cli/hdfs/nn")
	require.Contains(t, s, "/data/hadoop-cli/hdfs/dn")
}

func TestRenderWorkers_ListsDataNodes(t *testing.T) {
	s := RenderWorkers(fixture())
	require.Equal(t, "10.0.0.1\n10.0.0.2\n", s)
}
```

- [ ] **Step 2: Run test, see fail**

Run: `go test ./internal/components/hdfs/...`
Expected: FAIL.

- [ ] **Step 3: Implement config**

Create `internal/components/hdfs/config.go`:
```go
package hdfs

import (
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/render"
)

func Home(inv *inventory.Inventory) string {
	return inv.Cluster.InstallDir + "/hadoop"
}

func NNDir(inv *inventory.Inventory) string { return inv.Cluster.DataDir + "/hdfs/nn" }
func DNDir(inv *inventory.Inventory) string { return inv.Cluster.DataDir + "/hdfs/dn" }
func LogDir(inv *inventory.Inventory) string { return inv.Cluster.DataDir + "/hdfs/logs" }

func nameNodeAddress(inv *inventory.Inventory) (string, error) {
	if len(inv.Roles.NameNode) != 1 {
		return "", fmt.Errorf("expected exactly 1 namenode, got %d", len(inv.Roles.NameNode))
	}
	h, ok := inv.HostByName(inv.Roles.NameNode[0])
	if !ok {
		return "", fmt.Errorf("namenode host %q not in hosts list", inv.Roles.NameNode[0])
	}
	return h.Address, nil
}

func RenderCoreSite(inv *inventory.Inventory) (string, error) {
	nn, err := nameNodeAddress(inv)
	if err != nil {
		return "", err
	}
	return render.XMLSite([]render.Property{
		{Name: "fs.defaultFS", Value: fmt.Sprintf("hdfs://%s:%d", nn, inv.Overrides.HDFS.NameNodeRPC)},
		{Name: "hadoop.tmp.dir", Value: inv.Cluster.DataDir + "/hdfs/tmp"},
	})
}

func RenderHDFSSite(inv *inventory.Inventory) (string, error) {
	return render.XMLSite([]render.Property{
		{Name: "dfs.replication", Value: fmt.Sprintf("%d", inv.Overrides.HDFS.Replication)},
		{Name: "dfs.namenode.name.dir", Value: "file://" + NNDir(inv)},
		{Name: "dfs.datanode.data.dir", Value: "file://" + DNDir(inv)},
		{Name: "dfs.namenode.http-address", Value: fmt.Sprintf("0.0.0.0:%d", inv.Overrides.HDFS.NameNodeHTTP)},
		{Name: "dfs.permissions.enabled", Value: "false"},
	})
}

func RenderWorkers(inv *inventory.Inventory) string {
	var b strings.Builder
	for _, name := range inv.Roles.DataNode {
		if h, ok := inv.HostByName(name); ok {
			b.WriteString(h.Address)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func RenderHadoopEnv(inv *inventory.Inventory) string {
	return fmt.Sprintf(`export JAVA_HOME=%s
export HADOOP_LOG_DIR=%s
export HDFS_NAMENODE_OPTS="-Xmx%s"
export HDFS_DATANODE_OPTS="-Xmx%s"
`, inv.Cluster.JavaHome, LogDir(inv), inv.Overrides.HDFS.NameNodeHeap, inv.Overrides.HDFS.DataNodeHeap)
}
```

- [ ] **Step 4: Implement component lifecycle**

Create `internal/components/hdfs/hdfs.go`:
```go
package hdfs

import (
	"context"
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/components"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/hadoop-cli/hadoop-cli/internal/packages"
)

type HDFS struct{ ForceFormat bool }

func (HDFS) Name() string { return "hdfs" }

func (HDFS) Hosts(inv *inventory.Inventory) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, h := range append(append([]string{}, inv.Roles.NameNode...), inv.Roles.DataNode...) {
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			out = append(out, h)
		}
	}
	return out
}

func (c HDFS) allHosts(inv *inventory.Inventory) []string { return c.Hosts(inv) }

func (c HDFS) Install(ctx context.Context, e components.Env) []orchestrator.Result {
	spec, err := packages.HadoopSpec(e.Inv.Versions.Hadoop)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	cache := packages.NewCache(e.Cache)
	local, err := cache.Ensure(spec)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	remoteTarball := fmt.Sprintf("%s/.cache/%s", e.Inv.Cluster.InstallDir, spec.Filename)
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
mkdir -p %s %s %s %s
if [ -x %s/bin/hdfs ]; then exit 0; fi
tar -xzf %s -C %s
mv %s/hadoop-%s/* %s/
rmdir %s/hadoop-%s
`,
		home, NNDir(e.Inv), DNDir(e.Inv), LogDir(e.Inv),
		home,
		remoteTarball, e.Inv.Cluster.InstallDir,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.Hadoop, home,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.Hadoop,
	)
	task := orchestrator.Task{
		Name:  "hdfs-install",
		Cmd:   script,
		Files: []orchestrator.FileXfer{{Local: local, Remote: remoteTarball, Mode: 0o644}},
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), task)
}

func (c HDFS) Configure(ctx context.Context, e components.Env) []orchestrator.Result {
	coreSite, err := RenderCoreSite(e.Inv)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	hdfsSite, err := RenderHDFSSite(e.Inv)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	workers := RenderWorkers(e.Inv)
	envSh := RenderHadoopEnv(e.Inv)
	home := Home(e.Inv)

	inline := []orchestrator.InlineFile{
		{Remote: home + "/etc/hadoop/core-site.xml", Content: []byte(coreSite), Mode: 0o644},
		{Remote: home + "/etc/hadoop/hdfs-site.xml", Content: []byte(hdfsSite), Mode: 0o644},
		{Remote: home + "/etc/hadoop/workers", Content: []byte(workers), Mode: 0o644},
		{Remote: home + "/etc/hadoop/hadoop-env.sh", Content: []byte(envSh), Mode: 0o755},
	}
	task := orchestrator.Task{
		Name:   "hdfs-configure",
		Cmd:    fmt.Sprintf("mkdir -p %s/etc/hadoop", home),
		Inline: inline,
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), task)
}

func (c HDFS) Start(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	nnMarker := NNDir(e.Inv) + "/.formatted"
	forceFlag := ""
	if c.ForceFormat {
		forceFlag = " -force"
	}
	// NameNode: format if needed, then start
	nnScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
export HADOOP_CONF_DIR=%s/etc/hadoop
export HADOOP_LOG_DIR=%s
if [ ! -f %s ] || [ "%s" = " -force" ]; then
  %s/bin/hdfs namenode -format -nonInteractive%s cluster 2>&1 || true
  mkdir -p %s
  touch %s
fi
if ! jps -lm | grep -q NameNode; then
  %s/bin/hdfs --daemon start namenode
fi
`,
		e.Inv.Cluster.JavaHome, home, LogDir(e.Inv),
		nnMarker, forceFlag,
		home, forceFlag,
		NNDir(e.Inv), nnMarker,
		home,
	)
	nnResults := e.Runner.Run(ctx, e.Inv.Roles.NameNode, orchestrator.Task{Name: "hdfs-nn-start", Cmd: nnScript})

	dnScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
export HADOOP_CONF_DIR=%s/etc/hadoop
export HADOOP_LOG_DIR=%s
if ! jps -lm | grep -q DataNode; then
  %s/bin/hdfs --daemon start datanode
fi
`, e.Inv.Cluster.JavaHome, home, LogDir(e.Inv), home)
	dnResults := e.Runner.Run(ctx, e.Inv.Roles.DataNode, orchestrator.Task{Name: "hdfs-dn-start", Cmd: dnScript})

	return append(nnResults, dnResults...)
}

func (c HDFS) Stop(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	dnScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hdfs --daemon stop datanode || true
`, e.Inv.Cluster.JavaHome, home)
	dnResults := e.Runner.Run(ctx, e.Inv.Roles.DataNode, orchestrator.Task{Name: "hdfs-dn-stop", Cmd: dnScript})

	nnScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hdfs --daemon stop namenode || true
`, e.Inv.Cluster.JavaHome, home)
	nnResults := e.Runner.Run(ctx, e.Inv.Roles.NameNode, orchestrator.Task{Name: "hdfs-nn-stop", Cmd: nnScript})

	return append(dnResults, nnResults...)
}

func (c HDFS) Status(ctx context.Context, e components.Env) []orchestrator.Result {
	script := `jps -lm | grep -E 'NameNode|DataNode' || true`
	return e.Runner.Run(ctx, c.allHosts(e.Inv), orchestrator.Task{Name: "hdfs-status", Cmd: script})
}

func (c HDFS) Uninstall(ctx context.Context, e components.Env, purgeData bool) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hdfs --daemon stop datanode || true
%s/bin/hdfs --daemon stop namenode || true
rm -rf %s
`, e.Inv.Cluster.JavaHome, home, home, home)
	if purgeData {
		script += fmt.Sprintf("rm -rf %s %s %s\n", NNDir(e.Inv), DNDir(e.Inv), LogDir(e.Inv))
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), orchestrator.Task{Name: "hdfs-uninstall", Cmd: script})
}

func failAll(hosts []string, err error) []orchestrator.Result {
	out := make([]orchestrator.Result, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, orchestrator.Result{Host: h, OK: false, Err: err})
	}
	return out
}
```

- [ ] **Step 5: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/components/hdfs
git commit -m "$(cat <<'EOF'
feat(components/hdfs): install/configure/start/stop/status/uninstall, single NN

First start auto-formats the NameNode behind a .formatted marker; --force-format
opt-in re-runs the format. DataNodes start concurrently via orchestrator.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: `internal/components/hbase`

**Files:**
- Create: `internal/components/hbase/hbase.go`
- Create: `internal/components/hbase/config.go`
- Create: `internal/components/hbase/config_test.go`

- [ ] **Step 1: Write failing config test**

Create `internal/components/hbase/config_test.go`:
```go
package hbase

import (
	"strings"
	"testing"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/stretchr/testify/require"
)

func fixture() *inventory.Inventory {
	return &inventory.Inventory{
		Cluster:  inventory.Cluster{InstallDir: "/opt/hadoop-cli", DataDir: "/data/hadoop-cli", JavaHome: "/j"},
		Versions: inventory.Versions{HBase: "2.5.8"},
		Hosts: []inventory.Host{
			{Name: "n1", Address: "10.0.0.1"},
			{Name: "n2", Address: "10.0.0.2"},
			{Name: "n3", Address: "10.0.0.3"},
		},
		Roles: inventory.Roles{
			NameNode:     []string{"n1"},
			ZooKeeper:    []string{"n1", "n2", "n3"},
			HBaseMaster:  []string{"n1"},
			RegionServer: []string{"n1", "n2", "n3"},
		},
		Overrides: inventory.Overrides{
			HDFS:      inventory.HDFSOverrides{NameNodeRPC: 8020},
			ZooKeeper: inventory.ZKOverrides{ClientPort: 2181},
			HBase:     inventory.HBaseOverrides{MasterHeap: "1g", RegionServerHeap: "1g"},
		},
	}
}

func TestRenderHBaseSite_UsesDerivedRootDirAndZKQuorum(t *testing.T) {
	s, err := RenderHBaseSite(fixture())
	require.NoError(t, err)
	require.Contains(t, s, "<name>hbase.rootdir</name>")
	require.Contains(t, s, "<value>hdfs://10.0.0.1:8020/hbase</value>")
	require.Contains(t, s, "<name>hbase.zookeeper.quorum</name>")
	require.Contains(t, s, "<value>10.0.0.1,10.0.0.2,10.0.0.3</value>")
	require.Contains(t, s, "<name>hbase.cluster.distributed</name>")
	require.Contains(t, s, "<value>true</value>")
}

func TestRenderHBaseSite_HonorsExplicitRootDir(t *testing.T) {
	inv := fixture()
	inv.Overrides.HBase.RootDir = "hdfs://custom:9000/h"
	s, err := RenderHBaseSite(inv)
	require.NoError(t, err)
	require.Contains(t, s, "<value>hdfs://custom:9000/h</value>")
}

func TestRenderRegionServers_HasOneLinePerHost(t *testing.T) {
	s := RenderRegionServers(fixture())
	require.Equal(t, 3, strings.Count(s, "\n"))
}
```

- [ ] **Step 2: Run test, see fail; implement config**

Run: `go test ./internal/components/hbase/...` → FAIL.

Create `internal/components/hbase/config.go`:
```go
package hbase

import (
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/render"
)

func Home(inv *inventory.Inventory) string { return inv.Cluster.InstallDir + "/hbase" }
func LogDir(inv *inventory.Inventory) string { return inv.Cluster.DataDir + "/hbase/logs" }
func PidDir(inv *inventory.Inventory) string { return inv.Cluster.DataDir + "/hbase/pids" }
func ZKDataDir(inv *inventory.Inventory) string { return inv.Cluster.DataDir + "/hbase/zookeeper" }

func rootDir(inv *inventory.Inventory) (string, error) {
	if inv.Overrides.HBase.RootDir != "" {
		return inv.Overrides.HBase.RootDir, nil
	}
	if len(inv.Roles.NameNode) != 1 {
		return "", fmt.Errorf("cannot derive hbase.rootdir without single namenode")
	}
	h, ok := inv.HostByName(inv.Roles.NameNode[0])
	if !ok {
		return "", fmt.Errorf("namenode host not found")
	}
	return fmt.Sprintf("hdfs://%s:%d/hbase", h.Address, inv.Overrides.HDFS.NameNodeRPC), nil
}

func zkQuorum(inv *inventory.Inventory) string {
	addrs := make([]string, 0, len(inv.Roles.ZooKeeper))
	for _, name := range inv.Roles.ZooKeeper {
		if h, ok := inv.HostByName(name); ok {
			addrs = append(addrs, h.Address)
		}
	}
	return strings.Join(addrs, ",")
}

func RenderHBaseSite(inv *inventory.Inventory) (string, error) {
	root, err := rootDir(inv)
	if err != nil {
		return "", err
	}
	return render.XMLSite([]render.Property{
		{Name: "hbase.rootdir", Value: root},
		{Name: "hbase.cluster.distributed", Value: "true"},
		{Name: "hbase.zookeeper.quorum", Value: zkQuorum(inv)},
		{Name: "hbase.zookeeper.property.clientPort", Value: fmt.Sprintf("%d", inv.Overrides.ZooKeeper.ClientPort)},
		{Name: "hbase.unsafe.stream.capability.enforce", Value: "false"},
		{Name: "hbase.master.port", Value: fmt.Sprintf("%d", inv.Overrides.HBase.MasterPort)},
		{Name: "hbase.master.info.port", Value: fmt.Sprintf("%d", inv.Overrides.HBase.MasterInfoPort)},
		{Name: "hbase.regionserver.port", Value: fmt.Sprintf("%d", inv.Overrides.HBase.RSPort)},
		{Name: "hbase.regionserver.info.port", Value: fmt.Sprintf("%d", inv.Overrides.HBase.RSInfoPort)},
	})
}

func RenderRegionServers(inv *inventory.Inventory) string {
	var b strings.Builder
	for _, name := range inv.Roles.RegionServer {
		if h, ok := inv.HostByName(name); ok {
			b.WriteString(h.Address)
			b.WriteString("\n")
		}
	}
	return b.String()
}

func RenderBackupMasters(_ *inventory.Inventory) string { return "" }

func RenderHBaseEnv(inv *inventory.Inventory) string {
	return fmt.Sprintf(`export JAVA_HOME=%s
export HBASE_LOG_DIR=%s
export HBASE_PID_DIR=%s
export HBASE_MANAGES_ZK=false
export HBASE_HEAPSIZE=%s
export HBASE_MASTER_OPTS="-Xmx%s"
export HBASE_REGIONSERVER_OPTS="-Xmx%s"
`, inv.Cluster.JavaHome, LogDir(inv), PidDir(inv),
		inv.Overrides.HBase.MasterHeap,
		inv.Overrides.HBase.MasterHeap,
		inv.Overrides.HBase.RegionServerHeap,
	)
}
```

- [ ] **Step 3: Implement component lifecycle**

Create `internal/components/hbase/hbase.go`:
```go
package hbase

import (
	"context"
	"fmt"

	"github.com/hadoop-cli/hadoop-cli/internal/components"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
	"github.com/hadoop-cli/hadoop-cli/internal/packages"
)

type HBase struct{}

func (HBase) Name() string { return "hbase" }

func (HBase) Hosts(inv *inventory.Inventory) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, h := range append(append([]string{}, inv.Roles.HBaseMaster...), inv.Roles.RegionServer...) {
		if _, ok := seen[h]; !ok {
			seen[h] = struct{}{}
			out = append(out, h)
		}
	}
	return out
}

func (c HBase) allHosts(inv *inventory.Inventory) []string { return c.Hosts(inv) }

func (c HBase) Install(ctx context.Context, e components.Env) []orchestrator.Result {
	spec, err := packages.HBaseSpec(e.Inv.Versions.HBase)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	cache := packages.NewCache(e.Cache)
	local, err := cache.Ensure(spec)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	remoteTarball := fmt.Sprintf("%s/.cache/%s", e.Inv.Cluster.InstallDir, spec.Filename)
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
mkdir -p %s %s %s
if [ -x %s/bin/hbase ]; then exit 0; fi
tar -xzf %s -C %s
mv %s/hbase-%s/* %s/
rmdir %s/hbase-%s
`,
		home, LogDir(e.Inv), PidDir(e.Inv),
		home,
		remoteTarball, e.Inv.Cluster.InstallDir,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.HBase, home,
		e.Inv.Cluster.InstallDir, e.Inv.Versions.HBase,
	)
	task := orchestrator.Task{
		Name:  "hbase-install",
		Cmd:   script,
		Files: []orchestrator.FileXfer{{Local: local, Remote: remoteTarball, Mode: 0o644}},
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), task)
}

func (c HBase) Configure(ctx context.Context, e components.Env) []orchestrator.Result {
	site, err := RenderHBaseSite(e.Inv)
	if err != nil {
		return failAll(c.allHosts(e.Inv), err)
	}
	rs := RenderRegionServers(e.Inv)
	bm := RenderBackupMasters(e.Inv)
	envSh := RenderHBaseEnv(e.Inv)
	home := Home(e.Inv)

	task := orchestrator.Task{
		Name: "hbase-configure",
		Cmd:  fmt.Sprintf("mkdir -p %s/conf", home),
		Inline: []orchestrator.InlineFile{
			{Remote: home + "/conf/hbase-site.xml", Content: []byte(site), Mode: 0o644},
			{Remote: home + "/conf/regionservers", Content: []byte(rs), Mode: 0o644},
			{Remote: home + "/conf/backup-masters", Content: []byte(bm), Mode: 0o644},
			{Remote: home + "/conf/hbase-env.sh", Content: []byte(envSh), Mode: 0o755},
		},
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), task)
}

func (c HBase) Start(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	masterScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
if ! jps -lm | grep -q HMaster; then
  %s/bin/hbase-daemon.sh start master
fi
`, e.Inv.Cluster.JavaHome, home)
	masterResults := e.Runner.Run(ctx, e.Inv.Roles.HBaseMaster, orchestrator.Task{Name: "hbase-master-start", Cmd: masterScript})

	rsScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
if ! jps -lm | grep -q HRegionServer; then
  %s/bin/hbase-daemon.sh start regionserver
fi
`, e.Inv.Cluster.JavaHome, home)
	rsResults := e.Runner.Run(ctx, e.Inv.Roles.RegionServer, orchestrator.Task{Name: "hbase-rs-start", Cmd: rsScript})

	return append(masterResults, rsResults...)
}

func (c HBase) Stop(ctx context.Context, e components.Env) []orchestrator.Result {
	home := Home(e.Inv)
	rsScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hbase-daemon.sh stop regionserver || true
`, e.Inv.Cluster.JavaHome, home)
	rsResults := e.Runner.Run(ctx, e.Inv.Roles.RegionServer, orchestrator.Task{Name: "hbase-rs-stop", Cmd: rsScript})

	masterScript := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hbase-daemon.sh stop master || true
`, e.Inv.Cluster.JavaHome, home)
	masterResults := e.Runner.Run(ctx, e.Inv.Roles.HBaseMaster, orchestrator.Task{Name: "hbase-master-stop", Cmd: masterScript})

	return append(rsResults, masterResults...)
}

func (c HBase) Status(ctx context.Context, e components.Env) []orchestrator.Result {
	return e.Runner.Run(ctx, c.allHosts(e.Inv), orchestrator.Task{
		Name: "hbase-status",
		Cmd:  `jps -lm | grep -E 'HMaster|HRegionServer' || true`,
	})
}

func (c HBase) Uninstall(ctx context.Context, e components.Env, purgeData bool) []orchestrator.Result {
	home := Home(e.Inv)
	script := fmt.Sprintf(`set -e
export JAVA_HOME=%s
%s/bin/hbase-daemon.sh stop regionserver || true
%s/bin/hbase-daemon.sh stop master || true
rm -rf %s
`, e.Inv.Cluster.JavaHome, home, home, home)
	if purgeData {
		script += fmt.Sprintf("rm -rf %s %s\n", LogDir(e.Inv), PidDir(e.Inv))
	}
	return e.Runner.Run(ctx, c.allHosts(e.Inv), orchestrator.Task{Name: "hbase-uninstall", Cmd: script})
}

func failAll(hosts []string, err error) []orchestrator.Result {
	out := make([]orchestrator.Result, 0, len(hosts))
	for _, h := range hosts {
		out = append(out, orchestrator.Result{Host: h, OK: false, Err: err})
	}
	return out
}
```

- [ ] **Step 4: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/components/hbase
git commit -m "$(cat <<'EOF'
feat(components/hbase): install/configure/start/stop/status/uninstall

hbase.rootdir is derived from the NameNode (overridable); zookeeper.quorum is
derived from roles.zookeeper. HBASE_MANAGES_ZK=false because we run our own ZK.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 15: `internal/preflight` — remote environment checks

**Files:**
- Create: `internal/preflight/preflight.go`
- Create: `internal/preflight/preflight_test.go`

- [ ] **Step 1: Write failing test**

Create `internal/preflight/preflight_test.go`:
```go
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
		Cluster: inventory.Cluster{DataDir: "/data/hadoop-cli", JavaHome: "/j"},
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
		"preflight-jdk":    {OK: true, Stdout: "java version \"11.0.1\""},
		"preflight-ports":  {OK: true},
		"preflight-disk":   {OK: true, Stdout: "20G"},
		"preflight-clock":  {OK: true, Stdout: "0"},
	}}
	runner := orchestrator.NewRunner(fe, 2)
	rep, err := Run(context.Background(), baseInv(), runner)
	require.NoError(t, err)
	require.True(t, rep.OK)
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
```

- [ ] **Step 2: Run test, see fail; implement**

Run: `go test ./internal/preflight/...` → FAIL.

Create `internal/preflight/preflight.go`:
```go
package preflight

import (
	"context"
	"fmt"
	"strings"

	"github.com/hadoop-cli/hadoop-cli/internal/errs"
	"github.com/hadoop-cli/hadoop-cli/internal/inventory"
	"github.com/hadoop-cli/hadoop-cli/internal/orchestrator"
)

type Report struct {
	OK      bool
	Host    string
	Check   string
	Message string
	Results []orchestrator.Result
}

func Run(ctx context.Context, inv *inventory.Inventory, runner *orchestrator.Runner) (*Report, error) {
	hosts := inv.AllRoleHosts()

	checks := []struct {
		name     string
		cmd      string
		failCode errs.Code
	}{
		{
			name:     "preflight-jdk",
			cmd:      fmt.Sprintf("%s/bin/java -version 2>&1", inv.Cluster.JavaHome),
			failCode: errs.CodePreflightJDKMissing,
		},
		{
			name: "preflight-ports",
			cmd: fmt.Sprintf(`set -e
for p in %d %d %d %d %d %d %d; do
  if (echo > /dev/tcp/127.0.0.1/$p) 2>/dev/null; then echo "PORT_BUSY:$p"; exit 1; fi
done
echo ok
`,
				inv.Overrides.HDFS.NameNodeRPC, inv.Overrides.HDFS.NameNodeHTTP,
				inv.Overrides.ZooKeeper.ClientPort,
				inv.Overrides.HBase.MasterPort, inv.Overrides.HBase.MasterInfoPort,
				inv.Overrides.HBase.RSPort, inv.Overrides.HBase.RSInfoPort),
			failCode: errs.CodePreflightPortBusy,
		},
		{
			name:     "preflight-disk",
			cmd:      fmt.Sprintf("df -h %s 2>/dev/null | awk 'NR==2{print $4}'", inv.Cluster.DataDir),
			failCode: errs.CodePreflightDiskLow,
		},
		{
			name:     "preflight-clock",
			cmd:      `date -u +%s`,
			failCode: errs.CodePreflightClockSkew,
		},
	}

	allResults := []orchestrator.Result{}
	for _, ch := range checks {
		rs := runner.Run(ctx, hosts, orchestrator.Task{Name: ch.name, Cmd: ch.cmd})
		allResults = append(allResults, rs...)
		for _, r := range rs {
			if !r.OK {
				return &Report{OK: false, Host: r.Host, Check: ch.name, Message: strings.TrimSpace(r.Stderr + r.Stdout), Results: allResults},
					errs.New(ch.failCode, r.Host, fmt.Sprintf("%s on %s: %s", ch.name, r.Host, strings.TrimSpace(r.Stderr+r.Stdout)))
			}
		}
	}
	return &Report{OK: true, Results: allResults}, nil
}
```

- [ ] **Step 3: Run tests and commit**

```bash
go test ./... -race
go vet ./...
gofmt -l .
git add internal/preflight
git commit -m "$(cat <<'EOF'
feat(preflight): JDK / port / disk / clock checks mapped to error codes

Fails fast with PREFLIGHT_* codes and host context so Claude can point the
user at the exact fix.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 16: Cobra subcommands — wire everything into `hadoop-cli <verb>`

**Files:**
- Create: `cmd/common.go`
- Create: `cmd/preflight.go`
- Create: `cmd/install.go`
- Create: `cmd/configure.go`
- Create: `cmd/start.go`
- Create: `cmd/stop.go`
- Create: `cmd/status.go`
- Create: `cmd/uninstall.go`
- Modify: `cmd/root.go` (register subcommands)

- [ ] **Step 1: Common helpers and lifecycle dispatcher**

Create `cmd/common.go`:
```go
package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

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

func registry(forceFormat bool) map[string]components.Component {
	return map[string]components.Component{
		"zookeeper": zookeeper.ZooKeeper{},
		"hdfs":      hdfs.HDFS{ForceFormat: forceFormat},
		"hbase":     hbase.HBase{},
	}
}

type runCtx struct {
	Inv     *inventory.Inventory
	Runner  *orchestrator.Runner
	Pool    *sshx.Pool
	Env     components.Env
	Command string
	Progress *output.Progress
}

func prepare(cmd *cobra.Command, command string) (*runCtx, error) {
	invPath, _ := cmd.Flags().GetString("inventory")
	noColor, _ := cmd.Flags().GetBool("no-color")

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
		Inv:      inv,
		Runner:   runner,
		Pool:     pool,
		Env:      env,
		Command:  command,
		Progress: output.NewProgress(os.Stderr, noColor),
	}, nil
}

func writeEnvelope(cmd *cobra.Command, env *output.Envelope) {
	env.WithRunID(cmd.Context().Value(runIDKey{}).(string))
	_ = env.Write(os.Stdout)
}

type runIDKey struct{}

func withRunID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, runIDKey{}, id)
}

func hostsForLifecycle(rc *runCtx, componentFilter string, phase string) []string {
	// kept for future per-phase reporting; currently unused but documented
	_ = phase
	comps := componentsFor(componentFilter, false)
	seen := map[string]struct{}{}
	out := []string{}
	for _, c := range comps {
		for _, h := range c.Hosts(rc.Inv) {
			if _, ok := seen[h]; !ok {
				seen[h] = struct{}{}
				out = append(out, h)
			}
		}
	}
	return out
}

func componentsFor(filter string, reverse bool) []components.Component {
	reg := registry(false)
	order := components.Ordered()
	if reverse {
		order = components.ReverseOrdered()
	}
	var out []components.Component
	for _, name := range order {
		if filter != "" && filter != name {
			continue
		}
		out = append(out, reg[name])
	}
	return out
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

// deadline returns the default per-command timeout.
func defaultDeadline() time.Duration { return 20 * time.Minute }
```

- [ ] **Step 2: preflight command**

Create `cmd/preflight.go`:
```go
package cmd

import (
	"context"

	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/hadoop-cli/hadoop-cli/internal/preflight"
	"github.com/spf13/cobra"
)

func newPreflightCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "preflight",
		Short: "Run connectivity, JDK, port, disk, and clock checks on every host.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "preflight")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			ctx := withRunID(context.Background(), rc.Env.Run.ID)
			env := output.NewEnvelope("preflight")
			rep, err := preflight.Run(ctx, rc.Inv, rc.Runner)
			aggregate(env, rep.Results)
			_ = rc.Env.Run.SaveResult(env)
			if err != nil {
				env.WithError(output.EnvelopeError{
					Code:    "PREFLIGHT_FAILED",
					Host:    rep.Host,
					Message: err.Error(),
					Hint:    "fix the failing host per the message and rerun `hadoop-cli preflight`",
				})
			}
			writeEnvelope(cmd, env.WithRunID(rc.Env.Run.ID))
			if err != nil {
				return err
			}
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	return c
}
```

- [ ] **Step 3: install/configure commands**

Create `cmd/install.go`:
```go
package cmd

import (
	"context"

	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "install",
		Short: "Download, distribute, and extract tarballs for each component.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "install")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			ctx := withRunID(context.Background(), rc.Env.Run.ID)
			env := output.NewEnvelope("install")
			for _, comp := range componentsFor(component, false) {
				rc.Progress.Infof("", "installing %s ...", comp.Name())
				res := comp.Install(ctx, rc.Env)
				aggregate(env, res)
			}
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(cmd, env.WithRunID(rc.Env.Run.ID))
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	return c
}
```

Create `cmd/configure.go`:
```go
package cmd

import (
	"context"

	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newConfigureCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "configure",
		Short: "Render and push config files (core-site.xml, hdfs-site.xml, zoo.cfg, hbase-site.xml, env.sh, workers, regionservers).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "configure")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			ctx := withRunID(context.Background(), rc.Env.Run.ID)
			env := output.NewEnvelope("configure")
			for _, comp := range componentsFor(component, false) {
				rc.Progress.Infof("", "configuring %s ...", comp.Name())
				res := comp.Configure(ctx, rc.Env)
				aggregate(env, res)
			}
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(cmd, env.WithRunID(rc.Env.Run.ID))
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	return c
}
```

- [ ] **Step 4: start/stop/status commands**

Create `cmd/start.go`:
```go
package cmd

import (
	"context"

	"github.com/hadoop-cli/hadoop-cli/internal/components/hdfs"
	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newStartCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "start",
		Short: "Start the cluster in dependency order: ZK → HDFS → HBase.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "start")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()

			component, _ := cmd.Flags().GetString("component")
			forceFormat, _ := cmd.Flags().GetBool("force-format")
			ctx := withRunID(context.Background(), rc.Env.Run.ID)

			env := output.NewEnvelope("start")
			for _, name := range orderedWithFilter(component, false) {
				rc.Progress.Infof("", "starting %s ...", name)
				comp := pickComponent(name, forceFormat)
				res := comp.Start(ctx, rc.Env)
				aggregate(env, res)
				if !allOK(res) {
					break
				}
			}
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(cmd, env.WithRunID(rc.Env.Run.ID))
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	c.Flags().Bool("force-format", false, "force NameNode re-format (DESTRUCTIVE; wipes HDFS metadata)")
	return c
}

func pickComponent(name string, forceFormat bool) interface {
	Start(ctx context.Context, e componentsEnv) []componentsResult
} {
	// thin wrapper to avoid import cycle in snippet; see cmd/common.go componentsFor for full implementation
	return nil
}

// Note: in the real file, `start` uses componentsFor directly like install/configure.
// The helper above only illustrates the shape; delete it before compilation.
func _ = hdfs.HDFS{}
```

**Correction:** the helpers `pickComponent` / `componentsEnv` above are illustrative only. Use the same pattern as `install.go` — replace the body with:

```go
for _, comp := range componentsFor(component, false) {
    var res []orchestrator.Result
    if comp.Name() == "hdfs" {
        res = hdfs.HDFS{ForceFormat: forceFormat}.Start(ctx, rc.Env)
    } else {
        res = comp.Start(ctx, rc.Env)
    }
    aggregate(env, res)
    if !allOK(res) { break }
}
```

Add helper to `cmd/common.go`:
```go
func allOK(rs []orchestrator.Result) bool {
	for _, r := range rs {
		if !r.OK {
			return false
		}
	}
	return true
}

func orderedWithFilter(filter string, reverse bool) []string {
	order := components.Ordered()
	if reverse {
		order = components.ReverseOrdered()
	}
	if filter == "" {
		return order
	}
	for _, n := range order {
		if n == filter {
			return []string{n}
		}
	}
	return nil
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
```

Create `cmd/stop.go`:
```go
package cmd

import (
	"context"

	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "stop",
		Short: "Stop the cluster in reverse order: HBase → HDFS → ZK.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "stop")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			ctx := withRunID(context.Background(), rc.Env.Run.ID)
			env := output.NewEnvelope("stop")
			for _, comp := range componentsFor(component, true) {
				rc.Progress.Infof("", "stopping %s ...", comp.Name())
				aggregate(env, comp.Stop(ctx, rc.Env))
			}
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(cmd, env.WithRunID(rc.Env.Run.ID))
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	return c
}
```

Create `cmd/status.go`:
```go
package cmd

import (
	"context"

	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: "Check each component's processes on every host.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "status")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			ctx := withRunID(context.Background(), rc.Env.Run.ID)
			env := output.NewEnvelope("status")
			for _, comp := range componentsFor(component, false) {
				aggregate(env, comp.Status(ctx, rc.Env))
			}
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(cmd, env.WithRunID(rc.Env.Run.ID))
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	return c
}
```

Create `cmd/uninstall.go`:
```go
package cmd

import (
	"context"

	"github.com/hadoop-cli/hadoop-cli/internal/output"
	"github.com/spf13/cobra"
)

func newUninstallCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop processes and remove install_dir (optionally purge data_dir).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			rc, err := prepare(cmd, "uninstall")
			if err != nil {
				return err
			}
			defer rc.Pool.Close()
			component, _ := cmd.Flags().GetString("component")
			purge, _ := cmd.Flags().GetBool("purge-data")
			ctx := withRunID(context.Background(), rc.Env.Run.ID)
			env := output.NewEnvelope("uninstall")
			for _, comp := range componentsFor(component, true) {
				rc.Progress.Infof("", "uninstalling %s (purge_data=%v) ...", comp.Name(), purge)
				aggregate(env, comp.Uninstall(ctx, rc.Env, purge))
			}
			_ = rc.Env.Run.SaveResult(env)
			writeEnvelope(cmd, env.WithRunID(rc.Env.Run.ID))
			if !env.OK {
				return errFromEnvelope(env)
			}
			return nil
		},
	}
	c.Flags().String("component", "", "limit to one component: zookeeper|hdfs|hbase")
	c.Flags().Bool("purge-data", false, "also delete cluster.data_dir (DESTRUCTIVE)")
	return c
}
```

- [ ] **Step 5: Register all subcommands**

Modify `cmd/root.go` — add inside `NewRootCmd`, before `return`:
```go
	root.AddCommand(newPreflightCmd())
	root.AddCommand(newInstallCmd())
	root.AddCommand(newConfigureCmd())
	root.AddCommand(newStartCmd())
	root.AddCommand(newStopCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newUninstallCmd())
```

Add test `cmd/lifecycle_test.go`:
```go
package cmd

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoot_RegistersLifecycleCommands(t *testing.T) {
	buf := &bytes.Buffer{}
	root := NewRootCmd()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())
	for _, s := range []string{"preflight", "install", "configure", "start", "stop", "status", "uninstall"} {
		require.Containsf(t, buf.String(), s, "help should mention %s", s)
	}
}
```

- [ ] **Step 6: Run tests, build, commit**

```bash
go mod tidy
go test ./... -race
go vet ./...
gofmt -l .
go build -o bin/hadoop-cli .
./bin/hadoop-cli --help
git add cmd go.mod go.sum
git commit -m "$(cat <<'EOF'
feat(cmd): preflight/install/configure/start/stop/status/uninstall

All commands share one prepare/aggregate/envelope path and emit one JSON
envelope on stdout, human progress on stderr, and stable exit codes.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 17: Claude Code skill — `hbase-cluster-bootstrap`

**Files:**
- Create: `skills/hbase-cluster-bootstrap/SKILL.md`
- Create: `skills/hbase-cluster-bootstrap/references/inventory-schema.md`
- Create: `skills/hbase-cluster-bootstrap/references/bootstrap-runbook.md`
- Create: `skills/hbase-cluster-bootstrap/references/error-codes.md`
- Create: `skills/hbase-cluster-bootstrap/references/examples/3-node-dev.yaml`
- Create: `skills/hbase-cluster-bootstrap/references/examples/single-host.yaml`

- [ ] **Step 1: Create SKILL.md**

Content:
````markdown
---
name: hbase-cluster-bootstrap
version: 1.0.0
description: "基于 hadoop-cli 从零搭建 HBase 集群（含 HDFS、ZooKeeper）。当用户说'帮我搭一个 HBase 集群'/'在这几台机器上部署 HBase'/'快速起一个测试集群'时使用。覆盖 inventory 生成、preflight、install、configure、start 的端到端流程。"
metadata:
  requires:
    bins: ["hadoop-cli"]
  cliHelp: "hadoop-cli --help"
---

# hbase-cluster-bootstrap (v1)

## Prerequisites (user must have done these)

- Control machine can SSH without password to every node (`ssh.private_key` in inventory points to a valid key).
- Every node has JDK 8 or 11 installed; the path is `cluster.java_home` in inventory.
- `/etc/hosts` is consistent across nodes (hostnames resolve to the same addresses).
- The `cluster.user` account exists on every node and owns `cluster.install_dir` and `cluster.data_dir` (or the user has sudo — set `ssh.sudo: true` if so).

If any of the above is unknown, run `hadoop-cli preflight --inventory cluster.yaml` first and fix whatever fails.

## Standard bootstrap flow (follow in order)

1. **Generate inventory**. See `references/inventory-schema.md`. Minimal valid shape is in `references/examples/3-node-dev.yaml`.
2. **Preflight**:
   ```bash
   hadoop-cli preflight --inventory cluster.yaml
   ```
   Expected JSON: `{"command":"preflight","ok":true,...}`. On failure see `references/error-codes.md`.
3. **Install** (downloads tarballs, sftp to each node, extracts):
   ```bash
   hadoop-cli install --inventory cluster.yaml
   ```
   Idempotent: rerunning when nothing changed is a no-op.
4. **Configure** (renders and pushes config files):
   ```bash
   hadoop-cli configure --inventory cluster.yaml
   ```
5. **Start** (ZK → HDFS → HBase, first run auto-formats NameNode):
   ```bash
   hadoop-cli start --inventory cluster.yaml
   ```
6. **Verify**:
   ```bash
   hadoop-cli status --inventory cluster.yaml
   ```
   Expected: every namenode/datanode/zk/hmaster/regionserver process listed; no `ok:false` hosts.

## Common pitfalls

- `roles.zookeeper` must be odd (1, 3, 5). Preflight will reject even counts.
- v1 only supports a single NameNode (`roles.namenode` has exactly 1 host). HA is not available.
- First `start` formats the NameNode and writes `$data_dir/hdfs/nn/.formatted`. Never pass `--force-format` on an existing cluster unless wiping all HDFS data is intended.
- `install` and `configure` are idempotent. When in doubt, rerun; they will not duplicate work.
- If `install` fails mid-flight, read `~/.hadoop-cli/runs/<run-id>/<host>.stderr` for the exact remote output.
````

- [ ] **Step 2: Create references/inventory-schema.md**

Content:
````markdown
# cluster.yaml schema

Top-level keys: `cluster`, `versions`, `ssh`, `hosts`, `roles`, `overrides`.

## `cluster` (required)

| Key          | Type   | Example                      | Notes |
|--------------|--------|------------------------------|-------|
| name         | string | `hbase-dev`                  | Human label only |
| install_dir  | string | `/opt/hadoop-cli`            | MUST be absolute; component homes live under `<install_dir>/hadoop`, `/zookeeper`, `/hbase` |
| data_dir     | string | `/data/hadoop-cli`           | MUST be absolute; nn/dn/zk data, logs, pids live here |
| user         | string | `hadoop`                     | Remote account running processes |
| java_home    | string | `/usr/lib/jvm/java-11`       | Checked by preflight; JDK 8 or 11 |

## `versions` (required)

Supported (v1): Hadoop 3.3.4/3.3.5/3.3.6; ZooKeeper 3.7.2/3.8.3/3.8.4; HBase 2.5.5/2.5.7/2.5.8.

## `ssh` (required)

| Key          | Type    | Default            |
|--------------|---------|--------------------|
| port         | int     | 22                 |
| user         | string  | —                  |
| private_key  | string  | —                  |
| parallelism  | int     | 8                  |
| sudo         | bool    | false              |

## `hosts` (required)

A list of `{name, address}`. `name` is referenced by `roles`.

## `roles` (required)

- `namenode`: exactly 1 host.
- `datanode`: ≥ 1 host.
- `zookeeper`: odd number (1, 3, 5).
- `hbase_master`: ≥ 1 host.
- `regionserver`: ≥ 1 host.

## `overrides` (optional)

See the spec doc for the full list. Common knobs:

- `hdfs.replication` (default 3)
- `hdfs.namenode_heap` / `hdfs.datanode_heap`
- `zookeeper.client_port` (default 2181)
- `hbase.master_heap` / `hbase.regionserver_heap`
- `hbase.root_dir` (auto-derived from NameNode if absent)
````

- [ ] **Step 3: Create references/bootstrap-runbook.md**

Content:
````markdown
# Bootstrap runbook

1. `hadoop-cli preflight --inventory cluster.yaml`
   - On `PREFLIGHT_JDK_MISSING`: install JDK on the listed host and fix `cluster.java_home`.
   - On `PREFLIGHT_PORT_BUSY`: free the port, or change `overrides.*.port` in inventory.
   - On `PREFLIGHT_HOSTNAME_UNRESOLVABLE`: sync `/etc/hosts` across nodes.
2. `hadoop-cli install --inventory cluster.yaml`
3. `hadoop-cli configure --inventory cluster.yaml`
4. `hadoop-cli start --inventory cluster.yaml`
5. `hadoop-cli status --inventory cluster.yaml`

Re-running any step is safe. If a step fails, inspect
`~/.hadoop-cli/runs/<run-id>/<host>.stderr` before retrying.

## First-run NameNode format

The first `start` formats the NameNode. You never need `--force-format` unless
you intentionally want to wipe HDFS metadata.

## Scope boundaries

- HA is not supported.
- JDK / `/etc/hosts` / system user are NOT managed by hadoop-cli (user must prepare).
- Only Linux / macOS target nodes.
````

- [ ] **Step 4: Create references/error-codes.md**

Content:
````markdown
# Error codes → remediation

Every non-zero exit emits a JSON error object:
```json
{"command":"<name>","ok":false,"error":{"code":"<CODE>","host":"<host>","message":"...","hint":"..."}}
```

| Code                              | Typical fix |
|-----------------------------------|-------------|
| SSH_CONNECT_FAILED                | Verify the host is reachable and `ssh.port` is correct. |
| SSH_AUTH_FAILED                   | Fix `ssh.private_key`, run `ssh-copy-id`. |
| PREFLIGHT_JDK_MISSING             | Install JDK 8/11, set `cluster.java_home` to its path. |
| PREFLIGHT_PORT_BUSY               | Free the port or change `overrides.*.port`. |
| PREFLIGHT_HOSTNAME_UNRESOLVABLE   | Sync `/etc/hosts` across all nodes. |
| PREFLIGHT_DISK_LOW                | Free space under `cluster.data_dir`. |
| PREFLIGHT_CLOCK_SKEW              | Enable ntpd/chrony. |
| DOWNLOAD_FAILED                   | Check outbound network or pre-populate `~/.hadoop-cli/cache/`. |
| DOWNLOAD_CHECKSUM_MISMATCH        | Delete the cached tarball; rerun. |
| CONFIG_RENDER_FAILED              | Bug in a template — file an issue. |
| REMOTE_COMMAND_FAILED             | Inspect `~/.hadoop-cli/runs/<run-id>/<host>.stderr`. |
| TIMEOUT                           | Rerun with `--log-level debug`; investigate the slow host. |
| INVENTORY_INVALID                 | Fix `cluster.yaml` per the message. |
| COMPONENT_NOT_READY               | Wait (ZK quorum, NN live) or rerun `start`. |
````

- [ ] **Step 5: Create examples**

Create `skills/hbase-cluster-bootstrap/references/examples/3-node-dev.yaml`:
```yaml
cluster:
  name: hbase-dev
  install_dir: /opt/hadoop-cli
  data_dir: /data/hadoop-cli
  user: hadoop
  java_home: /usr/lib/jvm/java-11
versions:
  hadoop: 3.3.6
  zookeeper: 3.8.4
  hbase: 2.5.8
ssh:
  port: 22
  user: hadoop
  private_key: ~/.ssh/id_rsa
  parallelism: 8
hosts:
  - { name: node1, address: 10.0.0.11 }
  - { name: node2, address: 10.0.0.12 }
  - { name: node3, address: 10.0.0.13 }
roles:
  namenode:     [node1]
  datanode:     [node1, node2, node3]
  zookeeper:    [node1, node2, node3]
  hbase_master: [node1]
  regionserver: [node1, node2, node3]
overrides:
  hdfs:
    replication: 2
```

Create `skills/hbase-cluster-bootstrap/references/examples/single-host.yaml`:
```yaml
cluster:
  name: hbase-single
  install_dir: /opt/hadoop-cli
  data_dir: /data/hadoop-cli
  user: hadoop
  java_home: /usr/lib/jvm/java-11
versions: { hadoop: 3.3.6, zookeeper: 3.8.4, hbase: 2.5.8 }
ssh: { user: hadoop, private_key: ~/.ssh/id_rsa }
hosts:
  - { name: n1, address: 127.0.0.1 }
roles:
  namenode:     [n1]
  datanode:     [n1]
  zookeeper:    [n1]
  hbase_master: [n1]
  regionserver: [n1]
overrides:
  hdfs:
    replication: 1
```

- [ ] **Step 6: Commit skill**

```bash
git add skills/hbase-cluster-bootstrap
git commit -m "$(cat <<'EOF'
docs(skills): add hbase-cluster-bootstrap skill for Claude Code

SKILL.md + references (schema, runbook, error codes) + two example
inventories. Lets Claude drive a full bootstrap from one user request.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 18: Claude Code skill — `hbase-cluster-ops`

**Files:**
- Create: `skills/hbase-cluster-ops/SKILL.md`
- Create: `skills/hbase-cluster-ops/references/status-output.md`
- Create: `skills/hbase-cluster-ops/references/stop-start-ordering.md`
- Create: `skills/hbase-cluster-ops/references/uninstall-guide.md`
- Create: `skills/hbase-cluster-ops/references/troubleshooting.md`

- [ ] **Step 1: Create SKILL.md**

Content:
````markdown
---
name: hbase-cluster-ops
version: 1.0.0
description: "基于 hadoop-cli 的 HBase 集群日常运维：健康检查、停止、重启、卸载。当用户说'检查集群健康'/'重启 hbase'/'停掉集群'/'彻底清掉这个集群'时使用。依赖已经通过 hbase-cluster-bootstrap 建好的 cluster.yaml。"
metadata:
  requires:
    bins: ["hadoop-cli"]
  cliHelp: "hadoop-cli --help"
---

# hbase-cluster-ops (v1)

## Commands you run here

| User intent                | Command                                                                |
|----------------------------|------------------------------------------------------------------------|
| Check health               | `hadoop-cli status --inventory cluster.yaml`                           |
| Stop the cluster           | `hadoop-cli stop --inventory cluster.yaml`                             |
| Start it again             | `hadoop-cli start --inventory cluster.yaml`                            |
| Restart one component      | `hadoop-cli stop --component hbase && hadoop-cli start --component hbase` |
| Remove the install         | `hadoop-cli uninstall --inventory cluster.yaml`                        |
| Nuke install AND data      | `hadoop-cli uninstall --purge-data --inventory cluster.yaml` (DESTRUCTIVE — confirm with the user first) |

## Rules of engagement

- Always read `references/stop-start-ordering.md` before restarting a single
  component — dependencies matter (e.g., stopping ZK while HBase is up will
  error-flood the logs).
- `--purge-data` deletes `cluster.data_dir`. Never pass it without explicit
  user confirmation.
- After any failure, record the `run_id` from the JSON envelope and point
  the user at `~/.hadoop-cli/runs/<run-id>/`.
````

- [ ] **Step 2: Create reference docs**

Create `skills/hbase-cluster-ops/references/status-output.md`:
````markdown
# Reading `hadoop-cli status` output

The command emits one JSON envelope:
```json
{
  "command": "status",
  "ok": true,
  "hosts": [
    {"host": "node1", "ok": true, "elapsed_ms": 80, "message": "NameNode HRegionServer HMaster QuorumPeerMain"},
    {"host": "node2", "ok": true, "elapsed_ms": 75, "message": "DataNode HRegionServer QuorumPeerMain"}
  ]
}
```

Healthy cluster, per role:
- `QuorumPeerMain` on every ZooKeeper host.
- `NameNode` on the NameNode host.
- `DataNode` on every DataNode host.
- `HMaster` on the HBase master host(s).
- `HRegionServer` on every RegionServer host.

If one host is missing a process, restart that component:
```bash
hadoop-cli stop --component <name> --inventory cluster.yaml
hadoop-cli start --component <name> --inventory cluster.yaml
```
````

Create `skills/hbase-cluster-ops/references/stop-start-ordering.md`:
````markdown
# Start / stop ordering

- **start**: zookeeper → hdfs → hbase
- **stop**: hbase → hdfs → zookeeper

Single-component ops are allowed via `--component`, but follow the ordering:

- Restarting `hbase` alone is safe.
- Restarting `hdfs` alone is safe **only if** HBase is stopped first.
- Restarting `zookeeper` alone is safe **only if** HBase is stopped first
  (HDFS does not depend on ZooKeeper in v1, so HDFS can stay up).
````

Create `skills/hbase-cluster-ops/references/uninstall-guide.md`:
````markdown
# uninstall guide

Default (`hadoop-cli uninstall`): stops all processes and removes
`cluster.install_dir` on every node. HDFS data under `cluster.data_dir` is preserved.

Destructive (`hadoop-cli uninstall --purge-data`): ALSO deletes
`cluster.data_dir`, which wipes HDFS metadata, HBase data on HDFS is
effectively inaccessible after this. Confirm with the user before invoking.

After uninstall you can rerun `install` + `configure` + `start` to rebuild.
````

Create `skills/hbase-cluster-ops/references/troubleshooting.md`:
````markdown
# Troubleshooting by symptom

| Symptom                                       | Likely cause                             | Fix                                                                 |
|-----------------------------------------------|------------------------------------------|---------------------------------------------------------------------|
| `status` shows no HMaster                     | HMaster crashed                          | `hadoop-cli stop --component hbase && start --component hbase`; read master log in `$data_dir/hbase/logs/hbase-*-master-*.log`. |
| DataNode missing on one host                  | Disk full or datanode log shows errors   | Check `PREFLIGHT_DISK_LOW`; inspect `$data_dir/hdfs/logs/hadoop-*-datanode-*.log`. |
| ZK quorum not forming                         | Ports blocked, myid mismatch             | Check firewall on 2888/3888; rerun `configure` then `start`.        |
| `install` hangs                               | Slow mirror                              | Pre-download tarballs into `~/.hadoop-cli/cache/`, rerun install.   |
| "Cannot create directory" on any remote step  | `cluster.user` lacks permission          | Grant ownership of `install_dir` / `data_dir`, or set `ssh.sudo: true`. |

For every non-trivial failure: read `~/.hadoop-cli/runs/<run-id>/<host>.stderr`.
````

- [ ] **Step 3: Commit**

```bash
git add skills/hbase-cluster-ops
git commit -m "$(cat <<'EOF'
docs(skills): add hbase-cluster-ops skill for Claude Code

Day-two operations: status interpretation, restart ordering, uninstall with
--purge-data guardrail, and symptom-driven troubleshooting.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Task 19: Release packaging — goreleaser + `.claude-plugin` + README

**Files:**
- Create: `.goreleaser.yaml`
- Create: `.claude-plugin/plugin.json`
- Create: `README.md`
- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: goreleaser**

Create `.goreleaser.yaml`:
```yaml
version: 2
project_name: hadoop-cli

before:
  hooks:
    - go mod tidy

builds:
  - id: hadoop-cli
    main: ./
    binary: hadoop-cli
    env:
      - CGO_ENABLED=0
    goos:   [linux, darwin]
    goarch: [amd64, arm64]
    ldflags:
      - -s -w -X github.com/hadoop-cli/hadoop-cli/cmd.Version={{.Version}}

archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    files:
      - README.md
      - LICENSE
      - skills/**/*

checksum:
  name_template: "{{ .ProjectName }}_{{ .Version }}_checksums.txt"

changelog:
  sort: asc
```

- [ ] **Step 2: Claude Code plugin manifest**

Create `.claude-plugin/plugin.json`:
```json
{
  "name": "hadoop-cli",
  "version": "1.0.0",
  "description": "Bootstrap and manage an HBase cluster (HDFS + ZooKeeper + HBase) via hadoop-cli.",
  "skills": [
    { "path": "skills/hbase-cluster-bootstrap" },
    { "path": "skills/hbase-cluster-ops" }
  ],
  "requires": {
    "bins": ["hadoop-cli"]
  }
}
```

- [ ] **Step 3: README**

Create `README.md`:
```markdown
# hadoop-cli

Single-binary Go CLI that bootstraps and manages an HBase cluster
(HDFS single-NN + ZooKeeper + HBase) on a multi-node Linux/macOS environment
via agentless SSH — designed so [Claude Code](https://claude.com/claude-code)
can drive the whole lifecycle from one user request.

## Install

Download a release tarball, extract, move the binary onto your PATH:
```bash
tar -xzf hadoop-cli_<version>_linux_amd64.tar.gz
sudo mv hadoop-cli /usr/local/bin/
```

Or build from source (requires Go ≥ 1.23):
```bash
make build
sudo install bin/hadoop-cli /usr/local/bin/
```

## Quick start

1. Write a `cluster.yaml` (see `skills/hbase-cluster-bootstrap/references/examples/`).
2. Make sure SSH works: `ssh -i ~/.ssh/id_rsa hadoop@node1 true` on every node.
3. Bootstrap:
   ```bash
   hadoop-cli preflight --inventory cluster.yaml
   hadoop-cli install   --inventory cluster.yaml
   hadoop-cli configure --inventory cluster.yaml
   hadoop-cli start     --inventory cluster.yaml
   hadoop-cli status    --inventory cluster.yaml
   ```

## Using with Claude Code

Install the skills:
```bash
# via npm/npx if published
npx skills add yourorg/hadoop-cli -y -g

# or locally
claude code skills install ./skills/hbase-cluster-bootstrap
claude code skills install ./skills/hbase-cluster-ops
```

Then ask Claude something like "搭一个 3 节点 HBase 测试集群" — it will read
the skill, generate the inventory, and drive `hadoop-cli` end to end.

## Commands

| Command      | What it does |
|--------------|--------------|
| preflight    | JDK / port / disk / clock checks |
| install      | Download, distribute, extract tarballs |
| configure    | Render and push config files |
| start        | ZK → HDFS → HBase in order |
| stop         | Reverse order |
| status       | Process presence on every host |
| uninstall    | Stop and remove install_dir (`--purge-data` also wipes data_dir) |

All commands emit one JSON envelope on stdout and human-readable progress on stderr.

## Scope (v1)

- Single NameNode only. No HDFS HA.
- Only installs Hadoop / ZooKeeper / HBase. JDK, /etc/hosts, OS users must be set up beforehand.
- Linux / macOS target nodes.
```

- [ ] **Step 4: CI workflow**

Create `.github/workflows/ci.yml`:
```yaml
name: ci
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go mod download
      - run: go vet ./...
      - name: gofmt
        run: test -z "$(gofmt -l .)"
      - run: go test ./... -race
      - run: go build -o bin/hadoop-cli .
      - run: ./bin/hadoop-cli --help
      - name: lint skills
        run: |
          for f in skills/*/SKILL.md; do
            head -n 20 "$f" | grep -q '^name: ' || (echo "missing name in $f"; exit 1)
            head -n 20 "$f" | grep -q '^description: ' || (echo "missing description in $f"; exit 1)
          done
```

Create `.github/workflows/release.yml`:
```yaml
name: release
on:
  push:
    tags: ['v*']

jobs:
  release:
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 5: Add LICENSE and final commit**

Create `LICENSE` — copy the MIT License text (standard boilerplate; pick year 2026 and the project's owner name).

Commit:
```bash
git add .goreleaser.yaml .claude-plugin README.md .github LICENSE
git commit -m "$(cat <<'EOF'
chore(release): goreleaser, GitHub Actions CI/release, plugin manifest, README

Single-command release via goreleaser (linux/darwin × amd64/arm64), CI runs
vet + gofmt + tests + a smoke build, and .claude-plugin declares the two
bundled skills for Claude Code integration.

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>
EOF
)"
```

---

## Self-review (for the plan author, before handoff)

- **Spec coverage:** preflight ✓ install ✓ configure ✓ start ✓ stop ✓ status ✓ uninstall ✓ inventory schema ✓ validation ✓ single-NN constraint ✓ odd-ZK constraint ✓ auto-format guard (`.formatted` marker + `--force-format`) ✓ tarball download + SHA-512 ✓ `--purge-data` ✓ parallel SSH ✓ JSON envelope + stderr progress ✓ error codes + hints ✓ run records ✓ two skills + examples ✓ release packaging ✓.
- **Placeholder sweep:** only `PUT_REAL_SHA512_HERE_AT_IMPLEMENTATION_TIME` in `internal/packages/registry.go`; this is intentional and called out in Task 8 — the implementer must paste real values from Apache `.sha512` files before the first live install. Tests cover the cache with their own checksums so CI passes.
- **Type consistency:** `components.Component`, `components.Env`, `orchestrator.Task`, `orchestrator.Result`, `inventory.Inventory` are defined once and reused by name in every later task. `output.Envelope`, `output.HostResult`, `output.EnvelopeError`, `errs.Code`, `errs.CodedError` are likewise single-source.
- **Known loose end:** `cmd/start.go` in Task 16 Step 4 contains two illustrative snippets followed by an explicit "replace with" block — the final file should use the same `componentsFor` pattern as `install.go` (the replacement block is given literally).

## Plan complete

**Plan saved to `docs/superpowers/plans/2026-04-20-hadoop-cli.md`.** Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — execute tasks in this session with checkpoints.

Which approach?
