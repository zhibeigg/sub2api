## Context

Sub2API 当前的用户分组关系主要服务于专属分组授权：非专属标准分组默认可用，专属标准分组需要用户授权，订阅分组需要有效订阅。API Key 又支持单分组兼容字段和按优先级排列的多分组绑定，运行时还叠加分组状态、模型/媒体能力、订阅额度、余额与倍率计算。

本变更要增加“只允许某个用户访问指定标准分组”的管理员能力，同时不能把它误用为专属授权、订阅管理或倍率配置。限制更新还必须立即作用于存量 API Key，避免缓存窗口继续放行已撤销的标准分组。

## Goals / Non-Goals

**Goals:**

- 用明确的 `inherit|restricted` 模式表达标准分组默认继承或 allowlist 限制。
- 让空 allowlist 成为可配置、可测试的“禁止全部标准分组”。
- 保持标准分组限制、专属授权、订阅授权和用户倍率四个概念独立。
- 给单分组、多分组自动路由和显式分组选择提供一致、可预测的拒绝/跳过语义。
- 提供稳定管理员 API、校验、错误码、审计和立即缓存失效。
- 保持历史用户和没有显式限制的部署向后兼容。

**Non-Goals:**

- 不改变订阅套餐、订阅分组授权或订阅额度计算。
- 不修改 API Key 的 `group_id` / `group_bindings` 存储结构或自动重写存量 Key。
- 不把 `restricted_group_ids` 当作 denylist；它只在 restricted 模式下作为 allowlist。
- 不把 `exclusive_group_ids` 合并进 `restricted_group_ids`。
- 不改变 `group_rates`、分组模型倍率、媒体倍率的优先级或数值。
- 不增加 YAML 环境开关或其他运行时配置键。

## Decisions

### 1. 使用显式模式而不是依赖 null/空数组推断

配置结构：

```json
{
  "access_mode": "inherit",
  "restricted_group_ids": [],
  "exclusive_group_ids": [],
  "group_rates": {}
}
```

- `inherit`：标准分组限制门禁直接通过，继续使用升级前规则。
- `restricted`：只有 ID 位于 `restricted_group_ids` 的标准分组通过限制门禁。
- `restricted` 与空 `restricted_group_ids`：所有标准分组都被拒绝。

显式模式避免无法区分“尚未配置”和“管理员有意禁止全部标准分组”。历史无记录用户归一为 `inherit`。

### 2. 标准分组限制与专属授权取交集

最终分组资格按分组类型计算：

| 分组类型 | 标准限制门禁 | 额外授权门禁 | 最终规则 |
| --- | --- | --- | --- |
| 普通 standard | inherit 全部通过；restricted 仅 allowlist | 无 | 标准限制门禁 |
| exclusive standard | inherit 全部通过；restricted 仅 allowlist | 必须在 `exclusive_group_ids` 或由既有有效授权来源授予 | 两者都通过 |
| subscription | 不检查 | 必须有有效订阅/套餐授权 | 仅订阅规则 |

`restricted_group_ids` 与 `exclusive_group_ids` 可以包含相同 ID，但含义不同：前者回答“restricted 模式下是否允许这个标准分组”，后者回答“是否授予这个专属标准分组”。任何一方都不能隐式写入另一方。

### 3. 订阅分组完全绕过标准限制

订阅分组继续由订阅管理模块判断有效期、状态和额度。管理员把用户设为 `restricted` 或使用空 allowlist，不得撤销有效订阅分组，也不得把订阅分组 ID 写入 allowlist 后获得订阅权限。

保存 API 必须拒绝或规范化非 standard ID 出现在 `restricted_group_ids` 中；推荐返回校验错误，避免管理员误以为该字段能控制订阅。

### 4. 多分组自动路由可跳过，显式选择不可回退

统一候选流程：

1. 按 API Key 绑定优先级和 group ID 稳定排序。
2. 对每个候选先检查分组是否仍绑定。
3. 检查用户分组资格；被标准限制拒绝的候选记录为 `GROUP_NOT_ALLOWED` 并继续下一项。
4. 再检查分组状态、端点、模型、媒体能力、订阅、余额和额度等既有条件。
5. 第一个全部通过的候选成为实际路由和计费分组。

如果客户端发送 `X-Sub2API-Group-ID`，该请求表达明确选择：选中分组被限制时立即返回 `403 GROUP_NOT_ALLOWED`，不得切换到其他绑定。单分组 Key 同样返回该错误。

若多分组自动路由的所有候选都因用户限制被跳过，返回 `403 GROUP_NOT_ALLOWED`。若还存在其他类型失败，沿既有稳定错误优先级选择最具体原因，但不得把被限制分组实际调度或计费。

### 5. 不重写存量 API Key

管理员收紧用户权限后：

- 存量单分组 Key 保留原 `group_id`，后续请求按最新配置返回 403。
- 存量多分组 Key 保留全部绑定和优先级，被限制候选运行时跳过。
- 管理员重新放开权限后，原绑定无需重建即可恢复候选资格。

这让访问策略与 Key 配置解耦，也避免策略切换造成不可逆数据丢失。

### 6. 管理员 API 使用专用资源

端点：

```text
GET /api/v1/admin/users/:id/group-config
PUT /api/v1/admin/users/:id/group-config
```

GET 的业务数据固定包含：

```json
{
  "access_mode": "restricted",
  "restricted_group_ids": [12, 27],
  "exclusive_group_ids": [27],
  "group_rates": {
    "12": 0.8,
    "27": 0.6
  }
}
```

PUT 语义：

- `access_mode` 必须是 `inherit` 或 `restricted`。
- `restricted_group_ids` 和 `exclusive_group_ids` 是各自集合的完整替换值；服务端去重并按 ID 稳定排序。
- `restricted_group_ids` 只接受存在的 standard 分组。
- `exclusive_group_ids` 只接受存在的 exclusive standard 分组。
- `group_rates` 延续独立增量更新语义：数字设置倍率，单个值 `null` 清除对应分组倍率，未出现的分组倍率保持不变。
- GET 只返回已配置的数值倍率，不返回 `null` 项。

示例：

```json
{
  "access_mode": "restricted",
  "restricted_group_ids": [12, 27],
  "exclusive_group_ids": [27],
  "group_rates": {
    "12": 0.75,
    "27": null
  }
}
```

该请求允许标准分组 12、27，授予专属分组 27，把 12 的用户倍率设为 0.75，并清除 27 的用户倍率；权限与倍率操作互不推导。

### 7. 事务提交后立即失效缓存

PUT 的持久化步骤在一个数据库事务中完成。提交成功后：

1. 使当前实例中该用户全部 API Key 的 L1 认证快照失效。
2. 删除该用户全部 API Key 的 Redis L2 认证缓存。
3. 发布逐 Key 或用户级跨实例失效通知。
4. 其他实例收到通知后立即清理本地快照。

API 只有在持久化成功且本实例失效动作已发起后才返回成功。Redis publish 失败不能回滚已提交数据，但必须同步删除可访问的 L2、记录稳定错误并采用现有兜底机制；请求热路径不能继续主动写回旧快照。已经进入处理流程的请求不强制中断，后续新鉴权必须读取新配置。

### 8. 倍率保持独立

`group_rates` 只影响用户通过授权后实际使用分组的最终计费倍率：

- 收紧权限不删除倍率。
- 清空 restricted allowlist 不清空倍率。
- 移除 exclusive 授权不清空倍率。
- 订阅状态变化不清空倍率。
- 重新授权后原倍率仍可继续生效，除非管理员用 `null` 显式清除。

模型目录和练习场不得因为存在倍率配置就把无权限分组展示为可调用。

### 9. 错误码和审计保持稳定

最小错误集合：

| HTTP | code | 条件 |
| ---: | --- | --- |
| 400 | `INVALID_ACCESS_MODE` | access_mode 非法 |
| 400 | `INVALID_RESTRICTED_GROUP` | restricted 列表包含不存在或非 standard 分组 |
| 400 | `INVALID_EXCLUSIVE_GROUP` | exclusive 列表包含不存在或非 exclusive standard 分组 |
| 400 | `INVALID_GROUP_RATE` | 分组 ID 或倍率非法 |
| 403 | `GROUP_NOT_ALLOWED` | API Key 绑定/选择的标准分组不允许当前用户使用 |
| 404 | `USER_NOT_FOUND` | 用户不存在 |

管理员 PUT 纳入现有管理操作审计，只记录用户 ID、模式、集合数量/ID 摘要、倍率变更的分组 ID 和 request ID，不记录 API Key 明文或其他凭据。

## Risks / Trade-offs

- [管理员误把 restricted 空数组当成恢复默认] → UI/API 文档明确提示该组合会禁止全部标准分组，恢复默认必须设置 `inherit`。
- [两套 allowlist 容易混淆] → API 使用不同字段名，控制台分区展示，并在设计中固定取交集规则。
- [存量 Key 仍显示被限制绑定] → 保留配置便于重新授权；运行时、目录和练习场必须显示不可用原因或过滤，不得实际路由。
- [多实例缓存短暂不一致] → 提交后立即清理 L1/L2 并发布失效；补充跨实例测试和 publish 失败降级。
- [权限与倍率同请求更新造成耦合误解] → 持久层可以同事务更新，但业务规则禁止任何自动推导或清理。

## Migration Plan

1. 增加持久化字段/关系并把历史用户默认解释为 `inherit`。
2. 新增统一用户分组资格判定和单元测试，不先改变网关行为。
3. 接入 API Key 创建/更新、单分组鉴权、多分组候选、显式选择、模型目录和练习场。
4. 新增管理员 GET/PUT、审计与缓存失效。
5. 更新控制台和文档，明确空 allowlist、订阅边界和倍率独立性。
6. 以 inherit 作为所有历史用户默认值上线；管理员逐用户启用 restricted。

### 回滚

- 业务回滚优先把受影响用户设置为 `inherit`，保留 restricted 列表以便后续恢复。
- 代码回滚不得删除现有专属授权、订阅或倍率记录。
- 数据字段保留，不回退已应用 migration；旧版本应忽略新增字段并继续原行为。
