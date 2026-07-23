# Poke API - Codex 一键配置脚本 (Windows PowerShell)
# 用法: irm https://docs.poke2api.com/install/codex.ps1 | iex
$ErrorActionPreference = 'Stop'

$BaseUrl = 'https://www.poke2api.com/v1'
$ConsoleUrl = 'https://www.poke2api.com'
$Model = 'gpt-5.5'
$CodexHome = if ($env:CODEX_HOME) { $env:CODEX_HOME } else { Join-Path $HOME '.codex' }
$ConfigFile = Join-Path $CodexHome 'config.toml'
$Utf8NoBom = [Text.UTF8Encoding]::new($false)

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
Write-Host '     Poke API · Codex 一键配置'
Write-Host '======================================'

$node = Resolve-NativeCommand 'node'
$npm = Resolve-NativeCommand 'npm'
if (-not $node -or -not $npm) {
    throw '未检测到 Node.js 与 npm。请先完成 https://docs.poke2api.com/guide/nodejs'
}
Write-InfoMessage "Node.js 版本: $(& $node --version)"

$codex = Resolve-NativeCommand 'codex'
if (-not $codex) {
    Write-InfoMessage '未检测到 Codex，正在安装 @openai/codex@latest...'
    & $npm install --global '@openai/codex@latest'
    if ($LASTEXITCODE -ne 0) { throw 'Codex 安装失败。' }
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath) { $env:Path = "$userPath;$env:Path" }
    $codex = Resolve-NativeCommand 'codex'
}
if (-not $codex) { throw '未找到 codex 命令，请重新打开 PowerShell 后再次运行本脚本。' }
Write-SuccessMessage "Codex 已就绪: $(& $codex --version)"

$apiKey = Read-SecretText "请输入 Poke API Key（从 $ConsoleUrl 获取，输入不会回显）"
if ([string]::IsNullOrWhiteSpace($apiKey)) { throw 'API Key 不能为空。' }

[IO.Directory]::CreateDirectory($CodexHome) | Out-Null
$backupFile = $null
if (Test-Path -LiteralPath $ConfigFile -PathType Leaf) {
    $backupFile = "$ConfigFile.bak.$(Get-Date -Format 'yyyyMMdd-HHmmss-fff')"
    Copy-Item -LiteralPath $ConfigFile -Destination $backupFile -Force
    Write-SuccessMessage "已备份旧配置: $backupFile"
}

$configLines = @(
    'model_provider = "pokeapi"',
    "model = `"$Model`"",
    "review_model = `"$Model`"",
    'model_reasoning_effort = "high"',
    '',
    '[model_providers.pokeapi]',
    'name = "Poke API"',
    "base_url = `"$BaseUrl`"",
    'wire_api = "responses"',
    'env_key = "POKE_API_KEY"',
    'requires_openai_auth = false'
)
[IO.File]::WriteAllLines($ConfigFile, $configLines, $Utf8NoBom)
[Environment]::SetEnvironmentVariable('POKE_API_KEY', $apiKey, 'User')
$env:POKE_API_KEY = $apiKey
$apiKey = $null

Write-Host ''
Write-Host '======================================'
Write-SuccessMessage 'Codex 配置完成'
Write-Host "  Base URL : $BaseUrl"
Write-Host "  模型     : $Model"
Write-Host "  配置文件 : $ConfigFile"
Write-Host '  认证变量 : POKE_API_KEY（值未显示）'
Write-Host '======================================'
if ($backupFile) { Write-WarnMessage "旧配置备份: $backupFile" }
Write-WarnMessage '重新打开终端后，进入项目目录运行: codex'
