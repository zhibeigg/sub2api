# ADMIN_PAYMENT_INTEGRATION_API

> 单文件中英双语文档 / Single-file bilingual documentation (Chinese + English)

---

## 中文

### 目标
本文档用于对接外部支付系统（如 `sub2apipay`）与 Sub2API 的 Admin API，覆盖：
- 支付成功后充值
- 用户查询
- 人工余额修正
- 前端购买页参数透传
- 多分组共享额度订阅：套餐使用 `group_ids`（兼容 `group_id`），并支持 `daily_limit_usd`、`weekly_limit_usd`、`monthly_limit_usd`
- 管理员订阅分配：`POST /api/v1/admin/subscriptions/assign` 可传 `plan_id`，旧 `group_id` 格式继续兼容
- 独立禁购设置：`balance_disabled` 与 `subscription_disabled`；被禁用类型的新订单会由后端拒绝
- 内置支付设置与服务商实例管理，包括 EasyPay V1 MD5 / 彩虹易支付 2.0 RSA-SHA256 和 `qqpay`

### 基础地址
- 生产：`https://<your-domain>`
- Beta：`http://<your-server-ip>:8084`

### 认证
推荐使用：
- `x-api-key: admin-<64hex>`
- `Content-Type: application/json`
- 幂等接口额外传：`Idempotency-Key`

说明：管理员 JWT 也可访问 admin 路由，但服务间调用建议使用 Admin API Key。

### 内置支付设置与服务商实例 API

支付全局配置：
- `GET /api/v1/admin/payment/config`
- `PUT /api/v1/admin/payment/config`

前台统一展示支付宝、微信支付、QQ 支付；以下字段控制可见性与来源：

| 字段 | 说明 |
|---|---|
| `payment_visible_method_alipay_enabled` | 是否展示支付宝 |
| `payment_visible_method_alipay_source` | `official_alipay` 或 `easypay_alipay` |
| `payment_visible_method_wxpay_enabled` | 是否展示微信支付 |
| `payment_visible_method_wxpay_source` | `official_wxpay` 或 `easypay_wxpay` |
| `payment_visible_method_qqpay_enabled` | 是否展示 QQ 支付 |
| `payment_visible_method_qqpay_source` | `easypay_qqpay`；上游未开通 QQ 通道时应关闭 |

服务商实例 CRUD：
- `GET /api/v1/admin/payment/providers`
- `POST /api/v1/admin/payment/providers`
- `PUT /api/v1/admin/payment/providers/:id`
- `DELETE /api/v1/admin/payment/providers/:id`

创建和更新请求中的 `config` 是服务商专用字符串键值对象；实例还使用 `provider_key`、`name`、`supported_types`、`enabled`、`payment_mode`、`sort_order`、`limits`、`refund_enabled`、`allow_user_refund`。EasyPay 的 `provider_key` 始终为 `easypay`，不要为 V2 创建新的 provider key。

EasyPay 协议约定：
- `config.protocolVersion="1"` 使用 V1 MD5；历史 `config` 缺少该字段时也按 V1。
- `config.protocolVersion="2"` 使用彩虹易支付 2.0 RSA-SHA256，配置 `pid`、`apiBase`、`merchantPrivateKey`、`platformPublicKey`、`notifyUrl`、`returnUrl`。
- V1 的 `supported_types` 支持 `alipay`、`wxpay`；V2 支持 `alipay`、`wxpay`、`qqpay`，QQ 支付是否可用取决于上游通道。
- V1/V2 共用 `/api/v1/payment/webhook/easypay`。V2 创建阶段按官方 SDK 规则在本地签名并生成固定路径 `/api/pay/submit` 的托管收银台 URL，不调用或信任可能返回未签名字段的 `/api/pay/create`；初始 `payment_trade_no` 允许为空。
- V2 查单、退款和退款查询继续调用 `/api/pay/query`、`/api/pay/refund`、`/api/pay/refundquery`，允许 300 秒时钟偏差；这些响应及回调仍需完成 RSA、PID、时间戳、金额和订单号校验，字段缺失或不一致时安全失败。
- 同一退款重试必须复用稳定的 `out_refund_no`。
- 协议版本创建后不可切换；协议升级、PID 变更或密钥轮换必须新建实例，不要直接覆盖仍有关联订单的实例配置。
- `GET` 响应不会回传私钥等敏感配置；`PUT` 可只提交需要变更的 `config` 字段，未提交的敏感字段保持原值。不要把真实凭证写入文档、日志或示例。

微信官方支付能力约定：
- `config.nativeEnabled`、`config.h5Enabled`、`config.jsapiEnabled` 是字符串布尔值（`"true"` / `"false"`），显式值优先。
- 历史兼容：`nativeEnabled` 缺失时为 `true`；`h5Enabled` 缺失时仅在 `h5AppName` 和 `h5AppUrl` 完整时推导为 `true`；`jsapiEnabled` 缺失时仅在 `mpAppId` 非空时推导为 `true`。
- `h5Enabled="true"` 时，`h5AppName` 必填，`h5AppUrl` 必须是绝对 HTTPS URL。
- `jsapiEnabled="true"` 时，解析后的 JSAPI AppID 必须非空；`mpAppId` 非空时优先使用，否则使用 `appId`。
- 有 OpenID 时只允许 JSAPI；普通移动端优先 H5、回退 Native；桌面端使用 Native；微信内 JSAPI 关闭时不启动 OAuth，并可回退 Native 二维码。未启用的模式不会调用微信 API。
- 创建订单的结构化原因码包括 `NO_AVAILABLE_WXPAY_CAPABILITY`、`WECHAT_NATIVE_NOT_AUTHORIZED`、`WECHAT_H5_NOT_AUTHORIZED`、`WECHAT_JSAPI_NOT_AUTHORIZED`、`WECHAT_APPID_MCHID_MISMATCH`、`WECHAT_SIGN_ERROR`、`WECHAT_PAYMENT_API_ERROR`。
- 微信错误 metadata 只会使用 `mode`、`http_status`、`wechat_code`、`request_id`、`action`，不会返回凭据、请求体或其他敏感值。

微信能力配置示例（凭据仅为占位符）：
```json
{
  "provider_key": "wxpay",
  "name": "WeChat Pay",
  "config": {
    "appId": "<wechat-app-id>",
    "mchId": "<merchant-id>",
    "privateKey": "<merchant-private-key-pem>",
    "apiV3Key": "<32-byte-api-v3-key>",
    "publicKey": "<wechat-pay-public-key-pem>",
    "publicKeyId": "<wechat-pay-public-key-id>",
    "certSerial": "<merchant-certificate-serial>",
    "nativeEnabled": "true",
    "h5Enabled": "false",
    "jsapiEnabled": "false"
  },
  "supported_types": ["wxpay"],
  "enabled": true,
  "payment_mode": "qrcode"
}
```

V2 请求结构示例（仅占位符，不是可用密钥）：
```json
{
  "provider_key": "easypay",
  "name": "EasyPay V2",
  "config": {
    "protocolVersion": "2",
    "pid": "<merchant-pid>",
    "apiBase": "https://pay.example.com",
    "merchantPrivateKey": "<merchant-private-key>",
    "platformPublicKey": "<platform-public-key>",
    "notifyUrl": "https://sub2api.example.com/api/v1/payment/webhook/easypay",
    "returnUrl": "https://sub2api.example.com/payment/result"
  },
  "supported_types": ["alipay", "wxpay", "qqpay"],
  "enabled": true,
  "payment_mode": "qrcode",
  "sort_order": 0,
  "limits": "{}",
  "refund_enabled": true,
  "allow_user_refund": false
}
```

### 订阅套餐双类型

套餐接口：
- `GET /api/v1/admin/payment/plans`
- `POST /api/v1/admin/payment/plans`
- `PUT /api/v1/admin/payment/plans/:id`

`plan_type` 支持：
- `subscription`：`group_ids` 必须且只能包含一个 `subscription` 分组；套餐级 `daily_limit_usd`、`weekly_limit_usd`、`monthly_limit_usd` 会被清空，用户订阅继续读取分组自身限额。
- `standard_quota`：`group_ids` 可包含一个或多个 `standard`（余额）分组；至少设置一个正数套餐限额，所有分组共享同一订阅实例和用量。

标准共享额度套餐示例：
```json
{
  "plan_type": "standard_quota",
  "name": "多模型共享额度",
  "group_id": 11,
  "group_ids": [11, 12],
  "daily_limit_usd": 5,
  "weekly_limit_usd": 25,
  "monthly_limit_usd": 80,
  "description": "标准分组共享套餐额度",
  "price": 19.9,
  "validity_days": 30,
  "validity_unit": "days",
  "features": "共享额度",
  "for_sale": true,
  "sort_order": 0
}
```

原生订阅分组套餐示例：
```json
{
  "plan_type": "subscription",
  "name": "原生订阅套餐",
  "group_id": 21,
  "group_ids": [21],
  "daily_limit_usd": null,
  "weekly_limit_usd": null,
  "monthly_limit_usd": null,
  "description": "使用订阅分组自身限额",
  "price": 9.9,
  "validity_days": 30,
  "validity_unit": "days",
  "features": "原生限额",
  "for_sale": true,
  "sort_order": 0
}
```

运行时规则：标准分组存在有效 `standard_quota` 订阅时优先消耗套餐额度且不扣余额；套餐额度耗尽会直接拒绝请求，不会静默改扣余额；套餐过期后，公开标准分组恢复余额计费。旧版多订阅分组套餐会标记为 `legacy_shared_subscription` 并下架，已有订阅和已创建订单继续按快照履约。

### 管理员订单报表 API

以下接口使用相同的筛选参数：

- `GET /api/v1/admin/payment/orders`：分页订单明细；
- `GET /api/v1/admin/payment/orders/summary`：充值汇总和注册优惠码归因分组；
- `GET /api/v1/admin/payment/orders/promo-code-options`：当前与历史优惠码筛选选项；
- `GET /api/v1/admin/payment/orders/export?mode=orders|attribution`：按当前筛选导出 CSV，不受列表分页限制。

通用 Query 参数：

| 参数 | 说明 |
|---|---|
| `page` / `page_size` | 仅列表分页 |
| `user_id` | 正整数用户 ID |
| `status` | 订单状态 |
| `order_type` | `balance` 或 `subscription` |
| `payment_type` | 支付方式：`alipay`、`wxpay`、`qqpay` 等 |
| `keyword` | 订单号、用户邮箱、用户名或注册优惠码，最多 100 字符 |
| `promo_code_id` | 指定注册优惠码 ID |
| `promo_attribution` | `all`、`attributed`、`none`、`legacy_unknown` |
| `start_date` / `end_date` | `YYYY-MM-DD`，结束日期按包含当天处理 |
| `timezone` | IANA 时区，例如 `Asia/Shanghai` |
| `time_field` | `created_at`（默认）或 `paid_at` |

`summary` 额外支持 `group_page`、`group_page_size`。其金额字段使用固定两位小数字符串：`gross_recharge_amount`、`refunded_amount`、`net_recharge_amount`。充值仅统计已完成到账的余额充值订单；退款只在部分/全额退款最终完成后扣减。

订单明细新增稳定归因字段：

```json
{
  "signup_promo_attribution": "attributed",
  "signup_promo_code_id": 42,
  "signup_promo_code": "WELCOME25",
  "recharge_base_amount": 100,
  "recharge_bonus_multiplier": 1.25,
  "first_recharge_bonus_applied": true,
  "net_recharge_amount": 125
}
```

`none` 表示订单创建时明确为自然注册；`legacy_unknown` 表示历史数据无法可靠恢复，不会被错误归入自然注册。订单明细导出最多 100,000 行，超限返回 `EXPORT_LIMIT_EXCEEDED`；CSV 使用 UTF-8 BOM，并防护 Excel 公式注入。

### 1) 一步完成创建并兑换
`POST /api/v1/admin/redeem-codes/create-and-redeem`

用途：原子完成“创建兑换码 + 兑换到指定用户”。

> 此 Admin API 属于外部系统人工入账，不参与内置支付订单的优惠码首充加成判定，也不会自动应用 `recharge_bonus_multiplier`。首充优惠仅由内置余额充值订单在支付到账时原子判定。

请求头：
- `x-api-key`
- `Idempotency-Key`

请求体示例：
```json
{
  "code": "s2p_cm1234567890",
  "type": "balance",
  "value": 100.0,
  "user_id": 123,
  "notes": "sub2apipay order: cm1234567890"
}
```

幂等语义：
- 同 `code` 且 `used_by` 一致：`200`
- 同 `code` 但 `used_by` 不一致：`409`
- 缺少 `Idempotency-Key`：`400`（`IDEMPOTENCY_KEY_REQUIRED`）

curl 示例：
```bash
curl -X POST "${BASE}/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: pay-cm1234567890-success" \
  -H "Content-Type: application/json" \
  -d '{
    "code":"s2p_cm1234567890",
    "type":"balance",
    "value":100.00,
    "user_id":123,
    "notes":"sub2apipay order: cm1234567890"
  }'
```

### 2) 查询用户（可选前置校验）
`GET /api/v1/admin/users/:id`

```bash
curl -s "${BASE}/api/v1/admin/users/123" \
  -H "x-api-key: ${KEY}"
```

### 3) 余额调整（已有接口）
`POST /api/v1/admin/users/:id/balance`

用途：人工补偿 / 扣减，支持 `set` / `add` / `subtract`。

请求体示例（扣减）：
```json
{
  "balance": 100.0,
  "operation": "subtract",
  "notes": "manual correction"
}
```

```bash
curl -X POST "${BASE}/api/v1/admin/users/123/balance" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: balance-subtract-cm1234567890" \
  -H "Content-Type: application/json" \
  -d '{
    "balance":100.00,
    "operation":"subtract",
    "notes":"manual correction"
  }'
```

### 4) 购买页 / 自定义页面 URL Query 透传（iframe / 新窗口一致）
当 Sub2API 打开 `purchase_subscription_url` 或用户侧自定义页面 iframe URL 时，会统一追加：
- `user_id`
- `token`
- `theme`（`light` / `dark`）
- `lang`（例如 `zh` / `en`，用于向嵌入页传递当前界面语言）
- `ui_mode`（固定 `embedded`）

示例：
```text
https://pay.example.com/pay?user_id=123&token=<jwt>&theme=light&lang=zh&ui_mode=embedded
```

### 5) 失败处理建议
- 支付成功与充值成功分状态落库
- 回调验签成功后立即标记“支付成功”
- 支付成功但充值失败的订单允许后续重试
- 重试保持相同 `code`，并使用新的 `Idempotency-Key`

### 6) `doc_url` 配置建议
- 查看链接：`https://github.com/Wei-Shaw/sub2api/blob/main/ADMIN_PAYMENT_INTEGRATION_API.md`
- 下载链接：`https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/ADMIN_PAYMENT_INTEGRATION_API.md`

---

## English

### Purpose
This document describes the minimal Sub2API Admin API surface for external payment integrations (for example, `sub2apipay`), including:
- Recharge after payment success
- User lookup
- Manual balance correction
- Purchase page query parameter forwarding
- Built-in payment settings and provider instance management, including EasyPay V1 MD5 / Rainbow EasyPay 2.0 RSA-SHA256 and `qqpay`

### Base URL
- Production: `https://<your-domain>`
- Beta: `http://<your-server-ip>:8084`

### Authentication
Recommended headers:
- `x-api-key: admin-<64hex>`
- `Content-Type: application/json`
- `Idempotency-Key` for idempotent endpoints

Note: Admin JWT can also access admin routes, but Admin API Key is recommended for server-to-server integration.

### Built-in payment settings and provider APIs

Global payment configuration:
- `GET /api/v1/admin/payment/config`
- `PUT /api/v1/admin/payment/config`

The frontend exposes unified Alipay, WeChat Pay, and QQ Pay methods. These fields control visibility and routing:

| Field | Description |
|---|---|
| `payment_visible_method_alipay_enabled` | Show Alipay |
| `payment_visible_method_alipay_source` | `official_alipay` or `easypay_alipay` |
| `payment_visible_method_wxpay_enabled` | Show WeChat Pay |
| `payment_visible_method_wxpay_source` | `official_wxpay` or `easypay_wxpay` |
| `payment_visible_method_qqpay_enabled` | Show QQ Pay |
| `payment_visible_method_qqpay_source` | `easypay_qqpay`; keep disabled unless the upstream QQ channel is enabled |

Provider instance CRUD:
- `GET /api/v1/admin/payment/providers`
- `POST /api/v1/admin/payment/providers`
- `PUT /api/v1/admin/payment/providers/:id`
- `DELETE /api/v1/admin/payment/providers/:id`

The `config` member in create/update requests is a provider-specific string map. Instances also use `provider_key`, `name`, `supported_types`, `enabled`, `payment_mode`, `sort_order`, `limits`, `refund_enabled`, and `allow_user_refund`. EasyPay always uses `provider_key: "easypay"`; do not introduce a separate provider key for V2.

EasyPay protocol contract:
- `config.protocolVersion="1"` selects V1 MD5. A historical config without this field is also treated as V1.
- `config.protocolVersion="2"` selects Rainbow EasyPay 2.0 RSA-SHA256 with `pid`, `apiBase`, `merchantPrivateKey`, `platformPublicKey`, `notifyUrl`, and `returnUrl`.
- V1 `supported_types` supports `alipay` and `wxpay`; V2 supports `alipay`, `wxpay`, and `qqpay`. QQ Pay availability depends on the upstream channel.
- V1 and V2 share `/api/v1/payment/webhook/easypay`. V2 creation follows the official SDK flow: it signs locally and builds a hosted checkout URL at the fixed `/api/pay/submit` path instead of calling or trusting potentially unsigned `/api/pay/create` result fields. The initial `payment_trade_no` may be empty.
- V2 query, refund, and refund-query continue to call `/api/pay/query`, `/api/pay/refund`, and `/api/pay/refundquery`, with 300 seconds of allowed clock skew. These responses and callbacks still require RSA, PID, timestamp, amount, and order-number validation; missing or inconsistent fields fail safely.
- Retries of the same refund must reuse a stable `out_refund_no`.
- The protocol version is immutable after creation. Use a new instance for protocol upgrades, PID changes, or key rotation instead of overwriting an instance that still has associated orders.
- `GET` responses omit private keys and other sensitive config. A `PUT` may submit only changed `config` fields; omitted sensitive fields retain their stored values. Never place real credentials in documentation, logs, or examples.

Direct WeChat Pay capability contract:
- `config.nativeEnabled`, `config.h5Enabled`, and `config.jsapiEnabled` are string booleans (`"true"` / `"false"`); explicit values take precedence.
- Historical compatibility: absent `nativeEnabled` means `true`; absent `h5Enabled` is inferred as `true` only when both `h5AppName` and `h5AppUrl` are complete; absent `jsapiEnabled` is inferred as `true` only when `mpAppId` is non-empty.
- When `h5Enabled="true"`, `h5AppName` is required and `h5AppUrl` must be an absolute HTTPS URL.
- When `jsapiEnabled="true"`, the resolved JSAPI AppID must be non-empty. A non-empty `mpAppId` takes precedence; otherwise `appId` is used.
- An OpenID permits JSAPI only; ordinary mobile browsers prefer H5 and fall back to Native; desktop uses Native. If JSAPI is disabled, an in-WeChat request does not start OAuth and may fall back to a Native QR code. Disabled modes never call their WeChat APIs.
- Structured order-creation reasons include `NO_AVAILABLE_WXPAY_CAPABILITY`, `WECHAT_NATIVE_NOT_AUTHORIZED`, `WECHAT_H5_NOT_AUTHORIZED`, `WECHAT_JSAPI_NOT_AUTHORIZED`, `WECHAT_APPID_MCHID_MISMATCH`, `WECHAT_SIGN_ERROR`, and `WECHAT_PAYMENT_API_ERROR`.
- WeChat error metadata uses only `mode`, `http_status`, `wechat_code`, `request_id`, and `action`; credentials, request bodies, and other sensitive values are never returned.

WeChat capability request example using placeholders only:
```json
{
  "provider_key": "wxpay",
  "name": "WeChat Pay",
  "config": {
    "appId": "<wechat-app-id>",
    "mchId": "<merchant-id>",
    "privateKey": "<merchant-private-key-pem>",
    "apiV3Key": "<32-byte-api-v3-key>",
    "publicKey": "<wechat-pay-public-key-pem>",
    "publicKeyId": "<wechat-pay-public-key-id>",
    "certSerial": "<merchant-certificate-serial>",
    "nativeEnabled": "true",
    "h5Enabled": "false",
    "jsapiEnabled": "false"
  },
  "supported_types": ["wxpay"],
  "enabled": true,
  "payment_mode": "qrcode"
}
```

V2 request shape using placeholders only:
```json
{
  "provider_key": "easypay",
  "name": "EasyPay V2",
  "config": {
    "protocolVersion": "2",
    "pid": "<merchant-pid>",
    "apiBase": "https://pay.example.com",
    "merchantPrivateKey": "<merchant-private-key>",
    "platformPublicKey": "<platform-public-key>",
    "notifyUrl": "https://sub2api.example.com/api/v1/payment/webhook/easypay",
    "returnUrl": "https://sub2api.example.com/payment/result"
  },
  "supported_types": ["alipay", "wxpay", "qqpay"],
  "enabled": true,
  "payment_mode": "qrcode",
  "sort_order": 0,
  "limits": "{}",
  "refund_enabled": true,
  "allow_user_refund": false
}
```

### Dual subscription plan types

Plan endpoints:
- `GET /api/v1/admin/payment/plans`
- `POST /api/v1/admin/payment/plans`
- `PUT /api/v1/admin/payment/plans/:id`

Supported `plan_type` values:
- `subscription`: `group_ids` must contain exactly one `subscription` group. Plan-level daily, weekly, and monthly limits are cleared, and subscriptions use the group's native limits.
- `standard_quota`: `group_ids` may contain one or more `standard` balance groups. At least one positive plan limit is required, and all included groups share one subscription instance and usage counter.

Standard shared-quota example:
```json
{
  "plan_type": "standard_quota",
  "name": "Multi-model shared quota",
  "group_id": 11,
  "group_ids": [11, 12],
  "daily_limit_usd": 5,
  "weekly_limit_usd": 25,
  "monthly_limit_usd": 80,
  "description": "Shared quota across standard groups",
  "price": 19.9,
  "validity_days": 30,
  "validity_unit": "days",
  "features": "Shared quota",
  "for_sale": true,
  "sort_order": 0
}
```

Native subscription-group example:
```json
{
  "plan_type": "subscription",
  "name": "Native subscription plan",
  "group_id": 21,
  "group_ids": [21],
  "daily_limit_usd": null,
  "weekly_limit_usd": null,
  "monthly_limit_usd": null,
  "description": "Uses the subscription group's own limits",
  "price": 9.9,
  "validity_days": 30,
  "validity_unit": "days",
  "features": "Native limits",
  "for_sale": true,
  "sort_order": 0
}
```

Runtime behavior: while an active `standard_quota` subscription covers a standard group, plan quota takes priority and the user's balance is not charged. Exhausted plan quota rejects the request instead of silently falling back to balance. After expiration, public standard groups return to balance billing. Legacy multi-subscription-group plans are marked `legacy_shared_subscription` and taken off sale, while existing subscriptions and already-created orders continue from their snapshots.

### Admin order reporting APIs

The following endpoints share the same filter contract:

- `GET /api/v1/admin/payment/orders`: paginated order details;
- `GET /api/v1/admin/payment/orders/summary`: recharge totals and registration-promo attribution groups;
- `GET /api/v1/admin/payment/orders/promo-code-options`: current and historical promo filter options;
- `GET /api/v1/admin/payment/orders/export?mode=orders|attribution`: CSV export for all matching rows, independent of list pagination.

Common query parameters:

| Parameter | Description |
|---|---|
| `page` / `page_size` | List pagination only |
| `user_id` | Positive user ID |
| `status` | Order status |
| `order_type` | `balance` or `subscription` |
| `payment_type` | Payment method, including `alipay`, `wxpay`, and `qqpay` |
| `keyword` | Order number, user email/name, or registration promo code; max 100 characters |
| `promo_code_id` | Exact registration promo ID |
| `promo_attribution` | `all`, `attributed`, `none`, or `legacy_unknown` |
| `start_date` / `end_date` | `YYYY-MM-DD`; the end calendar day is inclusive |
| `timezone` | IANA timezone such as `Asia/Shanghai` |
| `time_field` | `created_at` (default) or `paid_at` |

`summary` additionally accepts `group_page` and `group_page_size`. Monetary totals are fixed-two-decimal strings: `gross_recharge_amount`, `refunded_amount`, and `net_recharge_amount`. Recharge totals include only fulfilled balance-recharge orders, and refunds are deducted only after a partial or full refund is finalized.

Order detail responses include stable attribution fields:

```json
{
  "signup_promo_attribution": "attributed",
  "signup_promo_code_id": 42,
  "signup_promo_code": "WELCOME25",
  "recharge_base_amount": 100,
  "recharge_bonus_multiplier": 1.25,
  "first_recharge_bonus_applied": true,
  "net_recharge_amount": 125
}
```

`none` means the order was created for a confirmed organic registration. `legacy_unknown` means historical attribution cannot be restored reliably and is not misclassified as organic. Order-detail exports are limited to 100,000 rows; larger exports return `EXPORT_LIMIT_EXCEEDED`. CSV responses include a UTF-8 BOM and spreadsheet-formula injection protection.

### 1) Create and Redeem in one step
`POST /api/v1/admin/redeem-codes/create-and-redeem`

Use case: atomically create a redeem code and redeem it to a target user.

> This Admin API is an external/manual credit path. It does not participate in the built-in payment order’s first-top-up promo decision and does not automatically apply `recharge_bonus_multiplier`. The first-top-up bonus is determined atomically only when a built-in balance payment order is fulfilled.

Headers:
- `x-api-key`
- `Idempotency-Key`

Request body:
```json
{
  "code": "s2p_cm1234567890",
  "type": "balance",
  "value": 100.0,
  "user_id": 123,
  "notes": "sub2apipay order: cm1234567890"
}
```

Idempotency behavior:
- Same `code` and same `used_by`: `200`
- Same `code` but different `used_by`: `409`
- Missing `Idempotency-Key`: `400` (`IDEMPOTENCY_KEY_REQUIRED`)

curl example:
```bash
curl -X POST "${BASE}/api/v1/admin/redeem-codes/create-and-redeem" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: pay-cm1234567890-success" \
  -H "Content-Type: application/json" \
  -d '{
    "code":"s2p_cm1234567890",
    "type":"balance",
    "value":100.00,
    "user_id":123,
    "notes":"sub2apipay order: cm1234567890"
  }'
```

### 2) Query User (optional pre-check)
`GET /api/v1/admin/users/:id`

```bash
curl -s "${BASE}/api/v1/admin/users/123" \
  -H "x-api-key: ${KEY}"
```

### 3) Balance Adjustment (existing API)
`POST /api/v1/admin/users/:id/balance`

Use case: manual correction with `set` / `add` / `subtract`.

Request body example (`subtract`):
```json
{
  "balance": 100.0,
  "operation": "subtract",
  "notes": "manual correction"
}
```

```bash
curl -X POST "${BASE}/api/v1/admin/users/123/balance" \
  -H "x-api-key: ${KEY}" \
  -H "Idempotency-Key: balance-subtract-cm1234567890" \
  -H "Content-Type: application/json" \
  -d '{
    "balance":100.00,
    "operation":"subtract",
    "notes":"manual correction"
  }'
```

### 4) Purchase / Custom Page URL query forwarding (iframe and new tab)
When Sub2API opens `purchase_subscription_url` or a user-facing custom page iframe URL, it appends:
- `user_id`
- `token`
- `theme` (`light` / `dark`)
- `lang` (for example `zh` / `en`, used to pass the current UI language to the embedded page)
- `ui_mode` (fixed: `embedded`)

Example:
```text
https://pay.example.com/pay?user_id=123&token=<jwt>&theme=light&lang=zh&ui_mode=embedded
```

### 5) Failure handling recommendations
- Persist payment success and recharge success as separate states
- Mark payment as successful immediately after verified callback
- Allow retry for orders with payment success but recharge failure
- Keep the same `code` for retry, and use a new `Idempotency-Key`

### 6) Recommended `doc_url`
- View URL: `https://github.com/Wei-Shaw/sub2api/blob/main/ADMIN_PAYMENT_INTEGRATION_API.md`
- Download URL: `https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/ADMIN_PAYMENT_INTEGRATION_API.md`
