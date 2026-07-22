# 分组容量提醒

## 目标与兼容性

分组容量提醒由每个分组选择一个指标和对应阈值：

- `predicted_requests`：保留原有预计剩余请求数模式，继续使用最近 `50` 条成功落账样本估算请求容量。
- `remaining_balance_usd`：字段值与 API 枚举保持不变，但语义调整为**分组预测剩余余额（USD）**。

金额模式的计算定义为：

```text
分组预测剩余余额（USD）
= 分组内池模式账号的权威 USD 余额之和
+ 分组内普通账号的估算 USD 余额之和
```

只有计算结果**严格小于**分组配置的阈值时才进入低容量状态并通知管理员。结果等于阈值时不告警。

`unknown`、`stale`、非权威余额、单位不兼容或其他数据不完整情况绝不按 `0` 计入。只要本次分组重算不能得到完整、可比较的结果，就跳过本次状态变更：既不创建新的低容量 episode，也不把已有低容量状态恢复为健康。

默认兼容旧配置：提醒开关关闭，指标为 `predicted_requests`，请求阈值为 `50`。

## 触发条件

### `predicted_requests`

请求预测模式保留原有触发和计算方式：一次请求成功落账后，以最终成功计费的分组和池模式账号为准；故障转移中尝试过但未最终计费的账号不参与本次计算。

请求还必须满足原有样本条件，例如计费幂等结果为 `Applied=true`、不是 `cyber` 拒绝记录、相关成本大于 `0`，并能组成固定 `50` 条有效成功落账样本。

### `remaining_balance_usd`

金额模式按分组重算，不再限定“本次请求最终命中池模式账号”。分组内任意普通账号或池模式账号成功计费后，都可以触发该分组的余额重算，但必须同时满足：

1. 本次计费已成功提交，幂等结果为 `Applied=true`。
2. 最终计费分组已开启 `pool_capacity_alert_enabled`，且选择 `remaining_balance_usd`。
3. 请求不是 `cyber` 拒绝记录，并满足计费事件的有效性要求。
4. 能读取该分组全部应计入账号的当前容量数据，并完成统一 USD 计算。

汇总账号范围为分组内当前启用、可调度且未因到期自动暂停的账号；重复关联的同一账号只计算一次。触发计费的账号只负责唤起重算。金额指标比较的是整个分组的预测剩余余额，不把触发账号本身当作唯一容量来源或瓶颈。

## `predicted_requests` 旧模式

请求预测模式继续读取最终分组最近 `50` 次符合条件的成功落账记录。由于 usage log 通过异步 worker 批量落库，评估查询读取此前最多 `49` 条，本次成功计费成本直接作为第 `50` 个样本。

平均值口径保持不变：

- `avg_account_cost`：账户统计成本平均值，使用 `account_stats_cost`（存在时）或 `total_cost`，再乘账户倍率。
- `avg_actual_cost`：用户实际扣费平均值，即 `actual_cost`。

原有账户容量、本地账户额度、API Key 配额和余额计费钱包的有限容量仍按旧逻辑换算为预计请求数，并取最小值。少于 `49` 条历史样本时不评估，不改变现有告警状态。

## `remaining_balance_usd` 分组金额模式

金额模式不使用 API Key 配额或用户钱包，也不再计算 `account / api_key / wallet` 最小瓶颈。它只汇总分组内账号容量。

### 池模式账号

池模式账号只接受当前、权威且单位为 `USD` 或 `$` 的上游余额。每个账号的合格 `remaining` 值直接作为该账号的 USD 贡献值。

以下数据不能用于金额汇总：

- `stale`、`unknown` 或 `unsupported`；
- 非权威快照；
- `requests`、EUR、空单位或其他非 USD 单位；
- 非法、缺失或无法安全比较的数值。

这些情况不等于余额为 `0`。任一应计入池账号缺少合格数据时，本次整个分组重算视为数据不完整并跳过。池账号若返回明确、权威的 `unlimited`，则分组结果为完整的无限容量，不会触发低余额告警。

### 普通账号

普通账号使用统一账户容量中的 `usage_window` 或 `local_quota` 数据估算 USD 贡献：

- **`usage_window`**：使用预计剩余请求数乘以平均每请求成本。

  ```text
  普通账号估算 USD 余额 = remaining_requests × average_cost_per_request
  ```

  容量状态必须为 `estimated`，且必须已有有效样本；预计请求数需为非负整数，平均成本需为有限正数。缺少预计请求数、平均成本或样本时，不能把该账号按 `0` 处理。

- **`local_quota`**：本地额度的剩余值已经是 USD，直接计入分组总额，不再做 requests → USD 换算。若快照仅因缺少均次成本而标记 `unknown / insufficient_cost_sample`，其请求数预测虽然未知，但已知、当前的本地 USD 剩余额度仍可计入；其他 `unknown`、`stale` 或 `unsupported` 情况仍会跳过。

普通账号的容量状态、模式或单位无法支持上述换算时，本次分组重算同样跳过。

### 分组汇总与阈值

只有全部应计入账号都成功产生兼容的 USD 贡献值时，才求和并比较：

```text
group_remaining_balance_usd < pool_capacity_alert_threshold_usd
```

例如阈值为 `$10.00`：

- `$9.99`：进入低容量状态并创建通知 episode；
- `$10.00` 或更高：健康；
- 任一账号数据为 `unknown`、`stale`、单位不兼容或无法换算：跳过，不改变状态。

金额模式无需等待 `50` 条历史成功落账样本。普通账号的 `usage_window` 换算只使用其容量摘要提供的平均成本，不读取请求预测模式的分组 50 样本作为替代。

## 上游余额探测与账户容量

池模式且具有 Bearer API Key 和自定义 `base_url` 的账号会探测 `{base_url}/v1/usage`。Sub2API 上游通过 `object=sub2api.key_usage`、`schema_version=1`、合法 `mode` 和完整结构进行强识别。

安全约束保持不变：

- 复用账号代理和 TLS 指纹设置；配置了代理但代理不可用时不回退直连。
- URL 校验与账号连通性测试保持一致；启用 `security.url_allowlist` 时，自定义上游主机必须加入 `upstream_hosts`。
- Bearer 认证头不会被 Header Override 覆盖；请求禁止重定向。
- 响应大小、缓存与超时均受 `account_capacity` 配置约束。
- 最近成功快照可以在管理界面标记为 `stale` 展示，但告警重算不会消费 stale 数据。

原生 AWS Bedrock SigV4 没有通用实时余额端点，且 AWS 凭据不会作为 Bearer Key 发出，因此这类账号可能显示 `unsupported`。如果该账号属于应汇总账号，金额模式会跳过本次不完整的分组重算，而不是按 `0` 处理。

## 持久化状态范围

两种指标使用不同状态范围，以保留请求预测旧模式：

- `predicted_requests` 使用原有 `context` 范围：

  ```text
  group_id + group_generation + account_id + api_key_id + user_id + billing_type
  ```

- `remaining_balance_usd` 使用新的 `group` 范围：

  ```text
  group_id + group_generation
  ```

金额状态不再按账号、API Key、用户或计费类型拆分。状态和事件保存分组总余额、池模式账号权威余额小计、普通账号估算余额小计，以及两类纳入账号数量；金额事件不会保存 API Key / 钱包瓶颈。请求预测状态和事件继续保留原有 context 明细。

状态行为：

- `healthy -> low`：完整计算值严格小于阈值时立即创建新 episode。
- 持续 `low`：达到 `reminder_cooldown_hours` 后创建提醒 episode。
- `low -> healthy`：完整计算值大于或等于阈值时恢复；之后再次低于阈值会创建新 episode。
- **金额数据不完整**：不执行上述任何迁移，保留当前 group 状态和 episode。
- 分组开关、metric 或对应阈值实际变化时，`pool_capacity_alert_generation` 自增一次。
- 旧 generation 的评估任务会被丢弃，尚未完成的旧 generation delivery 会被取消。
- 迁移到新的金额语义时，所有已选择 `remaining_balance_usd` 的分组 generation 会推进一次，取消旧 context 语义的待投递任务。
- 复制分组时复制 metric 和阈值作为惰性配置，但开关强制关闭，generation 重置为 `0`。

## 通知内容与收件人

`predicted_requests` 通知继续使用原有计费 context 内容。`remaining_balance_usd` 通知改为以**分组**为主体，应表达：

- 分组名称或 ID；
- 当前生效指标和分组预测剩余余额总和；
- 池模式账号权威 USD 余额小计与纳入账号数；
- 普通账号估算 USD 余额小计与纳入账号数；
- 配置阈值，以及“严格小于阈值”的触发关系；
- episode 类型和评估时间。

金额通知不再把某个触发账号描述为唯一告警对象，也不再包含 API Key 配额或用户钱包瓶颈。数据不完整时不会创建通知，因此通知中不会把 unknown、stale 或不兼容单位显示为 `$0`。

### 管理员邮件

事件创建时，为所有符合条件的管理员主邮箱创建独立 delivery：管理员必须处于启用状态、未软删除、主邮箱格式有效且不是连接占位邮箱。发送前会再次确认管理员资格。

邮件使用不可退订的管理员事务通知事件：

```text
account.pool_capacity_low
```

事件名为兼容性保持不变，但模板语义应按分组容量呈现。

### QQBot

仅当当前进程中的 QQBot 已启用且存在当前 AppID 时创建 QQBot delivery。收件人必须是启用管理员，并在当前机器人 AppID 下具有已验证的 C2C identity channel。发送前会重新校验管理员状态、绑定状态和当前 AppID。

## 异步与失败隔离

网关热路径只执行有界、非阻塞的内存队列投递：

- 队列满时丢弃本次评估并记录日志；
- 样本聚合、分组余额查询、状态机写入、SMTP 和 QQ HTTP 均在后台执行；
- 通知失败不回滚或影响已完成计费；
- delivery 使用数据库租约支持多实例抢占、进程重启恢复和指数退避重试；
- 发送前发现 generation、管理员资格或 QQ C2C 绑定失效时取消陈旧 delivery。

## 管理员 API

API 枚举和表单结构保持不变。分组创建、更新、列表和详情继续使用以下字段：

```json
{
  "pool_capacity_alert_enabled": true,
  "pool_capacity_alert_metric": "remaining_balance_usd",
  "pool_capacity_alert_threshold_requests": 50,
  "pool_capacity_alert_threshold_usd": 10.0
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

- 创建缺省为 `enabled=false`、`metric=predicted_requests`、`threshold_requests=50`、`threshold_usd=null`。
- 更新采用可选 patch 语义，未提供字段保持原值。
- metric 仍只允许 `predicted_requests` 或 `remaining_balance_usd`。
- 请求阈值范围为 `1..1_000_000_000`；USD 阈值范围为 `0.01..1e15`。
- 选择 `remaining_balance_usd` 时必须提供有效 USD 阈值。
- `remaining_balance_usd` 的字段名未变，但返回和配置语义是“分组预测剩余余额（USD）”，不是单账号权威余额，也不是 Key / 钱包最小余额。
- `predicted_requests` 继续使用原有 context 状态；`remaining_balance_usd` 使用分组级状态。状态范围不改变上述管理员 API 的请求或响应结构。
- 内部 generation 不通过管理员或用户 DTO 暴露；普通用户分组响应不暴露告警配置。

## 管理员账户列表容量展示

账户列表的“用量窗口”仍展示单账号容量来源，供理解分组金额汇总：

- 池模式：显示上游真实余额、来源和更新时间；查询失败可展示最近快照并明确标记 `stale`。
- 普通账号 `usage_window`：展示官方窗口或本地推算的预计剩余请求数以及平均每请求成本；金额汇总时两者相乘。
- 普通账号 `local_quota`：展示本地剩余 USD 额度；金额汇总时直接计入。
- `unknown` 不表示剩余为 `0`。

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
  group_balance_concurrency: 4
  group_balance_timeout_seconds: 60
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

- `account_capacity` 控制单账号上游探测超时、成功/错误缓存和 UI stale 保留时间。
- `pool_capacity_alert.enabled` 是全局运行时开关；每个分组仍默认关闭。
- `group_balance_concurrency` 限制一次分组余额重算中的账号查询并发数。
- `group_balance_timeout_seconds` 限制一次分组余额重算的总耗时；超时视为数据不完整，不改变状态。
- 请求预测样本数固定为 `50`；metric 和阈值仍由每个分组配置。
- `lease_seconds` 会被规范化为不小于 `send_timeout_seconds + 30`。

## 运维检查

1. 应用当前版本随附的全部数据库迁移，包括将金额告警切换为分组状态的 `194_group_predicted_balance_alert.sql`。
2. 确认 SMTP 已配置；如需 QQ 提醒，确认 QQBot 已启用且管理员完成当前机器人 C2C 绑定。
3. 在管理端分组创建/编辑弹窗中开启提醒，选择指标并配置对应阈值。
4. 请求模式：产生足够的成功落账样本，验证 `49 < 50` 告警而 `50` 不告警。
5. 金额模式：分别准备池模式权威 USD 余额、普通 `usage_window` 与普通 `local_quota` 账号，验证分组结果按求和公式计算。
6. 让分组内任意普通或池账号成功计费，确认都会触发金额模式的分组重算。
7. 分别模拟 `stale`、`unknown`、非权威、单位不兼容、缺少平均成本和查询超时，确认不会按 `0` 计入，也不会创建、恢复或提醒。
8. 验证金额通知仅表达分组聚合值和阈值，不再显示 API Key / 钱包瓶颈。
9. 修改开关、metric 或阈值，确认 generation 变化后旧任务和旧待投递通知被取消。
