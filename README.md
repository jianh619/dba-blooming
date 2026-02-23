# pgdba — PostgreSQL 虚拟 DBA 专家系统

`pgdba` 是一套面向生产环境的 PostgreSQL DBA 工具集，将高可用部署、故障切换、备份恢复、监控告警、配置调优、混沌测试等操作封装为统一的 CLI 命令，所有输出均为结构化 JSON，可直接供 AI 代理调用。

## 特性概览

- **统一 JSON 输出**：所有命令输出标准信封格式，AI 可直接解析
- **多 Provider 支持**：Docker（已实现）、裸金属 SSH、Kubernetes（规划中）
- **高可用编排**：基于 Patroni + etcd 的自动故障切换（规划中）
- **安全优先**：密码永远不写入配置文件，只从环境变量读取
- **TDD 开发**：87.4% 单元测试覆盖率，race detector 全程启用

## 当前状态

| 阶段 | 内容 | 状态 |
|------|------|------|
| 阶段一 | CLI 框架、配置系统、Provider 接口、health check | **已完成** |
| 阶段二 | 集群生命周期（cluster init/status/destroy/connect） | 规划中 |
| 阶段三 | 故障切换与从库管理 | 规划中 |
| 阶段四 | 备份与 PITR 恢复 | 规划中 |
| 阶段五 | 监控与告警（Prometheus + Grafana） | 规划中 |
| 阶段六 | 数据库配置自动调优 | 规划中 |
| 阶段七 | 混沌测试 | 规划中 |
| 阶段八 | 裸金属部署（Ansible） | 规划中 |
| 阶段九 | Kubernetes 支持（Helm + client-go） | 规划中 |

---

## 快速开始

### 环境要求

- Go 1.22+
- PostgreSQL 14+（用于 `health check` 等诊断命令）

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

## CLI 用法

### 全局参数

```
--format json|table|yaml    输出格式（默认 json）
--config <path>             配置文件路径（默认 ~/.pgdba/config.yaml）
--provider docker|baremetal|kubernetes  部署 Provider（默认 docker）
--verbose                   启用详细日志
```

### 所有输出均遵循统一信封格式

```json
{
  "success": true,
  "timestamp": "2026-02-23T10:00:00Z",
  "command": "health check",
  "data": { ... },
  "error": null
}
```

失败时：

```json
{
  "success": false,
  "timestamp": "2026-02-23T10:00:00Z",
  "command": "health check",
  "error": "connect to postgres localhost:5432: connection refused"
}
```

---

### 已实现命令

#### `pgdba health check`

对已配置的 PostgreSQL 实例执行全面健康检查。

```bash
pgdba health check
```

**JSON 输出示例：**

```json
{
  "success": true,
  "timestamp": "2026-02-23T10:00:00Z",
  "command": "health check",
  "data": {
    "pg_version": "PostgreSQL 16.1 on x86_64-pc-linux-gnu",
    "uptime_seconds": 86423.5,
    "connections": {
      "current": 12,
      "max": 100
    },
    "replication": {
      "standby_count": 2
    },
    "healthy": true
  }
}
```

**表格输出：**

```bash
pgdba health check --format table
```

```
STATUS       COMMAND              TIMESTAMP
SUCCESS      health check         2026-02-23T10:00:00Z
```

**YAML 输出：**

```bash
pgdba health check --format yaml
```

---

### 规划中的命令

以下命令在后续阶段实现，接口设计已确定：

```bash
# 集群管理
pgdba cluster init --primary-host X --standbys Y,Z --provider docker
pgdba cluster connect --patroni-url http://10.0.0.1:8008   # 接管已有 Patroni 集群
pgdba cluster status
pgdba cluster destroy --confirm

# 从库管理
pgdba replica add --host X
pgdba replica remove --host X
pgdba replica promote --host X

# 故障切换
pgdba failover trigger --target X
pgdba failover status

# 备份与恢复
pgdba backup create --type full|logical
pgdba backup restore --backup-id X --target-time "2026-02-23 09:00:00"
pgdba backup list
pgdba backup schedule --cron "0 2 * * *"

# 监控
pgdba monitor setup --prometheus --grafana
pgdba monitor status
pgdba monitor alerts list|add|remove

# 健康报告
pgdba health report

# 配置调优
pgdba config tune --workload oltp|olap|mixed
pgdba config show
pgdba config diff
pgdba config apply --file X

# 混沌测试（需要 --i-know-what-i-am-doing 标志）
pgdba chaos kill-node --host X --i-know-what-i-am-doing
pgdba chaos partition --isolate X
pgdba chaos corrupt --table X
pgdba chaos report

# 查询分析
pgdba query slow-log --threshold 1s
pgdba query index-suggest --table X
pgdba query analyze --sql "SELECT ..."
```

---

## 使用场景

### 场景一：全新部署（Docker）— 阶段一已支持诊断，阶段二支持全量

```bash
# 诊断已有 PG（任意实例，无需 Patroni）
export PGDBA_PG_HOST=10.0.0.1
pgdba health check

# 全新部署高可用集群（阶段二实现后）
pgdba cluster init \
  --primary-host 10.0.0.1 \
  --standbys 10.0.0.2,10.0.0.3 \
  --provider docker
```

### 场景二：接管已有 Patroni 集群（阶段二实现后）

```bash
# 接管集群，不创建新资源
pgdba cluster connect \
  --patroni-url http://10.0.0.1:8008 \
  --provider baremetal \
  --ssh-user postgres

# 接管后所有诊断和运维命令均可使用
pgdba cluster status
pgdba failover trigger --target 10.0.0.2
pgdba backup create --type full
```

### 场景三：仅诊断（无 Patroni，任意 PG 实例）

无需 Patroni，只需提供连接信息即可使用诊断类命令：

```bash
export PGDBA_PG_HOST=my-pg-host
export PGDBA_PG_PASSWORD=secret
pgdba health check
pgdba config tune --workload oltp    # 阶段六实现后
pgdba query slow-log --threshold 1s  # 阶段八实现后
```

HA 相关命令（`failover`、`replica`、`cluster`）在无 Patroni 环境下不可用，会返回明确错误。

### 场景四：Kubernetes 部署（阶段九实现后）

```bash
# K8s 模式：Patroni 使用 K8s API 作为 DCS，无需独立 etcd
pgdba cluster init \
  --provider kubernetes \
  --namespace postgres \
  --replicas 2

# K8s 混沌测试：删除主库 Pod，验证自动故障切换
pgdba chaos kill-node --host pg-primary-0 \
  --i-know-what-i-am-doing
```

---

## 项目结构

```
.
├── cmd/pgdba/main.go              # 程序入口
├── internal/
│   ├── cli/                       # cobra 命令定义
│   │   ├── root.go                # 根命令与全局参数
│   │   └── health.go              # health check 命令
│   ├── config/                    # 配置加载（viper + 环境变量）
│   │   ├── config.go
│   │   └── defaults.go
│   ├── output/                    # 统一输出信封与格式化器
│   │   ├── types.go               # Response 类型、Format 常量
│   │   └── formatter.go           # JSON / YAML / Table 格式化
│   ├── postgres/                  # PostgreSQL 连接管理
│   │   └── conn.go                # Config.DSN()、Connect()、Ping()
│   └── provider/                  # 部署平台抽象层
│       ├── provider.go            # Provider 接口定义
│       └── docker.go              # Docker Provider 实现（骨架）
├── tests/
│   └── unit/                      # 单元测试（44 个，覆盖率 87.4%）
├── Makefile                       # build / test / coverage / lint
├── .github/workflows/ci.yml       # GitHub Actions CI
├── plan.md                        # 完整实施计划
└── go.mod
```

---

## 开发指南

### 常用命令

```bash
# 编译
make build

# 运行所有单元测试（含 race detector）
make test-unit

# 运行所有测试
make test

# 检查覆盖率（要求 ≥80%，不达标则退出码非零）
make coverage

# 代码风格检查
make lint

# 清理构建产物
make clean
```

### 覆盖率要求

| 模块 | 目标覆盖率 |
|------|-----------|
| `internal/output/` | ≥90% |
| `internal/config/` | ≥85% |
| `internal/provider/` | ≥80% |
| `internal/postgres/` | ≥80% |
| `internal/cli/` | ≥80% |

### 环境变量速查

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PGDBA_PG_HOST` | PostgreSQL 主机 | — |
| `PGDBA_PG_PORT` | PostgreSQL 端口 | `5432` |
| `PGDBA_PG_USER` | 连接用户名 | `postgres` |
| `PGDBA_PG_DATABASE` | 数据库名 | `postgres` |
| `PGDBA_PG_PASSWORD` | 连接密码（**只从此变量读取**） | — |
| `PGDBA_PG_SSLMODE` | SSL 模式 | `prefer` |
| `PGDBA_PROVIDER_TYPE` | Provider 类型 | `docker` |
| `PGDBA_CLUSTER_NAME` | 集群名称 | — |
| `PGDBA_MONITOR_PROMETHEUS_URL` | Prometheus 地址 | — |
| `PGDBA_MONITOR_GRAFANA_URL` | Grafana 地址 | — |

### 安全规范

- `PGConfig` 结构体不含 `Password` 字段，密码只从 `PGDBA_PG_PASSWORD` 读取
- 配置文件（`~/.pgdba/config.yaml`）永远不写入任何密钥
- 连接字符串不打印到日志
- 所有用户输入在边界处验证（Provider 类型、端口范围、节点名称等）

### 贡献指南

1. Fork 本仓库
2. 遵循 TDD 流程：先写测试（RED），再写实现（GREEN），再重构（REFACTOR）
3. 确保 `make coverage` 通过（覆盖率 ≥80%）
4. 确保 `make lint` 无报错
5. 提交 PR，描述变更内容和测试方法

---

## 架构决策

| 决策 | 选择 | 原因 |
|------|------|------|
| 实现语言 | Go | 单二进制分发，无运行时依赖，强并发 |
| HA 编排 | Patroni + etcd | 行业标准，自动故障切换，REST API |
| K8s DCS | K8s API（无独立 etcd） | 复用 K8s 基础设施，降低组件数量 |
| CLI 框架 | cobra + viper | Go 生态事实标准 |
| 输出格式 | JSON 信封 | AI 代理可直接解析 |
| 部署抽象 | Provider 接口 | Docker / 裸金属 / K8s 统一操作接口 |

详细设计见 [plan.md](plan.md)。

---

## License

MIT
