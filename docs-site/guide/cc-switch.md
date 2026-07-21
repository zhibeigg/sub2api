# CC-Switch 配置教程

CC-Switch 是一款 Claude Code / Codex / Gemini CLI 全方位辅助工具，用图形界面统一管理多个 AI CLI 的供应商配置、MCP 服务器、Skills 扩展和系统提示词。

使用 CC-Switch，你可以：

- ✅ **一键切换 API 配置** —— 在多个 API 提供商之间快速切换
- ✅ **可视化配置管理** —— 通过图形界面轻松管理所有配置
- ✅ **自定义供应商模板** —— 手动添加 Poke API 作为供应商，一次配置长期复用
- ✅ **MCP 服务器管理** —— 管理 Model Context Protocol 服务器
- ✅ **系统托盘快捷操作** —— 通过托盘菜单快速切换

::: tip
CC-Switch 是第三方开源工具（[GitHub](https://github.com/farion1231/cc-switch)）。本教程介绍如何在其中添加 Poke API 作为供应商。
:::

## 一、软件下载

前往 CC-Switch 的 [GitHub Release 页面](https://github.com/farion1231/cc-switch/releases) 下载对应系统的安装包。

### Windows

在 Release 页面滚动到最下方，选择适合的安装包。Windows 推荐下载普通 `msi` 后缀的安装包进行安装。安装后运行 CC-Switch 主程序。

### macOS

推荐使用 Homebrew 安装，打开终端后依次运行：

```bash
# 添加 tap 源
brew tap farion1231/ccswitch

# 安装 CC-Switch
brew install --cask cc-switch
```

安装完成后，在「启动台」或「应用程序」文件夹中找到 CC-Switch 并启动。

### Linux

Debian / Ubuntu 系统：

```bash
# 下载 .deb 包（将 x.x.x 替换为最新版本号）
wget https://github.com/farion1231/cc-switch/releases/latest/download/cc-switch_x.x.x_amd64.deb

# 安装
sudo dpkg -i cc-switch_x.x.x_amd64.deb
```

## 二、环境检查

::: warning 建议先完成环境检查
如果你能确认 Node.js 环境以及 claude / codex / gemini 的 CLI 安装没问题、配置目录也都存在，可以跳过此步。否则请先完成 [Node.js 环境安装](/guide/nodejs)，并按各工具教程安装对应 CLI：[Claude Code](/guide/claude-code)、[Codex](/guide/codex)、[Gemini CLI](/guide/gemini-cli)。
:::

## 三、Claude Code 配置

1. 打开 CC-Switch 软件，进入初始界面。
2. 在分组条中，将分组切换至 **Claude**。
3. 在供应商分组中，点击「新增供应商」添加一个自定义供应商。
4. 前往 [Poke API 控制台](https://www.poke2api.com) 创建 Claude（CC）分组的令牌，点击复制按钮，把 API Key 复制到剪贴板。
5. 在新增供应商的表单中填写：
   - **名称**：`Poke API`（可自定义）
   - **Base URL**：`https://www.poke2api.com`
   - **API Key**：粘贴你刚才复制的令牌
6. 点击右下角「添加」按钮保存。
7. 添加成功后，在主界面会看到配置好的供应商，点击右侧「启用」按钮，显示「使用中」即配置完成。
8. 点击左上角「设置」按钮，在通用页面下拉找到 **跳过 Claude Code 初次安装确认**，务必勾选。
9. 在终端运行 `claude`，看到对话界面并能正常回复即表示配置完成。

::: tip
Claude / Anthropic 通道使用根地址 `https://www.poke2api.com`，**不要**加 `/v1`。
:::

## 四、Codex 配置

1. 打开 CC-Switch 软件，进入初始界面。
2. 在分组条中，将分组切换至 **Codex**。
3. 在供应商分组中，点击「新增供应商」添加一个自定义供应商。
4. 前往 [Poke API 控制台](https://www.poke2api.com) 创建 Codex 分组的令牌，复制 API Key。
5. 在表单中填写：
   - **名称**：`Poke API`（可自定义）
   - **Base URL**：`https://www.poke2api.com/v1`
   - **API Key**：粘贴你复制的令牌
6. 点击右下角「添加」按钮保存。
7. 在主界面点击右侧「启用」按钮，显示「使用中」即配置完成。
8. 在终端运行 `codex`，看到对话界面并能正常回复即表示配置完成。

::: tip
OpenAI / Codex 通道使用 `https://www.poke2api.com/v1`，**必须**带 `/v1`。
:::

## 五、Gemini 配置

1. 打开 CC-Switch 软件，进入初始界面。
2. 在分组条中，将分组切换至 **Gemini**。
3. 在供应商分组中，点击「新增供应商」添加一个自定义供应商。
4. 前往 [Poke API 控制台](https://www.poke2api.com) 创建 Gemini 分组的令牌，复制 API Key。
5. 在表单中填写：
   - **名称**：`Poke API`（可自定义）
   - **Base URL**：`https://www.poke2api.com`
   - **API Key**：粘贴你复制的令牌
6. 点击右下角「添加」按钮保存。
7. 在主界面点击右侧「启用」按钮，显示「使用中」即配置完成。
8. 在终端运行 `gemini`，看到对话界面并能正常回复即表示配置完成。

## 六、余额与用量显示

Sub2API 提供 Bearer API Key 鉴权的余额接口：

```http
GET /v1/usage
Authorization: Bearer <API_KEY>
Accept: application/json
```

响应通过以下字段进行强识别：

```json
{
  "object": "sub2api.key_usage",
  "schema_version": 1,
  "mode": "quota_limited",
  "isValid": true
}
```

- `quota_limited`：使用 `quota.limit / used / remaining / unit`。
- `unrestricted` + `subscription`：取日、周、月有限窗口中最小剩余额度。
- `unrestricted` + `balance`：使用钱包余额。
- 没有任何有限订阅窗口时，顶层 `remaining=-1` 表示无限。
- 响应设置 `Cache-Control: no-store`，客户端不应持久化余额响应或 API Key。

管理端池模式账户也使用同一兼容契约验证自定义上游真实余额。查询失败时显示“未知”或带 `stale` 标记的最近成功快照，不会回退为本地账户额度。原生 AWS Bedrock SigV4 没有通用余额端点，因此不支持；只有 Bedrock API Key/自定义 Base URL 明确兼容 Bearer `/v1/usage` 时才会尝试。

## 七、CLI 版本

如果你更常在服务器、SSH、终端等环境工作，CC-Switch 也提供了命令行版本，适合自动化场景。详见 [CC-Switch CLI 使用](/guide/cc-switch-cli)。
