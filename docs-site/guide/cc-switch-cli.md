# CC-Switch CLI 使用

CC-Switch CLI 是 CC-Switch 的命令行版本，是 Claude Code、Codex、Gemini、OpenCode 与 OpenClaw 的命令行管理工具，适合服务器、SSH、macOS 终端和自动化场景使用。

::: tip
如果你更习惯图形界面，可以使用 [CC-Switch 图形版教程](/guide/cc-switch)。CC-Switch CLI 是第三方开源工具（[GitHub](https://github.com/SaladDay/cc-switch-cli)）。
:::

## CC-Switch CLI 是什么

它包含两部分：

- **完整 CLI 命令**：用命令完成 Provider 列表查看、切换、环境检查、MCP 同步、Skills 管理、提示词管理、本地代理等操作。
- **完整 TUI 界面**：运行 `cc-switch` 后进入终端图形界面，可以像桌面版一样新增 Provider、填写 API Key、保存并切换配置。

如果你只是第一次配置 Poke API，推荐先用 TUI。配置完成后，日常切换、检查和排错可以直接用 CLI 命令完成。

## 安装 CC-Switch CLI

macOS 和 Linux 推荐使用一键安装脚本：

```bash
curl -fsSL https://github.com/SaladDay/cc-switch-cli/releases/latest/download/install.sh | bash
```

默认会安装到 `~/.local/bin`。如果终端提示找不到 `cc-switch`，请确认 `~/.local/bin` 已加入 `PATH`。

### 手动安装

::: code-group

```bash [macOS]
curl -LO https://github.com/saladday/cc-switch-cli/releases/latest/download/cc-switch-cli-darwin-universal.tar.gz
tar -xzf cc-switch-cli-darwin-universal.tar.gz
chmod +x cc-switch
sudo mv cc-switch /usr/local/bin/

# 如遇 “无法验证开发者” 提示
xattr -cr /usr/local/bin/cc-switch
```

```bash [Linux x64]
curl -LO https://github.com/saladday/cc-switch-cli/releases/latest/download/cc-switch-cli-linux-x64-musl.tar.gz
tar -xzf cc-switch-cli-linux-x64-musl.tar.gz
chmod +x cc-switch
sudo mv cc-switch /usr/local/bin/
```

```bash [Linux ARM64]
curl -LO https://github.com/saladday/cc-switch-cli/releases/latest/download/cc-switch-cli-linux-arm64-musl.tar.gz
tar -xzf cc-switch-cli-linux-arm64-musl.tar.gz
chmod +x cc-switch
sudo mv cc-switch /usr/local/bin/
```

:::

**Windows**：前往 [GitHub Releases](https://github.com/SaladDay/cc-switch-cli/releases) 下载 `cc-switch-cli-windows-x64.zip`，解压后把 `cc-switch.exe` 放到 `PATH` 目录中，或直接在当前目录运行 `.\cc-switch.exe`。

<figure class="tutorial-media tutorial-media--terminal">
  <img src="/images/cc-switch-cli/release-download.png" alt="Windows Terminal 中通过 GitHub CLI 查询 CC-Switch CLI v5.9.0 与 Windows 发布文件" loading="lazy">
  <figcaption>真实发布信息：通过 GitHub CLI 查询维护仓库的 v5.9.0 Release，可看到 Windows x64 压缩包与校验文件。版本更新后文件名中的版本号会变化。</figcaption>
</figure>

## 两种使用方式

### 进入 TUI 界面

```bash
cc-switch
```

如果要直接配置某个应用，可以加 `--app`：

```bash
cc-switch --app claude
cc-switch --app codex
cc-switch --app gemini
```

TUI 适合第一次配置。你可以在里面新增供应商，填入 API Key，然后保存并切换到该 Provider。

<figure class="tutorial-media tutorial-media--terminal">
  <img src="/images/cc-switch-cli/provider-tui.png" alt="CC-Switch CLI v5.9.0 的真实交互式 TUI 首页，包含应用标签、供应商入口和环境检查" loading="lazy">
  <figcaption>实机截图：CC-Switch CLI v5.9.0，使用独立的 <code>CC_SWITCH_CONFIG_DIR</code> 空白配置拍摄，不包含个人 Provider、API Key、MCP 或会话数据。</figcaption>
</figure>

### 使用 CLI 命令

```bash
cc-switch provider list
cc-switch provider current
cc-switch provider switch <id>
cc-switch env tools
cc-switch env check
```

`claude` 是默认应用。管理其他应用时使用 `--app`：

```bash
cc-switch --app codex provider list
cc-switch --app gemini provider current
```

CLI 命令适合服务器、脚本和日常排错，也适合交给 Claude Code / Codex 直接执行。

## 配置前准备

请先确认目标 CLI 已经安装：

```bash
cc-switch env tools
```

<figure class="tutorial-media tutorial-media--terminal">
  <img src="/images/cc-switch-cli/env-tools.png" alt="CC-Switch CLI 真实执行 version 与 env tools 后输出本地 CLI 检测表" loading="lazy">
  <figcaption>实机截图：<code>env tools</code> 会逐项显示本机工具状态。出现 <code>program not found</code> 或 <code>not installed</code> 时，先完成对应 CLI 的安装再继续。</figcaption>
</figure>

建议先运行一次目标 CLI 或帮助命令，让它创建自己的配置目录：

```bash
claude --help
codex --help
gemini --help
```

然后在 [Poke API 控制台](https://www.poke2api.com) 创建对应分组的令牌：

- **Claude Code**：创建 Claude（CC）分组令牌
- **Codex**：创建 Codex 分组令牌
- **Gemini**：创建 Gemini 分组令牌

## 配置 Poke API

第一次配置推荐使用 TUI。下面以 Claude Code 为例，Codex 和 Gemini 的配置方式相同，只需要用 `--app codex` 或 `--app gemini` 切换目标应用。

运行以下命令进入交互界面：

```bash
cc-switch
```

如果要直接配置 Codex 或 Gemini：

```bash
cc-switch --app codex
cc-switch --app gemini
```

配置步骤：

1. 在左侧选择 **Providers**，进入供应商管理页面，然后新增供应商。
2. 填写供应商信息：
   - **名称**：`Poke API`
   - **Base URL**：Claude 用 `https://www.poke2api.com`；Codex 用 `https://www.poke2api.com/v1`；Gemini 用 `https://www.poke2api.com`
   - **API Key**：填入你从 Poke API 控制台复制的令牌
3. 保存后，回到供应商列表，确认当前选中的是刚刚添加的 Poke API Provider。
4. 如果配置的是 Claude Code，进入 **设置**，找到 **跳过 Claude Code 初次安装确认** 并开启。这个选项会向 `~/.claude.json` 写入 `hasCompletedOnboarding=true`，避免 Claude Code 首次启动时停在安装确认流程。
5. 打开对应 CLI 测试是否可以正常对话：

```bash
claude   # 或 codex / gemini
```

## 检查 Sub2API 余额接口

配置 Provider 后，可以直接用 API Key 验证 Sub2API 的余额契约：

```bash
curl -sS \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H "Accept: application/json" \
  "$SUB2API_BASE_URL/v1/usage"
```

有效响应包含 `object: sub2api.key_usage`、`schema_version: 1`、合法的 `mode` 和 `isValid`。余额可能来自 Key 配额、订阅最紧窗口或钱包；没有有限订阅窗口时 `remaining=-1` 表示无限。

如果把 Sub2API 作为另一个 Sub2API 实例的池模式自定义上游，该实例也会调用同一端点验证真实余额。401/403 表示 Key 无权访问，404/405 表示上游不支持该契约，429 或网络超时会显示为未知，若有最近成功值则仅标记 `stale`。不要把 AWS Access Key/Secret 当作 Bearer Key 测试原生 Bedrock。

## 常用命令

```bash
cc-switch                         # 进入交互界面
cc-switch env tools               # 检查本地 CLI 是否安装
cc-switch env check               # 检查环境变量冲突

cc-switch provider list           # 查看 Claude 供应商
cc-switch provider current        # 查看当前 Claude 供应商
cc-switch provider switch <id>    # 切换 Claude 供应商

cc-switch --app codex provider list
cc-switch --app gemini provider list

cc-switch provider stream-check <id>  # 检查供应商流式响应
cc-switch provider fetch-models <id>  # 拉取远端模型列表
cc-switch update                      # 更新 CC-Switch CLI
```

管理 Codex、Gemini、OpenCode 或 OpenClaw 时，请使用全局参数 `--app` 指定目标应用。

## 高级玩法：让 AI 助手操作 CC-Switch CLI

如果你已经在 Claude Code 或 Codex 中工作，也可以直接让它们调用 `cc-switch` 命令来检查和切换配置。例如你可以这样说：

- 帮我运行 `cc-switch provider list`，看一下当前有哪些 Claude Provider。
- 帮我运行 `cc-switch --app codex provider current`，确认 Codex 当前是不是 Poke API。
- 帮我运行 `cc-switch env check --app claude`，检查有没有环境变量覆盖配置。
- 帮我切换到 Poke API provider，然后运行 `claude` 测试是否能正常回复。

AI 助手负责执行命令和解释结果，你只需要确认关键操作，比如切换 Provider、覆盖配置文件或删除配置。

## 常见问题

### 切换 Provider 后没有生效

请先确认目标 CLI 已经初始化配置目录，可以运行一次：

```bash
claude --help
codex --help
gemini --help
```

然后重新切换一次 Provider。

### 环境变量覆盖了配置

如果系统里设置了 `ANTHROPIC_API_KEY`、`OPENAI_API_KEY`、`GEMINI_API_KEY` 等环境变量，目标 CLI 可能会优先读取环境变量，导致 CC-Switch CLI 写入的配置没有生效。可以运行：

```bash
cc-switch env check --app claude
cc-switch env check --app codex
cc-switch env check --app gemini
```
