#!/usr/bin/env bash
set -e

echo "=================================================="
echo " DB Explorer MCP (Go) - Instalador Linux/macOS"
echo "=================================================="

if ! command -v go &> /dev/null; then
    echo "ERRO: O compilador Go não foi encontrado no PATH. Por favor, instale o Go 1.22+."
    exit 1
fi

# Diretório de destino compartilhado (~/.local/bin por padrão)
TARGET_BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"
mkdir -p "$TARGET_BIN_DIR"
mkdir -p build

echo ">> Compilando binários Go sem dependências externas (CGO_ENABLED=0)..."
export CGO_ENABLED=0

SERVER_PATH="$TARGET_BIN_DIR/db-explorer-mcp"
MANAGER_PATH="$TARGET_BIN_DIR/db-explorer-manager"

go build -o "$SERVER_PATH" ./cmd/db-explorer-mcp
go build -o "$MANAGER_PATH" ./cmd/db-explorer-manager

# Copia para a pasta local build também para conveniência
cp "$SERVER_PATH" build/db-explorer-mcp
cp "$MANAGER_PATH" build/db-explorer-manager

echo "✅ Binários instalados com sucesso em:"
echo "  - $SERVER_PATH"
echo "  - $MANAGER_PATH"

if [[ ":$PATH:" != *":$TARGET_BIN_DIR:"* ]]; then
    echo ""
    echo "⚠️ Nota: A pasta '$TARGET_BIN_DIR' não está no seu PATH."
    echo "Considere adicionar ao seu ~/.bashrc ou ~/.zshrc:"
    echo "  export PATH=\"\$PATH:$TARGET_BIN_DIR\""
fi

echo ""
prompt_yes_no() {
    local prompt="$1"
    local default_yes="${2:-true}"
    local suffix="[Padrão: s]"
    [[ "$default_yes" != "true" ]] && suffix="[Padrão: n]"

    read -p "$prompt (s/n) $suffix: " answer
    if [[ -z "$answer" ]]; then
        [[ "$default_yes" == "true" ]] && return 0 || return 1
    fi
    if [[ "$answer" =~ ^[sSyY]([iI][mM]|[eE][sS])?$ ]]; then
        return 0
    else
        return 1
    fi
}

update_mcp_json() {
    local target_file="$1"
    local server_bin="$2"
    local target_dir
    target_dir="$(dirname "$target_file")"
    mkdir -p "$target_dir"

    if command -v python3 &>/dev/null; then
        python3 -c "
import json, os, sys
path = sys.argv[1]
cmd = sys.argv[2]
data = {}
if os.path.exists(path) and os.path.getsize(path) > 0:
    try:
        with open(path, 'r', encoding='utf-8') as f:
            data = json.load(f)
    except Exception:
        data = {}
if not isinstance(data, dict):
    data = {}
if 'mcpServers' not in data or not isinstance(data['mcpServers'], dict):
    data['mcpServers'] = {}
data['mcpServers']['db-explorer'] = {'command': cmd}
with open(path, 'w', encoding='utf-8') as f:
    json.dump(data, f, indent=2, ensure_ascii=False)
" "$target_file" "$server_bin"
    elif command -v node &>/dev/null; then
        node -e "
const fs = require('fs');
const [,, path, cmd] = process.argv;
let data = {};
if (fs.existsSync(path)) {
    try { data = JSON.parse(fs.readFileSync(path, 'utf8')); } catch (e) { data = {}; }
}
if (!data || typeof data !== 'object') data = {};
if (!data.mcpServers || typeof data.mcpServers !== 'object') data.mcpServers = {};
data.mcpServers['db-explorer'] = { command: cmd };
fs.writeFileSync(path, JSON.stringify(data, null, 2), 'utf8');
" "$target_file" "$server_bin"
    elif command -v jq &>/dev/null && [ -f "$target_file" ] && [ -s "$target_file" ]; then
        tmp=$(mktemp)
        jq --arg cmd "$server_bin" '.mcpServers["db-explorer"] = {"command": $cmd}' "$target_file" > "$tmp" && mv "$tmp" "$target_file"
    else
        cat <<EOF > "$target_file"
{
  "mcpServers": {
    "db-explorer": {
      "command": "$server_bin"
    }
  }
}
EOF
    fi
}

update_codex_toml() {
    local target_file="$1"
    local server_bin="$2"
    local target_dir
    target_dir="$(dirname "$target_file")"
    mkdir -p "$target_dir"

    if command -v python3 &>/dev/null; then
        python3 -c "
import os, re, sys
path = sys.argv[1]
cmd = sys.argv[2]
block = f'''[mcp_servers.db-explorer]
command = \"{cmd}\"
enabled = true
'''
content = ''
if os.path.exists(path):
    with open(path, 'r', encoding='utf-8') as f:
        content = f.read()

pattern = r'(?m)^\[mcp_servers\.db-explorer\](\n(?:[^\[\n].*)?)*'
if re.search(pattern, content):
    new_content = re.sub(pattern, block.strip(), content)
    with open(path, 'w', encoding='utf-8') as f:
        f.write(new_content.strip() + '\n')
else:
    prefix = '\n\n' if content.strip() else ''
    with open(path, 'a', encoding='utf-8') as f:
        f.write(prefix + block.strip() + '\n')
" "$target_file" "$server_bin"
    else
        if [ -f "$target_file" ] && grep -q '\[mcp_servers\.db-explorer\]' "$target_file"; then
            awk '/^\[mcp_servers\.db-explorer\]/{flag=1; next} /^\[/{flag=0} !flag' "$target_file" > "$target_file.tmp"
            mv "$target_file.tmp" "$target_file"
        fi
        echo "" >> "$target_file"
        echo "[mcp_servers.db-explorer]" >> "$target_file"
        echo "command = \"$server_bin\"" >> "$target_file"
        echo "enabled = true" >> "$target_file"
    fi
}

echo ""
echo "=================================================="
echo " Configuração dos Clientes de IA / MCP"
echo "=================================================="

# 1. Claude Code
echo ""
echo "--- 1. Claude Code (CLI) ---"
if prompt_yes_no "Deseja registrar o MCP no Claude Code?"; then
    echo "Escolha o escopo de ativação no Claude Code:"
    echo "  [1] Global / User (Disponível em qualquer projeto) [Padrão]"
    echo "  [2] Local (Apenas para a pasta atual)"
    read -p "Escolha o escopo [Padrão: 1]: " SCOPE_CHOICE
    SCOPE="user"
    [[ "$SCOPE_CHOICE" == "2" ]] && SCOPE="local"

    if command -v claude &> /dev/null; then
        echo ">> Registrando MCP 'db-explorer' no Claude Code ($SCOPE)..."
        claude mcp remove --scope "$SCOPE" db-explorer >/dev/null 2>&1 || true
        claude mcp add --scope "$SCOPE" db-explorer -- "$SERVER_PATH"
        echo "✅ Registrado no Claude Code com sucesso!"
    else
        echo "⚠️ Comando 'claude' não encontrado no PATH."
        echo "Comando manual: claude mcp add --scope $SCOPE db-explorer -- \"$SERVER_PATH\""
    fi
else
    echo "⏭️ Registro no Claude Code ignorado."
fi

# 2. Claude Desktop
echo ""
echo "--- 2. Claude Desktop ---"
if prompt_yes_no "Deseja configurar o MCP no Claude Desktop?"; then
    if [[ "$OSTYPE" == "darwin"* ]]; then
        DESKTOP_PATH="$HOME/Library/Application Support/Claude/claude_desktop_config.json"
    else
        DESKTOP_PATH="$HOME/.config/Claude/claude_desktop_config.json"
    fi
    update_mcp_json "$DESKTOP_PATH" "$SERVER_PATH"
    echo "✅ Configuração adicionada ao Claude Desktop com sucesso!"
    echo "   Arquivo: $DESKTOP_PATH"
else
    echo "⏭️ Configuração no Claude Desktop ignorada."
fi

# 3. Cursor
echo ""
echo "--- 3. Cursor ---"
if prompt_yes_no "Deseja configurar o MCP no Cursor?"; then
    CURSOR_PATH="$HOME/.cursor/mcp.json"
    update_mcp_json "$CURSOR_PATH" "$SERVER_PATH"
    echo "✅ Configuração adicionada ao Cursor com sucesso!"
    echo "   Arquivo: $CURSOR_PATH"
else
    echo "⏭️ Configuração no Cursor ignorada."
fi

# 4. Codex
echo ""
echo "--- 4. Codex (CLI / Desktop) ---"
if prompt_yes_no "Deseja configurar o MCP no Codex?"; then
    CODEX_PATH="$HOME/.codex/config.toml"
    CODEX_REGISTERED=0

    if command -v codex &> /dev/null; then
        echo ">> Registrando MCP 'db-explorer' via Codex CLI..."
        codex mcp remove db-explorer >/dev/null 2>&1 || true
        if codex mcp add db-explorer -- "$SERVER_PATH"; then
            echo "✅ Registrado no Codex CLI com sucesso!"
            CODEX_REGISTERED=1
        fi
    fi

    update_codex_toml "$CODEX_PATH" "$SERVER_PATH"
    if [ $CODEX_REGISTERED -eq 0 ]; then
        echo "✅ Configuração gravada em: $CODEX_PATH"
        if ! command -v codex &> /dev/null; then
            echo "ℹ️ Comando 'codex' não detectado no PATH. O arquivo $CODEX_PATH foi configurado diretamente."
        fi
    fi
else
    echo "⏭️ Configuração no Codex ignorada."
fi

echo ""
echo "=================================================="
echo "✨ Instalação e configurações concluídas!"
echo "Você já pode usar o gerenciador rodando:"
echo "  db-explorer-manager list (ou $MANAGER_PATH list)"
echo "=================================================="
