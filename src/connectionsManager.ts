#!/usr/bin/env node

import { Command } from "commander";
import fs from "fs";
import path from "path";
import os from "os";
import readline from "readline";
// @ts-ignore
import oracledb from "oracledb";
import sql from "mssql";
import { Client } from "pg";
import mysql from "mysql2/promise";

import { Writable } from "stream";

class MuteStream extends Writable {
  muted = false;
  _write(chunk: any, encoding: BufferEncoding, callback: (error?: Error | null) => void) {
    if (!this.muted) {
      process.stdout.write(chunk, encoding);
    }
    callback();
  }
}

const mutableStdout = new MuteStream();

const rl = readline.createInterface({
  input: process.stdin,
  output: mutableStdout,
  terminal: true
});

function ask(query: string): Promise<string> {
  return new Promise(resolve => rl.question(query, resolve));
}

function askPassword(query: string): Promise<string> {
  return new Promise(resolve => {
    rl.question(query, (ans) => {
      mutableStdout.muted = false;
      console.log();
      resolve(ans);
    });
    mutableStdout.muted = true;
  });
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
  try {
    fs.chmodSync(CONFIG_PATH, 0o600);
  } catch (err) {
    // Ignora erro no Windows
  }
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
    if (!user) {
      const u = await ask("Usuário [system]: ");
      user = u.trim() === "" ? "system" : u.trim();
    }

    let password = options.password;
    while (!password) password = await askPassword("Senha: ");

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
        if (!sid) {
          const s = await ask("SID / Service Name [XE]: ");
          sid = s.trim() === "" ? "XE" : s.trim();
        }

        dsn = `${host}:${port}/${sid.trim()}`;
      }
    }

    let mode = options.mode;
    while (!mode || !["readonly", "normal", "teste"].includes(mode)) {
      const m = await ask("Modo (readonly, normal, teste) [normal]: ");
      mode = m.trim() === "" ? "normal" : m.trim();
    }

    const isLocal = dsn.toLowerCase().includes("localhost") || dsn.includes("127.0.0.1") || dsn.includes("::1");
    if ((mode === "normal" || mode === "teste") && !isLocal) {
      console.log(`\n❌ Segurança: O modo '${mode}' só é permitido para servidores locais (localhost ou 127.0.0.1). Para bases remotas, utilize o modo 'readonly'.`);
      process.exit(1);
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
  .option("-h, --host <host>", "Host do banco de dados")
  .option("-p, --port <port>", "Porta do banco de dados")
  .option("-i, --instance <instance>", "Instância do banco de dados (opcional)")
  .option("-d, --database <database>", "Nome do banco de dados")
  .option("-u, --user <user>", "Usuário do banco")
  .option("-pw, --password <password>", "Senha do banco")
  .option("-m, --mode <mode>", "Modo de acesso: readonly, normal ou teste")
  .action(async (aliasArg, options) => {
    let alias = aliasArg;
    while (!alias) alias = await ask("Alias (nome da conexão): ");

    let host = options.host;
    while (!host) {
      host = await ask("Host [localhost]: ");
      host = host.trim() === "" ? "localhost" : host.trim();
    }

    let portStr = options.port;
    if (!portStr) {
      portStr = await ask("Porta [1433] (deixe em branco para usar o padrão): ");
      portStr = portStr.trim() === "" ? "1433" : portStr.trim();
    }
    const port = parseInt(portStr, 10);

    let instance = options.instance;
    if (instance === undefined) {
      const instAns = await ask("Instância (ex: SQLEXPRESS, deixe em branco se não usar): ");
      instance = instAns.trim();
    }

    let database = options.database;
    while (!database) database = await ask("Database: ");

    let user = options.user;
    if (!user) {
      const u = await ask("Usuário [sa]: ");
      user = u.trim() === "" ? "sa" : u.trim();
    }

    let password = options.password;
    while (!password) password = await askPassword("Senha: ");

    let mode = options.mode;
    while (!mode || !["readonly", "normal", "teste"].includes(mode)) {
      const m = await ask("Modo (readonly, normal, teste) [normal]: ");
      mode = m.trim() === "" ? "normal" : m.trim();
    }

    const isLocal = host.toLowerCase().includes("localhost") || host.includes("127.0.0.1") || host.includes("::1");
    if ((mode === "normal" || mode === "teste") && !isLocal) {
      console.log(`\n❌ Segurança: O modo '${mode}' só é permitido para servidores locais (localhost ou 127.0.0.1). Para bases remotas, utilize o modo 'readonly'.`);
      process.exit(1);
    }

    console.log("\nTestando a conexão...");
    let success = false;
    try {
      const connectConfig: any = {
        user, password, server: host, database,
        options: { encrypt: true, trustServerCertificate: true }
      };
      if (port) connectConfig.port = port;
      if (instance) connectConfig.options.instanceName = instance;

      const pool = await sql.connect(connectConfig);
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
    config.connections[alias] = { type: "sqlserver", mode, host, port, instance, database, user, password };
    saveConfig(config);
    console.log(`✅ Conexão SQL Server '${alias}' salva com sucesso!`);
    process.exit(0);
  });

program
  .command("add-postgres")
  .description("Adicionar uma conexão PostgreSQL")
  .argument("[alias]", "Nome amigável da conexão")
  .option("-h, --host <host>", "Host do banco de dados")
  .option("-p, --port <port>", "Porta do banco de dados")
  .option("-d, --database <database>", "Nome do banco de dados")
  .option("-u, --user <user>", "Usuário do banco")
  .option("-pw, --password <password>", "Senha do banco")
  .option("-m, --mode <mode>", "Modo de acesso: readonly, normal ou teste")
  .action(async (aliasArg, options) => {
    let alias = aliasArg;
    while (!alias) alias = await ask("Alias (nome da conexão): ");

    let host = options.host;
    if (!host) {
      const h = await ask("Host [localhost]: ");
      host = h.trim() === "" ? "localhost" : h.trim();
    }

    let portStr = options.port;
    if (!portStr) {
      const p = await ask("Porta [5432]: ");
      portStr = p.trim() === "" ? "5432" : p.trim();
    }
    const port = parseInt(portStr as string, 10);

    let database = options.database;
    if (!database) {
      const db = await ask("Database [postgres]: ");
      database = db.trim() === "" ? "postgres" : db.trim();
    }

    let user = options.user;
    if (!user) {
      const u = await ask("Usuário [postgres]: ");
      user = u.trim() === "" ? "postgres" : u.trim();
    }

    let password = options.password;
    while (!password) password = await askPassword("Senha: ");

    let mode = options.mode;
    while (!mode || !["readonly", "normal", "teste"].includes(mode)) {
      const m = await ask("Modo (readonly, normal, teste) [normal]: ");
      mode = m.trim() === "" ? "normal" : m.trim();
    }

    const isLocal = host.toLowerCase().includes("localhost") || host.includes("127.0.0.1") || host.includes("::1");
    if ((mode === "normal" || mode === "teste") && !isLocal) {
      console.log(`\n❌ Segurança: O modo '${mode}' só é permitido para servidores locais (localhost ou 127.0.0.1). Para bases remotas, utilize o modo 'readonly'.`);
      process.exit(1);
    }

    console.log("\nTestando a conexão...");
    let success = false;
    try {
      const conn = new Client({ host, port, database, user, password });
      await conn.connect();
      await conn.end();
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
    config.connections[alias] = { type: "postgres", mode, host, port, database, user, password };
    saveConfig(config);
    console.log(`✅ Conexão PostgreSQL '${alias}' salva com sucesso!`);
    process.exit(0);
  });

program
  .command("add-mysql")
  .description("Adicionar uma conexão MySQL")
  .argument("[alias]", "Nome amigável da conexão")
  .option("-h, --host <host>", "Host do banco de dados")
  .option("-p, --port <port>", "Porta do banco de dados")
  .option("-d, --database <database>", "Nome do banco de dados")
  .option("-u, --user <user>", "Usuário do banco")
  .option("-pw, --password <password>", "Senha do banco")
  .option("-m, --mode <mode>", "Modo de acesso: readonly, normal ou teste")
  .action(async (aliasArg, options) => {
    let alias = aliasArg;
    while (!alias) alias = await ask("Alias (nome da conexão): ");

    let host = options.host;
    if (!host) {
      const h = await ask("Host [localhost]: ");
      host = h.trim() === "" ? "localhost" : h.trim();
    }

    let portStr = options.port;
    if (!portStr) {
      const p = await ask("Porta [3306]: ");
      portStr = p.trim() === "" ? "3306" : p.trim();
    }
    const port = parseInt(portStr as string, 10);

    let database = options.database;
    if (!database) {
      const db = await ask("Database [mysql]: ");
      database = db.trim() === "" ? "mysql" : db.trim();
    }

    let user = options.user;
    if (!user) {
      const u = await ask("Usuário [root]: ");
      user = u.trim() === "" ? "root" : u.trim();
    }

    let password = options.password;
    while (!password) password = await askPassword("Senha: ");

    let mode = options.mode;
    while (!mode || !["readonly", "normal", "teste"].includes(mode)) {
      const m = await ask("Modo (readonly, normal, teste) [normal]: ");
      mode = m.trim() === "" ? "normal" : m.trim();
    }

    const isLocal = host.toLowerCase().includes("localhost") || host.includes("127.0.0.1") || host.includes("::1");
    if ((mode === "normal" || mode === "teste") && !isLocal) {
      console.log(`\n❌ Segurança: O modo '${mode}' só é permitido para servidores locais (localhost ou 127.0.0.1). Para bases remotas, utilize o modo 'readonly'.`);
      process.exit(1);
    }

    console.log("\nTestando a conexão...");
    let success = false;
    try {
      const conn = await mysql.createConnection({ host, port, database, user, password });
      await conn.end();
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
    config.connections[alias] = { type: "mysql", mode, host, port, database, user, password };
    saveConfig(config);
    console.log(`✅ Conexão MySQL '${alias}' salva com sucesso!`);
    process.exit(0);
  });

program
  .command("edit")
  .description("Editar uma conexão existente")
  .argument("<alias>", "Nome amigável da conexão")
  .action(async (alias) => {
    const config = loadConfig();
    const conn = config.connections && config.connections[alias];
    if (!conn) {
      console.log(`❌ Conexão '${alias}' não encontrada.`);
      process.exit(1);
    }

    console.log(`Editando conexão '${alias}' do tipo '${conn.type}'`);
    console.log("Pressione Enter para manter o valor atual.\\n");

    if (conn.type === "oracle") {
      let user = await ask(`Usuário [${conn.user}]: `);
      conn.user = user.trim() === "" ? conn.user : user.trim();

      let password = await askPassword(`Senha [${conn.password ? '***' : ''}]: `);
      conn.password = password.trim() === "" ? conn.password : password.trim();

      let dsn = await ask(`DSN [${conn.dsn}]: `);
      conn.dsn = dsn.trim() === "" ? conn.dsn : dsn.trim();
    } else if (conn.type === "sqlserver") {
      let curHost = conn.host || conn.server || "localhost";
      let curPort = conn.port;
      if (!conn.host && conn.server) {
        const parts = conn.server.split(":");
        curHost = parts[0];
        if (parts[1]) curPort = parseInt(parts[1], 10);
      }
      if (!curPort) curPort = 1433;

      let host = await ask(`Host [${curHost}]: `);
      conn.host = host.trim() === "" ? curHost : host.trim();

      let portStr = await ask(`Porta [${curPort}]: `);
      conn.port = portStr.trim() === "" ? curPort : parseInt(portStr.trim(), 10);

      let instance = await ask(`Instância [${conn.instance || ''}]: `);
      conn.instance = instance.trim() === "" ? conn.instance : instance.trim();

      if (conn.server) delete conn.server;

      let database = await ask(`Database [${conn.database}]: `);
      conn.database = database.trim() === "" ? conn.database : database.trim();

      let user = await ask(`Usuário [${conn.user}]: `);
      conn.user = user.trim() === "" ? conn.user : user.trim();

      let password = await askPassword(`Senha [${conn.password ? '***' : ''}]: `);
      conn.password = password.trim() === "" ? conn.password : password.trim();
    } else if (conn.type === "postgres" || conn.type === "mysql") {
      let host = await ask(`Host [${conn.host}]: `);
      conn.host = host.trim() === "" ? conn.host : host.trim();

      let portStr = await ask(`Porta [${conn.port}]: `);
      conn.port = portStr.trim() === "" ? conn.port : parseInt(portStr.trim(), 10);

      let database = await ask(`Database [${conn.database}]: `);
      conn.database = database.trim() === "" ? conn.database : database.trim();

      let user = await ask(`Usuário [${conn.user}]: `);
      conn.user = user.trim() === "" ? conn.user : user.trim();

      let password = await askPassword(`Senha [${conn.password ? '***' : ''}]: `);
      conn.password = password.trim() === "" ? conn.password : password.trim();
    }

    let mode = await ask(`Modo (readonly, normal, teste) [${conn.mode || 'normal'}]: `);
    conn.mode = mode.trim() === "" ? (conn.mode || "normal") : mode.trim();

    // Verify local restriction for non-readonly modes
    const addressStr = conn.dsn || conn.server || conn.host || "";
    const isLocal = addressStr.toLowerCase().includes("localhost") || addressStr.includes("127.0.0.1") || addressStr.includes("::1");
    if ((conn.mode === "normal" || conn.mode === "teste") && !isLocal) {
      console.log(`\\n❌ Segurança: O modo '${conn.mode}' só é permitido para servidores locais (localhost ou 127.0.0.1). Para bases remotas, utilize o modo 'readonly'.`);
      process.exit(1);
    }

    console.log("\\nTestando a conexão atualizada...");
    let success = false;
    try {
      if (conn.type === "oracle") {
        const oraconn = await oracledb.getConnection({ user: conn.user, password: conn.password, connectString: conn.dsn });
        await oraconn.close();
      } else if (conn.type === "sqlserver") {
        const connectConfig: any = {
          user: conn.user, password: conn.password, server: conn.host, database: conn.database,
          options: { encrypt: true, trustServerCertificate: true }
        };
        if (conn.port) connectConfig.port = conn.port;
        if (conn.instance) connectConfig.options.instanceName = conn.instance;

        const pool = await sql.connect(connectConfig);
        await pool.close();
      } else if (conn.type === "postgres") {
        const pgconn = new Client({ host: conn.host, port: conn.port, database: conn.database, user: conn.user, password: conn.password });
        await pgconn.connect();
        await pgconn.end();
      } else if (conn.type === "mysql") {
        const myconn = await mysql.createConnection({ host: conn.host, port: conn.port, database: conn.database, user: conn.user, password: conn.password });
        await myconn.end();
      }
      success = true;
      console.log("✅ Conexão bem-sucedida!");
    } catch (err: any) {
      console.log("❌ Falha na conexão: " + err.message);
    }

    if (!success) {
      const ans = await ask("\\nDeseja salvar a conexão assim mesmo? (s/n) [n]: ");
      if (ans.toLowerCase() !== 's') {
        console.log("Operação cancelada.");
        process.exit(0);
      }
    }

    saveConfig(config);
    console.log(`✅ Conexão '${alias}' atualizada com sucesso!`);
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
