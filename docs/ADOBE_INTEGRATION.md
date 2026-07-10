# Adobe IMS / Firefly 平台接入

Sub2API 从 `0.31.5` 起将 Adobe 作为独立平台接入。平台真实值固定为 `adobe`，用于承载 Adobe IMS 凭据续期、Firefly 图片生成和异步视频任务。

Adobe 分组只会调度 Adobe 账号，不参与 OpenAI、Anthropic、Gemini、Grok、Kiro 或 Antigravity 的跨平台混合调度。

## 能力范围

- Adobe IMS access token、cookie 或 device token 凭据录入。
- access token 到期前自动续期；device token 优先，cookie 作为回退来源。
- Adobe profile 与 Firefly credits 查询。
- OpenAI Images 兼容的同步图片生成与图片编辑。
- OpenAI 风格的异步视频提交与状态查询。
- 图片按张、视频按秒的严格媒体定价。
- Redis 视频任务快照、任务归属校验与成功轮询幂等结算。
- Adobe 平台配额、Usage、Ops、错误透传、账号测试和管理端筛选。
- 复用现有内容审核、用户并发、账号并发和账号切换上限；参数/内容错误不会触发盲目换号。

本期不支持 Adobe 文本补全、Responses、Chat Completions、Embeddings、WebSocket、Gemini 原生协议或 Batch Image。

## 账号凭据

后台进入“账户管理 → 新增账户”，选择 Adobe。Adobe 账号复用现有 `oauth` 类型，但创建流程为单步凭据录入，不会进入浏览器 OAuth 第二步。

至少提供以下一种可用来源：

1. `device_token` 与 `device_id` 成对提供，推荐用于自动续期；
2. `cookie`，用于续期回退；
3. 尚未过期的 `access_token`，可用于短期运行。

`password` 只作为恢复元数据保存，不能单独构成可用登录来源。

通用管理 API：

```http
POST /api/v1/admin/accounts
Content-Type: application/json
Authorization: Bearer <admin-token>
```

```json
{
  "name": "Adobe Firefly Account",
  "platform": "adobe",
  "type": "oauth",
  "concurrency": 1,
  "credentials": {
    "access_token": "<optional-current-token>",
    "expires_at": 1760000000,
    "device_token": "<optional-device-token>",
    "device_id": "<required-with-device-token>",
    "cookie": "<optional-cookie>",
    "password": "<optional-recovery-metadata>"
  },
  "group_ids": [1]
}
```

### 凭据安全与编辑语义

服务端不会在列表、详情、导出或普通 API 响应中返回 Adobe 明文凭据。响应只通过 `credentials_status.has_access_token`、`has_cookie`、`has_device_token`、`has_device_id` 和 `has_password` 等布尔状态说明字段是否存在。

编辑敏感字段采用三态语义：

- 保留：不发送该字段；
- 替换：在 `credentials` 中发送非空新值；
- 清除：在 `clear_credentials` 中发送字段名。

同一字段不能同时替换和清除。`device_token` 与 `device_id` 必须保持成对状态。

```http
PUT /api/v1/admin/accounts/123
Content-Type: application/json
Authorization: Bearer <admin-token>
```

```json
{
  "credentials": {
    "cookie": "<replacement-cookie>"
  },
  "clear_credentials": ["access_token"]
}
```

可以使用以下通用端点执行账号测试、手动续期和 credits 查询：

| 方法 | 路径 | 用途 |
|------|------|------|
| POST | `/api/v1/admin/accounts/:id/test` | 测试凭据与 Firefly 可用性 |
| POST | `/api/v1/admin/accounts/:id/refresh` | 强制刷新 access token、profile 与 credits |
| GET | `/api/v1/admin/accounts/:id/usage` | 查看 credits、检查时间与本地用量 |

credits 的 `unknown`、`0` 和正数是三个不同状态；`0` 不会被当作未知值。

## 分组与定价

创建 `platform = adobe` 的分组，并将 Adobe 账号加入该分组。默认账号并发为 `1`，可继续使用代理、优先级、模型白名单、模型映射、调度和限流能力。

Adobe 媒体请求采用严格定价：

- 图片：`1K`、`2K`、`4K`，按生成张数计费；
- 视频：`720p`、`1080p`，按成功输出的时长秒数计费。

价格字段缺失与价格为 `0` 含义不同：

- 缺失：该档位不可用，请求会在上传或提交到 Adobe 前被拒绝；
- `0`：明确免费，允许请求并记录零费用 Usage。

如果配置了与 Adobe 分组同平台的 Channel 媒体价格，匹配档位时优先使用 Channel 快照；否则使用 Group 快照。Channel 定价不完整时不会静默回退到默认价格。

## 公开模型

`GET /v1/models` 在 Adobe 分组下返回 Firefly 模型目录。公开模型包括：

### 图片

- `nano-banana-pro`
- `nano-banana-v2`
- `nano-banana`

### 视频

- `veo3`
- `veo3.1`
- `sora`
- `sora-2-pro`

服务端还保留仅供账号显式 `model_mapping` 使用的隐藏解析别名，例如 `gpt-image-2`、`veo3.1-fast`、`veo3.1-lite` 与兼容的 Sora/Veo 内部别名；隐藏别名不会出现在公开模型列表中，也不会进入 Adobe 账号的默认映射。

## 图片生成

Adobe 图片能力复用现有 OpenAI Images 路由，不新增 `/adobe/v1/...` 前缀。

### 文生图

```bash
curl https://your-domain/v1/images/generations \
  -H "Authorization: Bearer <sub2api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nano-banana-pro",
    "prompt": "A restrained editorial product photo on a dark desk",
    "size": "2048x2048",
    "n": 1,
    "response_format": "url"
  }'
```

`response_format` 支持 `url` 与 `b64_json`。Adobe 当前只接受 `n = 1`，不支持流式图片响应。

### 图片编辑

```bash
curl https://your-domain/v1/images/edits \
  -H "Authorization: Bearer <sub2api-key>" \
  -F "model=nano-banana-v2" \
  -F "prompt=Turn the background into a quiet concrete studio" \
  -F "image=@reference.png" \
  -F "size=2048x2048" \
  -F "n=1"
```

参考图必须通过 MIME、大小和安全校验。当前不支持 mask，非 PNG 或不受支持的字段会返回参数错误。

图片请求在提交前冻结价格快照，生成成功后通过统一事务账务入口结算，并记录图片档位、来源和模型归因。

## 视频生成

### 提交任务

```bash
curl https://your-domain/v1/videos/generations \
  -H "Authorization: Bearer <sub2api-key>" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "veo3.1",
    "prompt": "A slow dolly shot through a precise industrial studio",
    "resolution": "1080p",
    "duration": 8,
    "aspect_ratio": "16:9",
    "generate_audio": true
  }'
```

提交成功后返回客户端 `request_id`。服务端只有在将不可变任务快照完整写入 Redis 后才会返回成功。Redis 写入失败时固定返回 `503`，不会结算、不会写成功 Usage，并会记录包含 Adobe task ID 和安全诊断信息的高优先级 orphan Ops 事件。

### 查询状态

```bash
curl https://your-domain/v1/videos/<request_id> \
  -H "Authorization: Bearer <sub2api-key>"
```

状态查询会固定使用提交时选择的账号和 Adobe 完整轮询 URL，不重新调度。请求者必须与任务快照中的用户和分组归属一致。

- 处理中：返回当前状态，不扣费；
- 失败或取消：返回终态，不扣费；
- 成功：先执行事务结算，再返回输出 URL；
- 临时结算失败：返回 `503`，保留可重试状态；
- 余额或配额不足：返回 `402`，不暴露输出 URL；
- 已成功结算：后续轮询返回缓存结果，不重复扣费。

Adobe 原始 task ID 作为 Usage 与账务去重键，保证成功任务只结算一次。

## 配置

`deploy/config.example.yaml` 包含 Adobe 默认配置：

```yaml
adobe:
  request_timeout_seconds: 120
  image_poll_interval_seconds: 2
  image_max_poll_attempts: 150
  video_task_ttl_seconds: 259200
  video_terminal_ttl_seconds: 86400
  token_refresh_skew_seconds: 300
  credits_cache_ttl_seconds: 300
```

说明：

- `request_timeout_seconds`：Adobe IMS / Firefly 单次请求超时；
- `image_poll_interval_seconds`：同步图片任务轮询间隔；
- `image_max_poll_attempts`：同步图片最大轮询次数；
- `video_task_ttl_seconds`：活动视频任务 Redis TTL，默认 72 小时；
- `video_terminal_ttl_seconds`：视频终态缓存 TTL，默认 24 小时；
- `token_refresh_skew_seconds`：access token 提前续期窗口，默认 5 分钟；
- `credits_cache_ttl_seconds`：profile / credits 快照缓存时间。

账号级代理仍通过现有 Proxy 管理功能配置，不在 Adobe 段保存代理凭据。

## 错误与故障切换

- Firefly `401`/`403` 会触发一次强制 token 刷新，并仅重试一次；
- 鉴权错误、`429` 和上游 `5xx` 可切换到同一 Adobe 分组内的其他 Adobe 账号；
- 参数错误、内容错误与业务拒绝不会盲目切换账号；
- Adobe 错误透传规则使用真实平台值 `adobe`；
- 日志、Ops、错误响应和导出不会包含 token、cookie、device token、password 或完整敏感请求体。

## 运维检查清单

1. Adobe 账号至少存在一种可用凭据来源；
2. device token 与 device ID 成对；
3. Adobe 分组只绑定 Adobe 账号；
4. 所需图片或视频档位已显式配置价格；
5. Redis 可用且视频任务 TTL 符合最长任务时长；
6. 代理出口可以访问 Adobe IMS 与 Firefly；
7. Usage、Ops 和 Error 页面使用 `adobe` 平台筛选核对请求；
8. 定期验证 credits 为未知、零或正数时的告警策略符合预期。

## 免责声明

本功能仅用于合法的开发、测试和运营场景，与 Adobe 无关联或背书关系。使用者必须自行遵守 Adobe 服务条款、所在地区法律法规和账号授权边界。