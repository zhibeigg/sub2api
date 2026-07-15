# Node.js 环境安装

在 Windows、macOS、Linux 上安装当前 Node.js LTS，并确认 `node` / `npm` 命令可用。Codex、Gemini CLI 与 OpenClaw 都依赖 Node.js；Claude Code 当前推荐使用原生安装器，不再强制依赖 Node.js。

::: tip 版本建议
优先选择 Node.js 官网标注为 **LTS** 的版本。截图中的具体小版本只代表拍摄当日，实际安装时以官网当前 LTS 为准。
:::

## 一、从官网下载 LTS

打开 [Node.js 官方下载页](https://nodejs.org/en/download)，确认版本旁带有 **LTS** 标记，再选择你的操作系统和安装方式。

<figure class="tutorial-media">
  <img src="/images/nodejs/download-lts.png" alt="Node.js 官方下载页真实截图，已选择 Windows 与 LTS 版本" loading="lazy">
  <figcaption>真实网页：Node.js 官方下载页会随系统与发布时间更新。截图拍摄于 2026 年 7 月，示例选择 Windows 与当时的 LTS。</figcaption>
</figure>

## 二、安装 Node.js

### Windows

最简单的方式是从官网下载安装包并保持默认选项；也可以使用包管理器：

::: code-group

```powershell [Chocolatey]
choco install nodejs-lts
```

```powershell [Scoop]
scoop install nodejs-lts
```

:::

安装结束后请**重新打开 PowerShell 或 Windows Terminal**，让新的 `PATH` 生效。

### macOS

::: code-group

```bash [Homebrew]
brew install node
```

```text [官方下载]
打开 https://nodejs.org/en/download
选择 macOS 与当前 LTS 安装包
```

:::

### Linux

Ubuntu / Debian 可以使用 NodeSource，也可以使用 `nvm`：

::: code-group

```bash [NodeSource]
curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
sudo apt-get install -y nodejs
```

```bash [nvm]
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash
nvm install --lts
```

:::

## 三、验证安装

在一个新终端中依次运行：

```bash
node --version
npm --version
```

Windows PowerShell 还可以确认命令已被系统识别：

```powershell
Get-Command node | Select-Object Name, CommandType
```

<figure class="tutorial-media tutorial-media--terminal">
  <img src="/images/nodejs/version-check.png" alt="Windows Terminal 中真实执行 node version、npm version 和 Get-Command node 的结果" loading="lazy">
  <figcaption>实机截图：Windows 11 / Windows Terminal，Node.js v22.16.0、npm 11.8.0。你的版本号可以不同，只要命令正常返回即可。</figcaption>
</figure>

如果提示“找不到命令”，先关闭并重新打开终端；仍无效时，重新安装 Node.js，并确认安装器已把 Node.js 加入 `PATH`。

## 四、继续配置工具

- [Codex 配置](/guide/codex)
- [Gemini CLI 配置](/guide/gemini-cli)
- [OpenClaw 配置](/guide/openclaw)
- [Claude Code 配置](/guide/claude-code)（当前使用原生安装器）
