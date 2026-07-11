# Cursor IDE Chat 与 Cloud Agent 接入

Sub2API 的 Cursor 接入包含两条彼此独立的上游路径：普通聊天默认模拟 Cursor IDE 客户端，通过 `https://api2.cursor.sh/aiserver.v1.ChatService/StreamUnifiedChatWithTools` 的 HTTP/2 Connect-Protobuf 流转发；显式任务模式继续使用 `https://api.cursor.com/v1/agents` 的官方 **Cloud Agents API（Beta）**。

旧版基于 `cursor.com/api/chat`、浏览器 Cookie `_vcrcs` 和 Cookie 导入扩展的实现已移除。IDE Chat 使用 Cursor Dashboard 登录会话，不使用浏览器 Cookie；Cloud Agent 使用官方 API Key。两条路径不会在同一次普通聊天请求中静默互相降级。

> IDE Chat 是 Cursor 客户端协议兼容层，并非 Cursor 已承诺稳定的第三方公开 API；Cloud Agents API 仍处于官方 Beta。两者的端点、字段、认证与可用范围都可能调整，升级前应在实际 Cursor 账号上执行探测和流式回归测试。

## 转发模式与能力边界

Cursor API Key 账号通过 `credentials.cursor_transport_mode` 选择路径：

| 模式 | 普通聊天路径 | 必需凭据 |
|---|---|---|
| `auto` | 存在 Dashboard Access Token 时优先 IDE Chat，否则使用 Cloud Agent | 至少配置一套凭据 |
| `ide_chat` | 仅 IDE Chat | `dashboard_access_token` |
| `cloud_agent` | 仅 Cloud Agent | 官方 Cursor `api_key` |

- `/v1/messages`、`/v1/chat/completions`、`/v1/responses` 在 IDE Chat 模式下实时消费 Connect-Protobuf 帧，并实时输出下游 SSE；不再创建临时 Cloud Agent 等待任务完成后模拟流式。
- 无工具请求采用 Cursor Ask 模式；带工具定义的请求采用 Agent 模式，并把 Cursor 原生 MCP 工具调用映射为 Anthropic、OpenAI Chat Completions 或 OpenAI Responses 的原生工具事件。
- IDE Chat 支持文本、thinking/reasoning、工具调用与工具结果。当前图片、音频、文件和文档输入仍会返回明确的 `400`。
- IDE Chat 强制使用独立 HTTP/2 连接池和 `application/connect+proto`，不会参与 OpenAI 上游的 HTTP/2→HTTP/1.1 降级。
- Cloud Agent 仍用于创建和管理可自主执行任务的持久资源；每次提示词执行对应一个 run，同一 Agent 同时只能有一个活跃 run。
- IDE 模型以 `AiService/AvailableModels` 为准；Cloud Agent 模型以 `GET /v1/models` 为准。管理后台会按当前模式同步、去重并排序模型 ID。
- Cursor 账号可显式启用混合调度。兼容层可承接三种普通聊天协议；Gemini 原生 `generateContent` 不会调度到 Cursor。不同上游的会话上下文与模型能力可能不同，应通过分组隔离账号，并只启用已同步且验证可用的模型。
- Cursor 套餐资格、模型用量、按需超额费用、速率限制和 Cloud Agent 执行成本均由 Cursor 侧决定；Sub2API 不应把本地价格、余额或配额描述成 Cursor 官方额度。

## 普通聊天兼容语义

- 三种入站协议先归一化为统一对话结构，再编码为 Cursor IDE 的 `StreamUnifiedChatWithTools` 请求。Connect 请求和响应均使用 5 字节帧头，并支持 gzip 数据帧与 end-stream 错误帧。
- `stream=true` 时，文本、reasoning、工具调用和完成原因到达后立即映射到对应下游 SSE；上游在首个有效事件前失败时仍可返回标准网关错误，已经开始下游流后则写入协议对应的流内错误。
- 非流式请求也消费同一实时上游流，但只在结束后组装 JSON 响应。Token 用量优先采用上游事件；上游未提供时使用本地估算。
- `previous_response_id` 仍由 Sub2API 在 Redis 中保存并恢复对话，不依赖 Cursor 服务端会话 ID。
- 工具定义通过 Cursor 原生 MCP 工具描述发送；若请求要求特定工具或至少一次工具调用，兼容层会同时收窄工具集合并加入明确约束。上游未返回必需工具调用时请求失败，不伪造工具结果。
- 普通聊天不会创建、轮询或删除 Cloud Agent。需要仓库任务、长期 Agent 或 run 管理时，应显式使用 Cloud Agent 任务模式，而不是依赖普通聊天端点。

## IDE Chat 认证与 Token 刷新

IDE Chat 使用 `dashboard_access_token` 作为 Bearer Token。建议同时保存 `dashboard_refresh_token`；已持久化账号会在 Access Token 临近到期时自动刷新，遇到 `401` 时强制刷新并重试一次。刷新后的凭据加密回写账号仓库，避免后续请求继续使用旧 Token。

客户端协议请求还会生成 Cursor IDE 所需的会话与校验头，包括 UUID v5 Session ID、SHA256 Client Key、Jyh checksum、客户端版本、操作系统信息、Ghost Mode 与 onboarding 状态。`cursor_client_version`、`cursor_client_os_version`、`cursor_config_version`、`cursor_machine_id` 可作为账号级非敏感覆盖；未配置时使用服务端默认值。

出于 Refresh Token 轮换安全考虑，创建账号前的临时探测和模型预览不会刷新尚未持久化的凭据。若临时 Access Token 已失效，应重新完成 Dashboard 授权或输入新 Token，再保存账号。

## Cloud Agent 认证

Cursor 官方 API 支持两种等价认证方式：

```http
Authorization: Bearer <cursor-api-key>
```

或将 API Key 作为 Basic Auth 用户名并使用空密码：

```bash
curl -u "$CURSOR_API_KEY:" https://api.cursor.com/v1/me
```

建议优先使用 Bearer Auth，并遵守以下边界：

- **用户 API Key**：绑定具体 Cursor 用户，适合个人自动化和以该用户身份创建 Agent。
- **服务账户 / 团队 API Key**：不绑定具体用户，适合团队自动化。需要 My Machines 用户范围令牌时，必须使用具备 agent scope 的团队服务账户 Key 调用 `POST /v1/sub-tokens`。
- API Key 只能保存在服务端凭据存储中，不得写入仓库、URL、日志、浏览器存储或返回给普通用户。
- 不再支持或记录 `_vcrcs`、Cookie 到期时间、Referer、浏览器 User-Agent 等旧字段。

## 验证 API Key：`GET /v1/me`

```bash
curl --fail-with-body \
  -H "Authorization: Bearer $CURSOR_API_KEY" \
  https://api.cursor.com/v1/me
```

响应包含 `apiKeyName` 和 `createdAt`。用户 Key 还可包含 `userId`、`userEmail`、姓名等所有者信息；服务账户 / 团队 Key 不绑定个人，因此会省略 `userId` 和 `userEmail`。

该端点只证明密钥可被 Cursor API 识别。具体 Agent 权限、套餐资格或功能灰度仍可能在调用业务端点时返回 `403`，例如 `plan_required`、`role_forbidden` 或 `feature_unavailable`。

## 管理后台用量窗口与刷新检测

- 账号列表会自动读取 Sub2API 本地 usage logs，展示今日请求、总 Token、缓存写入 Token、缓存读取 Token 及本地计费。
- 若 Cursor 账号配置了 Sub2API 的日、周或总额度，账号列表会复用统一的 `1d`、`7d`、`total` 进度条；这些是本地额度，不是 Cursor 官方套餐额度。
- 点击“刷新检测”或刷新账号列表时，管理 API 会以 `force=true` 调用 Cursor `GET /v1/me` 验证当前 Cloud Agents API Key；普通自动加载不会批量探测上游。
- Cursor Run 返回的 `cacheWriteTokens` 和 `cacheReadTokens` 分别保存为统一用量记录的 `cache_creation_tokens`（界面显示“缓存写入”）和 `cache_read_tokens`，并参与平台专属计费。

### Cursor Spending 官方套餐进度（可选）

Cursor 的 Cloud Agents API Key 不能读取 Spending 页面中的套餐进度。若希望在 Sub2API 账号列表显示与 `https://cursor.com/dashboard/spending` 一致的 `Total / First-party / API` 进度，应优先使用服务器一键 PKCE 授权，为该 Sub2API 账号建立一份**服务器独立 Dashboard 会话**。

#### 推荐：服务器一键 PKCE 授权

管理员在账号编辑页启动授权后，服务端生成短期 PKCE 登录事务并返回 Cursor 登录地址；浏览器完成登录后，前端只轮询授权状态：

| 管理 API | 用途 |
|---|---|
| `POST /api/v1/admin/cursor/dashboard-auth/start` | 创建短期 PKCE 授权事务并返回待打开的 Cursor 登录 URL |
| `POST /api/v1/admin/cursor/dashboard-auth/poll` | 轮询事务状态；成功时由服务端将 Dashboard 凭据加密绑定到目标账号 |

授权码、PKCE verifier、Access Token 和 Refresh Token 全程由服务端处理。`start` 与 `poll` 不向前端返回 Dashboard Token，普通账号查询、导出和日志也不会包含这些凭据。登录事务默认仅短期有效，超时后必须重新发起。

这份 Dashboard 会话由 Sub2API 使用独立 UUID/PKCE 流程创建，不再复用手工导入的 Cursor 桌面 Token：

- 在 Sub2API 中授权或续期不会替换 Cursor 桌面的本地登录状态；
- Cursor 桌面正常关闭、重启或自身 Token 轮换通常不会影响服务器会话；
- Cursor 是否在 Sign Out 时仅撤销当前会话或执行账户级撤销由其服务端决定，Sub2API 不作绝对保证；
- 在 Sub2API 中清除 Dashboard 凭据不会主动调用 Cursor `/auth/logout`，避免误撤销其他客户端；
- 若 Cursor 服务端撤销了服务器会话，Sub2API 会保留最后快照并标记 `reauth_required`，等待管理员重新授权。

服务端后台维护该独立会话：按配置周期检查凭据，在配置的提前刷新窗口内续期，并按较长周期主动探测 Dashboard 用量 RPC。刷新成功后会加密回写新凭据和最新套餐快照；同一 Dashboard 会话也可供 IDE Chat 和 IDE 模型同步使用，这些后台动作不需要前端持有 Token。

#### 高级兼容：手工导入桌面 Token

仅当一键 PKCE 流程不可用、需要迁移既有部署或排查兼容问题时，才手工配置：

- `dashboard_access_token`：Cursor 桌面登录 Access Token；
- `dashboard_refresh_token`：对应的 Refresh Token，建议同时提供以支持续期。

Windows 上可从 `%APPDATA%\\Cursor\\User\\globalStorage\\state.vscdb` 的 `ItemTable` 高级提取 `cursorAuth/accessToken` 与 `cursorAuth/refreshToken`。该方式依赖 Cursor 桌面内部存储结构，可能随客户端版本变化，不应作为常规接入步骤。

无论凭据来自 PKCE 还是手工兼容导入，Token 都只会加密保存在服务端，并只发送到配置项 `cursor.dashboard_base_url` 或 `cursor.chat_base_url`（默认均限定为 `https://api2.cursor.sh`）。它用于 IDE Chat、IDE 模型同步和 Dashboard 套餐探测，不会参与 Cloud Agent 创建或 Cloud Agent 模型同步。

强制刷新和后台探测会执行 Dashboard Connect RPC：

```http
POST /aiserver.v1.DashboardService/GetCurrentPeriodUsage
Authorization: Bearer <dashboard_access_token>
Content-Type: application/json
Connect-Protocol-Version: 1

{}
```

响应中的 `planUsage.totalPercentUsed`、`planUsage.autoPercentUsed`、`planUsage.apiPercentUsed`、`limit`、`totalSpend`、`remaining` 和 `billingCycleStart/End` 会归一化为官方套餐快照，金额单位是美分。遇到 `401` 且存在 Refresh Token 时，服务端尝试刷新凭据并重试一次。

该 Dashboard RPC 未作为稳定第三方 API 发布，字段或路径可能变化。因此 Sub2API 使用以下降级策略：

- 普通列表加载优先读取最后成功快照，不因展示页面而把 Token 下发到浏览器；
- 探测失败但存在旧快照时标记 `stale` 并继续显示旧数据；
- Refresh Token 失效、授权被撤销或无法恢复的认证错误标记 `reauth_required`，等待管理员重新执行 PKCE 授权；
- `reauth_required` 或 `stale` 只降级 Dashboard 套餐展示，不改变 Cloud Agents API Key 的验证状态，也不隐藏 Sub2API 本地用量；
- 无历史快照时保持明确的缺失/错误状态，不虚构零用量。

管理接口为 `GET /api/v1/admin/accounts/{id}/usage?source=active&force=true`。Cursor 响应除本地字段外还可包含：

- `cursor_dashboard_configured`
- `cursor_dashboard_state`：`configured / cached / verified / missing / stale / error`
- `cursor_dashboard_message`
- `cursor_dashboard_session.state`：`connected / refresh_due / reauth_required / error / missing`
- `cursor_plan_usage`：官方百分比、金额（美分）、账期和更新时间

## 模型发现

### IDE Chat：`AiService/AvailableModels`

IDE 模式向以下端点发送带 5 字节 Connect 帧头的 Protobuf 请求，不能发送裸 Protobuf：

```http
POST /aiserver.v1.AiService/AvailableModels
Authorization: Bearer <dashboard_access_token>
Content-Type: application/connect+proto
Connect-Protocol-Version: 1
```

响应通过 Connect 帧解码后解析 `AvailableModelsResponse` Protobuf；兼容重复 `modelNames` 字段和嵌套模型消息中的 `name`。管理端创建预览和已保存账号同步均支持此路径；已保存账号遇到过期 Token 时可自动刷新。

### Cloud Agent：`GET /v1/models`

```bash
curl --fail-with-body \
  -H "Authorization: Bearer $CURSOR_API_KEY" \
  https://api.cursor.com/v1/models
```

响应的 `items[].id` 可作为创建 Agent 时的 `model.id`。模型还可能提供：

- `aliases`：解析到同一模型的别名；
- `parameters`：允许传入的参数和值；
- `variants`：有效的 `id + params` 组合以及默认变体。

Sub2API 内置的模型目录只是离线兼容快照，不替代上游发现结果。账号可用模型应以当前转发模式的实时响应为准；Cloud Agent 的变体和参数以官方 `/v1/models` 为准。若调用方希望使用 Cursor 的账户默认模型，应完全省略 `model`，而不是发送空对象。

## 内置模型目录与本地价格

管理后台在无法同步上游模型时会提供以下离线默认目录。价格单位为 **USD/百万 Token**，列顺序为输入、缓存写入、缓存读取、输出；`-` 在本地价格快照中按 `0` 处理。同步得到的其他上游模型仍可加入白名单，但若没有渠道自定义价格或平台价格条目，Sub2API 会拒绝按未知价格结算，而不是静默套用其他平台同名模型价格。

| 模型 ID | 输入 | 缓存写入 | 缓存读取 | 输出 |
|---|---:|---:|---:|---:|
| `claude-4-sonnet` | $3 | $3.75 | $0.3 | $15 |
| `claude-4-sonnet-1m` | $6 | $7.5 | $0.6 | $22.5 |
| `claude-4.5-haiku` | $1 | $1.25 | $0.1 | $5 |
| `claude-4.5-opus` | $5 | $6.25 | $0.5 | $25 |
| `claude-4.5-sonnet` | $3 | $3.75 | $0.3 | $15 |
| `claude-4.6-opus` | $5 | $6.25 | $0.5 | $25 |
| `claude-4.6-sonnet` | $3 | $3.75 | $0.3 | $15 |
| `claude-4.7-opus` | $5 | $6.25 | $0.5 | $25 |
| `claude-fable-5` | $10 | $12.5 | $1 | $50 |
| `claude-4.7-opus-fast` | $30 | $37.5 | $3 | $150 |
| `claude-4.8-opus` | $5 | $6.25 | $0.5 | $25 |
| `claude-sonnet-5` | $3 | $3.75 | $0.3 | $15 |
| `composer-1` | $1.25 | - | $0.125 | $10 |
| `composer-2.5` | $0.5 | - | $0.2 | $2.5 |
| `gemini-2.5-flash` | $0.3 | - | $0.03 | $2.5 |
| `gemini-3-flash` | $0.5 | - | $0.05 | $3 |
| `gemini-3-pro` | $2 | - | $0.2 | $12 |
| `gemini-3-pro-image-preview` | $2 | - | $0.2 | $12 |
| `gemini-3.1-pro` | $2 | - | $0.2 | $12 |
| `gemini-3.5-flash` | $1.5 | - | $0.15 | $9 |
| `glm-5.2` | $1.4 | - | $0.26 | $4.4 |
| `gpt-5` | $1.25 | - | $0.125 | $10 |
| `gpt-5-fast` | $2.5 | - | $0.25 | $20 |
| `gpt-5-mini` | $0.25 | - | $0.025 | $2 |
| `gpt-5-codex` | $1.25 | - | $0.125 | $10 |
| `gpt-5.1-codex` | $1.25 | - | $0.125 | $10 |
| `gpt-5.1-codex-max` | $1.25 | - | $0.125 | $10 |
| `gpt-5.1-codex-mini` | $0.25 | - | $0.025 | $2 |
| `gpt-5.2` | $1.75 | - | $0.175 | $14 |
| `gpt-5.2-codex` | $1.75 | - | $0.175 | $14 |
| `gpt-5.3-codex` | $1.75 | - | $0.175 | $14 |
| `gpt-5.4` | $2.5 | - | $0.25 | $15 |
| `gpt-5.4-mini` | $0.75 | - | $0.075 | $4.5 |
| `gpt-5.4-nano` | $0.2 | - | $0.02 | $1.25 |
| `gpt-5.5` | $5 | - | $0.5 | $30 |
| `gpt-5.6-luna` | $1 | $1.25 | $0.1 | $6 |
| `gpt-5.6-sol` | $5 | $6.25 | $0.5 | $30 |
| `gpt-5.6-terra` | $2.5 | $3.125 | $0.25 | $15 |
| `grok-4.5` | $2 | - | $0.5 | $6 |
| `kimi-k2.7-code` | $0.95 | - | $0.19 | $4 |

价格解析优先级为：**渠道自定义价格 → Cursor 平台专属价格 → 全局动态价格/兜底价格**。因此 Cursor 的 `gpt-5.5`、`gpt-5.4-mini` 等价格不会覆盖 OpenAI 平台的同名模型，也不会被 OpenAI 的动态价格反向覆盖。

## 创建临时无仓库 Agent

`POST /v1/agents` 创建持久 Agent，并立即排入第一个 run。要启动不依赖 Git 仓库的临时任务，同时省略 `repos` 与 `env`，或传入 `repos: []`：

```bash
curl --fail-with-body \
  -X POST \
  -H "Authorization: Bearer $CURSOR_API_KEY" \
  -H "Content-Type: application/json" \
  https://api.cursor.com/v1/agents \
  -d '{
    "name": "Temporary analysis",
    "prompt": {
      "text": "Analyze the supplied task and return a concise report."
    },
    "repos": []
  }'
```

成功时返回 `201`，响应同时包含：

- `agent.id`：`bc-...` 格式的 Agent ID；
- `run.id`：`run-...` 格式的首个运行 ID；
- `agent.url`：Cursor Web 中查看 Agent 的地址；
- `agent.repos: []`：确认该 Agent 未绑定仓库。

“临时”是调用方的生命周期策略，不代表服务端自动创建一次性资源。任务结束且不再需要上下文时，应清理 Agent：

```bash
curl --fail-with-body \
  -X DELETE \
  -H "Authorization: Bearer $CURSOR_API_KEY" \
  "https://api.cursor.com/v1/agents/$AGENT_ID"
```

删除不可恢复；如需保留并暂时隐藏，应使用官方 archive 端点。

## Run 状态、流与后续提示

常用端点：

| 端点 | 用途 |
|---|---|
| `POST /v1/agents` | 创建 Agent 并启动首个 run |
| `GET /v1/agents` | 列出 Agent |
| `GET /v1/agents/{id}` | 获取 Agent 元数据 |
| `POST /v1/agents/{id}/runs` | 在现有上下文中提交后续提示 |
| `GET /v1/agents/{id}/runs/{runId}` | 查询 run 状态与结果 |
| `GET /v1/agents/{id}/runs/{runId}/stream` | 使用 SSE 接收 run 事件 |
| `POST /v1/agents/{id}/runs/{runId}/cancel` | 取消活跃 run |
| `DELETE /v1/agents/{id}` | 永久删除 Agent |

run 状态包括 `CREATING`、`RUNNING`、`FINISHED`、`ERROR`、`CANCELLED` 和 `EXPIRED`。SSE 断线恢复应使用服务端返回的事件 ID 和 `Last-Event-ID`，不要解析事件 ID 的内部格式。

## 服务账户的用户范围令牌

团队 My Machines worker 可由具备 agent scope 的服务账户 Key 调用：

```http
POST /v1/sub-tokens
```

请求必须且只能提供一个活跃团队成员：

```json
{
  "forUserEmail": "alice@example.com"
}
```

或：

```json
{
  "forUserId": 42
}
```

返回的 `accessToken` 是短期用户范围令牌，官方规范当前有效期为 1 小时，仅用于相应 worker；它不是可长期存储或继续签发子令牌的 API Key。

## Beta 与功能灰度

- Cloud Agents API 整体应视为官方 Beta，调用方必须兼容字段新增、功能灰度和 `feature_unavailable`。
- `envVars` 也是灰度中的 Beta 能力：未向账户开放时，创建请求可能静默忽略该字段。首次用于生产前，必须在实际 run 中验证变量已注入。
- `envVars` 最多 50 个，名称不能以 `CURSOR_` 开头；值由 Cursor 加密保存、注入 Agent shell，并随 Agent 删除。
- `envVars` 不能与调用方自定义 `agentId` 同时使用。需要会话密钥时应省略 `agentId`，由服务端生成。

## 计费边界

Cursor 官方定价页将 Cloud Agents 纳入符合条件的付费计划，并将模型使用量与超额按需使用纳入 Cursor 自身账单。实际可用额度、模型费率、按需开关、团队限制和企业条款以 Cursor Dashboard 与官方定价文档为准。

Sub2API 必须保持以下边界：

1. **Cursor 侧费用**：Cursor 套餐、模型用量、Cloud Agent 执行和按需超额费用，由 Cursor 向 API Key 所属用户或团队计费。
2. **Sub2API 侧费用**：显式模型默认使用本文内置的 Cursor 平台专属价格，管理员配置的渠道价格优先覆盖；未识别模型必须先配置价格后才能可靠结算。
3. 不得把本地 Token 估算、本地余额、平台 Quota、内置价格快照或 Channel 价格显示为 Cursor 官方 credits。
4. `GET /v1/agents/{id}/usage` 等早期访问能力可能返回 `403 feature_unavailable`；不可据此虚构零用量或无限额度。
5. 若同时启用 Cursor 按需计费与 Sub2API 本地计费，应在运营页面明确提示两套账单可能同时发生，避免用户误认为本地结算替代 Cursor 官方账单。

## 配置示例

```yaml
cursor:
  base_url: "https://api.cursor.com"
  chat_base_url: "https://api2.cursor.sh"
  dashboard_base_url: "https://api2.cursor.sh"
  default_transport_mode: "auto"
  client_version: "3.1.0"
  ghost_mode: false
  new_onboarding_completed: false
  max_frame_bytes: 8388608
  max_buffered_bytes: 16777216
  response_header_timeout_seconds: 60
  ide_stream_idle_timeout_seconds: 60
  request_timeout_seconds: 120
  stream_idle_timeout_seconds: 60
```

API Key 与 Dashboard Token 都属于账号敏感凭据，不应写入共享配置模板。`base_url` 固定限制为 `api.cursor.com`，`chat_base_url` 与 `dashboard_base_url` 固定限制为 `api2.cursor.sh`，避免任一凭据被发送到非 Cursor 主机。单帧和累计缓冲上限用于拒绝异常 Connect 流，生产环境不应无限放大。

## 迁移检查

从旧 Cookie 或“普通聊天创建临时 Cloud Agent”的实现迁移时，应确认：

- 已删除浏览器扩展、扩展打包器、Release 附件和 `/downloads/cursor-cookie-importer.zip`；
- 管理端不再要求 `_vcrcs`、Cookie 过期时间、Referer 或浏览器 User-Agent；
- 每个账号显式保存 `cursor_transport_mode`，旧账号缺失该字段时由 `cursor.default_transport_mode` 决定；
- IDE Chat 账号配置 Dashboard Access Token，建议同时配置 Refresh Token；Cloud Agent 账号配置官方 API Key；
- 普通 `/v1/messages`、`/v1/chat/completions`、`/v1/responses` 在 IDE 模式下使用 `StreamUnifiedChatWithTools`，不再创建临时 Agent；
- IDE 健康检查和模型发现使用 `AvailableModels` Connect RPC；Cloud Agent 检查仍使用 `/v1/me`，模型发现仍使用 `/v1/models`；
- Cloud Agent 调用继续使用 `/v1/agents` 与 run 端点，且只作为独立显式任务模式；
- 反向代理或 CDN 不得缓冲 SSE，并必须允许到 Cursor 上游的 HTTP/2；
- Cursor 官方账单与 Sub2API 本地账单在 UI、API 和文档中明确分开。
