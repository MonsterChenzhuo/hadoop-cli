# hadoop-cli

> 中文版: [README.zh-CN.md](README.zh-CN.md)

Single-binary Go CLI that bootstraps and manages an HBase cluster
(HDFS single-NN + ZooKeeper + HBase) on a multi-node Linux/macOS environment
via agentless SSH — designed so [Claude Code](https://claude.com/claude-code)
can drive the whole lifecycle from one user request.

## Install

Download a release tarball, extract, move the binary onto your PATH:
```bash
tar -xzf hadoop-cli_<version>_linux_amd64.tar.gz
sudo mv hadoop-cli /usr/local/bin/
```

Or build from source (requires Go ≥ 1.23):
```bash
make build
sudo install bin/hadoop-cli /usr/local/bin/
```

## Quick start

1. Write a `cluster.yaml` (see `skills/hbase-cluster-bootstrap/references/examples/`).
2. Make sure SSH works: `ssh -i ~/.ssh/id_rsa hadoop@node1 true` on every node.
3. Bootstrap:
   ```bash
   hadoop-cli preflight --inventory cluster.yaml
   hadoop-cli install   --inventory cluster.yaml
   hadoop-cli configure --inventory cluster.yaml
   hadoop-cli start     --inventory cluster.yaml
   hadoop-cli status    --inventory cluster.yaml
   ```

## Using with Claude Code

Install the skills:
```bash
# via npm/npx if published
npx skills add MonsterChenzhuo/hadoop-cli -y -g

# or locally
claude code skills install ./skills/hbase-cluster-bootstrap
claude code skills install ./skills/hbase-cluster-ops
```

Then ask Claude something like "搭一个 3 节点 HBase 测试集群" — it will read
the skill, generate the inventory, and drive `hadoop-cli` end to end.

## Commands

| Command      | What it does |
|--------------|--------------|
| preflight    | JDK / port / disk / clock checks |
| install      | Download, distribute, extract tarballs |
| configure    | Render and push config files |
| start        | ZK → HDFS → HBase in order |
| stop         | Reverse order |
| status       | Process presence on every host |
| uninstall    | Stop and remove install_dir (`--purge-data` also wipes data_dir) |
| snapshot        | Take an online HBase snapshot via hbase shell |
| export-snapshot | Sync a snapshot to a remote HDFS via hbase ExportSnapshot |

All commands emit one JSON envelope on stdout and human-readable progress on stderr.

## Snapshot & sync / 快照与同步

```bash
# Create a snapshot / 创建快照
hadoop-cli snapshot --inventory cluster.yaml \
    --table rta:tag_by_uid --name rta_tag_by_uid_1030

# Export to a remote HDFS URL / 同步到远端 HDFS
hadoop-cli export-snapshot --inventory cluster.yaml \
    --name rta_tag_by_uid_1030 --to hdfs://10.57.1.211:8020/hbase

# Export using the destination cluster.yaml / 用目标集群 inventory 推导地址
hadoop-cli export-snapshot --inventory src.yaml \
    --name rta_tag_by_uid_1030 --to-inventory dst.yaml
```

See [docs/snapshot.md](docs/snapshot.md) (English) or
[docs/snapshot.zh-CN.md](docs/snapshot.zh-CN.md) (中文) for full details.

## Deploying components independently

Use `cluster.components` in the inventory to pick any non-empty subset of
`{zookeeper, hdfs, hbase}`. Omitting the field defaults to the full stack, so
existing inventories continue to work unchanged. Deploying one component at a
time is easier to control than bringing the whole cluster up in one shot.

Dependency rules:

- `hbase` requires `zookeeper` in the same inventory (HBase needs a ZK quorum).
- `hbase` without `hdfs` requires `overrides.hbase.root_dir` set explicitly,
  pointing at an external HDFS (or compatible storage).
- `hdfs` and `zookeeper` have no dependencies in v1 (single-NN HDFS).

Required roles/versions are validated only for components that are present —
a ZooKeeper-only inventory does not need `versions.hadoop`, `roles.namenode`, etc.

Example — standalone ZooKeeper ensemble:

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

See `skills/hbase-cluster-bootstrap/references/examples/` for `zookeeper-only`,
`hdfs-only`, and full-stack inventories.

## Scope (v1)

- Single NameNode only. No HDFS HA.
- Only installs Hadoop / ZooKeeper / HBase. JDK, /etc/hosts, OS users must be set up beforehand.
- Linux / macOS target nodes.
