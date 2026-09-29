package mcp

import (
	"strings"
	"testing"

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
	t.Run("Truncado com contagem total exata", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:            make([]map[string]interface{}, 100),
			Columns:         []string{"id", "nome"},
			ReturnedCount:   100,
			TotalCount:      1400,
			Truncated:       true,
			ExceededCeiling: false,
		}

		mdMsg, plainMsg := buildSummaryMessage(res, 100)

		if !strings.Contains(mdMsg, "Mostrando 100 de 1400 linhas (resultados truncados no limite)") {
			t.Errorf("Mensagem Markdown não contém aviso de truncamento esperado: %s", mdMsg)
		}
		if !strings.Contains(mdMsg, "limit: 1400") {
			t.Errorf("Mensagem Markdown não sugere o limite correto de 1400: %s", mdMsg)
		}
		if !strings.Contains(plainMsg, "Mostrando 100 de 1400 linhas") {
			t.Errorf("Mensagem texto simples incorreta: %s", plainMsg)
		}
	})

	t.Run("Truncado excedendo teto máximo", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:            make([]map[string]interface{}, 100),
			Columns:         []string{"id", "nome"},
			ReturnedCount:   100,
			TotalCount:      10000,
			Truncated:       true,
			ExceededCeiling: true,
		}

		mdMsg, plainMsg := buildSummaryMessage(res, 100)

		if !strings.Contains(mdMsg, "Mostrando 100 de mais de 10000 linhas") {
			t.Errorf("Mensagem Markdown não contém 'mais de 10000': %s", mdMsg)
		}
		if !strings.Contains(plainMsg, "mais de 10000 linhas") {
			t.Errorf("Mensagem texto simples incorreta: %s", plainMsg)
		}
	})

	t.Run("Não truncado com múltiplas linhas", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:            make([]map[string]interface{}, 45),
			Columns:         []string{"id", "nome"},
			ReturnedCount:   45,
			TotalCount:      45,
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
			Truncated:     false,
		}

		mdMsg, plainMsg := buildSummaryMessage(res, 500)

		if mdMsg != "" || plainMsg != "" {
			t.Errorf("Esperado resumo vazio para DML/status, obteve md=%q, plain=%q", mdMsg, plainMsg)
		}
	})

	t.Run("Sem linhas retornadas", func(t *testing.T) {
		res := &db.QueryResult{
			Rows:          []map[string]interface{}{},
			Columns:       []string{"id"},
			ReturnedCount: 0,
			TotalCount:    0,
			Truncated:     false,
		}

		mdMsg, plainMsg := buildSummaryMessage(res, 500)

		if mdMsg != "" || plainMsg != "" {
			t.Errorf("Esperado resumo vazio para 0 linhas, obteve md=%q, plain=%q", mdMsg, plainMsg)
		}
	})
}

func TestToolRegistration(t *testing.T) {
	srv := NewServer()
	if srv == nil {
		t.Fatal("Esperado servidor não nulo")
	}
}
