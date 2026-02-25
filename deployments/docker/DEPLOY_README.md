# Docker Compose 本地部署指南

本文档说明如何在本地使用 Docker Compose 部署一套完整的 PostgreSQL HA 集群，供开发和测试使用。

---

## 部署架构

```
                        ┌─────────────────────────────────────────────────────┐
                        │                   pgdba-net (bridge)                │
                        │                                                     │
  应用 / psql           │  ┌─────────────┐    ┌──────────────────────────┐   │
  ──────────────────────┼──► PgBouncer   │    │   etcd 集群（DCS）        │   │
  127.0.0.1:6432        │  │ :6432       │    │                          │   │
  (连接池，推荐入口)     │  │ 连接池      │    │  etcd1 :2379/:2380       │   │
                        │  │ transaction │    │  etcd2 :2379/:2380       │   │
                        │  │ pool mode   │    │  etcd3 :2379/:2380       │   │
                        │  └──────┬──────┘    │                          │   │
                        │         │           │  3节点Raft保证高可用       │   │
                        │         ▼           └──────────┬───────────────┘   │
                        │  ┌──────────────┐              │ 选主锁/心跳        │
  直连主库（调试用）      │  │  pg-primary  │◄─────────────┘                   │
  127.0.0.1:5432        │  │  :5432/:8008 │                                  │
  127.0.0.1:8008(API)  ─┼──►  Spilo       │ 流复制                           │
                        │  │  (PG+Patroni)│──────────────────┐               │
                        │  │  Leader      │                  │               │
                        │  └──────────────┘                  │               │
                        │                        ┌───────────▼───────────┐   │
  直连从库（只读，调试）  │                        │  pg-replica-1  :5433  │   │
  127.0.0.1:5433       ─┼────────────────────────►  pg-replica-2  :5434  │   │
  127.0.0.1:5434        │                        │  Spilo (PG+Patroni)   │   │
                        │                        │  Replica              │   │
                        │                        └───────────────────────┘   │
                        └─────────────────────────────────────────────────────┘
```

### 组件说明

#### etcd 集群（3节点）
- **作用**：分布式配置存储（DCS），Patroni 通过抢占 etcd 中的 leader 锁来决定谁是主库
- **为什么需要3节点**：Raft 协议要求超过半数节点存活才能工作（容忍1节点故障）
- **镜像**：`quay.io/coreos/etcd:v3.5.9`
- **数据持久化**：`etcd1-data`、`etcd2-data`、`etcd3-data` named volumes，容器重启数据不丢失

#### pg-primary / pg-replica-1 / pg-replica-2（Spilo）
- **作用**：运行 PostgreSQL 16 + Patroni 的一体化容器
- **Spilo**：Zalando（Patroni 创建者）维护的官方参考镜像，无需自建
- **Patroni**：负责 PG 高可用编排：
  - 启动时竞争 etcd leader 锁，抢到的成为 primary，其余成为 replica
  - 持续向 etcd 发送心跳，心跳中断则其他节点发起选举
  - 主库宕机时，Patroni 自动提升一个从库并通知其余节点切换复制源
- **REST API**（`:8008`）：对外暴露节点状态，`pgdba cluster status` 即调用此接口
- **流复制**：primary → replica-1 / replica-2，WAL 实时同步

#### PgBouncer（连接池）
- **作用**：在应用与 PostgreSQL 之间做连接复用，减少 PG 连接数压力
- **为什么需要**：PG 每个连接占用约 5-10MB 内存，`max_connections` 通常 100-200；PgBouncer 允许数千应用连接复用少量 PG 连接
- **Pool mode — transaction**：事务结束即归还连接，适合 OLTP 高并发场景
- **镜像**：`edoburu/pgbouncer:v1.25.1-p0`（社区最广泛使用的 PgBouncer Docker 镜像）
- **始终指向 primary**：PgBouncer 配置写死连接 `pg-primary`，故障切换后 Patroni 会在同一容器名下恢复服务（Docker Compose 场景）

### 端口映射

| 服务 | 宿主机端口 | 容器端口 | 说明 |
|------|-----------|---------|------|
| PgBouncer | `127.0.0.1:6432` | 6432 | **推荐应用入口**，连接池 |
| pg-primary | `127.0.0.1:5432` | 5432 | 主库直连（调试用） |
| pg-primary Patroni | `127.0.0.1:8008` | 8008 | Patroni REST API |
| pg-replica-1 | `127.0.0.1:5433` | 5432 | 从库直连（只读，调试用） |
| pg-replica-1 Patroni | `127.0.0.1:8009` | 8008 | Patroni REST API |
| pg-replica-2 | `127.0.0.1:5434` | 5432 | 从库直连（只读，调试用） |
| pg-replica-2 Patroni | `127.0.0.1:8010` | 8008 | Patroni REST API |

> 所有端口均绑定 `127.0.0.1`，仅本机可访问，不对外暴露。

### 启动顺序依赖

```
etcd1 ─┐
etcd2 ─┼──► pg-primary ──► pg-replica-1
etcd3 ─┘              └──► pg-replica-2
                                │
                          pg-primary ──► pgbouncer
```

---

## 部署前置条件

- Docker 20.10+
- Docker Compose v2（`docker compose` 命令）
- 可用内存：建议 4GB 以上（7个容器同时运行）
- 网络：首次运行需拉取约 1.5GB 镜像

---

## 部署步骤

### 第一步：准备密码配置

```bash
cd deployments/docker

# 复制模板（.env 已加入 .gitignore，不会被提交到 git）
cp .env.example .env
```

编辑 `.env`，填入真实密码：

```bash
# deployments/docker/.env
POSTGRES_PASSWORD=your_strong_password_here
REPLICATION_PASSWORD=your_replication_password_here
```

> 密码要求：长度 ≥12 位，包含大小写字母和数字。

### 第二步：启动所有服务

```bash
docker compose up -d
```

首次运行会拉取镜像（约 1-5 分钟，取决于网速）。启动后可查看容器状态：

```bash
docker compose ps
```

期望输出（所有容器 `Status` 为 `Up`）：

```
NAME           IMAGE                              STATUS
etcd1          quay.io/coreos/etcd:v3.5.9         Up
etcd2          quay.io/coreos/etcd:v3.5.9         Up
etcd3          quay.io/coreos/etcd:v3.5.9         Up
pg-primary     ghcr.io/zalando/spilo-16:3.3-p3    Up
pg-replica-1   ghcr.io/zalando/spilo-16:3.3-p3    Up
pg-replica-2   ghcr.io/zalando/spilo-16:3.3-p3    Up
pgbouncer      edoburu/pgbouncer:v1.25.1-p0        Up
```

### 第三步：等待 Patroni 完成初始化

Patroni 需要约 **30-60 秒**完成 etcd 选主和 PostgreSQL 初始化。

```bash
# 实时观察主库日志，等待选主完成
docker compose logs -f pg-primary
```

看到以下日志表示初始化成功：

```
pg-primary  | pg-primary: promoted to leader by becoming master
pg-primary  | server started
```

或直接轮询 Patroni REST API：

```bash
# 等待返回 HTTP 200
until curl -sf http://localhost:8008/health > /dev/null; do
  echo "等待 Patroni 就绪..."
  sleep 5
done
echo "Patroni 已就绪"
```

### 第四步：验证集群状态

```bash
# 查询集群拓扑（原始 Patroni API）
curl -s http://localhost:8008/cluster | python3 -m json.tool
```

期望输出：

```json
{
  "members": [
    {
      "name": "pg-primary",
      "role": "leader",
      "state": "running",
      "host": "pg-primary",
      "port": 5432,
      "lag": 0
    },
    {
      "name": "pg-replica-1",
      "role": "replica",
      "state": "running",
      "host": "pg-replica-1",
      "port": 5432,
      "lag": 0
    },
    {
      "name": "pg-replica-2",
      "role": "replica",
      "state": "running",
      "host": "pg-replica-2",
      "port": 5432,
      "lag": 0
    }
  ]
}
```

```bash
# 用 pgdba 接管集群（在项目根目录执行）
cd ../..
./bin/pgdba cluster connect \
  --name local-ha \
  --patroni-url http://localhost:8008 \
  --pg-host localhost

# 通过 pgdba 查看集群状态
./bin/pgdba cluster status --name local-ha
```

### 第五步：连接数据库

```bash
# 通过 PgBouncer 连接（推荐，生产模式）
psql -h 127.0.0.1 -p 6432 -U postgres postgres

# 直连主库（调试用）
psql -h 127.0.0.1 -p 5432 -U postgres postgres

# 连接从库（只读查询）
psql -h 127.0.0.1 -p 5433 -U postgres postgres
psql -h 127.0.0.1 -p 5434 -U postgres postgres
```

### 第六步：验证主从复制

登录主库后执行：

```sql
-- 查看从库连接状态
SELECT client_addr, state, sent_lsn, replay_lsn,
       (sent_lsn - replay_lsn) AS replication_lag
FROM pg_stat_replication;
```

期望看到 2 行，分别对应 pg-replica-1 和 pg-replica-2。

---

## 日常操作

### 查看日志

```bash
# 所有服务
docker compose logs -f

# 单个服务
docker compose logs -f pg-primary
docker compose logs -f pg-replica-1
docker compose logs -f pgbouncer
docker compose logs -f etcd1
```

### 停止与重启

```bash
# 停止所有服务（保留数据）
docker compose stop

# 重新启动
docker compose start

# 停止并删除容器（保留 named volumes 中的数据）
docker compose down

# 完全清理（删除所有数据，回到初始状态）
docker compose down -v
```

### 扩缩容

```bash
# 临时停止某个从库（模拟节点故障）
docker compose stop pg-replica-1

# 恢复
docker compose start pg-replica-1
```

---

## 故障切换验证

### 模拟主库故障

```bash
# 停止主库
docker compose stop pg-primary

# 等待 Patroni 自动选主（约 30 秒）
sleep 30

# 查看哪个从库被提升为主库
curl -s http://localhost:8009/cluster | python3 -m json.tool
# 或
curl -s http://localhost:8010/cluster | python3 -m json.tool
```

### 手动触发 Switchover（无损切换）

```bash
# 触发 Switchover（当前主库优雅让出 leader 角色）
curl -s -XPOST -H "Content-Type: application/json" \
  http://localhost:8008/switchover \
  -d '{"leader": "pg-primary"}' | python3 -m json.tool

# 验证新主库
curl -s http://localhost:8008/cluster | python3 -m json.tool
```

---

## 常见问题

### 容器启动后立即退出

```bash
# 查看详细错误日志
docker compose logs pg-primary
```

常见原因：
- `.env` 文件未创建或密码为空 → 检查 `.env` 文件
- etcd 还未就绪，Patroni 连接失败 → 等待 30 秒后查看日志

### PgBouncer 连接失败

```bash
# 检查 pgbouncer 日志
docker compose logs pgbouncer
```

常见原因：
- `pg-primary` 还未完成初始化 → 等待主库就绪后 PgBouncer 会自动重连
- 密码错误 → 检查 `.env` 中的 `POSTGRES_PASSWORD`

### etcd 集群无法选主

```bash
docker compose logs etcd1
```

常见原因：
- 三个 etcd 容器未全部启动 → 等待所有 etcd 容器 `Up` 后重试

### 端口冲突

```
Error: Bind for 127.0.0.1:5432 failed: port is already allocated
```

本机已有 PostgreSQL 运行，停止本机 PG 后再启动：

```bash
# Ubuntu/Debian
sudo systemctl stop postgresql

# macOS (Homebrew)
brew services stop postgresql@16
```

### 重置集群（删除所有数据重新开始）

```bash
docker compose down -v
docker compose up -d
```

---

## 镜像说明

| 镜像 | 版本 | 说明 |
|------|------|------|
| `quay.io/coreos/etcd` | `v3.5.9` | etcd 官方镜像，分布式 KV 存储，Patroni 的 DCS 后端 |
| `ghcr.io/zalando/spilo-16` | `3.3-p3` | Zalando 官方 PG16+Patroni 一体镜像，无需自建 |
| `edoburu/pgbouncer` | `v1.25.1-p0` | 社区最广泛使用的 PgBouncer 镜像，轻量可配置 |

### 为什么选这些镜像

- **Spilo**：Patroni 的创建者（Zalando）维护，在 AWS 大规模生产验证，支持 PG16 + etcd3，无需自己在 `postgres:16` 上安装 Patroni
- **edoburu/pgbouncer**：环境变量直接映射 pgbouncer.ini 参数，配置直观；官方 `pgbouncer/pgbouncer` 镜像已 5 年未更新
- **quay.io/coreos/etcd**：CoreOS（etcd 原班人马）维护，版本固定（`v3.5.9`），确保构建可复现

---

## 生产环境注意事项

本 Docker Compose 配置面向**本地开发和测试**，生产环境需额外关注：

1. **TLS 加密**：etcd 节点间通信和客户端访问应启用 TLS，当前使用 HTTP 明文
2. **Patroni REST API 认证**：当前无认证，生产环境应配置 Basic Auth 或 TLS 客户端证书
3. **pg_hba.conf**：当前允许 Docker 内网段 `172.16.0.0/12` 访问，生产应进一步收紧
4. **数据持久化**：etcd 使用 named volumes，PG 数据在容器内，生产应挂载宿主机目录或使用云存储
5. **备份**：需配置 WAL-G 或 pgBackRest 进行定期备份和 PITR
6. **监控**：需集成 Prometheus + Grafana 监控 PG 和 Patroni 指标
7. **PgBouncer 高可用**：单点 PgBouncer 是潜在瓶颈，生产应部署多个实例配合 HAProxy
