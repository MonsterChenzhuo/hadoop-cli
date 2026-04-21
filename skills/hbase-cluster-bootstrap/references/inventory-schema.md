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
| components   | list   | `[zookeeper]`                | Optional. Supported values: `[zookeeper]` (standalone ZK) or `[zookeeper, hdfs, hbase]`. Omitted → full stack. |

## `versions` (required)

Supported (v1): Hadoop 3.3.4/3.3.5/3.3.6; ZooKeeper 3.7.2/3.8.3/3.8.4; HBase 2.5.5/2.5.7/2.5.8.

`versions.hadoop` and `versions.hbase` are only required when the
corresponding component is in `cluster.components`; `versions.zookeeper` is
always required.

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

- `zookeeper`: odd number (1, 3, 5) — always required.
- `namenode`: exactly 1 host — required only when `hdfs` is in `components`.
- `datanode`: ≥ 1 host — required only when `hdfs` is in `components`.
- `hbase_master`: ≥ 1 host — required only when `hbase` is in `components`.
- `regionserver`: ≥ 1 host — required only when `hbase` is in `components`.

## `overrides` (optional)

See the spec doc for the full list. Common knobs:

- `hdfs.replication` (default 3)
- `hdfs.namenode_heap` / `hdfs.datanode_heap`
- `zookeeper.client_port` (default 2181)
- `hbase.master_heap` / `hbase.regionserver_heap`
- `hbase.root_dir` (auto-derived from NameNode if absent)
