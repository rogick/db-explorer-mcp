# DB Explorer MCP (Go)

Este é um servidor MCP (Model Context Protocol) de alto desempenho escrito em **Go** para permitir que o Claude (ou outras IAs compatíveis) acesse e consulte bancos de dados de forma segura. Possui suporte nativo para os bancos **Oracle**, **SQL Server**, **PostgreSQL** e **MySQL**.

> 🚀 **Drivers Nativos Puros (Zero Client Dependency)**: Migrado para Go! Não requer a instalação de clients de banco locais (como Oracle Instant Client, OCI DLLs ou Node.js runtime). Compila para um único binário estático e autossuficiente.

---

## Funcionalidades
- **8 Tools Disponíveis:** `list_databases`, `list_tables`, `get_table_schema`, `execute_query`, `list_constraints`, `list_indexes`, `list_routines`, `get_routine_source`
- **Schema Detalhado com Níveis Configuráveis:** A tool `get_table_schema` retorna tamanho (`length`), precisão e escala (`precision`, `scale`), `nullable`, valores padrão (`default`), regras de validação por campo (`checks`) e todas as constraints da tabela (Primary Key, Foreign Keys com tabelas e colunas referenciadas, Unique e Check Constraints). Permite à IA escolher o nível de detalhamento via parâmetro `detail_level`:
  - `detailed` / `full` *(padrão)*: completo com tamanho, precisão, nullable, default, checks dos campos e todas as constraints.
  - `standard`: colunas com tamanho, precisão, nullable, default e constraints PK/FK/Unique (sem expressões de checks).
  - `basic`: colunas essenciais, tipos, nullable e PK (formato super compacto para economia de tokens de contexto).
  - Suporta também saída formatada em `json` (padrão) ou `md` (tabela e seções formatadas em Markdown).
- **Catálogo Uniforme entre Bancos (Constraints, Índices e Rotinas):**
  - `list_constraints`: Lista constraints (`PK`, `FK`, `UNIQUE`, `CHECK`, `DEFAULT`) de uma tabela ou do schema inteiro. Suporta filtro por tipo e o parâmetro `direction` (`own` para constraints da tabela, ou `referenced_by` para descobrir quais Foreign Keys de outras tabelas apontam para ela). Retorna colunas idênticas nos 4 bancos: `owner`, `table_name`, `constraint_name`, `constraint_type`, `columns`, `ref_owner`, `ref_table`, `ref_columns`, `on_delete`, `on_update`, `status`, `check_expression`.
  - `list_indexes`: Lista os índices da tabela ou schema com unicidade (`is_unique`), se é chave primária (`is_primary`), tipo de índice (`index_type`), colunas com ordenação (`columns` com ASC/DESC), colunas incluídas (`included_columns` em SQL Server e Postgres), predicados de filtro (`predicate` em índices parciais) e `status`.
  - `list_routines`: Lista procedures, functions, triggers e packages (Oracle) com linguagem, status de compilação, datas de criação/modificação, e detalhes de triggers (`parent_table`, `trigger_event`, `trigger_timing`), filtrando automaticamente triggers internas no PostgreSQL.
  - `get_routine_source`: Extrai o código-fonte integral de procedures, functions, triggers e packages (especificação + body). Retorna erro claro e explicativo se o objeto estiver criptografado (`WITH ENCRYPTION` no SQL Server), `WRAPPED` no Oracle ou sem permissão de leitura.
- **Múltiplos Formatos de Saída e Paginação:** Suporte a formatação em `md` (markdown tables - padrão no `execute_query`), `json` (padrão nas demais tools de listagem), `xml`, `csv` e `toon` (formato denso otimizado para IA). No formato `csv`, permite definir o delimitador de colunas via parâmetro `csv_separator` (padrão `,`, suportando `;`, `\t`, `|`, etc.). Permite controle de limite e paginação via parâmetros `limit` (padrão 500 linhas, `0` para sem limite), `offset` (linhas a pular) e `page` (número da página, 1-based). Emite aviso explícito e inteligente de paginação/truncamento (`truncated: true`).
- **Suporte Inteligente a LOBs e Exportação para Arquivos Temporários (Economia de Tokens):** A tool `execute_query` oferece duas estratégias para evitar explosão de contexto ao lidar com cargas volumosas ou campos LOB (`CLOB`, `BLOB`, `TEXT`, `LONGTEXT`, `XML`, `JSON`, `BYTEA`, `VARCHAR(MAX)`):
  - `lob_to_file: true`: Mantém os campos escalares na tabela retornada à IA, mas extrai automaticamente colunas LOB ou campos grandes (`>= lob_threshold_bytes`, padrão `2048` bytes) para arquivos temporários individuais com extensão detectada automaticamente (`.xml`, `.json`, `.bin`, `.txt`), substituindo o valor na célula por `[LOB salvo em: <caminho> (<tamanho>)]`.
  - `save_to_file: true`: Grava todo o resultado formatado da consulta em um arquivo temporário (ou no caminho informado em `file_path`) e retorna apenas um resumo JSON com caminho do arquivo, tamanho, contagem de linhas, colunas e um preview inicial.
  - Conta com limpeza automática (*housekeeping*) de arquivos temporários com mais de 24 horas.
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

## Instalação

### Opção 1: Binários Pré-compilados (Recomendado)
Você pode baixar o pacote pronto para seu sistema operacional na [página de Releases do GitHub](https://github.com/rogick/db-explorer-mcp/releases).

Ao descompactar o arquivo, você encontrará os binários e scripts interativos prontos para configurar seus clientes de IA:
- **Windows:** Execute `.\configure.ps1` (ou `.\configure-claude-code.ps1` no PowerShell)
- **Linux / macOS:** Execute `./configure.sh` (ou `./configure-claude-code.sh` no Terminal)

O script pergunta interativamente antes de registrar cada cliente:
1. **Claude Code (CLI):** Pergunta se deseja registrar e permite escolher o escopo (`user` ou `local`).
2. **Claude Desktop:** Pergunta se deseja configurar e atualiza automaticamente o arquivo `claude_desktop_config.json`.
3. **Cursor IDE:** Pergunta se deseja configurar e atualiza automaticamente o `~/.cursor/mcp.json`.
4. **Codex (OpenAI Codex CLI / Desktop):** Pergunta se deseja configurar, registra via `codex mcp add` e grava no `~/.codex/config.toml`.

### Opção 2: Compilar a partir do Código Fonte

#### No Windows (PowerShell)
```powershell
.\install.ps1
```

#### No Linux / macOS (Bash)
```bash
chmod +x install.sh
./install.sh
```

Os scripts compilam os binários, instalam na pasta `~/.local/bin` (ou `%USERPROFILE%\.local\bin` no Windows) e na pasta local `build/`. Em seguida, perguntam interativamente para cada um dos clientes suportados (**Claude Code**, **Claude Desktop**, **Cursor** e **Codex**) antes de registrar ou alterar seus arquivos de configuração.

---

## Configuração Manual dos Clientes (Opcional)

Se preferir realizar a configuração manual ou inspecionar os arquivos:

### Claude Code (CLI)
```bash
claude mcp add --scope user db-explorer -- "/caminho/para/db-explorer-mcp"
```

### Claude Desktop
- **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`
- **Linux:** `~/.config/Claude/claude_desktop_config.json`
- **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "db-explorer": {
      "command": "C:\\Users\\seu_usuario\\.local\\bin\\db-explorer-mcp.exe"
    }
  }
}
```

### Cursor
- **Windows:** `%USERPROFILE%\.cursor\mcp.json`
- **Linux / macOS:** `~/.cursor/mcp.json`

```json
{
  "mcpServers": {
    "db-explorer": {
      "command": "C:\\Users\\seu_usuario\\.local\\bin\\db-explorer-mcp.exe"
    }
  }
}
```

### Codex
- **CLI:**
```bash
codex mcp add db-explorer -- "/caminho/para/db-explorer-mcp"
```
- **Arquivo de Configuração (`~/.codex/config.toml`):**
```toml
[mcp_servers.db-explorer]
command = "C:\\Users\\seu_usuario\\.local\\bin\\db-explorer-mcp.exe"
enabled = true
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
# Ou filtrando por nome aproximado (prefixo, substring, fuzzy):
./build/db-explorer-manager list "soft"
```

### Exibindo detalhes de uma conexão
```bash
# Busca exata ou por nome aproximado (ex: "soft", "posgres", "remoto"):
./build/db-explorer-manager show "meu_alias"
# Ou também informando o alias/nome aproximado no list:
./build/db-explorer-manager list "meu_alias"
```
*(Nota de segurança: A senha é sempre mascarada e nunca é exibida).*


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
- `pkg/formatters/`: Conversores de resultado (`json`, `xml`, `md`, `csv`, `toon`).
- `pkg/tempfile/`: Gerenciador de arquivos temporários, detecção inteligente de extensão de LOBs e limpeza automática (*housekeeping*).
- `pkg/mcp/`: Manipuladores de requisições do protocolo MCP.

---

## Criando uma Nova Release

O projeto conta com automação via **GoReleaser** e **GitHub Actions**. Para publicar uma nova versão com binários para Windows, Linux e macOS:

```bash
# 1. Crie uma tag semântica apontando para o commit desejado
git tag -a v1.0.0 -m "Release v1.0.0"

# 2. Envie a tag para o repositório remoto
git push origin v1.0.0
```

O GitHub Actions executará a suíte de testes unitários e, se tudo passar, compilará os binários para todas as plataformas suportadas, compactará os arquivos e publicará a nova release automaticamente no GitHub.

