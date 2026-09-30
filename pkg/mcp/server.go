package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/rogick/db-explorer-mcp/pkg/config"
	"github.com/rogick/db-explorer-mcp/pkg/db"
	"github.com/rogick/db-explorer-mcp/pkg/formatters"
	"github.com/rogick/db-explorer-mcp/pkg/security"
)

// Version holds the current version of the MCP server, injected during build via -ldflags
var Version = "dev"

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
		Version,
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
		mcp.WithDescription("Retorna o schema detalhado de uma tabela específica, incluindo colunas, tipos de dados, tamanho, precisão, escala, nullable, valores padrão, checks de validação dos campos e constraints (PK, FK, Unique, Check). Permite escolher o nível de detalhamento via 'detail_level'."+s.getDynamicDbDescription()),
		mcp.WithString("db_alias", mcp.Required(), mcp.Description("O alias do banco de dados")),
		mcp.WithString("table_name", mcp.Required(), mcp.Description("O nome da tabela")),
		mcp.WithString("detail_level", mcp.Description("Nível de detalhamento do schema: 'detailed' ou 'full' (default: completo, com tamanho, precisão, nullable, default, checks dos campos e todas as constraints), 'standard' (inclui tamanho, precisão, default e constraints PK/FK/Unique, sem checks), ou 'basic' (apenas colunas, tipos, nullable e PK).")),
		mcp.WithString("format", mcp.Description("Formato de saída: 'json' (default) ou 'md'/'markdown'.")),
	)
	s.mcpServer.AddTool(getSchemaTool, s.handleGetTableSchema)

	// execute_query tool
	execQueryTool := mcp.NewTool(
		"execute_query",
		mcp.WithDescription("Executa uma consulta SQL no banco especificado. As operações permitidas dependem do modo de cada conexão: modo 'teste' permite TODAS as operações incluindo DROP, DELETE e TRUNCATE; modo 'normal' permite SELECT, CREATE, ALTER, INSERT, UPDATE mas bloqueia DROP, DELETE e TRUNCATE; modo 'readonly' permite apenas SELECT."+s.getDynamicDbDescription()),
		mcp.WithString("db_alias", mcp.Required(), mcp.Description("O alias do banco de dados")),
		mcp.WithString("query", mcp.Required(), mcp.Description("A consulta SQL a ser executada")),
		mcp.WithString("format", mcp.Description("Formato de saída: json, xml, md, csv, toon. Default: json")),
		mcp.WithNumber("limit", mcp.Description("Limite máximo de linhas a retornar (default: 500, use 0 para sem limite)")),
		mcp.WithNumber("offset", mcp.Description("Número de linhas para pular antes de retornar resultados (default: 0, para paginação)")),
		mcp.WithNumber("page", mcp.Description("Número da página a retornar (1-based, default: 1; ex: page: 2 com limit: 100 equivale a offset: 100)")),
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

	connDetails, _, exists := cfg.GetConnection(dbAlias)
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
	detailLevel, _ := req.Params.Arguments["detail_level"].(string)
	format, _ := req.Params.Arguments["format"].(string)

	if dbAlias == "" || tableName == "" {
		return mcp.NewToolResultError("Os argumentos 'db_alias' e 'table_name' são obrigatórios."), nil
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao carregar configuração: %v", err)), nil
	}

	connDetails, _, exists := cfg.GetConnection(dbAlias)
	if !exists {
		return mcp.NewToolResultError(fmt.Sprintf("Conexão '%s' não encontrada.", dbAlias)), nil
	}

	dbConn, dbType, err := s.exec.OpenConnection(connDetails)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao conectar no banco '%s': %v", dbAlias, err)), nil
	}
	defer dbConn.Close()

	schema, err := s.exec.GetTableSchema(dbConn, dbType, tableName, detailLevel)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao obter schema da tabela '%s': %v", tableName, err)), nil
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "md", "markdown", "llm":
		output := formatters.FormatTableSchemaMarkdown(schema, detailLevel)
		return mcp.NewToolResultText(output), nil
	default:
		data, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Erro ao serializar schema: %v", err)), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

func parseLimit(val interface{}, defaultLimit int) int {
	if val == nil {
		return defaultLimit
	}
	switch v := val.(type) {
	case float64:
		return int(v)
	case float32:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case string:
		v = strings.TrimSpace(v)
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultLimit
}

func buildSummaryMessage(res *db.QueryResult, limit int) (string, string) {
	// Se for resultado de comando DML/DDL (status e rowsAffected), não exibe contagem de linhas
	if len(res.Rows) == 1 && len(res.Columns) == 2 && res.Columns[0] == "status" && res.Columns[1] == "rowsAffected" {
		return "", ""
	}

	if len(res.Rows) == 0 && res.Offset == 0 {
		return "", ""
	}

	if len(res.Rows) == 0 && res.Offset > 0 {
		msg := fmt.Sprintf("\n\n> ℹ️ **Paginação:** Nenhum registro encontrado a partir do offset %d (total de registros: %d).", res.Offset, res.TotalCount)
		plain := fmt.Sprintf("Nenhum registro encontrado a partir do offset %d (total: %d).", res.Offset, res.TotalCount)
		return msg, plain
	}

	startRow := res.Offset + 1
	endRow := res.Offset + res.ReturnedCount

	totalStr := fmt.Sprintf("%d", res.TotalCount)
	if res.ExceededCeiling {
		totalStr = fmt.Sprintf("mais de %d", res.TotalCount)
	}

	// Informações de página quando limit > 0
	pageInfo := ""
	nextPageHint := ""
	if limit > 0 {
		currentPage := (res.Offset / limit) + 1
		totalPages := (res.TotalCount + limit - 1) / limit
		if res.ExceededCeiling {
			pageInfo = fmt.Sprintf(" — página %d de mais de %d", currentPage, totalPages)
		} else if totalPages > 1 {
			pageInfo = fmt.Sprintf(" — página %d de %d", currentPage, totalPages)
		}

		if res.Truncated {
			nextPage := currentPage + 1
			nextOffset := res.Offset + res.ReturnedCount
			nextPageHint = fmt.Sprintf(" Para a próxima página, use `page: %d` ou `offset: %d`. Para sem limite, use `limit: 0`.", nextPage, nextOffset)
		}
	}

	if res.Truncated || res.Offset > 0 {
		if res.Truncated {
			markdownMsg := fmt.Sprintf("\n\n> ⚠️ **Aviso:** Mostrando linhas %d a %d de %s%s (resultados truncados no limite).%s",
				startRow, endRow, totalStr, pageInfo, nextPageHint)
			plainMsg := fmt.Sprintf("⚠️ Aviso: Mostrando linhas %d a %d de %s%s (truncado).%s",
				startRow, endRow, totalStr, pageInfo, nextPageHint)
			return markdownMsg, plainMsg
		}

		// Última página de uma consulta paginada
		markdownMsg := fmt.Sprintf("\n\n*(Mostrando linhas %d a %d de %s%s — fim dos resultados)*",
			startRow, endRow, totalStr, pageInfo)
		plainMsg := fmt.Sprintf("Mostrando linhas %d a %d de %s%s (fim dos resultados).",
			startRow, endRow, totalStr, pageInfo)
		return markdownMsg, plainMsg
	}

	// Não truncado e sem offset (página única completa)
	plural := "linhas"
	if res.ReturnedCount == 1 {
		plural = "linha"
	}
	markdownMsg := fmt.Sprintf("\n\n*(Total: %d %s)*", res.ReturnedCount, plural)
	plainMsg := fmt.Sprintf("Total: %d %s", res.ReturnedCount, plural)
	return markdownMsg, plainMsg
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

	limit := 500
	if cfg.DefaultLimit > 0 {
		limit = cfg.DefaultLimit
	}
	if limitArg, exists := req.Params.Arguments["limit"]; exists && limitArg != nil {
		limit = parseLimit(limitArg, limit)
	}

	offset := 0
	if offsetArg, exists := req.Params.Arguments["offset"]; exists && offsetArg != nil {
		offset = parseLimit(offsetArg, 0)
		if offset < 0 {
			offset = 0
		}
	} else if pageArg, exists := req.Params.Arguments["page"]; exists && pageArg != nil {
		page := parseLimit(pageArg, 1)
		if page > 1 && limit > 0 {
			offset = (page - 1) * limit
		}
	}

	connDetails, _, exists := cfg.GetConnection(dbAlias)
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

	queryRes, err := s.exec.ExecuteQuery(dbConn, query, limit, offset)
	if err != nil {
		errData, _ := json.Marshal([]map[string]string{{"error": err.Error()}})
		return mcp.NewToolResultText(string(errData)), nil
	}

	output, err := formatters.FormatOutput(queryRes.Rows, format, queryRes.Columns)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao formatar resposta: %v", err)), nil
	}

	mdSummary, plainSummary := buildSummaryMessage(queryRes, limit)

	switch strings.ToLower(format) {
	case "md", "markdown", "llm":
		if mdSummary != "" {
			output += mdSummary
		}
		return mcp.NewToolResultText(output), nil

	case "toon":
		if queryRes.Truncated || queryRes.Offset > 0 {
			totalStr := fmt.Sprintf("%d", queryRes.TotalCount)
			if queryRes.ExceededCeiling {
				totalStr = fmt.Sprintf("mais de %d", queryRes.TotalCount)
			}
			startRow := queryRes.Offset + 1
			endRow := queryRes.Offset + queryRes.ReturnedCount
			output += fmt.Sprintf("\n# paginacao: mostrando linhas %d a %d de %s (use 'page' ou 'offset' para navegar).", startRow, endRow, totalStr)
		}
		return mcp.NewToolResultText(output), nil

	default: // "json", "csv", "xml"
		res := mcp.NewToolResultText(output)
		if queryRes.Truncated || queryRes.Offset > 0 {
			res.Content = append(res.Content, mcp.NewTextContent(plainSummary))
		}
		return res, nil
	}
}

func (s *Server) ServeStdio() error {
	return mcpserver.ServeStdio(s.mcpServer)
}
