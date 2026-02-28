# CLAUDE.md — pgdba 项目指南（Claude Code 专用）

本文件为 Claude Code 提供项目上下文，使其在全新目录下也能立即理解项目并继续工作。

---

## 项目概述

**pgdba** 是一套面向生产环境的 PostgreSQL 虚拟 DBA 专家系统 CLI 工具集。
将高可用部署、故障切换、配置调优、查询分析等操作封装为统一的 CLI 命令，所有输出为结构化 JSON，可供 AI 代理直接调用。

- 语言：Go 1.22+
- CLI 框架：cobra + viper
- 数据库驱动：pgx/v5
- HA 编排：Patroni + etcd（Zalando Spilo 镜像）
- 连接池：PgBouncer（transaction mode）

---

## 当前进度

| 阶段 | 状态 | 核心内容 |
|------|------|----------|
| Phase 1 | **已完成** | CLI 框架、配置系统、Provider 接口、health check |
| Phase 2 | **已完成** | 集群生命周期（init/status/connect/destroy）、Patroni 客户端、Docker 栈 |
| Phase 3 | **已完成** | 故障切换与从库管理（failover trigger/status, replica list/promote） |
| Phase 4 | **已完成** | 配置调优与查询分析（inspect, config, query, baseline） |
| Phase 5-9 | 规划中 | 备份PITR、物理备份、监控告警、裸金属、K8s |

**下一步工作**：Phase 5（备份与 PITR 恢复）。参考 `plan.md` 阶段四的步骤 23-26。

---

## 关键文件索引

### 入口与框架
- `cmd/pgdba/main.go` — 程序入口
- `internal/cli/root.go` — 根命令，注册所有子命令
- `internal/config/config.go` — viper 配置加载（环境变量 + YAML）
- `internal/output/types.go` + `formatter.go` — 统一 JSON 信封输出

### Phase 1-3 核心
- `internal/cli/health.go` — health check
- `internal/cli/cluster.go` — cluster init/connect/status/destroy
- `internal/cli/failover.go` — failover trigger/status
- `internal/cli/replica.go` — replica list/promote
- `internal/patroni/client.go` — Patroni REST API 客户端（GetClusterStatus, Switchover, Failover 等）
- `internal/cluster/registry.go` — 集群注册表（~/.pgdba/clusters.json）
- `internal/failover/precheck.go` — 故障切换预检逻辑
- `internal/provider/provider.go` + `docker.go` — Provider 接口与 Docker 实现

### Phase 4 核心（配置调优与查询分析）
- `internal/inspect/identity.go` — ClusterIdentity 三级指纹（SHA-256）
- `internal/inspect/types.go` — DiagSnapshot, ChangeSet, SamplingConfig, ParamChange 等
- `internal/inspect/collector.go` — 版本感知的诊断数据采集器
- `internal/inspect/db.go` — DB 接口（可 mock 的数据库抽象层）
- `internal/inspect/pgxdb.go` — pgx 真实数据库适配器
- `internal/inspect/lock.go` — Apply/Rollback 文件锁互斥
- `internal/tuning/engine.go` — PGTune 启发式推荐引擎（含置信度 + Rationale）
- `internal/tuning/apply.go` — DryRun / Apply / Rollback 安全管线
- `internal/query/types.go` — TopQuery, LockInfo, TableBloat, IndexSuggestion 等
- `internal/query/analysis.go` — SuggestIndexes, BuildLockChains

### 部署
- `deployments/docker/docker-compose.yml` — 完整 HA 栈（etcd×3 + Spilo×3 + PgBouncer）
- `deployments/docker/.env.example` — 密码模板（POSTGRES_PASSWORD, REPLICATION_PASSWORD）
- `deployments/docker/DEPLOY_README.md` — 详细部署指南与架构图

### 设计文档
- `plan.md` — 完整 9 阶段实施计划
- `plan-phase4.md` — Phase 4 详细设计（含所有反馈追踪 H1-H3, M1-M4, #1-#12）

---

## 构建与测试

```bash
make build          # 编译到 ./bin/pgdba
make test           # 全部测试（含 race detector）
make test-unit      # 仅单元测试（无外部依赖，202 个）
make test-e2e       # E2E 测试（httptest mock，38 个）
make test-integration  # 集成测试（需 Docker Compose，7 个）
make coverage       # 覆盖率检查（要求 ≥80%）
make lint           # golangci-lint
make clean          # 清理
```

---

## 开发规范

### TDD 流程（强制）
1. **RED** — 先写失败的测试
2. **GREEN** — 写最小实现使测试通过
3. **REFACTOR** — 重构优化
4. 运行 `make test-unit` 确认全部通过（race detector 启用）

### 测试文件位置
- 单元测试：`tests/unit/`（所有 `*_test.go`）
- E2E 测试：`tests/e2e/e2e_test.go`（黑盒测试编译后的二进制）
- 集成测试：`tests/integration/integration_test.go`（`//go:build integration` tag）

### 输出格式
所有 CLI 命令必须使用统一 JSON 信封（`internal/output/types.go`）：
```json
{
  "success": true/false,
  "timestamp": "RFC3339",
  "command": "命令名",
  "data": { ... },
  "error": "错误信息（仅失败时）"
}
```

### 安全规范
- 密码只从 `PGDBA_PG_PASSWORD` 环境变量读取，永远不写入配置文件或代码
- Docker 端口全部绑定 `127.0.0.1`
- `patroni.yml` 权限 `0600`
- `docker-compose.yml` 使用 `:?` 语法强制密码变量

### 代码风格
- 文件 < 800 行，函数 < 50 行
- 不可变模式：返回新对象而非修改原始对象
- 错误必须显式处理，不要静默吞掉
- 使用 `inspect.DB` 接口实现可测试性（mock 注入）

### commit 消息
```
<type>: <description>
```
类型：feat, fix, refactor, docs, test, chore, perf, ci

---

## 架构要点

### ClusterIdentity 三级指纹
- Tier 0: `pg_control_system()` → system_identifier（PG 13+）
- Tier 1: `inet_server_addr()` + `inet_server_port()` + datid（PG 12+）
- Tier 2: config_host + config_port（用户配置回退）

### DiagSnapshot vs ChangeSet
- **DiagSnapshot** — 只读、可降级（缺失 section = warning）
- **ChangeSet** — 必须完整、支持回滚

### 版本感知降级
- PG 12+: pg_settings, pg_stat_activity, pg_stat_user_tables
- PG 13+: pg_control_system()
- PG 14+: pg_stat_wal
- PG 16+: pg_stat_io

### Apply 安全管线
Lock → DryRun → PreSnapshot → ALTER SYSTEM → Reload → Verify → Unlock

### 接口设计
- `inspect.DB` — 抽象 pgx.Conn，支持 mock 单元测试
- `tuning.ApplyDB` — 抽象 ALTER SYSTEM/pg_reload_conf
- `query.DB` — 抽象查询分析所需的数据库操作
- `provider.Provider` — 抽象部署平台（Docker/裸金属/K8s）

---

## 环境变量

| 变量 | 必填 | 说明 |
|------|------|------|
| `PGDBA_PG_HOST` | 是* | PostgreSQL 主机 |
| `PGDBA_PG_PORT` | 否 | 端口（默认 5432） |
| `PGDBA_PG_USER` | 否 | 用户（默认 postgres） |
| `PGDBA_PG_DATABASE` | 否 | 数据库（默认 postgres） |
| `PGDBA_PG_PASSWORD` | 是* | 密码（只从此变量读取） |
| `PGDBA_PG_SSLMODE` | 否 | SSL 模式（默认 prefer） |

\* 使用 `--name` 引用注册表集群时，HOST 从注册表读取；PASSWORD 始终需要。

---

## 快速验证（克隆后）

```bash
git clone <repo-url> && cd dba-blooming
make build && make test-unit && make test-e2e
# 预期：202 unit + 38 e2e 全部 PASS
```

如需运行集成测试：
```bash
cd deployments/docker && cp .env.example .env
# 编辑 .env 填入密码
docker compose up -d && sleep 30
cd ../.. && make test-integration
```
