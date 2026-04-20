# hadoop-cli 设计文档

- **日期**：2026-04-20
- **状态**：Draft（待 review）
- **目标读者**：hadoop-cli 实现者、Claude Code 使用者
- **范围**：v1 —— HBase + HDFS（单 NN） + ZooKeeper 的多节点集群一键搭建 CLI

## 1. 目标与非目标

### 1.1 目标

1. 用一个静态编译的 Go 二进制 `hadoop-cli`，在**多节点真集群**（物理机 / 虚拟机）上完成 HDFS + ZooKeeper + HBase 的**全生命周期管理**：preflight、install、configure、start、stop、status、uninstall。
2. 通过**结构化输出**（stdout JSON + stderr 进度 + 稳定 error code）和配套 **Claude Code Skills**，让 Claude 通过"一行命令"把集群拉起来、停掉、或清理干净。
3. 控制机、目标节点**零运行时依赖**（除 SSH 与 JDK 外），安装包由 CLI 自动从 Apache 镜像下载并校验。

### 1.2 非目标（v1 不做）

- HDFS HA（双 NN / JournalNode / ZKFC）
- 自动安装 JDK、写 `/etc/hosts`、创建系统用户等 OS 级改动（改由 preflight 检查 + 报错提示）
- 滚动升级、滚动扩容、备份恢复
- Kerberos / TLS / 细粒度权限
- YARN / MapReduce / Hive / Spark / Kafka 等其他组件
- Windows 目标节点支持（Linux / macOS only）

## 2. 技术栈

- **语言**：Go ≥ 1.23
- **CLI 框架**：Cobra（与 lark-cli 保持一致）
- **SSH / 文件传输**：`golang.org/x/crypto/ssh`、`github.com/pkg/sftp`
- **配置渲染**：`text/template` + `encoding/xml`
- **分发**：goreleaser 预编译 linux/amd64、linux/arm64、darwin/amd64、darwin/arm64
- **目标节点**：Linux / macOS，已安装 JDK 8 或 11，已完成控制机到目标节点的 SSH 免密

## 3. 顶层架构

```
hadoop-cli（单二进制，仅在控制机运行）
├── cmd/                    Cobra 命令入口：
│                             preflight / install / configure / start / stop / status / uninstall
├── internal/
│   ├── inventory/          解析 cluster.yaml，校验角色/主机引用一致性
│   ├── orchestrator/       并发 SSH 执行器（goroutine pool + 每主机结果聚合）
│   ├── ssh/                crypto/ssh 封装：exec、sftp、sudo、连接池
│   ├── packages/           tarball 下载 + SHA-512 校验 + 本地缓存
│   ├── render/             配置渲染（text/template → xml/properties/env）
│   ├── components/
│   │   ├── zookeeper/
│   │   ├── hdfs/
│   │   └── hbase/          每个组件实现统一接口：Install/Configure/Start/Stop/Status/Uninstall
│   ├── preflight/          JDK / SSH / hostname / 端口 / 磁盘 / 时钟检查器
│   ├── output/             结构化错误 + stdout JSON / stderr 进度（照搬 lark-cli）
│   └── errs/               错误码定义 + 固定 hint
├── skills/                 Claude Code skills（见 §9）
├── docs/
├── .claude-plugin/         Claude Code 插件清单
└── main.go
```

### 3.1 顶层调用流（install 为例）

```
install 命令
  → inventory.Load(cluster.yaml)
  → preflight.Run(inv)                  // 若失败结构化报错退出
  → packages.EnsureLocal(versions)      // 控制机缓存 tarball，SHA-512 校验
  → orchestrator.Run(hosts, scpTask)    // 并发 sftp 到各节点 $install_dir/.cache/
  → orchestrator.Run(hosts, extractTask)// 并发解压
  → 写 run 记录到 ~/.hadoop-cli/runs/<id>/
```

## 4. Inventory Schema（cluster.yaml）

唯一权威输入。CLI 所有命令都接 `--inventory cluster.yaml`。

### 4.1 Schema

```yaml
cluster:
  name: hbase-dev
  install_dir: /opt/hadoop-cli        # 所有组件解压到这里
  data_dir: /data/hadoop-cli          # data / logs / pid 根目录
  user: hadoop                        # 远端执行账号
  java_home: /usr/lib/jvm/java-11     # preflight 校验用

versions:
  hadoop: 3.3.6
  zookeeper: 3.8.4
  hbase: 2.5.8

ssh:
  port: 22
  user: hadoop
  private_key: ~/.ssh/id_rsa
  parallelism: 8
  sudo: false                         # 远端是否以 sudo 执行

hosts:
  - { name: node1, address: 10.0.0.11 }
  - { name: node2, address: 10.0.0.12 }
  - { name: node3, address: 10.0.0.13 }

roles:                                # 角色 → 节点（引用 hosts.name）
  namenode:     [node1]
  datanode:     [node1, node2, node3]
  zookeeper:    [node1, node2, node3]
  hbase_master: [node1]
  regionserver: [node1, node2, node3]

overrides:                            # 可选；不写即用内置默认
  hdfs:
    replication: 2
    namenode_heap: 1g
    datanode_heap: 1g
  zookeeper:
    client_port: 2181
    tick_time: 2000
  hbase:
    master_heap: 1g
    regionserver_heap: 2g
    root_dir: hdfs://node1:8020/hbase
```

### 4.2 校验规则（v1）

- `roles.namenode` 必须恰好 1 个主机（单 NN）
- `roles.zookeeper` 主机数必须为奇数（1 / 3 / 5）
- `roles.*` 引用的所有 `name` 必须在 `hosts` 中存在
- `cluster.install_dir` 与 `cluster.data_dir` 必须是绝对路径
- `versions.*` 必须在一份内置支持矩阵内（至少覆盖 Hadoop 3.3.x、ZK 3.8.x、HBase 2.5.x）

未指定 `overrides` 时，所有端口、堆、副本、数据目录全部走内置默认。

## 5. 命令行接口

| 命令 | 作用 | 关键 flag |
|---|---|---|
| `hadoop-cli preflight` | 连通性、JDK、hostname、端口、磁盘、时钟漂移 | `--inventory`、`--component` |
| `hadoop-cli install` | 下载、分发、解压 tarball，写 env | `--inventory`、`--component`、`--skip-download`（用本地缓存） |
| `hadoop-cli configure` | 渲染并下发配置文件 | `--inventory`、`--component` |
| `hadoop-cli start` | 按依赖顺序启动 | `--inventory`、`--component`、`--force-format`（仅对 NN 有效） |
| `hadoop-cli stop` | 反向顺序停止 | `--inventory`、`--component` |
| `hadoop-cli status` | 各节点各角色健康状态 | `--inventory`、`--component`、`--json`（默认开） |
| `hadoop-cli uninstall` | 停进程 + 删 install_dir | `--inventory`、`--purge-data`（额外删 data_dir） |

全局 flag：`--run-id`、`--log-level`、`--no-color`。

## 6. 组件职责

### 6.1 ZooKeeper

- **install**：解压 `apache-zookeeper-<v>-bin.tar.gz` 到 `$install_dir/zookeeper`；在每台 ZK 节点的 `dataDir` 写 `myid`（按 `roles.zookeeper` 顺序编号 1..N）
- **configure**：渲染 `conf/zoo.cfg`、`conf/zookeeper-env.sh`、`conf/log4j.properties`
- **start**：各节点并发 `bin/zkServer.sh start`
- **stop**：`bin/zkServer.sh stop`
- **status**：`bin/zkServer.sh status` 解析 leader / follower / standalone

### 6.2 HDFS（单 NameNode）

- **install**：解压 `hadoop-<v>.tar.gz` 到 `$install_dir/hadoop`
- **configure**：渲染 `etc/hadoop/core-site.xml`、`hdfs-site.xml`、`hadoop-env.sh`、`workers`
- **start**：
  - NameNode 首次启动前 `hdfs namenode -format`（带 `.formatted` marker，幂等；`--force-format` 才会重来）
  - NameNode 节点 `hdfs --daemon start namenode`
  - DataNode 节点并发 `hdfs --daemon start datanode`
- **stop**：`hdfs --daemon stop …`
- **status**：`jps` + `http://<nn>:9870/jmx` 解析 live/dead DataNode 数量

### 6.3 HBase

- **install**：解压 `hbase-<v>-bin.tar.gz` 到 `$install_dir/hbase`
- **configure**：渲染 `conf/hbase-site.xml`、`conf/hbase-env.sh`、`conf/regionservers`、`conf/backup-masters`（本版固定为空）；`hbase.rootdir` 默认 `hdfs://<namenode>:8020/hbase`
- **start**：HMaster 节点 `bin/hbase-daemon.sh start master` → RegionServer 并发 `bin/hbase-daemon.sh start regionserver`
- **stop**：`bin/hbase-daemon.sh stop …`
- **status**：`jps` + `http://<master>:16010/jmx` 解析 live RegionServer 数

### 6.4 启停顺序（orchestrator 内强约束）

```
start: ZK(all) → wait quorum → NN(format if needed) → NN start → DN start → HMaster → RegionServer
stop : RegionServer → HMaster → DN → NN → ZK
```

## 7. 幂等性

- **install**：远端先比对 tarball SHA-512，一致则跳过传输和解压
- **configure**：渲染后 `diff` 远端现有文件，内容一致则不落盘
- **start**：先 `jps` 确认未启动再启；重复跑不报错，只日志跳过
- **NameNode 格式化**：写 `$data_dir/.formatted` marker，命令行必须 `--force-format` 才会重新格式化
- **uninstall**：默认保留 `data_dir`，`--purge-data` 才一并删除

## 8. SSH 编排 + 错误/输出模型

### 8.1 SSH 编排

- 每主机 1–2 条长连接（连接池），`sftp` 与 `exec` 共用
- 所有多主机操作统一原语：

  ```go
  type Task struct {
      Name  string
      Cmd   string       // 远端 shell 命令
      Files []FileXfer   // 可选：sftp 上传文件
  }
  type Result struct {
      Host      string
      Stdout    string
      Stderr    string
      ExitCode  int
      Err       error
      Elapsed   time.Duration
  }
  Run(inv *Inventory, hosts []string, task Task, parallelism int) []Result
  ```

- `parallelism` 默认 8，可被 `ssh.parallelism` 覆盖；单主机失败不中断其他主机，最后汇总
- 单任务默认超时 300s；长任务（下载、format）单独覆盖
- 重试：仅对连接类错误（TCP reset、auth 后断开）重试 2 次；业务非零退出不重试

### 8.2 文件传输

- tarball 由控制机下载到 `~/.hadoop-cli/cache/`（SHA-512 校验），再并发 sftp 到各节点 `$install_dir/.cache/`
- 配置文件：控制机本地 render 到 `/tmp/hadoop-cli-<run-id>/<host>/…`，sftp 推送，远端原子 mv 替换

### 8.3 输出模型

- **stdout = 单条 JSON 信封**：

  ```json
  {
    "command": "install",
    "ok": true,
    "summary": { "hosts_total": 3, "hosts_ok": 3, "elapsed_ms": 48213 },
    "hosts": [
      { "host": "node1", "ok": true, "elapsed_ms": 12034 },
      { "host": "node2", "ok": true, "elapsed_ms": 13102 },
      { "host": "node3", "ok": true, "elapsed_ms": 11988 }
    ]
  }
  ```

  失败：

  ```json
  {
    "command": "install",
    "ok": false,
    "error": {
      "code": "SSH_AUTH_FAILED",
      "host": "node2",
      "message": "public key authentication failed",
      "hint": "check ssh.private_key in inventory, or run `ssh-copy-id` to node2"
    }
  }
  ```

- **stderr = 人读进度**：`[node2] extracting hbase-2.5.8-bin.tar.gz … ok (3.2s)`
- **退出码**：
  - `0` 成功
  - `1` 业务失败（至少一台节点失败）
  - `2` 配置/参数错误（inventory 校验失败）
  - `3` preflight 失败
  - `70+` 保留未来

### 8.4 错误分类（`internal/errs`）

每个 code 配固定 `hint`，便于 Claude 自动修复：

| Code | 场景 |
|---|---|
| `SSH_CONNECT_FAILED` | TCP / auth 前失败 |
| `SSH_AUTH_FAILED` | 公钥认证失败 |
| `PREFLIGHT_JDK_MISSING` | JAVA_HOME 无效 |
| `PREFLIGHT_PORT_BUSY` | 端口被占用 |
| `PREFLIGHT_HOSTNAME_UNRESOLVABLE` | 主机名互通失败 |
| `DOWNLOAD_FAILED` | tarball 下载失败 |
| `DOWNLOAD_CHECKSUM_MISMATCH` | SHA-512 不匹配 |
| `CONFIG_RENDER_FAILED` | 模板渲染错误 |
| `REMOTE_COMMAND_FAILED` | 远端非零退出 |
| `TIMEOUT` | 任务超时 |
| `INVENTORY_INVALID` | YAML 校验失败 |

### 8.5 运行记录

每次 run 在 `~/.hadoop-cli/runs/<run-id>/` 下保存：

- `inventory.yaml`（快照）
- `rendered/<host>/…`（本次下发的所有配置文件）
- `<host>.stdout`、`<host>.stderr`
- `result.json`（聚合输出）

`status` 和错误输出都会打印最近 run-id，便于复盘。

## 9. Claude Code Skills

放在 `hadoop-cli/skills/` 下，沿用 lark-cli 的 `SKILL.md` + `references/` 结构。v1 交付两个 skill。

### 9.1 `hbase-cluster-bootstrap`

```
skills/hbase-cluster-bootstrap/
  SKILL.md
  references/
    inventory-schema.md        # cluster.yaml 字段全表 + 校验规则
    bootstrap-runbook.md       # preflight → install → configure → start 标准流程
    error-codes.md             # 所有错误码 + 建议修复
    examples/
      3-node-dev.yaml
      single-host.yaml
```

SKILL.md frontmatter：

```yaml
---
name: hbase-cluster-bootstrap
version: 1.0.0
description: "基于 hadoop-cli 搭建 HBase 集群（含 HDFS、ZooKeeper）。当用户说'帮我搭一个 HBase 集群'/'在这几台机器上部署 HBase'/'快速起一个测试集群'时使用。覆盖 inventory 生成、preflight、install、configure、start 的端到端流程。"
metadata:
  requires:
    bins: ["hadoop-cli"]
  cliHelp: "hadoop-cli --help"
---
```

Body 要点：

- 先决条件（SSH 免密、JDK、/etc/hosts 互通）
- 标准六步流程，每步给命令 + 预期 JSON 字段
- 常见坑（ZK 奇数、单 NN、首次 start 自动 format、install 幂等）

### 9.2 `hbase-cluster-ops`

```
skills/hbase-cluster-ops/
  SKILL.md
  references/
    status-output.md           # status JSON 字段含义 + 健康判断
    stop-start-ordering.md     # 启停顺序约束
    uninstall-guide.md         # uninstall 与 --purge-data 风险
    troubleshooting.md         # 常见故障码映射
```

description 聚焦"查状态 / 重启 / 卸载"，触发词示例："检查集群健康"、"重启 hbase"、"彻底清掉这个集群"。

### 9.3 集成方式

- 仓库根 `.claude-plugin/` 清单（参考 lark-cli 的 npm 包）
- 用户安装：`npx skills add yourorg/hadoop-cli -y -g`
- CI 加 skill lint：frontmatter 合法性、SKILL.md 中示例命令可 dry-run

## 10. 里程碑（供后续 plan 切分）

1. 工程骨架：Go module、Cobra、Makefile、lint、goreleaser 雏形、`output`/`errs` 包
2. Inventory 解析与校验
3. SSH / orchestrator / sftp 基建 + 并发执行器 + run 记录
4. packages 下载与缓存 + SHA-512 校验
5. ZooKeeper 组件：install / configure / start / stop / status
6. HDFS 组件：同上（含首次 format 幂等）
7. HBase 组件：同上
8. preflight 检查器
9. uninstall（含 --purge-data）
10. Skills：`hbase-cluster-bootstrap` + `hbase-cluster-ops` + 样例 inventory + CI skill lint
11. README、examples、goreleaser release workflow

## 11. 开放问题（进入 plan 前需明确）

目前无。如实现过程发现新问题再回补。
