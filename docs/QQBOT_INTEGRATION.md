# QQBot BotGo + SnowLuma OneBot v11 双链路集成指南

Sub2API `0.82.90` 保留腾讯官方 BotGo Webhook、消息 Runtime、绑定页面和运营管理，并新增 SnowLuma OneBot v11 并行接入。BotGo 继续处理官方群、C2C 与 QQ 频道能力；SnowLuma 通过本机反向 WebSocket 提供普通 QQ 群消息和 `group_increase` 真实进群事件。两条链路互不替换，管理员统一从 Sub2API 后台的 **QQBot** 页面配置、启停和诊断。

已有 QQBot 绑定数据、身份、首次赠送、余额流水和审计继续保存在 Sub2API PostgreSQL 中。OneBot 使用独立机器人标识和 Redis 事件流，不会与 BotGo OpenID、队列或活动配置混淆。

## 1. 架构

```text
腾讯官方 Webhook                         SnowLuma / Linux QQ
        │ Ed25519 / op=13                       │ Bearer + X-Self-ID
        ▼                                       ▼
Sub2API /webhooks/qq                  /webhooks/qq/onebot (reverse WS)
        │                                       │
        ├─ BotGo Runtime                        ├─ OneBot Runtime
        │   ├─ Redis Stream: sub2api:qqbot      │   ├─ 独立 Stream: sub2api:qqbot:onebot
        │   └─ BotGo OpenAPI                    │   └─ action/echo 通过同一 WS 返回
        │                                       │
        └──────────────┬────────────────────────┘
                       ▼
              共享命令、欢迎与绑定处理器
                       │
        ┌──────────────┴────────────────────────┐
        │ PostgreSQL                            │ Vue
        │ ├─ 两套加密 Runtime 配置              │ ├─ /admin/qqbot
        │ ├─ 绑定挑战与审计                     │ └─ /bind
        │ └─ identity、grant、余额与流水        │
        └───────────────────────────────────────┘
```

旧的 `/api/v1/integrations/qqbot` HMAC API 在迁移和回滚窗口内保留。独立实例删除后应关闭 `qqbot_integration.enabled` 并移除 HMAC 环境变量。

## 2. 身份与奖励规则

- 认证主体始终是腾讯事件中的官方 OpenID 或频道用户 ID。
- `provider_subject` 保持 `<scene>:<subject>` 格式，并与 `bot_app_id` 一起隔离身份。
- 支持场景：`group`、`c2c`、`guild`。
- 网页输入的数字 QQ 号只用于展示，不参与认证、唯一性判断或账户合并。
- 原始邮箱 token 只存在于邮件链接，数据库仅保存 SHA-256。
- 首次奖励唯一维度为 `user_id + provider_type=qqbot + grant_reason=first_bind`。
- 身份、provider grant、余额、兑换流水和挑战完成在一个 PostgreSQL 事务中提交。
- 解绑不删除历史 grant、不回收奖励，重新绑定不会再次获得首绑奖励。

## 3. 后台配置

入口：`/admin/qqbot`。

页面包括：

1. **概览**：BotGo 启用状态、Runtime 状态、队列积压、worker、最近 Webhook/事件/发送时间。
2. **BotGo 配置**：AppID、AppSecret、Webhook Secret、Sandbox、公共域名、worker、队列和 API timeout。
3. **SnowLuma / OneBot**：Self ID、加密 Token、反向 WS 地址、连接状态、独立队列、action pending、probe 和启用门禁。
4. **消息与欢迎**：绑定开关、首绑奖励、链接 TTL、独立帮助/欢迎文案、群/频道白名单与频道欢迎映射。
5. **绑定记录**：统计、分页、状态/场景/时间筛选和管理员解绑。
6. **诊断**：BotGo Webhook URL、域名归属校验 URL、配置版本、最近稳定错误码和腾讯凭据 probe。

### 3.1 敏感字段

AppSecret 和 Webhook Secret 使用 Sub2API 现有 `TOTP_ENCRYPTION_KEY` 派生的 AES-256-GCM encryptor 加密后保存到 PostgreSQL。

管理 API 只返回：

```json
{
  "app_secret_configured": true,
  "webhook_secret_configured": true
}
```

不会返回明文或密文。更新时将密钥字段留空表示保留原值；只有非空新值才会替换。日志、HTTP 响应和审计记录不得包含密钥。

生产必须显式配置稳定的 `TOTP_ENCRYPTION_KEY`。更换该密钥前必须先完成受控的密文重加密，否则历史 QQBot 凭据无法解密。

### 3.2 Runtime 配置

数据库设置 `qqbot_runtime_config` 保存：

- `enabled`
- `app_id`
- 加密后的 AppSecret/Webhook Secret
- `sandbox`
- `public_base_url`：必须是腾讯服务器可从公网访问的 HTTPS 根域地址，用于 Webhook、绑定页和 `/check` 的短期 HMAC 签名 PNG URL；启用 `/check` 时不得包含路径前缀、查询参数或用户信息，也不得填写内网地址、单标签内部主机、`localhost` 或 HTTP 地址
- `worker_count`
- `queue_capacity`
- `api_timeout_ms`
- `config_version`、更新人、更新时间和脱敏变更摘要

配置使用版本乐观锁。管理员保存成功后，本实例立即安装新快照，并通过 Redis Pub/Sub 通知其他实例 reload；周期 reload 作为兜底。

启用前会校验必填项并执行腾讯凭据 probe。probe 成功后，主 Redis 仅保存与 AppID、AppSecret、Webhook Secret、Sandbox 组合绑定的哈希指纹凭证，有效期 5 分钟；启用、AppID/Sandbox 变更或任一密钥轮换时，保存接口会在服务端强制校验该凭证，不能只绕过前端直接提交。新配置无法激活时，旧活动配置继续运行，后台显示 `degraded` 原因，不会把无效密钥静默切入生产。

`PublicBaseURL` 必须使用腾讯服务器可从公网访问的 HTTPS 根域，并正确反向代理到 Sub2API；它不能是容器名、内网 IP、单标签内部主机、`localhost`、带路径前缀的 URL 或仅管理员浏览器可访问的地址。开启 `channel_check_enabled` 时，后端保存与 probe 会强制执行该校验，前端也会提前阻止无效地址。`/check` PNG 默认使用运行镜像内的 Noto CJK 字体；如需自定义字体，可设置可选环境变量 `QQBOT_CHANNEL_CHECK_FONT_PATH` 指向容器内可读的字体文件，并确保运行用户具有读取权限。

### 3.2.1 SnowLuma / OneBot Runtime 配置

数据库设置 `qqbot_onebot_runtime_config` 独立保存：

- `enabled`，默认 `false`。
- `self_id`：SnowLuma 当前登录机器人的数字 QQ。
- `access_token_ciphertext`：使用与 BotGo 相同的 AES-256-GCM encryptor 加密；管理 API 只返回 `access_token_configured`。
- `worker_count`：默认 `2`。
- `queue_capacity`：默认 `1024`，对应独立 Redis Stream 与 consumer group。
- `action_timeout_ms`：默认 `10000`。
- `config_version`、更新人、更新时间和不含秘密的变更摘要。

启用流程必须按顺序执行：先以 disabled 保存 Self ID 与 Token；在 SnowLuma 配置 `ws://127.0.0.1:8080/webhooks/qq/onebot`、相同 Token、`messageFormat=array`、`reportSelfMessage=false`；确认反向 WS 连接后执行 `get_login_info` probe；只有当前 Self ID、Token 与超时组合在 5 分钟内 probe 成功，服务端才允许启用。SnowLuma 的 OneBot HTTP/正向 WS 端口必须继续只监听 `127.0.0.1`，不得为了 Sub2API 接入新增公网动作 API。

### 3.3 业务设置

为兼容现有生产数据，以下 key 继续保留：

- `qqbot_binding_enabled`
- `qqbot_first_bind_bonus`
- `qqbot_link_ttl_minutes`
- `qqbot_welcome_enabled`
- `qqbot_welcome_message`
- `qqbot_first_interaction_enabled`
- `qqbot_help_message`
- `qqbot_allowed_group_ids`
- `qqbot_allowed_guild_ids`
- `qqbot_guild_welcome_channels`
- `qqbot_channel_check_enabled`

群或频道白名单为空时，对应公共场景 fail-closed；C2C 私聊仍可使用绑定与帮助命令。`/check` 的权限更严格：白名单内的群/频道身份可直接使用，无需个人绑定；C2C 必须存在当前机器人 AppID 下、场景与官方身份均匹配且状态为 `active` 的绑定。

`/check` 采用双开关：QQBot 管理配置中的 `channel_check_enabled` 控制机器人入口，站点设置中的 `channel_monitor_enabled` 控制渠道监控能力；只有两者同时为 `true` 才执行检查。`channel_check_enabled` 在新安装和升级迁移中均默认保持关闭，管理员配置共享稳定的 `totp.encryption_key` 与公网根 HTTPS `PublicBaseURL` 后再显式开启。关闭任一开关时返回通用不可用提示，不泄露内部开关状态或监控数据。

腾讯官方 BotGo 没有普通 QQ 群成员加入事件；启用 SnowLuma 后，OneBot `notice/group_increase` 为白名单普通群提供真实进群欢迎，并以 `at + text` 消息段 @ 新成员。QQ 频道仍由 BotGo `GUILD_MEMBER_ADD` 和 `guild_welcome_channels` 处理，不改变原行为。欢迎文案使用独立 `qqbot_welcome_message`，支持纯文本占位符 `{site}`、`{user}`、`{bind_command}`；用户名称会移除控制字符、换行和可能形成提及/命令的字符，关闭绑定或 `/check` 时对应指令行自动省略。

## 4. Webhook 和消息处理

### 4.1 腾讯入口

```text
POST /webhooks/qq
```

- `op=13`：使用配置的 Webhook Secret（未单独配置时使用 AppSecret）签名 `event_ts + plain_token` 并返回地址校验响应。`X-Bot-Appid` 仅用于记录配置偏差，不会在签名证明完成前提前拒绝腾讯的地址校验请求。
- dispatch：使用 BotGo 官方 Ed25519 方案验证请求签名。
- 心跳和其他 opcode 按腾讯协议返回 ACK。
- 请求体有大小限制；非法签名、配置不可用或可靠队列不可用时 fail-closed。

域名归属校验文件由活动 AppID 动态提供：

```text
GET /<AppID>.json
```

响应只包含对应 `bot_appid`，AppID 变化后无需重新构建前端静态文件。

### 4.2 SnowLuma 反向 WebSocket

```text
GET /webhooks/qq/onebot
```

- 仅接受未经过外部代理、来源为 loopback/私有地址的直接 WebSocket Upgrade；任何 `Forwarded`、`X-Forwarded-*` 或 `X-Real-IP` 痕迹都会拒绝。
- `Authorization: Bearer <token>` 以固定时间摘要比较校验；`X-Self-ID` 必须精确等于保存的 Self ID。
- 每个配置代际只允许一个活动连接；新连接安全替换旧连接，旧连接上的 pending action 立即失败并可重试。
- action 使用唯一 `echo` 并发关联，限制 pending 数量、单帧大小与调用超时；断线、取消和配置重载会清理 pending 请求。
- 支持 `message/group`、`message/private`、`notice/group_increase`；忽略机器人自身事件、未知事件和缺少关键 ID 的 payload。OneBot 没有原生 event ID 时，使用事件类型、时间、Self ID、群、用户和消息 ID 生成稳定 SHA-256 指纹。
- `send_group_msg`、`send_private_msg`、`get_login_info` 均通过同一反向 WS 执行，不暴露额外动作端口。

### 4.3 可靠事件队列

Webhook 或 OneBot 完成鉴权和事件规范化后，使用 Redis Lua 原子完成：

1. event ID 24 小时去重。
2. 加密事件 payload。
3. `XADD` 到 QQBot Stream。

BotGo 使用 `sub2api:qqbot` 前缀和 `qqbot-runtime` consumer group；OneBot 使用 `sub2api:qqbot:onebot` 前缀和独立 consumer group。两套 backlog、pending、重试、死信、首次互动去重和绑定限流不会互相消费或覆盖。只有可靠写入成功后才确认接收；worker 支持 pending reclaim、有限重试、死信和主进程重启恢复，事件成功后才 `XACK`。

欢迎去重保留 180 天，`/bind` 默认每个官方身份 5 分钟 3 次。Redis I/O 使用有超时的 context，worker 和 reclaim goroutine 统一由 Runtime 生命周期管理。停用、重载或主进程退出时先停止接收新事件，并在最多 10 秒内等待 backlog/pending 清空；超过门限后取消 worker，未完成消息仍保留在 pending 中供下次 reclaim，不会因强制 ACK 丢失。

### 4.4 命令

支持：

```text
/help
help
帮助
/帮助
```

```text
/bind name@example.com
bind name@example.com
绑定 name@example.com
/绑定 name@example.com
```

```text
/check
check
```

未知邮箱和存在邮箱返回同形结果，防止账户枚举。已绑定的官方身份只返回真实账户邮箱的脱敏值，不创建新挑战或重复发送邮件。

`/check` 查询站点当前渠道监控结果并渲染为 PNG：

- 白名单内 QQ 群和 QQ 频道可直接使用，不要求发送者先绑定账户。
- C2C 私聊只允许当前官方身份存在 `active` 绑定时使用；过期、撤销、解绑或其他 AppID/场景的绑定均不可替代。
- 同一官方身份每 30 秒最多执行一次；同一事件的可靠队列重试不重复占用额度。限流依赖 Redis，无法可靠判定时 fail-closed，并只返回通用稍后重试提示。
- QQBot 配置字段 `channel_check_enabled` 与站点字段 `channel_monitor_enabled` 必须同时开启；任一关闭都不读取或展示渠道结果。
- 结果仅包含适合公开展示的渠道名称、状态与必要时间信息，不包含账号凭据、代理、上游响应正文、内部错误、数据库键或管理员信息。

### 4.5 PNG 生成与消息发送

PNG 在服务端内存中生成，不把渠道数据或敏感信息写入公共静态目录。公开图片地址基于 `PublicBaseURL` 的固定根路径生成，使用短期有效的 HMAC 签名、随机 nonce 与过期时间；签名 URL 只授权读取对应 PNG，过期、签名错误、重复/额外查询参数或路径被篡改时拒绝访问，且每个 nonce 最多允许 4 次腾讯拉取，不能用作其他接口凭据。签名密钥从显式配置且各实例共享的 `totp.encryption_key` 做域分离派生；启用 QQBot 且开启 `/check` 时，保存与 probe 都会拒绝进程自动生成的临时密钥，避免多实例或重启后出现随机验签失败。

腾讯不同场景使用各自支持的图片发送方式：

- **QQ 群 / C2C**：先通过腾讯文件上传接口取得 `file_info`，再以 `msg_type=7` 发送富媒体消息；同一事件重试复用固定 `msg_seq` 和已取得的 `file_info`，避免响应丢失后重复上传或重复消息；不得把内部文件路径或 HMAC 密钥放入消息体。
- **QQ 频道**：发送指向短期 HMAC PNG 的公网 HTTPS image URL；该 URL 必须能由腾讯服务器直接访问。

字体加载、PNG 渲染、签名 URL 生成、腾讯上传或图片消息发送出现临时错误时先由可靠队列使用相同消息序号重试；确认属于不可重试的确定性失败后，机器人回退为简短通用文本，提示暂时无法生成渠道检查图片并稍后重试；回退消息、日志、HTTP 响应和审计仅记录稳定错误码与短请求指纹，不暴露 AppSecret、Webhook Secret、HMAC 密钥、完整 OpenID、绑定邮箱、上游响应正文或可复用的未过期签名 URL。反向代理必须为图片精确路径关闭 access log 或使用不含 `$args` 的专用日志格式；仓库内 QQBot Nginx 模板已对该路径设置 `access_log off`。

## 5. HTTP API

所有响应沿用 Sub2API envelope：

```json
{
  "code": 0,
  "message": "success",
  "data": {}
}
```

### 5.1 公共绑定 API

#### 检查链接

```text
POST /api/v1/public/bindings/inspect
```

```json
{ "token": "raw-email-token" }
```

#### 完成绑定

```text
POST /api/v1/public/bindings/complete
```

```json
{
  "token": "raw-email-token",
  "qq_number": "12345678"
}
```

`qq_number` 必须是 5 至 12 位数字且不能以 0 开头。公共接口使用统一安全客户端 IP 解析：inspect 为每个来源 IP 每分钟 30 次，complete 为每个来源 IP 每 10 分钟 10 次；不返回用户 ID、完整邮箱或完整 OpenID。

主要错误 reason：

- `INVALID_BINDING_TOKEN`
- `BINDING_EXPIRED`
- `BINDING_REVOKED`
- `BINDING_DISABLED`
- `INVALID_QQ_NUMBER`
- `QQ_IDENTITY_CONFLICT`

### 5.2 管理 API

全部位于 Sub2API 标准管理员认证、合规和操作审计中间件之后：

| 方法 | 路径 | 用途 |
|---|---|---|
| GET | `/api/v1/admin/qqbot/config` | 读取脱敏配置 |
| PUT | `/api/v1/admin/qqbot/config` | 原子更新 Runtime 与业务设置 |
| POST | `/api/v1/admin/qqbot/probe` | 测试腾讯凭据/连接 |
| GET | `/api/v1/admin/qqbot/runtime` | BotGo Runtime、队列与版本状态 |
| GET | `/api/v1/admin/qqbot/onebot/config` | 读取 OneBot 脱敏配置 |
| PUT | `/api/v1/admin/qqbot/onebot/config` | 更新 OneBot 配置与启用状态 |
| POST | `/api/v1/admin/qqbot/onebot/probe` | 通过反向 WS 执行 `get_login_info` |
| GET | `/api/v1/admin/qqbot/onebot/runtime` | OneBot 连接、action、独立队列与版本状态 |
| GET | `/api/v1/admin/qqbot/stats` | 绑定统计 |
| GET | `/api/v1/admin/qqbot/bindings` | 分页和筛选绑定记录 |
| POST | `/api/v1/admin/qqbot/bindings/:id/unbind` | 管理员解绑 |

`GET /api/v1/admin/qqbot/config` 的脱敏响应与 `PUT /api/v1/admin/qqbot/config` 的更新请求均包含 QQBot 检查入口开关：

```json
{
  "channel_check_enabled": true
}
```

该字段只控制 QQBot `/check` 入口；站点级 `channel_monitor_enabled` 仍由系统设置管理 API 管理。运行时按双开关取有效值，必须同时为 `true`。管理前端调用 `POST /api/v1/admin/qqbot/probe` 时也会提交草稿中的 `channel_check_enabled`，使服务端按本次候选配置校验 HTTPS 与稳定签名密钥；旧客户端省略该字段时沿用当前已保存值。读取配置时不会因开关开启而附带渠道监控明细，更新配置时也不得接受客户端提交 HMAC 密钥、字体路径或签名 URL。

OneBot 配置响应只包含 `access_token_configured`，更新请求中 `access_token` 留空表示保留现值。`enabled=true` 时后端强制检查当前配置对应的短期 probe 凭证；前端流程不能替代服务端门禁。

管理员主体从认证上下文读取，客户端不能提交或伪造 `admin_subject`。

### 5.3 旧版 HMAC API

迁移窗口内保留：

```text
/api/v1/integrations/qqbot
```

它仅用于旧独立实例回滚。正常单体运行不依赖该接口。旧版 `PATCH /settings` 明确拒绝 `channel_check_enabled` 字段，避免绕过内置管理接口对共享稳定密钥与公网根 HTTPS URL 的原子校验。独立实例删除后：

- `qqbot_integration.enabled=false`
- 删除 `QQBOT_INTEGRATION_HMAC_SECRET` 等环境变量
- 不长期保留旧 HMAC secret

## 6. 安全与隐私

禁止记录或返回：

- QQ AppSecret、Webhook Secret、OneBot access token、旧 HMAC secret
- 原始邮件验证 token
- 完整邮箱
- 完整 OpenID/provider subject、完整 QQ/Self ID、群 ID
- 完整消息正文、OneBot action 响应或 `/bind` 邮箱参数
- `/check` PNG 的 HMAC 密钥、内部存储路径、完整渠道探测响应或可复用的未过期签名 URL
- 渠道账号凭据、代理配置、内部数据库/Redis key
- 管理员 Token、Cookie 或密码

日志只使用短 event/request ID、稳定错误码、脱敏邮箱和哈希指纹。QQBot 业务审计的 actor subject 使用哈希/短指纹，不保存完整官方身份。`/check` 的失败日志不得记录 PNG 二进制、完整腾讯上传响应或完整签名 URL；必要时只记录场景、错误阶段和不可逆短指纹。

管理员修改密钥、启停、解绑和 probe 均进入 Sub2API 统一审计。配置变更摘要只保存布尔状态、数量和哈希。

## 7. 从独立实例迁移

### 7.1 迁移前备份

备份：

- 当前 Sub2API 镜像、Compose 和 Nginx QQBot 配置。
- PostgreSQL 全量数据。
- 主 Redis 与 QQBot 专用 Redis 的持久数据。
- 独立 QQBot Compose、`.env` 和镜像的加密离线回滚包。

不得通过终端、日志或工单输出 `.env` 内容。

### 7.2 配置导入

首次部署内置 Runtime 时，migration 会写入一条 pristine、`enabled=false` 的 `qqbot_runtime_config`。当该记录尚未被管理员修改且检测到 `QQBOT_APP_ID`、`QQBOT_APP_SECRET`、`QQBOT_WEBHOOK_SECRET` 或显式 `QQBOT_PUBLIC_BASE_URL` 时，Runtime 可在受控部署窗口执行一次性导入；旧独立服务的通用 `PUBLIC_BASE_URL` 只有在同一环境同时存在 QQBot 凭据时才会读取，避免误用主站域名。

1. 读取旧 AppID、AppSecret、Webhook Secret、Sandbox、公共域名和 worker 参数。
2. 使用 Sub2API encryptor 加密敏感字段。
3. 写入数据库，仍保持 disabled，并标记 bootstrap 已完成，后续启动不重复导入。
4. 在后台确认只显示 configured 状态并通过 probe。
5. 从主 Compose 移除一次性 bootstrap secret，再重新创建主容器。

导入完成后所有修改都在 `/admin/qqbot` 完成。

### 7.3 Redis 状态

把旧专用 Redis 的 `sub2api:qqbot:*` 去重/欢迎键复制到主 Redis，保留原键名和剩余 TTL。迁移只比较键数量、类型和 TTL 范围，不读取或输出值。

新 Redis Stream 从空队列开始。切流前主 Redis 不应存在冲突的旧 QQBot key。

### 7.4 Nginx 切流

保持腾讯平台和邮件使用的域名：

```text
https://qqbot.mcwar.cn
```

把 QQBot Nginx upstream 从：

```text
127.0.0.1:8090
```

改为：

```text
127.0.0.1:8080
```

执行 `nginx -t` 后 reload。建议将旧 `qqbot.poke2api.com` 统一 308 重定向到 `qqbot.mcwar.cn`。

QQBot 域名只用于 Webhook、归属校验、公共绑定页和静态资源；应拒绝 `/api/v1/admin/*`。管理员从 Sub2API 正式后台进入 `/admin/qqbot`。

### 7.5 SnowLuma 接入

1. 备份 SnowLuma 的 OneBot JSON、systemd unit 和当前 Sub2API PostgreSQL/Compose。
2. 在后台以 disabled 保存 Self ID 与新生成的高强度 Token；不要在命令历史、日志或文档中输出 Token。
3. 在当前账号的 `wsClients` 增加 Universal reverse-ws：URL 为 `ws://127.0.0.1:8080/webhooks/qq/onebot`，使用相同 Token，`messageFormat=array`，`reportSelfMessage=false`。
4. 保留 SnowLuma 现有 `127.0.0.1:3010/3011` HTTP/WS 监听，不增加公网端口；重启服务并确认反向 WS 已连接。
5. 在后台执行 OneBot probe；成功后启用 Runtime，并把目标群 ID 加入 `allowed_group_ids`。
6. 需要回滚时先在 Sub2API 禁用 OneBot，再从 `wsClients` 移除 reverse-ws 并恢复 SnowLuma 配置；BotGo 链路全程保持运行。

## 8. 灰度验收

至少验证：

1. Runtime disabled/running/degraded 状态正确，启停无需重启主容器。
2. `<AppID>.json` 和 `op=13` 地址校验。
3. 非法 dispatch 签名被拒绝。
4. OneBot 无 Token、错误 Token、错误 `X-Self-ID`、代理转发请求和公网来源均被拒绝。
5. SnowLuma 反向 WS 能连接，重复连接安全替换，`get_login_info` probe 与 action/echo 超时关联正常。
6. 白名单普通群 `group_increase` 自动 @ 新成员并发送独立欢迎文案；关闭绑定或 `/check` 时不展示无效指令。
7. 测试群首次 @ 欢迎只发送一次。
8. C2C `/help`、`/bind` 和已绑定身份提示。
9. `/check` 在白名单群/频道无需绑定即可使用，C2C 只有 `active` 绑定可用；非白名单、非 active 绑定和 30 秒内重复请求均被安全拒绝。
10. `channel_check_enabled` 与 `channel_monitor_enabled` 任一关闭时 `/check` 不执行；双开后恢复。
11. BotGo 群/C2C 使用腾讯 `file_info` + `msg_type=7`，频道使用公网短期 HMAC URL；OneBot 使用 URL 图片消息段；过期或篡改 URL 被拒绝。
12. 自定义 `QQBOT_CHANNEL_CHECK_FONT_PATH` 与镜像默认 Noto CJK 字体均可正常渲染中文；渲染、上传或发送失败时回退通用文本。
13. 邮件链接、绑定完成、余额和兑换流水。
14. 同链接并发提交、BotGo/OneBot 事件重投和解绑后重绑不重复赠送。
15. 修改欢迎/帮助文案、白名单和 worker 后热更新生效。
16. 主容器重启后两套 pending Stream 事件均可恢复。
17. 日志和审计无 Token、密钥、完整邮箱、完整 QQ/OpenID、完整签名 URL、action 响应、渠道探测正文或消息正文。

切流后先停止旧容器但保留其容器、卷、镜像和部署目录，完成回滚演练后再删除。

## 9. 回滚

独立资源删除前的快速回滚：

1. 在后台禁用 SnowLuma / OneBot Runtime；BotGo 继续运行。
2. 从 SnowLuma `wsClients` 移除 Sub2API reverse-ws，恢复备份并重启 `snowluma.service`。
3. 如需回滚旧独立 BotGo 服务，再禁用内置 BotGo Runtime，并把 Nginx upstream 恢复为 `127.0.0.1:8090`。
4. 启动旧 QQBot 与专用 Redis，验证 `/healthz`、`/readyz` 和 HMAC bridge。
5. 必要时主镜像恢复到切流前版本；保留 OneBot 设置、绑定和审计数据，采用前向修复。

SQL migration 为向前兼容新增。回滚代码时保留 migration、新设置、挑战、身份、grant、余额、流水和审计，采用前向修复，禁止删除已产生的业务数据。

## 10. 最终下线

仅在单体验收与回滚演练通过后：

- 删除独立 QQBot/Redis 容器、专用网络和数据卷。
- 删除独立 QQBot 镜像、宿主机 8090 端口和活动部署目录。
- 关闭旧 HMAC bridge 并移除其环境变量。
- 轮换 QQ AppSecret/Webhook Secret，并只保存到 Sub2API 加密配置。
- 废弃独立管理员密码、session secret 和 HMAC secret。
- 加密离线回滚包按运维保留策略保存，活动服务器不保留明文 `.env`。
