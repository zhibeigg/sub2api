# 池模式账户容量预测提醒

## 目标

当一次请求完成成功计费后，如果**最终实际命中的账户**启用了 `pool_mode`，且**最终计费分组**启用了 `pool_capacity_alert_enabled`，系统会根据近期成功落账请求的平均成本估算该计费上下文还能支持多少次请求。

当最小预计剩余请求数严格小于 `50` 时，系统创建持久化告警事件，并通过管理员主邮箱和当前 QQBot 的已验证 C2C 绑定主动通知管理员。

> 该结果是基于历史均值的预测，不是上游实时余额保证。恰好剩余 `50` 次不会告警。

## 生效条件

必须同时满足：

1. 本次请求已由统一计费仓储成功提交，且幂等结果为 `Applied=true`。
2. 本次最终实际使用的账户 `Account.IsPoolMode()` 为 `true`。
3. 本次最终计费分组 `pool_capacity_alert_enabled=true`。
4. 请求不是 `cyber` 拒绝记录，且本次用户实际成本和账户成本均大于 `0`。
5. 最终分组、API Key、账户和用户信息完整。
6. 已取得该分组此前 `49` 条符合条件的成功落账样本，本次成本作为第 `50` 个样本。

如果请求在故障转移后使用了其他账户或分组，只以最终成功计费的账户和分组为准；中途尝试过但未最终计费的账户不参与判断。

## 样本口径

历史样本来自 `usage_logs`，按 `created_at DESC, id DESC` 取最终分组最近记录，并满足：

- `actual_cost > 0`；
- `request_type <> cyber`；
- 账户成本大于 `0`；
- 对应 `(request_id, api_key_id)` 已存在于 `usage_billing_dedup` 或归档表；
- 排除本次 `(request_id, api_key_id)`，避免 usage log 已提前可见时重复计算。

由于 usage log 通过异步 worker 批量落库，评估查询只读取此前最多 `49` 条，本次计费成本直接从成功计费结果加入，组成固定 `50` 条样本。

平均值：

- `avg_account_cost`：账户统计成本平均值，使用 `account_stats_cost`（存在时）或 `total_cost`，再乘账户倍率。
- `avg_actual_cost`：用户实际扣费平均值，即 `actual_cost`。

少于 `49` 条历史样本时不进行预测，也不触发告警。

## 容量计算

系统使用计费事务返回的**扣费后状态**，避免重新查询产生竞态：

- 账户容量：总额度、日额度、周额度中所有已配置维度的 `floor(remaining / avg_account_cost)` 最小值。
- API Key 容量：配置了 Key 配额时，使用 `floor((quota - quota_used) / avg_actual_cost)`。
- 用户容量：余额计费时使用扣费后余额，计算 `floor(balance / avg_actual_cost)`；订阅计费不使用用户钱包作为瓶颈。

最终预计剩余请求数为所有有限容量中的最小值。未配置限额的维度视为无限，不参与取最小值。

告警条件：

```text
predicted_requests < 50
```

因此：

- `49`：告警；
- `50`：不告警；
- `51`：不告警。

## 持久化状态机

数据库迁移：

- `190_add_group_pool_capacity_alert.sql`
- `191_pool_capacity_alert_runtime.sql`
- `192_add_usage_logs_pool_capacity_samples_index_notx.sql`（并发构建热表索引，不阻塞 usage log 写入）

运行时表：

- `pool_capacity_alert_states`：按分组 generation、账户、API Key、用户、计费类型保存 `healthy/low` 状态和 episode。
- `pool_capacity_alert_events`：保存每次告警 episode 的不可变预测快照。
- `pool_capacity_alert_deliveries`：保存 recipient/channel 级投递、租约、尝试次数、重试和最终状态。

状态范围：

```text
group_id + group_generation + account_id + api_key_id + user_id + billing_type
```

行为：

- `healthy -> low`：立即创建新 episode。
- 持续 `low`：达到 `reminder_cooldown_hours` 后创建提醒 episode。
- `low -> healthy`：恢复健康状态；下次再次低于阈值时创建新 episode。
- 分组开关发生实际变化时，`pool_capacity_alert_generation` 自增。
- 队列中的旧 generation 任务会被丢弃；尚未完成的旧 generation delivery 会被取消。
- 复制分组时该开关强制关闭，generation 重置为 `0`。

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
  "pool_capacity_alert_enabled": true
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

- 创建时未提供该字段，默认 `false`。
- 更新时只有显式提供字段才会处理。
- 仅当值实际变化时内部 generation 自增。
- `pool_capacity_alert_generation` 是内部一致性字段，不通过管理员或用户 DTO 暴露。
- 普通用户可见的分组响应不暴露告警开关。

## 部署配置

```yaml
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

- `enabled` 是全局运行时开关；每个分组仍默认关闭。
- 样本数 `50` 和告警阈值 `50` 是产品固定值，不通过 YAML 修改。
- `lease_seconds` 会被规范化为不小于 `send_timeout_seconds + 30`。
- worker、队列、超时和重试参数在加载配置时会限制到安全范围。

## 运维检查

1. 应用迁移 `190`、`191`、`192`；其中 `192` 必须按非事务迁移执行。
2. 确认 SMTP 已配置；否则邮件 delivery 会按策略重试或进入 dead。
3. 如需 QQ 提醒，确认 QQBot 已启用且管理员完成当前机器人 C2C 绑定。
4. 在管理端分组创建/编辑弹窗中显式开启容量提醒。
5. 使用测试池账户产生至少 50 次成功落账请求。
6. 验证恰好预计 50 次不创建事件，预计 49 次创建事件与两类 delivery。
7. 关闭分组开关，确认旧 generation 的待投递任务被取消。
