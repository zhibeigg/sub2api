# Poke API - Claude Code 一键配置脚本 (Windows PowerShell)
# 用法: irm https://docs.poke2api.com/install/claude-code.ps1 | iex
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$BaseUrl = 'https://www.poke2api.com'
$ConsoleUrl = 'https://www.poke2api.com'
$OfficialInstallerUrl = 'https://claude.ai/install.ps1'

function Write-InfoMessage([string]$Message) { Write-Host "[信息] $Message" -ForegroundColor Cyan }
function Write-SuccessMessage([string]$Message) { Write-Host "[成功] $Message" -ForegroundColor Green }
function Write-WarnMessage([string]$Message) { Write-Host "[提示] $Message" -ForegroundColor Yellow }

function Resolve-NativeCommand([string]$Name) {
    foreach ($candidate in @("$Name.exe", "$Name.cmd", $Name)) {
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
Write-Host '   Poke API · Claude Code 一键配置'
Write-Host '======================================'

$claude = Resolve-NativeCommand 'claude'
if (-not $claude) {
    Write-InfoMessage '未检测到 Claude Code，正在运行 Anthropic 官方原生安装器...'
    $installer = Invoke-RestMethod -Uri $OfficialInstallerUrl -UseBasicParsing
    & ([ScriptBlock]::Create([string]$installer))

    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath) { $env:Path = "$userPath;$env:Path" }
    $localBin = Join-Path $HOME '.local\bin'
    if (Test-Path -LiteralPath $localBin) { $env:Path = "$localBin;$env:Path" }
    $claude = Resolve-NativeCommand 'claude'
}

if ($claude) {
    $version = & $claude --version 2>$null
    if ($LASTEXITCODE -eq 0 -and $version) {
        Write-SuccessMessage "Claude Code 已就绪: $version"
    } else {
        Write-SuccessMessage 'Claude Code 已就绪'
    }
} else {
    throw '官方安装器已执行，但当前 PowerShell 尚未找到 claude。请重新打开终端后再次运行本脚本。'
}

Write-Host ''
$apiKey = Read-SecretText "请输入 Poke API Key（从 $ConsoleUrl 获取，输入不会回显）"
if ([string]::IsNullOrWhiteSpace($apiKey)) { throw 'API Key 不能为空。' }

[Environment]::SetEnvironmentVariable('ANTHROPIC_BASE_URL', $BaseUrl, 'User')
[Environment]::SetEnvironmentVariable('ANTHROPIC_AUTH_TOKEN', $apiKey, 'User')
[Environment]::SetEnvironmentVariable('CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC', '1', 'User')
$env:ANTHROPIC_BASE_URL = $BaseUrl
$env:ANTHROPIC_AUTH_TOKEN = $apiKey
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC = '1'
$apiKey = $null

Write-Host ''
Write-Host '======================================'
Write-SuccessMessage 'Claude Code 配置完成'
Write-Host "  Base URL : $BaseUrl"
Write-Host '  认证变量 : ANTHROPIC_AUTH_TOKEN（值未显示）'
Write-Host '======================================'
Write-WarnMessage '重新打开终端后，进入项目目录运行: claude'
Write-WarnMessage '如需诊断安装状态，可运行: claude doctor'
