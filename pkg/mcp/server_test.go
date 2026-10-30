package mcp

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcp_lib "github.com/mark3labs/mcp-go/mcp"
	"github.com/rogick/db-explorer-mcp/pkg/db"
	"github.com/rogick/db-explorer-mcp/pkg/tempfile"
)

func TestParseLimit(t *testing.T) {
	tests := []struct {
		name         string
		input        interface{}
		defaultLimit int
		expected     int
	}{
		{"nil input", nil, 500, 500},
		{"float64 value", float64(100), 500, 100},
		{"float64 zero", float64(0), 500, 0},
		{"float64 negative", float64(-1), 500, -1},
		{"int value", int(250), 500, 250},
		{"int64 value", int64(1000), 500, 1000},
		{"string valid", "1400", 500, 1400},
		{"string with spaces", "  300  ", 500, 300},
		{"string invalid", "abc", 500, 500},
		{"unsupported type", true, 500, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLimit(tt.input, tt.defaultLimit)
			if got != tt.expected {
				t.Errorf("parseLimit(%v, %d) = %d; esperado %d", tt.input, tt.defaultLimit, got, tt.expected)
			}
		})
	}
}

func TestBuildSummaryMessage(t *testing.T) {
	t.Run("Truncado com contagem total e paginação (Página 1)", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:            make([]map[string]interface{}, 100),
			Columns:         []string{"id", "nome"},
			ReturnedCount:   100,
			TotalCount:      1400,
			Offset:          0,
			Truncated:       true,
			ExceededCeiling: false,
		}

		mdMsg, plainMsg := buildSummaryMessage(res, 100)

		if !strings.Contains(mdMsg, "Mostrando linhas 1 a 100 de 1400") {
			t.Errorf("Mensagem Markdown não contém intervalo de linhas esperado: %s", mdMsg)
		}
		if !strings.Contains(mdMsg, "página 1 de 14") {
			t.Errorf("Mensagem Markdown não contém informação de página esperada: %s", mdMsg)
		}
		if !strings.Contains(mdMsg, "page: 2") || !strings.Contains(mdMsg, "offset: 100") {
			t.Errorf("Mensagem Markdown não sugere próxima página com page: 2 ou offset: 100: %s", mdMsg)
		}
		if !strings.Contains(plainMsg, "Mostrando linhas 1 a 100 de 1400") {
			t.Errorf("Mensagem texto simples incorreta: %s", plainMsg)
		}
	})

	t.Run("Truncado com paginação (Página 2)", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:            make([]map[string]interface{}, 100),
			Columns:         []string{"id", "nome"},
			ReturnedCount:   100,
			TotalCount:      1400,
			Offset:          100,
			Truncated:       true,
			ExceededCeiling: false,
		}

		mdMsg, _ := buildSummaryMessage(res, 100)

		if !strings.Contains(mdMsg, "Mostrando linhas 101 a 200 de 1400") {
			t.Errorf("Mensagem Markdown não contém linhas 101 a 200: %s", mdMsg)
		}
		if !strings.Contains(mdMsg, "página 2 de 14") {
			t.Errorf("Mensagem Markdown não contém página 2 de 14: %s", mdMsg)
		}
		if !strings.Contains(mdMsg, "page: 3") || !strings.Contains(mdMsg, "offset: 200") {
			t.Errorf("Mensagem Markdown não sugere próxima página 3 / offset 200: %s", mdMsg)
		}
	})

	t.Run("Última página da paginação", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:            make([]map[string]interface{}, 100),
			Columns:         []string{"id", "nome"},
			ReturnedCount:   100,
			TotalCount:      1400,
			Offset:          1300,
			Truncated:       false,
			ExceededCeiling: false,
		}

		mdMsg, plainMsg := buildSummaryMessage(res, 100)

		if !strings.Contains(mdMsg, "Mostrando linhas 1301 a 1400 de 1400") {
			t.Errorf("Mensagem Markdown não contém linhas 1301 a 1400: %s", mdMsg)
		}
		if !strings.Contains(mdMsg, "fim dos resultados") {
			t.Errorf("Mensagem Markdown não contém 'fim dos resultados': %s", mdMsg)
		}
		if !strings.Contains(plainMsg, "fim dos resultados") {
			t.Errorf("Mensagem texto simples não contém 'fim dos resultados': %s", plainMsg)
		}
	})

	t.Run("Truncado excedendo teto máximo", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:            make([]map[string]interface{}, 100),
			Columns:         []string{"id", "nome"},
			ReturnedCount:   100,
			TotalCount:      10000,
			Offset:          0,
			Truncated:       true,
			ExceededCeiling: true,
		}

		mdMsg, plainMsg := buildSummaryMessage(res, 100)

		if !strings.Contains(mdMsg, "Mostrando linhas 1 a 100 de mais de 10000") {
			t.Errorf("Mensagem Markdown não contém 'mais de 10000': %s", mdMsg)
		}
		if !strings.Contains(plainMsg, "mais de 10000") {
			t.Errorf("Mensagem texto simples incorreta: %s", plainMsg)
		}
	})

	t.Run("Não truncado com múltiplas linhas e sem paginação", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:            make([]map[string]interface{}, 45),
			Columns:         []string{"id", "nome"},
			ReturnedCount:   45,
			TotalCount:      45,
			Offset:          0,
			Truncated:       false,
			ExceededCeiling: false,
		}

		mdMsg, plainMsg := buildSummaryMessage(res, 500)

		if mdMsg != "\n\n*(Total: 45 linhas)*" {
			t.Errorf("Markdown esperado '\\n\\n*(Total: 45 linhas)*', obteve: %q", mdMsg)
		}
		if plainMsg != "Total: 45 linhas" {
			t.Errorf("Texto simples esperado 'Total: 45 linhas', obteve: %q", plainMsg)
		}
	})

	t.Run("Não truncado com 1 linha (singular)", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:            make([]map[string]interface{}, 1),
			Columns:         []string{"id"},
			ReturnedCount:   1,
			TotalCount:      1,
			Offset:          0,
			Truncated:       false,
			ExceededCeiling: false,
		}

		mdMsg, _ := buildSummaryMessage(res, 500)

		if mdMsg != "\n\n*(Total: 1 linha)*" {
			t.Errorf("Markdown esperado '\\n\\n*(Total: 1 linha)*', obteve: %q", mdMsg)
		}
	})

	t.Run("Comando DML com status e rowsAffected", func(t *testing.T) {
		res := &db.QueryResult{
			Rows: []map[string]interface{}{
				{"status": "success", "rowsAffected": int64(3)},
			},
			Columns:       []string{"status", "rowsAffected"},
			ReturnedCount: 1,
			TotalCount:    1,
			Offset:        0,
			Truncated:     false,
		}

		mdMsg, plainMsg := buildSummaryMessage(res, 500)

		if mdMsg != "" || plainMsg != "" {
			t.Errorf("Esperado resumo vazio para DML/status, obteve md=%q, plain=%q", mdMsg, plainMsg)
		}
	})

	t.Run("Offset além do total de registros", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:          []map[string]interface{}{},
			Columns:       []string{"id"},
			ReturnedCount: 0,
			TotalCount:    1400,
			Offset:        2000,
			Truncated:     false,
		}

		mdMsg, plainMsg := buildSummaryMessage(res, 100)

		if !strings.Contains(mdMsg, "Nenhum registro encontrado a partir do offset 2000") {
			t.Errorf("Mensagem Markdown não contém aviso de offset vazio: %s", mdMsg)
		}
		if !strings.Contains(plainMsg, "offset 2000") {
			t.Errorf("Mensagem texto simples não contém offset 2000: %s", plainMsg)
		}
	})
}

func TestToolRegistration(t *testing.T) {
	srv := NewServer()
	if srv == nil {
		t.Fatal("Esperado servidor não nulo")
	}
}

func TestHandleGetTableSchemaValidation(t *testing.T) {
	srv := NewServer()

	// Sem argumentos obrigatórios
	var reqEmpty mcp_lib.CallToolRequest
	reqEmpty.Params.Name = "get_table_schema"
	reqEmpty.Params.Arguments = map[string]interface{}{}

	res, err := srv.handleGetTableSchema(nil, reqEmpty)
	if err != nil {
		t.Fatalf("Erro inesperado retornado: %v", err)
	}
	if !res.IsError {
		t.Errorf("Esperado resultado de erro para argumentos vazios")
	}

	// Conexão inexistente
	var reqNonExistent mcp_lib.CallToolRequest
	reqNonExistent.Params.Name = "get_table_schema"
	reqNonExistent.Params.Arguments = map[string]interface{}{
		"db_alias":   "banco_que_nao_existe_xyz",
		"table_name": "tabela_teste",
	}

	res2, err := srv.handleGetTableSchema(nil, reqNonExistent)
	if err != nil {
		t.Fatalf("Erro inesperado retornado: %v", err)
	}
	if !res2.IsError {
		t.Errorf("Esperado resultado de erro para conexão inexistente")
	}
}

func TestParseStringArg(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]interface{}
		keys     []string
		expected string
	}{
		{
			name:     "mapa vazio",
			args:     map[string]interface{}{},
			keys:     []string{"csv_separator", "csv_delimiter"},
			expected: "",
		},
		{
			name:     "primeira chave presente",
			args:     map[string]interface{}{"csv_separator": ";"},
			keys:     []string{"csv_separator", "csv_delimiter"},
			expected: ";",
		},
		{
			name:     "fallback para segunda chave",
			args:     map[string]interface{}{"csv_delimiter": "\t"},
			keys:     []string{"csv_separator", "csv_delimiter"},
			expected: "\t",
		},
		{
			name:     "primeira chave vazia faz fallback para proxima",
			args:     map[string]interface{}{"csv_separator": "", "separator": "|"},
			keys:     []string{"csv_separator", "separator"},
			expected: "|",
		},
		{
			name:     "preserva espaco simples como separador",
			args:     map[string]interface{}{"csv_separator": " "},
			keys:     []string{"csv_separator"},
			expected: " ",
		},
		{
			name:     "ignora tipo nao string",
			args:     map[string]interface{}{"csv_separator": 123, "csv_delimiter": ";"},
			keys:     []string{"csv_separator", "csv_delimiter"},
			expected: ";",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseStringArg(tt.args, tt.keys...)
			if got != tt.expected {
				t.Errorf("parseStringArg() = %q; esperado %q", got, tt.expected)
			}
		})
	}
}

func TestHandleExecuteQueryValidation(t *testing.T) {
	srv := NewServer()

	// Sem argumentos obrigatórios
	var reqEmpty mcp_lib.CallToolRequest
	reqEmpty.Params.Name = "execute_query"
	reqEmpty.Params.Arguments = map[string]interface{}{}

	res, err := srv.handleExecuteQuery(nil, reqEmpty)
	if err != nil {
		t.Fatalf("Erro inesperado retornado: %v", err)
	}
	if !res.IsError {
		t.Errorf("Esperado resultado de erro para query e db_alias vazios")
	}

	// Conexão inexistente com formato CSV e separador customizado
	var reqNonExistent mcp_lib.CallToolRequest
	reqNonExistent.Params.Name = "execute_query"
	reqNonExistent.Params.Arguments = map[string]interface{}{
		"db_alias":      "banco_que_nao_existe_xyz",
		"query":         "SELECT 1",
		"format":        "csv",
		"csv_separator": ";",
	}

	res2, err := srv.handleExecuteQuery(nil, reqNonExistent)
	if err != nil {
		t.Fatalf("Erro inesperado retornado: %v", err)
	}
	if !res2.IsError {
		t.Errorf("Esperado resultado de erro para conexão inexistente")
	}
}

func TestResolveQueryFormat(t *testing.T) {
	tests := []struct {
		name         string
		format       string
		csvSeparator string
		expected     string
	}{
		{"Default format when empty", "", "", "md"},
		{"Default format when spaces only", "   ", "", "md"},
		{"CSV fallback when separator specified and format empty", "", ";", "csv"},
		{"CSV fallback with tab delimiter", "", "\t", "csv"},
		{"Explicit JSON", "json", "", "json"},
		{"Explicit XML", "xml", "", "xml"},
		{"Explicit TOON", "toon", "", "toon"},
		{"Explicit MD", "md", "", "md"},
		{"Explicit Markdown", "markdown", "", "markdown"},
		{"Explicit CSV with separator", "csv", ";", "csv"},
		{"Explicit format overrides separator", "json", ";", "json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveQueryFormat(tt.format, tt.csvSeparator)
			if got != tt.expected {
				t.Errorf("resolveQueryFormat(%q, %q) = %q; esperado %q", tt.format, tt.csvSeparator, got, tt.expected)
			}
		})
	}
}

func TestNewToolsRegistration(t *testing.T) {
	srv := NewServer()
	if srv == nil {
		t.Fatal("Esperado servidor não nulo")
	}

	// Testa se as 4 novas tools respondem para requisições com validação de parâmetros obrigatórios
	tools := []struct {
		name      string
		handler   func(context.Context, mcp_lib.CallToolRequest) (*mcp_lib.CallToolResult, error)
		emptyArgs map[string]interface{}
	}{
		{
			name:    "list_constraints",
			handler: srv.handleListConstraints,
			emptyArgs: map[string]interface{}{},
		},
		{
			name:    "list_indexes",
			handler: srv.handleListIndexes,
			emptyArgs: map[string]interface{}{},
		},
		{
			name:    "list_routines",
			handler: srv.handleListRoutines,
			emptyArgs: map[string]interface{}{},
		},
		{
			name:    "get_routine_source",
			handler: srv.handleGetRoutineSource,
			emptyArgs: map[string]interface{}{},
		},
	}

	for _, tt := range tools {
		t.Run(tt.name, func(t *testing.T) {
			var req mcp_lib.CallToolRequest
			req.Params.Name = tt.name
			req.Params.Arguments = tt.emptyArgs

			res, err := tt.handler(nil, req)
			if err != nil {
				t.Fatalf("Erro inesperado retornado: %v", err)
			}
			if !res.IsError {
				t.Errorf("Esperado erro para tool %s sem argumentos obrigatórios", tt.name)
			}
		})
	}
}

func TestHandleListConstraintsValidation(t *testing.T) {
	srv := NewServer()

	// Conexão inexistente
	var req mcp_lib.CallToolRequest
	req.Params.Name = "list_constraints"
	req.Params.Arguments = map[string]interface{}{
		"db_alias": "alias_inexistente_123",
	}

	res, err := srv.handleListConstraints(nil, req)
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if !res.IsError {
		t.Errorf("Esperado erro para conexão inexistente")
	}
}

func TestHandleListIndexesValidation(t *testing.T) {
	srv := NewServer()

	var req mcp_lib.CallToolRequest
	req.Params.Name = "list_indexes"
	req.Params.Arguments = map[string]interface{}{
		"db_alias": "alias_inexistente_123",
	}

	res, err := srv.handleListIndexes(nil, req)
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if !res.IsError {
		t.Errorf("Esperado erro para conexão inexistente")
	}
}

func TestHandleListRoutinesValidation(t *testing.T) {
	srv := NewServer()

	var req mcp_lib.CallToolRequest
	req.Params.Name = "list_routines"
	req.Params.Arguments = map[string]interface{}{
		"db_alias": "alias_inexistente_123",
	}

	res, err := srv.handleListRoutines(nil, req)
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if !res.IsError {
		t.Errorf("Esperado erro para conexão inexistente")
	}
}

func TestHandleGetRoutineSourceValidation(t *testing.T) {
	srv := NewServer()

	// Apenas db_alias sem routine_name
	var req mcp_lib.CallToolRequest
	req.Params.Name = "get_routine_source"
	req.Params.Arguments = map[string]interface{}{
		"db_alias": "meu_banco",
	}

	res, err := srv.handleGetRoutineSource(nil, req)
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if !res.IsError {
		t.Errorf("Esperado erro para routine_name ausente")
	}

	// Com routine_name mas conexão inexistente
	req.Params.Arguments = map[string]interface{}{
		"db_alias":     "alias_inexistente_123",
		"routine_name": "prc_processa",
	}

	res2, err := srv.handleGetRoutineSource(nil, req)
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if !res2.IsError {
		t.Errorf("Esperado erro para conexão inexistente")
	}
}

func TestRenderPagedResult_TruncatedFlag(t *testing.T) {
	srv := NewServer()

	paged := &db.PagedResult{
		Rows: []map[string]interface{}{
			{"id": 1, "nome": "item 1"},
			{"id": 2, "nome": "item 2"},
		},
		Columns:       []string{"id", "nome"},
		ReturnedCount: 2,
		TotalCount:    10,
		Offset:        0,
		Limit:         2,
		Truncated:     true,
	}

	// Teste em formato markdown
	resMd, err := srv.renderPagedResult(paged, "md", "")
	if err != nil {
		t.Fatalf("Erro ao renderizar markdown: %v", err)
	}
	mdText := resMd.Content[0].(mcp_lib.TextContent).Text
	if !strings.Contains(mdText, "truncated: true") {
		t.Errorf("Esperado aviso com 'truncated: true' no markdown, obteve: %s", mdText)
	}

	// Teste em formato json
	resJson, err := srv.renderPagedResult(paged, "json", "")
	if err != nil {
		t.Fatalf("Erro ao renderizar json: %v", err)
	}
	if len(resJson.Content) < 2 {
		t.Fatalf("Esperado conteúdo extra com aviso de truncamento em JSON")
	}
	jsonSummary := resJson.Content[1].(mcp_lib.TextContent).Text
	if !strings.Contains(jsonSummary, "truncated: true") {
		t.Errorf("Esperado aviso com 'truncated: true' no resumo do JSON, obteve: %s", jsonSummary)
	}

	// Teste em formato toon
	resToon, err := srv.renderPagedResult(paged, "toon", "")
	if err != nil {
		t.Fatalf("Erro ao renderizar toon: %v", err)
	}
	toonText := resToon.Content[0].(mcp_lib.TextContent).Text
	if !strings.Contains(toonText, "truncated: true") {
		t.Errorf("Esperado aviso com 'truncated: true' no formato toon, obteve: %s", toonText)
	}
}

type mcpMockLobDriver struct{}

func (d *mcpMockLobDriver) Open(name string) (driver.Conn, error) {
	return &mcpMockLobConn{}, nil
}

type mcpMockLobConn struct{}

func (c *mcpMockLobConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare não implementado")
}
func (c *mcpMockLobConn) Close() error              { return nil }
func (c *mcpMockLobConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("tx não implementada") }
func (c *mcpMockLobConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	return &mcpMockLobRows{}, nil
}

type mcpMockLobRows struct {
	current int
}

func (r *mcpMockLobRows) Columns() []string {
	return []string{"id", "cliente_id", "tipo_evento", "payload_xml", "data_evento"}
}

func (r *mcpMockLobRows) ColumnTypeDatabaseTypeName(index int) string {
	if index == 3 {
		return "CLOB"
	}
	return "VARCHAR2"
}

func (r *mcpMockLobRows) Close() error { return nil }

func (r *mcpMockLobRows) Next(dest []driver.Value) error {
	if r.current >= 2 {
		return io.EOF
	}
	r.current++
	dest[0] = int64(1022 + r.current)
	dest[1] = int64(45)
	dest[2] = "NFE_EMISSAO"
	dest[3] = fmt.Sprintf("<NFe><infNFe Id=\"%d\">%s</infNFe></NFe>", r.current, strings.Repeat("A", 3000))
	dest[4] = "2026-10-30 10:15:00"
	return nil
}

func init() {
	sql.Register("mcp_mock_lob_db", &mcpMockLobDriver{})
}

func TestExecuteQuery_SaveToFile(t *testing.T) {
	srv := NewServer()
	tmpDir := t.TempDir()
	srv.tempMgr = tempfile.NewManagerWithDir(tmpDir)

	dbConn, err := sql.Open("mcp_mock_lob_db", "test")
	if err != nil {
		t.Fatalf("Erro ao abrir mock db: %v", err)
	}
	defer dbConn.Close()

	queryRes, err := srv.exec.ExecuteQueryWithOptions(dbConn, "SELECT * FROM nfe_eventos", db.ExecuteQueryOptions{
		Limit:  500,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("Erro ao executar query: %v", err)
	}

	res, err := srv.processQueryExecutionResult("crm_oracle", queryRes, "json", "", 500, true, "")
	if err != nil {
		t.Fatalf("Erro ao processar resultado com save_to_file: %v", err)
	}
	if res.IsError {
		t.Fatalf("Resultado inesperado de erro: %v", res)
	}

	text := res.Content[0].(mcp_lib.TextContent).Text
	var savedResp SavedQueryResponse
	if err := json.Unmarshal([]byte(text), &savedResp); err != nil {
		t.Fatalf("Resposta não é um JSON válido de SavedQueryResponse: %v\nTexto: %s", err, text)
	}

	if !savedResp.SavedToFile {
		t.Errorf("Esperado saved_to_file=true")
	}
	if savedResp.RowCount != 2 {
		t.Errorf("Esperado row_count=2, obteve %d", savedResp.RowCount)
	}
	if savedResp.Format != "json" {
		t.Errorf("Esperado format='json', obteve %q", savedResp.Format)
	}
	if len(savedResp.Columns) != 5 {
		t.Errorf("Esperado 5 colunas, obteve %v", savedResp.Columns)
	}
	if savedResp.Preview == "" {
		t.Errorf("Preview não deveria estar vazio")
	}
	if !strings.Contains(savedResp.Hint, "economizar tokens") {
		t.Errorf("Hint inesperado: %s", savedResp.Hint)
	}

	// Verifica se o arquivo foi realmente gravado no diretório temporário isolado
	if !strings.HasPrefix(savedResp.FilePath, tmpDir) {
		t.Errorf("Arquivo salvo fora do diretório isolado: %s (esperado prefixo %s)", savedResp.FilePath, tmpDir)
	}
	fileBytes, err := os.ReadFile(savedResp.FilePath)
	if err != nil {
		t.Fatalf("Erro ao ler arquivo salvo %s: %v", savedResp.FilePath, err)
	}
	if !strings.Contains(string(fileBytes), "NFE_EMISSAO") {
		t.Errorf("Conteúdo do arquivo salvo não contém os dados esperados")
	}
}

func TestExecuteQuery_LobToFile(t *testing.T) {
	srv := NewServer()
	tmpDir := t.TempDir()
	srv.tempMgr = tempfile.NewManagerWithDir(tmpDir)

	dbConn, err := sql.Open("mcp_mock_lob_db", "test")
	if err != nil {
		t.Fatalf("Erro ao abrir mock db: %v", err)
	}
	defer dbConn.Close()

	queryRes, err := srv.exec.ExecuteQueryWithOptions(dbConn, "SELECT * FROM nfe_eventos", db.ExecuteQueryOptions{
		Limit:        500,
		Offset:       0,
		ExtractLOBs:  true,
		LOBThreshold: 2048,
		TempManager:  srv.tempMgr,
	})
	if err != nil {
		t.Fatalf("Erro ao executar query com lob_to_file: %v", err)
	}

	res, err := srv.processQueryExecutionResult("crm_oracle", queryRes, "md", "", 500, false, "")
	if err != nil {
		t.Fatalf("Erro ao renderizar tabela com lob_to_file: %v", err)
	}

	mdText := res.Content[0].(mcp_lib.TextContent).Text
	if !strings.Contains(mdText, "NFE_EMISSAO") {
		t.Errorf("Tabela Markdown deveria conter colunas normais ('NFE_EMISSAO'): %s", mdText)
	}
	if !strings.Contains(mdText, "[LOB salvo em: ") {
		t.Errorf("Tabela Markdown deveria conter marcador '[LOB salvo em: ...]', obteve:\n%s", mdText)
	}
	if strings.Contains(mdText, strings.Repeat("A", 100)) {
		t.Errorf("Tabela Markdown não deveria conter o payload bruto do LOB inline")
	}

	if len(queryRes.LobFiles) != 2 {
		t.Fatalf("Esperado 2 arquivos LOB gerados, obteve %d", len(queryRes.LobFiles))
	}
	for _, lobPath := range queryRes.LobFiles {
		if filepath.Ext(lobPath) != ".xml" {
			t.Errorf("Extensão esperada .xml para LOB XML, obteve %s", filepath.Ext(lobPath))
		}
		data, err := os.ReadFile(lobPath)
		if err != nil {
			t.Fatalf("Arquivo LOB não encontrado em %s: %v", lobPath, err)
		}
		if !strings.HasPrefix(string(data), "<NFe>") {
			t.Errorf("Conteúdo do arquivo LOB inválido")
		}
	}
}

func TestTempDirIsolationAndCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := tempfile.NewManagerWithDir(tmpDir)

	customFile := filepath.Join(tmpDir, "custom_export", "relatorio.md")
	info, err := mgr.CreateQueryResultFile("erp_db", "md", "| id |\n| --- |\n| 1 |", customFile)
	if err != nil {
		t.Fatalf("Erro ao criar arquivo com file_path customizado: %v", err)
	}
	if info.FilePath != customFile {
		t.Errorf("Esperado caminho %s, obteve %s", customFile, info.FilePath)
	}

	lobInfo, err := mgr.CreateLobFile("xml_col", 1, []byte("<root></root>"))
	if err != nil {
		t.Fatalf("Erro ao criar LOB no tempDir: %v", err)
	}
	oldTime := time.Now().Add(-30 * time.Hour)
	_ = os.Chtimes(lobInfo.FilePath, oldTime, oldTime)

	if err := mgr.CleanupOldFiles(24 * time.Hour); err != nil {
		t.Fatalf("Erro na limpeza: %v", err)
	}
	if _, err := os.Stat(lobInfo.FilePath); !os.IsNotExist(err) {
		t.Errorf("Arquivo LOB expirado deveria ter sido removido")
	}
}

