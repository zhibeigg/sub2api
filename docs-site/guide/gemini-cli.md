# Gemini CLI 配置教程

通过 Poke API 接入 Gemini CLI（Google 官方终端 AI 助手）。推荐使用一键脚本，脚本会自动检测 Node.js、安装 Gemini CLI、提示输入 API Key 并写入配置。

::: warning 前置条件
Gemini CLI 依赖 Node.js 运行环境。一键脚本会检测并提示，若未安装请先完成 [Node.js 环境安装教程](/guide/nodejs)。
:::

## 一键脚本配置（推荐）

::: code-group

```powershell [Windows PowerShell]
irm https://docs.poke2api.com/install/gemini.ps1 | iex
```

```bash [macOS / Linux / WSL]
curl -fsSL https://docs.poke2api.com/install/gemini.sh | bash
```

:::

::: tip
脚本执行后会提示输入 API Key，请在 [控制台](https://www.poke2api.com) 密钥管理中获取。配置写入后，已打开的终端需重启后才会生效。
:::

## 手动配置（备用）

### 第一步：安装 Gemini CLI

::: code-group

```powershell [Windows]
npm install -g @google/gemini-cli
```

```bash [Linux]
sudo npm install -g @google/gemini-cli
```

```bash [macOS (npm)]
sudo npm install -g @google/gemini-cli
```

```bash [macOS (Homebrew)]
brew install gemini-cli
```

:::

验证安装：

```bash
gemini --version
```

### 第二步：配置环境变量

设置环境变量让 Gemini CLI 连接到 Poke API 平台，需要设置三个变量：

- `GOOGLE_GEMINI_BASE_URL` —— 服务地址
- `GEMINI_API_KEY` —— 你的 API Key
- `GEMINI_MODEL` —— 使用的模型

#### 临时设置（当前会话）

::: code-group

```powershell [Windows]
$env:GOOGLE_GEMINI_BASE_URL = "https://www.poke2api.com"
$env:GEMINI_API_KEY = "YOUR_API_KEY"
$env:GEMINI_MODEL = "gemini-3-pro-preview"
```

```bash [macOS]
export GOOGLE_GEMINI_BASE_URL="https://www.poke2api.com"
export GEMINI_API_KEY="YOUR_API_KEY"
export GEMINI_MODEL="gemini-3-pro-preview"
```

```bash [Linux]
export GOOGLE_GEMINI_BASE_URL="https://www.poke2api.com"
export GEMINI_API_KEY="YOUR_API_KEY"
export GEMINI_MODEL="gemini-3-pro-preview"
```

:::

#### 永久设置

::: code-group

```powershell [Windows]
[System.Environment]::SetEnvironmentVariable("GOOGLE_GEMINI_BASE_URL", "https://www.poke2api.com", [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("GEMINI_API_KEY", "YOUR_API_KEY", [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("GEMINI_MODEL", "gemini-3-pro-preview", [System.EnvironmentVariableTarget]::User)
```

```bash [Linux (bash)]
echo 'export GOOGLE_GEMINI_BASE_URL="https://www.poke2api.com"' >> ~/.bashrc
echo 'export GEMINI_API_KEY="YOUR_API_KEY"' >> ~/.bashrc
echo 'export GEMINI_MODEL="gemini-3-pro-preview"' >> ~/.bashrc
source ~/.bashrc
```

```bash [macOS (zsh)]
echo 'export GOOGLE_GEMINI_BASE_URL="https://www.poke2api.com"' >> ~/.zshrc
echo 'export GEMINI_API_KEY="YOUR_API_KEY"' >> ~/.zshrc
echo 'export GEMINI_MODEL="gemini-3-pro-preview"' >> ~/.zshrc
source ~/.zshrc
```

:::

::: tip
请将命令中的 `YOUR_API_KEY` 替换为在 [控制台](https://www.poke2api.com) 密钥管理中复制的任意 API Key。如需更换模型，只需修改 `GEMINI_MODEL` 即可。
:::

## 开始使用

配置完成后，在项目目录中运行：

```bash
gemini
```

非交互式查询测试：

```bash
gemini -p "你好"
```

更多用法参考 [Google AI 官方文档](https://ai.google.dev/gemini-api/docs)。
