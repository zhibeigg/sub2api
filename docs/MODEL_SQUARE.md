# 模型广场独立开关与 API

## 概述

模型广场与“可用渠道”使用同一套用户可见渠道、分组、模型和定价聚合逻辑，但由两个独立的数据库系统设置控制。API 密钥编辑页中的分组模型预览读取模型广场接口，因此不会错误依赖“可用渠道”开关：

| 功能 | 设置键 | 用户接口 | 默认值 |
|---|---|---|---|
| 模型广场 | `model_square_enabled` | `GET /api/v1/models/available` | `false` |
| 可用渠道 | `available_channels_enabled` | `GET /api/v1/channels/available` | `false` |

两个开关都在 **管理后台 → 系统设置 → 功能开关** 中配置，不是 `config.yaml` 字段。关闭某个功能后，对应侧边栏入口隐藏，对应用户接口在认证成功后返回空数组；另一个功能不受影响。

## 升级兼容

首次引入 `model_square_enabled` 时，数据库迁移会在新键不存在的前提下继承 `available_channels_enabled` 的当前严格布尔状态：旧值仅在恰好为 `true` 时继承为开启，否则写入 `false`。

这保证升级后模型广场保持升级前的可见状态。迁移完成后两个键独立保存，后续修改任意一个都不会同步另一个。

## 管理端配置示例

管理员可通过 `PUT /api/v1/admin/settings` 更新系统设置。请求采用部分更新语义，未提交的字段保持原值。

仅开启模型广场：

```json
{
  "model_square_enabled": true,
  "available_channels_enabled": false
}
```

仅开启可用渠道：

```json
{
  "model_square_enabled": false,
  "available_channels_enabled": true
}
```

公开设置接口 `GET /api/v1/settings/public` 与服务端注入的 `window.__APP_CONFIG__` 都会分别返回这两个布尔字段，供前端首屏决定菜单显隐。

## 用户接口

### 模型广场

```http
GET /api/v1/models/available
Authorization: Bearer <JWT>
```

### 可用渠道

```http
GET /api/v1/channels/available
Authorization: Bearer <JWT>
```

两个接口都要求已登录用户的 JWT 会话鉴权，并遵循相同的安全过滤：

1. 仅返回状态为 active 且与当前用户可访问分组有交集的渠道；
2. 每个渠道只保留当前用户可访问的分组；
3. 模型按用户可见分组的平台过滤，避免跨平台信息泄漏；
4. 响应只包含用户端所需的白名单字段，不暴露内部渠道 ID、限制配置和管理字段；
5. `supported_models[].group_rates[]` 使用后端按用户、模型、高峰时段及媒体独立倍率解析的当前快照。

响应结构同构：

```json
[
  {
    "name": "OpenAI Channel",
    "description": "",
    "platforms": [
      {
        "platform": "openai",
        "groups": [],
        "supported_models": []
      }
    ]
  }
]
```

开关关闭时，已认证请求返回成功响应和空数组 `[]`；未认证请求仍优先返回 `401`。

## 独立行为矩阵

| 可用渠道 | 模型广场 | 侧边栏 | `/channels/available` | `/models/available` |
|---|---|---|---|---|
| 关 | 关 | 两者均隐藏 | `[]` | `[]` |
| 关 | 开 | 仅模型广场显示 | `[]` | 正常聚合数据 |
| 开 | 关 | 仅可用渠道显示 | 正常聚合数据 | `[]` |
| 开 | 开 | 两者均显示 | 正常聚合数据 | 正常聚合数据 |

## 配置注意事项

模型广场展示的数据仍来自渠道定价和分组授权。启用模型广场不会自动创建渠道、关联分组或补充模型价格；管理员应先在渠道管理中完成模型映射、定价与分组关联配置。
