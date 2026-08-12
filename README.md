# DB Explorer MCP (Go)

Este é um servidor MCP (Model Context Protocol) de alto desempenho escrito em **Go** para permitir que o Claude (ou outras IAs compatíveis) acesse e consulte bancos de dados de forma segura. Possui suporte nativo para os bancos **Oracle**, **SQL Server**, **PostgreSQL** e **MySQL**.

> 🚀 **Drivers Nativos Puros (Zero Client Dependency)**: Migrado para Go! Não requer a instalação de clients de banco locais (como Oracle Instant Client, OCI DLLs ou Node.js runtime). Compila para um único binário estático e autossuficiente.

---

## Funcionalidades
- **4 Tools Disponíveis:** `list_databases`, `list_tables`, `get_table_schema`, `execute_query`
- **Múltiplos Formatos de Saída:** A tool `execute_query` suporta formatação em `json`, `xml`, `llm` (markdown tables) e `toon` (formato denso otimizado para IA).
- **Descrições Dinâmicas:** A IA é capaz de ver os bancos e modos disponíveis antes de qualquer chamada.
- **Gerenciador de Conexões Interativo:** Adicione senhas e bancos via terminal de forma segura sem mexer em arquivos JSON e totalmente fora do alcance da IA.
- **Modos de Segurança Avançados:** Defina exatamente o que a IA pode fazer em cada banco. Protegido por um parser de AST/tokens SQL que evita bypasses com comentários ou múltiplas linhas.
- **Conexões Remotas Restritas:** Regras para impedir acesso a bancos arbitrários não cadastrados.
- **Compatibilidade Multiplataforma:** Binários nativos estáticos para **Windows** (x64), **Linux** e **macOS** (x64/ARM64).

---

### Modos de Conexão
Sempre que cadastrar um banco, você pode atribuir um dos seguintes níveis de segurança:
1. **`readonly`**: O mais restrito. A IA só pode realizar instruções passivas (`SELECT`, descrições, etc.). O servidor analisa a consulta SQL para garantir que não há bypasses ocultos com comentários, quebras de linha ou instruções mutativas encadeadas.
2. **`normal` (Padrão)**: Permite que a IA crie estruturas (`CREATE`/`ALTER`) e manipule dados (`INSERT`/`UPDATE`), sendo muito útil para tarefas de dev. **Bloqueia comandos destrutivos** como `DROP`, `DELETE` e `TRUNCATE`.
3. **`teste`**: Totalmente irrestrito. Pula todas as verificações de segurança do servidor MCP e permite qualquer comando. Use por sua conta e risco para automações em ambientes controlados descartáveis.

---

## Requisitos de Build
- [Go](https://go.dev/) >= 1.22

---

## Compilação e Instalação

### No Windows (PowerShell)
```powershell
.\install.ps1
```

### No Linux / macOS (Bash)
```bash
chmod +x install.sh
./install.sh
```

Os scripts compilam e instalam os binários na pasta compartilhada `~/.local/bin` (ou `%USERPROFILE%\.local\bin` no Windows) e também na pasta local `build/`. Além disso, perguntam se você deseja registrar o MCP no Claude CLI e fornecem as instruções de configuração para o `claude_desktop_config.json`.

---

## Registrando no Claude Desktop

No **Claude Desktop**, adicione a configuração abaixo:
- **Linux/macOS:** `~/.config/Claude/claude_desktop_config.json`
- **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`

```json
{
  "mcpServers": {
    "db-explorer": {
      "command": "C:\\Users\\seu_usuario\\.local\\bin\\db-explorer-mcp.exe"
    }
  }
}
```

---

## Gerenciando Conexões com `db-explorer-manager`

Para gerenciar as conexões cadastradas sem que a IA tenha acesso:

```bash
# Windows
.\build\db-explorer-manager.exe

# Linux / macOS
./build/db-explorer-manager
```

### Adicionando um Oracle (Zero Instant Client!)
```bash
./build/db-explorer-manager add-oracle
```
*O script é 100% interativo.* Funciona com Oracle 10g, 11g, 12c, 19c, 21c e 23c através do driver nativo `go-ora`.

### Adicionando um SQL Server
```bash
./build/db-explorer-manager add-sqlserver
```

### Adicionando um PostgreSQL
```bash
./build/db-explorer-manager add-postgres
```

### Adicionando um MySQL
```bash
./build/db-explorer-manager add-mysql
```

### Listando conexões
```bash
./build/db-explorer-manager list
```

### Removendo uma conexão
```bash
./build/db-explorer-manager remove "meu_alias"
```

---

## Testes Automatizados
O projeto conta com testes unitários cobrindo parser de segurança SQL, formatadores e validação de configurações. Para rodar:

```bash
go test ./... -v
```

---

## Estrutura Técnica
- `cmd/db-explorer-mcp/main.go`: Ponto de entrada do Servidor MCP stdio.
- `cmd/db-explorer-manager/main.go`: CLI interativo para gerenciar credenciais.
- `pkg/config/`: Leitura e gravação segura do arquivo `%USERPROFILE%\.db-explorer-config.json`.
- `pkg/db/`: Abstração de banco com drivers 100% nativos em Go (`go-ora/v2`, `go-mssqldb`, `pgx/v5`, `go-sql-driver/mysql`).
- `pkg/security/`: Parser de AST/tokens SQL para validação de segurança (`readonly`, `normal`, `teste`).
- `pkg/formatters/`: Conversores de resultado (`json`, `xml`, `llm`, `toon`).
- `pkg/mcp/`: Manipuladores de requisições do protocolo MCP.
