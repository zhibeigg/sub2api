# Claude Code 配置教程

通过 Poke API 接入 Claude Code。推荐使用一键脚本，脚本会自动检测 Node.js、安装 Claude Code、提示输入 API Key 并写入配置。

::: warning 前置条件
Claude Code 依赖 Node.js 运行环境。一键脚本会检测并提示，若未安装请先完成 [Node.js 环境安装教程](/guide/nodejs)。
:::

## 一键脚本配置（推荐）

复制对应系统命令，运行后按提示输入 API Key 即可。

::: code-group

```powershell [Windows PowerShell]
irm https://docs.poke2api.com/install/claude-code.ps1 | iex
```

```bash [macOS / Linux / WSL]
curl -fsSL https://docs.poke2api.com/install/claude-code.sh | bash
```

:::

::: tip
脚本执行后会提示输入 API Key，请在 [控制台](https://www.poke2api.com) 密钥管理中获取。配置写入后，已打开的终端需重启后才会读取新配置。
:::

## 手动配置（备用）

当需要审查命令或一键脚本不可用时，可按以下步骤手动配置。

### 第一步：安装 Claude Code

::: code-group

```powershell [Windows]
npm install -g @anthropic-ai/claude-code
```

```bash [Linux]
sudo npm install -g @anthropic-ai/claude-code
```

```bash [macOS]
sudo npm install -g @anthropic-ai/claude-code
```

:::

验证安装：

```bash
claude --version
```

### 第二步：配置环境变量

把下面的值写入当前终端或系统环境变量：

| 变量 | 值 |
| --- | --- |
| `ANTHROPIC_BASE_URL` | `https://www.poke2api.com` |
| `ANTHROPIC_AUTH_TOKEN` | 在控制台密钥管理获取 |
| `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` | `1` |

::: warning
- `ANTHROPIC_BASE_URL` 使用根地址，**不要**加 `/v1`。
- 请保留 `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`，用于关闭 Claude Code 的非必要遥测流量。
:::

#### 临时设置（当前会话）

::: code-group

```bat [Windows CMD]
set ANTHROPIC_BASE_URL=https://www.poke2api.com
set ANTHROPIC_AUTH_TOKEN=your-key
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
claude
```

```powershell [PowerShell]
$env:ANTHROPIC_BASE_URL="https://www.poke2api.com"
$env:ANTHROPIC_AUTH_TOKEN="your-key"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"
claude
```

```bash [Mac / Linux]
export ANTHROPIC_BASE_URL="https://www.poke2api.com"
export ANTHROPIC_AUTH_TOKEN="your-key"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"
claude
```

:::

#### 永久设置

::: code-group

```powershell [PowerShell]
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_BASE_URL", "https://www.poke2api.com", [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("ANTHROPIC_AUTH_TOKEN", "your-key", [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC", "1", [System.EnvironmentVariableTarget]::User)
```

```bash [Linux (bash)]
echo 'export ANTHROPIC_BASE_URL="https://www.poke2api.com"' >> ~/.bashrc
echo 'export ANTHROPIC_AUTH_TOKEN="your-key"' >> ~/.bashrc
echo 'export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"' >> ~/.bashrc
source ~/.bashrc
```

```bash [macOS (zsh)]
echo 'export ANTHROPIC_BASE_URL="https://www.poke2api.com"' >> ~/.zshrc
echo 'export ANTHROPIC_AUTH_TOKEN="your-key"' >> ~/.zshrc
echo 'export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1"' >> ~/.zshrc
source ~/.zshrc
```

:::

::: tip
请将命令中的 `your-key` 替换为在 [控制台](https://www.poke2api.com) 密钥管理中复制的任意 API Key。修改环境变量后，已打开的终端需重启才会生效。
:::

## 开始使用

进入项目目录后运行：

```bash
claude
```

启动后，Claude Code 会分析当前目录的代码，并提供智能编程辅助。更多用法参考 [Anthropic 官方文档](https://platform.claude.com/docs/zh-CN/intro)。
