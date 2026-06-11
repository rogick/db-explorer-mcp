#!/usr/bin/env node

import { Command } from "commander";
import fs from "fs";
import path from "path";
import os from "os";
import readline from "readline";
// @ts-ignore
import oracledb from "oracledb";
import sql from "mssql";

const rl = readline.createInterface({
  input: process.stdin,
  output: process.stdout
});

function ask(query: string): Promise<string> {
  return new Promise(resolve => rl.question(query, resolve));
}

// Ativa o Thick mode para o oracledb
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
}

const CONFIG_PATH = process.env.DB_EXPLORER_CONFIG_PATH 
    ? path.resolve(process.env.DB_EXPLORER_CONFIG_PATH) 
    : path.join(os.homedir(), ".db-explorer-config.json");

function loadConfig() {
  if (fs.existsSync(CONFIG_PATH)) {
    return JSON.parse(fs.readFileSync(CONFIG_PATH, "utf-8"));
  }
  return { connections: {} };
}

function saveConfig(config: any) {
  fs.writeFileSync(CONFIG_PATH, JSON.stringify(config, null, 4));
}

const program = new Command();

program
  .name("db-explorer-manager")
  .description("CLI para gerenciar conexões do DB Explorer MCP")
  .version("1.0.0");

program
  .command("add-oracle")
  .description("Adicionar uma conexão Oracle")
  .argument("[alias]", "Nome amigável da conexão")
  .option("-u, --user <user>", "Usuário do banco")
  .option("-p, --password <password>", "Senha do banco")
  .option("-d, --dsn <dsn>", "String de conexão DSN (ex: localhost:1521/XEPDB1)")
  .option("-m, --mode <mode>", "Modo de acesso: readonly, normal ou teste")
  .action(async (aliasArg, options) => {
    let alias = aliasArg;
    while (!alias) alias = await ask("Alias (nome da conexão): ");
    
    let user = options.user;
    while (!user) user = await ask("Usuário: ");
    
    let password = options.password;
    while (!password) password = await ask("Senha: ");
    
    let dsn = options.dsn;
    if (!dsn) {
        const choice = await ask("Como deseja informar o endereço? (1) Host/Porta/SID ou (2) DSN completo? [1]: ");
        if (choice.trim() === "2") {
            while (!dsn) dsn = await ask("DSN completo (ex: localhost:1521/XE): ");
        } else {
            let host = await ask("Host [localhost]: ");
            host = host.trim() === "" ? "localhost" : host.trim();
            
            let port = await ask("Porta [1521]: ");
            port = port.trim() === "" ? "1521" : port.trim();
            
            let sid = "";
            while (!sid) sid = await ask("SID / Service Name (ex: XE ou XEPDB1): ");
            
            dsn = `${host}:${port}/${sid.trim()}`;
        }
    }
    
    let mode = options.mode;
    while (!mode || !["readonly", "normal", "teste"].includes(mode)) {
        const m = await ask("Modo (readonly, normal, teste) [normal]: ");
        mode = m.trim() === "" ? "normal" : m.trim();
    }

    console.log("\nTestando a conexão...");
    let success = false;
    try {
        const conn = await oracledb.getConnection({ user, password, connectString: dsn });
        await conn.close();
        success = true;
        console.log("✅ Conexão bem-sucedida!");
    } catch (err: any) {
        console.log("❌ Falha na conexão: " + err.message);
    }

    if (!success) {
        const ans = await ask("\nDeseja salvar a conexão assim mesmo? (s/n) [n]: ");
        if (ans.toLowerCase() !== 's') {
            console.log("Operação cancelada.");
            process.exit(0);
        }
    }

    const config = loadConfig();
    config.connections[alias] = { type: "oracle", mode, user, password, dsn };
    saveConfig(config);
    console.log(`✅ Conexão Oracle '${alias}' salva com sucesso!`);
    process.exit(0);
  });

program
  .command("add-sqlserver")
  .description("Adicionar uma conexão SQL Server")
  .argument("[alias]", "Nome amigável da conexão")
  .option("-s, --server <server>", "Servidor (ex: localhost ou localhost:1433)")
  .option("-d, --database <database>", "Nome do banco de dados")
  .option("-u, --user <user>", "Usuário do banco")
  .option("-p, --password <password>", "Senha do banco")
  .option("-m, --mode <mode>", "Modo de acesso: readonly, normal ou teste")
  .action(async (aliasArg, options) => {
    let alias = aliasArg;
    while (!alias) alias = await ask("Alias (nome da conexão): ");
    
    let server = options.server;
    while (!server) server = await ask("Servidor (ex: localhost:1433): ");

    let database = options.database;
    while (!database) database = await ask("Database: ");
    
    let user = options.user;
    while (!user) user = await ask("Usuário: ");
    
    let password = options.password;
    while (!password) password = await ask("Senha: ");
    
    let mode = options.mode;
    while (!mode || !["readonly", "normal", "teste"].includes(mode)) {
        const m = await ask("Modo (readonly, normal, teste) [normal]: ");
        mode = m.trim() === "" ? "normal" : m.trim();
    }

    console.log("\nTestando a conexão...");
    let success = false;
    try {
        const parts = server.split(":");
        const srv = parts[0];
        const port = parts[1] ? parseInt(parts[1], 10) : 1433;
        const pool = await sql.connect({
            user, password, server: srv, port, database,
            options: { encrypt: false, trustServerCertificate: true }
        });
        await pool.close();
        success = true;
        console.log("✅ Conexão bem-sucedida!");
    } catch (err: any) {
        console.log("❌ Falha na conexão: " + err.message);
    }

    if (!success) {
        const ans = await ask("\nDeseja salvar a conexão assim mesmo? (s/n) [n]: ");
        if (ans.toLowerCase() !== 's') {
            console.log("Operação cancelada.");
            process.exit(0);
        }
    }

    const config = loadConfig();
    config.connections[alias] = { type: "sqlserver", mode, server, database, user, password };
    saveConfig(config);
    console.log(`✅ Conexão SQL Server '${alias}' salva com sucesso!`);
    process.exit(0);
  });

program
  .command("list")
  .description("Listar as conexões configuradas")
  .action(() => {
    const config = loadConfig();
    const conns = config.connections || {};
    const aliases = Object.keys(conns);

    if (aliases.length === 0) {
      console.log("Nenhuma conexão configurada.");
    } else {
      console.log("Conexões configuradas:");
      for (const alias of aliases) {
        const c = conns[alias];
        console.log(` - ${alias} (${c.type}) [Modo: ${c.mode || 'normal'}]`);
      }
    }
    process.exit(0);
  });

program
  .command("remove")
  .description("Remover uma conexão")
  .argument("<alias>", "Nome amigável da conexão")
  .action((alias) => {
    const config = loadConfig();
    if (config.connections && config.connections[alias]) {
      delete config.connections[alias];
      saveConfig(config);
      console.log(`✅ Conexão '${alias}' removida.`);
    } else {
      console.log(`❌ Conexão '${alias}' não encontrada.`);
    }
    process.exit(0);
  });

if (process.argv.length <= 2) {
    program.help();
    process.exit(0);
}

program.parseAsync(process.argv).catch((e) => {
    console.error(e);
    process.exit(1);
});
