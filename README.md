# pgdba — PostgreSQL 虚拟 DBA 专家系统

`pgdba` 是一套面向生产环境的 PostgreSQL DBA 工具集，将高可用部署、故障切换、备份恢复、监控告警、配置调优、混沌测试等操作封装为统一的 CLI 命令，所有输出均为结构化 JSON，可直接供 AI 代理调用。

## 特性概览

- **统一 JSON 输出**：所有命令输出标准信封格式，AI 可直接解析
- **多 Provider 支持**：Docker（已实现）、裸金属 SSH、Kubernetes（规划中）
- **高可用编排**：Patroni REST API 客户端完整实现，etcd×3 + PG 主从 + PgBouncer 完整栈
- **集群注册表**：`~/.pgdba/clusters.json` 持久化，区分 managed / external 集群
- **安全优先**：密码永远不写入配置文件，只从环境变量读取；容器端口绑定 127.0.0.1
- **TDD 开发**：154 单元测试 + 49 E2E 测试，覆盖率 83.7%，race detector 全程启用

## 当前状态

| 阶段 | 内容 | 状态 |
|------|------|------|
| 阶段一 | CLI 框架、配置系统、Provider 接口、health check | **已完成** |
| 阶段二 | 集群生命周期（cluster init/status/destroy/connect）、Patroni 客户端、Docker 部署栈 | **已完成** |
| 阶段三 | 故障切换与从库管理 | **已完成** |
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
- Docker & Docker Compose（用于完整 HA 栈）
- PostgreSQL 14+（用于 `health check` 等诊断命令，可复用 Docker 栈）

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

#### `pgdba health check`

对已配置的 PostgreSQL 实例执行全面健康检查（版本、在线时长、连接数、从库数量）。

```bash
pgdba health check
pgdba health check --format table
pgdba health check --format yaml
```

**JSON 输出示例：**

```json
{
  "success": true,
  "timestamp": "2026-02-24T10:00:00Z",
  "command": "health check",
  "data": {
    "pg_version": "PostgreSQL 16.1 on x86_64-pc-linux-gnu",
    "uptime_seconds": 86423.5,
    "connections": { "current": 12, "max": 100 },
    "replication": { "standby_count": 2 },
    "healthy": true
  }
}
```

---

#### `pgdba cluster connect`

将已有的 Patroni 集群注册到本地注册表（`~/.pgdba/clusters.json`）。注册后可通过 `--name` 引用，无需每次指定 URL。

```bash
pgdba cluster connect \
  --name prod-ha \
  --patroni-url http://10.0.0.1:8008 \
  --pg-host 10.0.0.1 \
  --pg-port 5432 \
  --provider baremetal
```

注册前会自动验证 Patroni REST API 是否可达。

---

#### `pgdba cluster status`

查询集群拓扑，显示所有成员角色、状态、复制延迟。

```bash
# 通过注册表名称查询
pgdba cluster status --name prod-ha

# 直接指定 Patroni URL（无需注册）
pgdba cluster status --patroni-url http://10.0.0.1:8008
```

**JSON 输出示例：**

```json
{
  "success": true,
  "command": "cluster status",
  "data": {
    "cluster_name": "prod-ha",
    "primary": "pg-primary",
    "replica_count": 2,
    "healthy": true,
    "members": [
      { "name": "pg-primary", "role": "leader", "state": "running", "lag": 0 },
      { "name": "pg-replica-1", "role": "replica", "state": "running", "lag": 0 }
    ]
  }
}
```

---

#### `pgdba cluster destroy`

删除由 pgdba 管理（`cluster init` 创建）的集群。**拒绝销毁通过 `cluster connect` 接管的外部集群**，防止误操作。

```bash
pgdba cluster destroy --name prod-ha --confirm
```

---

#### `pgdba cluster init`

命令框架已实现，参数校验完整。Docker Provider 的实际节点创建（`CreateNode`）待后续阶段实现。

```bash
pgdba cluster init \
  --name prod-ha \
  --primary-host 10.0.0.1 \
  --standbys 10.0.0.2,10.0.0.3 \
  --provider docker
# 当前返回: "provider CreateNode: not implemented"
```

---

#### `pgdba failover trigger`

触发受控切换（默认）或强制故障转移（`--force`）。

```bash
# 受控切换 — 自动选择最佳候选（延迟最小的 replica）
pgdba failover trigger --patroni-url http://10.0.0.1:8008

# 受控切换 — 指定候选节点（执行前会验证候选合法性）
pgdba failover trigger --patroni-url http://10.0.0.1:8008 --candidate pg-replica-1

# 强制故障转移 — 主库不可达时使用（跳过预检）
pgdba failover trigger --patroni-url http://10.0.0.1:8008 --force --candidate pg-replica-1

# 通过注册表名称引用集群
pgdba failover trigger --name prod-ha --candidate pg-replica-1
```

**JSON 输出示例（受控切换）：**

```json
{
  "success": true,
  "command": "failover trigger",
  "data": {
    "type": "switchover",
    "from": "pg-primary",
    "to": "pg-replica-1",
    "status": "completed"
  }
}
```

---

#### `pgdba failover status`

显示当前集群故障切换状态，包括主库、从库列表、是否有切换进行中。

```bash
pgdba failover status --patroni-url http://10.0.0.1:8008
pgdba failover status --name prod-ha
```

**JSON 输出示例：**

```json
{
  "success": true,
  "command": "failover status",
  "data": {
    "primary": "pg-primary",
    "replicas": ["pg-replica-1", "pg-replica-2"],
    "failover_in_progress": false,
    "paused": false,
    "member_count": 3
  }
}
```

---

#### `pgdba replica list`

列出所有从库节点及其复制延迟信息。

```bash
pgdba replica list --patroni-url http://10.0.0.1:8008
pgdba replica list --name prod-ha
```

**JSON 输出示例：**

```json
{
  "success": true,
  "command": "replica list",
  "data": {
    "replicas": [
      { "name": "pg-replica-1", "state": "running", "lag_bytes": 512, "host": "pg-replica-1", "port": 5432 },
      { "name": "pg-replica-2", "state": "running", "lag_bytes": 1024, "host": "pg-replica-2", "port": 5432 }
    ],
    "count": 2
  }
}
```

---

#### `pgdba replica promote`

通过受控切换将指定从库提升为主库（同 `failover trigger --candidate`，但更语义化）。

```bash
pgdba replica promote --patroni-url http://10.0.0.1:8008 --candidate pg-replica-1
pgdba replica promote --name prod-ha --candidate pg-replica-1
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

# 配置调优
pgdba config tune --workload oltp|olap|mixed
pgdba config show
pgdba config diff
pgdba config apply --file X

# 混沌测试（需要 --i-know-what-i-am-doing 标志）
pgdba chaos kill-node --host X --i-know-what-i-am-doing
pgdba chaos partition --isolate X
pgdba chaos report

# 查询分析
pgdba query slow-log --threshold 1s
pgdba query index-suggest --table X
pgdba query analyze --sql "SELECT ..."
```

---

## 使用场景

### 场景一：诊断已有 PG 实例（无需 Patroni）

```bash
export PGDBA_PG_HOST=10.0.0.1
export PGDBA_PG_PASSWORD=secret
pgdba health check
```

### 场景二：接管已有 Patroni 集群

```bash
# 接管集群，不创建任何新资源
pgdba cluster connect \
  --name prod \
  --patroni-url http://10.0.0.1:8008 \
  --pg-host 10.0.0.1

# 查看拓扑
pgdba cluster status --name prod
```

### 场景三：本地开发 HA 环境（Docker Compose）

```bash
cd deployments/docker && cp .env.example .env
# 填写密码后：
docker compose up -d
```

### 场景四：故障切换

```bash
# 查看集群当前主从状态
pgdba failover status --name prod-ha

# 列出所有从库及延迟
pgdba replica list --name prod-ha

# 受控切换主库（自动选最优 replica）
pgdba failover trigger --name prod-ha

# 主库宕机时强制故障转移
pgdba failover trigger --name prod-ha --force --candidate pg-replica-1
```

### 场景五：全新部署（待实现）

```bash
pgdba cluster init \
  --primary-host 10.0.0.1 \
  --standbys 10.0.0.2,10.0.0.3 \
  --provider docker
# 当前返回: "provider CreateNode: not implemented"
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
│   │   ├── failover.go            # failover trigger/status 命令
│   │   └── replica.go             # replica list/promote 命令
│   ├── config/                    # 配置加载（viper + 环境变量）
│   │   ├── config.go
│   │   └── defaults.go
│   ├── output/                    # 统一输出信封与格式化器
│   │   ├── types.go               # Response 类型、Format 常量
│   │   └── formatter.go           # JSON / YAML / Table 格式化
│   ├── patroni/                   # Patroni REST API 客户端
│   │   ├── client.go              # GetClusterStatus、Switchover、Failover 等
│   │   ├── config.go              # patroni.yml / etcd.yml 模板渲染
│   │   └── templates/             # Go embed 模板文件
│   ├── cluster/                   # 集群注册表
│   │   └── registry.go            # ~/.pgdba/clusters.json CRUD
│   ├── pgbouncer/                 # PgBouncer 配置生成
│   │   └── config.go              # RenderConfig() / RenderUserlist()
│   ├── postgres/                  # PostgreSQL 连接管理
│   │   └── conn.go                # Config.DSN()、Connect()、Ping()
│   ├── failover/                  # 故障切换预检逻辑
│   │   └── precheck.go            # FindPrimary/FindBestCandidate/CheckSwitchover/ListReplicas
│   └── provider/                  # 部署平台抽象层
│       ├── provider.go            # Provider 接口定义
│       └── docker.go              # Docker Provider（接口完整，待 SDK 集成）
├── deployments/docker/            # 完整 HA 部署栈
│   ├── docker-compose.yml         # etcd×3 + Spilo×3 + PgBouncer
│   ├── .env.example               # 环境变量模板（复制为 .env 后填写）
│   ├── Dockerfile.postgres        # 自建镜像方案（备选）
│   └── patroni-entrypoint.sh      # 自建镜像启动脚本（备选）
├── tests/
│   ├── unit/                      # 单元测试（154 个，覆盖率 83.7%）
│   │   ├── failover_precheck_test.go  # failover 预检逻辑（19 个）
│   │   └── failover_cmd_test.go       # failover/replica CLI 命令（14 个）
│   └── e2e/                       # E2E 测试（49 个，黑盒测试实际二进制）
├── Makefile                       # build / test-unit / test-e2e / coverage / lint
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

# 单元测试（含 race detector）
make test-unit

# E2E 测试（编译二进制后黑盒测试）
make test-e2e

# 运行所有测试
make test

# 覆盖率检查（要求 ≥80%）
make coverage

# 代码风格检查
make lint

# 清理构建产物
make clean
```

### 测试策略

| 类型 | 数量 | 运行方式 | 依赖 |
|------|------|----------|------|
| 单元测试 | 154 | `make test-unit` | 无外部服务 |
| E2E 测试 | 49 | `make test-e2e` | 无外部服务（用 httptest mock Patroni） |
| 集成测试 | — | 手动 | 需要 Docker Compose 栈 |

E2E 测试场景覆盖：帮助输出、输出格式（json/table/yaml）、所有命令的参数校验、cluster 完整生命周期、failover/replica 命令、JSON 信封契约。

### 覆盖率要求

| 模块 | 目标覆盖率 |
|------|-----------|
| `internal/output/` | ≥90% |
| `internal/config/` | ≥85% |
| `internal/patroni/` | ≥85% |
| `internal/failover/` | ≥90% |
| `internal/provider/` | ≥80% |
| `internal/postgres/` | ≥80% |
| `internal/cluster/` | ≥80% |
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
- Docker Compose 端口全部绑定 `127.0.0.1`，Patroni REST API 不对外暴露
- `patroni.yml` 以 `0600` 权限写入，防止密码被同容器其他进程读取
- `docker-compose.yml` 使用 `:?` 语法强制要求密码变量，缺失时直接报错拒绝启动
- `.env` 已加入 `.gitignore`，使用 `.env.example` 作为模板

### 贡献指南

1. Fork 本仓库
2. 遵循 TDD 流程：先写测试（RED），再写实现（GREEN），再重构（REFACTOR）
3. 确保 `make coverage` 通过（覆盖率 ≥80%）
4. 确保 `make test-e2e` 全部通过
5. 确保 `make lint` 无报错
6. 提交 PR，描述变更内容和测试方法

---

## 架构决策

| 决策 | 选择 | 原因 |
|------|------|------|
| 实现语言 | Go | 单二进制分发，无运行时依赖，强并发 |
| HA 编排 | Patroni + etcd | 行业标准，自动故障切换，REST API |
| 容器镜像 | Zalando Spilo | Patroni 创始团队维护，PG+Patroni 一体，无需自建 |
| K8s DCS | K8s API（无独立 etcd） | 复用 K8s 基础设施，降低组件数量 |
| CLI 框架 | cobra + viper | Go 生态事实标准 |
| 输出格式 | JSON 信封 | AI 代理可直接解析 |
| 部署抽象 | Provider 接口 | Docker / 裸金属 / K8s 统一操作接口 |
| 连接池 | PgBouncer（transaction 模式） | OLTP 场景下大幅降低连接开销 |

详细设计见 [plan.md](plan.md)。

---

## License

MIT
