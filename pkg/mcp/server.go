package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/rogick/db-explorer-mcp/pkg/config"
	"github.com/rogick/db-explorer-mcp/pkg/db"
	"github.com/rogick/db-explorer-mcp/pkg/formatters"
	"github.com/rogick/db-explorer-mcp/pkg/security"
	"github.com/rogick/db-explorer-mcp/pkg/tempfile"
)

// Version holds the current version of the MCP server, injected during build via -ldflags
var Version = "dev"

type Server struct {
	mcpServer *mcpserver.MCPServer
	exec      *db.Executor
	tempMgr   *tempfile.TempFileManager
}

func NewServer() *Server {
	s := &Server{
		exec:    db.NewExecutor(),
		tempMgr: tempfile.NewManager(),
	}

	// Limpeza assíncrona de arquivos temporários com mais de 24 horas
	go func() {
		_ = s.tempMgr.CleanupOldFiles(24 * time.Hour)
	}()

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
		mcp.WithDescription("Executa uma consulta SQL no banco especificado. As operações permitidas dependem do modo de cada conexão: modo 'teste' permite TODAS as operações incluindo DROP, DELETE e TRUNCATE; modo 'normal' permite SELECT, CREATE, ALTER, INSERT, UPDATE mas bloqueia DROP, DELETE e TRUNCATE; modo 'readonly' permite apenas SELECT. DICA PARA ECONOMIA DE TOKENS: Ao consultar tabelas de logs, NF-e, XMLs, JSONs de integração ou colunas CLOB/BLOB/TEXT extensas, utilize 'lob_to_file: true' para extrair automaticamente os campos pesados para arquivos temporários mantendo a tabela enxuta, ou 'save_to_file: true' para salvar todo o resultado em disco."+s.getDynamicDbDescription()),
		mcp.WithString("db_alias", mcp.Required(), mcp.Description("O alias do banco de dados")),
		mcp.WithString("query", mcp.Required(), mcp.Description("A consulta SQL a ser executada")),
		mcp.WithString("format", mcp.Description("Formato de saída: json, xml, md, csv, toon. Default: md")),
		mcp.WithString("csv_separator", mcp.Description("Separador/delimitador de colunas quando format='csv' (ex: ',', ';', '\\t', '|'). Default: ','")),
		mcp.WithNumber("limit", mcp.Description("Limite máximo de linhas a retornar (default: 500, use 0 para sem limite)")),
		mcp.WithNumber("offset", mcp.Description("Número de linhas para pular antes de retornar resultados (default: 0, para paginação)")),
		mcp.WithNumber("page", mcp.Description("Número da página a retornar (1-based, default: 1; ex: page: 2 com limit: 100 equivale a offset: 100)")),
		mcp.WithBoolean("save_to_file", mcp.Description("Se true, salva todo o resultado formatado em um arquivo temporário no disco e retorna apenas o caminho do arquivo, tamanho e um preview inicial. Altamente recomendado para consultas volumosas ou para economia de tokens.")),
		mcp.WithBoolean("lob_to_file", mcp.Description("Se true, mantém as colunas normais na resposta tabular, mas salva automaticamente o conteúdo de campos LOB (CLOB, BLOB, TEXT, XML, JSON grandes) em arquivos temporários individuais, substituindo o valor na tabela pelo caminho do arquivo.")),
		mcp.WithString("file_path", mcp.Description("Caminho customizado de destino para salvar o arquivo (opcional). Se omitido, um arquivo temporário seguro é gerado automaticamente no diretório temp do sistema.")),
		mcp.WithNumber("lob_threshold_bytes", mcp.Description("Tamanho mínimo em bytes (default: 2048) para considerar um campo como LOB quando 'lob_to_file' estiver ativo.")),
	)
	s.mcpServer.AddTool(execQueryTool, s.handleExecuteQuery)

	// list_constraints tool
	listConstraintsTool := mcp.NewTool(
		"list_constraints",
		mcp.WithDescription("Lista as constraints (PK, FK, UNIQUE, CHECK, DEFAULT) de uma tabela ou do schema inteiro, com colunas, referências e regras de integridade. Suporta filtro por tipo e direção (own ou referenced_by para descobrir quem referencia a tabela)."+s.getDynamicDbDescription()),
		mcp.WithString("db_alias", mcp.Required(), mcp.Description("O alias do banco de dados")),
		mcp.WithString("table_name", mcp.Description("Nome da tabela para restringir a busca. Sem ele, lista o schema inteiro (paginado)")),
		mcp.WithString("constraint_type", mcp.Description("Tipo de constraint: 'PK', 'FK', 'UNIQUE', 'CHECK' ou 'DEFAULT'. Sem ele, lista todos os tipos")),
		mcp.WithString("direction", mcp.Description("Direção do relacionamento: 'own' (padrão: constraints pertencentes à tabela) ou 'referenced_by' (FKs de outras tabelas que apontam para esta tabela)")),
		mcp.WithString("owner", mcp.Description("Schema (ou database no MySQL). Padrão: schema/database padrão da conexão")),
		mcp.WithString("format", mcp.Description("Formato de saída: json, xml, md, csv, toon. Default: json")),
		mcp.WithString("csv_separator", mcp.Description("Separador/delimitador de colunas quando format='csv' (ex: ',', ';', '\\t', '|'). Default: ','")),
		mcp.WithNumber("limit", mcp.Description("Limite máximo de registros a retornar (default: 500, use 0 para sem limite)")),
		mcp.WithNumber("offset", mcp.Description("Número de registros para pular antes de retornar resultados (default: 0, para paginação)")),
		mcp.WithNumber("page", mcp.Description("Número da página a retornar (1-based, default: 1; ex: page: 2 com limit: 100 equivale a offset: 100)")),
	)
	s.mcpServer.AddTool(listConstraintsTool, s.handleListConstraints)

	// list_indexes tool
	listIndexesTool := mcp.NewTool(
		"list_indexes",
		mcp.WithDescription("Lista os índices de uma tabela ou do schema inteiro, incluindo unicidade, tipo, colunas ordenadas (ASC/DESC), colunas incluídas (SQL Server/Postgres), predicados de filtro e status."+s.getDynamicDbDescription()),
		mcp.WithString("db_alias", mcp.Required(), mcp.Description("O alias do banco de dados")),
		mcp.WithString("table_name", mcp.Description("Nome da tabela para restringir a busca de índices (opcional)")),
		mcp.WithString("owner", mcp.Description("Schema (ou database no MySQL). Padrão: schema/database padrão da conexão")),
		mcp.WithString("format", mcp.Description("Formato de saída: json, xml, md, csv, toon. Default: json")),
		mcp.WithString("csv_separator", mcp.Description("Separador/delimitador de colunas quando format='csv'. Default: ','")),
		mcp.WithNumber("limit", mcp.Description("Limite máximo de registros a retornar (default: 500, use 0 para sem limite)")),
		mcp.WithNumber("offset", mcp.Description("Número de registros para pular antes de retornar resultados (default: 0, para paginação)")),
		mcp.WithNumber("page", mcp.Description("Número da página a retornar (1-based, default: 1)")),
	)
	s.mcpServer.AddTool(listIndexesTool, s.handleListIndexes)

	// list_routines tool
	listRoutinesTool := mcp.NewTool(
		"list_routines",
		mcp.WithDescription("Lista rotinas do banco de dados (procedures, functions, triggers e packages), com linguagem, status, datas de criação/modificação e detalhes de eventos/timing para triggers."+s.getDynamicDbDescription()),
		mcp.WithString("db_alias", mcp.Required(), mcp.Description("O alias do banco de dados")),
		mcp.WithString("routine_type", mcp.Description("Tipo de rotina: 'PROCEDURE', 'FUNCTION', 'TRIGGER' ou 'PACKAGE' (Oracle). Sem ele, lista todos")),
		mcp.WithString("name_like", mcp.Description("Filtro por nome com suporte a operador LIKE (ex: 'PRC_%')")),
		mcp.WithString("table_name", mcp.Description("Apenas para 'TRIGGER': filtra os triggers da tabela especificada")),
		mcp.WithString("owner", mcp.Description("Schema (ou database no MySQL). Padrão: schema/database padrão da conexão")),
		mcp.WithString("format", mcp.Description("Formato de saída: json, xml, md, csv, toon. Default: json")),
		mcp.WithString("csv_separator", mcp.Description("Separador/delimitador de colunas quando format='csv'. Default: ','")),
		mcp.WithNumber("limit", mcp.Description("Limite máximo de registros a retornar (default: 500, use 0 para sem limite)")),
		mcp.WithNumber("offset", mcp.Description("Número de registros para pular antes de retornar resultados (default: 0, para paginação)")),
		mcp.WithNumber("page", mcp.Description("Número da página a retornar (1-based, default: 1)")),
	)
	s.mcpServer.AddTool(listRoutinesTool, s.handleListRoutines)

	// get_routine_source tool
	getRoutineSourceTool := mcp.NewTool(
		"get_routine_source",
		mcp.WithDescription("Retorna o código-fonte completo de uma procedure, function, trigger ou package no banco de dados. Retorna erro claro caso a rotina não seja encontrada ou o código esteja criptografado/indisponível."+s.getDynamicDbDescription()),
		mcp.WithString("db_alias", mcp.Required(), mcp.Description("O alias do banco de dados")),
		mcp.WithString("routine_name", mcp.Required(), mcp.Description("Nome da rotina")),
		mcp.WithString("routine_type", mcp.Description("Tipo da rotina para desambiguação ('PROCEDURE', 'FUNCTION', 'TRIGGER', 'PACKAGE')")),
		mcp.WithString("owner", mcp.Description("Schema (ou database no MySQL). Padrão: schema/database padrão da conexão")),
		mcp.WithString("format", mcp.Description("Formato de saída: 'json' (default) ou 'md'/'markdown'")),
	)
	s.mcpServer.AddTool(getRoutineSourceTool, s.handleGetRoutineSource)
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

func parseStringArg(args map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if val, exists := args[key]; exists && val != nil {
			if s, ok := val.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
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
			markdownMsg := fmt.Sprintf("\n\n> ⚠️ **Aviso:** Mostrando linhas %d a %d de %s%s (resultados truncados no limite: truncated: true).%s",
				startRow, endRow, totalStr, pageInfo, nextPageHint)
			plainMsg := fmt.Sprintf("⚠️ Aviso: Mostrando linhas %d a %d de %s%s (truncado: truncated: true).%s",
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

func resolveQueryFormat(format string, csvSeparator string) string {
	format = strings.TrimSpace(format)
	if format == "" {
		if csvSeparator != "" {
			return "csv"
		}
		return "md"
	}
	return format
}

func parseBoolArg(val interface{}) bool {
	if val == nil {
		return false
	}
	switch v := val.(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s == "true" || s == "1" || s == "yes"
	case float64:
		return v != 0
	case int:
		return v != 0
	}
	return false
}

// SavedQueryResponse representa o payload JSON retornado quando save_to_file=true.
type SavedQueryResponse struct {
	SavedToFile bool     `json:"saved_to_file"`
	FilePath    string   `json:"file_path"`
	FileSize    string   `json:"file_size"`
	RowCount    int      `json:"row_count"`
	Columns     []string `json:"columns"`
	Format      string   `json:"format"`
	Preview     string   `json:"preview"`
	Hint        string   `json:"hint"`
}

func buildOutputPreview(output string, maxChars int) string {
	if maxChars <= 0 {
		maxChars = 500
	}
	if len(output) <= maxChars {
		return output
	}
	preview := output[:maxChars]
	if lastNL := strings.LastIndex(preview, "\n"); lastNL > maxChars/2 {
		preview = preview[:lastNL]
	}
	return preview + "\n... (primeiros registros)"
}

func (s *Server) handleExecuteQuery(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbAlias, _ := req.Params.Arguments["db_alias"].(string)
	query, _ := req.Params.Arguments["query"].(string)
	format, _ := req.Params.Arguments["format"].(string)

	csvSeparator := parseStringArg(req.Params.Arguments, "csv_separator", "csv_delimiter", "separator", "delimiter")
	format = resolveQueryFormat(format, csvSeparator)

	saveToFile := parseBoolArg(req.Params.Arguments["save_to_file"])
	lobToFile := parseBoolArg(req.Params.Arguments["lob_to_file"])
	customFilePath := parseStringArg(req.Params.Arguments, "file_path")
	lobThreshold := parseLimit(req.Params.Arguments["lob_threshold_bytes"], db.DefaultLOBThresholdBytes)
	if lobThreshold <= 0 {
		lobThreshold = db.DefaultLOBThresholdBytes
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

	queryRes, err := s.exec.ExecuteQueryWithOptions(dbConn, query, db.ExecuteQueryOptions{
		Limit:        limit,
		Offset:       offset,
		ExtractLOBs:  lobToFile,
		LOBThreshold: lobThreshold,
		TempManager:  s.tempMgr,
	})
	if err != nil {
		errData, _ := json.Marshal([]map[string]string{{"error": err.Error()}})
		return mcp.NewToolResultText(string(errData)), nil
	}

	return s.processQueryExecutionResult(dbAlias, queryRes, format, csvSeparator, limit, saveToFile, customFilePath)
}

func (s *Server) processQueryExecutionResult(
	dbAlias string,
	queryRes *db.QueryResult,
	format string,
	csvSeparator string,
	limit int,
	saveToFile bool,
	customFilePath string,
) (*mcp.CallToolResult, error) {
	output, err := formatters.FormatOutput(queryRes.Rows, format, queryRes.Columns, formatters.FormatOptions{
		CSVSeparator: csvSeparator,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao formatar resposta: %v", err)), nil
	}

	if saveToFile {
		mgr := s.tempMgr
		if mgr == nil {
			mgr = tempfile.NewManager()
		}
		fileInfo, err := mgr.CreateQueryResultFile(dbAlias, format, output, customFilePath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Erro ao salvar resultado em arquivo temporário: %v", err)), nil
		}

		resp := SavedQueryResponse{
			SavedToFile: true,
			FilePath:    fileInfo.FilePath,
			FileSize:    fileInfo.FormattedSize,
			RowCount:    queryRes.ReturnedCount,
			Columns:     queryRes.Columns,
			Format:      format,
			Preview:     buildOutputPreview(output, 500),
			Hint:        "O resultado foi salvo no disco para economizar tokens. Para inspecionar trechos específicos, utilize a ferramenta local de leitura de arquivos com paginação de linhas (ex: view_file com StartLine e EndLine).",
		}

		data, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Erro ao serializar metadados do arquivo salvo: %v", err)), nil
		}
		return mcp.NewToolResultText(string(data)), nil
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

func parsePaginationParams(req mcp.CallToolRequest, cfg *config.Config) (int, int) {
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
	return limit, offset
}

func (s *Server) renderPagedResult(
	paged *db.PagedResult,
	format string,
	csvSeparator string,
) (*mcp.CallToolResult, error) {
	output, err := formatters.FormatOutput(paged.Rows, format, paged.Columns, formatters.FormatOptions{
		CSVSeparator: csvSeparator,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao formatar resposta: %v", err)), nil
	}

	queryRes := &db.QueryResult{
		Rows:          paged.Rows,
		Columns:       paged.Columns,
		ReturnedCount: paged.ReturnedCount,
		TotalCount:    paged.TotalCount,
		Offset:        paged.Offset,
		Truncated:     paged.Truncated,
	}

	mdSummary, plainSummary := buildSummaryMessage(queryRes, paged.Limit)

	switch strings.ToLower(format) {
	case "md", "markdown", "llm":
		if mdSummary != "" {
			output += mdSummary
		}
		return mcp.NewToolResultText(output), nil

	case "toon":
		if paged.Truncated || paged.Offset > 0 {
			totalStr := fmt.Sprintf("%d", paged.TotalCount)
			startRow := paged.Offset + 1
			endRow := paged.Offset + paged.ReturnedCount
			output += fmt.Sprintf("\n# paginacao: mostrando linhas %d a %d de %s (truncated: %t; use 'page' ou 'offset' para navegar).", startRow, endRow, totalStr, paged.Truncated)
		}
		return mcp.NewToolResultText(output), nil

	default: // "json", "csv", "xml"
		res := mcp.NewToolResultText(output)
		if paged.Truncated || paged.Offset > 0 {
			res.Content = append(res.Content, mcp.NewTextContent(plainSummary))
		}
		return res, nil
	}
}

func (s *Server) handleListConstraints(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbAlias, _ := req.Params.Arguments["db_alias"].(string)
	tableName, _ := req.Params.Arguments["table_name"].(string)
	constraintType, _ := req.Params.Arguments["constraint_type"].(string)
	direction, _ := req.Params.Arguments["direction"].(string)
	owner, _ := req.Params.Arguments["owner"].(string)
	format, _ := req.Params.Arguments["format"].(string)
	csvSeparator := parseStringArg(req.Params.Arguments, "csv_separator", "csv_delimiter", "separator", "delimiter")

	if format == "" {
		if csvSeparator != "" {
			format = "csv"
		} else {
			format = "json"
		}
	}

	if strings.TrimSpace(dbAlias) == "" {
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

	limit, offset := parsePaginationParams(req, cfg)

	dbConn, dbType, err := s.exec.OpenConnection(connDetails)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao conectar no banco '%s': %v", dbAlias, err)), nil
	}
	defer dbConn.Close()

	resolvedOwner, cleanTable := db.ResolveOwner(connDetails, owner, tableName)

	constraints, err := s.exec.ListConstraints(dbConn, dbType, db.ListConstraintsOptions{
		Owner:          resolvedOwner,
		TableName:      cleanTable,
		ConstraintType: constraintType,
		Direction:      direction,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao listar constraints: %v", err)), nil
	}

	rows := make([]map[string]interface{}, 0, len(constraints))
	for _, c := range constraints {
		rows = append(rows, c.ToMap())
	}

	paged := db.PaginateResult(rows, db.ConstraintColumns, limit, offset)
	return s.renderPagedResult(paged, format, csvSeparator)
}

func (s *Server) handleListIndexes(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbAlias, _ := req.Params.Arguments["db_alias"].(string)
	tableName, _ := req.Params.Arguments["table_name"].(string)
	owner, _ := req.Params.Arguments["owner"].(string)
	format, _ := req.Params.Arguments["format"].(string)
	csvSeparator := parseStringArg(req.Params.Arguments, "csv_separator", "csv_delimiter", "separator", "delimiter")

	if format == "" {
		if csvSeparator != "" {
			format = "csv"
		} else {
			format = "json"
		}
	}

	if strings.TrimSpace(dbAlias) == "" {
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

	limit, offset := parsePaginationParams(req, cfg)

	dbConn, dbType, err := s.exec.OpenConnection(connDetails)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao conectar no banco '%s': %v", dbAlias, err)), nil
	}
	defer dbConn.Close()

	resolvedOwner, cleanTable := db.ResolveOwner(connDetails, owner, tableName)

	indexes, err := s.exec.ListIndexes(dbConn, dbType, db.ListIndexesOptions{
		Owner:     resolvedOwner,
		TableName: cleanTable,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao listar índices: %v", err)), nil
	}

	rows := make([]map[string]interface{}, 0, len(indexes))
	for _, idx := range indexes {
		rows = append(rows, idx.ToMap())
	}

	paged := db.PaginateResult(rows, db.IndexColumns, limit, offset)
	return s.renderPagedResult(paged, format, csvSeparator)
}

func (s *Server) handleListRoutines(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbAlias, _ := req.Params.Arguments["db_alias"].(string)
	routineType, _ := req.Params.Arguments["routine_type"].(string)
	nameLike, _ := req.Params.Arguments["name_like"].(string)
	tableName, _ := req.Params.Arguments["table_name"].(string)
	owner, _ := req.Params.Arguments["owner"].(string)
	format, _ := req.Params.Arguments["format"].(string)
	csvSeparator := parseStringArg(req.Params.Arguments, "csv_separator", "csv_delimiter", "separator", "delimiter")

	if format == "" {
		if csvSeparator != "" {
			format = "csv"
		} else {
			format = "json"
		}
	}

	if strings.TrimSpace(dbAlias) == "" {
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

	limit, offset := parsePaginationParams(req, cfg)

	dbConn, dbType, err := s.exec.OpenConnection(connDetails)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao conectar no banco '%s': %v", dbAlias, err)), nil
	}
	defer dbConn.Close()

	resolvedOwner, cleanTable := db.ResolveOwner(connDetails, owner, tableName)

	routines, err := s.exec.ListRoutines(dbConn, dbType, db.ListRoutinesOptions{
		Owner:       resolvedOwner,
		RoutineType: routineType,
		NameLike:    nameLike,
		TableName:   cleanTable,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Erro ao listar rotinas: %v", err)), nil
	}

	rows := make([]map[string]interface{}, 0, len(routines))
	for _, r := range routines {
		rows = append(rows, r.ToMap())
	}

	paged := db.PaginateResult(rows, db.RoutineColumns, limit, offset)
	return s.renderPagedResult(paged, format, csvSeparator)
}

func (s *Server) handleGetRoutineSource(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dbAlias, _ := req.Params.Arguments["db_alias"].(string)
	routineName, _ := req.Params.Arguments["routine_name"].(string)
	routineType, _ := req.Params.Arguments["routine_type"].(string)
	owner, _ := req.Params.Arguments["owner"].(string)
	format, _ := req.Params.Arguments["format"].(string)

	if strings.TrimSpace(dbAlias) == "" || strings.TrimSpace(routineName) == "" {
		return mcp.NewToolResultError("Os argumentos 'db_alias' e 'routine_name' são obrigatórios."), nil
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

	resolvedOwner, cleanRoutine := db.ResolveOwner(connDetails, owner, routineName)

	res, err := s.exec.GetRoutineSource(dbConn, dbType, db.GetRoutineSourceOptions{
		Owner:       resolvedOwner,
		RoutineName: cleanRoutine,
		RoutineType: routineType,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	switch strings.ToLower(strings.TrimSpace(format)) {
	case "md", "markdown", "llm":
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("### Rotina: `%s`.`%s` (%s)\n\n", res.Owner, res.RoutineName, res.RoutineType))
		sb.WriteString("```sql\n")
		sb.WriteString(res.Source)
		sb.WriteString("\n```")
		return mcp.NewToolResultText(sb.String()), nil
	default:
		data, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Erro ao serializar fonte: %v", err)), nil
		}
		return mcp.NewToolResultText(string(data)), nil
	}
}

func (s *Server) ServeStdio() error {
	return mcpserver.ServeStdio(s.mcpServer)
}
