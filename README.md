# hadoop-cli

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
npx skills add yourorg/hadoop-cli -y -g

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

All commands emit one JSON envelope on stdout and human-readable progress on stderr.

## Scope (v1)

- Single NameNode only. No HDFS HA.
- Only installs Hadoop / ZooKeeper / HBase. JDK, /etc/hosts, OS users must be set up beforehand.
- Linux / macOS target nodes.
