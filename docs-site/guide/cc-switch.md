# CC-Switch 配置教程

CC-Switch 是一款 Claude Code、Codex、Gemini CLI 等 AI CLI 的图形化配置管理工具。它可以统一管理供应商、API Key、MCP、Skills 和系统提示词，并在多个配置之间快速切换。

完成本教程后，你可以：

- 从 Poke API 控制台把密钥一键导入 CC-Switch
- 手动配置 Claude Code、Codex 或 Gemini CLI
- 在 CC-Switch 中切换并启用供应商
- 完成 Claude Code 首次启动设置并验证连接

::: tip 截图说明
本文使用 CC-Switch 官方仓库提供的真实中文界面截图，不使用生成式示意图。软件持续更新时，按钮位置或文案可能略有变化，请以当前版本界面为准。
:::

## 两分钟视频教程

<div class="tutorial-video">
  <iframe
    src="https://player.bilibili.com/player.html?bvid=BV1iAMn6gEUx&page=1&high_quality=1&danmaku=0&autoplay=0"
    title="2分钟教会新手如何使用 CC-Switch"
    loading="lazy"
    scrolling="no"
    frameborder="0"
    allow="fullscreen; autoplay; encrypted-media; picture-in-picture"
    allowfullscreen="true"
  ></iframe>
</div>
<p class="tutorial-media-caption">视频：PixForge《2分钟教会新手如何使用 最强AI管理工具 CC-Switch》。播放器无法加载时，可<a href="https://www.bilibili.com/video/BV1iAMn6gEUx/" target="_blank" rel="noreferrer">前往哔哩哔哩观看</a>。</p>

::: warning
视频录制时间、站点域名和软件界面可能与当前版本不同。配置地址请以本文的“连接信息速查”表格为准。
:::

## 一、安装 CC-Switch

CC-Switch 是第三方开源工具。请从以下官方入口下载，不要使用来源不明的安装包：

- [CC-Switch 官方网站](https://ccswitch.io)
- [GitHub Releases](https://github.com/farion1231/cc-switch/releases)

### Windows

在 Releases 页面下载：

- `CC-Switch-v{版本号}-Windows.msi`：安装版，推荐普通用户使用
- `CC-Switch-v{版本号}-Windows-Portable.zip`：绿色版，解压后直接运行

### macOS

推荐使用 Homebrew：

```bash
brew install --cask cc-switch
```

也可以在 Releases 页面下载 `CC-Switch-v{版本号}-macOS.dmg`。

### Linux

在 Releases 页面按系统和 CPU 架构选择安装包：

- Debian / Ubuntu：`.deb`
- Fedora / RHEL / openSUSE：`.rpm`
- 其他桌面发行版：`.AppImage`

安装后至少启动一次 CC-Switch。这样系统才能正确注册 `ccswitch://` 协议，供 Poke API 控制台执行一键导入。

<figure class="tutorial-media">
  <img src="/images/cc-switch/main-interface.png" alt="CC-Switch 中文主界面，顶部可切换 Claude、Codex 和 Gemini，右上角有新增供应商按钮" loading="lazy">
  <figcaption>真实界面：顶部选择要管理的客户端，右上角“+”用于新增供应商。</figcaption>
</figure>

## 二、配置前检查

请先确认目标 CLI 已经安装并至少运行过一次：

```bash
claude --help
codex --help
gemini --help
```

如果命令不存在，请先查看对应教程：

- [Node.js 环境安装](/guide/nodejs)
- [Claude Code 安装](/guide/claude-code)
- [Codex 安装](/guide/codex)
- [Gemini CLI 安装](/guide/gemini-cli)

然后前往 [Poke API 控制台](https://www.poke2api.com)，创建与目标客户端匹配的密钥。

## 三、连接信息速查

| 目标客户端 | 创建密钥时选择的分组 | CC-Switch 应用 | Base URL / Endpoint |
| --- | --- | --- | --- |
| Claude Code | Claude（CC） | Claude | `https://www.poke2api.com` |
| Codex | Codex | Codex | `https://www.poke2api.com/v1` |
| Gemini CLI | Gemini | Gemini | `https://www.poke2api.com` |

::: danger 地址不要填错
Claude Code 和 Gemini CLI 使用根地址，不要添加 `/v1`；Codex 必须使用带 `/v1` 的地址。
:::

## 四、推荐方式：从控制台一键导入

这是最适合新手的方式，Base URL、目标应用和用量查询配置会自动生成。

1. 启动 CC-Switch，并保持它在后台运行。
2. 登录 Poke API 控制台，进入“API 密钥”页面。
3. 找到要使用的密钥，点击操作栏中的“导入到 CCS”。
4. 浏览器询问是否打开 CC-Switch 时，选择允许。
5. 在 CC-Switch 的导入确认页面检查应用、名称和 Endpoint。
6. 确认导入，回到供应商列表后点击“启用”。
7. 供应商卡片显示“当前使用”或“使用中”后，重新打开目标 CLI 进行测试。

::: info
如果站点管理员隐藏了“导入到 CCS”按钮，或浏览器提示没有应用可以打开 `ccswitch://`，请使用下方手动配置方式。
:::

## 五、备用方式：手动添加供应商

在 CC-Switch 顶部先选择 Claude、Codex 或 Gemini，然后点击右上角“+”。在预设供应商区域选择“自定义配置”，再填写名称、Endpoint 和 API Key。

<figure class="tutorial-media">
  <img src="/images/cc-switch/add-provider.png" alt="CC-Switch 添加 Claude Code 供应商的真实界面，预设列表左上角有自定义配置按钮" loading="lazy">
  <figcaption>真实界面：进入新增供应商页面后，选择左上角“自定义配置”。截图中的其他供应商仅用于展示界面布局。</figcaption>
</figure>

### Claude Code

1. 在顶部应用切换器选择 **Claude**。
2. 点击“+”，选择“自定义配置”。
3. 填写以下内容：
   - 供应商名称：`Poke API`
   - Base URL / Endpoint：`https://www.poke2api.com`
   - API Key：粘贴 Claude（CC）分组的密钥
4. 保存后返回供应商列表，点击“启用”。
5. 打开左上角设置，在“通用”页面开启“跳过 Claude Code 初次安装确认”。
6. 重新运行 `claude` 测试。

<figure class="tutorial-media">
  <img src="/images/cc-switch/skip-claude-intro.png" alt="CC-Switch 设置页面中已开启跳过 Claude Code 初次安装确认" loading="lazy">
  <figcaption>真实界面：Claude Code 第一次启动前，建议开启“跳过 Claude Code 初次安装确认”。</figcaption>
</figure>

### Codex

1. 在顶部应用切换器选择 **Codex**。
2. 点击“+”，选择“自定义配置”。
3. 填写以下内容：
   - 供应商名称：`Poke API`
   - Base URL / Endpoint：`https://www.poke2api.com/v1`
   - API Key：粘贴 Codex 分组的密钥
4. 如果表单要求选择模型，请选择控制台当前支持的 Codex 模型。
5. 保存并启用供应商。
6. 关闭正在运行的 Codex 和旧终端窗口，重新打开终端后运行 `codex`。

### Gemini CLI

1. 在顶部应用切换器选择 **Gemini**。
2. 点击“+”，选择“自定义配置”。
3. 填写以下内容：
   - 供应商名称：`Poke API`
   - Base URL / Endpoint：`https://www.poke2api.com`
   - API Key：粘贴 Gemini 分组的密钥
4. 保存并启用供应商。
5. 重新运行 `gemini` 测试。

## 六、验证是否配置成功

分别启动你配置的客户端，并发送一句简单测试消息：

```bash
claude
# 或
codex
# 或
gemini
```

能够进入对话界面并正常收到回复，就表示配置完成。

如果命令能启动但请求失败，请依次检查：

1. 密钥分组是否与客户端一致。
2. Codex 地址是否带 `/v1`。
3. Claude Code 和 Gemini 地址是否误加了 `/v1`。
4. CC-Switch 中是否已经点击“启用”。
5. 系统环境变量是否覆盖了 CC-Switch 写入的配置。
6. Codex 是否在切换供应商后重新启动。

## 七、日常切换供应商

配置多个供应商后，可以在主界面点击“启用”，也可以从系统托盘快速切换。切换 Codex 后建议重启 Codex；Claude Code 和 Gemini CLI 通常可以直接读取新配置。

<figure class="tutorial-media tutorial-media--compact">
  <img src="/images/cc-switch/tray-switch.png" alt="CC-Switch 系统托盘中的 Claude、Codex 和 Gemini 供应商切换菜单" loading="lazy">
  <figcaption>真实界面：在系统托盘中按客户端快速切换供应商。</figcaption>
</figure>

## 八、余额与用量显示

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

管理端池模式账户也使用 CC-Switch 导入脚本的同款请求方法验证自定义上游真实余额。启用 URL 白名单时自定义主机必须位于 `upstream_hosts`；关闭白名单时仍会按账户连通性测试的规则校验 Base URL 并发起请求，不会直接显示为未知。查询失败时显示“未知”或带 `stale` 标记的最近成功快照，不会回退为本地账户额度。原生 AWS Bedrock SigV4 没有通用余额端点，因此不支持；只有 Bedrock API Key/自定义 Base URL 明确兼容 Bearer `/v1/usage` 时才会尝试。

## 九、常见问题

### 点击“导入到 CCS”没有反应

先手动打开一次 CC-Switch，再重新点击。如果浏览器仍提示无法打开协议，请重新安装最新版 CC-Switch，或改用手动添加供应商。

### 切换后仍然使用旧地址

关闭正在运行的 CLI 和终端窗口后重新启动，并检查是否存在以下环境变量：

```text
ANTHROPIC_API_KEY
ANTHROPIC_BASE_URL
OPENAI_API_KEY
OPENAI_BASE_URL
GEMINI_API_KEY
GOOGLE_GEMINI_BASE_URL
```

这些变量可能优先于配置文件，导致 CC-Switch 的切换结果没有生效。

### Claude Code 停在首次安装或登录页面

进入 CC-Switch“设置 → 通用”，开启“跳过 Claude Code 初次安装确认”，然后重新启动 Claude Code。

### Codex 返回 404

最常见原因是 Endpoint 少了 `/v1`。请确认地址为：

```text
https://www.poke2api.com/v1
```

## 十、服务器或纯终端环境

如果你主要在服务器、SSH 或无桌面环境中工作，可以使用命令行版本。详见 [CC-Switch CLI 使用](/guide/cc-switch-cli)。
