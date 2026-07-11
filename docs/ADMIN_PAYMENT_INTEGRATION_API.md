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

### 基础地址
- 生产：`https://<your-domain>`
- Beta：`http://<your-server-ip>:8084`

### 认证
推荐使用：
- `x-api-key: admin-<64hex>`
- `Content-Type: application/json`
- 幂等接口额外传：`Idempotency-Key`

说明：管理员 JWT 也可访问 admin 路由，但服务间调用建议使用 Admin API Key。

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
| `payment_type` | 支付方式 |
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

### Base URL
- Production: `https://<your-domain>`
- Beta: `http://<your-server-ip>:8084`

### Authentication
Recommended headers:
- `x-api-key: admin-<64hex>`
- `Content-Type: application/json`
- `Idempotency-Key` for idempotent endpoints

Note: Admin JWT can also access admin routes, but Admin API Key is recommended for server-to-server integration.

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
| `payment_type` | Payment method |
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
