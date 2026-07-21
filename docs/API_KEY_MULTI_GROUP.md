# API Key 多分组与显式路由

Sub2API API Key 可以按优先级绑定多个分组。优先级数字越小越先尝试；兼容字段 `group_id` 始终指向最高优先级绑定，但完整配置以 `group_bindings` 为准。

用户级标准分组限制会在绑定和运行时选择阶段参与判定。管理员配置、API 字段、缓存失效与完整兼容契约见 [管理员用户分组限制](ADMIN_USER_GROUP_RESTRICTIONS.md)。

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

## 用户级标准分组限制

管理员可以通过 `GET/PUT /api/v1/admin/users/:id/group-config` 为用户配置：

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

固定语义：

- 该限制只作用于 `standard` 分组；`access_mode=inherit` 保持历史行为；
- `access_mode=restricted` 时，`restricted_group_ids` 是允许访问的 standard allowlist；空数组会禁止全部标准分组；
- `exclusive_group_ids` 继续表达专属标准分组授权。专属标准分组必须同时通过 restricted allowlist 和 exclusive 授权，两套列表互不替代；
- `subscription` 分组仍由有效订阅、套餐状态和订阅额度管理，不会被 standard allowlist 授权或撤销；
- `group_rates` 独立保留。数字设置用户倍率，PUT 中单个值 `null` 清除对应倍率；权限变化不会自动删除倍率；
- 配置保存成功后会立即失效该用户全部 API Key 的本地与 Redis 认证缓存，并通知其他实例，不需要等待普通缓存 TTL。

对存量 Key 的影响：

- 单分组 Key 指向被限制 standard 分组时返回 `403 GROUP_NOT_ALLOWED`，不会改写原 `group_id`；
- 多分组 Key 自动路由会跳过被限制分组并继续下一个绑定；若全部候选都被限制，则返回 `403 GROUP_NOT_ALLOWED`；
- 管理员重新放开权限后，原有绑定无需重建即可恢复候选资格。

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
- 已删除、停用或失去权限的分组仍按现有分组错误返回；用户级 standard allowlist 拒绝时返回 `403 GROUP_NOT_ALLOWED`；
- 显式选择已绑定但被用户限制的分组时不会静默回退到其他绑定；
- 未发送请求头时保持向后兼容：按绑定优先级跳过被用户限制的分组，再选择第一个同时满足当前端点、模型和媒体能力的分组；图片请求还会检查分组生图开关与真实图片账号能力。若都不可用，则沿既有错误分类返回明确原因，不会把聊天账号误当成图片账号。

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

该接口只允许密钥所有者访问，不返回 API Key 明文或上游账号信息。响应汇总该 Key 全部有效绑定分组的模型集合，并按模型名（大小写不敏感）去重、排序。对于已关联活跃渠道的分组，模型集合与模型广场使用同一份渠道 `mapping ∪ pricing` 目录，再与当前可调度账号能力求交集，因此不会把账号映射中的跨平台模型或平台默认目录额外展示出来。每个模型仍保留最高优先级可用分组的隐藏路由元数据：

```json
[
  {
    "id": "27::claude-sonnet-4-6",
    "model": "claude-sonnet-4-6",
    "group_id": 27,
    "group_name": "Claude",
    "group_priority": 1,
    "platform": "anthropic",
    "capabilities": ["chat"],
    "features": {
      "image_input": true,
      "responses": true,
      "web_search": true,
      "code_execution": false,
      "web_fetch": true
    }
  }
]
```

同名模型位于多个绑定分组时只返回并显示一次；绑定优先级更高且当前可路由的分组负责实际请求，因此接口中的 `group_id`、`group_name`、`group_priority` 和 `platform` 继续作为路由元数据保留，但练习场模型选择器只展示模型名。Chat、Compare、Image 与 Video 只展示当前模式可调用的模型，并在实际请求及视频状态轮询中自动携带选中模型对应的 `X-Sub2API-Group-ID`。`features` 是向后兼容的能力提示，前端只在服务端明确标记时开启图片输入、Responses 联网搜索或上游代码执行；普通采样参数仍按兼容协议发送。

图片目录与正式图片调度使用同一套媒体能力判定：模型必须存在于分组所属渠道的模型广场目录中，分组必须启用生图，且至少存在一个支持所选模型和 generations/edits 能力的可调度图片账号。普通聊天账号、仅 upstream 兼容账号或已失效授权不会让图片按钮被误启用。

## 练习场生图工作台与参考图

- 桌面端提供“控制项 / 主画布 / 最近结果”三栏工作台；平板和移动端自动改为下移历史或单列布局。
- 无参考图时发送 `/v1/images/generations` JSON 请求；加入参考图后发送 `/v1/images/edits` multipart 请求，两者都使用模型选项对应的分组头。
- 参考图支持 PNG、JPEG、WebP，默认最多 4 张、单张 20 MiB、总计 80 MiB。浏览器仅保留本地预览与重试所需的 `File`，最近结果也只保留在当前页面会话。
- 服务端参考图写入 `${DATA_DIR}/tmp/playground-images` 下的请求级随机目录。成功、失败、拒绝、超时、连接中断和 panic 路径都会执行幂等删除；启动和周期清扫会回收异常退出遗留的过期目录。
- 上传超过限制会返回明确的 `413`，不会截断后继续请求上游；临时文件名、原始文件名、提示词和图片内容不会写入清扫日志。

## 练习场附件、高级参数与会话

- 图片附件支持 PNG、JPEG、WebP、GIF，单个最多 5 MB；文本/代码附件单个最多 512 KB；每轮最多 4 个附件，总负载最多 12 MB。
- 原始附件保存在浏览器 IndexedDB，会话 localStorage 只保存附件元数据和引用；API Key 明文不会写入会话或设置存储。
- 图片通过 OpenAI Chat `image_url` content part 发送；切换到 `/v1/responses` 时会转换为 `input_image`。文本/代码附件会带文件名边界作为文本上下文发送。
- Chat 支持 `temperature`、`top_p`、`max_tokens` 和 reasoning effort。Compare 对所有列应用相同采样参数；联网搜索、代码执行和网页抓取仅在 Chat 中启用。
- 联网搜索使用上游 `/v1/responses` `web_search` 工具；代码执行只对模型选项明确声明的上游原生能力开放，不在 Sub2API 主机或浏览器中执行任意模型代码。
- 会话支持新建、切换、重命名、删除撤销、清空，以及 Markdown / 自包含 JSON 导出。导出可内嵌附件，但不包含 API Key 明文。

## 安全网页抓取 API

登录用户可让练习场读取当前消息中明确出现的公开 URL：

```http
POST /api/v1/playground/fetch-url
Authorization: Bearer <session JWT>
Content-Type: application/json

{
  "urls": ["https://example.com/docs"]
}
```

成功响应的 `data.results` 包含原始 URL、最终 URL、状态码、Content-Type、可读文本和截断标记：

```json
{
  "results": [
    {
      "url": "https://example.com/docs",
      "final_url": "https://www.example.com/docs",
      "status_code": 200,
      "content_type": "text/html",
      "content": "Readable page text...",
      "truncated": false
    }
  ]
}
```

安全边界固定为：每次最多 3 个 URL、请求总超时 10 秒、单响应最多读取 1 MB、注入模型的可读文本最多约 100 KB、最多跟随 5 次重定向；仅允许 HTTP/HTTPS 文本内容。服务端会在初始解析、DNS 解析、每次重定向和实际拨号前阻断 localhost、URL 凭据、私网、链路本地、保留地址和非全局单播地址。该接口使用登录 JWT，不接收也不转发用户 API Key。

该能力使用内置安全上限，无需新增部署配置；`deploy/config.example.yaml` 记录了这一默认边界。

## 多端点与浏览器测速

API 密钥页会同时展示：

- 管理员设置的主 API 地址；
- `custom_endpoints` 中最多 10 个自定义入口；
- 内置备用入口 `https://www.pokeapi.top`（若已配置相同地址则自动去重）。

页面由当前用户的浏览器直接请求各端点 origin 下的 `/health`，显示从该用户网络出发的实时延迟、连接状态和最近测速时间。测速请求不携带登录凭据或 API Key，不经过 Sub2API 服务端代理，也不会上传或持久化测速结果。用户可点击“重新测速”刷新全部节点数据，点击端点可复制完整地址；鼠标悬浮、键盘聚焦或触摸聚焦时可查看节点名称、管理员说明、协议和详细状态。

管理员仍在 **设置 → 站点设置 → 自定义端点** 中维护名称、地址和说明，无需新增配置字段。推荐将说明写成对用户有帮助的网络信息，例如适用区域、运营商或备用线路用途，避免承诺无法持续保证的固定延迟。

## 安全与兼容边界

- `X-Sub2API-Group-ID` 只能在当前密钥绑定范围内选择，不提供任意分组访问能力；
- 用户级限制只控制 standard 分组，不能授予 subscription 分组；exclusive standard 分组仍需独立专属授权；
- 请求头名称固定，不允许通过配置修改；
- `/v1/models` 等公开兼容端点的既有响应格式不变，但只应展示当前用户实际有权使用的分组选项；
- 未启用 restricted 的历史用户和普通客户端保持原行为；
- 显式分组选择失败不会静默回退到其他分组，避免界面选择、计费归属和实际路由不一致；
- 权限与 `group_rates` 独立，倍率配置本身不提供任何分组访问能力。
