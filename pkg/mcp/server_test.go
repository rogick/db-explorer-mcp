package mcp

import (
	"strings"
	"testing"

	mcp_lib "github.com/mark3labs/mcp-go/mcp"
	"github.com/rogick/db-explorer-mcp/pkg/db"
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
