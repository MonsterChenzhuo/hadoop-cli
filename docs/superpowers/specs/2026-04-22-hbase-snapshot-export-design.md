# HBase Snapshot & ExportSnapshot Support — Design

Date: 2026-04-22
Scope: Add `hadoop-cli snapshot` and `hadoop-cli export-snapshot` subcommands.

## Goal

Let `hadoop-cli` drive two operational HBase actions over the existing SSH/inventory machinery:

1. Take an online snapshot of a table on the managed cluster.
2. Copy a snapshot to another HDFS location (remote cluster) via
   `hbase org.apache.hadoop.hbase.snapshot.ExportSnapshot`.

Concrete motivating example:

```bash
hbase org.apache.hadoop.hbase.snapshot.ExportSnapshot \
  -snapshot rta_tag_by_uid_1030 \
  -copy-to hdfs://10.57.1.211:8020/hbase
```

Out of scope (YAGNI): `list_snapshots`, `delete_snapshot`, `restore_snapshot`,
`clone_snapshot`, progress UI, snapshot pruning/retention policy.

## Architecture

A new internal package `internal/hbaseops` owns the command-building logic.
Snapshot/export are *operational* actions, not lifecycle (install/configure/
start/stop/status/uninstall); keeping them out of
`internal/components/hbase` preserves the lifecycle-only role of the
`components.Component` interface.

```
cmd/snapshot.go ─────┐
                     ├──► internal/hbaseops ──► orchestrator.Runner ──► SSHExecutor
cmd/export_snapshot.go ┘          │
                                   └──► inventory.Load (for --to-inventory)
```

- Component interface (`internal/components/component.go`) is **not** modified.
- `orchestrator.Runner.Run(ctx, hosts, task)` is reused with a single-host
  list (the chosen HBase master).
- `internal/components/hbase.Home(inv)` is already exported and is reused to
  locate `$install_dir/hbase/bin/hbase`.
- Bilingual docs (English + 中文) are a first-class deliverable.

## CLI

```
hadoop-cli snapshot \
  --inventory cluster.yaml \
  --table <ns:table> \
  --name <snapshot_name> \
  [--skip-flush] \
  [--on <host>]

hadoop-cli export-snapshot \
  --inventory cluster.yaml \
  --name <snapshot_name> \
  ( --to hdfs://host:port/hbase | --to-inventory path/to/dest.yaml ) \
  [--mappers N] \
  [--bandwidth MB] \
  [--overwrite] \
  [--extra-args "..."] \
  [--on <host>]
```

Flag semantics:

- `--inventory` comes from the root persistent flag — not redeclared.
- `snapshot --table` and `--name` are both required; `--skip-flush` toggles
  `{SKIP_FLUSH => true}` on the hbase-shell call (default: online flush).
- `export-snapshot` requires exactly one of `--to` or `--to-inventory`.
  - `--to` is passed verbatim as `-copy-to`. Basic validation: must start
    with `hdfs://`.
  - `--to-inventory`: load that inventory, require
    `len(Roles.NameNode) == 1`, then derive
    `hdfs://<namenode>:<Overrides.HDFS.NameNodeRPC>/hbase`.
- `--on <host>` overrides the execution host. If omitted, the source
  inventory's `Roles.HBaseMaster[0]` is used. Value must appear in
  `inv.AllRoleHosts()`; otherwise reject before SSH.
- `--mappers`: default is unset (pass nothing, let HBase pick; without YARN
  it falls back to `LocalJobRunner`). `0` is allowed as an explicit "local";
  negative values are rejected.
- `--bandwidth`, `--overwrite`: thin pass-throughs to the ExportSnapshot
  flags of the same name.
- `--extra-args` is a single string appended at the end of the command as an
  escape hatch; not re-parsed.

Output: reuse `internal/output`. On success, print one line with host +
elapsed; on failure print host, exit code, and the tail of stderr.

## Data flow

### `hadoop-cli snapshot`

1. Load inventory; require `len(Roles.HBaseMaster) >= 1`.
2. Pick host = `--on` or `Roles.HBaseMaster[0]`; verify it is in the
   inventory.
3. Build the script executed on the host:
   ```bash
   set -e
   export JAVA_HOME=<Cluster.JavaHome>
   echo "snapshot '<table>','<name>'[, {SKIP_FLUSH => true}]" \
     | <HBASE_HOME>/bin/hbase shell -n
   ```
   - `hbase shell -n` makes non-zero exit on Ruby exceptions (default
     behaviour swallows them).
   - The command string is injected via a single-quoted hbase-shell
     literal; `<table>` and `<name>` are validated beforehand (see Error
     handling) so quoting is safe.
4. `Runner.Run(ctx, []string{host}, task)`; report the single result.

### `hadoop-cli export-snapshot`

1. Load source inventory. If `--to-inventory` is set, load it too and
   compute `copyTo := fmt.Sprintf("hdfs://%s:%d/hbase", nn, rpc)`.
   Otherwise `copyTo := --to`.
2. Pick host same as above.
3. Build the command:
   ```bash
   set -e
   export JAVA_HOME=<Cluster.JavaHome>
   <HBASE_HOME>/bin/hbase org.apache.hadoop.hbase.snapshot.ExportSnapshot \
     -snapshot <name> \
     -copy-to <copyTo> \
     [-mappers N] [-bandwidth MB] [-overwrite] \
     <extra-args>
   ```
   Each interpolated value is shell-escaped (`%q` or equivalent) to
   neutralize spaces and metacharacters.
4. `Runner.Run`; propagate the single result. Do not attempt additional
   success verification — ExportSnapshot exits non-zero on failure.

## Error handling and validation

Reject before SSH whenever possible:

| Case | Behaviour |
|------|-----------|
| `--to` and `--to-inventory` both set or both empty | cobra-level error |
| `--to` not prefixed with `hdfs://` | error |
| `--to-inventory` fails `inventory.Load` or validate | surface error |
| `--to-inventory` target `Roles.NameNode` length ≠ 1 | error |
| `--on` host not in source inventory | error |
| `Roles.HBaseMaster` empty and `--on` unset | error |
| `--table` / `--name` empty, or contains `'` or `\n` | error (injection guard) |
| `--mappers` negative | error |

SSH failure: surface `orchestrator.Result.{ExitCode, Stderr}` and return a
non-zero process exit via the existing `cmd` error path.

Explicitly **not** handled (left to HBase / user):

- Whether the snapshot name already exists (HBase will complain).
- Whether the destination path already exists (user uses `--overwrite`).
- HDFS permissions on the destination.
- Progress reporting for large exports.

## Documentation (bilingual)

- `README.md`
  - Add `snapshot` and `export-snapshot` rows to the Commands table.
  - New `### Snapshot & sync / 快照与同步` subsection under Quick start,
    with short English + 中文 lines and 3 worked examples (create, export
    via URL, export via `--to-inventory`).
- `docs/snapshot.md` (English) and `docs/snapshot.zh-CN.md` (中文):
  parallel documents covering
  1. Concept overview and when to use it.
  2. Every flag of both subcommands.
  3. `--to` vs `--to-inventory` tradeoffs.
  4. LocalJobRunner performance note (no YARN in this cluster shape).
  5. Common errors and how to read them.
  6. A mapping table between our flags and native `ExportSnapshot` flags.
  Each file links to its counterpart at the top.
- `skills/hbase-cluster-ops/SKILL.md`: add a "Snapshot / 快照" section so
  Claude Code can route natural-language requests (e.g. "帮我把 rta 表快照
  一下并同步到集群 B") to the correct subcommands. Bilingual.
- `cmd.Short` / `cmd.Long` stay English (matches the existing style).
  `cmd.Example` includes both a short English note and a 中文 note per
  example so `hadoop-cli snapshot -h` is readable for Chinese users.

## Testing

Unit tests, no real SSH (matches the rest of the repo):

- `internal/hbaseops/snapshot_test.go`
  - Script assembly: table/name/skip-flush combinations produce the
    expected `snapshot 'x','y'` form, `SKIP_FLUSH => true`, and
    `hbase shell -n`.
  - Injection guard: names containing `'` or `\n` return an error.
- `internal/hbaseops/export_test.go`
  - URL mode passes `--to` through verbatim.
  - Inventory mode derives `hdfs://<nn>:<rpc>/hbase` from a fake inventory
    (e.g. `namenode=node9`, `rpc=9000` → `hdfs://node9:9000/hbase`).
  - Parameter combinations: `mappers / bandwidth / overwrite / extra-args`.
  - Negative cases: non-`hdfs://` `--to`, both flags set, NameNode count
    ≠ 1.
- `cmd/snapshot_test.go` and `cmd/export_snapshot_test.go`: follow the
  existing pattern in `cmd/root_test.go` using a fake `Runner`; assert
  flag parsing and host selection.

## Work items summary

1. New package `internal/hbaseops` with `Snapshot` and `ExportSnapshot`
   functions plus tests.
2. New cobra commands `cmd/snapshot.go` and `cmd/export_snapshot.go`,
   wired in `cmd/root.go`.
3. Bilingual docs: `README.md` delta, `docs/snapshot.md`,
   `docs/snapshot.zh-CN.md`, `skills/hbase-cluster-ops/SKILL.md` delta.
4. Tests as listed above.

Non-goals: changes to the `components.Component` interface, list/
delete/restore/clone, progress UI, YARN support.
