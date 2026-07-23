# Codex (OpenAI) 配置教程

通过 Poke API 接入 Codex CLI。以下流程与本站当前安装脚本一致：**准备 → 安装/配置 → 启动 → 验证**。

## 准备

1. 安装 Node.js 与 npm；尚未安装时先完成 [Node.js 环境安装教程](/guide/nodejs)。
2. 在 [控制台](https://www.poke2api.com) 的密钥管理中创建或复制 API Key。
3. 准备可联网的 PowerShell、macOS 终端或 Linux Shell。

![Codex 官方交互界面示意](/images/codex/official-splash.webp)

*图：来源为 OpenAI 上游官方 Codex 演示素材，官方示意图，画面中的版本与模型仅代表素材采集时状态；采集日期 2026-07-15。上游终端 UI、模型名称和提示文案可能随版本变化，请以本机界面为准。*

## 安装/配置

运行对应的站内 `/install` 脚本；如需先审查内容，可直接打开 [Windows 脚本](https://docs.poke2api.com/install/codex.ps1) 或 [macOS / Linux 脚本](https://docs.poke2api.com/install/codex.sh)。

::: code-group

```powershell [Windows PowerShell]
irm https://docs.poke2api.com/install/codex.ps1 | iex
```

```bash [macOS / Linux]
curl -fsSL https://docs.poke2api.com/install/codex.sh | bash
```

:::

脚本会检查 Node.js 与 npm，在缺少 `codex` 命令时安装 `@openai/codex@latest`，随后完成以下配置：

1. 在交互式终端中隐藏读取 Poke API Key；输入不会回显。
2. 使用 `CODEX_HOME`，未设置时默认为 `~/.codex`。
3. 写入 `config.toml`；如旧文件存在，先创建带时间戳的备份，不删除整个配置目录。
4. 将密钥保存为用户环境变量 `POKE_API_KEY`，结束日志只显示变量名称，不显示密钥值。

为避免把未经确认的选项写进教程，下面只展示当前 `config.toml` schema 中可确认的连接字段：

```toml
model_provider = "pokeapi"

[model_providers.pokeapi]
base_url = "https://www.poke2api.com/v1"
wire_api = "responses"
env_key = "POKE_API_KEY"
requires_openai_auth = false
```

其中 `base_url` **必须带 `/v1`**；`env_key` 指向脚本安全写入的 `POKE_API_KEY`。`requires_openai_auth = false` 表示此 Provider 不走 OpenAI 账号登录认证。这里不保留未经当前 schema 确认的旧教程字段，也不创建包含明文密钥的 `auth.json`。

::: danger 保护 API Key
只在脚本的隐藏输入提示中粘贴 API Key。不要把真实密钥写进 TOML、命令、截图、聊天记录或可提交文件；也不要把 `POKE_API_KEY` 的值打印到验证日志中。
:::

## 启动

配置完成后重新打开终端。macOS / Linux 也可以按脚本末尾提示重新加载对应的 Shell 配置文件。进入项目目录后运行：

```bash
codex
```

Codex 将读取 `config.toml`，并通过 `POKE_API_KEY` 连接 Poke API。更多用法参考 [OpenAI 官方文档](https://platform.openai.com/docs)。

## 验证

先执行不会发出模型请求、也不会显示 API Key 的本地检查：

```bash
codex --version
codex --help
```

![Codex CLI 版本与帮助输出](/images/codex/terminal-version.png)

*图：来源为本站 Windows PowerShell 实拍，Codex CLI `0.144.0`；采集日期 2026-07-15。上游命令、帮助文本和终端 UI 可能随版本变化，请以本机实际输出为准。*

满足以下条件即完成基础验证：

- `codex --version` 能输出版本号；
- `codex --help` 能显示 CLI 用法；
- 重新运行 `codex` 后可进入交互界面，并能在不显示 API Key 的情况下完成一次普通请求。
