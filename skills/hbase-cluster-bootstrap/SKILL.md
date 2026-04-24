---
name: hbase-cluster-bootstrap
version: 1.1.0
description: "基于 hadoop-cli 从零搭建 ZooKeeper / HDFS / HBase 集群,支持独立部署单个组件或完整 HBase 栈。当用户说'帮我搭一个 HBase 集群'/'只装一套 ZooKeeper'/'先把 HDFS 起来'/'在这几台机器上部署 HBase'/'快速起一个测试集群'时使用。覆盖 inventory 生成、preflight、install、configure、start 的端到端流程。"
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

If any of the above is unknown, run `hadoop-cli preflight` first and fix whatever fails.

> Place the generated `cluster.yaml` in the CWD or save it as `~/.hadoop-cli/cluster.yaml` so the remaining commands resolve it automatically. Lookup order: `--inventory <path>` → `$HADOOPCLI_INVENTORY` → `./cluster.yaml` → `~/.hadoop-cli/cluster.yaml`.

## Pick a deployment shape

`cluster.components` in the inventory declares which components this cluster
runs. Any non-empty subset of `{zookeeper, hdfs, hbase}` is accepted; omitting
the field defaults to the full stack. Dependency rules:

- `hbase` requires `zookeeper` in the same inventory (HBase needs a ZK quorum).
- `hbase` without `hdfs` requires `overrides.hbase.root_dir` set explicitly
  (external HDFS address).
- `hdfs` and `zookeeper` have no dependencies.

Required roles and versions are validated **only for the components you
declare**, so a ZK-only inventory does not need `versions.hadoop`,
`roles.namenode`, etc.

| Goal                            | `components` value                     | Example inventory |
|---------------------------------|----------------------------------------|-------------------|
| Full HBase stack (default)      | omit or `[zookeeper, hdfs, hbase]`     | `references/examples/3-node-dev.yaml` |
| Standalone ZooKeeper ensemble   | `[zookeeper]`                          | `references/examples/zookeeper-only.yaml` |
| Standalone HDFS                 | `[hdfs]`                               | `references/examples/hdfs-only.yaml` |
| Single host (lab)               | omit                                   | `references/examples/single-host.yaml` |

Prefer staged deployment when the user wants to bring components up one at a
time: build each inventory separately, or reuse one full-stack inventory and
drive each component with `--component <name>` on `install` / `configure` /
`start`.

## Standard bootstrap flow (follow in order)

1. **Generate inventory**. See `references/inventory-schema.md`. Pick the
   example above that matches the user's intent and edit from there.
2. **Preflight**:
   ```bash
   hadoop-cli preflight
   ```
   Expected JSON: `{"command":"preflight","ok":true,...}`. On failure see `references/error-codes.md`.
3. **Install** (downloads tarballs, sftp to each node, extracts):
   ```bash
   hadoop-cli install
   ```
   Idempotent: rerunning when nothing changed is a no-op.
4. **Configure** (renders and pushes config files):
   ```bash
   hadoop-cli configure
   ```
5. **Start** (honors declared components, in ZK → HDFS → HBase order; first HDFS run auto-formats NameNode):
   ```bash
   hadoop-cli start
   ```
6. **Verify**:
   ```bash
   hadoop-cli status
   ```
   Expected: every process that belongs to a declared component is listed; no `ok:false` hosts.

## Staged deployment (one component at a time)

When the user wants tighter control, run the lifecycle per component using
`--component`. This works with full-stack inventories too — components not
targeted by the flag are left untouched.

```bash
# 1. Bring up ZooKeeper first and verify
hadoop-cli install   --component zookeeper
hadoop-cli configure --component zookeeper
hadoop-cli start     --component zookeeper
hadoop-cli status    --component zookeeper

# 2. Then HDFS
hadoop-cli install   --component hdfs
hadoop-cli configure --component hdfs
hadoop-cli start     --component hdfs

# 3. Finally HBase (ZK + HDFS must be healthy first)
hadoop-cli install   --component hbase
hadoop-cli configure --component hbase
hadoop-cli start     --component hbase
```

`--component <name>` must match a component declared in `cluster.components`,
otherwise the CLI errors out with the declared set so you can fix the flag.

## Common pitfalls

- `roles.zookeeper` must be odd (1, 3, 5) when `zookeeper` is in `components`.
- v1 only supports a single NameNode (`roles.namenode` has exactly 1 host) when `hdfs` is in `components`. HA is not available.
- First `start` of HDFS formats the NameNode and writes `$data_dir/hdfs/nn/.formatted`. Never pass `--force-format` on an existing cluster unless wiping all HDFS data is intended.
- `install` and `configure` are idempotent. When in doubt, rerun; they will not duplicate work.
- If `install` fails mid-flight, read `~/.hadoop-cli/runs/<run-id>/<host>.stderr` for the exact remote output.
- When running `hbase` without `hdfs` in `components`, make sure
  `overrides.hbase.root_dir` points at a reachable external HDFS; otherwise
  `configure` will fail to render `hbase-site.xml`.
