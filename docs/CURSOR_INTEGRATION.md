# Cursor 文档聊天反代接入

Sub2API 从 `0.32.5` 起将 Cursor 作为独立平台 `cursor` 接入。该实现代理 Cursor 网站文档聊天端点 `/api/chat`，不是 Cursor 桌面端、官方账户 API 或 OAuth 服务。

## 能力边界

- 账户类型固定为 `cookie`，管理员手动录入包含 `_vcrcs` 的有效 Cookie。
- 不提供 Cursor OAuth、access token 刷新、订阅 credits、套餐余额或官方模型发现。
- 不启动无头浏览器，不实现 stealth，也不自动绕过 Vercel Challenge。
- 上游是文本协议。图片、音频、文件与文档输入会返回明确的 `400`，不会读取服务器本地文件或调用第三方 OCR。
- 工具调用是提示词约定的兼容模式，不是 Cursor 原生工具协议。模型需要输出 `json action` 块，Sub2API 才会转换为标准工具调用。
- thinking 参数可兼容接收，但不会诱导、伪造或暴露隐藏思维链。
- Cursor 返回 usage 时优先使用上游值；缺失时使用本地估算，因此不应视为 Cursor 官方账单数据。

## 支持的兼容端点

- `POST /v1/messages`
- `POST /v1/chat/completions`
- `POST /v1/responses`
- `POST /v1/messages/count_tokens`
- `GET /v1/models`

流式与非流式文本响应均受支持。Responses 的 `previous_response_id` 使用 Redis 保存，并绑定到原 API Key，默认保存 24 小时。

## 账号配置

在管理员账号页面选择 Cursor：

1. 输入账号名称。
2. 输入浏览器中已有的完整 Cookie；必须包含非空 `_vcrcs`。
3. 可选填写 Cookie 过期时间、上游模型和 Referer。
4. 可选绑定代理和 TLS 指纹模板。
5. 保存后运行账号测试。测试会向 `/api/chat` 发送最小文本请求，不查询不存在的额度接口。

Cookie 属于敏感凭据：列表、详情、导出和日志仅暴露 `credentials_status.has_cookie`，不会返回明文。编辑支持保留、替换和显式清除；Cursor 没有自动刷新能力，`refresh` 会提示管理员手动替换 Cookie。

## 模型目录与映射

默认公开模型：

- `cursor-chat`
- `google/gemini-3-flash`

两者默认映射到 `google/gemini-3-flash`。Cursor 文档聊天没有官方动态模型目录，管理员应通过账号或渠道模型映射显式增加别名，不应把映射别名描述成 Cursor 官方支持模型。

## 工具调用兼容模式

Sub2API 将工具 JSON Schema 压缩后作为文本指令发送，并解析如下响应块：

````text
```json action
{"tool":"tool_name","parameters":{"key":"value"}}
```
````

参数缺失、JSON 无法可靠修复或强制工具未返回时会产生协议错误；实现不会补造参数、隐藏拒绝文本或伪造工具执行结果。

## 计费、Usage 与 Quota

- 请求按实际平台 `cursor` 记录。
- 输入/输出 Token 使用上游 usage 或本地估算。
- 上游基础成本默认可为零；管理员可通过 Cursor Channel 配置 per-token 或 per-request 用户价格。
- Cursor 加入用户日/周/月平台 Quota。
- 账号用量仅显示本地请求、Token、账号成本、用户成本以及 Cookie 状态，不显示不存在的 Cursor credits。

## 调度与错误处理

Cursor 分组只允许绑定 Cursor 账号，fallback 链也必须保持 `cursor` 平台。

- 401/403、Challenge、429、5xx 与网络错误可在流开始前切换同组 Cursor 账号。
- 400/422 等内容或参数错误不可通过换号规避。
- 一旦向客户端写出流事件，禁止切换账号，避免拼接两个上游响应。

Ops 中上游端点固定显示为 `/api/chat`，错误日志和审计日志会脱敏 Cookie。

## 配置

```yaml
cursor:
  base_url: "https://cursor.com"
  request_timeout_seconds: 120
  stream_idle_timeout_seconds: 60
  default_model: "google/gemini-3-flash"
  referer: "https://cursor.com/docs"
  user_agent: "Mozilla/5.0 ..."
  max_history_tokens: 24000
  max_history_messages: 100
  max_auto_continue: 0
  responses_ttl_seconds: 86400
  tool_description_max_length: 1024
```

`base_url` 必须使用 HTTPS，且主机只能是 `cursor.com` 或其子域名。`max_auto_continue` 当前为保留配置且默认必须保持 `0`；客户端应根据标准结束原因自行决定是否继续，不使用语义猜测或拒绝绕过。
