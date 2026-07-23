# 管理员：分组容量展示与提醒

分组容量提醒用于提前发现整个分组的可用容量即将耗尽。每个分组开启后选择一个指标并配置对应阈值：

- **预计剩余请求数**（`predicted_requests`）：保留原有 50 条成功落账样本预测模式。
- **分组预测剩余余额（USD）**（`remaining_balance_usd`）：字段名不变，语义为分组内池模式账号权威 USD 余额之和，加上普通账号估算 USD 余额之和。

```text
分组预测剩余余额（USD）
= 池模式账号权威 USD 余额之和
+ 普通账号估算 USD 余额之和
```

只有当前值**严格小于**阈值时才提醒。等于阈值不提醒；`unknown`、`stale`、单位不兼容或其他不完整数据不会按 `0` 处理，也不会改变已有告警状态。

## 开启方式

进入管理端 **分组管理**，在创建或编辑分组时开启“分组容量提醒”。开关开启后会显示指标二选一和对应阈值输入。切换指标不会把 `50 requests` 解释成 `50 USD`，关闭开关也不会清空本次编辑值。

旧分组或旧响应缺少告警字段时，界面仍按“预计剩余请求数 / 50”显示，开关默认关闭。

## 配置列表容量预测

“预估剩余”列表展示有独立于告警指标的算法配置：

- **历史请求成本预测**（`historical_requests`，默认）：继续显示现有剩余请求数。
- **固定单次生图成本预测**（`fixed_image_cost`）：填写一张成功输出图片预计消耗的账号容量 USD，系统用分组已知余额除以该成本并向下取整，显示预计剩余图片数。

固定成本不是用户售价，不会自动叠加图片分辨率价格、模型价格或分组图片倍率；多图请求按输出张数计算多个预测单位。此配置适用于任意相关分组，不按平台或分组名称硬编码，且不会修改容量告警 metric、阈值、generation 或状态。

## 触发范围

### 预计剩余请求数

`predicted_requests` 保留原有行为：请求成功落账后，以最终成功计费的分组和池模式账号为准。故障转移中尝试过但未最终计费的账号不参与。系统需要最终分组最近 `50` 次有效成功落账样本，少于样本数时跳过评估且不改变状态。

### 分组预测剩余余额

`remaining_balance_usd` 按分组重算，不再要求本次请求最终命中池模式账号。只要分组启用了金额指标，分组内任意普通账号或池模式账号成功计费后，都可以触发该分组的余额重算。

汇总范围是分组内当前启用、可调度且未因到期自动暂停的账号；重复关联的同一账号只计算一次。触发计费的账号只是重算入口。系统会重新汇总全部应计入账号，不会只比较该账号的余额。

## 金额模式的账号口径

### 池模式账号

池模式账号必须提供当前、权威且单位为 `USD` 或 `$` 的上游 `remaining` 余额，该值直接计入分组总额。

以下情况不能按 `0` 计入：

- `stale`、`unknown`、`unsupported`；
- 非权威快照；
- `requests`、EUR、空单位或其他非 USD 单位；
- 缺失、非法或无法安全比较的数值。

任一应计入池账号出现上述情况，本次分组数据不完整，系统会跳过状态变更。池账号若返回明确、权威的 `unlimited`，则分组结果为完整的无限容量，不会触发低余额提醒。

### 普通账号

普通账号从统一容量数据中使用以下模式：

- **`usage_window`**：容量状态必须为 `estimated` 且已有有效样本，再用预计剩余请求数乘以平均每请求成本。

  ```text
  估算 USD 余额 = remaining_requests × average_cost_per_request
  ```

- **`local_quota`**：剩余值已经是 USD，直接计入分组总额。若快照只是因为缺少均次成本而标记为 `unknown / insufficient_cost_sample`，请求数预测虽不可用，但当前本地 USD 剩余额度仍然已知并可计入；其他 `unknown`、`stale` 或 `unsupported` 状态会跳过。

`usage_window` 缺少预计请求数或平均成本、`local_quota` 缺少可用 USD 剩余额度，或者状态/单位不兼容时，都视为数据不完整，不按 `0` 计入。

### 不再加入 Key 或钱包

金额模式只汇总账号容量，不再把以下数据加入比较：

- API Key 的 `quota - quota_used`；
- 用户钱包余额或最小余额预留；
- `account / api_key / wallet` 最小瓶颈。

阈值比较对象始终是完整的分组预测剩余余额总和。

## 严格阈值与不完整数据

金额模式仅在全部应计入账号都得到兼容 USD 贡献值后执行：

```text
group_remaining_balance_usd < pool_capacity_alert_threshold_usd
```

阈值为 `$10.00` 时：

- `$9.99`：提醒；
- `$10.00` 或更高：不提醒；
- 任一账号为 `unknown`、`stale`、单位不兼容、无法换算或查询超时：跳过本次计算，不创建低容量状态，也不恢复已有低容量状态。

金额模式不需要等待 50 条分组历史样本。普通 `usage_window` 的平均成本来自账号容量摘要，不使用请求模式的 50 样本反推池账号金额。

## 状态范围与降噪

`predicted_requests` 保留原有 context 状态范围：

```text
group_id + group_generation + account_id + api_key_id + user_id + billing_type
```

`remaining_balance_usd` 改用分组级状态范围：

```text
group_id + group_generation
```

金额状态和事件记录分组总余额、池模式账号权威余额小计、普通账号估算余额小计，以及两类纳入账号数量；不再按 API Key、用户或计费类型拆分。请求模式的 context 状态和通知内容保持原有行为。

状态迁移规则：

- `healthy -> low`：完整结果严格小于阈值时立即创建新 episode。
- 持续 `low`：达到 `reminder_cooldown_hours` 后创建提醒 episode。
- `low -> healthy`：完整结果大于或等于阈值时恢复。
- 金额数据不完整：保持现有 group 状态和 episode，不执行任何迁移。
- 告警开关、metric 或对应阈值发生变化时切换内部 generation，旧任务和旧待投递提醒不会跨配置世代继续发送。
- 列表展示的预测模式或固定生图成本变化不会切换告警 generation，也不会改变告警状态。
- 新金额语义上线时，已选择 `remaining_balance_usd` 的分组 generation 会推进一次，以取消旧 context 语义的待投递任务。
- 复制分组时保留 metric/阈值作为惰性配置，但提醒开关强制关闭，generation 从 `0` 开始。

## 通知内容与渠道

`predicted_requests` 继续使用原有计费 context 通知。`remaining_balance_usd` 通知改为以分组为主体，包含分组、分组预测剩余余额总和、阈值、触发关系、池模式账号权威余额小计与账号数、普通账号估算余额小计与账号数，以及 episode 信息。

金额通知不再把触发本次重算的账号描述成唯一告警对象，也不再显示 API Key 配额或用户钱包瓶颈。数据不完整时不会创建通知，因此 unknown、stale 或单位不兼容不会在通知中显示为 `$0`。

### 管理员邮件

系统向所有仍处于启用状态的管理员有效主邮箱发送事务通知。SMTP 发送失败不会影响网关计费，并由持久化 delivery 队列重试。

兼容事件名仍为：

```text
account.pool_capacity_low
```

邮件模板内容应使用新的分组级语义。

### QQBot

如果当前 QQBot 已启用，系统还会通知在当前机器人下完成 C2C 绑定的启用管理员。发送前会重新校验管理员身份、当前 AppID 和绑定状态；失效的陈旧 delivery 会被取消。

## 管理员 API 字段

创建或更新分组时可以同时传入列表预测配置和原告警配置：

```json
{
  "predicted_capacity_mode": "fixed_image_cost",
  "predicted_image_unit_cost_usd": 0.04,
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

列表预测缺省为 `historical_requests / null`；`fixed_image_cost` 必须配置 `1e-12..1e15` 的有限正数成本，更新时可用显式 `null` 清空。历史模式允许成本为空或保留有效休眠值。

告警创建缺省仍为关闭、`predicted_requests`、`50` 和 `null`，原阈值校验及可选 patch 语义不变。`remaining_balance_usd` 仍表示完整分组预测余额，不是单个池账号余额，也不是账号、Key、钱包中的最小余额。内部 generation 不通过 API 暴露，普通用户分组响应既不暴露提醒配置，也不暴露列表预测配置。

## 分组列表预测容量展示

管理端“分组管理”的“预估剩余”列按当前页批量展示 USD 余额与当前选中的请求或图片容量。它复用现有容量快照、缓存和并发边界；模式和固定成本来自分组数据库配置。

```http
GET /api/v1/admin/groups/predicted-capacity-summary?ids=1,2,3
```

固定生图成本模式会返回：

```json
{
  "prediction_mode": "fixed_image_cost",
  "prediction_unit": "image",
  "prediction_configured": true,
  "prediction_complete": false,
  "prediction_unlimited": false,
  "predicted_quantity": "1062",
  "prediction_unit_cost_usd": 0.04,
  "known_prediction_account_count": 3,
  "unknown_prediction_account_count": 1
}
```

- `ids` 为逗号分隔的正整数，按首次出现顺序去重，最多 100 个唯一 ID。
- 单个分组查询失败只返回该行 `available=false`，不会拖垮整页。
- 历史模式的通用字段映射现有请求估值；固定生图模式按 `floor(known_remaining_balance_usd / prediction_unit_cost_usd)` 计算图片数。
- 完整估值显示 `≈`；数据不完整但存在已知账号时显示 `≥` 已知下界；无限显示“无限”；完全不可估显示“数据不足”。
- 新通用字段 `predicted_quantity` 使用十进制字符串，固定图片容量即使超过 `int64` 也不会丢精度。
- 旧 `estimated_remaining_requests` 保持原有 JSON number / `int64` 契约并始终独立返回历史请求估值；图片数不会写入该字段，因此旧前端和现有告警语义保持兼容。
- `prediction_configured=false`、未知、过期、单位不兼容或读取失败都不会按 0 处理。
- 余额、历史请求数和当前所选数量独立判断完整性；固定生图模式以余额完整性为基础。
- 前端只在该列可见时查询，并在筛选、翻页、隐藏列或离开页面时取消陈旧请求。

## 账户容量展示

管理员账户列表的“用量窗口”继续展示单账号容量，供确认分组汇总来源：

- 池模式显示上游真实余额；最近成功值只能以 `stale` 展示，不能参与告警汇总。
- 普通 `usage_window` 显示预计剩余请求数和平均每请求成本，金额模式将两者相乘。
- 普通 `local_quota` 的剩余 USD 直接计入金额汇总。
- `unknown` 不代表剩余为 `0`；原生 AWS Bedrock SigV4 因没有通用余额端点可能显示 `unsupported`。

手动刷新池账号会调用 `GET /api/v1/admin/accounts/:id/usage?force=true`，绕过容量 TTL 并重新验证上游。

## 部署要求

示例配置位于 `deploy/config.example.yaml`：

```yaml
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

- `group_balance_concurrency`：一次分组重算中并发查询账号余额的最大数量。
- `group_balance_timeout_seconds`：一次分组余额重算的总超时；超时按数据不完整处理，不改变状态。
- 邮件提醒需要可用 SMTP 配置。
- QQBot 提醒需要启用当前机器人，并完成管理员 C2C 绑定。
- 池模式自定义上游必须提供 Bearer API Key 兼容的 `/v1/usage`；上游探测参数见 `account_capacity` 段。
- 列表预测算法不新增 YAML/环境变量；模式和固定成本存放在分组数据库字段中。
- 部署时应应用当前版本随附的全部数据库迁移，包括 `194_group_predicted_balance_alert.sql` 和 `196_add_group_capacity_prediction_mode.sql`。
