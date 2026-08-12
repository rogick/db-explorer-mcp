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
echo "=== Configuração no Claude CLI ==="
read -p "Digite o CLAUDE_CONFIG_DIR [default: ~/.claude/]: " CONFIG_DIR
CONFIG_DIR=${CONFIG_DIR:-~/.claude/}
CONFIG_DIR="${CONFIG_DIR/#\~/$HOME}"
export CLAUDE_CONFIG_DIR="$CONFIG_DIR"

echo ""
if command -v claude &> /dev/null; then
    echo ">> Registrando no Claude..."
    claude mcp add db-explorer -- "$SERVER_PATH"
    echo "✅ MCP 'db-explorer' instalado e registrado com sucesso!"
else
    echo "⚠️ Comando 'claude' não encontrado no PATH."
    echo "Comando gerado para execução manual:"
    echo "CLAUDE_CONFIG_DIR=\"$CONFIG_DIR\" claude mcp add db-explorer -- \"$SERVER_PATH\""
fi

echo ""
echo "=================================================="
echo "✨ Instalação Finalizada!"
echo "Você já pode usar o gerenciador rodando:"
echo "  db-explorer-manager list (ou $MANAGER_PATH list)"
echo "=================================================="
