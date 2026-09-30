package formatters

import (
	"strings"
	"testing"

	"github.com/rogick/db-explorer-mcp/pkg/db"
)

func TestFormatOutputJSON(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": 1, "name": "Alice"},
	}
	out, err := FormatOutput(rows, "json", []string{"id", "name"})
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if !strings.Contains(out, `"id": "1"`) || !strings.Contains(out, `"name": "Alice"`) {
		t.Errorf("Saída JSON incorreta: %s", out)
	}
}

func TestFormatOutputXML(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": 1, "name": "<Alice & Bob>"},
	}
	out, err := FormatOutput(rows, "xml", []string{"id", "name"})
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if !strings.Contains(out, "<name>&lt;Alice &amp; Bob&gt;</name>") {
		t.Errorf("Saída XML não escapou caracteres especiais corretamente: %s", out)
	}
}

func TestFormatOutputMD(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": 1, "name": "Alice"},
	}
	out, err := FormatOutput(rows, "md", []string{"id", "name"})
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if !strings.Contains(out, "| id | name |") || !strings.Contains(out, "| 1 | Alice |") {
		t.Errorf("Saída MD incorreta: %s", out)
	}

	// Teste de compatibilidade com alias llm
	outLLM, err := FormatOutput(rows, "llm", []string{"id", "name"})
	if err != nil {
		t.Fatalf("Erro inesperado ao usar alias llm: %v", err)
	}
	if outLLM != out {
		t.Errorf("Saída com alias llm diverge de md: %s vs %s", outLLM, out)
	}
}

func TestFormatOutputCSV(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": 1, "name": "Alice, Bob", "bio": "Linha 1\nLinha 2"},
		{"id": 2, "name": "Charlie \"The Boss\"", "bio": "Dev"},
	}
	out, err := FormatOutput(rows, "csv", []string{"id", "name", "bio"})
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	expected := "id,name,bio\n1,\"Alice, Bob\",\"Linha 1\nLinha 2\"\n2,\"Charlie \"\"The Boss\"\"\",Dev\n"
	if out != expected {
		t.Errorf("Saída CSV incorreta.\nEsperado:\n%s\nObtido:\n%s", expected, out)
	}

	// Teste com linhas vazias mas com columnOrder
	outEmptyCols, err := FormatOutput([]map[string]interface{}{}, "csv", []string{"id", "name"})
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if outEmptyCols != "id,name\n" {
		t.Errorf("Esperado apenas cabeçalho no CSV vazio com colunas, obtido: %q", outEmptyCols)
	}

	// Teste com linhas vazias e sem colunas
	outEmpty, err := FormatOutput([]map[string]interface{}{}, "csv", nil)
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if outEmpty != "" {
		t.Errorf("Esperado string vazia para CSV sem linhas e sem colunas, obtido: %q", outEmpty)
	}
}

func TestFormatOutputToon(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": 1, "name": "Alice, Bob"},
	}
	out, err := FormatOutput(rows, "toon", []string{"id", "name"})
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if !strings.Contains(out, "results[1]{id,name}:") || !strings.Contains(out, `  1,"Alice, Bob"`) {
		t.Errorf("Saída TOON incorreta: %s", out)
	}
}

func TestFormatTableSchemaMarkdown(t *testing.T) {
	intPtr := func(i int) *int { return &i }
	strPtr := func(s string) *string { return &s }

	schema := &db.TableSchema{
		Table: "clientes",
		Columns: []db.ColumnSchema{
			{
				Column:     "id",
				Type:       "BIGINT",
				Nullable:   false,
				PrimaryKey: true,
			},
			{
				Column:       "nome",
				Type:         "VARCHAR",
				Nullable:     false,
				Length:       intPtr(150),
				DefaultValue: strPtr("'SEM NOME'"),
			},
			{
				Column:    "limite_credito",
				Type:      "DECIMAL",
				Nullable:  true,
				Precision: intPtr(14),
				Scale:     intPtr(2),
				Checks:    []string{"limite_credito >= 0"},
			},
		},
		PrimaryKey: &db.PrimaryKeyInfo{
			Name:    "pk_clientes",
			Columns: []string{"id"},
		},
		ForeignKeys: []db.ForeignKeyInfo{
			{
				Name:              "fk_clientes_cidade",
				Columns:           []string{"cidade_id"},
				ReferencedTable:   "cidades",
				ReferencedColumns: []string{"id"},
			},
		},
		UniqueConstraints: []db.UniqueConstraintInfo{
			{
				Name:    "uk_clientes_cpf",
				Columns: []string{"cpf"},
			},
		},
		CheckConstraints: []db.CheckConstraintInfo{
			{
				Name:    "chk_limite_credito",
				Columns: []string{"limite_credito"},
				Clause:  "limite_credito >= 0",
			},
		},
	}

	t.Run("basic", func(t *testing.T) {
		out := FormatTableSchemaMarkdown(schema, "basic")
		if !strings.Contains(out, "### Tabela: clientes") {
			t.Errorf("Esperado cabeçalho da tabela: %s", out)
		}
		if !strings.Contains(out, "| Coluna | Tipo | Nullable | PK |") {
			t.Errorf("Esperado cabeçalho de colunas basic: %s", out)
		}
		// Não deve conter colunas de tamanho ou checks
		if strings.Contains(out, "Tamanho") || strings.Contains(out, "Checks") {
			t.Errorf("Nível basic não deve conter Tamanho ou Checks: %s", out)
		}
		// Não deve conter seção de Check Constraints
		if strings.Contains(out, "Check Constraints") {
			t.Errorf("Nível basic não deve conter Check Constraints: %s", out)
		}
	})

	t.Run("standard", func(t *testing.T) {
		out := FormatTableSchemaMarkdown(schema, "standard")
		if !strings.Contains(out, "| Coluna | Tipo | Tamanho | Precisão | Nullable | Padrão | PK |") {
			t.Errorf("Esperado cabeçalho standard: %s", out)
		}
		if !strings.Contains(out, "150") || !strings.Contains(out, "14,2") {
			t.Errorf("Esperado tamanho e precisão no standard: %s", out)
		}
		if !strings.Contains(out, "#### Primary Key") || !strings.Contains(out, "#### Foreign Keys") {
			t.Errorf("Esperado PK e FK no standard: %s", out)
		}
		if strings.Contains(out, "#### Check Constraints") {
			t.Errorf("Standard não deve listar Check Constraints: %s", out)
		}
	})

	t.Run("detailed", func(t *testing.T) {
		out := FormatTableSchemaMarkdown(schema, "detailed")
		if !strings.Contains(out, "| Coluna | Tipo | Tamanho | Precisão | Nullable | Padrão | PK | Checks |") {
			t.Errorf("Esperado cabeçalho detailed: %s", out)
		}
		if !strings.Contains(out, "limite_credito >= 0") {
			t.Errorf("Esperado check do campo na tabela: %s", out)
		}
		if !strings.Contains(out, "#### Check Constraints") {
			t.Errorf("Detailed deve listar seção Check Constraints: %s", out)
		}
		if !strings.Contains(out, "chk_limite_credito") {
			t.Errorf("Detailed deve conter nome da constraint de check: %s", out)
		}
	})

	t.Run("nil schema", func(t *testing.T) {
		out := FormatTableSchemaMarkdown(nil, "detailed")
		if out != "Nenhum schema disponível." {
			t.Errorf("Esperado 'Nenhum schema disponível.', obtido: %q", out)
		}
	})
}
