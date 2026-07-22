# API Key 用量与余额接口

`GET /v1/usage` 为 API Key 返回当前用量、配额、订阅或钱包余额。该接口可供 CC Switch 及其他受信客户端展示余额，也作为 Sub2API 池模式账户验证兼容上游真实余额的契约。池账户探测与 CC Switch 导入脚本使用相同的请求方法；URL 白名单开启时遵循 `upstream_hosts`，关闭时沿用普通账户连通性测试的 URL 格式校验，而不会直接禁用余额查询。

## 请求

```http
GET /v1/usage
Authorization: Bearer <API_KEY>
Accept: application/json
```

响应始终包含：

```json
{
  "object": "sub2api.key_usage",
  "schema_version": 1,
  "mode": "quota_limited",
  "isValid": true
}
```

成功和错误响应均设置：

```http
Cache-Control: no-store
```

客户端不得把该接口的认证头转发给重定向目标，也不应持久化 API Key 或完整响应正文。

## `quota_limited`

当 API Key 配置了总配额或速率限制时返回此模式。配置总配额时，`remaining` 与 `quota.remaining` 都表示 Key 的剩余美元额度：

```json
{
  "object": "sub2api.key_usage",
  "schema_version": 1,
  "mode": "quota_limited",
  "isValid": true,
  "status": "active",
  "remaining": 40,
  "unit": "USD",
  "quota": {
    "limit": 100,
    "used": 60,
    "remaining": 40,
    "unit": "USD"
  }
}
```

`quota.used` 在并发结算后可能略高于 `quota.limit`；此时 `quota.remaining` 为 `0`。

如果 Key 只配置了请求速率限制，响应可能只有 `rate_limits`，不一定具有可归一化的货币余额。

## `unrestricted`

### 订阅额度

存在有效订阅时，响应包含日、周、月用量和限额。顶层 `remaining` 是所有已配置有限窗口中最小的剩余额度；没有任何有限窗口时为 `-1`。

```json
{
  "object": "sub2api.key_usage",
  "schema_version": 1,
  "mode": "unrestricted",
  "isValid": true,
  "planName": "Team",
  "remaining": 5,
  "unit": "USD",
  "subscription": {
    "daily_usage_usd": 2,
    "daily_limit_usd": 10,
    "weekly_usage_usd": 5,
    "weekly_limit_usd": 10,
    "monthly_usage_usd": 20,
    "monthly_limit_usd": 100,
    "weekly_window_start": "2026-07-20T00:00:00Z",
    "expires_at": "2026-08-01T00:00:00Z"
  }
}
```

### 钱包余额

无 Key 配额和有效订阅时返回用户钱包余额：

```json
{
  "object": "sub2api.key_usage",
  "schema_version": 1,
  "mode": "unrestricted",
  "isValid": true,
  "planName": "钱包余额",
  "remaining": 42.5,
  "balance": 42.5,
  "unit": "USD"
}
```

## 兼容性与校验建议

新客户端应优先要求：

- `object` 必须为 `sub2api.key_usage`；
- `schema_version` 必须为 `1`；
- `mode` 必须为 `quota_limited` 或 `unrestricted`；
- `isValid` 必须存在。

为了兼容旧版 Sub2API，可在缺少 `object/schema_version` 时接受完整的 `quota`、`subscription` 或 `balance + remaining` 结构，但不能仅凭顶层 `remaining` 判断这是余额接口。

解析时应拒绝负数（订阅无限状态的顶层 `remaining=-1` 除外）、NaN/Inf、多个连续 JSON 值，以及顶层与嵌套剩余额度不一致的响应。

## 管理端容量状态

管理员账户列表的“用量窗口”列使用统一 `capacity` 结构：

- `verified`：本次已验证的上游真实余额；
- `stale`：最近成功快照，当前查询失败，仅供展示；
- `estimated`：根据非池官方窗口或本地额度估算的剩余请求数；
- `unsupported`：例如原生 AWS Bedrock SigV4 没有通用余额端点；
- `unknown`：无法验证，不等于剩余为 `0`；
- `unlimited`：上游或本地配置明确无限。

池容量告警只消费 `verified` 或明确 `unlimited` 的权威上游快照；`stale`、`unknown`、`unsupported` 不会创建新告警，也不会把已有低容量状态误恢复为健康。
