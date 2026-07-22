# 池模式账户容量提醒

## 目标

当一次请求完成成功计费后，如果**最终实际命中的账户**启用了 `pool_mode`，且**最终计费分组**启用了 `pool_capacity_alert_enabled`，系统会按该分组选择的单一指标评估容量：

- `predicted_requests`：根据近期成功落账请求的平均成本，预测该计费上下文还能支持多少次请求。
- `remaining_balance_usd`：直接比较经验证的权威 USD 上游余额与本地账户额度、API Key 配额和用户钱包中的最小剩余金额。

当所选指标值严格小于该分组的自定义阈值时，系统创建持久化告警事件，并通过管理员主邮箱和当前 QQBot 的已验证 C2C 绑定主动通知管理员。默认兼容旧行为：关闭提醒，指标为 `predicted_requests`，请求阈值为 `50`。

> 两种指标不会并行触发。请求模式中的“剩余请求数”是基于历史均次成本的预测；金额模式只接受当前、权威且单位为 USD 的真实余额，绝不通过预计请求数或均次成本反推金额。等于阈值不会告警。

## 生效条件

必须同时满足：

1. 本次请求已由统一计费仓储成功提交，且幂等结果为 `Applied=true`。
2. 本次最终实际使用的账户 `Account.IsPoolMode()` 为 `true`。
3. 本次最终计费分组 `pool_capacity_alert_enabled=true`。
4. 请求不是 `cyber` 拒绝记录，且本次用户实际成本和账户成本均大于 `0`。
5. 最终分组、API Key、账户和用户信息完整。
6. 若指标为 `predicted_requests`，已取得该分组此前 `49` 条符合条件的成功落账样本，本次成本作为第 `50` 个样本；金额模式不等待历史样本。

如果请求在故障转移后使用了其他账户或分组，只以最终成功计费的账户和分组为准；中途尝试过但未最终计费的账户不参与判断。

## 请求预测模式的样本口径

仅 `predicted_requests` 模式读取历史样本。样本来自 `usage_logs`，按 `created_at DESC, id DESC` 取最终分组最近记录，并满足：

- `actual_cost > 0`；
- `request_type <> cyber`；
- 账户成本大于 `0`；
- 对应 `(request_id, api_key_id)` 已存在于 `usage_billing_dedup` 或归档表；
- 排除本次 `(request_id, api_key_id)`，避免 usage log 已提前可见时重复计算。

由于 usage log 通过异步 worker 批量落库，评估查询只读取此前最多 `49` 条，本次计费成本直接从成功计费结果加入，组成固定 `50` 条样本。

平均值：

- `avg_account_cost`：账户统计成本平均值，使用 `account_stats_cost`（存在时）或 `total_cost`，再乘账户倍率。
- `avg_actual_cost`：用户实际扣费平均值，即 `actual_cost`。

少于 `49` 条历史样本时不进行请求预测，也不触发请求数告警；这不影响金额模式评估。

## 容量计算

### `predicted_requests`

账户、API Key 和用户三类容量使用不同的数据源：

- **账户容量**：从池账户自定义上游的 `GET /v1/usage` 获取经验证的真实余额。`unit=USD` 时计算 `floor(upstream_remaining / avg_account_cost)`；上游明确返回 `unit=requests` 时直接取请求数。
- **本地账户额度安全上限**：若账户同时配置了总/日/周本地额度，仍使用计费事务返回的扣费后状态计算本地容量，并与上游容量取较小值。本地额度不再被当成上游真实余额。
- **API Key 容量**：配置了 Key 配额时，使用计费事务返回的扣费后状态，计算 `floor((quota - quota_used) / avg_actual_cost)`。
- **用户容量**：余额计费时使用扣费后钱包余额，计算 `floor(balance / avg_actual_cost)`；订阅计费不使用用户钱包作为瓶颈。

最终预计剩余请求数为所有有限容量中的最小值。未配置限额或上游明确无限的维度不参与取最小值。

### `remaining_balance_usd`

金额模式不读取历史均次成本，也不进行 requests → USD 换算：

- **账户金额**：权威上游 USD `remaining` 与扣费后本地总/日/周 USD 剩余额度取较小值。
- **API Key 金额**：配置 Key 配额时使用扣费后 `quota - quota_used`。
- **用户钱包金额**：余额计费时使用扣费后余额减 `billing.minimum_balance_reserve`；订阅计费不加入钱包维度。
- **最终金额**：取所有有限 USD 容量的最小值，并记录 `account / api_key / wallet` 瓶颈。未配置或明确无限的维度不参与；全部明确无限时视为可信健康。

如果上游快照不是 `verified + authoritative + USD/$`，也不是明确 `unlimited`，整个金额评估会跳过，不能只凭本地额度触发或恢复状态。

## 上游真实余额探测

仅 `pool_mode` 且具有 Bearer API Key 和自定义 `base_url` 的账户会探测 `{base_url}/v1/usage`。Sub2API 上游通过 `object=sub2api.key_usage`、`schema_version=1`、合法 `mode` 和完整 `quota/subscription/balance` 结构进行强识别；旧版响应只有在结构完整且数值一致时才兼容。

安全约束：

- 复用账户代理和 TLS 指纹设置；配置了 `ProxyID` 但代理未加载或 ID 不一致时直接失败，不回退直连。
- URL 校验与账户连通性测试保持一致：启用 `security.url_allowlist` 时，自定义上游主机必须加入 `upstream_hosts`；关闭时仍会校验 URL 格式、协议、主机和端口，并按账户保存的精确 `base_url` 发起探测。生产环境若允许非受信管理员配置上游，仍建议开启白名单。
- Bearer 认证头不会被账户 Header Override 覆盖；请求禁止重定向。
- 默认总超时 `10` 秒，响应体上限 `64 KiB`，单次探测不自动重试。
- 成功缓存 `60` 秒、错误缓存 `30` 秒；最近成功值最多保留 `5` 分钟并只在管理端标记为 `stale`。

原生 AWS Bedrock SigV4 没有通用实时余额端点，且 AWS 凭据绝不会作为 Bearer Key 发出，因此标记为 `unsupported`。Bedrock API Key 或其他自定义 Base URL 只有满足兼容 Bearer `/v1/usage` 契约时才可验证。

告警状态机只消费 `verified` 或明确 `unlimited` 的权威快照。查询失败、`stale`、`unknown`、`unsupported`、非法响应或单位与当前指标不兼容时，本次评估直接跳过：既不创建新告警，也不把既有 `low` episode 错误恢复为 `healthy`。请求模式允许权威 USD 或 requests；金额模式只允许权威 USD/$。

告警条件统一为：

```text
metric_value < configured_threshold
```

例如请求阈值为 `50` 时：`49` 告警，`50` 和 `51` 不告警。USD 阈值为 `10.00` 时：`9.99` 告警，`10.00` 不告警。

## 持久化状态机

数据库迁移：

- `190_add_group_pool_capacity_alert.sql`
- `191_pool_capacity_alert_runtime.sql`
- `192_add_usage_logs_pool_capacity_samples_index_notx.sql`（并发构建热表索引，不阻塞 usage log 写入）
- `193_add_group_pool_capacity_alert_thresholds.sql`（增加分组指标/阈值以及状态、事件策略快照）

运行时表：

- `pool_capacity_alert_states`：按分组 generation、账户、API Key、用户、计费类型保存 `healthy/low` 状态、所选 metric、当前值/阈值和 episode。
- `pool_capacity_alert_events`：保存每次告警 episode 的不可变策略与容量快照；金额事件的请求预测字段允许为空。
- `pool_capacity_alert_deliveries`：保存 recipient/channel 级投递、租约、尝试次数、重试和最终状态。

状态范围：

```text
group_id + group_generation + account_id + api_key_id + user_id + billing_type
```

行为：

- `healthy -> low`：立即创建新 episode。
- 持续 `low`：达到 `reminder_cooldown_hours` 后创建提醒 episode。
- `low -> healthy`：恢复健康状态；下次再次低于阈值时创建新 episode。
- 分组开关、metric 或对应阈值任一实际值发生变化时，`pool_capacity_alert_generation` 只自增一次。
- 队列中的旧 generation 任务会被丢弃；尚未完成的旧 generation delivery 会被取消。
- 复制分组时复制 metric/阈值作为惰性配置，但开关强制关闭，generation 重置为 `0`。

## 通知收件人

### 管理员邮件

事件创建时，为所有满足以下条件的管理员主邮箱创建独立 delivery：

- `role=admin`；
- `status=active`；
- 未软删除；
- 主邮箱格式有效；
- 不是连接占位邮箱。

发送前会再次确认管理员仍处于启用状态。邮件使用通知模板事件：

```text
account.pool_capacity_low
```

该事件属于不可退订的管理员事务通知，可在通知邮件模板管理中自定义中英文模板。

### QQBot

仅当当前进程中的 QQBot 已启用且存在当前 AppID 时创建 QQBot delivery。收件人必须：

- 仍是启用管理员；
- 在当前机器人 AppID 下完成已验证身份绑定；
- 存在 `c2c` identity channel。

每次主动发送前，QQBot Service 会按 identity-channel ID 重新校验当前 AppID、管理员状态、验证状态和 C2C 场景。生产日志不会记录 OpenID 或 channel subject。

## 异步与失败隔离

网关热路径只执行有界、非阻塞的内存队列投递：

- 队列满时丢弃本次评估并记录告警日志；
- 历史聚合、状态机写入、SMTP 和 QQ HTTP 均在后台运行；
- 通知失败不回滚或影响已完成计费；
- delivery 使用数据库租约支持多实例抢占、进程重启恢复和指数退避重试；
- 发送前发现开关 generation、管理员资格或 QQ C2C 绑定已经失效时直接取消陈旧 delivery；
- 其余明确永久发送错误直接进入 `dead`，临时错误在最大尝试次数内重试。

## 管理员 API

分组创建、更新和管理员分组响应新增字段：

```json
{
  "pool_capacity_alert_enabled": true,
  "pool_capacity_alert_metric": "predicted_requests",
  "pool_capacity_alert_threshold_requests": 50,
  "pool_capacity_alert_threshold_usd": null
}
```

相关接口：

```http
POST /api/v1/admin/groups
PUT /api/v1/admin/groups/:id
GET /api/v1/admin/groups
GET /api/v1/admin/groups/:id
```

规则：

- 创建时未提供新字段，默认 `enabled=false`、`metric=predicted_requests`、`threshold_requests=50`、`threshold_usd=null`。
- 更新时四个字段均采用可选 patch 语义；未提供的字段保持原值。
- metric 只允许 `predicted_requests` 或 `remaining_balance_usd`；请求阈值范围为 `1..1_000_000_000`，USD 阈值范围为 `0.01..1e15`。
- 选择 `remaining_balance_usd` 时必须提供有效 USD 阈值。请求模式可保留 USD 阈值供以后切换，但当前不参与判断。
- 开关、metric 或任一阈值实际变化时内部 generation 只自增一次；重复提交相同值不递增。
- `pool_capacity_alert_generation` 是内部一致性字段，不通过管理员或用户 DTO 暴露。
- 普通用户可见的分组响应不暴露任何告警配置。

## 管理员账户列表容量展示

管理员账户列表的“用量窗口”列统一展示 `capacity`：

- 池模式：显示上游真实余额、来源、更新时间和可用时的预计剩余请求数；查询失败可显示最近快照并明确标记 `stale`。
- 非池模式：优先使用官方窗口的 `limit_requests/used_requests`；否则按 `floor(requests * (100 - utilization) / utilization)` 估算，并明确标记 `estimated`。
- 无官方窗口但配置了本地总/日/周额度时，按本地剩余额度与本地平均账户成本估算，模式标记为 `local_quota`。
- `unknown` 不表示剩余为 `0`；无有限本地额度时可显示 `unlimited`。

管理员接口为：

```http
GET /api/v1/admin/accounts/:id/usage
GET /api/v1/admin/accounts/:id/usage?force=true
```

`force=true` 会绕过池余额成功/错误 TTL，但仍参与 singleflight 合并。外部 API Key 用量契约见 [`API_KEY_USAGE.md`](API_KEY_USAGE.md)。

## 部署配置

```yaml
account_capacity:
  upstream_timeout_seconds: 10
  success_cache_seconds: 60
  error_cache_seconds: 30
  stale_cache_seconds: 300

pool_capacity_alert:
  enabled: true
  evaluation_worker_count: 2
  queue_size: 256
  evaluation_timeout_seconds: 15
  delivery_worker_count: 4
  delivery_batch_size: 50
  poll_interval_seconds: 5
  lease_seconds: 90
  max_attempts: 6
  retry_base_seconds: 30
  max_retry_seconds: 3600
  send_timeout_seconds: 20
  reminder_cooldown_hours: 24
```

说明：

- `account_capacity` 控制上游探测超时、成功/错误缓存和 UI stale 保留时间。
- `pool_capacity_alert.enabled` 是全局运行时开关；每个分组仍默认关闭。
- 请求预测的样本数固定为 `50`；告警 metric 和阈值由每个分组配置，不通过 YAML 设置。
- `lease_seconds` 会被规范化为不小于 `send_timeout_seconds + 30`。
- worker、队列、超时和重试参数在加载配置时会限制到安全范围。

## 运维检查

1. 应用迁移 `190`、`191`、`192`、`193`；其中 `192` 必须按非事务迁移执行。
2. 确认 SMTP 已配置；否则邮件 delivery 会按策略重试或进入 dead。
3. 如需 QQ 提醒，确认 QQBot 已启用且管理员完成当前机器人 C2C 绑定。
4. 为测试池账户配置 Bearer API Key 与自定义 Base URL，并确认其 `/v1/usage` 返回可验证的 Sub2API 契约；原生 Bedrock 不支持该探测。
5. 在管理端分组创建/编辑弹窗中开启提醒，选择一个 metric 并配置对应阈值。
6. 请求模式：产生至少 50 次成功落账请求，验证值等于阈值不告警、严格小于时创建事件与两类 delivery。
7. 金额模式：无需等待 50 次样本，验证权威 USD 值严格小于阈值时告警；requests/EUR/stale/unknown 快照必须跳过。
8. 模拟上游超时或 429，确认列表显示 `stale/unknown` 且告警状态机不发生恢复或新建。
9. 修改开关、metric 或阈值，确认 generation 变化后旧待投递任务被取消。
