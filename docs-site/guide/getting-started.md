# 快速开始

Poke API 是一个 AI API 网关平台，可通过统一入口访问 Claude、OpenAI、Gemini 等模型，并兼容对应接口协议。以下流程按 **准备 → 安装/配置 → 启动 → 验证** 排列。

## 一、准备

### 获取 API Key

1. 打开并登录 [控制台](https://www.poke2api.com)。
2. 进入 **密钥管理**（个人中心）。
3. 创建并复制对应分组的 API Key。

不要把真实密钥写进文档、源代码、Git 仓库或命令行参数。需要在终端中临时使用时，优先隐藏读取到当前进程的环境变量。

::: code-group

```bash [macOS / Linux / WSL2]
read -rsp "Poke API Key: " POKE_API_KEY; printf '\n'
export POKE_API_KEY
```

```powershell [Windows PowerShell]
$secureKey = Read-Host 'Poke API Key' -AsSecureString
$ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secureKey)
try {
  $env:POKE_API_KEY = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
} finally {
  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
}
```

:::

关闭终端后临时环境变量即失效。完成验证后也可以主动清理：

::: code-group

```bash [macOS / Linux / WSL2]
unset POKE_API_KEY
```

```powershell [Windows PowerShell]
Remove-Item Env:POKE_API_KEY
```

:::

## 二、安装与配置

Poke API 当前站点约定的基础地址如下：

| 协议类型 | Base URL | 请求路径示例 |
| --- | --- | --- |
| Anthropic（Claude Messages） | `https://www.poke2api.com` | `/v1/messages` |
| OpenAI（Responses / Chat / Images） | `https://www.poke2api.com/v1` | `/responses`、`/chat/completions`、`/images/generations` |
| Gemini | `https://www.poke2api.com` | 由对应 Gemini 客户端拼接协议路径 |

::: warning 常见错误
Claude / Anthropic 客户端的 Base URL 不要添加 `/v1`；OpenAI / Codex 客户端的 Base URL 必须包含 `/v1`。直接发送 HTTP 请求时，则按完整路径（例如 `https://www.poke2api.com/v1/messages`）调用。
:::

选择一种接入方式并完成对应配置：

### 命令行工具

- [Claude Code](/guide/claude-code)
- [Codex (OpenAI)](/guide/codex)
- [Gemini CLI](/guide/gemini-cli)
- [TRAE SOLO](/guide/trae-solo)
- [OpenClaw](/guide/openclaw)
- [Hermes](/guide/hermes)

需要 Node.js 的工具请先完成 [Node.js 环境安装](/guide/nodejs)。各工具的具体依赖以对应教程为准。

### 直接调用 API

无需额外 CLI；可继续阅读 [API 脚本接入](/guide/api-scripts)，使用 curl、PowerShell、Node.js 或 Python 发起请求。

## 三、启动请求

下面使用 OpenAI Chat Completions 路径发送最小请求。命令历史只会记录环境变量名，不包含密钥原文。

::: code-group

```bash [curl]
curl --fail-with-body --silent --show-error \
  https://www.poke2api.com/v1/chat/completions \
  -H "Authorization: Bearer ${POKE_API_KEY}" \
  -H "Content-Type: application/json" \
  --data '{
    "model": "gpt-5.5",
    "messages": [{"role": "user", "content": "用中文简单介绍一下 Poke API。"}]
  }'
```

```powershell [PowerShell]
if (-not $env:POKE_API_KEY) { throw '请先隐藏读取 POKE_API_KEY。' }
$headers = @{ Authorization = "Bearer $env:POKE_API_KEY" }
$body = @{
  model = 'gpt-5.5'
  messages = @(@{
    role = 'user'
    content = '用中文简单介绍一下 Poke API。'
  })
} | ConvertTo-Json -Depth 5

Invoke-RestMethod `
  -Uri 'https://www.poke2api.com/v1/chat/completions' `
  -Method Post `
  -Headers $headers `
  -ContentType 'application/json' `
  -Body $body
```

:::

模型名仅为示例，请以控制台中该密钥分组实际可用的模型为准。

## 四、验证

### 先验证鉴权边界

在**不发送 API Key**时，服务应返回真实鉴权失败，而不是成功内容：

```bash
curl --silent --output /dev/null --write-out 'HTTP %{http_code}\n' \
  https://www.poke2api.com/v1/chat/completions \
  -H 'Content-Type: application/json' \
  --data '{"model":"gpt-5.5","messages":[{"role":"user","content":"ping"}]}'
```

截至 **2026-07-15**，无密钥实测结果为真实的 `HTTP 401`。这只能证明请求到达鉴权层，不能证明模型调用成功。

![Windows PowerShell 无密钥请求返回真实 HTTP 401](/images/getting-started/unauthorized-check.png)

> **图：无密钥鉴权边界实拍。** 来源：本站在 Windows PowerShell 中直接请求 Poke API 线上 `/v1/models` 接口；采集日期：2026-07-15。画面只包含公开请求地址、响应头和 `API_KEY_REQUIRED` 错误，不包含 API Key。服务端错误文案、请求 ID 与响应头可能随版本变化，应以当次真实响应为准。

### 再验证真实模型响应

设置 `POKE_API_KEY` 后运行上一节请求：

- 收到 `2xx` 和真实 JSON 响应，才表示该密钥、分组、模型与接口路径可以工作。
- 收到 `401`，检查密钥是否正确、是否已被撤销。
- 收到 `404`，优先检查 Base URL 与 `/v1` 拼接方式。
- 收到模型不可用、额度或权限错误，应保留并排查服务返回的真实错误。

文档不会提供伪造的“成功响应”。没有可用密钥时，只能验证到真实的 `401`；不要用手写 JSON、Mock 输出或截图冒充线上调用成功。
