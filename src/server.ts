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
import { Client } from "pg";
import mysql from "mysql2/promise";
import { Parser } from "node-sql-parser";

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
        encrypt: true,
        trustServerCertificate: false,
      },
    });
    return { conn: pool, dbType };
  } else if (dbType === "postgres") {
    const conn = new Client({
      host: details.host,
      port: details.port,
      database: details.database,
      user: details.user,
      password: details.password,
    });
    await conn.connect();
    return { conn, dbType };
  } else if (dbType === "mysql") {
    const conn = await mysql.createConnection({
      host: details.host,
      port: details.port,
      user: details.user,
      password: details.password,
      database: details.database,
    });
    return { conn, dbType };
  } else {
    throw new Error(`Tipo de banco '${dbType}' não suportado.`);
  }
}

export function isSafeQuery(query: string, mode: string): { isSafe: boolean; errorMsg: string } {
  try {
    if (mode === "teste") {
        return { isSafe: true, errorMsg: "" };
    }

    const parser = new Parser();
    const ast = parser.astify(query);
    const asts = Array.isArray(ast) ? ast : [ast];

    if (mode === "readonly") {
        const hasNonSelect = asts.some((a: any) => a.type !== "select");
        if (hasNonSelect) {
            return { isSafe: false, errorMsg: "Conexão em modo 'readonly'. Apenas consultas de leitura são permitidas." };
        }
    } else {
        const hasDestructive = asts.some((a: any) => {
            const type = a.type ? a.type.toLowerCase() : "";
            return type === "drop" || type === "delete" || type === "truncate";
        });
        if (hasDestructive) {
            return { isSafe: false, errorMsg: "Operações destrutivas (DROP, DELETE, TRUNCATE) não são permitidas." };
        }
    }
    
    return { isSafe: true, errorMsg: "" };
  } catch (e: any) {
    if (mode === "teste") {
        return { isSafe: true, errorMsg: "" };
    }
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
  description: "Executa uma consulta SQL no banco especificado. As operações permitidas dependem do modo de cada conexão: modo 'teste' permite TODAS as operações incluindo DROP, DELETE e TRUNCATE; modo 'normal' permite SELECT, CREATE, ALTER, INSERT, UPDATE mas bloqueia DROP, DELETE e TRUNCATE; modo 'readonly' permite apenas SELECT.",
  inputSchema: {
    type: "object",
    properties: {
      db_alias: { type: "string" },
      query: { type: "string" },
      format: { type: "string", description: "Formato de saída: json, xml, llm, toon. Default: json" }
    },
    required: ["db_alias", "query"],
  },
};

server.setRequestHandler(ListToolsRequestSchema, async () => {
  const config = loadConfig();
  const conns = config.connections || {};
  
  const modeDescriptions: Record<string, string> = {
    teste: "TODAS as operações permitidas, incluindo DROP, DELETE e TRUNCATE",
    normal: "permite SELECT, CREATE, ALTER, INSERT, UPDATE; bloqueia DROP, DELETE, TRUNCATE",
    readonly: "apenas SELECT",
  };
  
  const dbsInfo = Object.keys(conns).map(alias => {
    const mode = conns[alias].mode || 'normal';
    const modeDesc = modeDescriptions[mode] || modeDescriptions.normal;
    return `'${alias}' (${conns[alias].type}, modo: ${mode} — ${modeDesc})`;
  }).join("; ");
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
        } else if (dbType === "postgres") {
          const result = await conn.query("SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname != 'pg_catalog' AND schemaname != 'information_schema'");
          tables = result.rows.map((r: any) => r.tablename);
        } else if (dbType === "mysql") {
          const [rows] = await conn.query("SHOW FULL TABLES WHERE Table_type = 'BASE TABLE'");
          tables = (rows as any[]).map((r: any) => Object.values(r)[0] as string);
        }
      } finally {
        if (dbType === "oracle") await conn.close();
        else if (dbType === "sqlserver") await conn.close();
        else if (dbType === "postgres") await conn.end();
        else if (dbType === "mysql") await conn.end();
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
          const result = await conn.execute(
            `SELECT column_name, data_type FROM all_tab_columns WHERE table_name = :tableName`,
            { tableName: table_name.toUpperCase() }
          );
          schema = result.rows.map((r: any) => ({ column: r[0], type: r[1] }));
        } else if (dbType === "sqlserver") {
          const result = await conn.request()
            .input('tableName', sql.VarChar, table_name)
            .query(`SELECT column_name, data_type FROM information_schema.columns WHERE table_name = @tableName`);
          schema = result.recordset.map((r: any) => ({ column: r.column_name, type: r.data_type }));
        } else if (dbType === "postgres") {
          const result = await conn.query(`SELECT column_name, data_type FROM information_schema.columns WHERE table_name = $1`, [table_name]);
          schema = result.rows.map((r: any) => ({ column: r.column_name, type: r.data_type }));
        } else if (dbType === "mysql") {
          const [rows] = await conn.execute(`SELECT column_name, data_type FROM information_schema.columns WHERE table_name = ? AND table_schema = DATABASE()`, [table_name]);
          schema = (rows as any[]).map((r: any) => ({ column: r.COLUMN_NAME, type: r.DATA_TYPE }));
        }
      } finally {
        if (dbType === "oracle") await conn.close();
        else if (dbType === "sqlserver") await conn.close();
        else if (dbType === "postgres") await conn.end();
        else if (dbType === "mysql") await conn.end();
      }

      return {
        content: [{ type: "text", text: JSON.stringify(schema, null, 2) }],
      };
    }

    if (toolName === "execute_query") {
      const db_alias = args.db_alias as string;
      const query = args.query as string;
      const format = (args.format as string) || "json";

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
          const result = await conn.execute(query, [], { outFormat: oracledb.OUT_FORMAT_OBJECT, maxRows: 100, autoCommit: false, timeout: 30000 });
          if (result.rows) {
            results = result.rows;
          } else {
            results = [{ status: "success", rowsAffected: result.rowsAffected || 0 }];
          }
        } else if (dbType === "sqlserver") {
          const request = conn.request();
          request.timeout = 30000;
          const result = await request.query(query);
          if (result.recordsets && result.recordsets.length > 0) {
            results = result.recordsets[0].slice(0, 100);
          } else {
            results = [{ status: "success", rowsAffected: result.rowsAffected[0] || 0 }];
          }
        } else if (dbType === "postgres") {
          const result = await conn.query(query);
          if (Array.isArray(result)) {
             const last = result[result.length - 1];
             if (last.rows) results = last.rows.slice(0, 100);
             else results = [{ status: "success", rowCount: last.rowCount || 0 }];
          } else if (result.rows) {
             results = result.rows.slice(0, 100);
          } else {
             results = [{ status: "success", rowCount: result.rowCount || 0 }];
          }
        } else if (dbType === "mysql") {
          const [rows] = await conn.query(query);
          if (Array.isArray(rows)) {
             results = rows.slice(0, 100);
          } else {
             results = [{ status: "success", affectedRows: (rows as any).affectedRows || 0 }];
          }
        }
      } catch (err: any) {
        results = [{ error: err.message }];
      } finally {
        if (dbType === "oracle") await conn.close();
        else if (dbType === "sqlserver") await conn.close();
        else if (dbType === "postgres") await conn.end();
        else if (dbType === "mysql") await conn.end();
      }

      const stringifiedResults = results.map((row: any) => {
        const newRow: any = {};
        for (const [key, val] of Object.entries(row)) {
          newRow[key] = val !== null && val !== undefined ? String(val) : null;
        }
        return newRow;
      });

      let formattedOutput = "";
      if (format === "xml") {
        formattedOutput = "<results>\n";
        for (const row of stringifiedResults) {
            formattedOutput += "  <row>\n";
            for (const [key, val] of Object.entries(row)) {
                const safeKey = key.replace(/[^a-zA-Z0-9_]/g, "_");
                const safeVal = String(val).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
                formattedOutput += `    <${safeKey}>${safeVal}</${safeKey}>\n`;
            }
            formattedOutput += "  </row>\n";
        }
        formattedOutput += "</results>";
      } else if (format === "llm") {
        if (stringifiedResults.length > 0) {
            const keys = Object.keys(stringifiedResults[0]);
            formattedOutput += "| " + keys.join(" | ") + " |\n";
            formattedOutput += "| " + keys.map(() => "---").join(" | ") + " |\n";
            for (const row of stringifiedResults) {
                formattedOutput += "| " + keys.map(k => String(row[k]).replace(/\\|/g, "\\\\|").replace(/\\n/g, " ")).join(" | ") + " |\n";
            }
        } else {
            formattedOutput = "Nenhum resultado retornado.";
        }
      } else if (format === "toon") {
        if (stringifiedResults.length === 0) {
            formattedOutput = "results[0]{}:\n";
        } else {
            const keys = Object.keys(stringifiedResults[0]);
            formattedOutput = `results[${stringifiedResults.length}]{${keys.join(',')}}:\n`;
            for (const row of stringifiedResults) {
                const values = keys.map(k => {
                    const val = row[k];
                    if (val === null || val === undefined) return "";
                    const str = String(val);
                    if (str.includes(',') || str.includes('\\n') || str.includes('"')) {
                        return `"${str.replace(/"/g, '""')}"`;
                    }
                    return str;
                });
                formattedOutput += `  ${values.join(',')}\n`;
            }
            formattedOutput = formattedOutput.trimEnd();
        }
      } else {
        formattedOutput = JSON.stringify(stringifiedResults, null, 2);
      }

      return {
        content: [{ type: "text", text: formattedOutput }],
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
