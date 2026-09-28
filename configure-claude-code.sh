#!/usr/bin/env bash
set -e

echo "=========================================================="
echo " Configuração do DB Explorer MCP no Claude Code (Linux/macOS)"
echo "=========================================================="

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOCAL_SERVER="$SCRIPT_DIR/db-explorer-mcp"
LOCAL_MANAGER="$SCRIPT_DIR/db-explorer-manager"

if [[ ! -f "$LOCAL_SERVER" ]]; then
    echo "ERRO: O arquivo 'db-explorer-mcp' não foi encontrado em: $SCRIPT_DIR"
    exit 1
fi

chmod +x "$LOCAL_SERVER"
[[ -f "$LOCAL_MANAGER" ]] && chmod +x "$LOCAL_MANAGER"

# 1. Perguntar onde deseja manter os binários
DEFAULT_BIN_DIR="$HOME/.local/bin"
echo ""
echo "Onde você deseja instalar os binários permanentemente?"
echo "  [1] Copiar para '$DEFAULT_BIN_DIR' (Recomendado - evita exclusão acidental)"
echo "  [2] Usar a pasta atual ('$SCRIPT_DIR')"
read -p "Escolha uma opção [Padrão: 1]: " CHOICE
CHOICE=${CHOICE:-1}

SERVER_PATH="$LOCAL_SERVER"
MANAGER_PATH="$LOCAL_MANAGER"

if [[ "$CHOICE" != "2" ]]; then
    mkdir -p "$DEFAULT_BIN_DIR"
    SERVER_PATH="$DEFAULT_BIN_DIR/db-explorer-mcp"
    MANAGER_PATH="$DEFAULT_BIN_DIR/db-explorer-manager"

    cp -f "$LOCAL_SERVER" "$SERVER_PATH"
    [[ -f "$LOCAL_MANAGER" ]] && cp -f "$LOCAL_MANAGER" "$MANAGER_PATH"

    chmod +x "$SERVER_PATH"
    [[ -f "$MANAGER_PATH" ]] && chmod +x "$MANAGER_PATH"

    echo "✅ Binários copiados para: $DEFAULT_BIN_DIR"

    if [[ ":$PATH:" != *":$DEFAULT_BIN_DIR:"* ]]; then
        echo ""
        echo "⚠️ Nota: A pasta '$DEFAULT_BIN_DIR' não está no seu PATH."
        echo "Recomenda-se adicionar a seguinte linha ao seu ~/.bashrc ou ~/.zshrc:"
        echo "  export PATH=\"\$PATH:$DEFAULT_BIN_DIR\""
    fi
fi

# 2. Escolher o escopo no Claude Code
echo ""
echo "Escolha o escopo de ativação no Claude Code:"
echo "  [1] Global / User (Disponível em qualquer projeto/pasta) [Recomendado]"
echo "  [2] Local (Apenas para a pasta atual)"
read -p "Escolha o escopo [Padrão: 1]: " SCOPE_CHOICE
SCOPE_CHOICE=${SCOPE_CHOICE:-1}

SCOPE="user"
if [[ "$SCOPE_CHOICE" == "2" ]]; then
    SCOPE="local"
fi

# 3. Registrar no Claude Code CLI
echo ""
if command -v claude &> /dev/null; then
    echo ">> Registrando MCP 'db-explorer' no Claude Code com escopo '$SCOPE'..."
    claude mcp remove --scope "$SCOPE" db-explorer >/dev/null 2>&1 || true
    claude mcp add --scope "$SCOPE" db-explorer -- "$SERVER_PATH"
    echo ""
    echo "✅ MCP 'db-explorer' registrado e configurado com sucesso no Claude Code!"
else
    echo "⚠️ O comando 'claude' não foi encontrado no PATH."
    echo "Após instalar o Claude Code CLI, você pode registrar o MCP executando:"
    echo "  claude mcp add --scope $SCOPE db-explorer -- \"$SERVER_PATH\""
fi

echo ""
echo "=========================================================="
echo "✨ Próximos Passos:"
echo "1. Para adicionar ou gerenciar conexões com bancos:"
echo "     db-explorer-manager (ou \"$MANAGER_PATH\")"
echo "2. Para verificar os servidores ativos no Claude Code:"
echo "     claude mcp list"
echo "=========================================================="
