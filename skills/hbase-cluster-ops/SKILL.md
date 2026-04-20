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
