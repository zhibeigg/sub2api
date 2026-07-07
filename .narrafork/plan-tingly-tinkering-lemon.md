# 创建密钥支持多分组 + 优先级顺序 + 分组可用模型展示（方案 C：真多分组）

## 诚实的范围与风险声明（先读）

这是一次**触及计费核心的后端改造**。sub2api 全链路（认证 / 网关调度 / 计费归属 / 订阅校验）都假设「一个密钥 = 一个分组」。为把风险降到最低，本方案采用**加法式、向后兼容**设计：

- **不删除、不修改** `api_keys.group_id` 旧字段与旧逻辑。
- 新增 `api_key_groups` 关联表（key_id + group_id + priority）。
- **兼容规则**：密钥有多分组绑定 → 走新的按优先级调度；无多分组绑定 → 完全走旧的单 `group_id` 逻辑（存量密钥零影响）。
- 计费、订阅、限额都归属到**实际服务请求的那个分组**（和现在单分组行为一致）。

风险点（会重点测试）：调度选账号、计费 group 归属、订阅分组校验、粘性会话。若任一验证不通过，我会停下来向你报告，不强行上线。

---

## 后端改动（Go + ent + Atlas 迁移）

### 1. 数据模型
- 新增 ent schema `ent/schema/api_key_group.go`：`APIKeyGroup{ api_key_id, group_id, priority, created_at }`，联合主键 `(api_key_id, group_id)`，索引 `(api_key_id, priority)`。
- `ent/schema/api_key.go` 加 edge `groups`（多对多 through api_key_groups）。
- `go generate ./ent`（ent 代码生成）+ 生成 Atlas 迁移 SQL。旧 `group_id` 保留不动。

### 2. 领域/服务层
- `service.APIKey` 增加 `GroupBindings []APIKeyGroupBinding{ GroupID, Priority, Group *Group }`（按 priority 升序）。旧 `GroupID *int64` 保留：多绑定时取优先级最高的那个填入，保证旧代码路径不崩。
- `APIKeyService`：创建/更新密钥时写入 `api_key_groups`；读取时预加载绑定（带 Group 详情）。
- 认证中间件：把「主分组」（优先级最高）设入 context（保持 `setGroupContext` 现有契约），同时把有序 `GroupBindings` 存入 context 供调度用。

### 3. 网关调度（核心，最小改动）
- 在选账号入口 `SelectAccountForModelWithExclusions` 外面包一层：
  - 若 context 有多分组绑定 → **按优先级依次尝试**每个分组的现有单分组选账号逻辑，第一个成功返回的分组即为服务分组；全部失败则返回最后一个错误。
  - 若无多分组 → 原样走旧逻辑。
- 计费/日志的 `group_id` 用**实际命中的服务分组**。粘性会话 key 也用服务分组 id（与现状一致）。
- 订阅校验：命中订阅型分组时按该分组校验（沿用 `ValidateAndCheckLimits`）。

### 4. 接口层（DTO）
- `CreateApiKeyRequest` / `UpdateApiKeyRequest` 增加 `group_ids: []{ group_id, priority }`（或有序数组）。保留 `group_id` 单字段兼容旧客户端：只传 `group_id` 时等价于单绑定。
- Key 响应 DTO 增加 `groups: [{ id, name, platform, rate_multiplier, priority, ... }]`。
- 新增只读接口 `GET /groups/:id/models`（或复用已有 `/channels/available` 的模型数据）返回某分组可调用模型列表，供前端展示"可调用模型 20 个"。

> 说明：模型列表数据来源已存在——`/channels/available` 已返回每个分组平台的 `supported_models`。前端可直接用它按 group 聚合，**优先零新增后端接口**；仅当聚合不足时才加 `/groups/:id/models`。

### 5. Go 测试
- 新增：多分组按优先级调度（第一个无可用账号→回退到第二个）、计费归属到服务分组、单绑定向后兼容、订阅分组命中。
- 跑通受影响的现有测试（auth / gateway / billing / api_key）。

---

## 前端改动（Vue）

重写 `KeysView.vue` 的创建/编辑密钥弹窗分组区，对齐截图交互：

1. **供应商 chips**：全部 + 各平台，点选过滤下方"选择分组"下拉。
2. **多选分组**：从下拉添加分组到"已选列表"。
3. **优先级排序**：已选分组可**拖拽排序**（用项目已有的 `vue-draggable-plus` 依赖），序号 1/2/3… 表示调用优先级；每项显示分组名 + 倍率 + 删除按钮。
4. **可调用模型展示**：每个已选分组下方展示其可调用模型 chips（"可调用模型 N 个"），数据来自 `/channels/available` 聚合。
5. 底部提示"可选择多个分组，按优先级顺序调用"。
6. 提交：`group_ids: [{group_id, priority}]`；编辑时回填已有绑定并保持顺序。
7. 校验：至少选一个分组。

涉及文件：`views/user/KeysView.vue`、`api/keys.ts`（类型 + 请求）、`types/index.ts`（ApiKey 增 groups）、`i18n zh/en`（新文案）。可能抽 `components/keys/GroupMultiSelect.vue` 组件。

---

## 验证与交付

1. 后端：`go build -tags embed` + 新增/受影响单测通过。
2. 前端：`pnpm typecheck` + `lint:check` + `build` 通过。
3. **本地 dev + mock** 验证弹窗：供应商筛选、多选、拖拽排序、模型展示、提交 payload 正确。
4. 服务器：**先在数据库做迁移演练**（`BEGIN...ROLLBACK` 或影子库）确认 Atlas 迁移无损；再重建 `sub2api:custom` 镜像；容器启动时自动跑迁移。
5. **端到端真实验证**（关键）：建一个多分组密钥 → 用它调 `/v1/chat/completions` → 确认按优先级命中、计费 group 正确、切到备用分组也能通。存量单分组密钥回归验证不受影响。
6. 全程 commit 分步提交；更新 API 文档 / README / 示例配置 / 语言文件（按项目规则）。

## 回滚预案
- 迁移为纯加法（新表 + 旧字段保留），如需回滚：下线新镜像换回旧镜像即可，`api_key_groups` 表留着不影响旧逻辑。
- 若端到端计费验证异常，立即回滚并保留现场数据排查，不带病上线。

## 需要你在实施中拍板的点
- 端到端验证要用真实密钥调用（会消耗额度）——我会建一个测试密钥测完即删，或用你指定的密钥。
- 多分组里若混了不同平台（如 OpenAI + Anthropic），调度按优先级跨平台回退是否是你要的语义（截图 GPT+国模就是跨平台）——默认按"优先级顺序，谁先有可用账号谁服务"，实施时以此为准。
