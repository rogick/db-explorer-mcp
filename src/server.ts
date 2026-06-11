#!/usr/bin/env node

import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import {
  CallToolRequestSchema,
  ListToolsRequestSchema,
  Tool,
} from "@modelcontextprotocol/sdk/types.js";
import fs from "fs";
import path from "path";
import os from "os";
// @ts-ignore
import oracledb from "oracledb";
import sql from "mssql";

// Ativa o Thick mode para o oracledb se possível (suporte a 11g)
try {
  const clientDir = path.join(os.homedir(), "oracle_client");
  if (fs.existsSync(clientDir)) {
    const clients = fs.readdirSync(clientDir).filter((f) => f.startsWith("instantclient_"));
    if (clients.length > 0) {
      oracledb.initOracleClient({ libDir: path.join(clientDir, clients[0]) });
    } else {
      oracledb.initOracleClient();
    }
  }
} catch (e) {
  // Ignora erros e continua no modo thin ou falha na conexão se thick for requerido
}

const CONFIG_PATH = process.env.DB_EXPLORER_CONFIG_PATH 
    ? path.resolve(process.env.DB_EXPLORER_CONFIG_PATH) 
    : path.join(os.homedir(), ".db-explorer-config.json");


function loadConfig(): any {
  if (fs.existsSync(CONFIG_PATH)) {
    return JSON.parse(fs.readFileSync(CONFIG_PATH, "utf-8"));
  }
  return { connections: {} };
}

async function getConnection(alias: string): Promise<{ conn: any, dbType: string }> {
  const config = loadConfig();
  const conns = config.connections || {};
  if (!conns[alias]) {
    throw new Error(`Conexão '${alias}' não encontrada.`);
  }

  const details = conns[alias];
  const dbType = details.type;

  if (dbType === "oracle") {
    const conn = await oracledb.getConnection({
      user: details.user,
      password: details.password,
      connectString: details.dsn,
    });
    return { conn, dbType };
  } else if (dbType === "sqlserver") {
    const parts = details.server.split(":");
    const server = parts[0];
    const port = parts[1] ? parseInt(parts[1], 10) : 1433;
    const pool = await sql.connect({
      user: details.user,
      password: details.password,
      server: server,
      port: port,
      database: details.database,
      options: {
        encrypt: false, // Default para conexões locais/antigas
        trustServerCertificate: true,
      },
    });
    return { conn: pool, dbType };
  } else {
    throw new Error(`Tipo de banco '${dbType}' não suportado.`);
  }
}

export function isSafeQuery(query: string, mode: string): { isSafe: boolean; errorMsg: string } {
  try {
    if (mode === "teste") {
        return { isSafe: true, errorMsg: "" };
    }

    const upperQuery = query.toUpperCase();
    if (upperQuery.includes("DROP ") || upperQuery.includes("DELETE ") || upperQuery.includes("TRUNCATE ")) {
        return { isSafe: false, errorMsg: "Operações destrutivas (DROP, DELETE, TRUNCATE) não são permitidas." };
    }
    
    if (mode === "readonly") {
        if (upperQuery.includes("INSERT ") || 
            upperQuery.includes("UPDATE ") || 
            upperQuery.includes("CREATE ") || 
            upperQuery.includes("ALTER ") || 
            upperQuery.includes("MERGE ") || 
            upperQuery.includes("GRANT ") || 
            upperQuery.includes("REVOKE ")) {
            return { isSafe: false, errorMsg: "Conexão em modo 'readonly'. Apenas consultas de leitura são permitidas." };
        }
    }
    
    // Simplificando segurança com regex básica para evitar overhead de AST complexas em TypeScript
    return { isSafe: true, errorMsg: "" };
  } catch (e: any) {
    return { isSafe: false, errorMsg: `Erro de parsing: ${e.message}` };
  }
}

const server = new Server(
  {
    name: "db-explorer-mcp",
    version: "1.0.0",
  },
  {
    capabilities: {
      tools: {},
    },
  }
);

const LIST_DATABASES_TOOL: Tool = {
  name: "list_databases",
  description: "Lista os aliases dos bancos de dados configurados disponíveis para consulta.",
  inputSchema: {
    type: "object",
    properties: {},
  },
};

const LIST_TABLES_TOOL: Tool = {
  name: "list_tables",
  description: "Lista as tabelas disponíveis no banco de dados especificado pelo alias.",
  inputSchema: {
    type: "object",
    properties: {
      db_alias: { type: "string", description: "O alias do banco de dados" },
    },
    required: ["db_alias"],
  },
};

const GET_TABLE_SCHEMA_TOOL: Tool = {
  name: "get_table_schema",
  description: "Retorna as colunas e os tipos de dados de uma tabela específica.",
  inputSchema: {
    type: "object",
    properties: {
      db_alias: { type: "string" },
      table_name: { type: "string" },
    },
    required: ["db_alias", "table_name"],
  },
};

const EXECUTE_QUERY_TOOL: Tool = {
  name: "execute_query",
  description: "Executa uma consulta SQL no banco especificado. Permite SELECT, CREATE, ALTER, INSERT, UPDATE. Bloqueia operações destrutivas como DROP, DELETE e TRUNCATE.",
  inputSchema: {
    type: "object",
    properties: {
      db_alias: { type: "string" },
      query: { type: "string" },
    },
    required: ["db_alias", "query"],
  },
};

server.setRequestHandler(ListToolsRequestSchema, async () => {
  const config = loadConfig();
  const conns = config.connections || {};
  const dbsInfo = Object.keys(conns).map(alias => `'${alias}' (${conns[alias].type}, modo: ${conns[alias].mode || 'normal'})`).join(", ");
  const availableStr = dbsInfo ? ` Bancos disponíveis: ${dbsInfo}.` : " Nenhum banco configurado.";

  const dynamicListTables = { ...LIST_TABLES_TOOL, description: LIST_TABLES_TOOL.description + availableStr };
  const dynamicGetSchema = { ...GET_TABLE_SCHEMA_TOOL, description: GET_TABLE_SCHEMA_TOOL.description + availableStr };
  const dynamicExecuteQuery = { ...EXECUTE_QUERY_TOOL, description: EXECUTE_QUERY_TOOL.description + availableStr };

  return {
    tools: [LIST_DATABASES_TOOL, dynamicListTables, dynamicGetSchema, dynamicExecuteQuery],
  };
});

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  const toolName = request.params.name;
  const args = request.params.arguments || {};

  try {
    if (toolName === "list_databases") {
      const config = loadConfig();
      const conns = config.connections || {};
      const dbs = Object.keys(conns).map(alias => ({
          alias: alias,
          type: conns[alias].type,
          mode: conns[alias].mode || "normal"
      }));
      return {
        content: [{ type: "text", text: JSON.stringify(dbs, null, 2) }],
      };
    }

    if (toolName === "list_tables") {
      const db_alias = args.db_alias as string;
      const { conn, dbType } = await getConnection(db_alias);
      let tables: string[] = [];

      try {
        if (dbType === "oracle") {
          const result = await conn.execute("SELECT table_name FROM all_tables FETCH FIRST 500 ROWS ONLY");
          tables = result.rows.map((r: any) => r[0]);
        } else if (dbType === "sqlserver") {
          const result = await conn.request().query("SELECT table_name FROM information_schema.tables WHERE table_type = 'BASE TABLE'");
          tables = result.recordset.map((r: any) => r.table_name);
        }
      } finally {
        if (dbType === "oracle") await conn.close();
        else if (dbType === "sqlserver") await conn.close();
      }

      return {
        content: [{ type: "text", text: JSON.stringify(tables, null, 2) }],
      };
    }

    if (toolName === "get_table_schema") {
      const db_alias = args.db_alias as string;
      const table_name = args.table_name as string;
      const { conn, dbType } = await getConnection(db_alias);
      let schema: any[] = [];

      try {
        if (dbType === "oracle") {
          const result = await conn.execute(`SELECT column_name, data_type FROM all_tab_columns WHERE table_name = '${table_name.toUpperCase()}'`);
          schema = result.rows.map((r: any) => ({ column: r[0], type: r[1] }));
        } else if (dbType === "sqlserver") {
          const result = await conn.request().query(`SELECT column_name, data_type FROM information_schema.columns WHERE table_name = '${table_name}'`);
          schema = result.recordset.map((r: any) => ({ column: r.column_name, type: r.data_type }));
        }
      } finally {
        if (dbType === "oracle") await conn.close();
        else if (dbType === "sqlserver") await conn.close();
      }

      return {
        content: [{ type: "text", text: JSON.stringify(schema, null, 2) }],
      };
    }

    if (toolName === "execute_query") {
      const db_alias = args.db_alias as string;
      const query = args.query as string;

      const config = loadConfig();
      const mode = config.connections?.[db_alias]?.mode || "normal";

      const { isSafe, errorMsg } = isSafeQuery(query, mode);
      if (!isSafe) {
        return {
          content: [{ type: "text", text: JSON.stringify([{ error: `Operação não permitida. ${errorMsg}` }]) }],
        };
      }

      const { conn, dbType } = await getConnection(db_alias);
      let results: any[] = [];

      try {
        if (dbType === "oracle") {
          const result = await conn.execute(query, [], { outFormat: oracledb.OUT_FORMAT_OBJECT, maxRows: 100, autoCommit: true });
          if (result.rows) {
            results = result.rows;
          } else {
            results = [{ status: "success", rowsAffected: result.rowsAffected || 0 }];
          }
        } else if (dbType === "sqlserver") {
          const request = conn.request();
          const result = await request.query(query);
          if (result.recordsets && result.recordsets.length > 0) {
            results = result.recordsets[0].slice(0, 100);
          } else {
            results = [{ status: "success", rowsAffected: result.rowsAffected[0] || 0 }];
          }
        }
      } catch (err: any) {
        results = [{ error: err.message }];
      } finally {
        if (dbType === "oracle") await conn.close();
        else if (dbType === "sqlserver") await conn.close();
      }

      const stringifiedResults = results.map((row: any) => {
        const newRow: any = {};
        for (const [key, val] of Object.entries(row)) {
          newRow[key] = val !== null && val !== undefined ? String(val) : null;
        }
        return newRow;
      });

      return {
        content: [{ type: "text", text: JSON.stringify(stringifiedResults, null, 2) }],
      };
    }

    throw new Error(`Tool not found: ${toolName}`);
  } catch (error: any) {
    return {
      isError: true,
      content: [{ type: "text", text: `Error executing tool: ${error.message}` }],
    };
  }
});

async function run() {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error("DB Explorer MCP (TypeScript) Server running on stdio");
}

if (process.env.NODE_ENV !== "test") {
  run().catch((error) => {
    console.error("Fatal error:", error);
    process.exit(1);
  });
}
