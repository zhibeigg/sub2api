# Hermes 配置教程

通过 Poke API 接入 Hermes，支持 Anthropic（Claude）与 OpenAI（Codex）两种通道。推荐使用一键脚本，运行后按提示选择通道并输入 API Key。

::: warning
Claude / Anthropic 接口不要带 `/v1`；OpenAI / Codex 接口必须带 `/v1`。
:::

## 一键脚本配置（推荐）

```bash
curl -fsSL https://docs.poke2api.com/install/hermes.sh | bash
```

::: tip
脚本会检测并安装 hermes-agent（依赖 Python 与 pipx），随后提示选择通道并输入 API Key，写入 `~/.hermes/config.yaml`。请先在 [控制台](https://www.poke2api.com) 密钥管理获取对应分组的 API Key。
:::

## 手动配置（备用）

### 第一步：安装 Hermes

```bash
pipx install hermes-agent
```

### 第二步：选择接入通道

编辑 `~/.hermes/config.yaml`：

::: code-group

```yaml [Anthropic（Claude）]
mkdir -p ~/.hermes
cat > ~/.hermes/config.yaml << 'EOF'
model:
  default: claude-opus-4-7
  provider: pokeapi-claude
providers:
  pokeapi-claude:
    api_mode: anthropic_messages
    base_url: https://www.poke2api.com
    api_key: YOUR_API_KEY
    default_model: claude-opus-4-7
    models:
      - claude-opus-4-7
EOF
```

```yaml [OpenAI（Codex）]
mkdir -p ~/.hermes
cat > ~/.hermes/config.yaml << 'EOF'
model:
  default: gpt-5.5
  provider: pokeapi-codex
providers:
  pokeapi-codex:
    api_mode: codex_responses
    base_url: https://www.poke2api.com/v1
    api_key: YOUR_API_KEY
    default_model: gpt-5.5
    models:
      - gpt-5.5
EOF
```

:::

::: tip
请先在 [控制台](https://www.poke2api.com) 密钥管理获取对应分组的 API Key，并替换命令中的 `YOUR_API_KEY`。
:::

## 开始使用

```bash
hermes
```
