# Poke API - Codex (OpenAI) 一键配置脚本 (Windows PowerShell)
# 用法: irm https://docs.poke2api.com/install/codex.ps1 | iex
$ErrorActionPreference = "Stop"

$BaseUrl    = "https://www.poke2api.com/v1"
$ConsoleUrl = "https://www.poke2api.com"
$Model      = "gpt-5.5"

function Info($m){ Write-Host "[信息] $m" -ForegroundColor Cyan }
function Ok($m){   Write-Host "[成功] $m" -ForegroundColor Green }
function Warn($m){ Write-Host "[提示] $m" -ForegroundColor Yellow }
function Err($m){  Write-Host "[错误] $m" -ForegroundColor Red }

Write-Host "======================================"
Write-Host "     Poke API - Codex 一键配置"
Write-Host "======================================"

# 1. 检测 Node.js
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
  Err "未检测到 Node.js。请先安装 Node.js LTS: https://nodejs.org"
  Err "教程: https://docs.poke2api.com/guide/nodejs"
  return
}
Info ("Node.js 版本: " + (node -v))

# 2. 检测 / 安装 Codex
if (-not (Get-Command codex -ErrorAction SilentlyContinue)) {
  Info "未检测到 Codex，正在通过 npm 安装..."
  npm install -g @openai/codex@latest
}
if (Get-Command codex -ErrorAction SilentlyContinue) {
  Ok "Codex 已就绪"
} else {
  Err "Codex 安装失败，请手动执行: npm install -g @openai/codex@latest"
  return
}

# 3. 输入 API Key
Write-Host ""
$ApiKey = Read-Host "请输入你的 Poke API Key (在 $ConsoleUrl 控制台获取)"
if ([string]::IsNullOrWhiteSpace($ApiKey)) { Err "API Key 不能为空。"; return }

# 4. 写入 ~/.codex 配置
$CodexDir = Join-Path $env:USERPROFILE ".codex"
if (Test-Path $CodexDir) { Remove-Item -Recurse -Force $CodexDir }
New-Item -ItemType Directory -Path $CodexDir | Out-Null

$ConfigToml = @"
model_provider = "codex"
model = "$Model"
review_model = "$Model"
model_reasoning_effort = "high"
disable_response_storage = true
network_access = "enabled"
windows_wsl_setup_acknowledged = true
model_context_window = 270000
model_auto_compact_token_limit = 270000
effective_context_window_percent = 95

[model_providers.codex]
name = "codex"
base_url = "$BaseUrl"
wire_api = "responses"
requires_openai_auth = true
"@
$ConfigToml | Out-File -FilePath (Join-Path $CodexDir "config.toml") -Encoding utf8

$AuthJson = @"
{
  "OPENAI_API_KEY": "$ApiKey"
}
"@
$AuthJson | Out-File -FilePath (Join-Path $CodexDir "auth.json") -Encoding utf8

Ok "配置已写入: $CodexDir"
Write-Host ""
Write-Host "======================================"
Ok "Codex 配置完成！"
Write-Host "  Base URL : $BaseUrl"
Write-Host "  模型     : $Model"
Write-Host "======================================"
Warn "已打开的 Codex 需重启后生效。进入项目目录运行: codex"
