# Kiro 账户接入（AWS CodeWhisperer → Claude）

sub2api 支持接入 **Kiro** 平台：把 AWS Kiro / CodeWhisperer 账户作为上游，对外提供 Claude 系列模型。核心逻辑移植自 [Quorinex/Kiro-Go](https://github.com/Quorinex/Kiro-Go)。

## 能力

- 对外端点：**Anthropic `/v1/messages`** 与 **OpenAI `/v1/chat/completions`** 均支持（Kiro 提供 Claude 模型）。
- 自动 token 刷新：按 `authMethod` 分流，`idc`（AWS Builder ID / IAM Identity Center）走 AWS OIDC，`social`（GitHub/Google 桌面登录）走 Kiro 桌面刷新端点；到期前自动刷新。
- 上游多 endpoint 自动 fallback（Kiro IDE / CodeWhisperer / AmazonQ），按 profileArn 区域化。
- 流式（AWS 二进制 event-stream 解析 → SSE）与非流式；thinking 模式（`-thinking` 后缀或 Anthropic `thinking` 配置）。
- 工具调用（tool_use / tool_calls）转换。

## 添加 Kiro 账户

后台「账户管理 → 新增账户」，平台选择 **Kiro**，粘贴凭证 JSON 即可（无需浏览器授权）。

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

## 实现位置（开发者）

- `backend/internal/pkg/kiro/`：自包含核心库（types / oidc 刷新 / headers / client + AWS event-stream 解析 / translator / stream SSE 组装），含单元测试。
- `backend/internal/service/kiro_oauth_service.go`：token 刷新 + 凭证 JSON 解析。
- `backend/internal/service/kiro_token_provider.go` / `kiro_token_refresher.go`：请求路径取 token（带缓存）+ 后台定时刷新。
- `backend/internal/service/kiro_gateway_service.go`：`Forward`（双端点转发，SSE / JSON）。
- 平台常量 `PlatformKiro`、`DefaultKiroModelMapping` 在 `backend/internal/domain/constants.go`。
- 前端：`CreateAccountModal.vue`（Kiro 平台卡片 + 凭证 JSON 导入）、`platformColors.ts`、`PlatformIcon.vue`、i18n。

## 限制与后续

- 当前仅支持**粘贴凭证 JSON 导入**；浏览器内 OAuth 授权流程（Builder ID device-code / IAM SSO PKCE）后续可作为增强。
- Kiro 上游返回 402/403/401/429 时按 sub2api 常规上游错误处理（失败切换 / 账户标记）。
- 免责声明：本功能仅供学习研究，与 Amazon / AWS / Kiro 无关联，使用者需自行遵守相关服务条款与法律。
