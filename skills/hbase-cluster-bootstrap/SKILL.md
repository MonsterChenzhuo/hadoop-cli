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
