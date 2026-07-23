# Poke API - Hermes 一键配置脚本
# 用法: irm https://docs.poke2api.com/install/hermes.ps1 | iex
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$ConsoleUrl = 'https://www.poke2api.com'
$ClaudeUrl = 'https://www.poke2api.com'
$CodexUrl = 'https://www.poke2api.com/v1'
$ClaudeModel = 'claude-opus-4-7'
$CodexModel = 'gpt-5.5'
$HermesHome = Join-Path $HOME '.hermes'
$ConfigFile = Join-Path $HermesHome 'config.yaml'
$EnvFile = Join-Path $HermesHome '.env'
$Utf8NoBom = [Text.UTF8Encoding]::new($false)

function Write-InfoMessage([string]$Message) { Write-Host "[信息] $Message" -ForegroundColor Cyan }
function Write-SuccessMessage([string]$Message) { Write-Host "[成功] $Message" -ForegroundColor Green }
function Write-WarnMessage([string]$Message) { Write-Host "[提示] $Message" -ForegroundColor Yellow }

function Resolve-NativeCommand([string]$Name) {
    $cmd = Get-Command "$Name.exe" -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $cmd = Get-Command "$Name.cmd" -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
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

function Backup-File([string]$Path, [string]$Stamp) {
    if (Test-Path -LiteralPath $Path -PathType Leaf) {
        $backup = "$Path.bak.$Stamp"
        Copy-Item -LiteralPath $Path -Destination $backup -Force
        Write-SuccessMessage "已备份: $backup"
    }
}

function Write-PokeEnvKey([string]$ApiKey) {
    $escaped = $ApiKey.Replace('\', '\\').Replace('"', '\"')
    $entry = "POKE_API_KEY=`"$escaped`""
    $lines = [Collections.Generic.List[string]]::new()
    $replaced = $false

    if (Test-Path -LiteralPath $EnvFile -PathType Leaf) {
        foreach ($line in [IO.File]::ReadAllLines($EnvFile)) {
            if ($line -match '^\s*(?:export\s+)?POKE_API_KEY=') {
                if (-not $replaced) {
                    $lines.Add($entry)
                    $replaced = $true
                }
            } else {
                $lines.Add($line)
            }
        }
    }

    if (-not $replaced) {
        if ($lines.Count -gt 0 -and $lines[$lines.Count - 1] -ne '') { $lines.Add('') }
        $lines.Add($entry)
    }
    [IO.File]::WriteAllLines($EnvFile, $lines, $Utf8NoBom)
}

Write-Host '======================================'
Write-Host '      Poke API · Hermes 一键配置'
Write-Host '======================================'

[IO.Directory]::CreateDirectory($HermesHome) | Out-Null
$backupStamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
Backup-File $ConfigFile $backupStamp
Backup-File $EnvFile $backupStamp

$hermes = Resolve-NativeCommand 'hermes'
if (-not $hermes) {
    Write-InfoMessage '未检测到 Hermes，正在运行 Nous Research 官方 install.ps1...'
    iex (irm https://hermes-agent.nousresearch.com/install.ps1)
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath) { $env:Path = "$userPath;$env:Path" }
    $hermes = Resolve-NativeCommand 'hermes'
}

if ($hermes) {
    $version = & $hermes --version 2>$null
    if ($LASTEXITCODE -eq 0 -and $version) {
        Write-SuccessMessage "Hermes 已就绪: $version"
    } else {
        Write-SuccessMessage 'Hermes 已就绪'
    }
} else {
    Write-WarnMessage "安装器已完成，但当前终端尚未找到 hermes；配置仍会写入 $HermesHome。"
    Write-WarnMessage '重新打开 PowerShell 后可使用 hermes 命令。'
}

Write-Host ''
Write-Host '请选择接入通道:'
Write-Host "  1) Anthropic (Claude)  - $ClaudeModel"
Write-Host "  2) OpenAI (Codex)      - $CodexModel"
$choice = Read-Host '输入序号 [1/2，默认 1]'

switch ($choice) {
    '2' {
        $channelName = 'OpenAI (Codex)'
        $baseUrl = $CodexUrl
        $modelId = $CodexModel
        $apiMode = 'codex_responses'
        $providerId = 'pokeapi-codex'
    }
    { $_ -eq '' -or $_ -eq '1' } {
        $channelName = 'Anthropic (Claude)'
        $baseUrl = $ClaudeUrl
        $modelId = $ClaudeModel
        $apiMode = 'anthropic_messages'
        $providerId = 'pokeapi-claude'
    }
    default { throw "无效选项: $choice" }
}

$apiKey = Read-SecretText "请输入 Poke API Key（从 $ConsoleUrl 获取，输入不会回显）"
if ([string]::IsNullOrWhiteSpace($apiKey)) { throw 'API Key 不能为空。' }
Write-PokeEnvKey $apiKey
$apiKey = $null

$configLines = @(
    'model:',
    "  provider: custom:$providerId",
    "  default: $modelId",
    '',
    'custom_providers:',
    "  - name: $providerId",
    "    base_url: $baseUrl",
    '    key_env: POKE_API_KEY',
    "    api_mode: $apiMode"
)
[IO.File]::WriteAllLines($ConfigFile, $configLines, $Utf8NoBom)

Write-Host ''
Write-Host '======================================'
Write-SuccessMessage 'Hermes 配置完成'
Write-Host "  通道     : $channelName"
Write-Host "  Provider : $providerId"
Write-Host "  Base URL : $baseUrl"
Write-Host "  模型     : $modelId"
Write-Host "  API 模式 : $apiMode"
Write-Host "  配置文件 : $ConfigFile"
Write-Host "  密钥文件 : $EnvFile"
Write-Host '======================================'
Write-WarnMessage '运行命令: hermes'
Write-WarnMessage '切换模型: hermes model'
Write-WarnMessage '验证配置: hermes config check; hermes doctor'
