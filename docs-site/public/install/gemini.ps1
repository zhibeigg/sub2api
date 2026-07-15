# Poke API - Gemini CLI 一键配置脚本 (Windows PowerShell)
# 用法: irm https://docs.poke2api.com/install/gemini.ps1 | iex
$ErrorActionPreference = "Stop"

$BaseUrl    = "https://www.poke2api.com"
$ConsoleUrl = "https://www.poke2api.com"
$Model      = "gemini-3-pro-preview"

function Info($m){ Write-Host "[信息] $m" -ForegroundColor Cyan }
function Ok($m){   Write-Host "[成功] $m" -ForegroundColor Green }
function Warn($m){ Write-Host "[提示] $m" -ForegroundColor Yellow }
function Err($m){  Write-Host "[错误] $m" -ForegroundColor Red }

Write-Host "======================================"
Write-Host "    Poke API - Gemini CLI 一键配置"
Write-Host "======================================"

# 1. 检测 Node.js
if (-not (Get-Command node -ErrorAction SilentlyContinue)) {
  Err "未检测到 Node.js。请先安装 Node.js LTS: https://nodejs.org"
  Err "教程: https://docs.poke2api.com/guide/nodejs"
  return
}
Info ("Node.js 版本: " + (node -v))

# 2. 检测 / 安装 Gemini CLI
if (-not (Get-Command gemini -ErrorAction SilentlyContinue)) {
  Info "未检测到 Gemini CLI，正在通过 npm 安装..."
  npm install -g @google/gemini-cli
}
if (Get-Command gemini -ErrorAction SilentlyContinue) {
  Ok "Gemini CLI 已就绪"
} else {
  Err "Gemini CLI 安装失败，请手动执行: npm install -g @google/gemini-cli"
  return
}

# 3. 输入 API Key
Write-Host ""
$ApiKey = Read-Host "请输入你的 Poke API Key (在 $ConsoleUrl 控制台获取)"
if ([string]::IsNullOrWhiteSpace($ApiKey)) { Err "API Key 不能为空。"; return }

# 4. 写入用户环境变量（永久）
[System.Environment]::SetEnvironmentVariable("GOOGLE_GEMINI_BASE_URL", $BaseUrl, [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("GEMINI_API_KEY", $ApiKey, [System.EnvironmentVariableTarget]::User)
[System.Environment]::SetEnvironmentVariable("GEMINI_MODEL", $Model, [System.EnvironmentVariableTarget]::User)
# 当前会话立即生效
$env:GOOGLE_GEMINI_BASE_URL = $BaseUrl
$env:GEMINI_API_KEY = $ApiKey
$env:GEMINI_MODEL = $Model

Ok "环境变量已写入用户配置"
Write-Host ""
Write-Host "======================================"
Ok "Gemini CLI 配置完成！"
Write-Host "  Base URL : $BaseUrl"
Write-Host "  模型     : $Model"
Write-Host "======================================"
Warn "请重新打开终端使环境变量全局生效，然后运行: gemini"
