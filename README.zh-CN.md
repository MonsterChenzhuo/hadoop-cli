# hadoop-cli

> English: [README.md](README.md)

单二进制的 Go CLI，通过无 agent SSH 在多节点 Linux/macOS 环境上引导并管理
HBase 集群（HDFS 单 NameNode + ZooKeeper + HBase）——设计目标是让
[Claude Code](https://claude.com/claude-code) 能用一句话驱动整个生命周期。

## 安装

下载 release tarball，解压后把二进制放到 PATH：
```bash
tar -xzf hadoop-cli_<version>_linux_amd64.tar.gz
sudo mv hadoop-cli /usr/local/bin/
```

或从源码构建（需要 Go ≥ 1.23）：
```bash
make build
sudo install bin/hadoop-cli /usr/local/bin/
```

## 快速开始

1. 写一份 `cluster.yaml`（参考 `skills/hbase-cluster-bootstrap/references/examples/`）。
2. 确认 SSH 可达：对每个节点执行 `ssh -i ~/.ssh/id_rsa hadoop@node1 true`。
3. 引导集群：
   ```bash
   hadoop-cli preflight --inventory cluster.yaml
   hadoop-cli install   --inventory cluster.yaml
   hadoop-cli configure --inventory cluster.yaml
   hadoop-cli start     --inventory cluster.yaml
   hadoop-cli status    --inventory cluster.yaml
   ```

## 配合 Claude Code 使用

安装 skills：
```bash
# 通过 npm/npx（如果已发布）
npx skills add MonsterChenzhuo/hadoop-cli -y -g

# 或从本地安装
claude code skills install ./skills/hbase-cluster-bootstrap
claude code skills install ./skills/hbase-cluster-ops
```

然后直接对 Claude 说比如"搭一个 3 节点 HBase 测试集群"——它会读取 skill、
生成 inventory，并端到端驱动 `hadoop-cli`。

## 命令

| 命令             | 作用 |
|------------------|------|
| preflight        | JDK / 端口 / 磁盘 / 时钟 检查 |
| install          | 下载、分发、解压 tarball |
| configure        | 渲染并推送配置文件 |
| start            | 按 ZK → HDFS → HBase 顺序启动 |
| stop             | 反向停止 |
| status           | 在每台主机上检查进程 |
| uninstall        | 停止并移除 install_dir（`--purge-data` 同时清空 data_dir） |
| snapshot         | 通过 hbase shell 打在线快照 |
| export-snapshot  | 通过 hbase ExportSnapshot 把快照同步到远端 HDFS |

所有命令在 stdout 输出一段 JSON envelope，在 stderr 输出人类可读的进度。

## 快照与同步

```bash
# 创建快照
hadoop-cli snapshot --inventory cluster.yaml \
    --table rta:tag_by_uid --name rta_tag_by_uid_1030

# 同步到远端 HDFS
hadoop-cli export-snapshot --inventory cluster.yaml \
    --name rta_tag_by_uid_1030 --to hdfs://10.57.1.211:8020/hbase

# 用目标集群 inventory 推导地址
hadoop-cli export-snapshot --inventory src.yaml \
    --name rta_tag_by_uid_1030 --to-inventory dst.yaml
```

详细说明见 [docs/snapshot.zh-CN.md](docs/snapshot.zh-CN.md)（中文）或
[docs/snapshot.md](docs/snapshot.md)（English）。

## 独立部署单个组件

通过 inventory 的 `cluster.components` 字段可以任选 `{zookeeper, hdfs, hbase}`
的非空子集。不写这个字段时默认部署整套，因此老的 inventory 不受影响。逐个
组件部署比一次拉起整个集群更好控制。

依赖规则：

- `hbase` 要求同一份 inventory 里有 `zookeeper`（HBase 依赖 ZK quorum）。
- `hbase` 不带 `hdfs` 时，必须显式设置 `overrides.hbase.root_dir`，指向
  外部 HDFS（或兼容的存储）。
- `hdfs` 和 `zookeeper` 在 v1 中没有依赖（单 NameNode HDFS）。

只会校验当前 inventory 声明的组件所需的 roles/versions——只部署 ZooKeeper
的 inventory 不需要写 `versions.hadoop`、`roles.namenode` 等字段。

示例——独立的 ZooKeeper 集群：

```yaml
cluster:
  name: zk-dev
  install_dir: /opt/hadoop-cli
  data_dir: /data/hadoop-cli
  user: hadoop
  java_home: /usr/lib/jvm/java-11
  components: [zookeeper]
versions:
  zookeeper: 3.8.4
ssh: { user: hadoop, private_key: ~/.ssh/id_rsa }
hosts:
  - { name: n1, address: 10.0.0.1 }
  - { name: n2, address: 10.0.0.2 }
  - { name: n3, address: 10.0.0.3 }
roles:
  zookeeper: [n1, n2, n3]
```

参见 `skills/hbase-cluster-bootstrap/references/examples/` 下的
`zookeeper-only`、`hdfs-only` 和全栈 inventory 示例。

## 范围（v1）

- 仅单 NameNode，不支持 HDFS HA。
- 只安装 Hadoop / ZooKeeper / HBase。JDK、/etc/hosts、OS 用户需要提前准备好。
- 目标节点为 Linux / macOS。
