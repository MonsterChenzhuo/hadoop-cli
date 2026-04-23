# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this project is

`hadoop-cli` is a single-binary Go CLI that installs, configures, and runs the lifecycle (preflight/install/configure/start/stop/status/uninstall) of an HBase stack — ZooKeeper + single-NN HDFS + HBase — on Linux/macOS nodes over agentless SSH, driven by one `cluster.yaml` inventory. The skill directory (`skills/hbase-cluster-bootstrap`, `skills/hbase-cluster-ops`) is how Claude Code is expected to drive this CLI end-to-end from a natural-language request.

v1 scope: single NameNode (no HA), target nodes Linux/macOS, JDK and `/etc/hosts` and the `cluster.user` must exist beforehand. `cluster.components` picks any non-empty subset of `{zookeeper, hdfs, hbase}`; HBase requires ZK in the same inventory, HBase without HDFS requires `overrides.hbase.root_dir`.

## Build / test / dev commands

```bash
make build        # go build -o bin/hadoop-cli .
make test         # go test ./... -race    (required before committing)
make vet
make fmt          # gofmt + fails on diff
make lint         # golangci-lint v2.1.6 via `go run`
make tidy         # go mod tidy
make all          # fmt + vet + test + build
```

Run a single test:

```bash
go test ./internal/preflight -run TestPreflight -race
go test ./cmd -run TestLifecycle -race -v
```

Local cluster smoke: write a `cluster.yaml` (examples in `skills/hbase-cluster-bootstrap/references/examples/`), then run the commands in the order listed in the README.

## Architecture

### Command layer (`cmd/`)
- Each subcommand is a file (`preflight.go`, `install.go`, ...). They all go through `cmd/common.go`'s `prepare()` which loads + validates the inventory, builds an SSH `Pool`, wraps it in an `orchestrator.Runner`, and creates a `runlog.Run` record. Returned `runCtx` is the shared handle.
- `componentsForInv()` intersects inventory-declared components with the optional `--component` filter and returns them in dependency order (or reverse for stop/uninstall). Order lives in `internal/components/component.go`: `Ordered()` = `[zookeeper, hdfs, hbase]`.
- Every subcommand emits exactly one JSON envelope on stdout (`internal/output`) and human-readable progress on stderr. The envelope shape (`command`, `ok`, `summary`, `hosts`, `error`, `run_id`) is stable and consumed by Claude via the skill.

### Components (`internal/components/{zookeeper,hdfs,hbase}`)
Each implements the `Component` interface: `Name/Hosts/Install/Configure/Start/Stop/Status/Uninstall`. Subcommands iterate over selected components and aggregate per-host `orchestrator.Result`s into the envelope via `aggregate()`. Components are expected to be **idempotent** — rerunning a successful install/configure is a no-op.

### Orchestrator + SSH (`internal/orchestrator`, `internal/ssh`)
`orchestrator.Runner` fans a `Task` out across hosts with bounded parallelism (`ssh.parallelism` in inventory, default 4) and a 5-minute default per-task timeout. `SSHExecutor` drives a pooled `ssh.Pool`; one connection per host, reused. Components never touch SSH directly — they build `orchestrator.Task` values and hand them to the runner.

### Inventory (`internal/inventory`)
`Load` parses YAML, `Validate` enforces structural rules (odd ZK count, single namenode in v1, required versions/roles per declared component). `HasComponent(name)` is the accessor subcommands use when deciding what to act on.

### Run log (`internal/runlog`)
Every invocation gets a `~/.hadoop-cli/runs/<run-id>/` directory. Per-host stdout/stderr from failed tasks lands there; the final envelope is saved as `result.json` via `SaveResult`. When debugging `install` failures, read `<run-id>/<host>.stderr`.

### Packages cache (`internal/packages`)
Upstream tarballs are fetched into `~/.hadoop-cli/packages/` (via `DefaultCacheDir()`), verified, and distributed to each host from the control machine. The cache is content-addressed — changing `versions.*` in inventory triggers a re-fetch.

### HBase ops (`internal/hbaseops`)
Snapshot + export-snapshot live here, not in `internal/components/hbase`, because they run against a live cluster rather than installing anything. `cmd/snapshot.go` and `cmd/export_snapshot.go` are thin wrappers. `BuildSnapshotScript` / `BuildExportCommand` / `DeriveCopyToFromInventory` / `PickHost` are the pieces worth knowing about when changing those commands. Shell arguments that reach `hbase shell` are metacharacter-rejected (`hbaseops` has injection guards) — do not weaken them.

### Preflight (`internal/preflight`)
Standalone read-only checks (JDK/port/disk/clock). Invoked by `cmd/preflight.go`. A design is in progress to layer a `plan` subcommand + facts safety gate on top of these checks — see `docs/superpowers/specs/2026-04-23-plan-subcommand-design.md`.

## Conventions

- **Stdout is machine output (envelope JSON), stderr is human output.** Never print free-form text to stdout.
- **Error propagation**: failing subcommands still write a full envelope before returning an error; `errFromEnvelope()` converts envelope errors into `error` so the process exits non-zero with a readable message.
- **Idempotence is part of the contract** for `install`, `configure`, `start`, `stop`, `uninstall`. Tests (`cmd/lifecycle_test.go`) exercise this.
- **Component filtering**: `--component` must name a component in `cluster.components`, otherwise the CLI errors out listing the declared set. Don't silently no-op.
- **SSH I/O**: go through `orchestrator.Task` / `Runner`. Do not open `ssh.Client` directly from a subcommand or component.
- **Skill-visible output**: the envelope schema (`internal/output/envelope.go`) is a public contract — skills parse it. Treat additions as additive; don't rename or remove fields without updating `skills/` in the same change.

## Documentation

- `README.md` / `README.zh-CN.md` — user-facing quick start.
- `docs/snapshot.md` / `docs/snapshot.zh-CN.md` — snapshot + export-snapshot.
- `docs/superpowers/specs/` — design docs for non-trivial features (TDD plans live in `docs/superpowers/plans/`).
- `skills/hbase-cluster-bootstrap/SKILL.md` + `skills/hbase-cluster-ops/SKILL.md` — what Claude Code reads to drive the CLI. Update these whenever user-visible CLI behavior or recommended flow changes.
