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
| Check health               | `hadoop-cli status`                                                    |
| Stop the cluster           | `hadoop-cli stop`                                                      |
| Start it again             | `hadoop-cli start`                                                     |
| Restart one component      | `hadoop-cli stop --component hbase && hadoop-cli start --component hbase` |
| Remove the install         | `hadoop-cli uninstall`                                                 |
| Nuke install AND data      | `hadoop-cli uninstall --purge-data` (DESTRUCTIVE — confirm with the user first) |
| 打快照 / take a snapshot           | `hadoop-cli snapshot --table <ns:t> --name <snap>` |
| 同步快照到 B 集群 / sync snapshot   | `hadoop-cli export-snapshot --name <snap> --to hdfs://<nn>:8020/hbase` |
| 同步到 B 集群 (已有 inventory)     | `hadoop-cli export-snapshot --inventory src.yaml --name <snap> --to-inventory dst.yaml` |

> Inventory is resolved from `$HADOOPCLI_INVENTORY`, `./cluster.yaml`, or `~/.hadoop-cli/cluster.yaml` unless `--inventory <path>` is passed. Pass `--inventory` explicitly when running against a non-default cluster or from an unrelated CWD; the export-snapshot row with `--to-inventory` keeps `--inventory src.yaml` because two inventories are involved.

## Rules of engagement

- Always read `references/stop-start-ordering.md` before restarting a single
  component — dependencies matter (e.g., stopping ZK while HBase is up will
  error-flood the logs).
- `--purge-data` deletes `cluster.data_dir`. Never pass it without explicit
  user confirmation.
- After any failure, record the `run_id` from the JSON envelope and point
  the user at `~/.hadoop-cli/runs/<run-id>/`.

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
