# Claude Code 配置教程

通过 Poke API 接入 Claude Code。以下流程与本站当前的 Windows、macOS 和 Linux 安装脚本一致：**准备 → 安装/配置 → 启动 → 验证**。

## 准备

1. 在 [控制台](https://www.poke2api.com) 的密钥管理中创建或复制 API Key。
2. 准备可联网的 PowerShell、macOS 终端或 Linux Shell。
3. 确认当前用户可以写入用户环境变量或 Shell 配置文件。

::: warning Claude Code 已改用原生安装器
本站脚本调用 Anthropic 当前的原生安装器：Windows 使用 `https://claude.ai/install.ps1`，macOS / Linux 使用 `https://claude.ai/install.sh`。旧的 npm 全局安装方式已弃用，本教程不再使用 `npm install -g @anthropic-ai/claude-code`，也不要求为 Claude Code 预装 Node.js。
:::

![Claude Code 官方终端演示界面](/images/claude-code/official-demo-frame.webp)

*图：来源为 Anthropic 上游官方演示素材，官方示意图，画面未标注具体版本；采集日期 2026-07-15。上游终端 UI、快捷键和提示文案可能随版本变化，请以本机界面为准。*

## 安装/配置

运行对应的站内 `/install` 脚本；如需先审查内容，可直接打开 [Windows 脚本](https://docs.poke2api.com/install/claude-code.ps1) 或 [macOS / Linux 脚本](https://docs.poke2api.com/install/claude-code.sh)。

::: code-group

```powershell [Windows PowerShell]
irm https://docs.poke2api.com/install/claude-code.ps1 | iex
```

```bash [macOS / Linux]
curl -fsSL https://docs.poke2api.com/install/claude-code.sh | bash
```

:::

脚本会按顺序执行：

1. 检查 `claude` 命令；未安装时调用 Anthropic 官方原生安装器。
2. 在交互式终端中读取 Poke API Key。**输入过程不会回显，也不会在完成日志中显示密钥值。**
3. 写入以下连接配置：

| 配置 | 值 |
| --- | --- |
| `ANTHROPIC_BASE_URL` | `https://www.poke2api.com` |
| `ANTHROPIC_AUTH_TOKEN` | 隐藏输入的 Poke API Key |
| `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` | `1` |

Windows 脚本写入当前用户环境变量；macOS / Linux 脚本写入 `~/.zshrc` 或 `~/.bashrc` 的受控配置区块。`ANTHROPIC_BASE_URL` 使用根地址，**不要添加 `/v1`**。

::: danger 保护 API Key
只在脚本的隐藏输入提示中粘贴 API Key。不要把真实密钥写进命令、截图、聊天记录或可提交的配置文件；本站脚本的结束日志只显示认证变量名称，不显示变量值。
:::

## 启动

配置完成后重新打开终端。macOS / Linux 也可以按脚本末尾提示重新加载对应的 Shell 配置文件。然后进入项目目录运行：

```bash
claude
```

首次进入项目时，请按本机界面完成目录信任等确认。更多用法参考 [Anthropic 官方文档](https://platform.claude.com/docs/zh-CN/intro)。

## 验证

先确认版本和原生安装状态：

```bash
claude --version
claude doctor
```

Windows PowerShell 实拍结果如下；截图只包含版本与诊断信息，不包含 API Key。

![Claude Code 版本与原生安装诊断](/images/claude-code/terminal-version.png)

*图：来源为本站 Windows PowerShell 实拍，Claude Code `2.1.207`，原生安装状态；采集日期 2026-07-15。上游命令输出和诊断字段可能随版本变化，请以本机实际输出为准。*

满足以下条件即完成基础验证：

- `claude --version` 能输出版本号；
- `claude doctor` 显示原生安装可用且没有安装问题；
- 重新运行 `claude` 后能进入交互界面，并可在不暴露 API Key 的前提下正常发起请求。
