# Node.js 环境安装

在 Windows、macOS、Linux 上安装 Node.js LTS 版本，并验证 `node` / `npm` 命令可用。

::: tip 提示
后续的 Claude Code、Gemini CLI、Codex 等工具都依赖 Node.js 运行环境。请先完成本教程，再继续各自工具的配置。
:::

## Windows

### 方法一：官方下载（推荐）

前往 [Node.js 官网](https://nodejs.org) 下载 LTS 版本，双击安装包按提示安装即可。

### 方法二：使用 Chocolatey

```powershell
choco install nodejs-lts
```

### 方法三：使用 Scoop

```powershell
scoop install nodejs-lts
```

## macOS

### 方法一：使用 Homebrew（推荐）

```bash
brew install node
```

### 方法二：官方下载

前往 [Node.js 官网](https://nodejs.org) 下载 macOS 安装包。

## Linux

### Ubuntu / Debian

```bash
curl -fsSL https://deb.nodesource.com/setup_lts.x | sudo -E bash -
sudo apt-get install -y nodejs
```

### 其他发行版

可使用 [nvm](https://github.com/nvm-sh/nvm) 安装：

```bash
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.1/install.sh | bash
nvm install --lts
```

## 验证安装

```bash
node --version
npm --version
```

如果能输出版本号，说明 Node.js 与 npm 已可用。

## 下一步

- [Claude Code 配置](/guide/claude-code)
- [Gemini CLI 配置](/guide/gemini-cli)
- [Codex 配置](/guide/codex)
