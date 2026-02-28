# pgdba 项目现状报告（Codex）

更新日期：2026-02-28  
当前分支：`codex/status-summary`

## 1. 总结结论

- 项目整体方向清晰，CLI 架构和基础能力（健康检查、集群接管、状态查询、受控切换、从库提升）已经具备可用性。
- “Phase 4 已完成”在文档层面成立，但在实现层面仍有多处占位或半实现，尤其是 `config tune`、`inspect --delta`、`baseline diff`。
- 当前最适合 Codex 的贡献方式是：先补齐 Phase 4 的执行闭环和测试闭环，再推进 Phase 5（备份/PITR）。

## 2. 架构与代码组织

- 入口清晰：`cmd/pgdba/main.go` -> `internal/cli/root.go`。
- 命令层按领域拆分：`health/cluster/failover/replica/inspect/config/query/baseline`。
- 领域能力按包分离：`inspect/`、`tuning/`、`query/`、`patroni/`、`cluster/`、`provider/`。
- 输出采用统一响应信封：`internal/output/types.go` + `internal/output/formatter.go`。

## 3. 已落地且可直接继续扩展的能力

- `cluster connect/status/destroy(external 保护)`、`failover trigger/status`、`replica list/promote` 主链路已实现。
- Patroni API 客户端能力完整（`/cluster`, `/patroni`, `/switchover`, `/failover`, `/restart` 等）。
- `inspect` 已有版本感知和降级策略；`tuning`、`query` 包提供了独立可测的算法/接口层。
- Docker Compose 本地 HA 栈齐全（etcd x3 + spilo x3 + pgbouncer），可作为集成验证环境。

## 4. 关键现状差距（重点）

### P1：功能“存在命令但未闭环”

- `cluster init` 仍直接返回未实现错误，`provider` 抽象尚未被真正接入执行流。
- `config tune --apply/--dry-run` 目前仅拼接返回结果，没有调用 `tuning.DryRun/Apply/Rollback` 真正执行。
- `inspect --delta` 和 `baseline --delta` 虽有参数，但采集器未使用 `SamplingConfig` 做双采样/差分。
- `baseline diff` 目前仅回显 before/after JSON，并未产生结构化差异结果。
- `baseline collect --sections` 参数已声明但未生效。

### P1：行为与参数不一致

- `query top --sort` 标志位对底层查询排序未生效（底层固定按 `total_exec_time`）。
- `query index-suggest --table` 通过字符串拼接构造 SQL，缺少参数化或标识符安全处理。

### P2：配置与运行时治理

- `--config` 标志默认值文案是 `~/.pgdba/config.yaml`，但实际仅在显式传入路径时才读取配置文件。
- `config.Validate()` 在运行时未调用（仅在测试中使用）。
- 根命令里 `--provider`、`--verbose` 目前未参与实际行为分支。

### P2：文档与实现不一致

- `docs/CONTRIBUTING.md` 仍提到 `output.Error()` / `output.PrintResult()`，代码中实际为 `output.Failure()` / `output.Success()` + `FormatResponse()`。
- README 里 E2E 数量存在多个版本（38 / 46），与当前代码中的测试数量不一致。

## 5. 测试与 CI 现状

- 静态统计测试函数数量：Unit `202`，E2E `39`，Integration `7`（总计 `248`）。
- E2E/Integration 覆盖重点仍在 Phase 1-3（cluster/failover/replica）；Phase 4 的端到端覆盖偏弱，更多是 help/参数校验层面的单测。
- CI 目前只跑 `make coverage` + `make build`，并未执行 `make test-e2e` 或 `make test-integration`。
- 本地环境执行受限：当前终端缺少 `go` 命令，无法在本次分析中实际跑通测试。

## 6. 建议的 Codex 贡献路线（按优先级）

1. **补齐 `config tune` 执行闭环**  
   接通 `tuning.DryRun/Apply/Rollback`，接入 lock 机制，补全 apply 前后验证与错误回滚路径。

2. **实现真实 delta 与 diff 能力**  
   在 `inspect.Collect` 中实现 instant/delta 双路径，完成 `baseline diff` 的字段级对比结构输出，落地 `--sections` 过滤。

3. **修复查询分析行为一致性**  
   让 `query top --sort` 真正驱动排序；`index-suggest --table` 改为安全查询构造。

4. **补齐配置加载与校验链路**  
   实现默认配置路径读取、运行时调用 `config.Validate()`，并清理未使用的全局参数行为。

5. **升级测试与 CI 策略**  
   为 Phase 4 增加 E2E/集成测试；CI 增加 `test-e2e`（可先无 integration）以降低回归风险。

## 7. 本次操作记录

- 已创建分支：`codex/status-summary`
- 已新增本文件：`status.md`
- 未修改其他业务代码
