# Poke API - Gemini CLI 一键配置脚本 (Windows PowerShell)
# 用法: irm https://docs.poke2api.com/install/gemini.ps1 | iex
$ErrorActionPreference = 'Stop'

$BaseUrl = 'https://www.poke2api.com'
$ConsoleUrl = 'https://www.poke2api.com'
$Model = 'gemini-3-pro-preview'

function Write-InfoMessage([string]$Message) { Write-Host "[信息] $Message" -ForegroundColor Cyan }
function Write-SuccessMessage([string]$Message) { Write-Host "[成功] $Message" -ForegroundColor Green }
function Write-WarnMessage([string]$Message) { Write-Host "[提示] $Message" -ForegroundColor Yellow }

function Resolve-NativeCommand([string]$Name) {
    foreach ($candidate in @("$Name.cmd", "$Name.exe", $Name)) {
        $command = Get-Command $candidate -ErrorAction SilentlyContinue
        if ($command) { return $command.Source }
    }
    return $null
}

function Read-SecretText([string]$Prompt) {
    $secure = Read-Host $Prompt -AsSecureString
    $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secure)
    try {
        return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr)
    } finally {
        [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr)
    }
}

Write-Host '======================================'
Write-Host '    Poke API · Gemini CLI 一键配置'
Write-Host '======================================'

$node = Resolve-NativeCommand 'node'
$npm = Resolve-NativeCommand 'npm'
if (-not $node -or -not $npm) {
    throw '未检测到 Node.js 与 npm。请先完成 https://docs.poke2api.com/guide/nodejs'
}
Write-InfoMessage "Node.js 版本: $(& $node --version)"

$gemini = Resolve-NativeCommand 'gemini'
if (-not $gemini) {
    Write-InfoMessage '未检测到 Gemini CLI，正在安装 @google/gemini-cli@latest...'
    & $npm install --global '@google/gemini-cli@latest'
    if ($LASTEXITCODE -ne 0) { throw 'Gemini CLI 安装失败。' }
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath) { $env:Path = "$userPath;$env:Path" }
    $gemini = Resolve-NativeCommand 'gemini'
}
if (-not $gemini) { throw '未找到 gemini 命令，请重新打开 PowerShell 后再次运行本脚本。' }
Write-SuccessMessage "Gemini CLI 已就绪: $(& $gemini --version)"

$apiKey = Read-SecretText "请输入 Poke API Key（从 $ConsoleUrl 获取，输入不会回显）"
if ([string]::IsNullOrWhiteSpace($apiKey)) { throw 'API Key 不能为空。' }

[Environment]::SetEnvironmentVariable('GOOGLE_GEMINI_BASE_URL', $BaseUrl, 'User')
[Environment]::SetEnvironmentVariable('GEMINI_API_KEY', $apiKey, 'User')
[Environment]::SetEnvironmentVariable('GEMINI_MODEL', $Model, 'User')
$env:GOOGLE_GEMINI_BASE_URL = $BaseUrl
$env:GEMINI_API_KEY = $apiKey
$env:GEMINI_MODEL = $Model
$apiKey = $null

Write-Host ''
Write-Host '======================================'
Write-SuccessMessage 'Gemini CLI 配置完成'
Write-Host "  Base URL : $BaseUrl"
Write-Host "  模型     : $Model"
Write-Host '  认证变量 : GEMINI_API_KEY（值未显示）'
Write-Host '======================================'
Write-WarnMessage '重新打开终端后运行: gemini'
Write-WarnMessage '首次启动如出现认证选择，请选择 “Use Gemini API key”。'
