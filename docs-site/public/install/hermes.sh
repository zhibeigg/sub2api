#!/usr/bin/env bash
# Poke API - Hermes 一键配置脚本
# 用法: curl -fsSL https://docs.poke2api.com/install/hermes.sh | bash
set -euo pipefail

CONSOLE_URL="https://www.poke2api.com"
CLAUDE_URL="https://www.poke2api.com"
CODEX_URL="https://www.poke2api.com/v1"
CLAUDE_MODEL="claude-opus-4-7"
CODEX_MODEL="gpt-5.5"
HERMES_HOME="$HOME/.hermes"
CONFIG_FILE="$HERMES_HOME/config.yaml"
ENV_FILE="$HERMES_HOME/.env"

BLUE='\033[0;34m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info() { printf '%b[信息]%b %s\n' "$BLUE" "$NC" "$1"; }
ok() { printf '%b[成功]%b %s\n' "$GREEN" "$NC" "$1"; }
warn() { printf '%b[提示]%b %s\n' "$YELLOW" "$NC" "$1"; }
err() { printf '%b[错误]%b %s\n' "$RED" "$NC" "$1" >&2; }

read_from_tty() {
  local prompt="$1"
  local value
  if [ ! -r /dev/tty ]; then
    err "当前环境没有可交互终端，无法安全读取输入。"
    exit 1
  fi
  printf '%s' "$prompt" > /dev/tty
  IFS= read -r value < /dev/tty
  printf '%s' "$value"
}

read_secret_from_tty() {
  local prompt="$1"
  local value
  if [ ! -r /dev/tty ]; then
    err "当前环境没有可交互终端，无法安全读取 API Key。"
    exit 1
  fi
  printf '%s' "$prompt" > /dev/tty
  IFS= read -r -s value < /dev/tty
  printf '\n' > /dev/tty
  printf '%s' "$value"
}

backup_file() {
  local file="$1"
  local stamp="$2"
  if [ -f "$file" ]; then
    local backup="${file}.bak.${stamp}"
    cp -p "$file" "$backup"
    ok "已备份: $backup"
  fi
}

write_env_key() {
  local key_value="$1"
  local escaped="$key_value"
  local tmp_file
  local replaced=false

  escaped="${escaped//\\/\\\\}"
  escaped="${escaped//\"/\\\"}"
  tmp_file="$(mktemp "$HERMES_HOME/.env.tmp.XXXXXX")"

  if [ -f "$ENV_FILE" ]; then
    while IFS= read -r line || [ -n "$line" ]; do
      case "$line" in
        POKE_API_KEY=*|'export POKE_API_KEY='*)
          if [ "$replaced" = false ]; then
            printf 'POKE_API_KEY="%s"\n' "$escaped" >> "$tmp_file"
            replaced=true
          fi
          ;;
        *) printf '%s\n' "$line" >> "$tmp_file" ;;
      esac
    done < "$ENV_FILE"
  fi

  if [ "$replaced" = false ]; then
    if [ -s "$tmp_file" ]; then printf '\n' >> "$tmp_file"; fi
    printf 'POKE_API_KEY="%s"\n' "$escaped" >> "$tmp_file"
  fi

  chmod 600 "$tmp_file"
  mv -f "$tmp_file" "$ENV_FILE"
}

printf '%s\n' "======================================"
printf '%s\n' "      Poke API · Hermes 一键配置"
printf '%s\n' "======================================"

mkdir -p "$HERMES_HOME"
chmod 700 "$HERMES_HOME" 2>/dev/null || true
backup_stamp="$(date '+%Y%m%d-%H%M%S')-$$"
backup_file "$CONFIG_FILE" "$backup_stamp"
backup_file "$ENV_FILE" "$backup_stamp"

if ! command -v hermes >/dev/null 2>&1; then
  if ! command -v curl >/dev/null 2>&1; then
    err "未检测到 curl，无法运行 Hermes 官方安装器。"
    exit 1
  fi
  info "未检测到 Hermes，正在运行 Nous Research 官方 install.sh..."
  curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash
  export PATH="$HOME/.local/bin:$HERMES_HOME/bin:$PATH"
fi

if command -v hermes >/dev/null 2>&1; then
  ok "Hermes 已就绪: $(hermes --version 2>/dev/null || printf '版本命令不可用')"
else
  warn "安装器已完成，但当前终端尚未找到 hermes；配置仍会写入 $HERMES_HOME。"
  warn "重新打开终端后可使用 hermes 命令。"
fi

printf '\n请选择接入通道:\n'
printf '  1) Anthropic (Claude)  - %s\n' "$CLAUDE_MODEL"
printf '  2) OpenAI (Codex)      - %s\n' "$CODEX_MODEL"
choice="$(read_from_tty '输入序号 [1/2，默认 1]: ')"

case "$choice" in
  2)
    channel_name="OpenAI (Codex)"
    base_url="$CODEX_URL"
    model_id="$CODEX_MODEL"
    api_mode="codex_responses"
    provider_id="pokeapi-codex"
    ;;
  1|'')
    channel_name="Anthropic (Claude)"
    base_url="$CLAUDE_URL"
    model_id="$CLAUDE_MODEL"
    api_mode="anthropic_messages"
    provider_id="pokeapi-claude"
    ;;
  *)
    err "无效选项: $choice"
    exit 1
    ;;
esac

api_key="$(read_secret_from_tty "请输入 Poke API Key（从 ${CONSOLE_URL} 获取，输入不会回显）: ")"
if [ -z "$api_key" ]; then
  err "API Key 不能为空。"
  exit 1
fi

write_env_key "$api_key"
unset api_key

config_tmp="$(mktemp "$HERMES_HOME/config.yaml.tmp.XXXXXX")"
cat > "$config_tmp" <<EOF
model:
  provider: custom:$provider_id
  default: $model_id

custom_providers:
  - name: $provider_id
    base_url: $base_url
    key_env: POKE_API_KEY
    api_mode: $api_mode
EOF
chmod 600 "$config_tmp"
mv -f "$config_tmp" "$CONFIG_FILE"

printf '\n%s\n' "======================================"
ok "Hermes 配置完成"
printf '  通道     : %s\n' "$channel_name"
printf '  Provider : %s\n' "$provider_id"
printf '  Base URL : %s\n' "$base_url"
printf '  模型     : %s\n' "$model_id"
printf '  API 模式 : %s\n' "$api_mode"
printf '  配置文件 : %s\n' "$CONFIG_FILE"
printf '  密钥文件 : %s\n' "$ENV_FILE"
printf '%s\n' "======================================"
warn "运行命令: hermes"
warn "切换模型: hermes model"
warn "验证配置: hermes config check && hermes doctor"
