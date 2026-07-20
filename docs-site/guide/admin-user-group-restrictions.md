# 管理员：用户分组访问限制

管理员可以为单个用户限制可使用的**标准（standard）分组**，同时保留专属分组授权、订阅分组和用户倍率的独立管理。

## 四个独立配置

| 字段 | 作用 |
| --- | --- |
| `access_mode` | `inherit` 保持默认行为；`restricted` 启用标准分组 allowlist |
| `restricted_group_ids` | restricted 模式下允许使用的 standard 分组 |
| `exclusive_group_ids` | 授予 existing exclusive standard 分组 |
| `group_rates` | 用户专属分组倍率，不授予访问权限 |

::: danger 空 allowlist 的含义
`access_mode=restricted` 且 `restricted_group_ids=[]` 会禁止该用户使用**全部标准分组**。若要恢复默认行为，请设置 `access_mode=inherit`。
:::

## 分组类型规则

- 普通 standard：restricted 时必须位于 `restricted_group_ids`。
- exclusive standard：必须同时位于 standard allowlist 和 `exclusive_group_ids`，两套授权取交集。
- subscription：不受 standard allowlist 影响，继续由有效订阅和订阅额度管理。

## 管理员 API

读取：

```http
GET /api/v1/admin/users/:id/group-config
```

更新：

```http
PUT /api/v1/admin/users/:id/group-config
Content-Type: application/json
```

请求示例：

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

其中 `group_rates` 的数字值用于设置倍率，`null` 用于清除对应分组倍率。权限变化不会自动删除其他倍率。

## API Key 路由

- 已有单分组 Key 指向被限制分组：返回 `403 GROUP_NOT_ALLOWED`。
- 多分组 Key 自动路由：跳过被限制分组，继续尝试下一个绑定。
- 客户端使用 `X-Sub2API-Group-ID` 显式选择被限制分组：返回 403，不回退。
- 如果多分组全部候选都被限制：返回 `403 GROUP_NOT_ALLOWED`。

## 立即生效

管理员 PUT 成功后，服务端会立即清除该用户 API Key 的本地和 Redis 认证缓存，并通知其他实例失效。后续新请求不需要等待普通缓存 TTL；已经进入处理流程的在途请求不会被强制中断。

## 向后兼容

历史用户默认使用 `inherit`，现有专属授权、订阅、API Key 绑定和用户倍率保持不变。该功能保存在数据库中，不需要在部署 YAML 中增加配置键。

常见错误码：

- `INVALID_ACCESS_MODE`
- `INVALID_RESTRICTED_GROUP`
- `INVALID_EXCLUSIVE_GROUP`
- `INVALID_GROUP_RATE`
- `GROUP_NOT_ALLOWED`
- `USER_NOT_FOUND`

完整管理员 API、缓存与兼容说明见仓库文档 `docs/ADMIN_USER_GROUP_RESTRICTIONS.md`。
