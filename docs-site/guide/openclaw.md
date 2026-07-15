# OpenClaw 配置教程

通过 Poke API 接入 OpenClaw，支持 Anthropic（Claude）与 OpenAI（Codex）两种通道。推荐使用一键脚本，运行后按提示选择通道并输入 API Key。

| 通道 | Base URL | 说明 |
| --- | --- | --- |
| Anthropic（Claude） | `https://www.poke2api.com` | 不要带 `/v1`，示例模型：`claude-opus-4-6` |
| OpenAI（Codex） | `https://www.poke2api.com/v1` | 必须带 `/v1`，示例模型：`gpt-5.5` |

## 一键脚本配置（推荐）

```bash
curl -fsSL https://docs.poke2api.com/install/openclaw.sh | bash
```

::: tip
脚本会自动检测并安装 OpenClaw，随后提示选择通道（Claude / Codex）并输入 API Key。请先在 [控制台](https://www.poke2api.com) 密钥管理获取对应分组的 API Key。
:::

## 手动配置（备用）

### 第一步：安装 OpenClaw

```bash
npm install -g @openclaw/cli
```

### 第二步：选择接入通道

::: code-group

```bash [Anthropic（Claude）]
export ANTHROPIC_API_KEY="YOUR_API_KEY"
openclaw onboard --auth-choice custom-api-key \
  --custom-base-url https://www.poke2api.com \
  --custom-api-key-env ANTHROPIC_API_KEY \
  --custom-compatibility anthropic \
  --custom-model claude-opus-4-6
```

```bash [OpenAI（Codex）]
export OPENAI_API_KEY="YOUR_API_KEY"
openclaw onboard --auth-choice custom-api-key \
  --custom-base-url https://www.poke2api.com/v1 \
  --custom-api-key-env OPENAI_API_KEY \
  --custom-compatibility openai \
  --custom-model gpt-5.5
```

:::

> Anthropic（Claude）通道使用根地址，不要加 `/v1`；OpenAI（Codex）通道使用 `/v1` 地址。

::: tip
请先在 [控制台](https://www.poke2api.com) 密钥管理获取对应分组的 API Key，并替换命令中的 `YOUR_API_KEY`。
:::

## 开始使用

```bash
openclaw
```
