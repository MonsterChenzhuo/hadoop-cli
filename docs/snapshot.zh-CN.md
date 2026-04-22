# HBase 快照与同步

> English: [snapshot.md](snapshot.md)

`hadoop-cli` 封装了两个常用的 HBase 快照动作，让 Claude Code 在一次请求里
就能完成：

1. `hadoop-cli snapshot` — 对一张表做在线快照。
2. `hadoop-cli export-snapshot` — 把快照同步到远端 HDFS。

两条命令都会 SSH 到 HBase master（`roles.hbase_master` 的第 1 台），在那
里调用对应的 `bin/hbase`。

## `hadoop-cli snapshot`

| Flag          | 必填 | 说明                                                         |
|---------------|------|--------------------------------------------------------------|
| `--table`     | 是   | HBase 表名，例如 `rta:tag_by_uid`。                          |
| `--name`      | 是   | 快照名（不能包含 `'`、空白或换行）。                         |
| `--skip-flush`| 否   | 追加 `{SKIP_FLUSH => true}`，跳过 memstore flush。           |
| `--on`        | 否   | 指定执行节点，默认 `roles.hbase_master[0]`。                 |

示例：

```bash
hadoop-cli snapshot --inventory cluster.yaml \
    --table rta:tag_by_uid --name rta_tag_by_uid_1030
```

实际执行：

```
echo "snapshot 'rta:tag_by_uid','rta_tag_by_uid_1030'" | $HBASE_HOME/bin/hbase shell -n
```

## `hadoop-cli export-snapshot`

| Flag              | 必填 | 说明                                                                |
|-------------------|------|---------------------------------------------------------------------|
| `--name`          | 是   | 要同步的快照名。                                                    |
| `--to`            | 二选一 | 目标 HDFS URL，必须以 `hdfs://` 开头。                            |
| `--to-inventory`  | 二选一 | 目标集群的 `cluster.yaml`；从中推导 `hdfs://<nn>:<rpc>/hbase`。   |
| `--mappers`       | 否   | mapper 数量。不传 = 用 HBase 默认；`0` = LocalJobRunner。           |
| `--bandwidth`     | 否   | 单个 mapper 的 MB/s 限速，`0` 表示不限速。                          |
| `--overwrite`     | 否   | 覆盖目标已有快照。                                                  |
| `--extra-args`    | 否   | 原样追加到命令末尾。                                                |
| `--on`            | 否   | 指定执行节点，同 `snapshot`。                                       |

`--to` 与 `--to-inventory` 互斥，必须二选一。

示例：

```bash
# URL 模式 —— 等价于直接调 hbase 原生命令
hadoop-cli export-snapshot --inventory cluster.yaml \
    --name rta_tag_by_uid_1030 --to hdfs://10.57.1.211:8020/hbase

# Inventory 模式 —— 从 dst.yaml 的 roles.namenode 和
# overrides.hdfs.namenode_rpc_port（默认 8020）推导 URL
hadoop-cli export-snapshot --inventory src.yaml \
    --name rta_tag_by_uid_1030 --to-inventory dst.yaml
```

### 参数映射到原生 `ExportSnapshot`

| hadoop-cli 参数   | 原生 `hbase ExportSnapshot` 参数  |
|-------------------|-----------------------------------|
| `--name`          | `-snapshot`                       |
| `--to` / `--to-inventory` | `-copy-to`                |
| `--mappers N`     | `-mappers N`                      |
| `--bandwidth N`   | `-bandwidth N`                    |
| `--overwrite`     | `-overwrite`                      |
| `--extra-args`    | 原样追加                          |

## 性能说明：LocalJobRunner

本 CLI 搭出来的集群不含 YARN，`ExportSnapshot` 会自动退回 Hadoop 的
`LocalJobRunner` 进程内单机拷贝。小中快照够用，大快照会慢。需要时用
`--mappers` / `--bandwidth` 调参，或通过 `--extra-args "-D ..."` 接入外部
YARN。

## 常见错误

- `--to must start with hdfs://` —— 传了本地路径。请用 `--to hdfs://...`
  或 `--to-inventory <yaml>`。
- `destination inventory must have exactly 1 roles.namenode` —— 目标
  `cluster.yaml` 的 NameNode 数量不是 1（可能是 HA）。HA 暂不支持，改用
  `--to` 直接传 URL。
- `--on host "X" is not in the inventory` —— 主机名拼错；必须匹配源
  inventory `hosts:` 里声明过的名字。
- HBase 报 `Snapshot 'X' already exists` —— 换名字，或先手工 `hbase shell`
  删除后再跑；export 可加 `--overwrite`。
