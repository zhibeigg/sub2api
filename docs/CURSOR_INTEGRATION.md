# Cursor 文档聊天反代接入

Sub2API 从 `0.32.5` 起将 Cursor 作为独立平台 `cursor` 接入。该实现代理 Cursor 网站文档聊天端点 `/api/chat`，不是 Cursor 桌面端、官方账户 API 或 OAuth 服务。

## 能力边界

- 账户类型固定为 `cookie`。管理员可通过可选浏览器扩展从当前浏览器会话导入 `_vcrcs`，也可手动录入包含 `_vcrcs` 的有效 Cookie。
- 网页登录助手提供接近 OAuth 的交互，但它不是 Cursor OAuth；不提供 access token 刷新、订阅 credits、套餐余额或官方模型发现。
- 扩展只打开 Cursor 原站并等待用户正常登录，不启动无头浏览器，不实现 stealth，也不自动处理或绕过验证码、Vercel Challenge。
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

在管理员账号页面选择 Cursor 后采用两步创建：

1. 第一步输入账号名称，并配置代理、TLS 指纹模板、模型、配额、调度和分组。
2. 第二步推荐点击“使用 Cursor 网页登录并创建”。扩展打开 `cursor.com`，用户在原站完成登录后，扩展只读取 `_vcrcs` 并返回发起操作的后台标签页。
3. 前端在内存中构造最小 Cookie Header，随后自动调用创建前预检；真实探测成功后才创建账号。
4. 未安装扩展、当前后台域名未授权或自动导入失败时，可展开“手动导入 Cookie”粘贴包含非空 `_vcrcs` 的完整 Cookie。
5. Cookie 到期时间、Cursor 内部模型与 Referer 位于“高级设置”。内部模型默认留空，留空时按账号模型映射和系统默认值解析。
6. 失败不会创建临时数据库账号、写入 Redis 或在响应中回显 Cookie。

## 浏览器扩展登录助手

发布包内提供 Chrome/Edge Manifest V3 扩展，管理后台可从 `/downloads/cursor-cookie-importer.zip` 下载。非商店版本需要解压后在 `chrome://extensions` 开启开发者模式并选择“加载已解压的扩展程序”；安装后刷新管理后台。

默认直接支持：

- `https://www.poke2api.com`
- `https://www.pokeapi.top`
- `http://localhost`
- `http://127.0.0.1`

自托管 HTTPS 域名可在扩展设置页中授权精确 origin。公网明文 HTTP、`file:` 和任意宽泛来源不会被接受。

扩展的数据边界：

- 只有用户在 Cursor 创建流程中主动点击后才开始读取。
- 只调用浏览器 Cookie API读取 `cursor.com` 的 `_vcrcs`；不枚举其他 Cookie。
- 不读取密码、页面 DOM、localStorage、IndexedDB、表单或网络响应。
- Cookie 不进入 URL、扩展持久存储、日志、剪贴板或第三方服务。
- 消息使用协议版本、随机 flow ID、nonce、页面实例和原始标签页绑定；结果不广播给其他标签页。
- 登录等待最长 10 分钟；关闭 Cursor 标签页、刷新后台、切换平台或关闭创建弹窗会取消流程。
- 扩展不会刷新 Cookie。凭据过期后仍需管理员重新发起网页登录或手动替换。

创建前预检 API：

```http
POST /api/v1/admin/accounts/validate-credentials
Content-Type: application/json
Authorization: Bearer <admin-token>
```

```json
{
  "platform": "cursor",
  "type": "cookie",
  "proxy_id": 12,
  "model_id": "cursor-chat",
  "prompt": "Reply with OK.",
  "credentials": {
    "cookie": "_vcrcs=<value>; ...",
    "cursor_upstream_model": "google/gemini-3-flash",
    "cursor_referer": "https://cursor.com/docs"
  }
}
```

响应只返回安全的验证消息和截断后的文本摘要，不回显 Cookie。该接口只允许 `adobe/oauth` 和 `cursor/cookie`，其他平台或类型返回 `400`。

Cookie 属于敏感凭据：列表、详情、预检响应、导出和日志仅暴露 `credentials_status.has_cookie` 或安全摘要，不会返回明文。编辑支持保留、替换和显式清除；Cursor 没有自动刷新能力，`refresh` 会提示管理员手动替换 Cookie。

## 模型目录与映射

默认公开模型：

- `cursor-chat`
- `google/gemini-3-flash`

两者默认映射到 `google/gemini-3-flash`。Cursor 文档聊天没有官方动态模型目录，管理员应通过账号或渠道模型映射显式增加别名，不应把映射别名描述成 Cursor 官方支持模型。

“Cursor 内部模型（可选）”是高级覆盖项，优先级为：账号 `cursor_upstream_model` → 账号模型映射 → `cursor.default_model`。普通管理员应保持留空，避免无意绕过已有模型映射。

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

`base_url` 必须使用 HTTPS，且主机只能是 `cursor.com` 或其子域名。`max_auto_continue` 当前为保留配置且默认必须保持 `0`；客户端应根据标准结束原因自行决定是否继续，不使用语义猜测或拒绝绕过。两步预检复用现有 Cursor 超时、Referer、User-Agent、代理和 TLS 指纹设置，不新增默认配置或示例配置项。
