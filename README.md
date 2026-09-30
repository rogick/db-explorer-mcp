# DB Explorer MCP (Go)

Este é um servidor MCP (Model Context Protocol) de alto desempenho escrito em **Go** para permitir que o Claude (ou outras IAs compatíveis) acesse e consulte bancos de dados de forma segura. Possui suporte nativo para os bancos **Oracle**, **SQL Server**, **PostgreSQL** e **MySQL**.

> 🚀 **Drivers Nativos Puros (Zero Client Dependency)**: Migrado para Go! Não requer a instalação de clients de banco locais (como Oracle Instant Client, OCI DLLs ou Node.js runtime). Compila para um único binário estático e autossuficiente.

---

## Funcionalidades
- **4 Tools Disponíveis:** `list_databases`, `list_tables`, `get_table_schema`, `execute_query`
- **Schema Detalhado com Níveis Configuráveis:** A tool `get_table_schema` retorna tamanho (`length`), precisão e escala (`precision`, `scale`), `nullable`, valores padrão (`default`), regras de validação por campo (`checks`) e todas as constraints da tabela (Primary Key, Foreign Keys com tabelas e colunas referenciadas, Unique e Check Constraints). Permite à IA escolher o nível de detalhamento via parâmetro `detail_level`:
  - `detailed` / `full` *(padrão)*: completo com tamanho, precisão, nullable, default, checks dos campos e todas as constraints.
  - `standard`: colunas com tamanho, precisão, nullable, default e constraints PK/FK/Unique (sem expressões de checks).
  - `basic`: colunas essenciais, tipos, nullable e PK (formato super compacto para economia de tokens de contexto).
  - Suporta também saída formatada em `json` (padrão) ou `md` (tabela e seções formatadas em Markdown).
- **Múltiplos Formatos de Saída e Paginação:** A tool `execute_query` suporta formatação em `json`, `xml`, `md` (markdown tables), `csv` e `toon` (formato denso otimizado para IA). Permite controle de limite e paginação via parâmetros `limit` (padrão 500 linhas, `0` para sem limite), `offset` (linhas a pular) e `page` (número da página, 1-based). Emite aviso explícito e inteligente de paginação/truncamento (ex: *"Mostrando linhas 1 a 100 de 1400 — página 1 de 14"*) com indicação de páginas e comando para próxima página.
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

Ao descompactar o arquivo, você encontrará os binários e scripts prontos para registrar o MCP no **Claude Code**:
- **Windows:** Execute `.\configure-claude-code.ps1` (no PowerShell)
- **Linux / macOS:** Execute `./configure-claude-code.sh` (no Terminal)

O script automaticamente copia os binários para a pasta padrão (`~/.local/bin` ou `%USERPROFILE%\.local\bin`), ajusta as permissões e registra o MCP com `claude mcp add`.

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

