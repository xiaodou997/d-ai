# uni-ai-api 超级大重构 - 完整设计方案

> **激进重构原则**：不兼容历史数据，不留历史包袱，长期最优优先于短期修补。
> **总改动量预估**：后端 ~3500 行 / 前端 ~250 行 / 5 个 Phase 独立可提交。

---

## 一、重构目标

1. **协议矩阵 N×M 完全打通**：客户端协议（OpenAI Chat / OpenAI Responses / Anthropic Messages）与上游协议（openai_chat / openai_responses / anthropic_messages / gemini_generate）任意组合，响应按客户端协议回写。
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
| D13 | 协议矩阵 | 客户端 ↔ 上游完全解耦，模型 code 不绑协议 |
| D14 | 特性降级 | 上游不支持的特性静默丢弃 + 在 X-Route-Trace 记录 |
| D15 | count_tokens | 上游支持时透传，否则本地 tiktoken-go 估算 |

---

## 三、新架构总览

### 3.1 协议矩阵

```
┌─────────────────── 客户端协议（in）────────────────────┐
│  /v1/chat/completions  (OpenAI Chat)                  │
│  /v1/responses         (OpenAI Responses)             │
│  /v1/messages          (Anthropic Messages)           │
│  /v1/embeddings        (OpenAI Embeddings)            │
│  /v1/images/generations(OpenAI Images)                │
│  /v1/models            (OpenAI / Anthropic 按 UA 分发) │
│  /v1/messages/count_tokens (Anthropic)                │
└────────────────────────┬───────────────────────────────┘
                         ↓ formats.ParseClientRequest
            ┌──────────────────────────────┐
            │   canonical.{Chat,Embed,...} │   ← 内部规范层
            └──────────────────────────────┘
                         ↓ formats.ToUpstreamRequest
┌─────────────────── 上游协议（out）─────────────────────┐
│  openai_chat / openai_responses / openai_embeddings   │
│  anthropic_messages                                    │
│  gemini_generate / gemini_embeddings                  │
└────────────────────────┬───────────────────────────────┘
                         ↓ 上游响应
            ┌──────────────────────────────┐
            │   formats.ToCanonical*       │   ← 反向规范化
            └──────────────────────────────┘
                         ↓ RelayToClient（按 req.ClientProtocol 选择）
            ┌──────────────────────────────┐
            │  CanonicalTo{OpenAIChat,     │
            │   OpenAIResponses,Anthropic} │
            └──────────────────────────────┘
```

**协议适配矩阵（输出端必须实现的转换器）**：

| Canonical → | OpenAI Chat | OpenAI Responses | Anthropic Messages |
|---|---|---|---|
| 非流式 | `CanonicalToOpenAIChat` ✅已有 | 🔴 **新建** | 🔴 **新建** |
| 流式 SSE | `CanonicalToOpenAIChatChunk` ✅已有 | 🔴 **新建** | 🔴 **新建** |
| 错误体 | `OpenAIErrorBody` | `OpenAIErrorBody` 共用 | 🔴 **新建** AnthropicErrorBody |

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

### **P0：协议回写修复 + Anthropic 端到端**（修 bug + 补齐协议矩阵）
> **优先级最高**：当前 `/v1/messages` 和 `/v1/responses` 实际是 broken 的，Claude Code 接不通。

- 新建 `formats/claude/response.go` + `stream.go` + `error.go`
- 新建 `formats/openai/responses/output.go` + `stream_out.go`
- 新建 `formats/tokens/counter.go`（透传 + tiktoken-go fallback）
- 新建 `serving/relay.go` ClientRelay 接口 + 三种协议实现
- 重构 `httpserver/runtime_pipeline.go` 持有 RequestEnvelope，剥离 W/R from Request
- 修 `convert.go` 输出端按 ClientProtocol 分发
- 补 `POST /v1/messages/count_tokens` handler
- 修 `GET /v1/models` 按 UA 分发（user-agent 含 `claude-cli` / `anthropic` 走 Anthropic 格式）
- 增加端到端测试矩阵：3 客户端协议 × 4 上游协议 = 12 个 case + streaming 各 12 个

**改动量**：~900 行
**验收**：Claude Code、OpenAI SDK、Anthropic SDK 同时连同一个 model code 都跑通，流式&非流式都对。

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

## 七、N×M 协议矩阵 - 特性降级表

> 客户端协议特性在上游不支持时的处理策略。原则：能转的转，不能转的静默丢弃 + X-Route-Trace 记录 dropped_features。

| 客户端特性 | OpenAI Chat 上游 | OpenAI Responses 上游 | Anthropic 上游 | Gemini 上游 |
|---|---|---|---|---|
| Anthropic `thinking` block | 静默丢弃 | 转 `reasoning` 段 | 透传 | 静默丢弃 |
| Anthropic `cache_control` | 丢弃（OpenAI 自动 cache） | 丢弃 | 透传 | 丢弃 |
| Anthropic `tool_use` | 转 `tool_calls` | 转 Responses tool 事件 | 透传 | 转 Gemini function_call |
| Anthropic `system` 数组 | flatten 进 messages[0].system | 转 instructions | 透传 | 转 systemInstruction |
| OpenAI `function`/`tool_calls` | 透传 | 透传 | 转 tool_use block | 转 function_call |
| OpenAI Responses `instructions` | 转 system message | 透传 | 转 system 字段 | 转 systemInstruction |
| OpenAI Responses `previous_response_id` | ❌ 拒绝（400） | 透传 | ❌ 拒绝 | ❌ 拒绝 |
| 图片输入 (multimodal) | 转 image_url | 转 input_image | 转 image source.base64 | 转 inlineData |

**实施细则**：
- 所有协议特性在 canonical 层定义为 optional field，downstream 转换器自行决定丢弃/映射
- 拒绝类（如 previous_response_id 不支持持久化时）返回 400 `unsupported_feature_for_route`
- 丢弃的特性写入 `req.Attempts[].dropped_features` 数组，可见于 X-Route-Trace

---

## 八、流式事件对齐策略

**SSE 事件模型差异**：

| 协议 | 事件模型 |
|---|---|
| OpenAI Chat | 匿名 SSE，data 全部是 `chat.completion.chunk`，最后 `data: [DONE]` |
| OpenAI Responses | 命名事件 `response.created` / `response.output_text.delta` / `response.completed` 等 ~20 种 |
| Anthropic Messages | 命名事件 `message_start` / `content_block_start` / `content_block_delta` / `content_block_stop` / `message_delta` / `message_stop` |

**canonical 流式模型**：
```go
type StreamEvent struct {
    Kind      EventKind  // start | text_delta | tool_use_start | tool_use_delta | reasoning_delta | usage | finish | error
    Index     int        // content block index
    Text      string
    ToolCall  *ToolCallDelta
    Usage     *Usage
    FinishReason string
}
```

每个 client 协议的回写器消费 `StreamEvent` 序列，按各自协议生成正确的命名事件序列：
- Anthropic 回写器维护 content block 生命周期（start/delta/stop）
- OpenAI Responses 回写器维护 output_item 生命周期
- OpenAI Chat 回写器扁平输出 delta

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
