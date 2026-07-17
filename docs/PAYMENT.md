# Payment System Configuration Guide

Sub2API has a built-in payment system that enables user self-service top-up without deploying a separate payment service.

Subscription plans have two modes. A `subscription` plan binds exactly one subscription group and uses that group's native limits. A `standard_quota` plan binds one or more standard balance groups and defines shared daily, weekly, or monthly USD plan limits. Plan type, group access, limits, and validity are snapshotted when purchased or assigned, so later plan edits do not retroactively change existing subscriptions. Administrators can also hide balance recharge and subscription purchase independently; the backend rejects new orders of the disabled type.

### Subscription plan types

| `plan_type` | Eligible groups | Count | Limit source | After expiration |
|---|---|---:|---|---|
| `subscription` | Subscription groups | Exactly 1 | The group's native daily/weekly/monthly limits | The subscription group is no longer available |
| `standard_quota` | Standard balance groups | One or more | Shared plan-level limits; at least one period is required | Public standard groups return to balance billing; exclusive groups require separate access |

An active `standard_quota` subscription takes priority over balance billing. Exhausted plan quota rejects the request instead of silently charging balance. Legacy multi-subscription-group plans are retained as read-only `legacy_shared_subscription` plans and taken off sale; existing subscriptions and order snapshots remain valid.

---

## Table of Contents

- [Supported Payment Methods](#supported-payment-methods)
- [Quick Start](#quick-start)
- [System Settings](#system-settings)
- [Provider Configuration](#provider-configuration)
- [Provider Instance Management](#provider-instance-management)
- [Webhook Configuration](#webhook-configuration)
- [Payment Flow](#payment-flow)
- [Migrating from Sub2ApiPay](#migrating-from-sub2apipay)

---

## Supported Payment Methods

| Provider | Payment Methods | Description |
|----------|----------------|-------------|
| **EasyPay** | Alipay, WeChat Pay, QQ Pay | One `easypay` provider supports EasyPay V1 MD5 and Rainbow EasyPay 2.0 RSA-SHA256 through `protocolVersion=1/2`; QQ Pay availability depends on the upstream channel |
| **Alipay (Direct)** | Desktop QR code, mobile Alipay redirect | Direct integration with Alipay Open Platform, returning desktop QR codes and mobile WAP/app launch links |
| **WeChat Pay (Direct)** | Native QR, H5, Official Account/WeCom JSAPI | Direct WeChat Pay APIv3 integration with environment- and OAuth-aware routing |
| **Stripe** | Card, Alipay, WeChat Pay, Link, etc. | International payments, multi-currency support |

> Direct Alipay/WeChat Pay and EasyPay can coexist as backend provider instances, while the frontend exposes three unified methods: `Alipay`, `WeChat Pay`, and `QQ Pay`. Administrators independently control visibility and choose one source for each method: Alipay and WeChat Pay may use direct or EasyPay sources, while QQ Pay uses EasyPay. Actual QQ Pay availability still depends on the upstream merchant channel being enabled.

> **EasyPay Provider Recommendations**: Both options below are third-party aggregators compatible with the EasyPay protocol. Pick based on the funding channel and settlement currency you need:
>
> - **Domestic channel / CNY settlement** — [ZPay](https://z-pay.cn/?uid=23808) (`https://z-pay.cn/?uid=23808`): direct integration with official Alipay / WeChat Pay APIs, fee **1.6%**; funds go straight to the merchant account with **T+1 automatic settlement**. Supports **individual users** (no business license required) with up to 10,000 CNY daily transactions; business-licensed accounts have no limit. Link contains the referral code of [Sub2ApiPay](https://github.com/touwaeriol/sub2apipay) original author [@touwaeriol](https://github.com/touwaeriol) — feel free to remove it.
> - **International channel / USDT or USD settlement** — [Kyren Topup](https://kyren.top/?code=SUB2API) (`https://kyren.top/?code=SUB2API`): a ready-to-launch global payment stack for AI startups with WeChat Pay and Alipay support, local-currency checkout, and USD settlement. Fees: WeChat 2%, Alipay 2.5%; withdrawal 0.1% (min $40, max $150), settled in **USDT or USD**. No qualification review required — sign up and use immediately, making it the lowest barrier to entry. Withdrawal threshold is relatively high, recommended for users **who do not use domestic Chinese payment channels, cannot tolerate Stripe's 6%+ fees, have high transaction volume, and have USD or USDT channels to receive withdrawn funds**. Kyren Topup charges a $200 account opening fee; signing up via this link (which contains Sub2Api author [@Wei-Shaw](https://github.com/Wei-Shaw)'s referral code) **waives the opening fee**. Feel free to remove it if you prefer.
>
> Please evaluate the security, reliability, and compliance of any third-party payment provider on your own — this project does not endorse or guarantee any of them.

---

## Quick Start

1. Go to Admin Dashboard → **Settings** → **Payment Settings** tab
2. Enable **Payment**
3. Configure basic parameters (amount range, timeout, etc.)
4. Add at least one provider instance in **Provider Management**
5. Users can now top up from the frontend; once payment is enabled, a top-up shortcut appears next to the header balance and in the mobile user menu

---

## System Settings

Configure the following in Admin Dashboard **Settings → Payment Settings**:

### Basic Settings

| Setting | Description | Default |
|---------|-------------|---------|
| **Enable Payment** | Enable or disable the payment system | Off |
| **Product Name Prefix** | Prefix shown on payment page | - |
| **Product Name Suffix** | Suffix (e.g., "Credits") | - |
| **Minimum Amount** | Minimum single top-up amount | 1 |
| **Maximum Amount** | Maximum single top-up amount (empty = unlimited) | - |
| **Daily Limit** | Per-user daily cumulative limit (empty = unlimited) | - |
| **Order Timeout** | Order timeout in minutes (minimum 1) | 30 |
| **Max Pending Orders** | Maximum concurrent pending orders per user | 3 |
| **Load Balance Strategy** | Strategy for selecting provider instances | Round Robin |

### Frontend Visible Method Routing

The frontend exposes three unified methods without showing backend provider brands directly:

- **Alipay**: `payment_visible_method_alipay_enabled` controls visibility, and `payment_visible_method_alipay_source` accepts `official_alipay` or `easypay_alipay`
- **WeChat Pay**: `payment_visible_method_wxpay_enabled` controls visibility, and `payment_visible_method_wxpay_source` accepts `official_wxpay` or `easypay_wxpay`
- **QQ Pay**: `payment_visible_method_qqpay_enabled` controls visibility, and `payment_visible_method_qqpay_source` uses `easypay_qqpay`; leave it disabled unless the upstream merchant has enabled QQ Pay
- Each visible method can route to only one source. A method is hidden when no source or usable provider instance is available

### Load Balance Strategies

| Strategy | Description |
|----------|-------------|
| **Round Robin** | Distribute orders to instances in rotation |
| **Least Amount** | Prefer instances with the lowest daily cumulative amount |

### Cancel Rate Limiting

Prevents users from repeatedly creating and canceling orders:

| Setting | Description |
|---------|-------------|
| **Enable Limit** | Toggle |
| **Window Mode** | Sliding / Fixed window |
| **Time Window** | Window duration |
| **Window Unit** | Minutes / Hours |
| **Max Cancels** | Maximum cancellations allowed within the window |

### Help Information

| Setting | Description |
|---------|-------------|
| **Help Image** | Customer service QR code or help image (supports upload) |
| **Help Text** | Instructions displayed on the payment page |

---

## Provider Configuration

Each provider type requires different credentials. Select the type when adding a new provider instance in **Provider Management → Add Provider**.

> **Callback URLs are auto-generated**: When adding a provider, the Notify URL and Return URL are automatically constructed from your site domain. You only need to confirm the domain is correct.

### EasyPay

A single `easypay` provider type selects the protocol through `protocolVersion`:

| `protocolVersion` | Protocol | Signature |
|---|---|---|
| `1` | Traditional EasyPay V1 | MD5 |
| `2` | Rainbow EasyPay 2.0 | RSA-SHA256 |

Existing instances without `protocolVersion` are treated as V1 for backward compatibility, while new instances default to V2. The protocol version cannot be changed after creation. For a protocol upgrade, PID change, or key rotation, create a new provider instance and move traffic to it instead of overwriting an instance that still owns historical orders.

**V1 configuration** keeps the existing `pid`, `pkey`, `apiBase`, `notifyUrl`, `returnUrl`, and optional channel fields.

**V2 configuration:**

| Config key | Description | Required |
|---|---|---|
| `protocolVersion` | Must be `2` | Yes |
| `pid` | Rainbow EasyPay 2.0 merchant PID | Yes |
| `apiBase` | Upstream site origin without a path, query, or user info | Yes |
| `merchantPrivateKey` | Merchant RSA private key; store only the real production credential | Yes |
| `platformPublicKey` | Platform RSA public key used for verification | Yes |
| `notifyUrl` | Asynchronous notification URL using the Sub2API EasyPay webhook | Yes |
| `returnUrl` | Synchronous browser return URL | Yes |

EasyPay V1 supports `alipay` and `wxpay`; V2 supports `alipay`, `wxpay`, and `qqpay`. QQ Pay works only when enabled for the upstream merchant. Never copy keys from a local SDK, test project, or documentation sample.

### Alipay (Direct)

Direct integration with Alipay Open Platform. Mobile flows return an Alipay WAP/app redirect URL. Desktop flows prefer Face-to-Face Precreate QR payloads; if the merchant has not enabled that product, the provider falls back to Computer Website Pay and also returns the cashier URL so the frontend can render a QR code or open the hosted checkout page directly.

| Parameter | Description | Required |
|-----------|-------------|----------|
| **AppID** | Alipay application AppID | Yes |
| **Private Key** | RSA2 application private key | Yes |
| **Alipay Public Key** | Alipay public key | Yes |

### WeChat Pay (Direct)

Direct integration with WeChat Pay APIv3. Supports Native QR code payment, H5 payment, and JSAPI payment in Official Account or WeCom environments.

| Parameter | Description | Required |
|-----------|-------------|----------|
| **AppID** | WeChat AppID (`wx`) or WeCom CorpID (`ww`) associated in Merchant Platform | Yes |
| **Merchant ID (MchID)** | WeChat Pay merchant ID | Yes |
| **Merchant API Private Key** | Merchant API private key (PEM format) | Yes |
| **APIv3 Key** | 32-byte APIv3 key | Yes |
| **WeChat Pay Public Key** | WeChat Pay public key (PEM format) | Yes |
| **WeChat Pay Public Key ID** | WeChat Pay public key ID | Yes |
| **Certificate Serial Number** | Merchant certificate serial number | Yes |

The `APIv3 Key` must exactly match the key currently configured in WeChat Pay Merchant Platform. It cannot be retrieved through WeChat APIs or management endpoints; if it is lost, reset it in Merchant Platform and immediately update the provider instance. A mismatch allows signature verification to succeed but prevents decryption of real payment notifications.

API responses continue to use the configured WeChat Pay public key. Payment notifications use combined verification, accepting either the WeChat Pay public-key ID or a platform-certificate serial backed by certificates automatically downloaded and maintained by the SDK. Both paths remain strictly verified; never work around configuration problems by disabling signature or timestamp checks.

JSAPI payment supports two OAuth identity modes selected by `jsapiAuthType=mp|wecom`; a missing value defaults to `mp` for backward compatibility. The base payment `appId` may still be a Merchant-Platform-associated WeChat AppID (lowercase `wx`) or WeCom CorpID (lowercase `ww`), but the JSAPI identity, OAuth credential, and AppID must match the selected mode as one configuration set.

| Config key | Description | Default / compatibility behavior |
|------------|-------------|----------------------------------|
| `nativeEnabled` | Allow Native QR payment | Defaults to `true` when absent; unaffected by OAuth mode |
| `h5Enabled` | Allow H5 payment | When absent, inferred as `true` only if both `h5AppName` and `h5AppUrl` are complete |
| `jsapiEnabled` | Allow JSAPI payment | When absent, inferred as `true` only for `mp` with a non-empty `mpAppId`; WeCom should set it explicitly |
| `jsapiAuthType` | JSAPI OAuth mode: `mp` or `wecom` | Defaults to `mp` when absent |
| `h5AppName` | H5 application name registered in WeChat Pay Merchant Platform | Required when `h5Enabled=true` |
| `h5AppUrl` | H5 application site URL | Must be an absolute HTTPS URL when `h5Enabled=true` |
| `mpAppId` | Official Account AppID (`wx` prefix) | Used by `mp`; legacy `wx` configs may fall back to the base `appId` |
| `wecomAppSecret` | Secret of the WeCom custom app | Required when WeCom JSAPI is enabled; sensitive |
| `wecomAgentId` | AgentId of the WeCom custom app | Optional; when present it must be a positive integer |

- **Official Account mode (`mp`)** uses `mpAppId` (`wx`) and the global WeChat Connect/OAuth Secret for that same Official Account. The payment instance AppID must match the global MP OAuth AppID. Legacy `wx` instances may use the base `appId` fallback, but new configurations should set `mpAppId` explicitly.
- **WeCom mode (`wecom`)** requires the instance `appId` to be the WeCom CorpID (`ww`) and uses `wecomAppSecret` from a custom app under that CorpID; `wecomAgentId` is optional. OAuth always uses `snsapi_base`. For internal members, the returned `userid` is converted server-side to the OpenID required by payment; an external visitor's directly returned OpenID is used as-is.

Before OAuth starts, the server selects the payment instance and issues a short-lived signed `context_token`. The context binds the user, amount, order type, provider instance, `authType`, JSAPI AppID, and necessary plan/page details. The callback resume token is forced back to that exact instance; if it is disabled, removed, or materially reconfigured, the request fails safely instead of being load-balanced elsewhere. Clients cannot select a provider instance.

For WeCom pages, the frontend first applies the server-generated `js_config`, signed for the current same-origin HTTPS page URL, and then still invokes payment through `WeixinJSBridge`. The JS-SDK configuration validates and authorizes page capabilities; it does not replace the payment bridge. The URL fragment is removed before signing.

Complete these prerequisites before enabling WeCom JSAPI:

1. Create a **WeCom custom app** under the matching CorpID, configure its Secret, and set the correct **application visibility scope**.
2. Configure the WeCom **trusted domain / JS-SDK trusted domain** for the payment page host.
3. Configure the WeCom OAuth **web authorization callback domain** for the host serving `/api/v1/auth/oauth/wechat/payment/callback`.
4. In WeChat Pay Merchant Platform, bind the merchant to the Official Account AppID or WeCom CorpID used by the instance.
5. Configure the **JSAPI payment authorization directory** to cover the actual recharge and subscription page paths.
6. If H5 is needed, enable and configure the H5 product separately. Keep `h5Enabled=false` in examples and new deployments until its application name and domain are ready.

Explicit booleans override historical inference. H5 is independent from JSAPI/OAuth: a WeCom OAuth, identity conversion, JS-SDK, or JSAPI failure **never automatically falls back to H5**. Native is also unaffected by `jsapiAuthType`. Ordinary mobile browsers use H5 only when it has been enabled separately, desktop uses Native, and the WeCom built-in browser requires a matching `wecom` JSAPI instance without silently changing identity mode or payment instance.

Enabled instances are validated on save, while historical instances are checked again before the selected mode is used. Structured reasons include `WXPAY_CONFIG_JSAPI_AUTH_TYPE_INVALID`, `WXPAY_CONFIG_WECOM_CORPID_INVALID`, `WXPAY_CONFIG_WECOM_APP_SECRET_REQUIRED`, `WXPAY_CONFIG_WECOM_AGENT_ID_INVALID`, `WECHAT_PAYMENT_CLIENT_ENVIRONMENT_INVALID`, `WECOM_PAYMENT_PAGE_URL_INVALID`, `WECOM_PAYMENT_PAGE_URL_ORIGIN_MISMATCH`, `WECOM_PAYMENT_INSTANCE_UNAVAILABLE`, `WECHAT_PAYMENT_INSTANCE_CHANGED`, `NO_AVAILABLE_WXPAY_CAPABILITY`, `WECHAT_NATIVE_NOT_AUTHORIZED`, `WECHAT_H5_NOT_AUTHORIZED`, `WECHAT_JSAPI_NOT_AUTHORIZED`, `WECHAT_APPID_MCHID_MISMATCH`, `WECHAT_SIGN_ERROR`, and `WECHAT_PAYMENT_API_ERROR`. Error metadata is limited to troubleshooting fields such as mode, instance, HTTP status, WeChat code, request ID, or action; bodies and credentials are excluded.

`wecomAppSecret`, merchant private keys, APIv3 keys, and payment public keys are omitted from admin GET responses. Leaving a sensitive field empty during an edit preserves its stored value. While an instance owns `PENDING`, `PAID`, or `RECHARGING` orders, protected identity/key changes, disabling, and deletion are blocked so historical orders retain their original instance and verification context.

### Stripe

International payment platform supporting multiple payment methods and currencies.

| Parameter | Description | Required |
|-----------|-------------|----------|
| **Secret Key** | Stripe secret key (`sk_live_...` or `sk_test_...`) | Yes |
| **Publishable Key** | Stripe publishable key (`pk_live_...` or `pk_test_...`) | Yes |
| **Webhook Secret** | Stripe Webhook signing secret (`whsec_...`) | Yes |

---

## Provider Instance Management

You can create **multiple instances** of the same provider type for load balancing and risk control:

- **Multi-instance load balancing** — Distribute orders via round-robin or least-amount strategy
- **Independent limits** — Each instance can have its own min/max amount and daily limit
- **Independent toggle** — Enable/disable individual instances without affecting others
- **Refund control** — Enable or disable refunds per instance
- **Payment methods** — Each instance can support a subset of payment methods
- **Ordering** — Drag to reorder instances

### Instance Limit Configuration

Each instance supports these limits:

| Limit | Description |
|-------|-------------|
| **Minimum Amount** | Minimum order amount accepted by this instance |
| **Maximum Amount** | Maximum order amount accepted by this instance |
| **Daily Limit** | Daily cumulative transaction limit for this instance |

> During load balancing, instances that exceed their limits are automatically skipped.

---

## Webhook Configuration

Payment callbacks are essential for the payment system to work correctly.

### Callback URL Format

When adding a provider, the system auto-generates callback URLs from your site domain:

| Provider | Callback Path |
|----------|-------------|
| **EasyPay** | `https://your-domain.com/api/v1/payment/webhook/easypay` |
| **Alipay (Direct)** | `https://your-domain.com/api/v1/payment/webhook/alipay` |
| **WeChat Pay (Direct)** | `https://your-domain.com/api/v1/payment/webhook/wxpay` |
| **Stripe** | `https://your-domain.com/api/v1/payment/webhook/stripe` |

> Replace `your-domain.com` with your actual domain. EasyPay V1 and V2 share `/api/v1/payment/webhook/easypay`; do not create a separate V2 callback route. EasyPay / Alipay / WeChat Pay callback URLs are auto-filled when adding the provider.

### EasyPay V2 endpoints and security requirements

- Creation does not call `/api/pay/create`, whose result fields may be unsigned. Sub2API follows the official SDK flow, signs payment parameters locally with RSA-SHA256, and returns a same-origin hosted checkout URL at the fixed `GET /api/pay/submit` path. QR mode returns the same URL as both `pay_url` and `qr_code`; the initial upstream `trade_no` may be empty.
- API query, refund, and refund-query still use `POST /api/pay/query`, `POST /api/pay/refund`, and `POST /api/pay/refundquery`; responses must pass RSA, PID, and timestamp verification. Unsigned create responses and `pay_info` are never trusted.
- Verify callback RSA-SHA256 signatures first, then validate the amount, PID, and merchant order number before crediting an order.
- Query, refund, and refund-query responses are also checked against the original order number, refund number, amount, and status; missing or inconsistent fields fail safely.
- The allowed timestamp clock skew is 300 seconds; keep the deployment host synchronized to a reliable time source.
- Return plain-text `success` only after the notification has been verified and processed successfully.
- Refunds require a stable `out_refund_no`; reuse the same value when retrying the same refund to prevent duplicate refunds.

### Stripe Webhook Setup

1. Log in to [Stripe Dashboard](https://dashboard.stripe.com/)
2. Go to **Developers → Webhooks**
3. Add an endpoint with the callback URL
4. Subscribe to events: `payment_intent.succeeded`, `payment_intent.payment_failed`
5. Copy the generated Webhook Secret (`whsec_...`) to your provider configuration

### Important Notes

- Callback URLs must use **HTTPS** (required by Stripe, strongly recommended for others)
- Ensure your firewall allows callback requests from payment platforms
- The system automatically verifies callback signatures to prevent forgery
- Balance top-up is processed automatically upon successful payment — no manual intervention needed

---

## First Top-up Promo Bonus

In **Admin Dashboard → Promo Code Management**, an administrator can set a “first top-up bonus multiplier” for a registration promo code. For example, `1.2` credits 20% extra on the registered user’s first successful balance top-up.

- The bonus applies only to **balance orders created by the built-in payment system**. Subscription orders, manual admin balance adjustments, and external Admin API credits do not participate.
- Eligibility is claimed atomically per user during paid-order fulfillment. Even if several orders are paid concurrently, only one receives the bonus; the others fall back to their snapshotted base credit amount.
- Unpaid, cancelled, or expired orders do not consume eligibility. If payment succeeds but credit fulfillment fails, the decision remains attached to that order so an admin retry cannot claim or apply the bonus twice.
- Refunding a completed top-up does not restore first-top-up eligibility.
- The global balance recharge multiplier still applies to every top-up as before; the promo-code multiplier is an additional one-time multiplier on the first successful top-up.
- During upgrade, users with any historical balance payment containing `paid_at` are marked as having used the first-top-up eligibility, preventing an old user from receiving it again.

---

## Payment Flow

```
User selects amount and payment method
       │
       ▼
  Create Order (PENDING)
  ├─ Validate amount range, pending order count, daily limit
  ├─ Load balance to select provider instance
  └─ Call provider to get payment info
       │
       ▼
  User completes payment
  ├─ EasyPay     → V1 MD5 or V2 RSA-SHA256; Alipay / WeChat / QQ QR or redirect
  ├─ Alipay      → Desktop QR payload (Face-to-Face preferred, Website Pay fallback) / mobile Alipay redirect
  ├─ WeChat Pay  → Desktop Native QR / non-WeChat H5 / in-WeChat JSAPI
  └─ Stripe      → Payment Element (card/Alipay/WeChat/etc.)
       │
       ▼
  Webhook callback verified → Order PAID
       │
       ▼
  Auto top-up to user balance → Order COMPLETED
```

### Order Status Reference

| Status | Description |
|--------|-------------|
| `PENDING` | Waiting for user to complete payment |
| `PAID` | Payment confirmed, awaiting balance credit |
| `COMPLETED` | Balance credited successfully |
| `EXPIRED` | Timed out without payment |
| `CANCELLED` | Cancelled by user |
| `FAILED` | Balance credit failed, admin can retry |
| `REFUND_REQUESTED` | Refund requested |
| `REFUNDING` | Refund in progress |
| `REFUNDED` | Refund completed |

### Admin Order Attribution and Export

The admin **Order Management** page attributes orders to the promo code bound when the user registered; it does not use payment-time promotions. New orders snapshot the registration promo ID, code, and attribution state. Historical records that cannot be classified reliably are shown as “legacy attribution unknown” instead of being counted as organic registrations. Promo codes already bound to users or present in usage records can be disabled but not deleted.

Filters include status, order type, payment method, keyword, registration promo code, and date range. A date range can use **order creation time** (default) or **payment time**, and the backend converts the browser-provided IANA timezone into an inclusive calendar-date, half-open timestamp range.

Recharge totals use the system balance/USD crediting basis:

- Gross recharge: `amount` from completed balance-recharge orders;
- Refunded amount: only finalized partial or full refunds;
- Net recharge: gross recharge minus finalized refunds;
- Pending, failed, cancelled, unfulfilled, and subscription orders do not contribute to recharge totals.

Admins can export all rows matching the active filters, not only the current page:

- Order detail CSV;
- Registration-promo attribution summary CSV.

A single order-detail export is limited to 100,000 rows. Oversized exports return an explicit error and are never silently truncated. CSV output includes a UTF-8 BOM and spreadsheet-formula injection protection. This feature adds no new default or example configuration keys.

### Timeout and Fallback

- Before marking an order as expired, the background job queries the upstream payment status first
- If the user has actually paid but the callback was delayed, the system will reconcile automatically
- The background job runs every 60 seconds to check for timed-out orders

---

## Migrating from Sub2ApiPay

If you previously used [Sub2ApiPay](https://github.com/touwaeriol/sub2apipay) as an external payment system, you can migrate to the built-in payment system:

### Key Differences

| Aspect | Sub2ApiPay | Built-in Payment |
|--------|-----------|-----------------|
| Deployment | Separate service (Next.js + PostgreSQL) | Built into Sub2API, no extra deployment |
| Payment Methods | EasyPay, Alipay, WeChat, Stripe | Built-in EasyPay V1/V2, Alipay, WeChat, upstream-enabled QQ Pay, and Stripe |
| Configuration | Environment variables + separate admin UI | Unified in Sub2API admin dashboard |
| Top-up Integration | Via Admin API callback | Internal processing, more reliable |
| Subscription Plans | Supported | Built in, with native single-group and standard shared-quota modes |
| Order Management | Separate admin interface | Integrated in Sub2API admin dashboard |

### Migration Steps

1. Enable payment in Sub2API admin dashboard and configure providers (use the same payment credentials)
2. Update webhook callback URLs to Sub2API's callback endpoints
3. Verify that new orders are processed correctly via built-in payment
4. Decommission the Sub2ApiPay service

> **Note**: Historical order data from Sub2ApiPay will not be automatically migrated. Keep Sub2ApiPay running for a while to access historical records.
