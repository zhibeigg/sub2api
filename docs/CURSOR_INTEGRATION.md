# Cursor Cloud Agents 官方 API 接入

Sub2API 的 Cursor 接入应使用 Cursor 官方 **Cloud Agents API（Beta）**，生产基础地址为 `https://api.cursor.com`。旧版基于 `cursor.com/api/chat`、浏览器 Cookie `_vcrcs` 和本地 Cookie 导入扩展的文档与发布链已移除。

> Cloud Agents API 仍处于官方 Beta。端点、字段、权限和可用范围可能调整；部署前应以 Cursor Dashboard、官方文档和官方 OpenAPI 为准。

## 能力边界

- Cloud Agents API 用于创建和管理可自主执行任务的 Cursor Cloud Agent，不是 OpenAI Chat Completions、Anthropic Messages 或 Cursor 桌面聊天的通用反向代理。
- API 使用 Cursor Dashboard 签发的 API Key，不使用浏览器 Cookie、网页登录助手或 Cookie 刷新流程。
- Agent 是持久资源；每次提示词执行对应一个 run。同一 Agent 同时只能有一个活跃 run。
- 可创建关联 GitHub 仓库、Pull Request、命名环境或无仓库的 Agent。无仓库 Agent 适合临时任务，但若不再使用，调用方应显式删除 Agent。
- 生产使用的模型 ID 应以 `GET /v1/models` 为准。管理后台内置 40 个模型兼容 ID 作为离线默认目录，新建与编辑账号表单可调用上游模型接口并把返回 ID 合并到模型白名单；省略 `model` 字段表示使用 Cursor 配置的默认模型。
- Cursor 账号可显式启用混合调度。开启后可按模型产品族加入 Cursor、Anthropic、Gemini、OpenAI 与 Grok 分组，覆盖 Claude、Gemini、GPT、Grok、GLM、Kimi、Composer/default 等已同步模型；兼容层可承接 `/v1/messages`、`/v1/chat/completions` 与 `/v1/responses`。Gemini 原生 `generateContent` 不会调度到 Cursor。不同上游的会话上下文与模型能力可能不同，应通过分组隔离账号，并只启用已同步且验证可用的模型。
- Cursor 套餐资格、模型用量、按需超额费用、速率限制和 Cloud Agent 执行成本均由 Cursor 侧决定；Sub2API 不应把本地价格、余额或配额描述成 Cursor 官方额度。

## Sub2API 兼容层语义

- 每个 OpenAI/Anthropic 兼容请求都会创建一个临时无仓库 Agent 和首个 run；完成后删除 Agent。分组平台仅决定渠道、计费与调度范围，实际模型 ID 会按 Cursor 账号映射传给 Cloud Agents API。它不是对 Cursor 桌面聊天会话的复用。
- `previous_response_id` 由 Sub2API 在 Redis 中保存对话并在下一次请求时重新整理为 prompt，不会复用已删除的 Cursor Agent。
- 入站 `stream=true` 会消费 Cursor Run SSE 并在任务完成后合成为现有 OpenAI/Anthropic SSE 格式；它不是逐事件透明转发，首字节延迟受整个 Agent run 时长影响。
- 工具定义会被编码为明确的 JSON fenced-action 指令，再转换回 OpenAI/Anthropic tool call；这属于兼容约定，不等同于 Cloud Agent 原生 `tool_call` 事件。
- 当前兼容层只接受文本、系统指令、工具调用与工具结果。OpenAI/Anthropic 的图片、音频、文件和文档输入会返回明确的 `400`，即使 Cloud Agents 原生 API 本身支持图片。
- 若 Run SSE 在 `result` 前正常结束，Sub2API 会轮询 Run 直到终态；客户端取消或运行失败时会取消活跃 Run，并异步重试删除临时 Agent。

## 认证

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
- 点击“刷新检测”或刷新账号列表时，管理 API 会以 `force=true` 调用 Cursor `GET /v1/me` 验证当前 API Key，并返回检测状态和时间；普通自动加载不会批量探测上游。
- 管理接口为 `GET /api/v1/admin/accounts/{id}/usage?source=active&force=true`。Cursor 响应包含 `cursor_local_usage`、`cursor_api_key_configured`、`cursor_probe_state`、`cursor_probe_message` 和 `cursor_checked_at`。
- Cursor Run 返回的 `cacheWriteTokens` 和 `cacheReadTokens` 分别保存为统一用量记录的 `cache_creation_tokens`（界面显示“缓存写入”）和 `cache_read_tokens`，并参与平台专属计费。

## 获取模型：`GET /v1/models`

```bash
curl --fail-with-body \
  -H "Authorization: Bearer $CURSOR_API_KEY" \
  https://api.cursor.com/v1/models
```

响应的 `items[].id` 可作为创建 Agent 时的 `model.id`。模型还可能提供：

- `aliases`：解析到同一模型的别名；
- `parameters`：允许传入的参数和值；
- `variants`：有效的 `id + params` 组合以及默认变体。

Sub2API 内置的模型目录只是离线兼容快照，不替代上游发现结果。账号可用模型、变体和参数应以该 API 的实时响应为准；若调用方希望使用 Cursor 的账户默认模型，应完全省略 `model`，而不是发送空对象。

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
  request_timeout_seconds: 120
  stream_idle_timeout_seconds: 60
```

API Key 属于账号凭据，不应写入共享配置模板。`base_url` 应保持为官方 HTTPS 地址；如未来允许覆盖，也必须限制到可信 Cursor API 主机，避免凭据被发送到非官方端点。

## 迁移检查

从旧 Cookie 文档聊天实现迁移时，应确认：

- 已删除浏览器扩展、扩展打包器、Release 附件和 `/downloads/cursor-cookie-importer.zip`；
- 管理端不再要求 `_vcrcs`、Cookie 过期时间、Referer 或浏览器 User-Agent；
- 凭据类型改为用户 API Key 或服务账户 API Key；
- 健康检查使用 `/v1/me`，模型发现使用 `/v1/models`；
- Agent 调用使用 `/v1/agents` 与 run 端点，而不是 `/api/chat` 或通用聊天补全协议；
- Cursor 官方账单与 Sub2API 本地账单在 UI、API 和文档中明确分开。
