# TRAE SOLO 配置教程

在 TRAE SOLO 中添加自定义模型，分别接入 Codex 和 Claude 通道。

## 第一步：进入自定义模型配置

依次操作：**打开设置 → 模型 → 添加模型 → 自定义配置**。

## 第二步：按模型类型填写配置

### Codex 模型配置

OpenAI / Codex 地址使用 `https://www.poke2api.com/v1`（必须带 `/v1`）。

| 字段 | 值 |
| --- | --- |
| Base URL | `https://www.poke2api.com/v1` |
| API Key | 在控制台密钥管理获取 |
| 模型示例 | `gpt-5.5` |

### Claude 模型配置

Claude / Anthropic 地址使用 `https://www.poke2api.com`（不要带 `/v1`）。

| 字段 | 值 |
| --- | --- |
| Base URL | `https://www.poke2api.com` |
| API Key | 在控制台密钥管理获取 |
| 模型示例 | `claude-opus-4-7` |

::: tip
API Key 请在 [控制台](https://www.poke2api.com) 密钥管理获取；Codex 模型示例可填 `gpt-5.5`，Claude 模型示例可填 `claude-opus-4-7`。
:::
