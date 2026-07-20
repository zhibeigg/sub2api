## ADDED Requirements

### Requirement: 用户标准分组访问必须使用显式模式
系统 SHALL 为每个用户维护 `access_mode=inherit|restricted` 和独立 `restricted_group_ids`。`restricted_group_ids` MUST 仅在 restricted 模式下作为 standard 分组 allowlist，不得解释为 denylist。

#### Scenario: 历史用户没有限制配置
- **WHEN** 用户没有显式保存过标准分组限制配置
- **THEN** 系统 MUST 将其有效 `access_mode` 视为 `inherit`
- **THEN** 该用户的 standard 分组行为 MUST 与升级前一致

#### Scenario: restricted 列表包含分组
- **WHEN** 用户的 `access_mode=restricted` 且 `restricted_group_ids=[12,27]`
- **THEN** standard 分组 12 和 27 MUST 通过标准分组限制门禁
- **THEN** 其他 standard 分组 MUST 被该门禁拒绝

#### Scenario: restricted 列表为空
- **WHEN** 用户的 `access_mode=restricted` 且 `restricted_group_ids=[]`
- **THEN** 系统 MUST 禁止该用户使用全部 standard 分组
- **THEN** 系统 MUST NOT 把空数组解释为继承或允许全部

### Requirement: 标准限制与专属授权必须独立并取交集
系统 SHALL 使用独立 `exclusive_group_ids` 表达 existing exclusive standard group authorization。对于 exclusive standard 分组，用户 MUST 同时通过标准分组限制门禁和专属授权门禁。

#### Scenario: inherit 用户访问已授权专属标准分组
- **WHEN** `access_mode=inherit` 且目标 exclusive standard 分组位于 `exclusive_group_ids`
- **THEN** 标准限制门禁 MUST 通过
- **THEN** 系统 MUST 按既有专属分组规则允许后续资格检查

#### Scenario: restricted allowlist 缺少已授权专属分组
- **WHEN** 目标 exclusive standard 分组位于 `exclusive_group_ids` 但不位于 `restricted_group_ids`
- **THEN** 系统 MUST 因标准分组限制拒绝该分组
- **THEN** 系统 MUST NOT 因已有专属授权绕过 restricted allowlist

#### Scenario: allowlist 包含但未授予专属分组
- **WHEN** 目标 exclusive standard 分组位于 `restricted_group_ids` 但不位于 `exclusive_group_ids` 且没有其他既有有效授权来源
- **THEN** 系统 MUST 因缺少专属授权拒绝该分组
- **THEN** 系统 MUST NOT 把 restricted allowlist 当作专属授权

### Requirement: 订阅分组必须继续由订阅管理
系统 SHALL 只对 standard 分组应用 `access_mode` 和 `restricted_group_ids`。subscription 分组 MUST 继续由有效订阅、套餐状态和订阅额度规则授权。

#### Scenario: restricted 空列表但存在有效订阅
- **WHEN** 用户禁止全部 standard 分组但拥有某 subscription 分组的有效订阅
- **THEN** 该 subscription 分组 MUST NOT 因 restricted 空列表被拒绝
- **THEN** 系统 MUST 继续执行既有订阅状态和额度检查

#### Scenario: restricted 列表包含订阅分组 ID
- **WHEN** 管理员尝试把 subscription 分组 ID 写入 `restricted_group_ids`
- **THEN** PUT MUST 返回 `400 INVALID_RESTRICTED_GROUP`
- **THEN** 系统 MUST NOT 因该 ID 出现在列表中授予订阅权限

### Requirement: 用户分组倍率必须与访问权限独立
系统 SHALL 独立保留 `group_rates`。修改 `access_mode`、`restricted_group_ids` 或 `exclusive_group_ids` MUST NOT 自动新增、删除、覆盖或重新计算用户分组倍率。

#### Scenario: 收紧权限后保留倍率
- **WHEN** 管理员移除某 standard 分组访问权限且该用户存在该分组的专属倍率
- **THEN** 倍率配置 MUST 继续保留
- **THEN** 无权限期间系统 MUST NOT 因存在倍率而允许访问

#### Scenario: 显式清除倍率
- **WHEN** 管理员在 PUT 的 `group_rates` 中为某分组提交 `null`
- **THEN** 系统 MUST 清除该分组的用户专属倍率
- **THEN** 其他未出现在 `group_rates` 中的倍率 MUST 保持不变

### Requirement: 管理员必须通过专用 API 管理用户分组配置
系统 SHALL 提供 `GET /api/v1/admin/users/:id/group-config` 和 `PUT /api/v1/admin/users/:id/group-config`。业务数据字段 MUST 固定为 `{access_mode, restricted_group_ids, exclusive_group_ids, group_rates}`。

#### Scenario: 管理员读取配置
- **WHEN** 已认证管理员读取存在用户的 group-config
- **THEN** 响应 MUST 返回归一化 access_mode、稳定排序且去重的两个 ID 数组和已配置数值倍率
- **THEN** GET 的 `group_rates` MUST NOT 返回 `null` 项

#### Scenario: 管理员更新配置
- **WHEN** 管理员提交合法 access_mode、两个分组集合和倍率变更
- **THEN** 服务端 MUST 在同一事务内完成配置更新
- **THEN** `restricted_group_ids` 和 `exclusive_group_ids` MUST 分别完整替换对应集合
- **THEN** 数值倍率 MUST 被设置且 null 倍率 MUST 被清除

#### Scenario: 管理员提交非法模式
- **WHEN** `access_mode` 不是 inherit 或 restricted
- **THEN** API MUST 返回 `400 INVALID_ACCESS_MODE`
- **THEN** 任何权限或倍率配置 MUST NOT 改变

#### Scenario: 用户不存在
- **WHEN** group-config 路径中的用户 ID 不存在
- **THEN** API MUST 返回 `404 USER_NOT_FOUND`

### Requirement: 配置更新后认证缓存必须立即失效
系统 SHALL 在 group-config PUT 成功提交后立即失效该用户全部 API Key 的本地和 Redis 认证/授权缓存，并向其他实例传播失效通知。

#### Scenario: 更新已有缓存用户
- **WHEN** 管理员成功收紧一个已有活跃 API Key 的用户权限
- **THEN** 当前实例 MUST 清除该用户相关 L1 快照
- **THEN** Redis L2 快照 MUST 被删除
- **THEN** 其他实例 MUST 收到失效通知并清除本地快照
- **THEN** 后续新请求 MUST 使用更新后的权限

#### Scenario: 事务失败
- **WHEN** group-config 持久化事务失败
- **THEN** API MUST 返回失败
- **THEN** 系统 MUST NOT 暴露部分更新
- **THEN** 系统 MUST NOT 发送代表成功配置的缓存失效事件

### Requirement: 单分组 API Key 被限制时必须明确拒绝
存量或新建单分组 API Key 指向当前用户无权使用的 standard 分组时，系统 SHALL 返回 `403 GROUP_NOT_ALLOWED`，且 MUST NOT 选择账号、预扣计费或请求上游。

#### Scenario: 存量单分组 Key 权限被撤销
- **WHEN** 管理员把该 Key 的 standard 分组移出用户 restricted allowlist
- **THEN** 下一次使用该 Key 的请求 MUST 返回 403
- **THEN** 错误 code MUST 为 `GROUP_NOT_ALLOWED`
- **THEN** Key 的持久化 `group_id` MUST 保持不变

### Requirement: 多分组 API Key 必须跳过受限候选
多分组 API Key 未显式选择分组时，系统 SHALL 按绑定优先级跳过被用户分组限制拒绝的候选并继续尝试下一个绑定。

#### Scenario: 第一候选被限制而第二候选允许
- **WHEN** 第一优先级 standard 分组不允许当前用户且第二优先级分组允许并满足其他路由条件
- **THEN** 系统 MUST NOT 调度或计费第一分组
- **THEN** 系统 MUST 使用第二分组继续请求

#### Scenario: 所有候选都被限制
- **WHEN** 多分组 Key 的全部候选均因用户标准分组限制不可用
- **THEN** 系统 MUST 返回 `403 GROUP_NOT_ALLOWED`
- **THEN** 系统 MUST NOT 请求任何上游

#### Scenario: 显式选择被限制分组
- **WHEN** 客户端用 `X-Sub2API-Group-ID` 选择一个已绑定但当前不允许的 standard 分组
- **THEN** 系统 MUST 返回 `403 GROUP_NOT_ALLOWED`
- **THEN** 系统 MUST NOT 静默回退到其他绑定

### Requirement: 分组目录与实际路由必须使用同一权限判定
API Key 可用分组、模型目录、练习场模型选项和正式网关 SHALL 使用同一用户分组资格规则。展示层 MUST NOT 把仅有倍率但无访问权限的 standard 分组表示为可调用。

#### Scenario: 练习场加载多分组模型
- **WHEN** 某绑定 standard 分组已被用户限制拒绝
- **THEN** 练习场 MUST 过滤该分组模型选项或明确标记不可用
- **THEN** 从其他允许分组生成的模型选项 MUST 继续可用

### Requirement: 升级必须保持向后兼容
系统 SHALL 保持历史用户、现有专属授权、订阅分组、API Key 绑定和用户倍率数据的语义。启用本能力 MUST NOT 要求新增 YAML 配置键。

#### Scenario: 升级未配置限制
- **WHEN** 部署升级但管理员未修改任何用户 group-config
- **THEN** 所有用户 MUST 继续按 inherit 行为访问 standard 分组
- **THEN** 现有 exclusive 授权、有效订阅和 group_rates MUST 继续生效

#### Scenario: 管理员恢复继承模式
- **WHEN** 管理员把 restricted 用户改回 `access_mode=inherit`
- **THEN** `restricted_group_ids` MAY 保留以便后续恢复
- **THEN** 当前 standard 分组访问 MUST 不再受该列表限制
- **THEN** exclusive 和 subscription 的既有授权门禁 MUST 继续执行
