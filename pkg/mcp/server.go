package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/rogick/db-explorer-mcp/pkg/config"
	"github.com/rogick/db-explorer-mcp/pkg/db"
	"github.com/rogick/db-explorer-mcp/pkg/formatters"
	"github.com/rogick/db-explorer-mcp/pkg/security"
)

type Server struct {
	mcpServer *mcpserver.MCPServer
	exec      *db.Executor
}

func NewServer() *Server {
	s := &Server{
		exec: db.NewExecutor(),
	}

	mcpSrv := mcpserver.NewMCPServer(
		"db-explorer-mcp",
		"1.0.0",
	)

	s.mcpServer = mcpSrv
	s.registerTools()

	return s
}

func (s *Server) registerTools() {
	// list_databases tool
	listDbsTool := mcp.NewTool(
		"list_databases",
		mcp.WithDescription("Lista os aliases dos bancos de dados configurados disponíveis para consulta."),
	)
	s.mcpServer.AddTool(listDbsTool, s.handleListDatabases)

	// list_tables tool
	listTablesTool := mcp.NewTool(
		"list_tables",
		mcp.WithDescription("Lista as tabelas disponíveis no banco de dados especificado pelo alias."+s.getDynamicDbDescription()),
		mcp.WithString("db_alias", mcp.Required(), mcp.Description("O alias do banco de dados")),
	)
	s.mcpServer.AddTool(listTablesTool, s.handleListTables)

	// get_table_schema tool
	getSchemaTool := mcp.NewTool(
		"get_table_schema",
		mcp.WithDescription("Retorna as colunas e os tipos de dados de uma tabela específica."+s.getDynamicDbDescription()),
		mcp.WithString("db_alias", mcp.Required(), mcp.Description("O alias do banco de dados")),
		mcp.WithString("table_name", mcp.Required(), mcp.Description("O nome da tabela")),
	)
	s.mcpServer.AddTool(getSchemaTool, s.handleGetTableSchema)

	// execute_query tool
	execQueryTool := mcp.NewTool(
		"execute_query",
		mcp.WithDescription("Executa uma consulta SQL no banco especificado. As operações permitidas dependem do modo de cada conexão: modo 'teste' permite TODAS as operações incluindo DROP, DELETE e TRUNCATE; modo 'normal' permite SELECT, CREATE, ALTER, INSERT, UPDATE mas bloqueia DROP, DELETE e TRUNCATE; modo 'readonly' permite apenas SELECT."+s.getDynamicDbDescription()),
		mcp.WithString("db_alias", mcp.Required(), mcp.Description("O alias do banco de dados")),
		mcp.WithString("query", mcp.Required(), mcp.Description("A consulta SQL a ser executada")),
		mcp.WithString("format", mcp.Description("Formato de saída: json, xml, llm, toon. Default: json")),
	)
	s.mcpServer.AddTool(execQueryTool, s.handleExecuteQuery)
}

func (s *Server) getDynamicDbDescription() string {
	cfg, err := config.LoadConfig()
	if err != nil || len(cfg.Connections) == 0 {
		return " Nenhum banco configurado."
	}

	modeDescriptions := map[string]string{
		"teste":    "TODAS as operações permitidas, incluindo DROP, DELETE e TRUNCATE",
		"normal":   "permite SELECT, CREATE, ALTER, INSERT, UPDATE; bloqueia DROP, DELETE, TRUNCATE",
		"readonly": "apenas SELECT",
	}

	var dbsInfo []string
	for alias, conn := range cfg.Connections {
		mode := conn.Mode
		if mode == "" {
			mode = "normal"
		}
		desc, ok := modeDescriptions[mode]
		if !ok {
			desc = modeDescriptions["normal"]
		}
		dbsInfo = append(dbsInfo, fmt.Sprintf("'%s' (%s, modo: %s — %s)", alias, conn.Type, mode, desc))
	}

	return " Bancos disponíveis: " + strings.Join(dbsInfo, "; ") + "."
}

func (s *Server) handleListDatabases(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao carregar configurações: %v", err)), nil
	}

	type dbInfo struct {
		Alias string `json:"alias"`
		Type  string `json:"type"`
		Mode  string `json:"mode"`
	}

	var dbs []dbInfo
	for alias, conn := range cfg.Connections {
		mode := conn.Mode
		if mode == "" {
			mode = "normal"
		}
		dbs = append(dbs, dbInfo{
			Alias: alias,
			Type:  conn.Type,
			Mode:  mode,
		})
	}

	data, _ := json.MarshalIndent(dbs, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleListTables(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbAlias, ok := req.Params.Arguments["db_alias"].(string)
	if !ok || dbAlias == "" {
		return mcp.NewToolResultError("O argumento 'db_alias' é obrigatório."), nil
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao carregar configuração: %v", err)), nil
	}

	connDetails, exists := cfg.Connections[dbAlias]
	if !exists {
		return mcp.NewToolResultError(fmt.Sprintf("Conexão '%s' não encontrada.", dbAlias)), nil
	}

	dbConn, dbType, err := s.exec.OpenConnection(connDetails)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao conectar no banco '%s': %v", dbAlias, err)), nil
	}
	defer dbConn.Close()

	tables, err := s.exec.ListTables(dbConn, dbType)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao listar tabelas do banco '%s': %v", dbAlias, err)), nil
	}

	data, _ := json.MarshalIndent(tables, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleGetTableSchema(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbAlias, _ := req.Params.Arguments["db_alias"].(string)
	tableName, _ := req.Params.Arguments["table_name"].(string)

	if dbAlias == "" || tableName == "" {
		return mcp.NewToolResultError("Os argumentos 'db_alias' e 'table_name' são obrigatórios."), nil
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao carregar configuração: %v", err)), nil
	}

	connDetails, exists := cfg.Connections[dbAlias]
	if !exists {
		return mcp.NewToolResultError(fmt.Sprintf("Conexão '%s' não encontrada.", dbAlias)), nil
	}

	dbConn, dbType, err := s.exec.OpenConnection(connDetails)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao conectar no banco '%s': %v", dbAlias, err)), nil
	}
	defer dbConn.Close()

	schema, err := s.exec.GetTableSchema(dbConn, dbType, tableName)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao obter schema da tabela '%s': %v", tableName, err)), nil
	}

	data, _ := json.MarshalIndent(schema, "", "  ")
	return mcp.NewToolResultText(string(data)), nil
}

func (s *Server) handleExecuteQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbAlias, _ := req.Params.Arguments["db_alias"].(string)
	query, _ := req.Params.Arguments["query"].(string)
	format, _ := req.Params.Arguments["format"].(string)

	if format == "" {
		format = "json"
	}

	if dbAlias == "" || query == "" {
		return mcp.NewToolResultError("Os argumentos 'db_alias' e 'query' são obrigatórios."), nil
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao carregar configuração: %v", err)), nil
	}

	connDetails, exists := cfg.Connections[dbAlias]
	if !exists {
		return mcp.NewToolResultError(fmt.Sprintf("Conexão '%s' não encontrada.", dbAlias)), nil
	}

	mode := connDetails.Mode
	if mode == "" {
		mode = "normal"
	}

	checkRes := security.IsSafeQuery(query, mode)
	if !checkRes.IsSafe {
		errData, _ := json.Marshal([]map[string]string{{"error": fmt.Sprintf("Operação não permitida. %s", checkRes.ErrorMsg)}})
		return mcp.NewToolResultText(string(errData)), nil
	}

	dbConn, _, err := s.exec.OpenConnection(connDetails)
	if err != nil {
		errData, _ := json.Marshal([]map[string]string{{"error": err.Error()}})
		return mcp.NewToolResultText(string(errData)), nil
	}
	defer dbConn.Close()

	rows, cols, err := s.exec.ExecuteQuery(dbConn, query)
	if err != nil {
		errData, _ := json.Marshal([]map[string]string{{"error": err.Error()}})
		return mcp.NewToolResultText(string(errData)), nil
	}

	output, err := formatters.FormatOutput(rows, format, cols)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao formatar resposta: %v", err)), nil
	}

	return mcp.NewToolResultText(output), nil
}

func (s *Server) ServeStdio() error {
	return mcpserver.ServeStdio(s.mcpServer)
}
