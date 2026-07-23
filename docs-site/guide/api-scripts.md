# API 脚本接入

在程序中直接通过 HTTP 调用 Poke API。以下流程按 **准备 → 安装/配置 → 启动 → 验证** 排列，示例不会把 API Key 硬编码进脚本。

## 一、准备

### 接口与地址

当前站点统一以 `https://www.poke2api.com` 为服务根地址；OpenAI 与 Anthropic 的直接 HTTP 路径均以 `/v1` 开头。

| 接口 | 方法与完整路径 | 说明 |
| --- | --- | --- |
| OpenAI Responses | `POST https://www.poke2api.com/v1/responses` | 文本、图片理解与流式输出 |
| OpenAI Chat Completions | `POST https://www.poke2api.com/v1/chat/completions` | 兼容传统 Chat 格式 |
| Anthropic Messages | `POST https://www.poke2api.com/v1/messages` | Claude Messages 格式 |
| 图片生成 | `POST https://www.poke2api.com/v1/images/generations` | 文生图 |
| 图片编辑 | `POST https://www.poke2api.com/v1/images/edits` | `multipart/form-data` 图片编辑 |

> 在 OpenAI 客户端中填写的 Base URL 是 `https://www.poke2api.com/v1`；在 Anthropic 客户端中填写的 Base URL 是 `https://www.poke2api.com`。直接 HTTP 请求使用上表完整路径。

### 隐藏读取 API Key

不要在命令中写 `sk-...`，也不要在 Python / Node.js 文件中写死密钥。先把密钥隐藏读取到当前进程环境变量：

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

## 二、安装与配置

- curl 示例需要可用的 `curl`。
- PowerShell 示例使用系统自带的 `Invoke-RestMethod`。
- Node.js 示例使用内置 `fetch`，建议 Node.js 18 或更高版本。
- Python 示例只使用标准库，建议 Python 3.9 或更高版本。

无需安装 OpenAI 或 Anthropic SDK。模型名（如 `gpt-5.5`、`claude-sonnet-4-6`、`gpt-image-2`）仅为示例，请以控制台中密钥分组实际可用的模型为准。

## 三、启动请求

### OpenAI Responses：四种可直接运行的示例

以下示例均发送非流式请求，便于先验证鉴权与 JSON 格式。

::: code-group

```bash [curl]
curl --fail-with-body --silent --show-error \
  https://www.poke2api.com/v1/responses \
  -H "Authorization: Bearer ${POKE_API_KEY}" \
  -H "Content-Type: application/json" \
  --data '{
    "model": "gpt-5.5",
    "input": "用中文简单介绍一下 Poke API。"
  }'
```

```powershell [PowerShell]
if (-not $env:POKE_API_KEY) { throw '请先隐藏读取 POKE_API_KEY。' }

$headers = @{ Authorization = "Bearer $env:POKE_API_KEY" }
$body = @{
  model = 'gpt-5.5'
  input = '用中文简单介绍一下 Poke API。'
} | ConvertTo-Json -Depth 5

Invoke-RestMethod `
  -Uri 'https://www.poke2api.com/v1/responses' `
  -Method Post `
  -Headers $headers `
  -ContentType 'application/json' `
  -Body $body
```

```javascript [Node.js]
async function main() {
  const apiKey = process.env.POKE_API_KEY;
  if (!apiKey) throw new Error("请先设置 POKE_API_KEY 环境变量");

  const response = await fetch("https://www.poke2api.com/v1/responses", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${apiKey}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      model: "gpt-5.5",
      input: "用中文简单介绍一下 Poke API。",
    }),
  });

  const raw = await response.text();
  if (!response.ok) throw new Error(`HTTP ${response.status}: ${raw}`);
  console.log(JSON.stringify(JSON.parse(raw), null, 2));
}

main().catch((error) => {
  console.error(error.message);
  process.exitCode = 1;
});
```

```python [Python]
import json
import os
import urllib.error
import urllib.request

api_key = os.environ.get("POKE_API_KEY")
if not api_key:
    raise RuntimeError("请先设置 POKE_API_KEY 环境变量")

body = {
    "model": "gpt-5.5",
    "input": "用中文简单介绍一下 Poke API。",
}
request = urllib.request.Request(
    "https://www.poke2api.com/v1/responses",
    data=json.dumps(body).encode("utf-8"),
    method="POST",
    headers={
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    },
)

try:
    with urllib.request.urlopen(request, timeout=900) as response:
        result = json.loads(response.read().decode("utf-8"))
except urllib.error.HTTPError as error:
    detail = error.read().decode("utf-8", errors="replace")
    raise RuntimeError(f"HTTP {error.code}: {detail}") from error

print(json.dumps(result, ensure_ascii=False, indent=2))
```

:::

### OpenAI Chat Completions

```bash
curl --fail-with-body --silent --show-error \
  https://www.poke2api.com/v1/chat/completions \
  -H "Authorization: Bearer ${POKE_API_KEY}" \
  -H "Content-Type: application/json" \
  --data '{
    "model": "gpt-5.5",
    "messages": [{"role": "user", "content": "用中文简单介绍一下 Poke API。"}]
  }'
```

文本加图片时，`messages[].content` 使用多模态数组：

```json
{
  "model": "gpt-5.5",
  "messages": [{
    "role": "user",
    "content": [
      {"type": "text", "text": "这张图片有什么？请用中文简要描述。"},
      {"type": "image_url", "image_url": {"url": "https://www.poke2api.com/logo.png"}}
    ]
  }]
}
```

### Anthropic Messages

```bash
curl --fail-with-body --silent --show-error \
  https://www.poke2api.com/v1/messages \
  -H "Authorization: Bearer ${POKE_API_KEY}" \
  -H "Content-Type: application/json" \
  --data '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "messages": [{"role": "user", "content": "用中文简单介绍一下 Poke API。"}]
  }'
```

### 图片生成

```bash
curl --fail-with-body --silent --show-error \
  https://www.poke2api.com/v1/images/generations \
  -H "Authorization: Bearer ${POKE_API_KEY}" \
  -H "Content-Type: application/json" \
  --data '{
    "model": "gpt-image-2",
    "prompt": "一只在星空下奔跑的皮卡丘，赛博朋克风格",
    "size": "1024x1024",
    "quality": "high",
    "response_format": "b64_json"
  }' > image-response.json
```

服务可能在 `data[0].b64_json` 返回 Base64 图片。应解析真实响应后再保存文件，不要预先伪造 `image-response.json`。

### 图片编辑

`POST /v1/images/edits` 使用 `multipart/form-data`，字段包含 `model`、`prompt`、`image`（可重复）及可选的 `size`、`quality`、`n`、`response_format`。

```bash
curl --fail-with-body --silent --show-error \
  https://www.poke2api.com/v1/images/edits \
  -H "Authorization: Bearer ${POKE_API_KEY}" \
  -F 'model=gpt-image-2' \
  -F 'prompt=保留主体，把背景改为克制的黑白编辑风格，并加入少量深蓝信号色' \
  -F 'size=1024x1024' \
  -F 'quality=high' \
  -F 'response_format=b64_json' \
  -F 'image=@reference.png;type=image/png' \
  > image-edit-response.json
```

参考图只在当前请求中使用：服务器将文件写入隔离的请求级临时目录，并在成功、失败、拒绝、超时或连接中断后清理；启动和周期清扫会回收进程异常退出遗留的过期目录。默认最多 4 张，支持 PNG、JPEG、WebP，单张不超过 20 MiB、总计不超过 80 MiB；超限返回 `413`，不会静默截断。

::: tip
- 多分组 API Key 应携带 `X-Sub2API-Group-ID`，该 ID 必须属于当前 Key；练习场会自动发送所选模型对应的分组。
- `gpt-image-*` 的质量值通常为 `auto`、`low`、`medium`、`high`；DALL-E 使用 `standard`、`hd`。不支持的参数应省略。
- 上述图片示例使用非流式 JSON 返回；模型名仅为示例，请以控制台实际可用且具备图片能力的模型列表为准。
:::

## 四、验证

### 无密钥验证只能得到真实 401

先故意不带 `Authorization` 请求头：

```bash
curl --silent --output /dev/null --write-out 'HTTP %{http_code}\n' \
  https://www.poke2api.com/v1/responses \
  -H 'Content-Type: application/json' \
  --data '{"model":"gpt-5.5","input":"ping"}'
```

截至 **2026-07-15**，无密钥实测结果为真实的 `HTTP 401`。这表示接口鉴权正常拒绝了请求，但不能证明模型调用成功。

![Windows PowerShell 无密钥请求返回真实 HTTP 401](/images/api-scripts/unauthorized-check.png)

> **图：API 脚本鉴权失败实拍。** 来源：本站在 Windows PowerShell 中直接请求 Poke API 线上 `/v1/models` 接口；采集日期：2026-07-15。截图未发送、未显示任何 API Key，只用于证明真实服务会拒绝无凭据请求；错误文案、请求 ID 与响应头存在时效性，以当次线上响应为准。

### 有密钥时检查真实结果

1. 运行本页的隐藏输入命令，确认 `POKE_API_KEY` 只存在于当前进程环境变量。
2. 执行任一完整示例。
3. 只有收到真实 `2xx` 响应和服务返回的 JSON，才可判定调用成功。
4. `401` 表示鉴权问题；`404` 通常与路径或 Base URL 拼接有关；模型、额度、分组错误应按原始响应排查。

严禁为了让教程“看起来成功”而手写响应 JSON、Mock 在线结果或隐藏真实错误。没有可用密钥时，本地最多只能验证请求格式并获得服务返回的真实 `401`。

完成后清理临时密钥：

::: code-group

```bash [macOS / Linux / WSL2]
unset POKE_API_KEY
```

```powershell [Windows PowerShell]
Remove-Item Env:POKE_API_KEY
```
:::
