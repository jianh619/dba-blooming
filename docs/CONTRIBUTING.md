# 贡献指南

## 开发环境准备

### 必要工具

| 工具 | 版本要求 | 用途 |
|------|----------|------|
| Go | 1.22+ | 编译与测试 |
| Docker | 20.10+ | 集成测试与本地 HA 栈 |
| Docker Compose | v2 | `docker compose` 命令 |
| golangci-lint | latest | 代码风格检查（`make lint` 自动安装） |

### 克隆与验证

```bash
git clone https://github.com/jianh619/dba-blooming.git
cd dba-blooming

# 编译
make build
# 输出: ./bin/pgdba

# 运行单元测试（无任何外部依赖）
make test-unit
# 预期: 202 tests PASS

# 运行 E2E 测试（无外部依赖，使用 httptest mock）
make test-e2e
# 预期: 38 tests PASS

# 代码风格检查
make lint
```

### 使用 Claude Code 继续开发

本项目包含 `CLAUDE.md` 文件，Claude Code 启动时会自动加载该文件获取项目上下文。在新目录下克隆项目后直接使用 Claude Code 即可继续开发：

```bash
git clone https://github.com/jianh619/dba-blooming.git
cd dba-blooming
claude   # Claude Code 自动读取 CLAUDE.md，了解项目架构、进度和规范
```

Claude Code 需要了解的关键文件：
- `CLAUDE.md` — 项目上下文（架构、进度、规范）
- `plan.md` — 完整 9 阶段实施计划
- `plan-phase4.md` — Phase 4 详细设计（含架构反馈追踪）
- `README.md` — 用户文档与命令参考

---

## 开发流程

### TDD（测试驱动开发）— 强制执行

所有新功能和 bug 修复必须遵循 TDD 流程：

```
1. RED    — 先写测试，运行确认失败
2. GREEN  — 写最小实现使测试通过
3. REFACTOR — 重构优化，确保测试仍通过
```

示例流程：

```bash
# 1. 写测试
vim tests/unit/my_new_feature_test.go

# 2. 确认测试失败（RED）
make test-unit  # 预期：编译失败或测试 FAIL

# 3. 写实现
vim internal/mypackage/feature.go

# 4. 确认测试通过（GREEN）
make test-unit  # 预期：PASS

# 5. 重构（REFACTOR）
# 优化代码，再次运行测试确认通过
make test-unit
```

### 测试分层

| 层级 | 文件位置 | 运行命令 | 外部依赖 |
|------|----------|----------|----------|
| 单元测试 | `tests/unit/*_test.go` | `make test-unit` | 无 |
| E2E 测试 | `tests/e2e/e2e_test.go` | `make test-e2e` | 无（httptest mock） |
| 集成测试 | `tests/integration/integration_test.go` | `make test-integration` | Docker Compose 集群 |

**单元测试**：使用 mock 注入（`inspect.DB`、`tuning.ApplyDB` 等接口），不依赖真实数据库。

**E2E 测试**：编译实际二进制，用 `httptest.NewServer` 模拟 Patroni API，黑盒验证 CLI 行为。

**集成测试**：需要运行 Docker Compose 集群（`deployments/docker/`），使用 `//go:build integration` tag。

### 覆盖率要求

```bash
make coverage
# 要求整体 ≥80%
```

核心逻辑模块覆盖率目标更高：
- `internal/inspect/` — ≥80%
- `internal/tuning/` — ≥80%
- `internal/patroni/` — ≥85%
- `internal/failover/` — ≥90%
- `internal/output/` — ≥90%

---

## 代码规范

### 文件组织
- 按功能域组织（`inspect/`、`tuning/`、`query/`），而非按类型
- 单文件 < 800 行，单函数 < 50 行
- 高内聚低耦合：每个文件专注一个职责

### 接口设计
- 用接口实现可测试性：`inspect.DB`、`tuning.ApplyDB`、`query.DB`
- 真实实现在 `pgxdb.go`（或类似命名），仅用于生产连接
- 测试中使用 mock 实现注入

### 错误处理
- 每个错误必须显式处理
- 用 `fmt.Errorf("context: %w", err)` 包装错误
- CLI 层统一用 `output.Error()` 输出错误信封
- 不要静默吞掉错误

### 安全
- 密码只从 `PGDBA_PG_PASSWORD` 环境变量读取
- 配置文件（config.yaml, clusters.json）中不存储任何密钥
- Docker 端口绑定 `127.0.0.1`
- 敏感文件权限 `0600`

### 输出格式
所有 CLI 命令使用统一 JSON 信封（`internal/output/types.go`）：

```json
{
  "success": true,
  "timestamp": "2026-02-28T10:00:00Z",
  "command": "health check",
  "data": { ... }
}
```

---

## 新增 CLI 命令检查清单

添加新命令时：

1. [ ] 在 `internal/cli/` 创建命令文件（如 `backup.go`）
2. [ ] 在 `internal/cli/root.go` 注册命令
3. [ ] 使用 `output.PrintResult()` 输出统一信封
4. [ ] 先写单元测试和/或 E2E 测试
5. [ ] 更新 README.md 命令参考表（`<!-- AUTO-GENERATED: command-reference-start -->` 区域内）
6. [ ] 更新 CLAUDE.md 关键文件索引（如添加了新包）

---

## 提交 PR

### commit 消息格式

```
<type>: <description>
```

类型：`feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `ci`

示例：
```
feat: implement backup create command with pg_basebackup
fix: handle Patroni leader role in cluster status
test: add integration tests for failover trigger
```

### PR 提交前检查清单

- [ ] `make test-unit` 全部通过（race detector 启用）
- [ ] `make test-e2e` 全部通过
- [ ] `make lint` 无报错
- [ ] `make coverage` ≥ 80%
- [ ] 无硬编码密钥
- [ ] 新命令已更新 README.md 命令参考表
- [ ] 新包已更新 CLAUDE.md 文件索引

---

## 运行集成测试

集成测试需要 Docker Compose 集群：

```bash
# 1. 准备密码
cd deployments/docker
cp .env.example .env
# 编辑 .env，填入 POSTGRES_PASSWORD 和 REPLICATION_PASSWORD

# 2. 启动集群
docker compose up -d

# 3. 等待 Patroni 选主（约 30-60 秒）
until curl -sf http://localhost:8008/health > /dev/null; do
  echo "等待 Patroni..."
  sleep 5
done

# 4. 运行集成测试
cd ../..
make test-integration

# 5. 清理
cd deployments/docker && docker compose down -v
```

详细部署说明见 `deployments/docker/DEPLOY_README.md`。

---

## 项目路线图

当前已完成 Phase 1-4，后续阶段参考 `plan.md`：

| 阶段 | 内容 | 优先级 |
|------|------|--------|
| Phase 5 | 备份与 PITR 恢复（pg_basebackup, pg_dump） | 高 |
| Phase 6 | 物理备份与归档（WAL-G / pgBackRest） | 高 |
| Phase 7 | 监控告警（Prometheus + Grafana + postgres_exporter） | 中 |
| Phase 8 | 裸金属部署（Ansible + SSH Provider） | 中 |
| Phase 9 | Kubernetes 支持（Helm + client-go + K8s Provider） | 低 |

Phase 5-6 可并行开发，Phase 8-9 可并行开发。
