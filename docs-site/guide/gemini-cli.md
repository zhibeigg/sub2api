# Gemini CLI 配置教程

通过 Poke API 接入 Gemini CLI（Google 官方终端 AI 助手）。以下流程与本站当前安装脚本一致：**准备 → 安装/配置 → 启动 → 验证**。

## 准备

1. 安装 Node.js 与 npm；尚未安装时先完成 [Node.js 环境安装教程](/guide/nodejs)。
2. 在 [控制台](https://www.poke2api.com) 的密钥管理中创建或复制 API Key。
3. 准备可联网的 PowerShell、macOS 终端或 Linux Shell。

![Gemini CLI 官方交互界面示意](/images/gemini-cli/official-interface.webp)

*图：来源为 Google 上游 Gemini CLI 官方演示素材，官方示意图，画面中的模型与功能仅代表素材采集时状态；采集日期 2026-07-15。上游终端 UI、模型名称和提示文案可能随版本变化，请以本机界面为准。*

## 安装/配置

运行对应的站内 `/install` 脚本；如需先审查内容，可直接打开 [Windows 脚本](https://docs.poke2api.com/install/gemini.ps1) 或 [macOS / Linux 脚本](https://docs.poke2api.com/install/gemini.sh)。

::: code-group

```powershell [Windows PowerShell]
irm https://docs.poke2api.com/install/gemini.ps1 | iex
```

```bash [macOS / Linux]
curl -fsSL https://docs.poke2api.com/install/gemini.sh | bash
```

:::

脚本会检查 Node.js 与 npm，在缺少 `gemini` 命令时安装 `@google/gemini-cli@latest`，然后在交互式终端中隐藏读取 Poke API Key。输入不会回显，完成日志也不会显示密钥值。

脚本写入的三个变量如下：

| 变量 | 值 |
| --- | --- |
| `GEMINI_API_KEY` | 隐藏输入的 Poke API Key |
| `GEMINI_MODEL` | `gemini-3-pro-preview` |
| `GOOGLE_GEMINI_BASE_URL` | `https://www.poke2api.com` |

Windows 脚本写入当前用户环境变量；macOS / Linux 脚本写入 `~/.zshrc` 或 `~/.bashrc` 的受控配置区块。服务地址使用根地址，**不要添加 `/v1`**。

::: danger 保护 API Key
只在脚本的隐藏输入提示中粘贴 API Key。不要把真实密钥直接写进命令、截图、聊天记录或可提交文件；验证时也不要打印 `GEMINI_API_KEY` 的值。
:::

## 启动

配置完成后重新打开终端。macOS / Linux 也可以按脚本末尾提示重新加载对应的 Shell 配置文件。然后运行：

```bash
gemini
```

首次启动如果出现认证方式选择，请选择 **Use Gemini API key**，让 Gemini CLI 使用已经配置的 `GEMINI_API_KEY`。进入交互界面后即可在当前目录提问。更多用法参考 [Google AI 官方文档](https://ai.google.dev/gemini-api/docs)。

## 验证

先执行不会发出模型请求、也不会显示 API Key 的本地检查：

```bash
gemini --version
gemini --help
```

![Gemini CLI 版本与帮助输出](/images/gemini-cli/terminal-version.png)

*图：来源为本站 Windows PowerShell 实拍，Gemini CLI `0.32.1`；采集日期 2026-07-15。上游命令、帮助文本和终端 UI 可能随版本变化，请以本机实际输出为准。*

满足以下条件即完成基础验证：

- `gemini --version` 能输出版本号；
- `gemini --help` 能显示 CLI 用法；
- 重新运行 `gemini` 后可进入交互界面，并能在不显示 API Key 的情况下完成一次普通请求。
