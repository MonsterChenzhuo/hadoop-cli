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
| `--name`      | yes      | Snapshot name (must not contain `'`, whitespace, or newline).               |
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

> **Note on `--extra-args`:** args are appended raw; the native `hbase ExportSnapshot` takes the *last* occurrence of any duplicated flag. Don't duplicate flags that `hadoop-cli` already exposes (`-mappers`, `-bandwidth`, `-overwrite`, `-snapshot`, `-copy-to`) — the result is implementation-defined.

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
