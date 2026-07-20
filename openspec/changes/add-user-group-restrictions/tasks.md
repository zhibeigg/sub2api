## 1. 数据模型与统一授权契约

- [ ] 1.1 增加用户 `access_mode` 与 `restricted_group_ids` 持久化，历史数据默认归一为 `inherit`
- [ ] 1.2 为现有专属标准分组授权提供稳定 `exclusive_group_ids` 读写映射，不改变底层授权语义
- [ ] 1.3 定义统一 `CanUserAccessGroup` 判定，覆盖普通 standard、exclusive standard 和 subscription 三类分组
- [ ] 1.4 固定 restricted 空数组禁止全部 standard、专属标准分组双门禁、订阅分组绕过限制的单元测试
- [ ] 1.5 保证权限变更不新增、删除或修改任何 `group_rates` 记录

## 2. 管理员 API

- [ ] 2.1 注册 `GET /api/v1/admin/users/:id/group-config` 并返回固定四字段 DTO
- [ ] 2.2 注册 `PUT /api/v1/admin/users/:id/group-config`，完整替换两个分组集合并校验 `inherit|restricted`
- [ ] 2.3 校验 restricted 仅含 standard、exclusive 仅含 exclusive standard，去重并稳定排序
- [ ] 2.4 实现 `group_rates` 数字设置、单项 `null` 清除、未提交项保持不变
- [ ] 2.5 在单事务中保存权限和倍率更新，补充失败回滚测试
- [ ] 2.6 复用管理员鉴权、响应 envelope 和操作审计，审计信息不得包含 API Key 明文
- [ ] 2.7 覆盖 `INVALID_ACCESS_MODE`、`INVALID_RESTRICTED_GROUP`、`INVALID_EXCLUSIVE_GROUP`、`INVALID_GROUP_RATE`、`USER_NOT_FOUND`

## 3. 缓存立即失效

- [ ] 3.1 PUT 提交成功后失效该用户全部 API Key 的进程内 L1 认证缓存
- [ ] 3.2 删除该用户全部 API Key 的 Redis L2 认证缓存并发布跨实例失效通知
- [ ] 3.3 确保事务失败不失效缓存，提交成功后新请求不能继续写回旧授权快照
- [ ] 3.4 增加双实例失效、Redis publish 失败和存量缓存命中回归测试

## 4. API Key 与网关路由

- [ ] 4.1 API Key 创建/更新绑定统一使用新用户分组资格判定，拒绝非法标准分组并返回 `GROUP_NOT_ALLOWED`
- [ ] 4.2 存量单分组 Key 指向被限制标准分组时返回 `403 GROUP_NOT_ALLOWED`
- [ ] 4.3 多分组自动路由按优先级跳过被限制分组并尝试下一个候选
- [ ] 4.4 多分组所有候选均被限制时返回 `403 GROUP_NOT_ALLOWED`
- [ ] 4.5 `X-Sub2API-Group-ID` 显式选择被限制分组时返回 403 且不回退
- [ ] 4.6 保持订阅、余额、额度、分组状态、端点/模型/媒体能力和错误优先级的既有检查
- [ ] 4.7 证明最终计费、用量和倍率只使用实际获准并选中的分组

## 5. 目录、练习场与控制台

- [ ] 5.1 模型目录和 API Key 可用分组列表过滤或标记无权限标准分组
- [ ] 5.2 练习场模型选项与正式网关使用同一分组资格判定
- [ ] 5.3 管理员用户分组界面分开展示标准限制、专属授权和用户倍率
- [ ] 5.4 restricted 空数组保存前显示“禁止全部标准分组”明确确认
- [ ] 5.5 订阅分组在界面中只读展示并引导到订阅管理，不允许通过 restricted/exclusive 列表授权

## 6. 兼容性、文档与验证

- [ ] 6.1 回归历史无配置用户、现有专属授权、有效订阅、单分组 Key、多分组 Key 和用户倍率
- [ ] 6.2 更新管理员/API、多分组、README、docs-site 指南/导航和示例配置说明
- [ ] 6.3 运行 `openspec validate add-user-group-restrictions --type change --strict --no-interactive`
- [ ] 6.4 运行后端授权、API Key、中间件、管理 API 和缓存测试
- [ ] 6.5 运行前端 lint、typecheck、相关组件测试和 docs-site build
- [ ] 6.6 验证没有新增无效 YAML 配置键，文档示例不包含真实凭据
