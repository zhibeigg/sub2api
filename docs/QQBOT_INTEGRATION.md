# QQBot 账户绑定集成

Sub2API `0.57.60` 增加供独立 `sub2api-qqbot` 服务使用的 HMAC 私有 API；`0.57.63` 补充绑定前的现有 QQ 身份检测与统一邮箱脱敏响应。QQBot 只接收腾讯官方事件、发送消息、代理网页与管理请求，不直接连接 Sub2API 数据库。

## 架构与信任边界

```text
腾讯官方 Webhook
        │ Ed25519
        ▼
sub2api-qqbot ── HMAC-SHA256 + timestamp + nonce ──► Sub2API
        │                                               │
        ├─ Redis：事件去重、限流、欢迎去重              ├─ Redis：HMAC nonce 防重放
        └─ Vue：绑定页、管理后台                         └─ PostgreSQL：身份、赠送、流水、审计
```

真正绑定主体是腾讯事件提供的 OpenID/频道用户 ID。网页填写的数字 QQ 号只用于展示，不参与认证、唯一性判断或账户合并。

## 配置

`config.yaml`：

```yaml
qqbot_integration:
  enabled: true
  key_id: "qqbot-primary"
  hmac_secret: "至少 32 字节随机密钥"
  public_base_url: "https://qqbot.poke2api.com"
  timestamp_tolerance_seconds: 300
  nonce_ttl_seconds: 600
```

生产环境建议通过环境变量注入：

```dotenv
QQBOT_INTEGRATION_ENABLED=true
QQBOT_INTEGRATION_KEY_ID=qqbot-primary
QQBOT_INTEGRATION_HMAC_SECRET=<secret>
QQBOT_INTEGRATION_PUBLIC_BASE_URL=https://qqbot.poke2api.com
QQBOT_INTEGRATION_TIMESTAMP_TOLERANCE_SECONDS=300
QQBOT_INTEGRATION_NONCE_TTL_SECONDS=600
```

QQBot 侧对应配置 `SUB2API_QQBOT_KEY_ID`、`SUB2API_QQBOT_HMAC_SECRET` 与 Sub2API 内网地址。

## HMAC 协议

请求头：

- `X-QQBot-Key-Id`
- `X-QQBot-Timestamp`：Unix 秒
- `X-QQBot-Nonce`：CSPRNG 随机值
- `X-QQBot-Signature`：HMAC-SHA256 十六进制小写

规范串：

```text
METHOD\nrequest.URL.RequestURI()\ntimestamp\nnonce\nsha256(body)
```

Sub2API 先校验 key、时间窗口和签名，再使用 Redis `SET NX` 写入 nonce。Redis 不可用时 fail-closed，私有请求返回 `503 QQBOT_REPLAY_GUARD_UNAVAILABLE`。

## API

前缀：`/api/v1/integrations/qqbot`

| 方法 | 路径 | 用途 |
|---|---|---|
| POST | `/bindings/prepare` | 先检查 QQ 身份是否已绑定；未绑定时创建邮箱验证挑战并异步发送邮件 |
| POST | `/bindings/inspect` | 检查 token 状态 |
| POST | `/bindings/complete` | 原子完成绑定和首次赠送 |
| GET | `/bindings` | 管理端分页记录 |
| POST | `/bindings/{id}/unbind` | 管理员解绑 |
| GET | `/stats` | 管理统计 |
| GET | `/settings` | 读取运行设置 |
| PATCH | `/settings` | 更新运行设置并写审计 |

所有响应沿用 Sub2API envelope：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

`POST /bindings/prepare` 在腾讯官方身份已经绑定时返回：

```json
{
  "accepted": true,
  "already_bound": true,
  "masked_email": "7***7@qq.com"
}
```

此时不会创建新挑战或重复发送验证邮件。未绑定时 `already_bound` 省略或为 `false`，`masked_email` 仍使用“首字符 + 星号 + 尾字符”的统一格式。

常用错误 reason：`INVALID_BINDING_TOKEN`、`BINDING_EXPIRED`、`BINDING_REVOKED`、`BINDING_DISABLED`、`INVALID_QQ_NUMBER`、`QQ_IDENTITY_CONFLICT`。

## 邮箱验证与防枚举

1. 用户在 QQ 发送 `/bind name@example.com`。
2. QQBot 将事件 ID、Bot AppID、场景、官方身份和邮箱发给 Sub2API。
3. Sub2API 先按 `provider_type=qqbot + BotAppID + 官方身份` 查询现有绑定；已绑定时返回真实账户邮箱的脱敏值，不创建挑战、不发送邮件。
4. 未绑定时，对存在且可用的账户创建一次性挑战，向账户主邮箱发送链接。
5. 未知或禁用账户仍返回同形 `accepted=true`，但不会发送邮件，避免账户枚举。
6. 原始 token 只存在于邮件链接；数据库仅保存 SHA-256。
7. 链接默认 15 分钟过期，完成、过期或撤销后不可再次改变身份或奖励。

## 原子绑定与首次赠送

Migration `183_qqbot_account_binding.sql`：

- 将 `qqbot` 加入 `auth_identities`、`auth_identity_channels`、`pending_auth_sessions` 和 `user_provider_default_grants` 的 provider 约束。
- 创建 `qqbot_binding_challenges` 与 `qqbot_binding_audit_logs`。
- 写入 QQBot 动态设置默认值。

完成绑定在同一个 PostgreSQL 事务中执行：

1. `FOR UPDATE` 锁定挑战。
2. 使用 advisory transaction lock 串行化同一 OpenID 和同一用户。
3. 创建或确认 `provider_type=qqbot` canonical identity 与场景 channel。
4. 插入唯一 ledger：`user_id + qqbot + first_bind`。
5. 只有 ledger 首次插入成功时才增加余额并创建已使用 balance redeem 流水。
6. 更新挑战余额前后快照和完成状态。
7. 写入不可变审计记录。
8. 提交后异步失效余额缓存并发送完成通知。

因此并发点击、Webhook 重投、HTTP 重试和解绑后重绑均不会重复到账。解绑不回收既有余额，也不会删除首次 grant。

## 动态设置

数据库 key：

- `qqbot_binding_enabled`
- `qqbot_first_bind_bonus`
- `qqbot_link_ttl_minutes`
- `qqbot_welcome_enabled`
- `qqbot_first_interaction_enabled`
- `qqbot_help_message`
- `qqbot_allowed_group_ids`
- `qqbot_allowed_guild_ids`
- `qqbot_guild_welcome_channels`

群和频道白名单为空时 QQBot 对相应公共场景 fail-closed；C2C 私聊仍可用。

## 隐私与审计

禁止记录原始 token、完整邮箱、完整 OpenID、QQ AppSecret、Webhook Secret、HMAC secret 或管理员会话。管理列表只返回脱敏邮箱和 OpenID 短指纹。审计覆盖 prepare、complete、expire、email、notify、unbind 和 settings。

## 部署顺序

1. 备份 PostgreSQL、Sub2API 配置和镜像。
2. 部署 Sub2API `0.57.63`（包含 migration `183`、标准 MIME 邮件修复和已绑定身份检测）。
3. 配置并启用 `qqbot_integration`，确认 Redis 可用。
4. 部署 QQBot，先保持 `QQBOT_ENABLED=false`，检查 `/readyz`。
5. 配置 Nginx、TLS 与腾讯 Webhook。
6. 使用测试群/频道完成灰度，再开启正式白名单和绑定。

回滚代码时保留 migration `183` 的新增表和列，采用前向修复；不要删除已经产生的身份、grant、流水或审计记录。
