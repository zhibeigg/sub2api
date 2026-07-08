# Kiro 账户接入（AWS CodeWhisperer → Claude）

sub2api 支持接入 **Kiro** 平台：把 AWS Kiro / CodeWhisperer 账户作为上游，对外提供 Claude 系列模型。核心逻辑移植自 [Quorinex/Kiro-Go](https://github.com/Quorinex/Kiro-Go)。

## 能力

- 对外端点：**Anthropic `/v1/messages`** 与 **OpenAI `/v1/chat/completions`** 均支持（Kiro 提供 Claude 模型）。
- **四种登录方式**：AWS Builder ID 设备码、IAM Identity Center（PKCE 授权码）、SSO Token 导入、凭证 JSON 粘贴。
- 自动 token 刷新：按 `authMethod` 分流，`idc`（AWS Builder ID / IAM Identity Center）走 AWS OIDC，`social`（GitHub/Google 桌面登录）走 Kiro 桌面刷新端点；到期前自动刷新。
- **账号运营**：订阅类型 / 用量 / 额度 / 试用 / 重置日期 / 超额（Overages）查询与开关；健康检查（usage 探测，自动封禁/恢复）；动态模型发现。
- 上游多 endpoint 自动 fallback（Kiro IDE / CodeWhisperer / AmazonQ），按 profileArn 区域化。
- 流式（AWS 二进制 event-stream 解析 → SSE）与非流式；thinking 模式（`-thinking` 后缀或 Anthropic `thinking` 配置）。
- 工具调用（tool_use / tool_calls）转换；**系统提示过滤器**（Claude Code 检测替换 / env noise / 边界标记 / 自定义规则，默认关闭）。
- **credits / context-usage** 可观测（写入账号快照，不参与计费）。

## 添加 Kiro 账户

后台「账户管理 → 新增账户」，平台选择 **Kiro**，可选四种「添加方式」：

1. **AWS Builder ID（设备码）**：点击「开始登录」→ 浏览器打开授权页并输入 user code → 前端自动轮询，授权成功后建账号。
2. **IAM Identity Center（企业 SSO）**：填 SSO 起始地址 → 生成授权链接 → 浏览器完成授权 → 把回调 URL 粘回「回调 URL」→ 完成登录。
3. **SSO Token 导入**：粘贴 `x-amz-sso_authn` bearer token → 导入并建账号。
4. **凭证 JSON 粘贴**（默认）：直接粘贴凭证 JSON（无需浏览器授权）。

凭证 JSON 字段（camelCase，来自 Kiro-Go 导出；也接受 snake_case）：

```json
{
  "accessToken": "...",
  "refreshToken": "...",
  "clientId": "...",
  "clientSecret": "...",
  "authMethod": "idc",
  "region": "us-east-1",
  "profileArn": "arn:aws:codewhisperer:us-east-1:...:profile/XXXX",
  "machineId": "optional-uuid",
  "expiresAt": 1730000000
}
```

字段说明：

| 字段 | 必填 | 说明 |
|------|------|------|
| `accessToken` | 二选一 | 当前 access token（作为 Bearer） |
| `refreshToken` | 二选一 | 刷新 token（长期有效，推荐提供以便自动续期） |
| `authMethod` | 否 | `idc`（默认）或 `social` |
| `clientId` / `clientSecret` | idc 刷新必填 | AWS OIDC client 凭证（`authMethod=idc` 且要刷新时必填） |
| `region` | 否 | OIDC 区域，默认 `us-east-1` |
| `profileArn` | 建议 | CodeWhisperer profile ARN，生成请求需要；缺失时会尝试自动解析 |
| `machineId` | 否 | 写入 User-Agent 尾巴，用于请求追踪 |
| `expiresAt` | 否 | access token 过期 Unix 秒；缺失则视为需刷新 |

> **注意**：AWS Builder ID（个人）不支持 `ListAvailableProfiles`，请确保凭证里带上 `profileArn`。

## 使用

把 Kiro 账户加入一个 `platform = kiro` 的分组，然后像调用普通 Claude 分组一样调用：

```bash
# Anthropic 格式
curl https://your-domain/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: <你的 sub2api key>" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model":"claude-sonnet-4.5","max_tokens":1024,"messages":[{"role":"user","content":"Hello!"}]}'

# OpenAI 格式
curl https://your-domain/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <你的 sub2api key>" \
  -d '{"model":"claude-sonnet-4.5","messages":[{"role":"user","content":"Hello!"}]}'
```

### 模型与 thinking

- 模型名会经内部映射归一化（如 `claude-3-5-sonnet` → `claude-sonnet-4.5`，`gpt-4o` → `claude-sonnet-4.5`）。
- thinking：模型名带 `-thinking` 后缀（如 `claude-sonnet-4.5-thinking`），或 Anthropic 请求体顶层 `thinking: {"type":"enabled"}` / `{"type":"adaptive"}`。

## 管理端点（admin，需管理员鉴权）

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/admin/kiro/oauth/builderid/start` | 启动 Builder ID 设备码登录，返回 `session_id/user_code/verification_uri/interval` |
| POST | `/admin/kiro/oauth/builderid/poll` | 轮询设备授权状态（`pending`/`completed`），完成时返回 `credentials` |
| POST | `/admin/kiro/oauth/iam-sso/start` | 生成 IAM SSO 授权链接（PKCE），返回 `session_id/auth_url/state` |
| POST | `/admin/kiro/oauth/iam-sso/complete` | 用回调 URL 兑换 token，返回 `credentials` |
| POST | `/admin/kiro/oauth/sso-token` | 用 SSO bearer token 导入，返回 `credentials` |
| POST | `/admin/kiro/oauth/create-account` | 用 `credentials` 创建 Kiro 账号 |
| POST | `/admin/kiro/accounts/:id/refresh` | 手动刷新账号 token |
| GET  | `/admin/kiro/accounts/:id/usage` | 探测并返回用量/订阅/超额快照 |
| POST | `/admin/kiro/accounts/:id/overage` | 开关账号超额（body `{"enabled":true}`） |

## 配置（可选）

`config.yaml` 的 `kiro` 段可覆盖客户端版本、启用系统提示过滤器、设置 thinking 输出格式：

```yaml
kiro:
  client:
    kiro_version: ""      # 留空用内置默认
    node_version: ""
    system_version: ""
  filter:
    claude_code: false    # 检测 Claude Code CLI 提示并替换
    env_noise: false      # 剥离环境元数据行
    strip_boundaries: false
    rules: []             # 自定义 regex / lines-containing 规则
  thinking:
    suffix: "-thinking"
    openai_format: "reasoning_content"  # reasoning_content | thinking | think
    claude_format: "thinking"
```

## 实现位置（开发者）

- `backend/internal/pkg/kiro/`：自包含核心库（types / oidc 刷新 / oauth 交互式登录底层 / rest 用量接口 / overage 超额 / headers / client + AWS event-stream 解析 / translator + filters / stream SSE 组装），含单元测试。
- `backend/internal/service/kiro_oauth_service.go` / `kiro_oauth_interactive.go`：token 刷新、凭证 JSON 解析、交互式登录 session。
- `backend/internal/service/kiro_usage_service.go` / `kiro_usage_fetcher.go`：用量/超额探测、账号 Extra 快照、UsageInfo 构建、模型发现缓存。
- `backend/internal/service/kiro_token_provider.go` / `kiro_token_refresher.go`：请求路径取 token（带缓存）+ 后台定时刷新。
- `backend/internal/service/kiro_gateway_service.go`：`Forward`（双端点转发，SSE / JSON）+ credits/context-usage 回调。
- `backend/internal/handler/admin/kiro_oauth_handler.go`：管理端点；路由在 `backend/internal/server/routes/admin.go`。
- 平台常量 `PlatformKiro`、`DefaultKiroModelMapping` 在 `backend/internal/domain/constants.go`。
- 前端：`CreateAccountModal.vue`（四种添加方式）、`composables/useKiroOAuth.ts`、`api/admin/kiro.ts`、`platformColors.ts`、`PlatformIcon.vue`、i18n。

## 限制与说明

- **credits 仅可观测**：Kiro 的 credits/context-usage 记录到账号快照用于展示，不改 `usage_logs` 表、不参与计费。
- AWS Builder ID（个人）不支持 `ListAvailableProfiles`，会自动回退到刷新 token 解析 profileArn，并对失败做 24h 冷却抑制。
- Kiro 上游返回 402/403/401/429 时按 sub2api 常规上游错误处理（失败切换 / 账户标记）。
- 免责声明：本功能仅供学习研究，与 Amazon / AWS / Kiro 无关联，使用者需自行遵守相关服务条款与法律。
