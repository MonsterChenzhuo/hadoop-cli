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

## Staged deployment (one component at a time)

Use `--component <name>` to act on a single component while leaving the
others alone. Respect the start order: ZooKeeper before HDFS, HDFS before
HBase. For each component, run install → configure → start → status before
moving on.

```bash
# stage 1: zookeeper
hadoop-cli install   --inventory cluster.yaml --component zookeeper
hadoop-cli configure --inventory cluster.yaml --component zookeeper
hadoop-cli start     --inventory cluster.yaml --component zookeeper
hadoop-cli status    --inventory cluster.yaml --component zookeeper

# stage 2: hdfs (first start auto-formats the NameNode)
hadoop-cli install   --inventory cluster.yaml --component hdfs
hadoop-cli configure --inventory cluster.yaml --component hdfs
hadoop-cli start     --inventory cluster.yaml --component hdfs
hadoop-cli status    --inventory cluster.yaml --component hdfs

# stage 3: hbase (ZK + HDFS must be healthy)
hadoop-cli install   --inventory cluster.yaml --component hbase
hadoop-cli configure --inventory cluster.yaml --component hbase
hadoop-cli start     --inventory cluster.yaml --component hbase
hadoop-cli status    --inventory cluster.yaml --component hbase
```

`--component` must match a component declared in `cluster.components` in the
inventory; otherwise the CLI errors out listing the declared set.

## Single-component inventories

If the user only ever wants one component on this cluster, declare it in the
inventory and skip `--component`:

- ZooKeeper-only: `cluster.components: [zookeeper]` (see
  `examples/zookeeper-only.yaml`). No Hadoop / HBase version or roles needed.
- HDFS-only: `cluster.components: [hdfs]` (see `examples/hdfs-only.yaml`).
  No ZK / HBase version or roles needed.
- HBase pointing at an external HDFS: `cluster.components: [zookeeper, hbase]`
  plus `overrides.hbase.root_dir: hdfs://<external-nn>:<port>/hbase`.

## First-run NameNode format

The first `start` of HDFS formats the NameNode. You never need `--force-format`
unless you intentionally want to wipe HDFS metadata.

## Scope boundaries

- HA is not supported.
- JDK / `/etc/hosts` / system user are NOT managed by hadoop-cli (user must prepare).
- Only Linux / macOS target nodes.
