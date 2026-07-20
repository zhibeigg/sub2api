## Why

当前用户分组授权把“专属分组授权”与普通标准分组的默认可见性混在既有 `allowed_groups` 语义中：管理员可以授予用户专属分组，但不能明确限制某个用户只能使用一部分标准分组。直接复用专属分组列表会破坏公开标准分组默认可用、订阅分组由订阅管理、用户专属倍率独立计算等既有规则，也无法清楚表达“禁止全部标准分组”。

本变更引入独立的用户标准分组访问模式和 allowlist，并提供专用管理员 API。升级后没有新配置记录的用户继续继承现有行为；管理员显式切换为 restricted 后，限制才生效。

## What Changes

- 为用户增加 `access_mode=inherit|restricted` 标准分组访问模式。
- 增加独立 `restricted_group_ids` allowlist；`restricted` + 空数组明确表示禁止该用户使用全部标准分组。
- 保留并明确现有专属分组授权为 `exclusive_group_ids`；标准专属分组必须同时通过标准分组限制和专属授权，两套列表互不替代。
- 订阅分组继续只由有效订阅和订阅套餐管理，不受 `access_mode` 或 `restricted_group_ids` 影响。
- 保留独立 `group_rates` 用户专属倍率；访问权限变化不得删除、重算或合并倍率配置。
- 新增管理员 API：`GET /api/v1/admin/users/:id/group-config` 与 `PUT /api/v1/admin/users/:id/group-config`，字段固定为 `{access_mode, restricted_group_ids, exclusive_group_ids, group_rates}`；`group_rates` 中的 `null` 用于清除对应分组倍率。
- 已有单分组 API Key 指向被限制标准分组时返回 `403 GROUP_NOT_ALLOWED`。
- 已有多分组 API Key 自动路由时跳过被限制的标准分组并尝试下一个绑定；显式选择被限制分组时返回 `403 GROUP_NOT_ALLOWED`，不静默回退。
- 成功保存用户分组配置后立即使该用户全部 API Key 的认证/授权缓存失效，并向其他实例发布失效通知。
- 补充管理员操作说明、API 示例、错误码、向后兼容和多分组路由文档。

## Capabilities

### New Capabilities

- `user-group-access`: 定义用户标准分组限制、专属分组授权、订阅分组边界、用户倍率独立性、管理员配置 API、缓存失效和 API Key 路由行为。

### Modified Capabilities

无。仓库当前没有已发布的用户分组访问 capability；本变更把现有行为作为兼容基线并新增独立能力。

## Impact

- **数据模型**：需要持久化 `access_mode` 与 `restricted_group_ids`，并把现有专属授权以 `exclusive_group_ids` DTO 暴露；不得把限制列表复用为倍率或订阅记录。
- **管理 API**：新增用户级 group-config 读取和更新端点，复用现有管理员鉴权、错误 envelope 与管理操作审计。
- **API Key 生命周期**：创建/更新绑定时校验新权限；存量 Key 不被自动改写，运行时按最新用户配置决定允许、跳过或拒绝。
- **网关与目录**：单分组、显式分组、多分组自动路由、模型目录和练习场选项必须使用同一用户分组判定。
- **缓存**：PUT 成功后必须立即失效该用户的 API Key L1/L2 缓存并跨实例通知；事务失败不得提前失效或暴露部分配置。
- **计费**：`group_rates`、分组模型倍率、媒体倍率和订阅额度计算保持独立，只有最终获准并实际选中的分组参与计费。
- **兼容性**：历史用户默认解析为 `inherit`；现有专属授权、订阅、单/多分组 Key 和倍率数据不迁移语义、不自动删除。
- **部署配置**：该能力由数据库中的管理员配置驱动，不新增 YAML 运行时配置键；示例配置只增加说明性注释。
