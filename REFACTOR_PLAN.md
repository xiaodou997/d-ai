# uni-ai-api 超级大重构 - 完整设计方案

> **激进重构原则**：不兼容历史数据，不留历史包袱，长期最优优先于短期修补。
> **总改动量预估**：后端 ~3500 行 / 前端 ~250 行 / 5 个 Phase 独立可提交。
>
> **⚠️ P0 策略已更新（2025-05）**：原计划 P0 采用 canonical 中间格式做 N×M 跨协议转换，实际实施时改为**严格 1:1 协议透传**——client_protocol 必须与 upstream_protocol 完全一致才允许路由，删除所有跨协议转换代码。详见 3.1 和第七章。

---

## 一、重构目标

1. **严格 1:1 协议透传**：客户端协议（由请求路径推导）必须与 deployment 的 `upstream_protocol` 完全一致才允许路由；不做任何跨协议转换，请求和响应字节原样透传。同模型多协议通过同一 endpoint 下挂多行 deployment（不同 `upstream_protocol`）实现。
2. **请求级 fallback 重试**：非流式 + 流式 pre-flush 阶段都支持跨 route 自动重试，最多 3 次 / 总预算 90s。
3. **多维评分路由**：cost / latency / load / health 加权打分，替代单一 priority+weighted random。
4. **统一健康模型**：错误分级 + 半开探测 + Deployment / Pool credential 共用接口。
5. **OAuth credential 级 sticky**：以最大化上游 prompt cache 命中率为目标。
6. **可观测可重放**：X-Route-Trace 响应头 + 失败请求 body 落盘 + admin Replay API。

---

## 二、关键决策汇总（已确认）

| # | 决策项 | 方案 |
|---|---|---|
| D1 | 重试粒度 | 非流式全做，流式仅 pre-flush 做 |
| D2 | Sticky 来源 | X-Conversation-Id（opt-in），不传即新会话 |
| D3 | Pool 粘性粒度 | 粘到具体 credential，凭据失效才重选 |
| D4 | 路由算法 | 多维评分（cost / latency / load / health） |
| D5 | 路由状态存储 | Redis 主 + 本地 5s 缓存 + Redis 不可用降级 priority+weighted |
| D6 | 熔断器 | 错误分级 + 半开探测 + 统一 HealthTracker 接口 |
| D7 | conv_id 兜底 | 按 OpenAI 标准（不传当新会话），不做 messages 指纹 |
| D8 | 重试计费 | Freeze 一次按候选最贵 route 估算，Confirm 按实际命中重算 |
| D9 | 错误追踪 | X-Route-Trace 响应头 + ai_request_payloads 失败落盘 + Replay API |
| D10 | 重试预算 | 最多 3 次，总预算 90s，单次 = min(route.timeout, deadline-now) |
| D11 | 单次超时 | 不加 route 级 override，全局预算 + endpoint timeout 即可 |
| D12 | 多 attempt 日志 | 主行只记最终结果 + `attempts_count`；attempt 明细落 payload 表 JSONB |
| D13 | 协议路由 | **严格 1:1**：client_protocol == upstream_protocol 才允许路由；不匹配返回 400 no_matching_deployment |
| D14 | 跨协议转换 | **已全部删除**：不再有 canonical 中间格式转换，网关只做字节透传 |
| D15 | count_tokens | 上游支持时透传，否则本地 tiktoken-go 估算 |

---

## 三、新架构总览

### 3.1 协议路由：严格 1:1 透传

> **⚠️ 已于 P0 阶段实施完毕**。原 N×M 跨协议转换方案已废弃。

**核心原则**：`client_protocol == upstream_protocol` 才允许路由。网关不做任何格式转换，请求体和响应体原样透传。

```
┌─────────────────── 客户端协议（由路径推导）────────────────┐
│  /v1/chat/completions      → openai_chat                  │
│  /v1/responses             → openai_responses             │
│  /v1/messages              → anthropic_messages           │
│  /v1/embeddings            → openai_embeddings            │
│  /v1/images/generations    → openai_images                │
│  /v1beta/models/{m}:action → gemini_generate/gemini_embeddings │
│  /v1/messages/count_tokens → (独立 handler，不走路由)       │
└────────────────────────┬────────────────────────────────────┘
                         ↓ formats.DetectClientProtocol
                         ↓ SQL WHERE ud.upstream_protocol = $clientProtocol
┌─────────────────── 上游协议（只匹配与客户端一致的）──────────┐
│  openai_chat / openai_responses / openai_embeddings        │
│  anthropic_messages                                         │
│  gemini_generate / gemini_embeddings                       │
└────────────────────────┬────────────────────────────────────┘
                         ↓ 请求/响应字节原样透传
                         ↓ 仅做：model 名替换 / OAuth body 改写 / SSE usage 提取
┌─────────────────── 客户端响应（与请求同协议）───────────────┐
│  错误体按客户端协议格式输出（Anthropic ↔ OpenAI 各自格式）     │
└────────────────────────────────────────────────────────────┘
```

**同模型多协议**：一个 endpoint 下挂多行 deployment（不同 `upstream_protocol`），路由 SQL 的 `client_protocol` 过滤确保各自命中正确的 deployment。

**OAuth pool 协议映射**：`ai_credential_pools.fixed_provider_type` 在运行时映射到对应的 `upstream_protocol`，由 `oauthFixedTypesForProtocol()` Go 端常量实现，COALESCE 写入 SQL：

| client_protocol | OAuth fixed_provider_type 池 |
|---|---|
| openai_responses | codex |
| anthropic_messages | claude_oauth |
| gemini_generate | gemini_cli, antigravity |
| 其它 | 空数组（SQL `ANY({})` 自动排除） |

**无匹配部署时**：返回 HTTP 400 + `no_matching_deployment` 错误码，消息体包含 client_protocol 和 model 信息。

### 3.2 Pipeline 新形态

```
AuthN → AuthZ → QuotaCheck
  → RouteCandidates(选出排序后的所有候选，不只选 1 条)
  → RateLimit
  → QuotaReserve(按候选最贵 route 估算上限)
  → URMFreeze(同上)
  → Execute (内部重试循环：选→打→判→重试，含 RelayToClient)
  → URMConfirm(按实际命中 route 的费率回算 + 退冻多余)
  → UsageLog
```

**Execute 内部三层拆分（关键解耦）**：

```go
type ExecuteStep struct {
    Scorer      RouteScorer
    Builder     UpstreamRequestBuilder
    Client      UpstreamClient
    Relay       ClientRelay
    Health      HealthTracker
    OAuthPool   OAuthCredentialPool
    Budget      RetryBudget // 3 次 + 90s
}

func (s *ExecuteStep) Execute(ctx, req) error {
    deadline := time.Now().Add(s.Budget.TotalTimeout)
    for attempt := 1; attempt <= s.Budget.MaxAttempts; attempt++ {
        cand := s.Scorer.Pick(req.Candidates, req.UsedCandidates, req.StickyHint)
        if cand == nil { return errNoMoreCandidates }

        upReq := s.Builder.Build(cand, req)
        perAttemptTimeout := min(cand.TimeoutMs, deadline.Sub(now))
        upResp, err := s.Client.Do(ctxWithDeadline, upReq, perAttemptTimeout)

        outcome := classifyOutcome(upResp, err)
        s.Health.RecordOutcome(cand.TargetID, outcome)
        req.Attempts = append(req.Attempts, AttemptRecord{...})

        switch outcome.Decision() {
        case decisionRetry:    continue       // 5xx/timeout/network/429
        case decisionRetryNewCred: // 401
            s.OAuthPool.MarkInvalid(...)
            continue
        case decisionGiveUp:   return apiError(...) // 4xx
        case decisionAccept:
            return s.Relay.Stream(ctx, req.W, upResp, req.ClientProtocol)
        }
    }
    return errBudgetExhausted
}
```

**关键变化**：`serving.Request` **移除 `W http.ResponseWriter` 和 `R *http.Request` 字段**，改由 handler 层持有 `RequestEnvelope` 传给 `Relay.Stream`。Pipeline Request 变成纯数据结构，便于测试与重试。

### 3.3 多维评分路由

**评分公式**：
```
score(route) = w_cost    * normalize(1 / route.cost_per_1k_tokens)
             + w_latency * normalize(1 / route.ewma_latency_ms)
             + w_load    * normalize(1 / route.inflight_count)
             + w_health  * health_factor(route)
其中 health_factor ∈ {1.0 CLOSED, 0.3 HALF_OPEN, 0.0 OPEN}
```

**默认权重**（写入 `ai_route_score_weights` 表 scope='global'）：
```json
{"cost": 0.4, "latency": 0.3, "load": 0.2, "health": 0.1}
```

**选路流程**：
1. `RouteCandidatesStep` 拉出所有 healthy route 排序成候选列表（不再只选 1 条）。
2. `Execute` 内 `Scorer.Pick()` 用上述公式给候选打分，softmax 加权随机选 1 条。
3. 已尝试过的 route 排除在外。
4. Sticky 命中时直接返回粘性目标（绕过评分）。

**Pool 路由自然偏好**：cost=0 → `1/cost = +∞` 实际取 cap 1000 → 分数极高 → 自动优先选 Pool。这符合"OAuth 池免费上游优先消化"的业务直觉。

### 3.4 统一健康模型

```go
// internal/routing/health.go
type TargetKind int
const (
    TargetDeployment TargetKind = iota
    TargetCredential
)

type Outcome struct {
    Status    ResultStatus  // success | client_error | rate_limited | server_error | timeout | network
    LatencyMs int
}

type HealthState int
const (
    StateClosed   HealthState = iota // 正常
    StateOpen                        // 熔断中
    StateHalfOpen                    // 探测中
)

type HealthTracker interface {
    RecordOutcome(targetID string, kind TargetKind, o Outcome)
    State(targetID string) HealthState
    Snapshot() []HealthRecord // for admin /system/status
}
```

**错误分级表**：

| 上游响应 | 计入熔断 | 触发重试 | 行为 |
|---|---|---|---|
| 2xx | 重置计数 | — | RecordSuccess |
| 4xx（非 401/429） | ❌ | ❌ | 客户端错误，直接返回 |
| 401（OAuth） | 计入 credential 失败 | ✅ 换凭据 | 立刻 MarkInvalid |
| 429 | ❌（不应熔断限流） | ✅ 换 route + 指数退避 300ms·2^n（max 2s） | |
| 5xx / timeout / network | ✅ | ✅ 换 route | |

**半开探测**：
- CLOSED → 5 次连续失败 → OPEN
- OPEN 60s 后 → HALF_OPEN
- HALF_OPEN 放过 1 个请求：成功 → CLOSED；失败 → OPEN（指数退避，下次等待时间翻倍，max 30min）

**状态同步**：
- 单节点：内存即可
- 多节点：Redis Pub/Sub 广播状态变更 + 本地内存快照（最终一致）
- Redis 不可用：降级单节点内存

### 3.5 Sticky Routing

- 客户端在请求 header 传 `X-Conversation-Id: <任意字符串>` → 启用粘性
- 不传 → 当作新会话，每次走多维评分
- Redis key：`uni_ai_api:conv:{tenant}:{identity}:{model}:{conv_id}`，TTL 24h
- Value 内容：
  - Deployment 路由：`{"target_kind":"deployment","deployment_id":...,"endpoint_id":...}`
  - Pool 路由：`{"target_kind":"credential","credential_id":...}`
- 粘性目标失效（凭据 invalid / 熔断 OPEN）→ 回退评分选路 + 重写 sticky

### 3.6 可观测性

**响应头**（生产默认开启简版）：
```
X-Request-Id:     <uuid>
X-Route-Attempts: 2
X-Route-Used:     <route_id>
X-Route-Trace:    <base64 JSON 详版，env UNI_AI_TRACE_HEADER=1 开启>
```

**X-Route-Trace JSON 结构**：
```json
{
  "attempts": [
    {"route_id":"r1","score":0.82,"outcome":"server_error","http":503,"latency_ms":1240},
    {"route_id":"r2","score":0.71,"outcome":"success","http":200,"latency_ms":820}
  ],
  "sticky_hit": false,
  "scorer_weights": {"cost":0.4,"latency":0.3,"load":0.2,"health":0.1}
}
```

**失败请求 body 落盘 + Replay**：
- 新表 `ai_request_payloads`（失败必留，成功按配置采样率，默认 0.5%）
- AES-GCM 加密 upstream_body / upstream_response
- 默认保留 7 天，定时清理
- Authorization / x-api-key 等敏感 header 不入库
- `POST /api/v1/usage-logs/{id}/replay` → 用原 body 重打当前生效的路由策略

---

## 四、数据库 Schema 改动（不兼容历史数据，直接 drop+recreate）

```sql
-- 既有 ai_request_payloads 表（若有）DROP
DROP TABLE IF EXISTS ai_request_payloads;

-- ALTER 既有表
ALTER TABLE ai_model_routes
  ADD COLUMN score_weights_override JSONB,
  ADD COLUMN sticky_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  ADD COLUMN cost_per_1k_tokens NUMERIC(10,6) NOT NULL DEFAULT 0; -- 用于评分

ALTER TABLE ai_credential_pools
  ADD COLUMN sticky_granularity TEXT NOT NULL DEFAULT 'credential'
    CHECK (sticky_granularity IN ('credential', 'pool'));

ALTER TABLE ai_usage_logs
  ADD COLUMN attempts_count INT NOT NULL DEFAULT 1,
  ADD COLUMN final_route_id UUID,
  ADD COLUMN client_protocol TEXT NOT NULL DEFAULT 'openai_chat';

-- NEW 表
CREATE TABLE ai_route_score_weights (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  scope      TEXT NOT NULL,        -- 'global' | future 'tenant:<id>' | 'model:<id>'
  weights    JSONB NOT NULL,       -- {"cost":0.4,"latency":0.3,"load":0.2,"health":0.1}
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(scope)
);
INSERT INTO ai_route_score_weights(scope, weights)
  VALUES ('global', '{"cost":0.4,"latency":0.3,"load":0.2,"health":0.1}'::jsonb);

CREATE TABLE ai_request_payloads (
  id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  usage_log_id      UUID REFERENCES ai_usage_logs(id) ON DELETE CASCADE,
  upstream_body     BYTEA,           -- AES-GCM 加密
  upstream_response BYTEA,
  route_attempts    JSONB NOT NULL,  -- [{route_id, score, outcome, http_status, latency_ms}]
  sampled           BOOLEAN NOT NULL,
  client_protocol   TEXT NOT NULL,   -- 重放时知道按哪种协议回写
  raw_client_body   BYTEA,           -- 客户端原始 body（重放需要）
  created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  expires_at        TIMESTAMPTZ NOT NULL
);
CREATE INDEX ON ai_request_payloads(expires_at);
CREATE INDEX ON ai_request_payloads(usage_log_id);

-- Redis key 约定（无需 schema，仅文档）
-- route:stats:{route_id}       Hash {ewma_latency_ms, error_rate_pct, last_updated}, TTL 1h
-- route:inflight:{route_id}    Counter, TTL 60s（每次请求开始 INCR，结束 DECR）
-- health:state:{target_id}     Hash {state, fail_count, opened_at, next_probe_at}, TTL 30min
-- conv:{tenant}:{identity}:{model}:{conv_id}  JSON sticky binding, TTL 24h
-- payload:cleanup:lock         分布式锁 for payload 清理 job, TTL 1min
```

---

## 五、模块清单

### 新增

| 路径 | 作用 |
|---|---|
| `internal/formats/claude/response.go` | canonical → Anthropic Messages 非流式 |
| `internal/formats/claude/stream.go` | canonical → Anthropic SSE 命名事件流 |
| `internal/formats/claude/error.go` | Anthropic 错误体序列化 |
| `internal/formats/openai/responses/output.go` | canonical → OpenAI Responses 非流式 |
| `internal/formats/openai/responses/stream_out.go` | canonical → OpenAI Responses SSE 命名事件流 |
| `internal/formats/tokens/counter.go` | tiktoken-go 本地估算 + 上游透传分发 |
| `internal/serving/relay.go` | ClientRelay 接口 + 三种协议实现 |
| `internal/serving/scorer.go` | RouteScorer 多维评分实现 |
| `internal/serving/budget.go` | RetryBudget 控制 |
| `internal/serving/classifier.go` | Outcome 分类 + Decision 判定 |
| `internal/routing/health.go` | 统一 HealthTracker（替代 breaker.go） |
| `internal/routing/state.go` | HealthState + 半开探测状态机 |
| `internal/observability/trace.go` | X-Route-Trace 编码 |
| `internal/adapters/postgres/payloads.go` | ai_request_payloads CRUD + 清理 job |
| `internal/adapters/postgres/route_weights.go` | 评分权重表 CRUD |
| `internal/adapters/redis/route_stats.go` | EWMA stats + inflight counter |
| `internal/adapters/redis/health_sync.go` | 健康状态 Pub/Sub 广播 |
| `internal/httpserver/handler_replay.go` | POST /api/v1/usage-logs/{id}/replay |
| `internal/httpserver/handler_count_tokens.go` | POST /v1/messages/count_tokens |
| `admin/src/views/AIGateway/GatewayRouting.vue` | 评分权重配置 UI |

### 重写

| 路径 | 改动 |
|---|---|
| `internal/serving/pipeline.go` | Request 去掉 W/R；新增 Candidates/StickyHint/Attempts |
| `internal/serving/execute.go` | 拆三层 + 重试循环 + 接入 HealthTracker |
| `internal/serving/steps.go` | RouteSelectStep → RouteCandidatesStep（选出多条） |
| `internal/adapters/postgres/routes.go` | SelectRoute → SelectCandidates；接入 Redis stats |
| `internal/formats/convert.go` | 输出端按 ClientProtocol 分发，不再写死 OpenAI |
| `internal/httpserver/runtime_pipeline.go` | 持有 RequestEnvelope，调用 Relay 替代直接 ResponseWriter |
| `internal/httpserver/server.go` | 注册新路由 + Relay 接入 + 评分权重 admin API |
| `internal/httpserver/handler_models.go` | GET /v1/models 按 Accept/UA 分发 OpenAI/Anthropic 格式 |

### 删除

| 路径 | 原因 |
|---|---|
| `internal/routing/breaker.go` + `breaker_test.go` | 被 health.go 取代 |
| `internal/serving/execute.go` 中 OAuth 401 单点重试代码 | 合并入统一重试循环 |

---

## 六、Phase 拆分（独立可提交可回滚）

### **P0：严格 1:1 协议透传** ✅ **已完成**
> **策略变更**：原计划 N×M 跨协议转换，实施时改为严格 1:1 透传，更彻底地消灭了一类成功率玄学问题。

**已完成的工作**：

- 删除整个 `formats/canonical/` 包（~240 行）
- 删除 `formats/claude/` 的 `messages.go`、`random.go`、`response_out.go`、`stream_out.go`（~890 行）
- 删除 `formats/gemini/generate.go`（~400 行）
- 删除整个 `formats/openai/` 目录含 `chat.go` + `responses/` 子目录 6 个文件（~1160 行）
- 删除 `serving/relay.go` ClientRelay 层（~200 行）
- 精简 `formats/convert.go` 从 308 行到 ~45 行（仅保留 `DetectClientProtocol` + `ParseRequestMeta`）
- 新增 `formats/rewrite.go`（model 名替换 + Codex body 改写）
- 新增 `formats/usage.go`（各协议 usage 提取，替代 canonical 中间层）
- `adapters/postgres/routes.go` SQL WHERE 增加 `upstream_protocol = $clientProtocol` 过滤 + `oauthFixedTypesForProtocol()` 映射
- `serving/steps.go` `RouteCandidatesStep` 保留 `*APIError` 透传，无匹配返回 400 `no_matching_deployment`
- `serving/execute.go` 改为直接透传 `Envelope.ClientBody`，流式响应直接 io.Copy
- `serving/pipeline.go` 删除 `ChatReq`/`EmbedReq` 等 canonical 中间字段
- `httpserver/server.go` 新增 `/v1beta/models/{modelAction}` Gemini 原生端点
- `httpserver/runtime_pipeline.go` 重构 pipeline 入口，Gemini 走独立 handler
- 前端 `GatewayProviders.vue` 增加协议匹配提示 + base_url trim
- 计费保底（billing floor=1）+ URM Freeze 跳过零金额
- base_url TrimSpace + buildURL 路径去重 + OpenAI 模型列表 URL 修正

**实际改动量**：-3579 / +560 行
**验收**：各协议客户端直连同协议上游跑通，无匹配部署返回 400，同模型多协议互不串扰。

---

### **P1：Execute 解耦 + 重试骨架**

- `serving.Request` 移除 W/R 字段
- `Execute` 拆 `Builder` / `Client` / `Relay` 三层
- 引入 `RetryBudget`（3 次 / 90s / 单次按剩余预算）
- `Outcome` 分类器 + Decision 判定
- 暂时复用旧的 priority+weighted 选路（不开评分）
- 错误分级：429 不熔断、4xx 不重试、5xx/timeout/network 重试、401 换凭据
- OAuth 401 重试合并入统一循环
- `RouteCandidatesStep` 替代 `RouteSelectStep`（一次拉所有候选）
- URMFreeze 改为按候选最贵 route 估算上限

**改动量**：~700 行
**验收**：注入 mock upstream 模拟各种错误，能按设计正确重试/放弃；流式 pre-flush 失败能重试，已 flush 后失败正确关闭。

---

### **P2：HealthTracker + 半开探测**

- 新建 `routing/health.go` + `state.go`
- DeploymentHealth 和 CredentialHealth 共用接口
- 半开探测状态机（指数退避）
- Redis Pub/Sub 多节点状态同步
- 删除 `routing/breaker.go`
- `adapters/postgres/oauth_credentials.go` 的 RecordFailure 改调 HealthTracker
- admin `/api/v1/system/status` 返回新统一健康快照

**改动量**：~500 行
**验收**：连续 5 次失败 → OPEN；60s 后 HALF_OPEN 放过 1 个；成功恢复 / 失败延长 OPEN。

---

### **P3：多维评分路由**

- 新建 `serving/scorer.go`（cost/latency/load/health 加权）
- 新建 `adapters/redis/route_stats.go`（EWMA + inflight counter）
- 本地 5s 缓存层（pull-based，请求路径只读内存）
- Redis 不可用时降级 priority+weighted random
- DB 改动：`ai_route_score_weights` 表 + `ai_model_routes.score_weights_override` + `cost_per_1k_tokens`
- admin API：`GET/PUT /api/v1/route-weights/{scope}`

**改动量**：~800 行
**验收**：构造多 route 场景，故意把其中一条延迟拉高，验证流量自动向健康 route 倾斜；Redis 关掉后请求仍能完成。

---

### **P4：Sticky + 可观测 + admin UI**

- `serving/pipeline.go` 增加 `StickyHint` 字段
- 解析 `X-Conversation-Id` header
- Sticky 写入/读取 Redis（Deployment 粘 deployment+endpoint，Pool 粘 credential）
- 失效兜底（凭据 invalid / 熔断 OPEN）
- `observability/trace.go` 编码 X-Route-Trace 响应头
- `adapters/postgres/payloads.go`（失败 body 落盘 + 采样成功 + 定时清理 job）
- `httpserver/handler_replay.go` POST /api/v1/usage-logs/{id}/replay
- 前端 `admin/src/views/AIGateway/GatewayRouting.vue` 评分权重配置页 + 侧边栏路由
- 前端 `admin/src/views/Dashboard/index.vue` 增加"Top 失败 + 一键 Replay"小卡片

**改动量**：~700 行后端 + ~250 行前端
**验收**：传 X-Conversation-Id 同会话 10 次能 100% 粘同一 credential；失败请求能在 admin 列表找到并 replay 出新 request_id。

---

## 七、协议路由策略（已实施：严格 1:1）

> **⚠️ 已于 P0 阶段实施完毕**。原 N×M 特性降级表已废弃。

**设计原则**：`client_protocol == upstream_protocol` 严格字符串相等，不做任何跨协议转换。

**协议与端点对应表**：

| 客户端端点 | 推导的 client_protocol | 只匹配的 upstream_protocol |
|---|---|---|
| `/v1/chat/completions` | openai_chat | openai_chat |
| `/v1/responses` | openai_responses | openai_responses |
| `/v1/messages` | anthropic_messages | anthropic_messages |
| `/v1/embeddings` | openai_embeddings | openai_embeddings |
| `/v1/images/generations` | openai_images | openai_images |
| `/v1beta/models/{m}:generateContent` | gemini_generate | gemini_generate |
| `/v1beta/models/{m}:embedContent` | gemini_embeddings | gemini_embeddings |

**同模型多协议部署**：schema 已支持——`ai_upstream_deployments` 的 UNIQUE 约束包含 `(endpoint_id, upstream_model, upstream_protocol)`，同一 endpoint + upstream_model 可挂多行 deployment（不同 protocol），路由 SQL 的 `client_protocol` 过滤确保各自命中。

**不匹配时的行为**：返回 HTTP 400 + `no_matching_deployment` 错误码 + 提示信息（含 client_protocol 和 model），引导用户配置对应协议的 deployment。

**OAuth pool 隐式协议映射**：`ai_credential_pools.fixed_provider_type` 在运行时映射到 `upstream_protocol`（由 Go 端 `oauthFixedTypesForProtocol()` 常量实现，无需 schema 新增列）。映射关系见 3.1 节。

**保留的协议级轻量改写**（非转换，仅限同协议内）：
- `RewriteModelInBody`：替换请求体中的 `model` 字段为上游模型名
- `ApplyCodexRequestModifications`：Codex 特定 body 改写（strip 不支持字段、force store=false）
- OAuth body transform：Claude `thinking` 块过滤、Gemini CLI/Antigravity envelope 包装
- `ExtractUsage` / `ExtractStreamUsage`：从各协议原始响应/流式 chunk 提取 token usage（用于计费）

---

## 八、流式透传策略（已实施）

> **⚠️ 已于 P0 阶段实施完毕**。原 canonical 流式模型已废弃，改为直接透传。

**核心原则**：流式响应直接从上游 io.Copy 到客户端，不做任何格式转换或事件重编码。

**SSE 事件模型**（各协议透传，互不干扰）：

| 协议 | 事件模型 |
|---|---|
| OpenAI Chat | 匿名 SSE，data 全部是 `chat.completion.chunk`，最后 `data: [DONE]` |
| OpenAI Responses | 命名事件 `response.created` / `response.output_text.delta` / `response.completed` 等 ~20 种 |
| Anthropic Messages | 命名事件 `message_start` / `content_block_start` / `content_block_delta` / `content_block_stop` / `message_delta` / `message_stop` |
| Gemini | `text/event-stream` 格式，含 `candidates` / `usageMetadata` 字段 |

**透传路径中的轻量处理**（不改变事件格式）：
- SSE chunk 计数 + token usage 提取（`formats.ExtractStreamUsage`）——按各协议格式解析，不转换
- 首个 chunk 到达时记录 `FirstTokenMs`
- 流式错误检测（读取 SSE data 行判断是否为 error event）

---

## 九、风险与回滚

| 风险 | 缓解 |
|---|---|
| Anthropic 流式事件实现复杂，容易漏 stop 事件 | P0 阶段编写 12 case × 流式 + 客户端真机验证（Claude Code、claude-cli） |
| 多维评分让流量集中到少数 route 导致雪崩 | softmax 温度参数可调，默认温度高（更均匀），生产观察后再降 |
| Redis 故障时降级路径未经充分测试 | P3 验收必须含"杀掉 Redis 跑 1000 请求"场景 |
| X-Route-Trace 暴露内部路由细节 | env `UNI_AI_TRACE_HEADER` 默认关闭，仅 admin 排障时开 |
| 失败 body 落盘可能含 PII | 仅加密存储 + 7 天 TTL + 仅 admin role 可见 |
| 重构期间生产中断 | 每个 Phase 独立 PR，feature flag `UNI_AI_NEW_PIPELINE=1` 控制新旧 pipeline，灰度切换 |

**回滚策略**：每个 Phase 保留旧实现一个版本，通过 env flag 切换；P0 协议回写修复是兼容性扩展（旧 OpenAI 客户端行为不变），无回滚风险。

---

## 十、Done 标准（全部完成时验收）

1. ✅ Claude Code 直连 `/v1/messages` 跑通多轮 tool_use 对话
2. ✅ OpenAI Python SDK 走 `/v1/chat/completions` 命中 Anthropic 上游模型，流式正常
3. ✅ 故意把一条 route 配成 50% 错误率，多维评分自动把流量倾斜到健康 route
4. ✅ 杀掉 Redis 进程，请求继续正常处理（降级 priority+weighted）
5. ✅ 同会话传 X-Conversation-Id 10 次，10 次都粘同一 OAuth credential（Anthropic 上游 cache_read 占比 >50%）
6. ✅ 触发 503，X-Route-Attempts 显示 2 或 3，最终成功
7. ✅ admin 评分权重 UI 可改可保存，下个请求生效
8. ✅ 失败请求能在 admin payload 列表搜到，replay API 调用成功
9. ✅ 全部既有单元测试 + 新增协议矩阵测试通过

---

## 附录：开发顺序建议

| 周 | Phase | 备注 |
|---|---|---|
| W1 | P0 | 修协议 bug 紧迫，先解锁 Claude Code 用户 |
| W2 | P1 | Execute 解耦是 P2/P3 的前置 |
| W3 | P2 | HealthTracker 替换 breaker |
| W4-W5 | P3 | 多维评分含 Redis 调优需时间 |
| W6 | P4 | sticky + 可观测 + admin UI 收尾 |

---

**审阅完毕请直接说"开干 P0"或指出需要调整的地方。**
