# Poke API - OpenClaw 一键配置脚本
# 用法: irm https://docs.poke2api.com/install/openclaw.ps1 | iex
$ErrorActionPreference = 'Stop'

$ConsoleUrl = 'https://www.poke2api.com'
$ClaudeUrl = 'https://www.poke2api.com'
$CodexUrl = 'https://www.poke2api.com/v1'
$ClaudeModel = 'claude-opus-4-6'
$CodexModel = 'gpt-5.5'

function Write-InfoMessage([string]$Message) { Write-Host "[信息] $Message" -ForegroundColor Cyan }
function Write-SuccessMessage([string]$Message) { Write-Host "[成功] $Message" -ForegroundColor Green }
function Write-WarnMessage([string]$Message) { Write-Host "[提示] $Message" -ForegroundColor Yellow }
function Write-ErrorMessage([string]$Message) { Write-Host "[错误] $Message" -ForegroundColor Red }

function Resolve-NativeCommand([string]$Name) {
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

Write-Host '======================================'
Write-Host '     Poke API · OpenClaw 一键配置'
Write-Host '======================================'

$node = Resolve-NativeCommand 'node'
$npm = Resolve-NativeCommand 'npm'
if (-not $node -or -not $npm) {
    Write-ErrorMessage '未检测到 Node.js 与 npm，请先安装 Node.js LTS。'
    Write-ErrorMessage '教程: https://docs.poke2api.com/guide/nodejs'
    throw 'Node.js 或 npm 不可用。'
}
Write-InfoMessage "Node.js 版本: $(& $node --version)"

$openclaw = Resolve-NativeCommand 'openclaw'
if (-not $openclaw) {
    Write-InfoMessage '未检测到 OpenClaw，正在安装 npm 包 openclaw...'
    & $npm install --global openclaw
    if ($LASTEXITCODE -ne 0) { throw 'npm install --global openclaw 执行失败。' }
    $openclaw = Resolve-NativeCommand 'openclaw'
}
if (-not $openclaw) {
    throw '未找到 openclaw 命令，请重新打开 PowerShell 后再运行本脚本。'
}
$version = & $openclaw --version 2>$null
if ($LASTEXITCODE -eq 0 -and $version) {
    Write-SuccessMessage "OpenClaw 已就绪: $version"
} else {
    Write-SuccessMessage 'OpenClaw 已就绪'
}

Write-Host ''
Write-Host '请选择接入通道:'
Write-Host "  1) Anthropic (Claude)  - $ClaudeModel"
Write-Host "  2) OpenAI (Codex)      - $CodexModel"
$choice = Read-Host '输入序号 [1/2，默认 1]'

switch ($choice) {
    '2' {
        $channelName = 'OpenAI (Codex)'
        $compatibility = 'openai-responses'
        $baseUrl = $CodexUrl
        $modelId = $CodexModel
        $providerId = 'pokeapi-codex'
    }
    { $_ -eq '' -or $_ -eq '1' } {
        $channelName = 'Anthropic (Claude)'
        $compatibility = 'anthropic'
        $baseUrl = $ClaudeUrl
        $modelId = $ClaudeModel
        $providerId = 'pokeapi-claude'
    }
    default { throw "无效选项: $choice" }
}

$apiKey = Read-SecretText "请输入 Poke API Key（从 $ConsoleUrl 获取，输入不会回显）"
if ([string]::IsNullOrWhiteSpace($apiKey)) { throw 'API Key 不能为空。' }

Write-InfoMessage '正在写入 OpenClaw 本地配置（Gateway 仅绑定 loopback）...'
$arguments = @(
    'onboard',
    '--non-interactive',
    '--accept-risk',
    '--mode', 'local',
    '--auth-choice', 'custom-api-key',
    '--custom-base-url', $baseUrl,
    '--custom-model-id', $modelId,
    '--custom-api-key', $apiKey,
    '--custom-provider-id', $providerId,
    '--custom-compatibility', $compatibility,
    '--custom-image-input',
    '--gateway-bind', 'loopback',
    '--no-install-daemon',
    '--skip-health',
    '--skip-channels',
    '--skip-skills',
    '--skip-search',
    '--skip-ui',
    '--skip-hooks'
)
& $openclaw @arguments
$exitCode = $LASTEXITCODE
$apiKey = $null
$arguments = $null
if ($exitCode -ne 0) { throw "openclaw onboard 执行失败，退出码: $exitCode" }

Write-Host ''
Write-Host '======================================'
Write-SuccessMessage 'OpenClaw 配置完成'
Write-Host "  通道       : $channelName"
Write-Host "  Provider ID: $providerId"
Write-Host "  Base URL   : $baseUrl"
Write-Host "  模型       : $modelId"
Write-Host '  Gateway    : loopback'
Write-Host '======================================'
Write-WarnMessage '启动 Gateway: openclaw gateway run'
Write-WarnMessage '另开终端启动 TUI: openclaw tui'
Write-WarnMessage '验证命令: openclaw gateway status --deep; openclaw models status'
