# 管理员用户分组限制

本文档定义用户级分组访问控制、管理员 API、API Key 路由、错误码、缓存失效和向后兼容契约。

## 适用范围

该能力**只限制 `standard` 分组**，并把四类配置保持为独立概念：

| 配置 | 含义 | 不负责 |
| --- | --- | --- |
| `access_mode` | 是否启用用户级标准分组 allowlist | 不授予专属分组，不管理订阅，不设置倍率 |
| `restricted_group_ids` | restricted 模式下允许使用的 standard 分组 ID | 不是 denylist，不授予订阅分组 |
| `exclusive_group_ids` | 现有 exclusive standard 分组授权 | 不能绕过 restricted allowlist |
| `group_rates` | 用户在指定分组上的专属倍率 | 不能授予访问权限 |

订阅分组继续由订阅计划、有效期、状态和额度管理，不受 `access_mode` 或 `restricted_group_ids` 影响。

## 访问模式

### `inherit`

保持升级前行为：

- 普通 standard 分组继续按公开分组规则可用；
- exclusive standard 分组仍要求存在于 `exclusive_group_ids` 或由既有有效授权来源授予；
- subscription 分组仍要求有效订阅；
- `restricted_group_ids` 可以保留，但在 inherit 模式下不参与判定。

### `restricted`

只有 `restricted_group_ids` 中的 standard 分组可以通过用户级限制门禁。

> `access_mode=restricted` 且 `restricted_group_ids=[]` 表示**禁止全部标准分组**，不是继承默认值。恢复默认行为必须把 `access_mode` 改回 `inherit`。

## 最终授权规则

| 分组类型 | 用户级标准限制 | 其他必要授权 | 最终结果 |
| --- | --- | --- | --- |
| 普通 standard | inherit 全部通过；restricted 仅 allowlist | 分组状态、端点、模型/媒体能力、余额/额度等既有检查 | 全部检查通过才可用 |
| exclusive standard | inherit 全部通过；restricted 仅 allowlist | 还必须有 `exclusive_group_ids` 授权或既有有效授权来源 | 两个授权门禁取交集 |
| subscription | 不检查 `access_mode` / `restricted_group_ids` | 有效订阅、套餐状态和订阅额度 | 只按订阅规则 |

例如，exclusive standard 分组 27 同时出现在两个列表时才表示：

```json
{
  "access_mode": "restricted",
  "restricted_group_ids": [12, 27],
  "exclusive_group_ids": [27]
}
```

- 分组 12：通过普通 standard allowlist；
- 分组 27：同时通过 standard allowlist 和 exclusive 授权；
- 其他 standard 分组：被拒绝；
- subscription 分组：继续按订阅判断。

## 管理员 API

### 认证

两个接口均位于管理员路由下，复用现有管理员 JWT 或 Admin API Key 鉴权：

```http
Authorization: Bearer <admin-jwt>
```

或服务间管理员鉴权：

```http
x-api-key: admin-<64hex>
```

### 读取用户分组配置

```http
GET /api/v1/admin/users/:id/group-config
```

业务数据固定包含以下字段：

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

说明：

- 两个 ID 数组应由服务端去重并按 ID 稳定排序；
- GET 的 `group_rates` 只返回当前已配置的数值倍率，不返回 `null`；
- 历史用户没有显式限制配置时返回 `access_mode: "inherit"`；
- `restricted_group_ids` 在 inherit 模式下可保留，便于以后重新启用 restricted。

请求示例：

```bash
curl --fail-with-body \
  -H 'x-api-key: admin-<64hex>' \
  https://your-domain.example/api/v1/admin/users/42/group-config
```

### 更新用户分组配置

```http
PUT /api/v1/admin/users/:id/group-config
Content-Type: application/json
```

请求字段：

| 字段 | 类型 | 语义 |
| --- | --- | --- |
| `access_mode` | `inherit \| restricted` | 设置标准分组访问模式 |
| `restricted_group_ids` | `number[]` | 完整替换 standard allowlist；restricted 下空数组禁止全部 standard |
| `exclusive_group_ids` | `number[]` | 完整替换 existing exclusive standard 授权集合 |
| `group_rates` | `object` | 数字设置倍率；单个分组值为 `null` 时清除该分组倍率；未出现的倍率保持不变 |

完整示例：

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

该请求执行四项互相独立的操作：

1. 启用 standard allowlist；
2. 只允许 standard 分组 12、27；
3. 授予 exclusive standard 分组 27；
4. 把分组 12 的用户倍率设为 0.75，并清除分组 27 的用户倍率。

请求示例：

```bash
curl --fail-with-body \
  -X PUT \
  -H 'x-api-key: admin-<64hex>' \
  -H 'Content-Type: application/json' \
  https://your-domain.example/api/v1/admin/users/42/group-config \
  -d '{
    "access_mode": "restricted",
    "restricted_group_ids": [12, 27],
    "exclusive_group_ids": [27],
    "group_rates": {
      "12": 0.75,
      "27": null
    }
  }'
```

成功响应返回保存后的归一化四字段配置。权限集合与倍率更新应在同一数据库事务中完成；任何校验或持久化失败都不得暴露部分更新。

## 校验规则

- `access_mode` 只接受 `inherit`、`restricted`；
- `restricted_group_ids` 只接受存在的 standard 分组 ID；
- `exclusive_group_ids` 只接受存在的 exclusive standard 分组 ID；
- subscription 分组 ID 不能通过上述两个字段获得订阅权限；
- ID 必须为正整数，重复项会被去重；
- 倍率必须满足现有用户专属倍率范围和数值校验；
- `group_rates.<group_id>=null` 只清除对应倍率，不删除权限，也不修改其他倍率。

## API Key 行为

### 新建或更新绑定

用户创建或更新 API Key 时，目标 standard 分组必须通过当前用户分组资格检查。无权绑定时返回：

```text
HTTP 403
code: GROUP_NOT_ALLOWED
```

### 已有单分组 Key

管理员收紧权限后不会改写或删除 Key 的 `group_id`。下一次请求如果仍指向被限制 standard 分组，返回：

```json
{
  "code": "GROUP_NOT_ALLOWED",
  "message": "API Key 所属分组不允许当前用户使用"
}
```

HTTP 状态为 403，且请求不会进入账号选择、计费预扣或上游转发。

### 已有多分组 Key

未发送 `X-Sub2API-Group-ID` 时：

1. 按 `group_bindings` 优先级尝试；
2. 遇到被用户限制拒绝的 standard 分组时跳过；
3. 继续尝试下一个绑定；
4. 第一个同时满足访问权限、分组状态、端点/模型/媒体能力、订阅、余额和额度的分组成为实际路由与计费分组。

如果全部候选都因用户限制不可用，返回 `403 GROUP_NOT_ALLOWED`。

显式发送：

```http
X-Sub2API-Group-ID: 27
```

表示客户端要求使用该绑定分组。若分组 27 被用户限制拒绝，服务端返回 `403 GROUP_NOT_ALLOWED`，**不会**静默回退到其他绑定，从而保证界面选择、用量归属、倍率和实际路由一致。

## 倍率与计费

权限和倍率完全独立：

- 移出 `restricted_group_ids` 不删除 `group_rates`；
- 移出 `exclusive_group_ids` 不删除 `group_rates`；
- restricted 空数组不清空倍率；
- 订阅过期或恢复不清空倍率；
- 重新授权后，原用户倍率仍可继续生效；
- 只有 PUT 中显式提交 `null` 才清除对应用户倍率。

最终计费只使用**实际获准并被选中**的分组。存在用户倍率不能让无权限分组变为可用。

## 缓存立即失效

PUT 成功提交后，服务端必须立即：

1. 清除该用户全部 API Key 的进程内 L1 认证/授权快照；
2. 删除 Redis L2 认证缓存；
3. 向其他实例发布缓存失效通知；
4. 让其他实例清理本地快照。

因此后续新鉴权请求会读取最新配置，不需要等待 `api_key_auth_cache` 的普通 TTL。已经完成鉴权并进入处理流程的在途请求不强制中断。

若 Redis publish 失败，数据库更新仍保持已提交状态；服务端必须记录稳定错误并执行现有降级失效策略，不能把旧授权重新写回缓存。

## 错误码

| HTTP | code | 说明 |
| ---: | --- | --- |
| 400 | `INVALID_ACCESS_MODE` | `access_mode` 不是 inherit/restricted |
| 400 | `INVALID_RESTRICTED_GROUP` | restricted 列表含不存在或非 standard 分组 |
| 400 | `INVALID_EXCLUSIVE_GROUP` | exclusive 列表含不存在或非 exclusive standard 分组 |
| 400 | `INVALID_GROUP_RATE` | 分组 ID、倍率类型或倍率范围非法 |
| 403 | `GROUP_NOT_ALLOWED` | API Key 绑定或显式选择的 standard 分组不允许当前用户使用 |
| 404 | `USER_NOT_FOUND` | 用户不存在 |

校验错误示例：

```json
{
  "code": "INVALID_RESTRICTED_GROUP",
  "message": "restricted_group_ids contains a non-standard or unknown group"
}
```

实际响应外层 envelope 沿用 Sub2API 现有 API 规范；客户端应以 HTTP 状态和稳定 `code` 分支处理，不依赖完整文案。

## 向后兼容

- 历史用户和没有新配置记录的用户有效模式为 `inherit`；
- 现有普通 standard 分组行为不变；
- 现有 exclusive 授权继续保留并通过 `exclusive_group_ids` 暴露；
- 现有 subscription 分组和订阅套餐不受新 allowlist 影响；
- 现有单分组/多分组 API Key 不迁移、不删除、不重排；
- 现有 `group_rates` 不迁移语义、不自动清除；
- `/v1/models` 等兼容接口的既有 envelope 不变，只过滤当前用户实际无权使用的分组选项；
- 该能力由数据库中的管理员配置驱动，不需要也不允许新增 YAML 运行时配置键。

## 运维与审计建议

- 在把用户切换为 restricted 前，先读取其 API Key 绑定，确认至少保留一个可用 standard 分组；
- restricted 空数组适合明确暂停用户的全部余额分组访问，但不会暂停有效订阅；
- 管理操作审计应记录管理员、用户 ID、模式、两个集合的数量/ID 摘要、倍率变更分组和 request ID；
- 审计记录不得包含 API Key 明文、上游凭据或请求正文；
- 权限更新后可用一个存量 Key 分别验证单分组 403、多分组跳过和显式选择不回退。
