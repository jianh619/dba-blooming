# 运维手册（Runbook）

本文档面向运维人员和开发者，涵盖 pgdba 工具的部署、日常操作、故障排查和应急处理。

---

## 部署

### 编译安装

```bash
git clone https://github.com/jianh619/dba-blooming.git
cd dba-blooming
make build
# 二进制: ./bin/pgdba
# 可复制到 /usr/local/bin/pgdba 供全局使用
```

### 配置

pgdba 支持三种配置方式（优先级从高到低）：

1. **命令行参数**（如 `--name`, `--pg-host`）
2. **环境变量**（`PGDBA_PG_HOST`, `PGDBA_PG_PASSWORD` 等）
3. **配置文件**（`~/.pgdba/config.yaml`）

```bash
# 最小配置（环境变量）
export PGDBA_PG_HOST=10.0.0.1
export PGDBA_PG_PORT=5432
export PGDBA_PG_USER=postgres
export PGDBA_PG_PASSWORD=<your-password>

# 验证连接
pgdba health check
```

**密码安全**：`PGDBA_PG_PASSWORD` 是获取密码的唯一途径。配置文件和注册表中不存储密码。

---

## 本地 HA 集群部署（Docker Compose）

详细步骤见 `deployments/docker/DEPLOY_README.md`。快速启动：

```bash
cd deployments/docker
cp .env.example .env
# 编辑 .env 填入密码

docker compose up -d
sleep 30  # 等待 Patroni 选主

# 验证
curl -s http://localhost:8008/cluster | python3 -m json.tool

# 用 pgdba 接管
cd ../..
./bin/pgdba cluster connect --name local-ha --patroni-url http://localhost:8008 --pg-host localhost
./bin/pgdba cluster status --name local-ha
```

### 栈组成

| 服务 | 端口（127.0.0.1） | 作用 |
|------|-------------------|------|
| etcd1/2/3 | 内部通信 | 分布式配置存储，Patroni 领导者选举 |
| pg-primary | 5432, 8008 | PostgreSQL 主库 + Patroni REST API |
| pg-replica-1 | 5433, 8009 | 从库 + Patroni REST API |
| pg-replica-2 | 5434, 8010 | 从库 + Patroni REST API |
| pgbouncer | 6432 | 连接池（推荐应用入口） |

---

## 日常操作

### 健康检查

```bash
# 基本健康检查
pgdba health check

# 带格式输出
pgdba health check --format table
```

### 查看集群状态

```bash
pgdba cluster status --name <cluster-name>
# 或直接指定 Patroni URL
pgdba cluster status --patroni-url http://10.0.0.1:8008
```

### 配置调优

```bash
# 查看当前配置
pgdba config show --name <cluster>

# 对比推荐值
pgdba config diff --name <cluster> --workload oltp --ram-gb 16 --cpu-cores 4

# 预览变更（不实际应用）
pgdba config tune --name <cluster> --workload oltp --ram-gb 16 --dry-run

# 应用变更
pgdba config tune --name <cluster> --workload oltp --ram-gb 16 --apply
```

### 查询分析

```bash
# Top N 慢查询（需要 pg_stat_statements）
pgdba query top --name <cluster> --limit 20

# SQL 执行计划
pgdba query analyze --name <cluster> --sql "SELECT * FROM orders WHERE id = 1"

# 缺失索引建议
pgdba query index-suggest --name <cluster> --min-rows 10000

# 锁等待链
pgdba query locks --name <cluster>

# 表膨胀
pgdba query bloat --name <cluster>

# Vacuum 健康
pgdba query vacuum-health --name <cluster>
```

### 基线对比

```bash
# 采集基线
pgdba baseline collect --name <cluster> --save before.json

# 做变更后采集新基线
pgdba baseline collect --name <cluster> --save after.json

# 对比
pgdba baseline diff --before before.json --after after.json
```

---

## 故障切换

### 受控切换（Switchover）

主库正常运行时，优雅地将主库角色转移到指定从库。

```bash
# 自动选择最佳候选
pgdba failover trigger --name <cluster>

# 指定候选节点
pgdba failover trigger --name <cluster> --candidate pg-replica-1

# 查看切换状态
pgdba failover status --name <cluster>
```

### 强制故障转移（Failover）

主库不可达时使用，存在数据丢失风险。

```bash
pgdba failover trigger --name <cluster> --force --candidate pg-replica-1
```

### 从库管理

```bash
# 列出所有从库
pgdba replica list --name <cluster>

# 提升从库为主库
pgdba replica promote --name <cluster> --candidate pg-replica-1
```

---

## 故障排查

### pgdba 命令返回错误

所有命令输出标准 JSON 信封，`success: false` 时查看 `error` 字段：

```bash
pgdba health check 2>&1 | python3 -m json.tool
```

常见错误：
- `connection refused` — PG 不可达，检查 HOST/PORT/防火墙
- `password authentication failed` — 检查 PGDBA_PG_PASSWORD
- `cluster not found` — 集群未注册，使用 `cluster connect` 接管
- `Patroni unreachable` — Patroni API 不可达，检查 URL 和网络

### Docker Compose 集群问题

```bash
# 查看所有容器状态
docker compose ps

# 查看服务日志
docker compose logs -f pg-primary
docker compose logs -f pgbouncer
docker compose logs -f etcd1

# Patroni 集群状态
curl -s http://localhost:8008/cluster | python3 -m json.tool
```

常见问题：

| 问题 | 原因 | 解决方案 |
|------|------|----------|
| 容器启动后退出 | .env 密码为空 | 检查 `deployments/docker/.env` |
| PgBouncer 连接失败 | 主库未就绪 | 等待 Patroni 完成初始化（30-60s） |
| etcd 无法选主 | 节点未全部启动 | `docker compose up -d` 确保全部启动 |
| 端口冲突 | 本机已有 PG 运行 | 停止本机 PG：`sudo systemctl stop postgresql` |
| Patroni 不选主 | etcd 未就绪 | 等待 etcd 集群健康后重启 PG 容器 |

### 完全重置集群

```bash
cd deployments/docker
docker compose down -v   # 删除所有数据
docker compose up -d     # 重新创建
```

---

## 集群注册表

pgdba 在 `~/.pgdba/clusters.json` 中维护已注册集群：

```bash
# 查看注册表
cat ~/.pgdba/clusters.json | python3 -m json.tool

# 接管新集群
pgdba cluster connect --name <name> --patroni-url <url> --pg-host <host>
```

集群类型：
- **managed** — pgdba 创建的集群，可 init/destroy
- **external** — 通过 connect 接管的集群，拒绝 destroy 操作

---

## CI/CD

### GitHub Actions

`.github/workflows/ci.yml` 在 push/PR 时自动运行：

1. `go mod download` — 下载依赖
2. `make coverage` — 运行单元测试 + 覆盖率检查（≥80%）
3. `make build` — 编译二进制

### 本地 CI 模拟

```bash
make lint && make coverage && make build && make test-e2e
```

---

## 数据目录

| 路径 | 内容 |
|------|------|
| `~/.pgdba/config.yaml` | 全局配置 |
| `~/.pgdba/clusters.json` | 集群注册表 |
| `~/.pgdba/snapshots/<fingerprint>/` | 诊断快照和变更集 |
| `~/.pgdba/snapshots/<fingerprint>/.lock` | Apply/Rollback 互斥锁 |
