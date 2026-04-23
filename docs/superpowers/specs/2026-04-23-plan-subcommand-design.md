# `hadoop-cli plan` + facts 安全门 — 设计

日期：2026-04-23
作者：zhuoyuchen
状态：Draft（待用户 review）

## 1. 背景与动机

现在 hadoop-cli 的标准流程是 `preflight → install → configure → start`。`preflight` 做 pass/fail 的健康检查，但不产出"目标机器当前是什么状态"的事实，也不会把"接下来会在每台机器上做什么"摊开来给人 review。

当 Claude Code 通过 skill 驱动部署时，这一层"执行前的上下文"只能靠 Claude 自己脑补，容易出现：
- inventory 里声明的东西在目标机器上已经部分存在，但没人提前发现。
- 磁盘/端口/JDK 的问题在 `install` 跑到一半才暴露，留下残局。
- 执行前没有可 review 的"将要做什么"清单，自动化里也缺 gate。

本设计为 hadoop-cli 新增一个 `plan` 子命令，让"事实采集 + 计划生成 + 风险评估"成为执行前的显式一步，并让后续的 `install/configure/start` 以 facts 为安全门。

## 2. 目标与非目标

### 目标
- 新增 `hadoop-cli plan --inventory <path>` 子命令，产出：
  - 每台机器的事实（facts），read-only SSH 采集。
  - 按 `install / configure / start` 分阶段的执行动作清单，附风险标注。
  - 阻断项（blocker）和警告（warning）列表。
  - JSON envelope（stdout）+ 人类可读渲染（stderr），退出码 = `blocker 数 > 0 ? 1 : 0`。
- facts 作为 `plan` 的副产物写入 run 目录和一个稳定指针；后续命令默认读它做 blocker 安全门。
- `install/configure/start` 在 facts 缺失/过期/有未解决 blocker 时默认拒绝执行，`--force` 旁路。
- 更新 `hbase-cluster-bootstrap` skill，把 `plan` 放进标准流程。

### 非目标（后续版本再考虑）
- 基于 facts 反推/生成 inventory（本期只校验已有 inventory，不产生）。
- 用 facts 做 install/configure 的幂等短路（现有幂等逻辑已足够）。
- 多集群并行 plan、HA NameNode、在线升级 plan。
- 把 `preflight` 并入或替换为 `plan`（两者并存，定位不同）。

## 3. 关键决策（已与用户对齐）

| 决策点 | 选择 | 理由 |
|---|---|---|
| 一个还是两个命令 | 一个 `plan` 命令，facts 作副产物 | 符合现有"单命令 + run 目录留痕"风格 |
| facts 对后续命令的作用 | 安全门（拒绝执行），`--force` 旁路 | 让 plan 真正起到护栏作用；熟手/CI 能绕过 |
| plan 输出形式 | 按 install/configure/start 分 phase 的 action 清单 + blockers + warnings | 用户能直接 review 执行路径，CI 可用退出码 gate |
| facts 存放 | `runs/<id>/facts.json` + `facts/<inv-sha>.json` 稳定指针 | inventory 一变 → 指针自动失效 |
| facts freshness | TTL 30 分钟，`HADOOP_CLI_FACTS_TTL` 覆写 | 足够完成一次 bootstrap，过期则重采 |
| 和 `preflight` 的关系 | 并存，plan 复用 preflight 的检查代码 | 零回归风险，`preflight` 仍是轻量健康检查 |
| 采集事实范围 | 主机级 + 集群状态级 + 按 components 条件采集的外部依赖 | 覆盖常见翻车点 |

## 4. 架构概览

```
 ┌──────────────┐           ┌──────────────────┐
 │ cluster.yaml │──────────▶│ hadoop-cli plan  │
 └──────────────┘           │                  │
         ▲                   │  collect facts   │──▶ SSH 每台 host
         │                   │  compute phases  │
         │                   │  evaluate block  │
         │                   └────────┬─────────┘
         │                            │
         │        ┌───────────────────┼──────────────────┐
         │        ▼                   ▼                  ▼
         │  runs/<id>/facts.json  runs/<id>/plan.json  stdout envelope + stderr 人类输出
         │        │                                      │
         │        └──────────┐                           ▼
         │                   ▼                       用户 review
         │       facts/<inv-sha>.json（稳定指针）
         │                   │
         │                   │  TTL 30min
         │                   ▼
 ┌───────┴──────┐    ┌──────────────┐
 │ install/     │───▶│ load facts   │
 │ configure/   │    │ gate check   │ ── 缺失/过期/有 blocker 且无 --force ─▶ 拒绝
 │ start        │    └──────────────┘
 └──────────────┘
```

## 5. `plan` 子命令规格

### 输入
- `--inventory <path>`（必填，与其它命令一致）
- `--component <name>`（可选，过滤 `zookeeper|hdfs|hbase`；过滤后 facts 仍按整个 inventory 采，但 phases 里只列目标组件相关的 action）
- `--output human|json|both`（默认 `both`）

### 流程
1. `inventory.Load` + `inventory.Validate`。
2. 并发 SSH 采集事实（只读命令）。
3. 基于 facts + inventory 计算每 phase 的 action 列表。
4. 评估 blocker / warning。
5. 写 `runs/<id>/facts.json`、`runs/<id>/plan.json`、`facts/<inv-sha>.json`。
6. 输出 JSON envelope（stdout）+ 人类可读渲染（stderr）。
7. 退出码：`blocker 数 == 0 ? 0 : 1`。

`plan` 不产生任何远端变更——只用 read-only 的 SSH 命令（`cat`、`ls`、`df`、`ss`/`netstat`、`jps`、`test -e` 等）。

## 6. 采集的事实

### 主机级（所有机器都采）
| Fact | 采集方式 |
|---|---|
| `os` | `uname -srm` + `/etc/os-release` |
| `jdk` | `$JAVA_HOME/bin/java -version` + 路径 `test -x` |
| `resources.memory_mb` | `/proc/meminfo` 或 `sysctl` |
| `resources.cpu_cores` | `nproc` 或 `sysctl hw.ncpu` |
| `resources.disk_install_mb` | `df -Pm <install_dir-mount>` |
| `resources.disk_data_mb` | `df -Pm <data_dir-mount>` |
| `clock_skew_ms` | SSH 调用前后打点，粗略 drift |
| `hosts_file` | `getent hosts <node>` / `grep <node> /etc/hosts` |
| `user_state` | `id <cluster.user>`、目录所有权、sudo 可用性 |

主机级 1-4 的检查函数复用 `internal/preflight` 现有实现，不另起一套。

### 集群状态级
| Fact | 说明 |
|---|---|
| `installed_pkgs` | `install_dir` 下现有 hadoop/zookeeper/hbase 目录 + 版本（目录名或 `VERSION` 文件推断） |
| `data_state.hdfs_formatted` | `data_dir/hdfs/nn/.formatted` 是否存在 |
| `data_state.zk_myid` | `data_dir/zookeeper/myid` 是否存在，值是多少 |
| `data_state.hbase_wal` | `data_dir/hbase/WALs/` 是否非空 |
| `ports` | 按 `cluster.components` 裁剪的端口列表（NN 8020/9870、DN 9864/9866、ZK 2181/2888/3888、HBase 16000/16020…），用 `ss -ltnp` 或 `lsof` 判断占用进程 |
| `processes` | `jps` 输出里目标进程（NameNode/DataNode/QuorumPeerMain/HMaster/HRegionServer）是否在跑 |

### 外部依赖级（条件采集）
| Fact | 条件 | 说明 |
|---|---|---|
| `external_hdfs_reachable` | `cluster.components` 含 `hbase` 但不含 `hdfs` | 验证 `overrides.hbase.root_dir` 可达 + 写权限 |
| `zk_quorum_mesh` | `cluster.components` 含 `zookeeper` | ZK 节点两两之间 2888/3888 连通性 |

## 7. facts 存储与 freshness

### 文件布局
```
~/.hadoop-cli/
  runs/<run-id>/
    facts.json         # 每次 plan 运行的完整事实快照
    plan.json          # 对应的 plan 输出
    <host>.stdout/stderr  # 已有机制
  facts/
    <inv-sha256>.json  # 稳定指针，给后续命令用
                       # 内容：{ run_id, inventory_sha, collected_at, facts... }
  packages/            # 已有
```

### freshness 规则
- 默认 TTL = **30 分钟**；`HADOOP_CLI_FACTS_TTL` 环境变量可覆写（格式：Go `time.ParseDuration`）。
- key = inventory 文件内容的 sha256；inventory 一改动 → 旧指针不再匹配 → 必须重跑 `plan`。
- 过期或缺失 → 后续命令在无 `--force` 时报对应错误码。

## 8. plan 输出 schema

### JSON envelope（stdout 单行）
```json
{
  "command": "plan",
  "ok": false,
  "run_id": "2026-04-23T14-03-11Z-ab12",
  "inventory_sha": "ab12cd34...",
  "summary": {"blockers": 1, "warnings": 2, "actions": 14},
  "facts_path": "/Users/.../runs/<id>/facts.json",
  "phases": [
    {
      "name": "install",
      "actions": [
        {
          "id": "install.hadoop.node1",
          "hosts": ["node1"],
          "description": "download + extract hadoop-3.3.6",
          "skip_reason": null,
          "risk": "low"
        }
      ]
    },
    {"name": "configure", "actions": [...]},
    {"name": "start",     "actions": [...]}
  ],
  "blockers": [
    {
      "code": "DISK_TOO_SMALL",
      "host": "node2",
      "message": "data_dir /data has 12GB free, inventory expects ≥ 50GB",
      "hint": "扩容挂载点或换 data_dir 后重跑 plan"
    }
  ],
  "warnings": [
    {
      "code": "EXISTING_ZK_RUNNING",
      "host": "node1",
      "message": "QuorumPeerMain pid 1234 already running; install will stop it"
    }
  ]
}
```

### 人类可读渲染（stderr）
按 phase 列表格，每行一个 action，末尾汇总 blocker/warning：

```
Phase: install
  [skip] node1         hadoop-3.3.6 already present (sha match)
  [run]  node1..node3  download + extract hadoop-3.3.6
  [run]  node1..node3  download + extract zookeeper-3.8.4

Phase: configure
  [run]  node1         render core-site.xml (3 keys changed)
  ...

Phase: start
  [run]  node1 (NN)    format HDFS  ⚠ destructive, data_dir empty → safe
  [run]  node1..node3  start DataNode

Blockers (1):
  [DISK_TOO_SMALL] node2: data_dir /data has 12GB free, need ≥ 50GB
    → 扩容挂载点或换 data_dir 后重跑 plan

Warnings (2):
  [EXISTING_ZK_RUNNING] node1: QuorumPeerMain pid 1234 already running; install will stop it
  ...
```

## 9. Blocker / Warning 清单（v1）

### Blockers（拒绝执行）
| code | 触发条件 |
|---|---|
| `DISK_TOO_SMALL` | `data_dir` 或 `install_dir` 挂载点剩余 < 阈值（默认 data 50GB / install 5GB；inventory 可覆写） |
| `JDK_MISSING` | `cluster.java_home` 路径不存在或无 `bin/java` |
| `PORT_OCCUPIED_BY_FOREIGN` | 目标端口被非本集群进程占用 |
| `DATA_DIRTY_NOT_FORCED` | HDFS `.formatted` 存在且 inventory 未声明保留、未带 `--force-format` |
| `EXTERNAL_HDFS_UNREACHABLE` | 只部 HBase 时 `overrides.hbase.root_dir` 不可达或无写权限 |
| `USER_MISSING` | `cluster.user` 不存在且无 sudo |
| `HOSTS_INCONSISTENT` | inventory 节点间 `/etc/hosts` 解析不一致 |

### Warnings（标注但不拒绝）
| code | 说明 |
|---|---|
| `EXISTING_ZK_RUNNING` / `EXISTING_NN_RUNNING` / `EXISTING_HBASE_RUNNING` | 目标进程已在跑，install 会先 stop |
| `PARTIAL_INSTALL` | `install_dir` 下有部分组件版本与 inventory 对不上 |
| `CLOCK_SKEW_OVER_2S` | 相对控制机时钟偏差 > 2s |

## 10. install/configure/start 的安全门

在 `cmd/common.go` 的 `prepare()` 里，为这三个命令增加安全门步骤。伪代码：

```go
if !force && isGatedCommand(command) {
    facts, err := facts.LoadForInventory(inv)
    switch {
    case errors.Is(err, facts.ErrNotFound):
        return nil, envelopeErr("FACTS_MISSING",
            "no facts found for this inventory; run `hadoop-cli plan --inventory <path>` first")
    case errors.Is(err, facts.ErrStale):
        return nil, envelopeErr("FACTS_STALE",
            "facts older than TTL; rerun `hadoop-cli plan`")
    case err == nil && facts.HasBlockers():
        return nil, envelopeErr("FACTS_HAS_BLOCKERS",
            "plan reported blockers; resolve them and rerun `hadoop-cli plan`")
    }
    env.Facts = facts
}
```

- `--force` flag：跳过安全门。旁路时在 run envelope 里记录 `forced: true`；若当时 facts 存在则附上其新鲜度和 blocker 数，若缺失则明确记录"forced without facts"，便于事后审计。
- 被安全门约束的命令：`install`、`configure`、`start`。
- 不受约束：`preflight`（门的一部分）、`plan`（门本身）、`status`（只读）、`uninstall`（清理场景）、`snapshot` / `export-snapshot`（运维场景）。
- 错误码统一走 envelope：`FACTS_MISSING`、`FACTS_STALE`、`FACTS_HAS_BLOCKERS`。

## 11. 与 `preflight` 的关系

- `preflight` 继续存在，定位不变：轻量、快速的 pass/fail 健康检查，不产生 facts。
- `plan` 在内部复用 `preflight` 已有的检查函数（JDK / 端口 / 磁盘 / 时钟），并扩展出集群状态级和外部依赖级采集。
- 推荐流程：`preflight`（快速筛）→ `plan`（深度评估 + 计划）→ `install/...`。`preflight` 仍是可选前置。

## 12. 代码组织

### 新增
```
internal/
  facts/
    facts.go          # Facts 结构体 + 序列化
    store.go          # 读写 runs/<id>/ 和 facts/<inv-sha>.json
    freshness.go      # TTL 检查 + inv-sha 匹配
    collect.go        # 协调各采集器，生成 Facts
    collect_host.go   # 主机级采集（复用 preflight 的检查函数）
    collect_cluster.go# 集群状态级采集
    collect_deps.go   # 外部依赖级采集
  plan/
    plan.go           # 由 Facts + Inventory 生成 phased action list
    blockers.go       # blocker / warning 评估规则
    render.go         # 人类可读渲染
cmd/
  plan.go             # cobra 入口
```

### 修改
- `cmd/common.go`：`prepare()` 加安全门逻辑 + 读取 `--force`；`components.Env` 增加 `Facts *facts.Facts` 字段。
- `cmd/install.go` / `cmd/configure.go` / `cmd/start.go`：注册 `--force` flag，其余行为不变。
- `cmd/root.go`：注册 `newPlanCmd()`。
- `skills/hbase-cluster-bootstrap/SKILL.md`：把 `plan` 加入标准流程，文档里列明 `--force` 的使用场景。
- README（中英双版）：commands 表加一行 `plan`。

## 13. Skill 流程更新

`skills/hbase-cluster-bootstrap/SKILL.md` 的 "Standard bootstrap flow" 改为：

```
1. generate inventory
2. hadoop-cli preflight            # 可选但推荐
3. hadoop-cli plan                 # 采集事实 + 生成计划
   - 读 JSON envelope 里的 blockers
   - 逐项解决后重跑 plan 直到 ok:true
   - 同步告诉用户 warnings 里值得注意的项
4. hadoop-cli install
5. hadoop-cli configure
6. hadoop-cli start
7. hadoop-cli status
```

同时在 "Common pitfalls" 里补：
- `install/configure/start` 报 `FACTS_MISSING` / `FACTS_STALE` 时请先重跑 `plan`。
- 只有在明确理解风险时（例如 inventory 和现场不会变）才用 `--force`。

## 14. 测试策略

### 单元测试
- `internal/facts`：每种采集器有 happy + edge cases（mock SSH 返回值）。
- `internal/plan`：给定 inventory + 伪造 facts → 生成的 phases / actions / blockers 断言。
- `cmd/common.go` 安全门：缺失 / 过期 / 有 blocker / `--force` 旁路 四条路径。
- freshness：inventory 内容变化后旧指针不再被采纳。

### 集成测试
- 现有 `lifecycle_test.go` 风格扩展：先跑 `plan`、再跑 `install/configure/start` 的完整链路。
- 无 facts 场景下 `install` 报 `FACTS_MISSING` 并能通过 `plan` 恢复。
- 带 `--force` 能旁路安全门。

## 15. 向后兼容性

- 安全门对 `install/configure/start` 是默认行为变化。现有用户 / CI 首次升级会遇到 `FACTS_MISSING`。
- 缓解：release notes 显式告知；skill 更新同步；`--force` 作逃生口；错误消息 hint 里直接给出 `plan` 命令。
- `preflight` / `status` / `snapshot` / `export-snapshot` 行为零变化。

## 16. 风险与后续

- **facts 陈旧但 TTL 内没感知**：机器上某些东西可能在 30min 内变了。缓解：安全门错误消息强调"重跑 plan"，审计里保留 facts_collected_at。
- **SSH 采集耗时**：并发执行 + 只读命令，目标单节点 < 5s。
- **facts.json 体积**：目标 < 50KB/node，100 节点以内可忽略。
- **后续演进**：v2 可考虑用 facts 做 install 的幂等短路、用 facts 反向生成 inventory 建议、跨 run 的 facts diff。
