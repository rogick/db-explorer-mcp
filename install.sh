#!/usr/bin/env bash
set -e

echo "=================================================="
echo " DB Explorer MCP - Instalador Global via NPM"
echo "=================================================="

REPO="github:rogick/db-explorer-mcp"

# 1. Instalação Global via NPM
echo ">> Baixando, compilando e instalando pacote globalmente..."
if command -v bun &> /dev/null; then
    echo ">> (Usando Bun)"
    bun install -g "$REPO"
else
    echo ">> (Usando NPM)"
    npm install -g "$REPO"
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
    # Como foi instalado globalmente, o binário 'db-explorer-mcp' já está no PATH
    claude mcp add db-explorer -- db-explorer-mcp
    echo "✅ MCP 'db-explorer' instalado e registrado com sucesso!"
else
    echo "⚠️ Comando 'claude' não encontrado no PATH."
    echo "Comando gerado para execução manual:"
    echo "CLAUDE_CONFIG_DIR=\"$CONFIG_DIR\" claude mcp add db-explorer -- db-explorer-mcp"
fi

echo ""
echo "=================================================="
echo "✨ Instalação Finalizada!"
echo "Você já pode usar o gerenciador rodando no terminal de qualquer lugar:"
echo "  db-explorer-manager add-oracle"
echo "=================================================="
