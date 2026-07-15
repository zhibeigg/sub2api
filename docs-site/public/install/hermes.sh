#!/usr/bin/env bash
# Poke API - Hermes 一键配置脚本
# 用法: curl -fsSL https://docs.poke2api.com/install/hermes.sh | bash
set -euo pipefail

CONSOLE_URL="https://www.poke2api.com"
CLAUDE_URL="https://www.poke2api.com"
CODEX_URL="https://www.poke2api.com/v1"
CLAUDE_MODEL="claude-opus-4-7"
CODEX_MODEL="gpt-5.5"

BLUE='\033[0;34m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info(){ printf "${BLUE}[信息]${NC} %s\n" "$1"; }
ok(){   printf "${GREEN}[成功]${NC} %s\n" "$1"; }
warn(){ printf "${YELLOW}[提示]${NC} %s\n" "$1"; }
err(){  printf "${RED}[错误]${NC} %s\n" "$1" >&2; }
read_tty(){ if { exec 3</dev/tty; } 2>/dev/null; then read -r "$@" <&3; exec 3<&-; else read -r "$@"; fi; }

echo "======================================"
echo "      Poke API · Hermes 一键配置"
echo "======================================"

# 1. 检测 pipx / pip
if command -v pipx >/dev/null 2>&1; then
  INSTALLER="pipx"
elif command -v pip >/dev/null 2>&1; then
  INSTALLER="pip"
elif command -v pip3 >/dev/null 2>&1; then
  INSTALLER="pip3"
else
  err "未检测到 pipx / pip。请先安装 Python 与 pipx。"
  exit 1
fi
info "使用安装器: $INSTALLER"

# 2. 检测 / 安装 Hermes
if ! command -v hermes >/dev/null 2>&1; then
  info "未检测到 Hermes，正在安装 hermes-agent..."
  "$INSTALLER" install hermes-agent || err "安装失败，请手动执行: $INSTALLER install hermes-agent"
fi
command -v hermes >/dev/null 2>&1 && ok "Hermes 已就绪" || warn "未检测到 hermes 命令，请确认安装路径已加入 PATH。"

# 3. 选择通道
echo ""
echo "请选择接入通道:"
echo "  1) Anthropic (Claude)  - 模型 $CLAUDE_MODEL"
echo "  2) OpenAI (Codex)      - 模型 $CODEX_MODEL"
printf "输入序号 [1/2]: "
read_tty CH

# 4. 输入 API Key
printf "请输入你的 Poke API Key (在 %s 控制台获取): " "$CONSOLE_URL"
read_tty API_KEY
if [ -z "${API_KEY:-}" ]; then err "API Key 不能为空。"; exit 1; fi

# 5. 写入 ~/.hermes/config.yaml
mkdir -p "$HOME/.hermes"
if [ "$CH" = "2" ]; then
  cat > "$HOME/.hermes/config.yaml" <<EOF
model:
  default: $CODEX_MODEL
  provider: pokeapi-codex
providers:
  pokeapi-codex:
    api_mode: codex_responses
    base_url: $CODEX_URL
    api_key: $API_KEY
    default_model: $CODEX_MODEL
    models:
      - $CODEX_MODEL
EOF
  CH_NAME="OpenAI (Codex)"; B_URL="$CODEX_URL"; MODEL="$CODEX_MODEL"
else
  cat > "$HOME/.hermes/config.yaml" <<EOF
model:
  default: $CLAUDE_MODEL
  provider: pokeapi-claude
providers:
  pokeapi-claude:
    api_mode: anthropic_messages
    base_url: $CLAUDE_URL
    api_key: $API_KEY
    default_model: $CLAUDE_MODEL
    models:
      - $CLAUDE_MODEL
EOF
  CH_NAME="Anthropic (Claude)"; B_URL="$CLAUDE_URL"; MODEL="$CLAUDE_MODEL"
fi

ok "配置已写入: $HOME/.hermes/config.yaml"
echo ""
echo "======================================"
ok "Hermes 配置完成！"
echo "  通道     : $CH_NAME"
echo "  Base URL : $B_URL"
echo "  模型     : $MODEL"
echo "======================================"
warn "运行命令: hermes"
