# pgdba — PostgreSQL 虚拟 DBA 专家系统

`pgdba` 是一套面向生产环境的 PostgreSQL DBA 工具集，将高可用部署、故障切换、备份恢复、监控告警、配置调优、查询分析等操作封装为统一的 CLI 命令，所有输出均为结构化 JSON，可直接供 AI 代理调用。

## 特性概览

- **统一 JSON 输出**：所有命令输出标准信封格式，AI 可直接解析
- **多 Provider 支持**：Docker（已实现）、裸金属 SSH、Kubernetes（规划中）
- **高可用编排**：Patroni REST API 客户端完整实现，etcd×3 + PG 主从 + PgBouncer 完整栈
- **集群注册表**：`~/.pgdba/clusters.json` 持久化，区分 managed / external 集群
- **数据库诊断**：inspect 快照、配置调优、查询分析、基线报告，支持 PG 12-16+
- **安全优先**：密码永远不写入配置文件，只从环境变量读取；容器端口绑定 127.0.0.1
- **TDD 开发**：202 单元测试 + 38 E2E 测试 + 7 集成测试，race detector 全程启用

## 当前状态

| 阶段 | 内容 | 状态 |
|------|------|------|
| 阶段一 | CLI 框架、配置系统、Provider 接口、health check | **已完成** |
| 阶段二 | 集群生命周期（cluster init/status/destroy/connect）、Patroni 客户端、Docker 部署栈 | **已完成** |
| 阶段三 | 故障切换与从库管理（failover trigger/status, replica list/promote） | **已完成** |
| 阶段四 | 配置调优与查询分析（inspect, config tune/show/diff, query top/analyze/locks/bloat, baseline） | **已完成** |
| 阶段五 | 备份与 PITR 恢复 | 规划中 |
| 阶段六 | 物理备份与归档 | 规划中 |
| 阶段七 | 监控与告警（Prometheus + Grafana） | 规划中 |
| 阶段八 | 裸金属部署（Ansible） | 规划中 |
| 阶段九 | Kubernetes 支持（Helm + client-go） | 规划中 |

---

## 快速开始

### 环境要求

- Go 1.22+
- Docker & Docker Compose（用于完整 HA 栈和集成测试）
- PostgreSQL 12+（诊断命令支持 PG 12-16+，部分功能需要更高版本）

### 编译

```bash
git clone https://github.com/jianh619/dba-blooming.git
cd dba-blooming
make build
# 二进制输出到 ./bin/pgdba
```

### 配置

通过环境变量配置连接信息（**密码只能通过环境变量传递**）：

```bash
export PGDBA_PG_HOST=localhost
export PGDBA_PG_PORT=5432
export PGDBA_PG_USER=postgres
export PGDBA_PG_DATABASE=postgres
export PGDBA_PG_PASSWORD=<your-password>   # 只从环境变量读取，不写入任何文件
export PGDBA_PG_SSLMODE=prefer
```

也可通过配置文件（密码不在其中）：

```yaml
# ~/.pgdba/config.yaml
pg:
  host: localhost
  port: 5432
  user: postgres
  database: postgres
  sslmode: prefer

provider:
  type: docker   # docker | baremetal | kubernetes

cluster:
  name: my-cluster
```

---

## 启动本地 HA 集群（Docker Compose）

使用 Zalando Spilo（PostgreSQL + Patroni 一体镜像）+ etcd×3 + PgBouncer：

```bash
cd deployments/docker

# 1. 复制并填写密码（.env 已加入 .gitignore，不会提交）
cp .env.example .env
vim .env            # 填入 POSTGRES_PASSWORD 和 REPLICATION_PASSWORD

# 2. 启动（无需构建镜像，直接拉取公开镜像）
docker compose up -d

# 3. 等待 Patroni 选主（约 30s）
sleep 30

# 4. 查看集群状态
curl http://localhost:8008/cluster | python3 -m json.tool

# 5. 用 pgdba 接管集群
./bin/pgdba cluster connect \
  --name local-ha \
  --patroni-url http://localhost:8008 \
  --pg-host localhost

# 6. 通过 pgdba 查看拓扑
./bin/pgdba cluster status --name local-ha

# 7. health check（通过 PgBouncer 连接池）
PGDBA_PG_HOST=localhost PGDBA_PG_PORT=6432 \
PGDBA_PG_USER=postgres PGDBA_PG_PASSWORD=<your-password> \
./bin/pgdba health check

# 8. 停止并清理
docker compose down -v
```

栈组成：

| 容器 | 镜像 | 端口（127.0.0.1） |
|------|------|-------------------|
| etcd1/2/3 | `quay.io/coreos/etcd:v3.5.9` | 内部 |
| pg-primary | `ghcr.io/zalando/spilo-16:3.3-p3` | 5432, 8008 |
| pg-replica-1 | 同上 | 5433, 8009 |
| pg-replica-2 | 同上 | 5434, 8010 |
| pgbouncer | `edoburu/pgbouncer:v1.25.1-p0` | 6432 |

> 所有端口仅绑定 `127.0.0.1`，不对外暴露。

---

## CLI 用法

### 全局参数

```
--format json|table|yaml    输出格式（默认 json）
--config <path>             配置文件路径（默认 ~/.pgdba/config.yaml）
--provider docker|baremetal|kubernetes  部署 Provider（默认 docker）
--verbose                   启用详细日志
```

### 统一信封格式

所有命令输出均遵循以下结构：

```json
{
  "success": true,
  "timestamp": "2026-02-24T10:00:00Z",
  "command": "health check",
  "data": { ... }
}
```

失败时：

```json
{
  "success": false,
  "timestamp": "2026-02-24T10:00:00Z",
  "command": "cluster connect",
  "error": "Patroni unreachable at http://10.0.0.1:8008: ..."
}
```

---

### 已实现命令

<!-- AUTO-GENERATED: command-reference-start -->

| 命令 | 说明 | 实现阶段 |
|------|------|----------|
| `pgdba health check` | PostgreSQL 健康检查（版本、连接数、复制状态） | 阶段一 |
| `pgdba cluster connect` | 接管已有 Patroni 集群，注册到本地注册表 | 阶段二 |
| `pgdba cluster status` | 查看集群拓扑、成员角色、健康状态 | 阶段二 |
| `pgdba cluster init` | 初始化新集群（框架已实现，待 Provider 集成） | 阶段二 |
| `pgdba cluster destroy` | 销毁 managed 集群（拒绝删除 external 集群） | 阶段二 |
| `pgdba failover trigger` | 触发受控切换或强制故障转移 | 阶段三 |
| `pgdba failover status` | 查看故障切换状态 | 阶段三 |
| `pgdba replica list` | 列出所有从库及复制延迟 | 阶段三 |
| `pgdba replica promote` | 提升指定从库为主库 | 阶段三 |
| `pgdba inspect` | 采集诊断快照（pg_settings, pg_stat_*, identity） | 阶段四 |
| `pgdba config show` | 查看当前 PostgreSQL 配置 | 阶段四 |
| `pgdba config diff` | 对比当前配置与推荐值的差异 | 阶段四 |
| `pgdba config tune` | 生成并可选应用调优建议 | 阶段四 |
| `pgdba query top` | 显示资源消耗最高的查询（pg_stat_statements） | 阶段四 |
| `pgdba query analyze` | 对指定 SQL 运行 EXPLAIN ANALYZE | 阶段四 |
| `pgdba query index-suggest` | 基于表统计信息建议缺失索引 | 阶段四 |
| `pgdba query locks` | 显示活跃锁及等待链 | 阶段四 |
| `pgdba query bloat` | 估算表膨胀（仅用 catalog，无需扩展） | 阶段四 |
| `pgdba query vacuum-health` | 显示 vacuum 状态、死元组、autovacuum 活跃度 | 阶段四 |
| `pgdba baseline collect` | 生成综合基线报告（含调优建议） | 阶段四 |
| `pgdba baseline diff` | 对比两个基线快照的差异 | 阶段四 |

<!-- AUTO-GENERATED: command-reference-end -->

---

### 命令详细用法

#### `pgdba health check`

对已配置的 PostgreSQL 实例执行全面健康检查。

```bash
pgdba health check
pgdba health check --format table
```

#### `pgdba cluster connect / status`

```bash
# 接管已有 Patroni 集群
pgdba cluster connect \
  --name prod-ha \
  --patroni-url http://10.0.0.1:8008 \
  --pg-host 10.0.0.1

# 查看拓扑
pgdba cluster status --name prod-ha
```

#### `pgdba failover trigger`

```bash
# 受控切换 — 自动选择最佳候选
pgdba failover trigger --name prod-ha

# 指定候选节点
pgdba failover trigger --name prod-ha --candidate pg-replica-1

# 强制故障转移 — 主库不可达时使用
pgdba failover trigger --name prod-ha --force --candidate pg-replica-1
```

#### `pgdba replica list / promote`

```bash
pgdba replica list --name prod-ha
pgdba replica promote --name prod-ha --candidate pg-replica-1
```

#### `pgdba inspect`

采集诊断快照，支持 instant 和 delta 两种采样模式。自动检测 PG 版本，降级不可用的数据源（如 PG 12 无 pg_control_system）。

```bash
# 即时快照
pgdba inspect --name local-ha

# Delta 模式（采样两个时间点，计算差值）
pgdba inspect --name local-ha --delta --interval 30s
```

**Identity 三级指纹**：系统优先使用 `pg_control_system()` (PG 13+) 生成稳定指纹，回退到 `inet_server_addr():inet_server_port():datid`，最后回退到配置地址。

#### `pgdba config show / diff / tune`

```bash
# 查看当前配置
pgdba config show --name local-ha

# 对比推荐值（基于 PGTune 启发式）
pgdba config diff --name local-ha --workload oltp --ram-gb 16 --cpu-cores 4 --storage ssd

# 一键调优（生成建议，可选 --apply 或 --dry-run）
pgdba config tune --name local-ha --workload oltp --ram-gb 16 --dry-run
```

**调优参数**：shared_buffers、effective_cache_size、work_mem、maintenance_work_mem、random_page_cost、checkpoint_completion_target、max_connections。

**安全机制**：
- 每个参数检查 `pg_settings.context`（postmaster 需重启、sighup 可 reload）
- 检查当前角色权限（per-parameter permission check）
- 检查 Patroni DCS 覆盖冲突（查询 Patroni `/config` endpoint）
- Apply/Rollback 文件锁互斥（防止并发操作）
- Dry-run 模式预览变更

#### `pgdba query top / analyze / index-suggest / locks / bloat / vacuum-health`

```bash
# 资源消耗 Top N 查询（需要 pg_stat_statements 扩展）
pgdba query top --name local-ha --limit 20 --sort total_time

# SQL 执行计划分析
pgdba query analyze --name local-ha --sql "SELECT * FROM orders WHERE id = 1"

# 缺失索引建议（排除小于 10k 行的小表）
pgdba query index-suggest --name local-ha --min-rows 10000
pgdba query index-suggest --name local-ha --table orders

# 锁等待链
pgdba query locks --name local-ha

# 表膨胀估算（使用 pg catalog，无需额外扩展）
pgdba query bloat --name local-ha

# Vacuum 健康检查
pgdba query vacuum-health --name local-ha
```

#### `pgdba baseline collect / diff`

```bash
# 采集完整基线
pgdba baseline collect --name local-ha --save baseline-before.json

# 应用调优后再采集
pgdba baseline collect --name local-ha --save baseline-after.json

# 对比前后差异
pgdba baseline diff --before baseline-before.json --after baseline-after.json
```

---

### 规划中的命令

```bash
# 备份与恢复
pgdba backup create --type full|logical
pgdba backup restore --backup-id X --target-time "2026-02-24 09:00:00"
pgdba backup list
pgdba backup schedule --cron "0 2 * * *"

# 监控
pgdba monitor setup --prometheus --grafana
pgdba monitor status
pgdba monitor alerts list|add|remove

# 混沌测试（需要 --i-know-what-i-am-doing 标志）
pgdba chaos kill-node --host X --i-know-what-i-am-doing
pgdba chaos partition --isolate X
pgdba chaos report
```

---

## 使用场景

### 场景一：诊断已有 PG 实例（无需 Patroni）

```bash
export PGDBA_PG_HOST=10.0.0.1
export PGDBA_PG_PASSWORD=secret

# 健康检查
pgdba health check

# 诊断快照
pgdba inspect --name my-pg

# 配置调优建议
pgdba config diff --name my-pg --workload oltp --ram-gb 32

# 查询分析
pgdba query top --name my-pg
pgdba query vacuum-health --name my-pg
```

### 场景二：接管已有 Patroni 集群

```bash
pgdba cluster connect \
  --name prod \
  --patroni-url http://10.0.0.1:8008 \
  --pg-host 10.0.0.1

pgdba cluster status --name prod
pgdba failover trigger --name prod
```

### 场景三：本地开发 HA 环境（Docker Compose）

```bash
cd deployments/docker && cp .env.example .env
# 填写密码后：
docker compose up -d
```

### 场景四：配置调优工作流

```bash
# 1. 采集基线
pgdba baseline collect --name prod --save before.json

# 2. 查看调优建议
pgdba config diff --name prod --workload oltp --ram-gb 64

# 3. Dry-run 预览
pgdba config tune --name prod --workload oltp --ram-gb 64 --dry-run

# 4. 应用（需要 superuser 权限）
pgdba config tune --name prod --workload oltp --ram-gb 64 --apply

# 5. 采集调优后基线
pgdba baseline collect --name prod --save after.json

# 6. 对比效果
pgdba baseline diff --before before.json --after after.json
```

---

## 项目结构

```
.
├── cmd/pgdba/main.go              # 程序入口
├── internal/
│   ├── cli/                       # cobra 命令定义
│   │   ├── root.go                # 根命令与全局参数
│   │   ├── health.go              # health check 命令
│   │   ├── cluster.go             # cluster init/status/connect/destroy
│   │   ├── failover.go            # failover trigger/status
│   │   ├── replica.go             # replica list/promote
│   │   ├── inspect.go             # inspect 诊断快照
│   │   ├── config.go              # config show/diff/tune
│   │   ├── query.go               # query top/analyze/index-suggest/locks/bloat/vacuum-health
│   │   └── baseline.go            # baseline collect/diff
│   ├── inspect/                   # 诊断快照核心（Phase 4 新增）
│   │   ├── identity.go            # ClusterIdentity 三级指纹
│   │   ├── types.go               # DiagSnapshot, ChangeSet, SamplingConfig 等
│   │   ├── collector.go           # 版本感知的诊断数据采集器
│   │   ├── db.go                  # DB 接口 + PGSetting/PGSSRow 等数据类型
│   │   ├── pgxdb.go               # pgx 实现（真实数据库适配器）
│   │   └── lock.go                # Apply/Rollback 文件锁互斥
│   ├── tuning/                    # 配置调优引擎（Phase 4 新增）
│   │   ├── engine.go              # PGTune 启发式推荐 + 置信度 + Rationale
│   │   └── apply.go               # DryRun / Apply / Rollback 安全管线
│   ├── query/                     # 查询分析（Phase 4 新增）
│   │   ├── types.go               # TopQuery, LockInfo, TableBloat 等
│   │   └── analysis.go            # SuggestIndexes, BuildLockChains
│   ├── config/                    # 配置加载（viper + 环境变量）
│   │   ├── config.go
│   │   └── defaults.go
│   ├── output/                    # 统一输出信封与格式化器
│   │   ├── types.go
│   │   └── formatter.go
│   ├── patroni/                   # Patroni REST API 客户端
│   │   ├── client.go              # GetClusterStatus、Switchover 等
│   │   ├── config.go              # patroni.yml 模板渲染
│   │   └── templates/             # Go embed 模板
│   ├── cluster/                   # 集群注册表
│   │   └── registry.go            # ~/.pgdba/clusters.json CRUD
│   ├── pgbouncer/                 # PgBouncer 配置生成
│   │   └── config.go
│   ├── postgres/                  # PostgreSQL 连接管理
│   │   └── conn.go
│   ├── failover/                  # 故障切换预检逻辑
│   │   └── precheck.go
│   └── provider/                  # 部署平台抽象层
│       ├── provider.go
│       └── docker.go
├── deployments/docker/            # 完整 HA 部署栈
│   ├── docker-compose.yml         # etcd×3 + Spilo×3 + PgBouncer
│   ├── .env.example               # 环境变量模板
│   ├── Dockerfile.postgres        # 自建镜像方案（备选）
│   └── patroni-entrypoint.sh      # 自建镜像启动脚本（备选）
├── tests/
│   ├── unit/                      # 单元测试（202 个）
│   ├── e2e/                       # E2E 测试（46 个，黑盒测试二进制）
│   └── integration/               # 集成测试（7 个，需 Docker Compose）
├── docs/
│   ├── CONTRIBUTING.md            # 贡献指南（开发环境、TDD 流程、PR 检查清单）
│   └── RUNBOOK.md                 # 运维手册（部署、操作、故障排查）
├── CLAUDE.md                      # Claude Code 项目上下文（架构、进度、规范）
├── plan.md                        # 完整实施计划（9 阶段）
├── plan-phase4.md                 # Phase 4 详细设计（含所有反馈追踪）
├── Makefile
├── go.mod
└── .github/workflows/ci.yml
```

---

## 开发指南

### 常用命令

<!-- AUTO-GENERATED: makefile-reference-start -->

| 命令 | 说明 |
|------|------|
| `make build` | 编译二进制到 `./bin/pgdba` |
| `make test` | 运行所有测试（含 race detector） |
| `make test-unit` | 仅运行单元测试（无外部依赖） |
| `make test-e2e` | 运行 E2E 测试（先编译二进制，用 httptest mock Patroni） |
| `make test-integration` | 运行集成测试（需 Docker Compose 集群，`-tags integration`） |
| `make coverage` | 覆盖率检查（要求 ≥80%） |
| `make lint` | 代码风格检查（golangci-lint） |
| `make clean` | 清理构建产物 |

<!-- AUTO-GENERATED: makefile-reference-end -->

### 克隆后快速验证

```bash
# 1. 编译并运行单元测试（不需要任何外部服务）
make build && make test-unit

# 2. 运行 E2E 测试（不需要外部服务）
make test-e2e

# 3.（可选）运行集成测试（需要 Docker Compose 集群）
cd deployments/docker && cp .env.example .env
# 填写 POSTGRES_PASSWORD 和 REPLICATION_PASSWORD
docker compose up -d
# 等待 30s 让 Patroni 选主
cd ../.. && make test-integration
```

### 测试策略

| 类型 | 数量 | 运行方式 | 外部依赖 |
|------|------|----------|----------|
| 单元测试 | 202 | `make test-unit` | 无 |
| E2E 测试 | 38 | `make test-e2e` | 无（httptest mock） |
| 集成测试 | 7 | `make test-integration` | Docker Compose 集群 |

**单元测试** 覆盖所有核心逻辑：输出格式化、配置加载、Patroni 客户端、故障切换预检、诊断快照采集、配置调优引擎、Apply/DryRun/Rollback 管线、查询分析、锁等待链、索引建议。

**E2E 测试** 黑盒测试实际二进制：帮助输出、输出格式、所有命令的参数校验、cluster 生命周期、failover/replica 命令。

**集成测试** 需要 Docker Compose 集群：cluster connect/status、failover trigger、replica list/promote、switchover 后集群恢复验证。集成测试使用 `//go:build integration` build tag。

### 覆盖率要求

| 模块 | 目标 |
|------|------|
| `internal/inspect/` (collector, identity, lock) | ≥80% |
| `internal/tuning/` (engine, apply) | ≥80% |
| `internal/query/` (analysis) | ≥80% |
| `internal/patroni/` | ≥85% |
| `internal/failover/` | ≥90% |
| `internal/output/` | ≥90% |
| `internal/config/` | ≥85% |
| `internal/cli/` | 按命令覆盖（CLI 层主要通过 E2E/集成测试覆盖） |

### 环境变量速查

<!-- AUTO-GENERATED: env-reference-start -->

| 变量 | 必填 | 说明 | 默认值 |
|------|------|------|--------|
| `PGDBA_PG_HOST` | 是* | PostgreSQL 主机 | — |
| `PGDBA_PG_PORT` | 否 | PostgreSQL 端口 | `5432` |
| `PGDBA_PG_USER` | 否 | 连接用户名 | `postgres` |
| `PGDBA_PG_DATABASE` | 否 | 数据库名 | `postgres` |
| `PGDBA_PG_PASSWORD` | 是* | 连接密码（**只从此变量读取**） | — |
| `PGDBA_PG_SSLMODE` | 否 | SSL 模式 | `prefer` |
| `PGDBA_PROVIDER_TYPE` | 否 | Provider 类型 | `docker` |
| `PGDBA_CLUSTER_NAME` | 否 | 集群名称 | — |
| `PGDBA_MONITOR_PROMETHEUS_URL` | 否 | Prometheus 地址 | — |
| `PGDBA_MONITOR_GRAFANA_URL` | 否 | Grafana 地址 | — |

\* 使用 `--name` 引用注册表中的集群时，PG_HOST 从注册表读取；PASSWORD 始终需要。

<!-- AUTO-GENERATED: env-reference-end -->

### Docker Compose .env 变量

| 变量 | 说明 |
|------|------|
| `POSTGRES_PASSWORD` | PostgreSQL superuser 密码（Patroni 和 PgBouncer 共用） |
| `REPLICATION_PASSWORD` | 流复制用户密码 |

### 安全规范

- `PGConfig` 结构体不含 `Password` 字段，密码只从 `PGDBA_PG_PASSWORD` 读取
- 配置文件（`~/.pgdba/config.yaml`）永远不写入任何密钥
- Docker Compose 端口全部绑定 `127.0.0.1`，Patroni REST API 不对外暴露
- `patroni.yml` 以 `0600` 权限写入，防止密码被同容器其他进程读取
- `docker-compose.yml` 使用 `:?` 语法强制要求密码变量，缺失时直接报错拒绝启动
- `.env` 已加入 `.gitignore`，使用 `.env.example` 作为模板
- Config apply 操作使用文件锁防止并发，DryRun 模式预览变更

### 贡献指南

详见 [docs/CONTRIBUTING.md](docs/CONTRIBUTING.md)，快速概览：

1. Fork 本仓库
2. 遵循 TDD 流程：先写测试（RED），再写实现（GREEN），再重构（REFACTOR）
3. 确保 `make test-unit` 全部通过（race detector 启用）
4. 确保 `make test-e2e` 全部通过
5. 确保 `make lint` 无报错
6. 提交 PR，描述变更内容和测试方法

### 运维手册

部署、日常操作和故障排查见 [docs/RUNBOOK.md](docs/RUNBOOK.md)。

### 使用 Claude Code 继续开发

项目包含 [CLAUDE.md](CLAUDE.md)，克隆到新目录后 Claude Code 自动加载项目上下文：

```bash
git clone https://github.com/jianh619/dba-blooming.git && cd dba-blooming
claude   # 自动读取 CLAUDE.md，了解架构、进度和规范
```

---

## Phase 4 架构设计

Phase 4 实现了数据库诊断、配置调优和查询分析三大能力包。详细设计见 [plan-phase4.md](plan-phase4.md)。

### 核心概念

**ClusterIdentity（三级指纹）**：使用 SHA-256 生成稳定的集群标识。

| 级别 | 数据源 | 适用版本 |
|------|--------|----------|
| Tier 0 | `pg_control_system()` → system_identifier | PG 13+ |
| Tier 1 | `inet_server_addr()` : `inet_server_port()` : datid | PG 12+ |
| Tier 2 | config_host : config_port（用户配置回退） | 所有版本 |

**DiagSnapshot vs ChangeSet**：
- `DiagSnapshot` — 只读、可降级（缺失 section = warning，不中断）
- `ChangeSet` — 必须完整、支持回滚（DryRun → Apply → Verify → Rollback）

**版本感知降级（PG 12-16+）**：
- PG 12+: pg_settings, pg_stat_activity, pg_stat_user_tables
- PG 13+: pg_control_system()
- PG 14+: pg_stat_wal
- PG 16+: pg_stat_io

**Apply 安全管线**：Lock → DryRun → PreSnapshot → ALTER SYSTEM → Reload → Verify → Unlock

---

## 架构决策

| 决策 | 选择 | 原因 |
|------|------|------|
| 实现语言 | Go | 单二进制分发，无运行时依赖，强并发 |
| HA 编排 | Patroni + etcd | 行业标准，自动故障切换，REST API |
| 容器镜像 | Zalando Spilo | Patroni 创始团队维护，PG+Patroni 一体 |
| K8s DCS | K8s API（无独立 etcd） | 复用 K8s 基础设施 |
| CLI 框架 | cobra + viper | Go 生态事实标准 |
| 输出格式 | JSON 信封 | AI 代理可直接解析 |
| 部署抽象 | Provider 接口 | Docker / 裸金属 / K8s 统一操作接口 |
| 连接池 | PgBouncer（transaction 模式） | OLTP 场景下大幅降低连接开销 |
| 诊断快照 | inspect.DB 接口 + mock | 核心逻辑 100% 可单元测试 |
| 调优引擎 | PGTune 启发式 + 置信度 | 可解释推荐，支持保守模式 |

详细设计见 [plan.md](plan.md) 和 [plan-phase4.md](plan-phase4.md)。

---

## License

MIT
