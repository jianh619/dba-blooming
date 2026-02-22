# PostgreSQL 虚拟 DBA 专家系统 — 实施计划

## 概述

构建一套基于 **Go** 语言的 CLI 工具集，将所有 PostgreSQL DBA 操作封装为结构化、可供 AI 调用的 CLI 命令（JSON 输出）。系统采用 **Patroni + etcd** 实现高可用编排，**PgBouncer** 实现连接池管理，**Prometheus + Grafana** 实现监控告警。所有操作同时支持 Docker/容器化、裸金属和 **Kubernetes** 三种部署模式。

---

## 使用场景与边界

本工具面向五类典型场景，每类场景的支持程度和前提条件不同。

### 场景矩阵

| 场景 | 描述 | 支持程度 | 是否需要预先存在的数据库 | 实现阶段 |
|------|------|----------|--------------------------|----------|
| **场景一** | 全新部署（Docker/Compose） | 完全支持 | 否，工具自动创建 | 阶段一～七 |
| **场景二** | 全新部署（裸金属/SSH） | 完全支持 | 否，工具自动创建 | 阶段八 |
| **场景三** | 全新部署（Kubernetes） | 完全支持 | 否，工具自动创建 | 阶段九 |
| **场景四** | 接管已有 Patroni 集群 | 部分支持（见下表） | 是，已有 Patroni 管理的集群 | 阶段二起 |
| **场景五** | 诊断已有 PG（无 Patroni） | 仅诊断命令 | 是，任意可连接的 PG 实例 | 阶段一起 |

---

### 场景四详述：接管已有 Patroni 集群

使用 `pgdba cluster connect` 命令接入已有集群，无需工具创建基础设施。

| 命令类别 | 具体命令 | 支持程度 | 说明 |
|----------|----------|----------|------|
| 状态查询 | `cluster status` | ✅ 完全支持 | 直接调用 Patroni REST API |
| 故障切换 | `failover trigger/status` | ✅ 完全支持 | 调用 Patroni REST API，兼容已有集群 |
| 健康检查 | `health check/report` | ✅ 完全支持 | 连接 PG 查询，兼容任意实例 |
| 监控部署 | `monitor setup/status/alerts` | ✅ 完全支持 | 监控栈独立于集群创建方式 |
| 配置调优 | `config tune/show/diff/apply` | ✅ 完全支持 | 直接查询 `pg_settings` |
| 查询分析 | `query slow-log/analyze/index-suggest` | ✅ 完全支持 | 查询 `pg_stat_statements` |
| 备份管理 | `backup create/restore/schedule/list` | ✅ 完全支持 | 封装 `pg_basebackup`/`pg_dump` |
| 从库管理 | `replica add` | ⚠️ 部分支持 | 需要 Provider 能访问目标主机（SSH/kubectl） |
| 混沌测试 | `chaos kill-node/partition` | ⚠️ 部分支持 | 需配置对应 Provider 的访问权限 |
| 集群创建 | `cluster init` | ❌ 不适用 | 仅用于工具自己创建的集群 |
| 集群销毁 | `cluster destroy` | ❌ 不适用 | 不管理工具外创建的资源 |

**接入命令**：
```bash
# 接管已有 Patroni 集群（只读诊断 + HA 操作）
pgdba cluster connect \
  --patroni-url http://10.0.0.1:8008 \
  --pg-host 10.0.0.1 --pg-port 5432 \
  --provider baremetal --ssh-user postgres \
  --name my-existing-cluster

# 接管后即可使用所有兼容命令
pgdba cluster status
pgdba failover trigger --target 10.0.0.2
pgdba backup create --type full
```

---

### 场景五详述：诊断已有 PG（无 Patroni）

仅需提供连接信息，无需任何 Patroni 或特定部署方式。

```bash
# 通过环境变量指定连接信息
export PGDBA_PG_HOST=10.0.0.1
export PGDBA_PG_PORT=5432
export PGDBA_PG_USER=postgres
export PGDBA_PG_PASSWORD=<from-secret-manager>

# 诊断类命令均可使用
pgdba health check
pgdba health report
pgdba config tune --workload oltp
pgdba config show
pgdba query slow-log --threshold 1s
pgdba query index-suggest --table orders
pgdba backup create --type logical --database mydb
```

HA 相关命令（`failover`、`replica`、`cluster`）在无 Patroni 的环境下不可用，会返回明确错误信息。

---

### 场景三详述：Kubernetes 部署的关键差异

Kubernetes 环境与 Docker/裸金属有本质不同，需要独立的 Provider 实现：

| 差异点 | Docker/裸金属 | Kubernetes |
|--------|--------------|------------|
| 节点操作 | `docker exec` / SSH | `kubectl exec` |
| 节点杀死 | `docker stop` / `kill` 信号 | `kubectl delete pod` |
| 网络分区 | `docker network disconnect` / iptables | `NetworkPolicy` |
| 服务发现 | hosts 文件 / DNS | K8s Service + Headless Service |
| 配置存储 | 文件系统 / etcd 独立部署 | ConfigMap + K8s API（Patroni K8s DCS 模式） |
| 部署方式 | Compose / Ansible | StatefulSet + Helm Chart |
| 存储 | 本地目录 / NFS | PersistentVolumeClaim |
| Patroni DCS | 独立 etcd 集群 | K8s API（内置，无需额外 etcd） |

**K8s 模式下 Patroni 使用 Kubernetes API 本身作为 DCS**，不需要额外部署 etcd，显著降低运维复杂度。

---

## 架构决策

### 决策一：Go 作为实现语言
- **原因**：单二进制分发，无运行时依赖，优秀的 CLI 库（cobra），强并发支持（适合并行健康检查），目标机器无需安装额外依赖。
- **备选方案**：Python —— 已排除，因为需要在每个目标节点安装运行时和管理依赖。

### 决策二：Patroni 而非 Repmgr
- **原因**：Patroni 是生产环境 PostgreSQL 高可用的行业标准。提供自动故障切换、基于 DCS 的领导者选举（etcd/ZooKeeper/Consul/K8s API）、REST API 集成接口，已在 GitLab、Zalando 等公司生产环境中得到验证。Repmgr 缺乏分布式共识存储，在脑裂场景下需要手动隔离。
- **权衡**：Patroni 需要额外的 DCS（Docker/裸金属用 etcd，K8s 模式复用 K8s API），但两者都是轻量且成熟的方案。

### 决策三：etcd 作为分布式配置存储（非 K8s 模式）
- **原因**：与 Patroni 原生集成，部署简单，强一致性保证，广泛采用。
- **K8s 模式**：使用 Kubernetes API 作为 DCS，无需独立 etcd。
- **备选方案**：Consul（较重）、ZooKeeper（运维更复杂）。

### 决策四：Cobra + JSON 输出
- **原因**：Cobra 是 Go 生态事实标准 CLI 框架。每个命令默认输出结构化 JSON，AI 代理可直接解析。通过 `--format` 参数支持 `json`、`table`、`yaml` 三种格式。

### 决策五：多 Provider 抽象层
- **原因**：`provider` 接口抽象 Docker、裸金属、Kubernetes 等部署平台的差异。每个 Provider 实现节点配置、服务管理和网络管理。所有 CLI 命令与 Provider 无关。

### 决策六：Kubernetes Provider 使用 client-go + Helm
- **原因**：`client-go` 是 K8s 官方 Go 客户端，稳定且功能完整。Helm Chart 提供参数化的 K8s 资源模板，支持版本管理和升级。
- **Patroni K8s DCS 模式**：Patroni 直接使用 K8s ConfigMap 和 Endpoints 进行领导者选举，无需额外 etcd。
- **混沌测试**：网络分区通过动态创建 `NetworkPolicy` 实现，节点故障通过 `kubectl delete pod` 触发（StatefulSet 自动重建，Patroni 处理角色切换）。

---

## CLI 命令设计

所有命令遵循统一格式：`pgdba <资源> <操作> [参数]`

所有命令返回统一的 JSON 信封：

```json
{
  "success": true,
  "timestamp": "2026-02-22T10:00:00Z",
  "command": "cluster status",
  "data": { ... },
  "error": null
}
```

### 命令速查表

| 命令 | 说明 |
|------|------|
| `pgdba cluster init --primary-host X --standbys Y,Z --provider docker\|baremetal\|kubernetes` | 初始化高可用集群 |
| `pgdba cluster connect --patroni-url X --provider baremetal\|kubernetes` | **接管已有集群（新增）** |
| `pgdba cluster status` | 查看集群拓扑和健康状态 |
| `pgdba cluster destroy --confirm` | 销毁工具创建的集群 |
| `pgdba replica add --host X` | 添加流复制从库 |
| `pgdba replica remove --host X` | 移除从库 |
| `pgdba replica promote --host X` | 提升从库为主库 |
| `pgdba failover trigger --target X` | 触发受控故障切换 |
| `pgdba failover status` | 查看故障切换状态 |
| `pgdba backup create --type full\|incremental\|logical` | 创建备份 |
| `pgdba backup restore --backup-id X --target-time T` | PITR 时间点恢复 |
| `pgdba backup list` | 列出可用备份 |
| `pgdba backup schedule --cron "0 2 * * *"` | 调度定期备份 |
| `pgdba monitor setup --prometheus --grafana` | 部署监控栈 |
| `pgdba monitor status` | 查看指标摘要 |
| `pgdba monitor alerts list\|add\|remove` | 管理告警规则 |
| `pgdba health check` | 执行全面健康检查 |
| `pgdba health report` | 生成健康报告 |
| `pgdba config tune --workload oltp\|olap\|mixed` | 自动调优 PG 配置 |
| `pgdba config show` | 查看当前配置 |
| `pgdba config diff` | 查看配置与推荐值的差异 |
| `pgdba config apply --file X` | 应用配置变更 |
| `pgdba chaos kill-node --host X` | 模拟节点宕机 |
| `pgdba chaos partition --isolate X` | 模拟网络分区 |
| `pgdba chaos corrupt --table X` | 模拟数据损坏 |
| `pgdba chaos report` | 查看混沌测试结果 |
| `pgdba query slow-log --threshold 1s` | 查看慢查询 |
| `pgdba query index-suggest --table X` | 建议缺失索引 |
| `pgdba query analyze --sql "SELECT ..."` | 分析查询执行计划 |

---

## 目录结构

```
/home/luckyjian/code/dba-blooming-test/
├── cmd/
│   └── pgdba/
│       └── main.go                    # 程序入口
├── internal/
│   ├── cli/
│   │   ├── root.go                    # 根命令（cobra）
│   │   ├── cluster.go                 # cluster init/connect/status/destroy
│   │   ├── failover.go                # failover trigger/status/rollback
│   │   ├── replica.go                 # replica add/remove/promote
│   │   ├── backup.go                  # backup create/restore/schedule/list
│   │   ├── monitor.go                 # monitor setup/status/alerts
│   │   ├── config.go                  # config tune/show/diff/apply
│   │   ├── chaos.go                   # chaos kill-node/partition/corrupt
│   │   ├── query.go                   # query analyze/slow-log/index-suggest
│   │   └── health.go                  # health check/report
│   ├── provider/
│   │   ├── provider.go                # Provider 接口定义
│   │   ├── docker.go                  # Docker/Compose 实现
│   │   ├── baremetal.go               # 基于 SSH 的裸金属实现
│   │   └── kubernetes.go              # K8s 实现（client-go + kubectl）
│   ├── patroni/
│   │   ├── client.go                  # Patroni REST API 客户端
│   │   ├── config.go                  # Patroni 配置生成
│   │   └── templates/                 # Patroni YAML 模板
│   │       ├── patroni.yml.tmpl        # Docker/裸金属模式（etcd DCS）
│   │       ├── patroni-k8s.yml.tmpl    # K8s 模式（K8s API DCS）
│   │       └── etcd.yml.tmpl
│   ├── postgres/
│   │   ├── conn.go                    # 连接管理
│   │   ├── replication.go             # 流复制相关查询
│   │   ├── metrics.go                 # pg_stat_* 指标查询
│   │   ├── tuning.go                  # 参数调优逻辑
│   │   ├── backup.go                  # pg_basebackup、pg_dump 封装
│   │   └── wal.go                     # WAL 归档配置
│   ├── pgbouncer/
│   │   ├── config.go                  # PgBouncer 配置生成
│   │   └── manager.go                 # PgBouncer 生命周期管理
│   ├── monitoring/
│   │   ├── prometheus.go              # Prometheus 配置生成
│   │   ├── grafana.go                 # Grafana 仪表盘预置
│   │   ├── alerting.go                # 告警规则定义
│   │   └── healthcheck.go             # 健康检查端点逻辑
│   ├── chaos/
│   │   ├── engine.go                  # 混沌测试编排器
│   │   ├── scenarios.go               # 内置故障场景
│   │   └── report.go                  # 混沌测试结果报告
│   ├── output/
│   │   ├── formatter.go               # JSON/table/YAML 输出格式化
│   │   └── types.go                   # 公共响应信封类型
│   └── config/
│       ├── config.go                  # 全局工具配置
│       └── defaults.go                # 默认值和常量
├── deployments/
│   ├── docker/
│   │   ├── docker-compose.yml         # 完整服务栈 Compose 文件
│   │   ├── Dockerfile.postgres        # PG + Patroni 镜像
│   │   └── Dockerfile.pgbouncer       # PgBouncer 镜像
│   ├── ansible/
│   │   ├── inventory.yml.tmpl         # 裸金属主机清单模板
│   │   ├── site.yml                   # 主 Playbook
│   │   └── roles/
│   │       ├── postgresql/
│   │       ├── patroni/
│   │       ├── etcd/
│   │       ├── pgbouncer/
│   │       └── monitoring/
│   └── kubernetes/
│       ├── helm/                      # Helm Chart（推荐）
│       │   ├── Chart.yaml
│       │   ├── values.yaml            # 默认参数（副本数、资源规格、存储等）
│       │   └── templates/
│       │       ├── statefulset.yaml   # PG + Patroni StatefulSet
│       │       ├── service.yaml       # Headless Service（StatefulSet）+ RW/RO Service
│       │       ├── configmap.yaml     # Patroni 配置
│       │       ├── rbac.yaml          # Patroni 操作 K8s API 所需权限
│       │       └── pgbouncer.yaml     # PgBouncer Deployment
│       └── manifests/                 # 原始 K8s YAML（无 Helm 时使用）
│           ├── namespace.yaml
│           ├── statefulset.yaml
│           ├── service.yaml
│           ├── configmap.yaml
│           └── rbac.yaml
├── monitoring/
│   ├── prometheus/
│   │   ├── prometheus.yml             # Prometheus 抓取配置
│   │   └── rules/
│   │       ├── replication.rules.yml  # 复制相关告警规则
│   │       ├── performance.rules.yml  # 性能相关告警规则
│   │       └── availability.rules.yml # 可用性相关告警规则
│   └── grafana/
│       └── dashboards/
│           ├── cluster-overview.json  # 集群概览仪表盘
│           ├── replication.json       # 复制状态仪表盘
│           └── performance.json       # 性能仪表盘
├── tests/
│   ├── unit/
│   │   ├── patroni_client_test.go
│   │   ├── tuning_test.go
│   │   ├── formatter_test.go
│   │   └── config_test.go
│   ├── integration/
│   │   ├── cluster_test.go
│   │   ├── failover_test.go
│   │   ├── backup_test.go
│   │   ├── connect_test.go            # 接管已有集群测试
│   │   └── testutil/
│   │       ├── docker_helper.go       # 启动测试容器的工具
│   │       └── k8s_helper.go          # 启动 kind/k3s 测试集群的工具
│   └── e2e/
│       ├── full_lifecycle_test.go
│       ├── chaos_test.go
│       ├── k8s_lifecycle_test.go      # K8s 全流程 E2E
│       └── connect_existing_test.go   # 接管已有集群 E2E
├── go.mod
├── go.sum
├── Makefile
├── README.md
└── .github/
    └── workflows/
        ├── ci.yml
        └── release.yml
```

---

## 实施阶段

### 阶段一：基础框架与 CLI 骨架（MVP）

交付可运行的 CLI 二进制文件，含输出框架和 Docker Provider，暂不实现真实 PG 操作，用于验证架构。

| 步骤 | 操作 | 风险 |
|------|------|------|
| 1 | 初始化 Go 模块，添加依赖（cobra、viper、pgx/v5、zerolog、client-go） | 低 |
| 2 | 创建输出信封与格式化器（`internal/output/`） | 低 |
| 3 | 创建根命令与全局参数（`--format`、`--config`、`--verbose`、`--provider`） | 低 |
| 4 | 创建全局配置系统，从 `~/.pgdba/config.yaml` 和环境变量加载，禁止硬编码密钥 | 低 |
| 5 | 定义 Provider 接口（`CreateNode`、`DestroyNode`、`ExecOnNode`、`PartitionNode` 等） | 中 |
| 6 | 实现 Docker Provider，使用 Docker SDK | 中 |
| 7 | 实现 `pgdba health check` 命令（首个真实命令，验证完整链路） | 低 |
| 8 | 编写单元测试（格式化器、配置加载，TDD：先写测试） | 低 |
| 9 | 创建 Makefile 和 CI 流水线（构建、测试、覆盖率检查 ≥80%） | 低 |

---

### 阶段二：集群生命周期（高可用核心）

| 步骤 | 操作 | 风险 |
|------|------|------|
| 10 | 创建 Patroni 配置模板（patroni.yml.tmpl、etcd.yml.tmpl），密钥通过环境变量注入 | 中 |
| 11 | 实现 Patroni REST API 客户端（状态查询、故障切换、重新初始化等） | 中 |
| 12 | 实现 `pgdba cluster init`：依次启动 etcd、主库（Patroni）、从库，验证复制建立 | 高 |
| 13 | 实现 `pgdba cluster status`：查询 Patroni API 和 PG，返回拓扑、复制延迟、节点健康 | 低 |
| 14 | **实现 `pgdba cluster connect`**：接管已有 Patroni 集群，写入本地集群注册表，验证连通性 | 中 |
| 15 | 实现 `pgdba cluster destroy --confirm`：通过 Provider 销毁所有节点（拒绝对 connect 接入的集群执行） | 中 |
| 16 | 创建 Docker Compose 完整栈（etcd 3节点 + PG 主库 + 2个从库 + PgBouncer） | 中 |
| 17 | 实现 PgBouncer 配置生成（事务模式、连接池参数、拓扑变更时自动重载） | 低 |
| 18 | 编写集群生命周期集成测试（init → status → destroy；connect → status → failover） | 中 |

---

### 阶段三：故障切换、从库管理与恢复

| 步骤 | 操作 | 风险 |
|------|------|------|
| 19 | 实现 `pgdba replica add/remove/promote`（配置流复制、注册 Patroni、Switchover） | 中 |
| 20 | 实现 `pgdba failover trigger/status`，含切换前预检（目标是否已追上？集群是否健康？） | 高 |
| 21 | 实现节点重新加入逻辑：优先 pg_rewind，失败自动回退到 pg_basebackup | 高 |
| 22 | 编写故障切换集成测试（切换 → 新主接受写入 → 旧主作为从库重加入） | 中 |

---

### 阶段四：备份与恢复

| 步骤 | 操作 | 风险 |
|------|------|------|
| 23 | 实现 `pgdba backup create`（pg_basebackup 全量、pg_dump 逻辑备份），含备份校验 | 中 |
| 24 | 实现 `pgdba backup restore --target-time`（PITR 时间点恢复，配置 recovery_target_time） | 高 |
| 25 | 实现 `pgdba backup schedule`（生成 cron 任务/systemd timer/K8s CronJob，含保留策略自动清理） | 低 |
| 26 | 编写备份与恢复集成测试（备份 → 写入数据 → 恢复到写入前时间点 → 验证数据不存在） | 中 |

---

### 阶段五：监控与告警

| 步骤 | 操作 | 风险 |
|------|------|------|
| 27 | 实现 `pgdba monitor setup`：部署 Prometheus（含 postgres_exporter）和 Grafana | 中 |
| 28 | 定义告警规则：复制延迟 >30s（警告）/>60s（严重）、连接数 >80%、锁等待 >10s、磁盘 >80% 等 | 低 |
| 29 | 提供预构建 Grafana 仪表盘：集群概览、复制状态、性能分析 | 低 |
| 30 | 实现健康检查端点（HTTP 200/503）和 `pgdba health report`（聚合所有指标的 JSON 报告） | 低 |

---

### 阶段六：数据库配置调优

| 步骤 | 操作 | 风险 |
|------|------|------|
| 31 | 实现自动调优引擎：基于系统资源（RAM/CPU/磁盘类型）推算 PGTune 等效参数 | 中 |
| 32 | 实现 `pgdba config show/diff/apply`：查看当前配置、与推荐值对比、通过 ALTER SYSTEM 应用 | 中 |
| 33 | 编写调优逻辑单元测试（不同 RAM 规格、OLTP vs OLAP 配置差异） | 低 |

**调优参数覆盖**：
- `shared_buffers`（推荐 RAM 的 25%）
- `effective_cache_size`（推荐 RAM 的 75%）
- `work_mem`、`maintenance_work_mem`
- `wal_buffers`、`max_connections`
- `checkpoint_completion_target`
- `random_page_cost`（SSD：1.1，HDD：4.0）

---

### 阶段七：混沌测试

| 步骤 | 操作 | 风险 |
|------|------|------|
| 34 | 实现混沌测试引擎：内置场景（kill-primary、网络分区、磁盘写满、慢副本） | 高 |
| 35 | 实现 `pgdba chaos kill-node/partition/corrupt/report`，必须使用 `--i-know-what-i-am-doing` 标志 | 高 |
| 36 | 编写 E2E 混沌测试：启动集群 → 持续写入 → 杀死主库 → 验证切换 <30s → 验证零数据丢失 → 旧主重加入 | 中 |

**安全保障**：
- 要求 `--cluster` 参数指定测试集群
- 拒绝对标记为生产的集群执行破坏性操作
- 任何破坏性操作前自动创建备份

---

### 阶段八：查询优化与裸金属支持

| 步骤 | 操作 | 风险 |
|------|------|------|
| 37 | 实现查询分析命令（慢查询、EXPLAIN ANALYZE、索引建议） | 低 |
| 38 | 实现裸金属 Provider（基于 SSH，iptables 模拟网络分区） | 中 |
| 39 | 创建 Ansible Playbook（幂等部署 PG、Patroni、etcd、PgBouncer 的角色） | 中 |
| 40 | 编写完整生命周期 E2E 测试（init → 增加从库 → 备份 → 故障切换 → PITR 恢复 → 混沌测试 → 销毁） | 低 |

---

### 阶段九：Kubernetes 支持（新增）

| 步骤 | 操作 | 风险 |
|------|------|------|
| 41 | 实现 Kubernetes Provider（`internal/provider/kubernetes.go`，使用 client-go） | 高 |
|    | — `CreateNode`：创建 StatefulSet Pod（通过 Helm install/upgrade） | |
|    | — `DestroyNode`：`kubectl delete pod`（StatefulSet 控制自动重建，配合 Patroni 触发切换） | |
|    | — `ExecOnNode`：`kubectl exec` | |
|    | — `PartitionNode`：动态创建 `NetworkPolicy` 实现网络隔离 | |
| 42 | 创建 Patroni K8s 模式配置模板（`patroni-k8s.yml.tmpl`，DCS 设置为 kubernetes） | 中 |
|    | — Patroni 使用 K8s ConfigMap 存储集群状态，Endpoints 实现领导者选举 | |
|    | — 无需独立 etcd，依赖 K8s API Server | |
| 43 | 创建 Helm Chart（`deployments/kubernetes/helm/`） | 中 |
|    | — StatefulSet：每个 Pod 运行 PG + Patroni Sidecar 模式 | |
|    | — Service：Headless Service（StatefulSet 内部通信）+ RW Service（指向主库）+ RO Service（指向从库） | |
|    | — RBAC：Patroni 读写 ConfigMap 和 Endpoints 的权限 | |
|    | — PVC：为每个 Pod 分配独立 PersistentVolumeClaim | |
| 44 | 实现 `pgdba cluster init --provider kubernetes --namespace X --replicas 2` | 高 |
|    | — 调用 `helm install` 部署集群 | |
|    | — 等待所有 Pod Ready 且 Patroni 选出 Leader | |
|    | — 验证流复制建立 | |
| 45 | 适配混沌测试 K8s 场景 | 高 |
|    | — `kill-node`：`kubectl delete pod` + 监控 Patroni 自动切换 | |
|    | — `partition`：创建拒绝特定 Pod 入站/出站的 NetworkPolicy | |
|    | — `corrupt`：写入垃圾数据后验证恢复流程 | |
| 46 | 适配备份调度 K8s 场景 | 中 |
|    | — `backup schedule` 在 K8s 模式下创建 K8s CronJob 而非系统 cron | |
|    | — 备份存储支持 PVC 或 S3 兼容对象存储 | |
| 47 | 编写 K8s 集成测试（使用 kind 或 k3s 轻量集群） | 中 |
|    | — `tests/integration/testutil/k8s_helper.go`：自动启动 kind 集群 | |
|    | — 测试完整的 init → status → failover → backup → destroy 流程 | |
| 48 | 编写 K8s E2E 混沌测试 | 中 |
|    | — `tests/e2e/k8s_lifecycle_test.go`：删除主库 Pod → 验证自动故障切换 → 旧主 Pod 重建后作为从库加入 | |
| 49 | 完善 README（K8s 快速入门、Helm values 参考、K8s 特有注意事项） | 低 |

---

## 测试策略

| 层级 | 测试内容 | 目标覆盖率 |
|------|----------|-----------|
| 单元测试 | 输出格式化、配置加载、调优计算、Patroni 配置生成 | ≥90% |
| 集成测试 | 集群 init/destroy、故障切换、备份/恢复（Docker Provider）；接管已有集群（connect） | ≥80% |
| K8s 集成测试 | K8s 集群 init/failover/backup（kind 集群） | ≥80% |
| E2E 测试 | 完整生命周期（Docker）、混沌场景、K8s 全流程 | 关键路径 100% |

- Docker 集成测试：CI 使用 GitHub Actions `services: docker`
- K8s 集成测试：CI 使用 `kind`（Kubernetes in Docker），在 GitHub Actions 中自动启停

---

## 风险评估

| 风险 | 严重程度 | 缓解措施 |
|------|----------|----------|
| **故障切换时脑裂** | 严重 | Patroni + etcd/K8s API 提供隔离机制；PgBouncer 只路由到 Patroni 确认的主库；混沌测试验证 |
| **故障切换时数据丢失** | 严重 | 提供同步复制模式选项（`--sync-mode`），默认异步并监控复制延迟 |
| **pg_rewind 重加入失败** | 高 | 自动回退到 pg_basebackup，记录使用了哪种方法 |
| **混沌测试误操作生产环境** | 高 | 要求 `--cluster` 指向测试集群，拒绝生产标签集群，操作前强制备份 |
| **K8s RBAC 权限配置错误** | 高 | Patroni 需要读写 ConfigMap 和 Endpoints；RBAC 模板经过最小权限原则审查；CI 测试验证权限 |
| **K8s NetworkPolicy 分区效果不完全** | 中 | 依赖 CNI 插件（Calico/Cilium）支持 NetworkPolicy；kind 测试环境默认使用 kindnetd，需额外配置 |
| **K8s PVC 跨节点不可用** | 中 | 使用 `ReadWriteOnce`（单节点挂载），StatefulSet 保证 Pod 与 PVC 绑定；生产建议使用支持多副本的存储类 |
| **`cluster connect` 接管不完整的集群** | 中 | `connect` 命令执行连通性预检：Patroni API 可达、PG 可连接、复制状态正常，失败时输出明确错误 |
| **Patroni REST API 不可用** | 中 | 指数退避重试，只读操作回退到直接 PG 查询 |
| **Docker Provider 局限性** | 中 | 文档说明（macOS Docker Desktop 无法真实模拟网络分区），生产混沌测试推荐裸金属或 K8s |
| **配置文件泄露密钥** | 中 | 密钥仅从环境变量加载，配置文件只引用变量名，启动时验证必要环境变量已设置 |
| **Provider 接口设计僵化** | 中 | 先基于 Docker 实现确定接口，再抽象；K8s Provider 实现前完成接口稳定版本 |

---

## 验收标准

### 通用标准
- [ ] 所有 CLI 命令输出合法 JSON，可供 AI 代理直接解析
- [ ] 单元测试覆盖率 ≥80%
- [ ] 代码库中无任何硬编码密钥

### Docker 场景（阶段一～七）
- [ ] `pgdba cluster init` 在 Docker 环境下 3 分钟内部署 1主+2从 Patroni 集群
- [ ] `pgdba cluster status` 以结构化 JSON 返回准确的拓扑信息
- [ ] `pgdba failover trigger` 在 30 秒内完成故障切换
- [ ] `pgdba backup create` + `pgdba backup restore --target-time` 实现 PITR 时间点恢复
- [ ] `pgdba chaos kill-node`（同步模式）触发自动切换，零数据丢失
- [ ] `pgdba config tune` 产出等效于 PGTune 的调优建议
- [ ] `pgdba health check` 以 JSON 格式返回全面的健康报告
- [ ] 集成测试在 CI（Docker 环境）中全部通过

### 接管已有集群（阶段二）
- [ ] `pgdba cluster connect` 成功接入已有 Patroni 集群，输出集群信息
- [ ] 接管后 `failover trigger`、`backup create`、`health report` 等命令正常工作
- [ ] 对 connect 接入的集群执行 `cluster destroy` 时返回明确错误，拒绝操作

### 裸金属场景（阶段八）
- [ ] `pgdba cluster init --provider baremetal` 通过 SSH + Ansible 部署集群
- [ ] 集成测试覆盖裸金属 Provider 核心操作

### Kubernetes 场景（阶段九）
- [ ] `pgdba cluster init --provider kubernetes` 通过 Helm 部署 K8s StatefulSet 集群
- [ ] `pgdba chaos kill-node`（K8s 模式）删除主库 Pod，验证 Patroni 自动故障切换
- [ ] `pgdba backup schedule`（K8s 模式）创建 K8s CronJob
- [ ] K8s 集成测试（kind 集群）在 CI 中全部通过

---

## 推荐实施顺序

**阶段一**（基础）是一切的前提，必须优先完成。

**阶段二**（集群生命周期）交付核心价值，同时实现 `cluster connect` 接管能力。

**阶段三**（故障切换）是高可用场景的关键，优先级仅次于阶段二。

**阶段四**（备份）和**阶段五**（监控）可由不同成员并行推进。

**阶段六**（配置调优）和**阶段七**（混沌测试）互相独立，可并行开发。

**阶段八**（裸金属）和**阶段九**（Kubernetes）互相独立，可并行开发。

每个阶段均可独立合并并交付增量价值：
- 阶段一+二 = 可用的集群管理工具 + 接管已有集群
- 加上阶段三 = 生产可用的高可用方案
- 加上阶段四+五 = 具备备份与可观测性的完整方案
- 加上阶段八或九 = 扩展到裸金属或 Kubernetes 部署
