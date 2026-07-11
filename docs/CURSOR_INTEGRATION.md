# Cursor Cloud Agents 官方 API 接入

Sub2API 的 Cursor 接入应使用 Cursor 官方 **Cloud Agents API（Beta）**，生产基础地址为 `https://api.cursor.com`。旧版基于 `cursor.com/api/chat`、浏览器 Cookie `_vcrcs` 和本地 Cookie 导入扩展的文档与发布链已移除。

> Cloud Agents API 仍处于官方 Beta。端点、字段、权限和可用范围可能调整；部署前应以 Cursor Dashboard、官方文档和官方 OpenAPI 为准。

## 能力边界

- Cloud Agents API 用于创建和管理可自主执行任务的 Cursor Cloud Agent，不是 OpenAI Chat Completions、Anthropic Messages 或 Cursor 桌面聊天的通用反向代理。
- API 使用 Cursor Dashboard 签发的 API Key，不使用浏览器 Cookie、网页登录助手或 Cookie 刷新流程。
- Agent 是持久资源；每次提示词执行对应一个 run。同一 Agent 同时只能有一个活跃 run。
- 可创建关联 GitHub 仓库、Pull Request、命名环境或无仓库的 Agent。无仓库 Agent 适合临时任务，但若不再使用，调用方应显式删除 Agent。
- 模型 ID 必须来自 `GET /v1/models`。省略 `model` 字段表示使用 Cursor 配置的默认模型，不应写死或猜测模型名称。
- Cursor 套餐资格、模型用量、按需超额费用、速率限制和 Cloud Agent 执行成本均由 Cursor 侧决定；Sub2API 不应把本地价格、余额或配额描述成 Cursor 官方额度。

## Sub2API 兼容层语义

- 每个 OpenAI/Anthropic 兼容请求都会创建一个临时无仓库 Agent 和首个 run；完成后删除 Agent。它不是对 Cursor 桌面聊天会话的复用。
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

不要维护静态“官方模型列表”。若调用方希望使用 Cursor 的账户默认模型，应完全省略 `model`，而不是发送空对象或自造模型 ID。

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
2. **Sub2API 侧费用**：仅在管理员明确配置本地渠道价格、用户价格或请求附加费时，由 Sub2API 记录和结算。
3. 不得把本地 Token 估算、本地余额、平台 Quota 或 Channel 价格显示为 Cursor 官方 credits。
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
