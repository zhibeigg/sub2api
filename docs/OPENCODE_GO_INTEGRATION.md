# OpenCode Go 平台接入

Sub2API 从 `0.61.67` 起将 OpenCode Go 作为独立文本平台接入。平台真实值固定为 `opencode`，账号类型固定为 `apikey`，默认上游 Base URL 为 `https://opencode.ai/zen/go`。

## 能力与公开入口

OpenCode Go 分组复用 Sub2API API Key 鉴权、计费、并发、限流、代理、模型白名单、模型映射、错误透传和故障转移，并开放三种客户端入口：

| 入站协议 | 公开入口 | 说明 |
|---|---|---|
| Anthropic Messages | `POST /v1/messages` | 可直接转发或转换到模型指定的上游协议 |
| OpenAI Chat Completions | `POST /v1/chat/completions` | 可直接转发或转换到 Messages |
| OpenAI Responses | `POST /v1/responses` | 兼容入口；转换到模型指定的 Chat Completions 或 Messages 上游 |

OpenCode Go 上游只有两类模型级协议：`chat_completions` 与 `messages`。三种公开入口均支持流式和非流式响应；网关按模型解析协议，并在需要时转换请求、响应、SSE 工具调用与用量字段。

OpenCode Go 账号可启用 `extra.mixed_scheduling=true`，但只会加入 **Anthropic** 或 **OpenAI** 分组的候选池，不会加入 Gemini、Grok 或其他原生分组。独立 OpenCode Go 分组仍只调度 `platform=opencode` 的账号。

## 渠道与模型广场配置

OpenCode Go 的账号平台值统一为 `opencode`，但不同模型家族不应合并到同一个渠道。用户侧 `GET /api/v1/channels/available` 返回的每个 `group` 增加 `allow_messages_dispatch` 字段；该字段仅控制 OpenAI 分组是否开放 Anthropic `/v1/messages` 兼容调度。OpenCode Go 分组自身固定支持 `/v1/chat/completions`、`/v1/messages` 与 `/v1/responses` 三个公开入口，不依赖此开关。模型广场详情按平台、媒体类型与分组能力矩阵展示真实可用端点。

同一接口的 `supported_models[].group_rates[]` 返回按当前用户、模型级倍率、当前高峰和媒体独立倍率解析后的 Token / 图片 / 视频倍率快照；分组同时公开视频独立倍率及 480p/720p/1080p 配置价。模型详情使用这些后端快照计算倍率后价格，避免前端复刻计费优先级。

用户侧可用渠道接口会把一个渠道内同平台的分组和该渠道全部支持模型聚合成同一个 section；如果六个 `opencode` 分组共用一个渠道，模型广场会把渠道内所有模型同时关联到每个分组，导致各分组显示相同的模型数量。

生产配置应保持 **一个模型家族、一个分组、一个渠道**：

| 渠道 / 分组 | 模型 |
|---|---|
| GLM 低价渠道 | `glm-5.2`、`glm-5.1` |
| Kimi 低价渠道 | `kimi-k3`、`kimi-k2.7-code`、`kimi-k2.6` |
| MIMO 低价渠道 | `mimo-v2.5`、`mimo-v2.5-pro` |
| MiniMax 低价渠道 | `minimax-m3`、`minimax-m2.7` |
| Qwen 低价渠道 | `qwen3.7-max`、`qwen3.7-plus`、`qwen3.6-plus` |
| DeepSeek 低价渠道 | `deepseek-v4-pro`、`deepseek-v4-flash` |

每个渠道必须启用 `restrict_models=true`，只保留本家族定价，并使用 `billing_model_source=requested`。渠道服务要求一个分组只能属于一个渠道；拆分历史聚合渠道时，应先备份配置，再将聚合渠道原地转换为其中一个独立渠道，随后创建其余渠道，避免分组冲突或同时失去计价来源。

## 账号凭据

`credentials` 支持以下字段：

| 字段 | 必需 | 说明 |
|---|---|---|
| `api_key` | 是 | OpenCode Go API Key；Chat Completions 与模型目录使用 `Authorization: Bearer`，Anthropic Messages 使用 `x-api-key` 并发送 `anthropic-version: 2023-06-01` |
| `base_url` | 否 | 账号级上游地址，优先于全局 `opencode.base_url` |
| `model_mapping` | 否 | 入站模型到上游模型的字符串映射 |
| `model_protocols` | 否 | 模型到 `chat_completions` / `messages` 的协议映射 |
| `quota_cookie` | 否 | 仅用于查询 OpenCode 网页额度，不参与推理认证 |
| `quota_workspace_id` | 否 | 配额查询的可选 Workspace ID；未填时自动发现 |

示例：

```json
{
  "api_key": "<opencode-api-key>",
  "base_url": "https://opencode.ai/zen/go",
  "model_mapping": {
    "my-grok": "grok-4.5",
    "my-minimax": "minimax-m3"
  },
  "model_protocols": {
    "grok-4.5": "chat_completions",
    "minimax-m3": "messages"
  },
  "quota_cookie": "<optional-cookie>",
  "quota_workspace_id": "<optional-workspace-id>"
}
```

模型解析顺序为：去除可选的 `opencode-go/` 前缀，应用 `model_mapping`，再按 `model_protocols` 覆盖协议；未覆盖时使用内置目录。新增或改名模型若不在内置目录中，必须同时配置协议，否则请求会以 `400` 拒绝，不会猜测上游端点。

敏感字段不会在账号列表、详情或普通响应中返回明文。更新时不发送敏感字段表示保留，在 `credentials` 中发送非空值表示替换，在 `clear_credentials` 中发送 `api_key` 或 `quota_cookie` 表示显式清除；同一字段不能同时替换和清除。

## 官方当前模型目录

截至 2026 年 7 月 18 日，本版本内置的 OpenCode Go 目录如下；实际官方可用范围始终以账号认证后的 `GET https://opencode.ai/zen/go/v1/models` 为准，管理员可使用账号模型同步功能刷新白名单。

**Chat Completions 上游**

- `grok-4.5`
- `glm-5.2`
- `glm-5.1`
- `kimi-k3`
- `kimi-k2.7-code`
- `kimi-k2.6`
- `deepseek-v4-pro`
- `deepseek-v4-flash`
- `mimo-v2.5`
- `mimo-v2.5-pro`

**Anthropic Messages 上游**

- `minimax-m3`
- `minimax-m2.7`
- `minimax-m2.5`
- `qwen3.7-max`
- `qwen3.7-plus`
- `qwen3.6-plus`

`GET /v1/models`、管理端账号测试与上游模型同步都使用账号的 `api_key` 调用官方目录；目录变化不应通过修改 `base_url` 或伪造协议解决，应更新 `model_mapping` / `model_protocols` 并完成真实请求验证。

## 配额查询与降级

`quota_cookie` 和 `quota_workspace_id` 只用于管理后台展示 OpenCode Go 的 rolling、weekly、monthly 配额窗口。配额查询具有独立超时、fresh/stale 缓存和后台刷新：

- 未配置 Cookie 时显示 `missing`；
- 查询成功时显示 `verified` / `cached`；
- 旧快照仍可用时显示 `stale` 并异步刷新；
- Cookie 过期、页面结构变化或配额接口限流时显示 `error`，或保留旧快照。

账号用量接口只使用 `open_code_quota` 字段，不提供旧别名。快照顶层包含 `configured`、`state`、可选 `message`、`fetched_at`，窗口位于 `rolling` / `weekly` / `monthly`；每个窗口使用 `usage_percent`，重置时间优先读取 `reset_at`，否则可由 `fetched_at + reset_in_seconds` 推导。前端更新时间以 `fetched_at` 为准。

**额度查询失败不会暂停账号、阻断调度或影响推理。** 推理只依赖 `api_key`；配额 Cookie 失效时应单独替换或清除，不要清除仍可用的 API Key。只有一次成功快照明确显示窗口已满且具有未来重置时间时，服务才会将账号冷却到该重置时间。

## 错误与限流

- 无效 JSON、缺少模型、未知模型或无协议映射返回 `400`。
- 缺少或失效的 OpenCode API Key 通常返回 `401` / `403`，并按账号错误参与故障转移。
- 上游 `429`、`Retry-After` 与限流响应会进入现有账号限流和 failover 流程；已开始流式输出后，错误使用对应协议的流内错误格式返回。
- 网络失败、代理错误与上游 `5xx` 进入现有临时不可调度/账号切换流程；客户端参数错误不会盲目换号。
- 上游响应头中的请求 ID 和安全响应头会按网关规则保留，hop-by-hop 头不会透传。

## 管理 API 示例

以下示例使用管理员 JWT；也可按部署配置使用管理员 API Key。

### 创建账号

```bash
curl -sS "$SUB2API_BASE_URL/api/v1/admin/accounts" \
  -H "Authorization: Bearer $SUB2API_JWT" \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"OpenCode Go",
    "platform":"opencode",
    "type":"apikey",
    "concurrency":1,
    "credentials":{
      "api_key":"<opencode-api-key>",
      "base_url":"https://opencode.ai/zen/go",
      "model_mapping":{"my-grok":"grok-4.5"},
      "model_protocols":{"grok-4.5":"chat_completions","minimax-m3":"messages"},
      "quota_cookie":"<optional-cookie>",
      "quota_workspace_id":"<optional-workspace-id>"
    },
    "extra":{"mixed_scheduling":false},
    "group_ids":[1]
  }'
```

### 更新非敏感配置并替换 API Key

`credentials` 是账号凭据更新对象；未发送的 `api_key` / `quota_cookie` 会保留。

```bash
curl -sS -X PUT "$SUB2API_BASE_URL/api/v1/admin/accounts/123" \
  -H "Authorization: Bearer $SUB2API_JWT" \
  -H 'Content-Type: application/json' \
  -d '{
    "credentials":{
      "api_key":"<replacement-api-key>",
      "base_url":"https://opencode.ai/zen/go",
      "model_mapping":{"my-minimax":"minimax-m3"},
      "model_protocols":{"minimax-m3":"messages"}
    },
    "extra":{"mixed_scheduling":true}
  }'
```

### 清除配额 Cookie

```bash
curl -sS -X PUT "$SUB2API_BASE_URL/api/v1/admin/accounts/123" \
  -H "Authorization: Bearer $SUB2API_JWT" \
  -H 'Content-Type: application/json' \
  -d '{"clear_credentials":["quota_cookie"]}'
```

清除 `api_key` 会使账号无法推理；通常应先停用账号或在同一账号池中确认其他账号可接管。管理 CLI 的等价示例见 `skills/sub2api-admin/references/admin-cli.md`。

## 配置与实现触点

全局默认配置位于 `config.yaml` 的 `opencode` 段：

```yaml
opencode:
  base_url: "https://opencode.ai/zen/go"
  inference_timeout_seconds: 600
  quota_cache_ttl_seconds: 300
  quota_stale_ttl_seconds: 1800
  quota_request_timeout_seconds: 15
```

账号级 `credentials.base_url` 优先于全局默认。部署示例已收录在 `deploy/config.example.yaml`；实现触点和修改约束见 `DEV_GUIDE.md`。
