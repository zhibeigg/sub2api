# Hermes 配置教程

通过 Poke API 为 Hermes Agent 配置自定义模型端点。以下流程按 **准备 → 安装/配置 → 启动 → 验证** 排列；Hermes 的普通配置写入 `~/.hermes/config.yaml`，API Key 等秘密必须写入 `~/.hermes/.env`。

| 通道 | Base URL | 示例模型 | API mode |
| --- | --- | --- | --- |
| Anthropic（Claude） | `https://www.poke2api.com` | `claude-opus-4-7` | `anthropic_messages` |
| OpenAI（Codex） | `https://www.poke2api.com/v1` | `gpt-5.5` | `codex_responses` |

## 一、准备

1. 在 [控制台](https://www.poke2api.com) 的密钥管理中创建对应分组的 API Key。
2. 准备一个可交互终端；安装器和模型向导需要读取用户输入。
3. 确认配置位置：非秘密设置使用 `~/.hermes/config.yaml`，秘密使用 `~/.hermes/.env`。

::: warning 旧变量已移除
不要再使用旧教程中的 `OPENAI_BASE_URL` 或 `LLM_MODEL`。当前应通过 `config.yaml` 的 Provider / model 配置选择端点和模型，并把 API Key 放入 `.env`。
:::

## 二、安装与配置

### 安装 Hermes

请使用 Hermes 官方安装命令：

::: code-group

```powershell [Windows]
iex (irm https://hermes-agent.nousresearch.com/install.ps1)
```

```bash [macOS / Linux / WSL2]
curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash
```

:::

安装完成后重新打开终端；macOS / Linux / WSL2 也可以按安装器提示重新加载 shell 配置。

::: warning 不要把“脚本已下载”当成“安装成功”
下载到 `install.ps1` / `install.sh`、查看文件大小或看到安装器启动，都不能证明 Hermes 已安装。必须在新终端中运行以下命令，并以真实输出为准：

```bash
hermes --version
hermes --help
```

若命令不存在，或安装器出现 GitHub、Python、Git 等下载错误，应先解决原始错误，不能继续声称安装成功。
:::

### 推荐：使用模型向导

安装后优先运行：

```bash
hermes model
```

在向导中选择自定义端点，按所选通道填写 Base URL、模型 ID、API mode 与 API Key。向导应把普通配置保存到 `~/.hermes/config.yaml`，把秘密保存到 `~/.hermes/.env`；完成后可用下文命令核对路径。

<figure class="tutorial-media">
  <img src="/images/hermes/model-picker.png" alt="Hermes Agent 官方 Web UI 的模型选择器，界面版本 v0.11.0" loading="lazy">
  <figcaption>Hermes Agent 官方界面截图，采集于 2026-07-15，左下角显示 v0.11.0。模型目录、Provider 名称和界面布局具有时效性，实际选择时以当前 <code>hermes model</code> 或 Web UI 为准。</figcaption>
</figure>

### 站点辅助脚本

如果希望由脚本安装并写入 Poke API 的自定义 Provider，可使用：

::: code-group

```powershell [Windows PowerShell]
irm https://docs.poke2api.com/install/hermes.ps1 | iex
```

```bash [macOS / Linux / WSL2]
curl -fsSL https://docs.poke2api.com/install/hermes.sh | bash
```

:::

辅助脚本仍调用上面的 Hermes 官方安装器。API Key 通过隐藏输入读取，写入 `~/.hermes/.env`，不会在完成摘要中显示；脚本会先备份已有的 `config.yaml` 与 `.env`。

### 手动配置参考

选择 Claude 通道时，`~/.hermes/config.yaml` 可写为：

```yaml
model:
  provider: custom:pokeapi-claude
  default: claude-opus-4-7

custom_providers:
  - name: pokeapi-claude
    base_url: https://www.poke2api.com
    key_env: POKE_API_KEY
    api_mode: anthropic_messages
```

选择 Codex 通道时，使用：

```yaml
model:
  provider: custom:pokeapi-codex
  default: gpt-5.5

custom_providers:
  - name: pokeapi-codex
    base_url: https://www.poke2api.com/v1
    key_env: POKE_API_KEY
    api_mode: codex_responses
```

`~/.hermes/.env` 只保存秘密：

```dotenv
POKE_API_KEY="在此文件中填写密钥，不要把真实值粘贴到命令行"
```

macOS / Linux / WSL2 应限制文件权限：

```bash
chmod 700 ~/.hermes
chmod 600 ~/.hermes/config.yaml ~/.hermes/.env
```

## 三、启动

启动交互式 Hermes：

```bash
hermes
```

需要切换 Provider 或模型时，再次运行：

```bash
hermes model
```

## 四、验证

先确认 Hermes 实际读取的配置与秘密文件路径：

```bash
hermes config path
hermes config env-path
hermes config check
hermes config show
hermes doctor
```

`hermes config show` 不应输出 API Key 明文。最后运行 `hermes` 并发送一条简短消息；收到模型的真实回复才表示接入成功。若出现 `401`，请检查密钥、分组、Base URL 与 API mode，不要使用伪造的成功响应替代实际验证结果。
