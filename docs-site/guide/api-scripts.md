# API 脚本接入

在你自己的程序中通过 HTTP 直接调用 Poke API，完整兼容 OpenAI 与 Anthropic 官方协议。所有请求使用 `Authorization: Bearer YOUR_API_KEY` 鉴权，请将示例中的 `YOUR_API_KEY` 替换为 [控制台](https://www.poke2api.com) 中获取的密钥。

## 接口总览

| 接口 | 方法与路径 | 说明 |
| --- | --- | --- |
| OpenAI Responses | `POST /v1/responses` | 新一代 OpenAI 接口，适合文本对话、图片理解和流式输出 |
| OpenAI Chat Completions | `POST /v1/chat/completions` | 兼容传统 OpenAI Chat 格式，便于已有客户端平滑迁移 |
| Anthropic Messages | `POST /v1/messages` | Claude / Anthropic Messages 格式，支持文本与视觉 |
| 图片生成 | `POST /v1/images/generations` | 文生图（gpt-image-2） |
| 图片编辑 | `POST /v1/images/edits` | 上传图片并按提示修改 |

> 基础地址：`https://www.poke2api.com`（OpenAI 系列接口路径均以 `/v1` 开头）。

## OpenAI Responses（文本 + 视觉）

`POST /v1/responses` —— 适合文本对话、图片理解和流式输出。

### 纯文本

::: code-group

```python [Python]
import json
import urllib.request

API_URL = "https://www.poke2api.com/v1/responses"
API_KEY = "YOUR_API_KEY"

body = {
    "model": "gpt-5.5",
    "stream": True,
    "input": "用中文简单介绍一下 Poke API。",
}

def iter_sse(response):
    buffer = ""
    while chunk := response.read(4096):
        buffer += chunk.decode("utf-8", errors="replace")
        frames = buffer.split("\n\n")
        buffer = frames.pop()
        for frame in frames:
            data = "\n".join(line[5:].strip() for line in frame.splitlines() if line.startswith("data:")).strip()
            if data and data != "[DONE]":
                yield data

request = urllib.request.Request(
    API_URL,
    data=json.dumps(body).encode("utf-8"),
    method="POST",
    headers={"Authorization": "Bearer " + API_KEY, "Content-Type": "application/json", "Accept": "text/event-stream"},
)

with urllib.request.urlopen(request, timeout=900) as response:
    for data in iter_sse(response):
        event = json.loads(data)
        if event.get("type") == "response.output_text.delta":
            print(event.get("delta", ""), end="", flush=True)
        if event.get("type") in ("response.completed", "response.done"):
            break
print()
```

```javascript [Node.js]
const API_URL = "https://www.poke2api.com/v1/responses";
const API_KEY = "YOUR_API_KEY";

const body = {
  model: "gpt-5.5",
  stream: true,
  input: "用中文简单介绍一下 Poke API。",
};

async function* readSse(response) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  while (true) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const frames = buffer.split(/\r?\n\r?\n/);
    buffer = frames.pop() || "";
    for (const frame of frames) {
      const data = frame.split(/\r?\n/).filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trim()).join("\n");
      if (data && data !== "[DONE]") yield data;
    }
  }
}

const response = await fetch(API_URL, {
  method: "POST",
  headers: { Authorization: "Bearer " + API_KEY, "Content-Type": "application/json", Accept: "text/event-stream" },
  body: JSON.stringify(body),
});
if (!response.ok) throw new Error(await response.text());

for await (const data of readSse(response)) {
  const event = JSON.parse(data);
  if (event.type === "response.output_text.delta") process.stdout.write(event.delta || "");
  if (event.type === "response.completed" || event.type === "response.done") break;
}
process.stdout.write("\n");
```

:::

### 文本 + 图片

```python
body = {
    "model": "gpt-5.5",
    "stream": True,
    "input": [{
        "role": "user",
        "content": [
            {"type": "input_text", "text": "这张图片有什么？请用中文简要描述。"},
            {"type": "input_image", "image_url": "https://www.poke2api.com/logo.png"},
        ],
    }],
}
```

> 仅需把 `input` 换成上面的多模态数组，其余请求与 SSE 解析逻辑与「纯文本」示例一致。

## OpenAI Chat Completions（文本 + 视觉）

`POST /v1/chat/completions` —— 兼容传统 OpenAI Chat 格式。

### 纯文本

::: code-group

```python [Python]
import json
import urllib.request

API_URL = "https://www.poke2api.com/v1/chat/completions"
API_KEY = "YOUR_API_KEY"

body = {
    "model": "gpt-5.5",
    "stream": True,
    "messages": [{"role": "user", "content": "用中文简单介绍一下 Poke API。"}],
}

def iter_sse(response):
    buffer = ""
    while chunk := response.read(4096):
        buffer += chunk.decode("utf-8", errors="replace")
        frames = buffer.split("\n\n")
        buffer = frames.pop()
        for frame in frames:
            data = "\n".join(line[5:].strip() for line in frame.splitlines() if line.startswith("data:")).strip()
            if data and data != "[DONE]":
                yield data

request = urllib.request.Request(
    API_URL,
    data=json.dumps(body).encode("utf-8"),
    method="POST",
    headers={"Authorization": "Bearer " + API_KEY, "Content-Type": "application/json", "Accept": "text/event-stream"},
)

with urllib.request.urlopen(request, timeout=900) as response:
    for data in iter_sse(response):
        chunk = json.loads(data)
        for choice in chunk.get("choices") or []:
            print((choice.get("delta") or {}).get("content", ""), end="", flush=True)
print()
```

```javascript [Node.js]
const API_URL = "https://www.poke2api.com/v1/chat/completions";
const API_KEY = "YOUR_API_KEY";

const body = {
  model: "gpt-5.5",
  stream: true,
  messages: [{ role: "user", content: "用中文简单介绍一下 Poke API。" }],
};

// readSse 同上文 Responses 示例
const response = await fetch(API_URL, {
  method: "POST",
  headers: { Authorization: "Bearer " + API_KEY, "Content-Type": "application/json", Accept: "text/event-stream" },
  body: JSON.stringify(body),
});
if (!response.ok) throw new Error(await response.text());

for await (const data of readSse(response)) {
  const chunk = JSON.parse(data);
  for (const choice of chunk.choices || []) process.stdout.write(choice.delta?.content || "");
}
process.stdout.write("\n");
```

:::

### 文本 + 图片

```python
body = {
    "model": "gpt-5.5",
    "stream": True,
    "messages": [{
        "role": "user",
        "content": [
            {"type": "text", "text": "这张图片有什么？请用中文简要描述。"},
            {"type": "image_url", "image_url": {"url": "https://www.poke2api.com/logo.png"}},
        ],
    }],
}
```

## Anthropic Messages（文本 + 视觉）

`POST /v1/messages` —— Claude / Anthropic Messages 格式。

::: code-group

```python [Python]
import json
import urllib.request

API_URL = "https://www.poke2api.com/v1/messages"
API_KEY = "YOUR_API_KEY"

body = {
    "model": "claude-sonnet-4-6",
    "max_tokens": 64000,
    "stream": True,
    "messages": [{"role": "user", "content": "用中文简单介绍一下 Poke API。"}],
}

def iter_sse(response):
    buffer = ""
    while chunk := response.read(4096):
        buffer += chunk.decode("utf-8", errors="replace")
        frames = buffer.split("\n\n")
        buffer = frames.pop()
        for frame in frames:
            data = "\n".join(line[5:].strip() for line in frame.splitlines() if line.startswith("data:")).strip()
            if data and data != "[DONE]":
                yield data

request = urllib.request.Request(
    API_URL,
    data=json.dumps(body).encode("utf-8"),
    method="POST",
    headers={"Authorization": "Bearer " + API_KEY, "Content-Type": "application/json", "Accept": "text/event-stream"},
)

with urllib.request.urlopen(request, timeout=900) as response:
    for data in iter_sse(response):
        event = json.loads(data)
        if event.get("type") == "content_block_delta":
            print((event.get("delta") or {}).get("text", ""), end="", flush=True)
        if event.get("type") == "message_stop":
            break
print()
```

```javascript [Node.js]
const API_URL = "https://www.poke2api.com/v1/messages";
const API_KEY = "YOUR_API_KEY";

const body = {
  model: "claude-sonnet-4-6",
  max_tokens: 64000,
  stream: true,
  messages: [{ role: "user", content: "用中文简单介绍一下 Poke API。" }],
};

// readSse 同上文 Responses 示例
const response = await fetch(API_URL, {
  method: "POST",
  headers: { Authorization: "Bearer " + API_KEY, "Content-Type": "application/json", Accept: "text/event-stream" },
  body: JSON.stringify(body),
});
if (!response.ok) throw new Error(await response.text());

for await (const data of readSse(response)) {
  const event = JSON.parse(data);
  if (event.type === "content_block_delta") process.stdout.write(event.delta?.text || "");
  if (event.type === "message_stop") break;
}
process.stdout.write("\n");
```

:::

## 图片生成（gpt-image-2）

`POST /v1/images/generations` —— 支持文生图。

```python
import base64
import json
import urllib.request

API_URL = "https://www.poke2api.com/v1/images/generations"
API_KEY = "YOUR_API_KEY"

body = {
    "model": "gpt-image-2",
    "prompt": "一只在星空下奔跑的皮卡丘，赛博朋克风格",
    "size": "1024x1024",
    "quality": "high",
    "response_format": "b64_json",
}

request = urllib.request.Request(
    API_URL,
    data=json.dumps(body).encode("utf-8"),
    method="POST",
    headers={"Authorization": "Bearer " + API_KEY, "Content-Type": "application/json"},
)

with urllib.request.urlopen(request, timeout=900) as response:
    result = json.loads(response.read().decode("utf-8"))
    b64 = result["data"][0]["b64_json"]
    with open("output.png", "wb") as f:
        f.write(base64.b64decode(b64))
print("已保存 output.png")
```

## 图片编辑（gpt-image-2）

`POST /v1/images/edits` —— 上传参考图片并按提示修改。请求使用 `multipart/form-data`，字段包含 `model`、`prompt`、`image`（可重复）及可选的 `size`、`quality`、`n`、`response_format`。

```bash
curl --fail-with-body https://www.poke2api.com/v1/images/edits \
  -H 'Authorization: Bearer YOUR_API_KEY' \
  -H 'X-Sub2API-Group-ID: YOUR_BOUND_GROUP_ID' \
  -F 'model=gpt-image-2' \
  -F 'prompt=保留主体，把背景改为克制的黑白编辑风格，并加入少量深蓝信号色' \
  -F 'size=1024x1024' \
  -F 'quality=high' \
  -F 'response_format=b64_json' \
  -F 'image=@reference.png;type=image/png'
```

参考图只在当前请求中使用：服务器将文件写入隔离的请求级临时目录，并在成功、失败、拒绝、超时或连接中断后清理；启动和周期清扫会回收进程异常退出遗留的过期目录。默认最多 4 张，支持 PNG、JPEG、WebP，单张不超过 20 MiB、总计不超过 80 MiB；超限返回 `413`，不会静默截断。

::: tip
- 多分组 API Key 应携带 `X-Sub2API-Group-ID`，该 ID 必须属于当前 Key；练习场会自动发送所选模型对应的分组。
- `gpt-image-*` 的质量值通常为 `auto`、`low`、`medium`、`high`；DALL-E 使用 `standard`、`hd`。不支持的参数应省略。
- 上述图片示例使用非流式 JSON 返回；模型名仅为示例，请以控制台实际可用且具备图片能力的模型列表为准。
:::
