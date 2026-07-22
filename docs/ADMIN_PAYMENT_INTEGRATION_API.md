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
- 多分组共享额度订阅：套餐使用 `group_ids`（兼容 `group_id`），并支持 `daily_limit_usd`、`weekly_limit_usd`、`monthly_limit_usd` 与可选 `concurrency_limit`
- 管理员订阅分配：`POST /api/v1/admin/subscriptions/assign` 可传 `plan_id`，旧 `group_id` 格式继续兼容
- 独立禁购设置：`balance_disabled` 与 `subscription_disabled`；被禁用类型的新订单会由后端拒绝
- 内置支付设置与服务商实例管理，包括 EasyPay V1 MD5 / 彩虹易支付 2.0 RSA-SHA256、`qqpay`，以及微信 JSAPI 的 `mp`/`wecom` 双 OAuth 模式

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

微信官方支付双 OAuth 模式与能力约定：
- `config.nativeEnabled`、`config.h5Enabled`、`config.jsapiEnabled` 是字符串布尔值（`"true"` / `"false"`），显式值优先。
- 历史兼容：`nativeEnabled` 缺失时为 `true`；`h5Enabled` 缺失时仅在 `h5AppName` 和 `h5AppUrl` 完整时推导为 `true`；`jsapiEnabled` 缺失时仅在 `mpAppId` 非空时推导为 `true`。
- `h5Enabled="true"` 时，`h5AppName` 必填，`h5AppUrl` 必须是绝对 HTTPS URL。
- `jsapiEnabled="true"` 时，解析后的 JSAPI AppID 必须非空；`mpAppId` 非空时优先使用，否则使用 `appId`。
- `appId` 与非空 `mpAppId` 必须以小写 `wx` 开头，后接 16 或 18 位字母数字字符；`wecom` JSAPI 使用 `ww` CorpID，并与企业微信自建应用 OAuth 凭据成套配置。启用实例保存时校验，历史实例在预下单前按模式再次校验。
- 有 OpenID 时只允许 JSAPI；普通移动端优先 H5、回退 Native；桌面端使用 Native；微信内 JSAPI 关闭时不启动 OAuth，并可回退 Native 二维码。企业微信 OAuth、身份转换、JS-SDK 或 JSAPI 失败不会自动回退 H5。
- 创建订单的结构化原因码包括 `NO_AVAILABLE_WXPAY_CAPABILITY`、`WECHAT_NATIVE_NOT_AUTHORIZED`、`WECHAT_H5_NOT_AUTHORIZED`、`WECHAT_JSAPI_NOT_AUTHORIZED`、`WECHAT_APPID_MCHID_MISMATCH`、`WECHAT_SIGN_ERROR`、`WECHAT_PAYMENT_API_ERROR`。
- API 响应继续由 `publicKeyId` 对应的 `publicKey` 验签；通知使用组合验签，既接受该微信支付公钥 ID，也接受 SDK 自动下载并维护的平台证书序列号。
- `apiV3Key` 必须与微信支付商户平台当前设置完全一致。该密钥不能通过微信 API 读取；遗失时必须在商户平台重置并同步更新实例，否则真实通知即使签名有效也无法解密。
- 微信错误 metadata 只会使用 `auth_type`、`client_environment`、`instance_id`、`mode`、`http_status`、`wechat_code`、`request_id`、`action`，不会返回凭据、请求体或其他敏感值。

| `config` 字段 | 说明 | 约束 / 默认 |
|---|---|---|
| `appId` | 基础支付 AppID；`wecom` 模式下也是 CorpID | `mp`/Native/H5 可为商户已绑定的 `wx` 或 `ww`；`wecom` JSAPI 必须为 `ww` CorpID |
| `mpAppId` | 公众号 JSAPI AppID | `mp` 模式使用，必须为 `wx`；历史 `wx` 配置可回退 `appId` |
| `jsapiAuthType` | JSAPI OAuth 身份模式 | `mp` 或 `wecom`；缺失默认 `mp` |
| `wecomAppSecret` | 企业微信自建应用 Secret | `wecom` 且 `jsapiEnabled="true"` 时必填；敏感字段，不回显 |
| `wecomAgentId` | 企业微信自建应用 AgentId | 可选；非空时必须为正整数 |
| `nativeEnabled` | Native 能力开关 | 字符串布尔值；缺失默认 `true`，不受 OAuth 模式影响 |
| `h5Enabled` | H5 能力开关 | 字符串布尔值；缺失时仅在 `h5AppName`、`h5AppUrl` 完整时推导为 `true` |
| `jsapiEnabled` | JSAPI 能力开关 | 字符串布尔值；缺失时仅 `mp` + 非空 `mpAppId` 推导为 `true` |
| `h5AppName` / `h5AppUrl` | H5 产品登记信息 | H5 开启时必填；URL 必须为绝对 HTTPS URL |

- `mp` 使用 `mpAppId` 和全局微信连接配置中的同公众号 OAuth Secret；实例解析出的公众号 AppID 必须与全局 OAuth AppID 一致。
- `wecom` 使用实例 `appId`（CorpID）、实例敏感字段 `wecomAppSecret` 与可选 `wecomAgentId`。企业微信 OAuth 固定 `snsapi_base`：内部成员按 `code -> userid -> openid` 转换，外部访问者可由 `code` 直接得到 OpenID。
- 企业微信页面先使用响应中的 `jsapi.js_config` 完成当前页面 URL 的 JS-SDK 签名配置，再通过 `WeixinJSBridge` 调起支付。`js_config` 仅在 `wecom` 模式返回；支付参数仍在 `jsapi` 对象中。
- H5 是独立产品能力，企业微信 OAuth、身份转换、JS-SDK 或 JSAPI 失败不会自动回退 H5。Native 不受 `jsapiAuthType` 影响。示例保持 `h5Enabled="false"`。
- 上线前必须配置企业微信可信域名/JS-SDK 可信域名、OAuth 网页授权回调域名、自建应用可见范围，以及微信支付商户号与公众号 AppID/CorpID 绑定、JSAPI 支付授权目录；H5 需另行开通并配置。
- Admin `GET` 不返回 `wecomAppSecret`、私钥、APIv3 密钥或支付公钥等敏感值；`PUT` 中敏感字段省略或传空字符串会保留原值。
- 实例存在 `PENDING`、`PAID` 或 `RECHARGING` 订单时，修改受保护的身份/密钥字段、禁用实例、删除实例或移除在用支付类型会返回 `PENDING_ORDERS`。

#### CreateOrder 的微信 OAuth / JSAPI 契约

认证用户通过 `POST /api/v1/payment/orders` 创建订单。请求中的 `wechat_page_url` 可选，但仅企业微信 JSAPI 使用：必须是当前站点同源的绝对 HTTPS URL，服务端会移除 fragment 后签名。其他支付模式传入该字段会返回结构化错误。

客户端**不得指定支付实例**；请求契约没有 `provider_instance_id`。服务端在 OAuth 前完成实例选择，并将用户、金额、订单类型、实例、`authType`、JSAPI AppID、套餐与页面 URL 等写入短时签名 context。OAuth 回调后的恢复令牌强制加载原实例，不会再次负载均衡。

需要 OAuth 时，`result_type` 为 `oauth_required`，并返回：

```json
{
  "result_type": "oauth_required",
  "oauth": {
    "authorize_url": "/api/v1/auth/oauth/wechat/payment/start?context_token=<signed-context-token>",
    "appid": "<wx-or-ww-app-id>",
    "scope": "snsapi_base",
    "redirect_url": "/auth/wechat/payment/callback",
    "auth_type": "<mp-or-wecom>"
  }
}
```

新客户端必须使用仅含服务端签名 `context_token` 的 `authorize_url`。旧版 query 参数式启动 URL 仅作为 legacy MP 兼容桥保留，不支持企业微信模式，也不应由新集成继续生成。

OAuth 恢复并预下单成功时，`result_type` 为 `jsapi_ready`。`jsapi.auth_type` 始终标识实际身份模式；企业微信还返回 `jsapi.js_config`：

```json
{
  "result_type": "jsapi_ready",
  "jsapi": {
    "appId": "<wx-or-ww-app-id>",
    "timeStamp": "<unix-seconds-string>",
    "nonceStr": "<payment-nonce>",
    "package": "prepay_id=<prepay-id>",
    "signType": "RSA",
    "paySign": "<payment-signature>",
    "auth_type": "<mp-or-wecom>",
    "js_config": {
      "appId": "<wecom-corp-id>",
      "timestamp": 1700000000,
      "nonceStr": "<js-sdk-nonce>",
      "signature": "<js-sdk-signature>",
      "jsApiList": ["chooseWXPay"]
    }
  }
}
```

公众号模式不返回 `js_config`。兼容字段 `jsapi_payload` 与 `jsapi` 表示同一支付参数对象；新客户端优先读取 `jsapi`。

相关结构化错误码包括：

| 类别 | 原因码 |
|---|---|
| 配置校验 | `WXPAY_CONFIG_APPID_INVALID`、`WXPAY_CONFIG_JSAPI_APPID_INVALID`、`WXPAY_CONFIG_JSAPI_AUTH_TYPE_INVALID`、`WXPAY_CONFIG_WECOM_CORPID_INVALID`、`WXPAY_CONFIG_WECOM_APP_SECRET_REQUIRED`、`WXPAY_CONFIG_WECOM_AGENT_ID_INVALID` |
| 客户端/页面 | `WECHAT_PAYMENT_CLIENT_ENVIRONMENT_INVALID`、`WECOM_PAYMENT_PAGE_URL_INVALID`、`WECOM_PAYMENT_PAGE_URL_ORIGIN_MISMATCH`、`WECOM_PAYMENT_PAGE_URL_NOT_ALLOWED` |
| OAuth/context | `WECHAT_PAYMENT_OAUTH_NOT_CONFIGURED`、`WECHAT_PAYMENT_MP_NOT_CONFIGURED`、`WECHAT_PAYMENT_MP_APP_MISMATCH`、`INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT`、`WECHAT_PAYMENT_OAUTH_CONTEXT_EXPIRED`、`INVALID_WECHAT_PAYMENT_RESUME_TOKEN` |
| 实例固定 | `WECOM_PAYMENT_INSTANCE_UNAVAILABLE`、`WECHAT_PAYMENT_INSTANCE_UNAVAILABLE`、`WECHAT_PAYMENT_INSTANCE_CHANGED`、`PENDING_ORDERS` |
| 微信支付 API | `NO_AVAILABLE_WXPAY_CAPABILITY`、`WECHAT_NATIVE_NOT_AUTHORIZED`、`WECHAT_H5_NOT_AUTHORIZED`、`WECHAT_JSAPI_NOT_AUTHORIZED`、`WECHAT_APPID_MCHID_MISMATCH`、`WECHAT_SIGN_ERROR`、`WECHAT_PAYMENT_API_ERROR` |

错误 metadata 只包含必要的 `auth_type`、`client_environment`、`instance_id`、`mode`、`http_status`、`wechat_code`、`request_id` 或 `action`，不返回凭据、请求体或上游敏感响应。

企业微信模式配置示例（全部凭据均为占位符）：
```json
{
  "provider_key": "wxpay",
  "name": "<provider-display-name>",
  "config": {
    "appId": "<wecom-corp-id>",
    "mchId": "<merchant-id>",
    "privateKey": "<merchant-private-key-pem>",
    "apiV3Key": "<32-byte-api-v3-key>",
    "publicKey": "<wechat-pay-public-key-pem>",
    "publicKeyId": "<wechat-pay-public-key-id>",
    "certSerial": "<merchant-certificate-serial>",
    "nativeEnabled": "true",
    "h5Enabled": "false",
    "jsapiEnabled": "true",
    "jsapiAuthType": "wecom",
    "wecomAppSecret": "<wecom-custom-app-secret>",
    "wecomAgentId": "<positive-agent-id>"
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
- `subscription`：`group_ids` 必须且只能包含一个 `subscription` 分组；套餐级 `daily_limit_usd`、`weekly_limit_usd`、`monthly_limit_usd` 与 `concurrency_limit` 会被清空，用户订阅继续读取分组自身限额。
- `standard_quota`：`group_ids` 可包含一个或多个 `standard`（余额）分组；至少设置一个正数套餐限额，所有分组共享同一订阅实例、用量和可选并发池。

`concurrency_limit` 仅接受 `1`–`2147483647` 的正整数或 `null`。`null` / 省略表示套餐不额外限制并发；更新套餐时传 `concurrency_limit_set: true` 可把 `concurrency_limit: null` 解释为显式清除，而不是“未修改”。`quota_limits_set` 对日/周/月额度保持相同的显式更新语义。

运行时达到订阅实例并发上限时返回 HTTP `429`，并设置 `Retry-After: 1` 与 `X-Sub2API-Error-Code: SUBSCRIPTION_CONCURRENCY_LIMIT_EXCEEDED`；OpenAI 兼容错误体使用 `rate_limit_error`。Responses WebSocket 空闲连接不占套餐槽位，每个 `response.create` turn 独立抢占；槽位不足时以 `1013 Try Again Later` 关闭，连接内切换模型会以 `1008 Policy Violation` 拒绝，客户端需为新模型重新连接。由于 Batch Image 当前只支持余额预冻结/结算，携带有效 `standard_quota` 订阅的 `POST /v1/images/batches` 会 fail-closed 返回 HTTP `409` 和 `BATCH_IMAGE_SUBSCRIPTION_UNSUPPORTED`，不会退回余额计费。

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
  "concurrency_limit": 4,
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
  "concurrency_limit": null,
  "description": "使用订阅分组自身限额",
  "price": 9.9,
  "validity_days": 30,
  "validity_unit": "days",
  "features": "原生限额",
  "for_sale": true,
  "sort_order": 0
}
```

运行时规则：标准分组存在有效 `standard_quota` 订阅时优先消耗套餐额度且不扣余额；套餐额度耗尽会直接拒绝请求，不会静默改扣余额。正整数 `concurrency_limit` 按 `user_subscriptions.id` 订阅实例计数，同一实例的多个标准分组共享并发池，并与用户全局并发、上游账号并发同时生效；`null` 表示不增加套餐层限制。支付下单、管理员分配和用户订阅响应都会携带并发快照，套餐后续编辑不追溯已有实例；订阅列表/详情必须读取实例的 `concurrency_limit`，不能用当前套餐值替代。旧订单或旧订阅缺失字段时按 `null` 兼容。套餐过期后，公开标准分组恢复余额计费。旧版多订阅分组套餐会标记为 `legacy_shared_subscription` 并下架，已有订阅和已创建订单继续按快照履约。

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

`summary` 额外支持 `group_page`、`group_page_size`。`totals.paid_order_count` 统计存在 `paid_at` 的订单数，`totals.paid_amounts` 按支付币种返回 `{ currency, order_count, amount }`，金额来自 `pay_amount`，包含余额充值和订阅订单，后续退款不回冲，便于与支付渠道账单核对。余额金额字段使用固定两位小数字符串：`gross_recharge_amount`、`refunded_amount`、`net_recharge_amount`；余额到账仅统计已完成到账的余额充值订单，退款只在部分/全额退款最终完成后扣减。

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
- Built-in payment settings and provider instance management, including EasyPay V1 MD5 / Rainbow EasyPay 2.0 RSA-SHA256, `qqpay`, and `mp`/`wecom` dual OAuth modes for WeChat JSAPI

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

Direct WeChat Pay dual-OAuth modes and capability contract:
- `config.nativeEnabled`, `config.h5Enabled`, and `config.jsapiEnabled` are string booleans (`"true"` / `"false"`); explicit values take precedence.
- Historical compatibility: absent `nativeEnabled` means `true`; absent `h5Enabled` is inferred as `true` only when both `h5AppName` and `h5AppUrl` are complete; absent `jsapiEnabled` is inferred as `true` only when `mpAppId` is non-empty.
- When `h5Enabled="true"`, `h5AppName` is required and `h5AppUrl` must be an absolute HTTPS URL.
- When `jsapiEnabled="true"`, the resolved JSAPI AppID must be non-empty. A non-empty `mpAppId` takes precedence; otherwise `appId` is used.
- `appId` and any non-empty `mpAppId` must start with lowercase `wx` and contain 16 or 18 following ASCII alphanumeric characters; WeCom JSAPI uses a `ww` CorpID together with matching custom-app OAuth credentials. Enabled instances are checked when saved, while historical instances are checked again for the selected mode before prepay.
- An OpenID permits JSAPI only; ordinary mobile browsers prefer H5 and fall back to Native; desktop uses Native. If JSAPI is disabled, an in-WeChat request does not start OAuth and may fall back to a Native QR code. WeCom OAuth, identity conversion, JS-SDK, or JSAPI failures never automatically fall back to H5.
- Structured order-creation reasons include `NO_AVAILABLE_WXPAY_CAPABILITY`, `WECHAT_NATIVE_NOT_AUTHORIZED`, `WECHAT_H5_NOT_AUTHORIZED`, `WECHAT_JSAPI_NOT_AUTHORIZED`, `WECHAT_APPID_MCHID_MISMATCH`, `WECHAT_SIGN_ERROR`, and `WECHAT_PAYMENT_API_ERROR`.
- API responses continue to be verified with the `publicKey` identified by `publicKeyId`. Notifications use combined verification, accepting either that WeChat Pay public-key ID or a platform-certificate serial backed by certificates automatically downloaded and maintained by the SDK.
- `apiV3Key` must exactly match the current WeChat Pay Merchant Platform setting. It cannot be read through WeChat APIs; if lost, reset it in Merchant Platform and update the provider instance, otherwise validly signed real notifications cannot be decrypted.
- WeChat error metadata is limited to `auth_type`, `client_environment`, `instance_id`, `mode`, `http_status`, `wechat_code`, `request_id`, and `action`; credentials, request bodies, and other sensitive values are never returned.

| `config` field | Description | Constraint / default |
|---|---|---|
| `appId` | Base payment AppID; also the CorpID in `wecom` mode | Native/H5/`mp` may use a merchant-bound `wx` or `ww`; WeCom JSAPI requires a `ww` CorpID |
| `mpAppId` | Official Account JSAPI AppID | Used by `mp`, must start with `wx`; legacy `wx` configs may fall back to `appId` |
| `jsapiAuthType` | JSAPI OAuth identity mode | `mp` or `wecom`; defaults to `mp` when absent |
| `wecomAppSecret` | Secret of the WeCom custom app | Required when WeCom JSAPI is enabled; sensitive and never echoed |
| `wecomAgentId` | AgentId of the WeCom custom app | Optional; when non-empty it must be a positive integer |
| `nativeEnabled` | Native capability switch | String boolean; defaults to `true`; unaffected by OAuth mode |
| `h5Enabled` | H5 capability switch | String boolean; inferred only when both H5 fields are complete |
| `jsapiEnabled` | JSAPI capability switch | String boolean; absent is inferred only for `mp` with non-empty `mpAppId` |
| `h5AppName` / `h5AppUrl` | H5 product registration | Required when H5 is enabled; URL must be absolute HTTPS |

- `mp` uses `mpAppId` and the global WeChat Connect configuration's OAuth Secret for the same Official Account. The resolved instance MP AppID must match the global OAuth AppID.
- `wecom` uses instance `appId` (CorpID), sensitive instance field `wecomAppSecret`, and optional `wecomAgentId`. OAuth is fixed to `snsapi_base`: internal members follow `code -> userid -> openid`, while an external visitor may return OpenID directly.
- A WeCom page first applies `jsapi.js_config`, signed for the current page URL, and then invokes payment through `WeixinJSBridge`. `js_config` is returned only for `wecom`; payment invocation fields remain in `jsapi`.
- H5 is an independent product capability. WeCom OAuth, identity conversion, JS-SDK, or JSAPI failures never automatically fall back to H5. Native is unaffected by `jsapiAuthType`. Examples keep `h5Enabled="false"`.
- Before rollout, configure the WeCom trusted/JS-SDK domain, OAuth web authorization callback domain, custom-app visibility scope, WeChat Pay merchant AppID/CorpID association, and JSAPI payment authorization directory. H5 requires separate product enablement and registration.
- Admin `GET` responses omit `wecomAppSecret`, private keys, APIv3 keys, payment public keys, and other secrets. Omitting a sensitive field or submitting an empty string in `PUT` preserves its stored value.
- While an instance has `PENDING`, `PAID`, or `RECHARGING` orders, protected identity/key changes, disabling, deletion, and removal of an in-use payment type return `PENDING_ORDERS`.

#### CreateOrder WeChat OAuth / JSAPI contract

Authenticated users create orders with `POST /api/v1/payment/orders`. The optional `wechat_page_url` is accepted only for WeCom JSAPI. It must be an absolute same-origin HTTPS URL; the server removes its fragment before signing. Supplying it for another mode returns a structured error.

Clients **must not select a provider instance**; the request contract has no `provider_instance_id`. The server selects an instance before OAuth and puts the user, amount, order type, instance, `authType`, JSAPI AppID, plan, and page URL into a short-lived signed context. The post-callback resume token must load that exact instance and is never load-balanced again.

When OAuth is required, `result_type` is `oauth_required`:

```json
{
  "result_type": "oauth_required",
  "oauth": {
    "authorize_url": "/api/v1/auth/oauth/wechat/payment/start?context_token=<signed-context-token>",
    "appid": "<wx-or-ww-app-id>",
    "scope": "snsapi_base",
    "redirect_url": "/auth/wechat/payment/callback",
    "auth_type": "<mp-or-wecom>"
  }
}
```

New clients must use the `authorize_url` containing only the server-signed `context_token`. The old query-parameter start URL remains only as a legacy MP compatibility bridge; it does not support WeCom and should not be generated by new integrations.

After OAuth resume and successful prepay, `result_type` is `jsapi_ready`. `jsapi.auth_type` always identifies the actual identity mode, and WeCom also returns `jsapi.js_config`:

```json
{
  "result_type": "jsapi_ready",
  "jsapi": {
    "appId": "<wx-or-ww-app-id>",
    "timeStamp": "<unix-seconds-string>",
    "nonceStr": "<payment-nonce>",
    "package": "prepay_id=<prepay-id>",
    "signType": "RSA",
    "paySign": "<payment-signature>",
    "auth_type": "<mp-or-wecom>",
    "js_config": {
      "appId": "<wecom-corp-id>",
      "timestamp": 1700000000,
      "nonceStr": "<js-sdk-nonce>",
      "signature": "<js-sdk-signature>",
      "jsApiList": ["chooseWXPay"]
    }
  }
}
```

Official Account mode omits `js_config`. The compatibility field `jsapi_payload` represents the same payment object as `jsapi`; new clients should prefer `jsapi`.

Relevant structured reasons include:

| Category | Reasons |
|---|---|
| Configuration | `WXPAY_CONFIG_APPID_INVALID`, `WXPAY_CONFIG_JSAPI_APPID_INVALID`, `WXPAY_CONFIG_JSAPI_AUTH_TYPE_INVALID`, `WXPAY_CONFIG_WECOM_CORPID_INVALID`, `WXPAY_CONFIG_WECOM_APP_SECRET_REQUIRED`, `WXPAY_CONFIG_WECOM_AGENT_ID_INVALID` |
| Client/page | `WECHAT_PAYMENT_CLIENT_ENVIRONMENT_INVALID`, `WECOM_PAYMENT_PAGE_URL_INVALID`, `WECOM_PAYMENT_PAGE_URL_ORIGIN_MISMATCH`, `WECOM_PAYMENT_PAGE_URL_NOT_ALLOWED` |
| OAuth/context | `WECHAT_PAYMENT_OAUTH_NOT_CONFIGURED`, `WECHAT_PAYMENT_MP_NOT_CONFIGURED`, `WECHAT_PAYMENT_MP_APP_MISMATCH`, `INVALID_WECHAT_PAYMENT_OAUTH_CONTEXT`, `WECHAT_PAYMENT_OAUTH_CONTEXT_EXPIRED`, `INVALID_WECHAT_PAYMENT_RESUME_TOKEN` |
| Instance binding | `WECOM_PAYMENT_INSTANCE_UNAVAILABLE`, `WECHAT_PAYMENT_INSTANCE_UNAVAILABLE`, `WECHAT_PAYMENT_INSTANCE_CHANGED`, `PENDING_ORDERS` |
| WeChat Pay API | `NO_AVAILABLE_WXPAY_CAPABILITY`, `WECHAT_NATIVE_NOT_AUTHORIZED`, `WECHAT_H5_NOT_AUTHORIZED`, `WECHAT_JSAPI_NOT_AUTHORIZED`, `WECHAT_APPID_MCHID_MISMATCH`, `WECHAT_SIGN_ERROR`, `WECHAT_PAYMENT_API_ERROR` |

Error metadata is limited to necessary `auth_type`, `client_environment`, `instance_id`, `mode`, `http_status`, `wechat_code`, `request_id`, or `action` fields. Credentials, request bodies, and sensitive upstream responses are not returned.

WeCom-mode configuration example using placeholders only:
```json
{
  "provider_key": "wxpay",
  "name": "<provider-display-name>",
  "config": {
    "appId": "<wecom-corp-id>",
    "mchId": "<merchant-id>",
    "privateKey": "<merchant-private-key-pem>",
    "apiV3Key": "<32-byte-api-v3-key>",
    "publicKey": "<wechat-pay-public-key-pem>",
    "publicKeyId": "<wechat-pay-public-key-id>",
    "certSerial": "<merchant-certificate-serial>",
    "nativeEnabled": "true",
    "h5Enabled": "false",
    "jsapiEnabled": "true",
    "jsapiAuthType": "wecom",
    "wecomAppSecret": "<wecom-custom-app-secret>",
    "wecomAgentId": "<positive-agent-id>"
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
- `subscription`: `group_ids` must contain exactly one `subscription` group. Plan-level daily, weekly, monthly, and concurrency limits are cleared, and subscriptions use the group's native limits.
- `standard_quota`: `group_ids` may contain one or more `standard` balance groups. At least one positive plan limit is required, and all included groups share one subscription instance, usage counter, and optional concurrency pool.

`concurrency_limit` accepts an integer from `1` through `2147483647`, or `null`, only. `null` / omission means the plan adds no concurrency restriction. On plan updates, send `concurrency_limit_set: true` with `concurrency_limit: null` to explicitly clear the value instead of leaving it unchanged. `quota_limits_set` keeps the same explicit-update behavior for daily, weekly, and monthly limits.

At runtime, a saturated subscription-instance pool returns HTTP `429` with `Retry-After: 1` and `X-Sub2API-Error-Code: SUBSCRIPTION_CONCURRENCY_LIMIT_EXCEEDED`; OpenAI-compatible error bodies use `rate_limit_error`. Idle Responses WebSocket connections do not hold a plan slot: each `response.create` turn acquires its own slot. Capacity rejection closes with `1013 Try Again Later`, while switching models on an existing connection is rejected with `1008 Policy Violation`; reconnect for the new model. Because Batch Image currently supports balance hold/settlement only, `POST /v1/images/batches` with an active `standard_quota` subscription fails closed with HTTP `409` and `BATCH_IMAGE_SUBSCRIPTION_UNSUPPORTED` instead of falling back to balance billing.

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
  "concurrency_limit": 4,
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
  "concurrency_limit": null,
  "description": "Uses the subscription group's own limits",
  "price": 9.9,
  "validity_days": 30,
  "validity_unit": "days",
  "features": "Native limits",
  "for_sale": true,
  "sort_order": 0
}
```

Runtime behavior: while an active `standard_quota` subscription covers a standard group, plan quota takes priority and the user's balance is not charged. Exhausted plan quota rejects the request instead of silently falling back to balance. A positive `concurrency_limit` is counted by `user_subscriptions.id`; all standard groups in the same instance share the pool, and user-global plus upstream-account concurrency limits still apply independently. `null` adds no plan-level restriction. Payment creation, admin assignment, and user-subscription responses carry the concurrency snapshot. Later plan edits are not retroactive, so subscription lists/details must read the instance `concurrency_limit` instead of the current plan value. Missing fields in old orders or subscriptions are treated as `null`. After expiration, public standard groups return to balance billing. Legacy multi-subscription-group plans are marked `legacy_shared_subscription` and taken off sale, while existing subscriptions and already-created orders continue from their snapshots.

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

`summary` additionally accepts `group_page` and `group_page_size`. `totals.paid_order_count` counts orders with `paid_at`, while `totals.paid_amounts` returns `{ currency, order_count, amount }` grouped by payment currency and summed from `pay_amount`. It includes balance top-ups and subscriptions and does not reverse later refunds, which makes it suitable for reconciling gateway statements. Balance totals remain fixed-two-decimal strings in `gross_recharge_amount`, `refunded_amount`, and `net_recharge_amount`; they include only fulfilled balance top-ups, and refunds are deducted only after a partial or full refund is finalized.

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
