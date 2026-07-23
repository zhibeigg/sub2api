# 模型错误响应契约 / Model Error Response Contract

本契约适用于模型客户端入口，不适用于普通 REST、管理、OAuth、账单等接口。

This contract applies to model-facing endpoints only. Ordinary REST, admin, OAuth, and billing APIs keep their existing error behavior.

## 语言协商 / Language negotiation

- 客户端通过 `Accept-Language` 请求中文或英文。
- `zh`、`zh-CN`、`zh-Hans` 等中文标签解析为中文；其他受支持英文标签解析为英文。
- 请求头缺失、无有效匹配或格式不可用时，回退到 `gateway.model_error_default_locale`。
- `gateway.model_error_default_locale` 仅支持 `en`、`zh`，默认 `zh`。
- 语言中间件只记录选择结果；成功响应和非模型接口不会因此增加模型错误响应头。

Clients select English or Chinese with `Accept-Language`. Missing or unsupported preferences fall back to `gateway.model_error_default_locale` (`en` or `zh`, default `zh`). Localization metadata is emitted only when a model error is written.

## 通用展示规则 / Common presentation rules

所有模型客户端可见错误遵守以下规则：

1. `message` 以 `[PokeAPI]` 开头。
2. 上游原始正文、HTML、内部地址、凭据、令牌、Cookie、调试字段和敏感响应头不得直接返回客户端。
3. 原始上游信息只用于服务端日志、错误分类、账号切换和管理员错误规则匹配。
4. 管理员错误透传规则可保留自定义安全文案和状态码，但文案仍会统一脱敏并添加 `[PokeAPI]`。
5. 已经品牌化的消息重复经过 writer 时保持幂等，不会出现重复前缀。

Every client-visible model error is safely classified and branded. Raw upstream diagnostics remain server-side and are never treated as client response content.

## 前端错误边界 / Frontend error boundary

- Vue 根组件使用 `onErrorCaptured` 隔离子组件的 render、setup 与生命周期异常，展示不含异常正文的安全提示。
- 用户可以重试当前页面（强制重新挂载路由视图）或返回首页。
- Vue 全局错误处理器与 bootstrap Promise 兜底会在根组件无法渲染时直接创建安全 DOM 页面；日志仅记录错误类型和 Vue 阶段，不记录异常 message、stack 或响应正文。
- 错误边界不替代模型协议错误响应；它只防止前端异常导致空白页面。

The root Vue boundary contains component failures and provides retry/home recovery. Bootstrap failures use a DOM-only fallback, and neither path exposes raw exception details to the user.

## 稳定响应头 / Stable response headers

模型 HTTP 错误响应包含：

| 响应头 | 含义 |
|---|---|
| `X-PokeAPI-Error-Code` | 与供应商协议无关的稳定错误分类，例如 `POKE_AUTH_INVALID`、`POKE_CONTEXT_TOO_LARGE`、`POKE_UPSTREAM_UNAVAILABLE`。客户端应优先使用该字段做稳定判断。 |
| `X-PokeAPI-Request-ID` | PokeAPI 请求 ID，可用于日志关联和支持排查。 |
| `Content-Language` | 实际错误语言，当前为 `en` 或 `zh-CN`。 |
| `Vary: Accept-Language` | 表示错误展示可能随语言请求头变化。 |

协议 JSON 中原有的 `type`、`code`、`status`、`details` 仍属于对应 SDK/协议兼容字段，不应替代 `X-PokeAPI-Error-Code` 作为跨协议分类依据。

## 稳定分类 / Stable categories

当前稳定分类覆盖：

- 鉴权：缺失、无效、停用、过期、暂不可用。
- 权限与额度：权限拒绝、余额不足、配额耗尽、订阅缺失、日/周/月使用上限。
- 路由与请求：分组不可用、端点不允许、请求无效、请求体过大、缺少模型、模型不支持或不存在、上下文超限。
- 容量与风控：并发限制、队列已满、速率限制、内容或安全策略阻断。
- 上游：鉴权失败、权限不足、限流、过载、超时、不可用、无效响应。
- 服务端：服务暂不可用、内部错误。

完整标识定义以 `backend/internal/pkg/modelerror/codes.go` 为准。稳定标识只增补，不通过修改既有含义来复用。

## 协议兼容 / Protocol compatibility

### Anthropic Messages

- 保持原 HTTP 状态。
- 保持顶层 `{"type":"error","error":{...}}` envelope。
- 保持协议 `error.type`，已有协议 `error.code` 时继续保留。
- SSE 流内错误继续使用 `event: error` 和 Anthropic error data 结构。

### OpenAI Chat Completions

- 保持原 HTTP 状态。
- 保持 `{"error":{"type","code","message"}}` 兼容结构；仅在原路径具有 `code` 时写入对应协议码。
- 已开始的 SSE 流使用 OpenAI 兼容错误 data 帧，不追加第二个 JSON 响应。

### OpenAI Responses

- 保持原 HTTP 状态及 Responses error envelope 的 `type`、`code` 语义。
- 流式失败使用 `response.failed` 终止事件，并保留 `response.status=failed` 与 `response.error` 结构。
- 安全审核路径保留既有 `api_error` 等协议类型，不用稳定分类覆盖协议字段。

### OpenAI Images

- 永久模型或端点不兼容继续返回 `404 model_not_found`。
- 已配置兼容账号因模型限流、配额暂停、并发或临时运行时状态不可用时返回 `503`，协议 `error.code` 为 `image_capacity_unavailable`，并附带 `Retry-After`。
- 客户端可刷新模型目录并向用户显示临时不可用状态，但不得自动重复提交生成请求，以免上游已受理时产生重复图片。

### Gemini

- 保持 Google 风格 `{"error":{"code","message","status","details"}}`。
- HTTP 状态、Google `status`（例如 `UNAVAILABLE`）和已有 `details` 保持兼容。

### WebSocket

- 保持既有 WebSocket 关闭码和错误帧结构。
- 关闭原因和客户端错误帧按请求语言本地化并添加 `[PokeAPI]`。
- 关闭原因按 UTF-8 边界安全截断，避免产生非法关闭帧。
- WebSocket 握手前可用 HTTP 错误响应仍包含稳定错误头；升级后的错误通过协议帧或关闭原因传递。

## 状态码与重试 / Status and retry behavior

本地化和安全展示不会改变原有故障处理决策：

- 请求级确定性错误不因翻译而切换账号。
- 可重试上游错误仍按现有 failover、`Retry-After` 和账号健康规则处理。
- SSE 已提交响应头后仍使用协议内终止事件，HTTP 状态不会被二次改写。
- WebSocket 关闭码保持原语义；客户端不应仅解析自然语言关闭原因决定重试。

客户端建议同时使用 HTTP 状态、协议字段和 `X-PokeAPI-Error-Code`。自然语言 `message` 只用于向用户展示，不应作为程序分支条件。
