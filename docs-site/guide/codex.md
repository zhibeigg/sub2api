# Codex (OpenAI) 配置教程

通过 Poke API 接入 Codex。推荐使用一键脚本，脚本会自动检测 Node.js、安装 Codex、提示输入 API Key 并写入 `~/.codex` 配置。

::: warning 前置条件
Codex 依赖 Node.js 运行环境。一键脚本会检测并提示，若未安装请先完成 [Node.js 环境安装教程](/guide/nodejs)。
:::

## 一键脚本配置（推荐）

::: code-group

```powershell [Windows PowerShell]
irm https://docs.poke2api.com/install/codex.ps1 | iex
```

```bash [macOS / Linux / WSL]
curl -fsSL https://docs.poke2api.com/install/codex.sh | bash
```

:::

::: tip
脚本执行后会提示输入 API Key，请在 [控制台](https://www.poke2api.com) 密钥管理中获取。配置写入 `~/.codex` 后，已打开的 Codex 需要重启才会读取新配置。
:::

## 手动配置（备用）

### 第一步：安装 Codex

::: code-group

```powershell [Windows]
npm install -g @openai/codex@latest
```

```bash [Linux]
sudo npm install -g @openai/codex@latest
```

```bash [macOS]
sudo npm install -g @openai/codex@latest
```

:::

验证安装：

```bash
codex --version
```

### 第二步：创建配置文件

Codex 使用配置文件进行连接设置，需要创建 `config.toml` 和 `auth.json` 两个文件。注意 OpenAI / Codex 接口地址**必须**带 `/v1`。

::: code-group

```bash [macOS / Linux]
# 删除旧目录并创建新目录
rm -rf ~/.codex && mkdir -p ~/.codex

# 创建 config.toml
cat > ~/.codex/config.toml << 'EOF'
model_provider = "codex"
model = "gpt-5.5"
review_model = "gpt-5.5"
model_reasoning_effort = "high"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 270000
model_auto_compact_token_limit = 270000
effective_context_window_percent = 95

[model_providers.codex]
name = "codex"
base_url = "https://www.poke2api.com/v1"
wire_api = "responses"
requires_openai_auth = true
EOF

# 创建 auth.json
cat > ~/.codex/auth.json << 'EOF'
{
  "OPENAI_API_KEY": "YOUR_API_KEY"
}
EOF
```

```powershell [Windows PowerShell]
# 删除旧目录并创建新目录
if (Test-Path "$env:USERPROFILE\.codex") { Remove-Item -Recurse -Force "$env:USERPROFILE\.codex" }
mkdir "$env:USERPROFILE\.codex"

# 创建 config.toml
@"
model_provider = "codex"
model = "gpt-5.5"
review_model = "gpt-5.5"
model_reasoning_effort = "high"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 270000
model_auto_compact_token_limit = 270000
effective_context_window_percent = 95

[model_providers.codex]
name = "codex"
base_url = "https://www.poke2api.com/v1"
wire_api = "responses"
requires_openai_auth = true
"@ | Out-File -FilePath "$env:USERPROFILE\.codex\config.toml" -Encoding utf8

# 创建 auth.json
@"
{
  "OPENAI_API_KEY": "YOUR_API_KEY"
}
"@ | Out-File -FilePath "$env:USERPROFILE\.codex\auth.json" -Encoding utf8
```

:::

::: tip
请将 `YOUR_API_KEY` 替换为在 [控制台](https://www.poke2api.com) 密钥管理中复制的任意 API Key。
:::

## 开始使用

进入项目目录后运行：

```bash
codex
```

更多用法参考 [OpenAI 官方文档](https://platform.openai.com/docs)。
