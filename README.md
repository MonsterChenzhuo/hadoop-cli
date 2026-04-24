# hadoop-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-blue.svg)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/MonsterChenzhuo/hadoop-cli.svg)](https://github.com/MonsterChenzhuo/hadoop-cli/releases)

[中文版](./README.zh-CN.md) | [English](./README.md)

Single-binary Go CLI that bootstraps and manages an HBase cluster (HDFS single-NN + ZooKeeper + HBase) across multi-node Linux/macOS hosts over agentless SSH — designed so [Claude Code](https://claude.com/claude-code) can drive the whole lifecycle from one natural-language request.

[Install](#installation) · [Upgrade](#upgrade) · [Quick Start](#quick-start) · [Claude Code](#use-with-claude-code) · [Commands](#commands) · [Snapshot](#snapshot--sync) · [Components](#deploy-components-independently) · [Scope](#scope-v1)

## Why hadoop-cli?

- **One binary, one inventory** — `cluster.yaml` is the single source of truth; no agents to install on target nodes
- **Full lifecycle** — preflight → install → configure → start/stop/status → uninstall, with consistent JSON output
- **Agent-native** — ships with two [Claude Code skills](#use-with-claude-code); Claude can take "搭一个 3 节点 HBase 测试集群" end-to-end
- **Composable components** — any non-empty subset of `{zookeeper, hdfs, hbase}`; deploy a ZK-only ensemble or reuse an external HDFS
- **Idempotent by design** — rerunning `install` / `configure` / `start` is a no-op; safe to retry
- **Machine-friendly** — every command emits one JSON envelope on stdout, human progress on stderr

## Installation

### Requirements

- Target nodes: Linux or macOS, reachable by SSH from the control machine
- JDK, `/etc/hosts`, and the `cluster.user` provisioned on each node beforehand
- Control machine: `curl`, `tar`, `bash` (for one-line install); Go `v1.23`+ only if you build from source

### Option 1 — One-line install (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/MonsterChenzhuo/hadoop-cli/main/scripts/install.sh | bash
```

The script auto-detects OS/arch (linux/darwin × amd64/arm64), downloads the latest release, verifies the checksum, installs the binary to `/usr/local/bin/hadoop-cli`, and drops the bundled skills under `~/.hadoop-cli/skills/`.

Customize with environment variables:

```bash
# Pin a specific version
curl -fsSL https://raw.githubusercontent.com/MonsterChenzhuo/hadoop-cli/main/scripts/install.sh | VERSION=v0.1.2 bash

# Install to a user-local prefix (no sudo)
curl -fsSL https://raw.githubusercontent.com/MonsterChenzhuo/hadoop-cli/main/scripts/install.sh | PREFIX=$HOME/.local/bin NO_SUDO=1 bash
```

### Option 2 — Download a release tarball

Pick your platform from [GitHub Releases](https://github.com/MonsterChenzhuo/hadoop-cli/releases):

```bash
VER=v0.1.2
OS=linux   # or darwin
ARCH=amd64 # or arm64
curl -fsSL -o hadoop-cli.tar.gz \
  "https://github.com/MonsterChenzhuo/hadoop-cli/releases/download/${VER}/hadoop-cli_${VER#v}_${OS}_${ARCH}.tar.gz"
tar -xzf hadoop-cli.tar.gz
sudo install -m 0755 hadoop-cli /usr/local/bin/
```

### Option 3 — Build from source

Requires Go `v1.23`+.

```bash
git clone https://github.com/MonsterChenzhuo/hadoop-cli.git
cd hadoop-cli
make build
sudo install bin/hadoop-cli /usr/local/bin/
```

### Verify

```bash
hadoop-cli --version
hadoop-cli --help
```

## Upgrade

The installer is idempotent — to upgrade to the latest release, run **the same one-liner** again:

```bash
curl -fsSL https://raw.githubusercontent.com/MonsterChenzhuo/hadoop-cli/main/scripts/install.sh | bash
```

This overwrites `hadoop-cli` in place and refreshes `~/.hadoop-cli/skills/`. Your `cluster.yaml`, run logs under `~/.hadoop-cli/runs/`, and cached packages under `~/.hadoop-cli/packages/` are untouched.

If you installed from source, upgrade with:

```bash
cd hadoop-cli
git pull
make build
sudo install bin/hadoop-cli /usr/local/bin/
```

## Quick Start

1. **Write `cluster.yaml`** — pick an example from [`skills/hbase-cluster-bootstrap/references/examples/`](./skills/hbase-cluster-bootstrap/references/examples/). Save it in the current directory or as `~/.hadoop-cli/cluster.yaml` and you can skip `--inventory` on every command.
2. **Verify SSH reachability** — `ssh -i ~/.ssh/id_rsa hadoop@node1 true` on every node listed in `hosts:`.
3. **Bootstrap the cluster**:

   ```bash
   hadoop-cli preflight    # JDK / port / disk / clock checks
   hadoop-cli install      # download, distribute, extract tarballs
   hadoop-cli configure    # render and push config files
   hadoop-cli start        # ZK → HDFS → HBase in order
   hadoop-cli status       # process presence on every host
   ```

Inventory is resolved in this order: `--inventory <path>` → `$HADOOPCLI_INVENTORY` → `./cluster.yaml` → `~/.hadoop-cli/cluster.yaml`. The resolved path is printed to stderr (`using inventory: …`) and echoed in every JSON envelope as `inventory_path`.

Every command writes one JSON envelope to stdout (stable schema: `command`, `ok`, `summary`, `hosts`, `error`, `run_id`, `inventory_path`) and human-readable progress to stderr. Per-run logs land in `~/.hadoop-cli/runs/<run-id>/`.

## Use with Claude Code

Two skills ship in the release archive; installing them lets Claude Code drive `hadoop-cli` end-to-end from a natural-language request.

```bash
# Skills are already in ~/.hadoop-cli/skills/ after one-line install.
# Register them with Claude Code:
claude code skills install ~/.hadoop-cli/skills/hbase-cluster-bootstrap
claude code skills install ~/.hadoop-cli/skills/hbase-cluster-ops
```

Then ask Claude something like **"搭一个 3 节点 HBase 测试集群"** — it reads the skill, generates `cluster.yaml`, and runs `hadoop-cli preflight → install → configure → start → status`.

| Skill                        | Description                                                         |
| ---------------------------- | ------------------------------------------------------------------- |
| `hbase-cluster-bootstrap`    | Author `cluster.yaml` and run the full bootstrap lifecycle          |
| `hbase-cluster-ops`          | Day-2 operations: status checks, snapshots, export, uninstall       |

## Commands

| Command           | What it does                                                            |
| ----------------- | ----------------------------------------------------------------------- |
| `preflight`       | JDK / port / disk / clock checks                                        |
| `install`         | Download, distribute, extract tarballs                                  |
| `configure`       | Render and push config files                                            |
| `start`           | ZK → HDFS → HBase in dependency order                                   |
| `stop`            | Reverse order                                                           |
| `status`          | Process presence on every host                                          |
| `uninstall`       | Stop and remove `install_dir` (`--purge-data` also wipes `data_dir`)    |
| `snapshot`        | Take an online HBase snapshot via `hbase shell`                         |
| `export-snapshot` | Sync a snapshot to a remote HDFS via `hbase ExportSnapshot`             |

Use `--component zookeeper,hdfs,hbase` to restrict a command to a subset of what the inventory declares.

## Snapshot & Sync

```bash
# Create a snapshot
hadoop-cli snapshot \
    --table rta:tag_by_uid --name rta_tag_by_uid_1030

# Export to a remote HDFS URL
hadoop-cli export-snapshot \
    --name rta_tag_by_uid_1030 --to hdfs://10.57.1.211:8020/hbase

# Export using the destination cluster.yaml to derive the URL
hadoop-cli export-snapshot --inventory src.yaml \
    --name rta_tag_by_uid_1030 --to-inventory dst.yaml
```

Full details in [docs/snapshot.md](docs/snapshot.md) (English) / [docs/snapshot.zh-CN.md](docs/snapshot.zh-CN.md) (中文).

## Deploy Components Independently

Use `cluster.components` to pick any non-empty subset of `{zookeeper, hdfs, hbase}`. Omitting the field deploys the full stack, so existing inventories keep working.

Dependency rules:

- `hbase` requires `zookeeper` in the same inventory (HBase needs a ZK quorum).
- `hbase` without `hdfs` requires `overrides.hbase.root_dir` set explicitly, pointing at an external HDFS (or compatible storage).
- `hdfs` and `zookeeper` have no dependencies in v1 (single-NN HDFS).

Required roles/versions are validated only for components actually present — a ZooKeeper-only inventory does not need `versions.hadoop`, `roles.namenode`, etc.

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

See `skills/hbase-cluster-bootstrap/references/examples/` for `zookeeper-only`, `hdfs-only`, and full-stack inventories.

## Scope (v1)

- Single NameNode only. No HDFS HA.
- Only installs Hadoop / ZooKeeper / HBase. JDK, `/etc/hosts`, and OS users must be set up beforehand.
- Linux / macOS target nodes.

## Contributing

Community contributions are welcome. Please open an [Issue](https://github.com/MonsterChenzhuo/hadoop-cli/issues) or [Pull Request](https://github.com/MonsterChenzhuo/hadoop-cli/pulls) — for non-trivial changes, start a discussion in an Issue first.

## License

Licensed under the [MIT License](./LICENSE).
