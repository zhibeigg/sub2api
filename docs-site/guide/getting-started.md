# 快速开始

Poke API 是一个 AI API 网关平台，你只需一个 API Key，即可通过统一入口访问 Claude、OpenAI、Gemini 等多家大模型，并完整兼容各家官方接口协议。

## 第一步：获取 API Key

1. 打开并登录控制台：[https://www.poke2api.com](https://www.poke2api.com)
2. 进入 **密钥管理**（个人中心）页面
3. 创建并复制一个 API Key，形如 `sk-xxxxxxxx`

> 请妥善保管你的 API Key，不要泄露或提交到代码仓库。

## 第二步：了解两个基础地址

Poke API 提供两类兼容入口，请根据所用工具/协议选择正确的地址：

| 协议类型 | Base URL | 是否带 `/v1` |
| --- | --- | --- |
| Anthropic（Claude Messages） | `https://www.poke2api.com` | ❌ 不带 |
| OpenAI（Responses / Chat / Images） | `https://www.poke2api.com/v1` | ✅ 必须 |
| Gemini | `https://www.poke2api.com` | ❌ 不带（走 `GOOGLE_GEMINI_BASE_URL`） |

::: warning 常见错误
Claude / Anthropic 接口地址 **不要** 加 `/v1`；OpenAI / Codex 接口地址 **必须** 加 `/v1`。地址填错会导致 404 或鉴权失败。
:::

## 第三步：选择接入方式

Poke API 支持两大类接入方式：

### 命令行工具（推荐）

一行脚本即可完成安装与配置，适合日常 AI 编程：

- [Claude Code](/guide/claude-code) —— Anthropic 官方 CLI
- [Codex (OpenAI)](/guide/codex) —— OpenAI 官方 CLI
- [Gemini CLI](/guide/gemini-cli) —— Google 官方 CLI
- [TRAE SOLO](/guide/trae-solo)
- [OpenClaw](/guide/openclaw)
- [Hermes](/guide/hermes)

> 上述工具大多依赖 Node.js 运行环境，首次使用请先完成 [Node.js 环境安装](/guide/nodejs)。

### 直接调用 API

在自己的程序中通过 HTTP 调用，兼容主流 SDK：

- [API 脚本接入](/guide/api-scripts) —— Responses、Chat Completions、Anthropic Messages、图片生成与编辑

## 快速验证

配置好 API Key 后，可用如下命令快速验证连通性（OpenAI 兼容接口）：

```bash
curl https://www.poke2api.com/v1/chat/completions \
  -H "Authorization: Bearer YOUR_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-5.5",
    "messages": [{"role": "user", "content": "用中文简单介绍一下 Poke API。"}]
  }'
```

返回正常的 JSON 响应即表示接入成功。将 `YOUR_API_KEY` 替换为你在控制台复制的密钥。
