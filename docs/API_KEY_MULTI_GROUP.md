# API Key 多分组与显式路由

Sub2API API Key 可以按优先级绑定多个分组。优先级数字越小越先尝试；兼容字段 `group_id` 始终指向最高优先级绑定，但完整配置以 `group_bindings` 为准。

## 创建与更新

创建或更新用户 API Key 时可以提交：

```json
{
  "name": "mixed-models",
  "group_id": 12,
  "group_bindings": [
    { "group_id": 12, "priority": 0 },
    { "group_id": 27, "priority": 1 },
    { "group_id": 35, "priority": 2 }
  ]
}
```

服务端会验证每个分组是否存在、是否可用，以及密钥所有者是否有权绑定该分组，并按 `priority`、`group_id` 稳定排序和去重。保存后 `group_id` 会同步为第一项的分组 ID。

更新语义：

- 未传 `group_bindings`：保持现有多分组绑定不变；
- 传非空数组：整体替换绑定和优先级；
- 传空数组：清空多分组绑定；
- 旧客户端只传 `group_id`：切换为单分组兼容模式，并清除旧的多分组绑定。

API Key 查询响应会返回有序绑定及分组概要：

```json
{
  "group_id": 12,
  "group_bindings": [
    {
      "group_id": 12,
      "priority": 0,
      "group": {
        "id": 12,
        "name": "Chinese Models",
        "platform": "openai",
        "rate_multiplier": 0.4
      }
    }
  ]
}
```

## 显式选择已绑定分组

兼容网关请求可以携带固定请求头：

```http
X-Sub2API-Group-ID: 27
```

服务端会在分组状态、用户权限、订阅、余额、额度和路由判断之前选择该绑定分组。因此路由平台、实际账号调度、订阅消耗、倍率计费和用量记录都会使用同一个分组。

约束：

- 该 ID 必须是 API Key 的一个绑定分组，或等于旧单分组 Key 的 `group_id`；
- 非法数字返回 `400 INVALID_GROUP_SELECTOR`；
- 未绑定的分组返回 `403 GROUP_NOT_BOUND_TO_API_KEY`；
- 已删除、停用或失去权限的分组仍按现有分组错误返回；
- 未发送请求头时保持向后兼容：按绑定优先级选择第一个能服务当前模型的分组；若都不可用，则沿最高优先级分组返回既有错误。

请求示例：

```bash
curl --fail-with-body https://your-domain.example/v1/chat/completions \
  -H 'Authorization: Bearer sk-your-key' \
  -H 'X-Sub2API-Group-ID: 27' \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "claude-sonnet-4-6",
    "messages": [{"role":"user","content":"hello"}],
    "stream": false
  }'
```

## 练习场模型选项

登录用户可以按 API Key ID 查询练习场模型选项：

```http
GET /api/v1/playground/api-keys/{id}/model-options
Authorization: Bearer <session JWT>
```

该接口只允许密钥所有者访问，不返回 API Key 明文或上游账号信息。响应按绑定优先级聚合当前可路由模型，并为每个选项返回稳定复合 ID、分组、平台和能力：

```json
[
  {
    "id": "27::claude-sonnet-4-6",
    "model": "claude-sonnet-4-6",
    "group_id": 27,
    "group_name": "Claude",
    "group_priority": 1,
    "platform": "anthropic",
    "capabilities": ["chat"]
  }
]
```

同名模型位于不同分组时会保留为不同选项。练习场的 Chat、Compare、Image 与 Video 只展示当前模式可调用的选项，并在实际请求及视频状态轮询中自动携带对应的 `X-Sub2API-Group-ID`。

## 多端点与浏览器测速

API 密钥页会同时展示：

- 管理员设置的主 API 地址；
- `custom_endpoints` 中最多 10 个自定义入口；
- 内置备用入口 `https://www.pokeapi.top`（若已配置相同地址则自动去重）。

页面由当前用户的浏览器直接请求各端点 origin 下的 `/health`，显示从该用户网络出发的实时延迟、连接状态和最近测速时间。测速请求不携带登录凭据或 API Key，不经过 Sub2API 服务端代理，也不会上传或持久化测速结果。用户可点击“重新测速”刷新全部节点数据，点击端点可复制完整地址；鼠标悬浮、键盘聚焦或触摸聚焦时可查看节点名称、管理员说明、协议和详细状态。

管理员仍在 **设置 → 站点设置 → 自定义端点** 中维护名称、地址和说明，无需新增配置字段。推荐将说明写成对用户有帮助的网络信息，例如适用区域、运营商或备用线路用途，避免承诺无法持续保证的固定延迟。

## 安全与兼容边界

- `X-Sub2API-Group-ID` 只能在当前密钥绑定范围内选择，不提供任意分组访问能力；
- 请求头名称固定，不允许通过配置修改；
- `/v1/models` 等公开兼容端点的既有响应格式不变；
- 普通客户端无需使用新请求头，原单分组行为保持不变；
- 分组选择失败不会静默回退到其他分组，避免界面选择、计费归属和实际路由不一致。
