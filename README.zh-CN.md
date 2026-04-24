# hadoop-cli

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-blue.svg)](https://go.dev/)
[![Release](https://img.shields.io/github/v/release/MonsterChenzhuo/hadoop-cli.svg)](https://github.com/MonsterChenzhuo/hadoop-cli/releases)

[中文版](./README.zh-CN.md) | [English](./README.md)

单二进制的 Go CLI，通过无 agent SSH 在多节点 Linux/macOS 环境上引导并管理 HBase 集群（HDFS 单 NameNode + ZooKeeper + HBase）。设计目标是让 [Claude Code](https://claude.com/claude-code) 用一句话驱动整个生命周期。

[安装](#安装) · [升级](#升级) · [快速开始](#快速开始) · [Claude Code](#配合-claude-code-使用) · [命令](#命令) · [快照](#快照与同步) · [组件](#独立部署单个组件) · [范围](#范围v1)

## 为什么选 hadoop-cli？

- **单二进制 + 单 inventory** — `cluster.yaml` 是唯一真实源，目标节点无需安装 agent
- **全生命周期覆盖** — preflight → install → configure → start/stop/status → uninstall，统一 JSON 输出
- **为 Agent 原生设计** — 自带两个 [Claude Code 技能](#配合-claude-code-使用)，Claude 可端到端完成「搭一个 3 节点 HBase 测试集群」
- **可组合组件** — 任选 `{zookeeper, hdfs, hbase}` 的非空子集，可独立部署 ZK 集群或复用外部 HDFS
- **幂等设计** — 反复执行 `install` / `configure` / `start` 是 no-op，安全可重试
- **机器友好** — 每条命令 stdout 输出一段 JSON envelope，stderr 输出人类可读进度

## 安装

### 环境要求

- 目标节点：Linux 或 macOS，可从控制机通过 SSH 访问
- 每个节点预先就绪：JDK、`/etc/hosts`、`cluster.user`
- 控制机：`curl`、`tar`、`bash`（一键安装）；仅源码构建需要 Go `v1.23`+

### 方式一 — 一键安装（推荐）

```bash
curl -fsSL https://raw.githubusercontent.com/MonsterChenzhuo/hadoop-cli/main/scripts/install.sh | bash
```

脚本会自动识别 OS/arch（linux/darwin × amd64/arm64），下载最新 release、校验 checksum，将二进制安装到 `/usr/local/bin/hadoop-cli`，并把内置 skills 解压到 `~/.hadoop-cli/skills/`。

通过环境变量可定制：

```bash
# 指定版本
curl -fsSL https://raw.githubusercontent.com/MonsterChenzhuo/hadoop-cli/main/scripts/install.sh | VERSION=v0.1.2 bash

# 安装到用户目录（不需要 sudo）
curl -fsSL https://raw.githubusercontent.com/MonsterChenzhuo/hadoop-cli/main/scripts/install.sh | PREFIX=$HOME/.local/bin NO_SUDO=1 bash
```

### 方式二 — 下载 release 压缩包

从 [GitHub Releases](https://github.com/MonsterChenzhuo/hadoop-cli/releases) 选择对应平台：

```bash
VER=v0.1.2
OS=linux   # 或 darwin
ARCH=amd64 # 或 arm64
curl -fsSL -o hadoop-cli.tar.gz \
  "https://github.com/MonsterChenzhuo/hadoop-cli/releases/download/${VER}/hadoop-cli_${VER#v}_${OS}_${ARCH}.tar.gz"
tar -xzf hadoop-cli.tar.gz
sudo install -m 0755 hadoop-cli /usr/local/bin/
```

### 方式三 — 从源码构建

需要 Go `v1.23`+。

```bash
git clone https://github.com/MonsterChenzhuo/hadoop-cli.git
cd hadoop-cli
make build
sudo install bin/hadoop-cli /usr/local/bin/
```

### 验证

```bash
hadoop-cli --version
hadoop-cli --help
```

## 升级

安装脚本是幂等的——**再执行一次同样的一键命令**即可升级到最新版：

```bash
curl -fsSL https://raw.githubusercontent.com/MonsterChenzhuo/hadoop-cli/main/scripts/install.sh | bash
```

脚本会原地覆盖 `hadoop-cli` 并刷新 `~/.hadoop-cli/skills/`。你的 `cluster.yaml`、运行日志（`~/.hadoop-cli/runs/`）、已缓存的安装包（`~/.hadoop-cli/packages/`）都不会被动。

如果是源码安装：

```bash
cd hadoop-cli
git pull
make build
sudo install bin/hadoop-cli /usr/local/bin/
```

## 快速开始

1. **写一份 `cluster.yaml`** — 参考 [`skills/hbase-cluster-bootstrap/references/examples/`](./skills/hbase-cluster-bootstrap/references/examples/) 下的示例。把它放在当前目录，或保存为 `~/.hadoop-cli/cluster.yaml`，之后所有命令都不需要再带 `--inventory`。
2. **确认 SSH 可达** — 对每个 `hosts:` 下的节点执行 `ssh -i ~/.ssh/id_rsa hadoop@node1 true`。
3. **引导集群**：

   ```bash
   hadoop-cli preflight    # JDK / 端口 / 磁盘 / 时钟检查
   hadoop-cli install      # 下载、分发、解压 tarball
   hadoop-cli configure    # 渲染并推送配置文件
   hadoop-cli start        # 按 ZK → HDFS → HBase 顺序启动
   hadoop-cli status       # 在每台主机上检查进程
   ```

inventory 的查找顺序：`--inventory <path>` → `$HADOOPCLI_INVENTORY` → `./cluster.yaml` → `~/.hadoop-cli/cluster.yaml`。解析结果会在 stderr 打印一行 `using inventory: …`，并回填到 JSON envelope 的 `inventory_path` 字段。

每条命令在 stdout 输出一段 JSON envelope（稳定字段：`command`、`ok`、`summary`、`hosts`、`error`、`run_id`、`inventory_path`），在 stderr 输出人类可读的进度。每次运行的详细日志位于 `~/.hadoop-cli/runs/<run-id>/`。

## 配合 Claude Code 使用

发布包自带两个技能，注册后 Claude Code 可以端到端驱动 `hadoop-cli`。

```bash
# 一键安装后 skills 已在 ~/.hadoop-cli/skills/，注册给 Claude Code：
claude code skills install ~/.hadoop-cli/skills/hbase-cluster-bootstrap
claude code skills install ~/.hadoop-cli/skills/hbase-cluster-ops
```

然后直接对 Claude 说比如 **「搭一个 3 节点 HBase 测试集群」**——它会读取 skill、生成 `cluster.yaml`，并依次执行 `hadoop-cli preflight → install → configure → start → status`。

| Skill                        | 说明                                                  |
| ---------------------------- | ----------------------------------------------------- |
| `hbase-cluster-bootstrap`    | 生成 `cluster.yaml` 并完成首次引导                   |
| `hbase-cluster-ops`          | 日常运维：状态检查、快照、导出、卸载                 |

## 命令

| 命令              | 作用                                                                   |
| ----------------- | ---------------------------------------------------------------------- |
| `preflight`       | JDK / 端口 / 磁盘 / 时钟检查                                           |
| `install`         | 下载、分发、解压 tarball                                               |
| `configure`       | 渲染并推送配置文件                                                     |
| `start`           | 按 ZK → HDFS → HBase 顺序启动                                          |
| `stop`            | 反向停止                                                               |
| `status`          | 在每台主机上检查进程                                                   |
| `uninstall`       | 停止并移除 `install_dir`（`--purge-data` 同时清空 `data_dir`）         |
| `snapshot`        | 通过 `hbase shell` 打在线快照                                          |
| `export-snapshot` | 通过 `hbase ExportSnapshot` 把快照同步到远端 HDFS                      |

用 `--component zookeeper,hdfs,hbase` 可将命令限定在 inventory 所声明组件的子集上。

## 快照与同步

```bash
# 创建快照
hadoop-cli snapshot \
    --table rta:tag_by_uid --name rta_tag_by_uid_1030

# 同步到远端 HDFS
hadoop-cli export-snapshot \
    --name rta_tag_by_uid_1030 --to hdfs://10.57.1.211:8020/hbase

# 用目标集群 inventory 推导地址
hadoop-cli export-snapshot --inventory src.yaml \
    --name rta_tag_by_uid_1030 --to-inventory dst.yaml
```

详细说明见 [docs/snapshot.zh-CN.md](docs/snapshot.zh-CN.md)（中文）/ [docs/snapshot.md](docs/snapshot.md)（English）。

## 独立部署单个组件

通过 inventory 的 `cluster.components` 字段可以任选 `{zookeeper, hdfs, hbase}` 的非空子集。不写这个字段时默认部署整套，老的 inventory 不受影响。

依赖规则：

- `hbase` 要求同一份 inventory 里有 `zookeeper`（HBase 依赖 ZK quorum）。
- `hbase` 不带 `hdfs` 时，必须显式设置 `overrides.hbase.root_dir`，指向外部 HDFS（或兼容的存储）。
- `hdfs` 和 `zookeeper` 在 v1 中没有依赖（单 NameNode HDFS）。

只会校验当前 inventory 声明的组件所需的 roles/versions——只部署 ZooKeeper 的 inventory 不需要写 `versions.hadoop`、`roles.namenode` 等字段。

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

更多示例见 `skills/hbase-cluster-bootstrap/references/examples/` 下的 `zookeeper-only`、`hdfs-only` 和全栈 inventory。

## 范围（v1）

- 仅单 NameNode，不支持 HDFS HA。
- 只安装 Hadoop / ZooKeeper / HBase。JDK、`/etc/hosts`、OS 用户需要提前准备好。
- 目标节点为 Linux / macOS。

## 贡献

欢迎社区贡献。请提交 [Issue](https://github.com/MonsterChenzhuo/hadoop-cli/issues) 或 [Pull Request](https://github.com/MonsterChenzhuo/hadoop-cli/pulls)——对于较大改动，建议先在 Issue 中讨论。

## 许可证

本项目基于 [MIT License](./LICENSE) 开源。
