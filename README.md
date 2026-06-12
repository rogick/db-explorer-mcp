# DB Explorer MCP

Este é um servidor MCP (Model Context Protocol) escrito em TypeScript e Node.js para permitir que o Claude (ou outras IAs compatíveis) acesse e consulte bancos de dados de forma segura. Atualmente, possui suporte para os bancos **Oracle** e **SQL Server**.

## Funcionalidades
- **4 Tools Disponíveis:** `list_databases`, `list_tables`, `get_table_schema`, `execute_query`
- **Descrições Dinâmicas:** A IA é capaz de ver os bancos e modos disponíveis antes de qualquer chamada.
- **Gerenciador de Conexões Interativo:** Adicione senhas e bancos via terminal de forma segura sem mexer em arquivos JSON e totalmente fora do alcance da IA.
- **Modos de Segurança:** Defina exatamente o que a IA pode fazer em cada banco.

### Modos de Conexão
Sempre que cadastrar um banco, você pode atribuir um dos seguintes níveis de segurança:
1. **`readonly`**: O mais restrito. A IA só pode realizar instruções passivas (`SELECT`, descrições, etc.). Qualquer comando do tipo `INSERT`, `UPDATE`, `CREATE`, `ALTER` e afins é imediatamente bloqueado antes de chegar ao banco.
2. **`normal` (Padrão)**: Permite que a IA crie estruturas (`CREATE`/`ALTER`) e manipule dados (`INSERT`/`UPDATE`), sendo muito útil para tarefas de dev. **Bloqueia comandos destrutivos** como `DROP`, `DELETE` e `TRUNCATE`.
3. **`teste`**: Totalmente irrestrito. Pula todas as verificações de segurança do servidor MCP e permite qualquer comando. Use por sua conta e risco para automações em ambientes controlados descartáveis.

## Requisitos
- [Node.js](https://nodejs.org/en/) >= 18
- (Opcional, porém recomendado) [Bun](https://bun.sh/)
- Para conexões Oracle avançadas: [Oracle Instant Client](https://www.oracle.com/database/technologies/instant-client.html) instalado em `~/oracle_client`.

## Instalação Rápida (direto do GitHub)
Você pode instalar o `db-explorer-mcp` globalmente sem clonar o repositório. O build do TypeScript roda automaticamente (via script `prepare`) durante a instalação.

```bash
# npm
npm install -g https://github.com/rogick/db-explorer-mcp

# yarn
yarn global add https://github.com/rogick/db-explorer-mcp

# bun
bun install -g github:rogick/db-explorer-mcp

# npx (executa sem instalar globalmente)
npx github:rogick/db-explorer-mcp
```

Após a instalação global, ficam disponíveis os binários:
- `db-explorer-mcp` — inicia o servidor MCP.
- `db-explorer-manager` — gerencia as conexões de banco (equivalente a `node build/connectionsManager.js`).

> Para fixar uma versão/branch, anexe `#<tag-ou-branch>` à URL (ex: `...db-explorer-mcp#v1.0.0`).

### Registrando o servidor no Claude (instalação global)
O `install.sh` só roda no fluxo de clone. Instalando globalmente, registre o MCP manualmente apontando para o binário:
```bash
# escopo de usuário (vale para todos os projetos)
claude mcp add db-explorer db-explorer-mcp --scope user

# escopo de projeto
claude mcp add db-explorer db-explorer-mcp
```
No Claude Desktop, adicione ao `claude_desktop_config.json`:
```json
{
  "mcpServers": {
    "db-explorer": {
      "command": "db-explorer-mcp"
    }
  }
}
```

## Instalação (a partir do clone)
O projeto possui um script inteligente que irá compilar o TypeScript, gerenciar dependências e já integrá-lo diretamente com o seu Claude CLI/Desktop de forma interativa.
1. Abra seu terminal na pasta do projeto.
2. Dê permissão e rode o script:
```bash
chmod +x install.sh
./install.sh
```
3. O script perguntará se você deseja instalá-lo no escopo global de Usuário (integrando com `~/.claude/`) ou num escopo de projeto específico.

## Gerenciando as Conexões de Banco de Dados
A IA não tem permissão nem mecanismos para editar ou adicionar conexões de bancos. Isso é feito de forma isolada por você usando o CLI do `connectionsManager`.

Os comandos abaixo têm duas formas, dependendo de como você instalou:
- **Instalação global** (npm/yarn/bun `-g`): use o binário `db-explorer-manager`.
- **A partir do clone**: use `node build/connectionsManager.js` na pasta do projeto.

Para gerenciar, execute:
```bash
# instalação global
db-explorer-manager

# a partir do clone
node build/connectionsManager.js
```

### Adicionando um Oracle
```bash
db-explorer-manager add-oracle          # instalação global
node build/connectionsManager.js add-oracle   # a partir do clone
```
*O script é 100% interativo.* Ele perguntará o Alias, se você deseja informar o Host separado ou DSN completo, seu usuário, senha e o nível de segurança. Depois, testará a conexão na mesma hora.

### Adicionando um SQL Server
```bash
db-explorer-manager add-sqlserver          # instalação global
node build/connectionsManager.js add-sqlserver   # a partir do clone
```

### Removendo uma conexão
```bash
db-explorer-manager remove "meu_alias"          # instalação global
node build/connectionsManager.js remove "meu_alias"   # a partir do clone
```

### Listando conexões
```bash
db-explorer-manager list          # instalação global
node build/connectionsManager.js list   # a partir do clone
```

## Testes
O projeto possui uma cobertura completa de testes unitários (`isSafeQuery`) e de integração usando o **Jest**. Para rodá-los:
```bash
npm run test
```

## Estrutura Técnica
- `src/server.ts`: Código principal do Servidor MCP responsável por receber chamadas.
- `src/connectionsManager.ts`: CLI interativo para manipular as credenciais de banco.
- `tests/`: Suíte de validação de código.
- `~/.db-explorer-config.json`: Local físico seguro onde as credenciais ficam salvas (criado automaticamente pelo CLI).
