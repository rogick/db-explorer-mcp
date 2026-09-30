package db

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeDetailLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"basic", "basic"},
		{"BASIC", "basic"},
		{"resumido", "basic"},
		{"summary", "basic"},
		{"simple", "basic"},

		{"standard", "standard"},
		{"STANDARD", "standard"},
		{"padrao", "standard"},
		{"padrão", "standard"},
		{"normal", "standard"},
		{"medium", "standard"},

		{"detailed", "detailed"},
		{"DETAILED", "detailed"},
		{"full", "detailed"},
		{"completo", "detailed"},
		{"all", "detailed"},
		{"detalhado", "detailed"},
		{"", "detailed"},
		{"desconhecido", "detailed"},
	}

	for _, tt := range tests {
		got := NormalizeDetailLevel(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeDetailLevel(%q) = %q; esperado %q", tt.input, got, tt.expected)
		}
	}
}

func TestSplitSchemaTable(t *testing.T) {
	tests := []struct {
		input          string
		expectedSchema string
		expectedTable  string
	}{
		{"clientes", "", "clientes"},
		{"public.clientes", "public", "clientes"},
		{"dbo.Usuarios", "dbo", "Usuarios"},
		{`"app"."pedidos"`, "app", "pedidos"},
		{"`loja`.`itens`", "loja", "itens"},
		{"  esquema . tabela  ", "esquema", "tabela"},
	}

	for _, tt := range tests {
		s, tbl := splitSchemaTable(tt.input)
		if s != tt.expectedSchema || tbl != tt.expectedTable {
			t.Errorf("splitSchemaTable(%q) = (%q, %q); esperado (%q, %q)", tt.input, s, tbl, tt.expectedSchema, tt.expectedTable)
		}
	}
}

func TestCleanCheckClause(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"CHECK (age >= 18)", "age >= 18"},
		{"CHECK ((age >= 18))", "age >= 18"},
		{"([salario]>(0))", "[salario]>(0)"},
		{"((valor >= 0))", "valor >= 0"},
		{"status IN ('A', 'I')", "status IN ('A', 'I')"},
		{"(status = 'A') AND (tipo = 1)", "(status = 'A') AND (tipo = 1)"},
	}

	for _, tt := range tests {
		got := cleanCheckClause(tt.input)
		if got != tt.expected {
			t.Errorf("cleanCheckClause(%q) = %q; esperado %q", tt.input, got, tt.expected)
		}
	}
}

func TestCleanDefaultValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"((0))", "0"},
		{"('PENDENTE')", "'PENDENTE'"},
		{"(getdate())", "getdate()"},
		{"nextval('seq'::regclass)", "nextval('seq'::regclass)"},
		{"", ""},
	}

	for _, tt := range tests {
		got := cleanDefaultValue(tt.input)
		if got != tt.expected {
			t.Errorf("cleanDefaultValue(%q) = %q; esperado %q", tt.input, got, tt.expected)
		}
	}
}

func TestContainsWord(t *testing.T) {
	tests := []struct {
		text     string
		word     string
		expected bool
	}{
		{"(age >= 18)", "age", true},
		{"(user_age >= 18)", "age", false},
		{"age_years >= 18", "age", false},
		{"status in ('A', 'I')", "status", true},
		{"", "id", false},
	}

	for _, tt := range tests {
		got := containsWord(tt.text, tt.word)
		if got != tt.expected {
			t.Errorf("containsWord(%q, %q) = %v; esperado %v", tt.text, tt.word, got, tt.expected)
		}
	}
}

func TestIsOracleNotNullCheck(t *testing.T) {
	tests := []struct {
		cond     string
		colName  string
		expected bool
	}{
		{`"NOME" IS NOT NULL`, "NOME", true},
		{`NOME IS NOT NULL`, "NOME", true},
		{`("NOME" IS NOT NULL)`, "NOME", true},
		{`"STATUS" IN ('A', 'I')`, "STATUS", false},
		{`"VALOR" > 0`, "VALOR", false},
	}

	for _, tt := range tests {
		got := isOracleNotNullCheck(tt.cond, tt.colName)
		if got != tt.expected {
			t.Errorf("isOracleNotNullCheck(%q, %q) = %v; esperado %v", tt.cond, tt.colName, got, tt.expected)
		}
	}
}

func TestTableSchemaJSONSerialization(t *testing.T) {
	intPtr := func(i int) *int { return &i }
	strPtr := func(s string) *string { return &s }

	schema := &TableSchema{
		Table: "produtos",
		Columns: []ColumnSchema{
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
				Length:       intPtr(100),
				DefaultValue: strPtr("''"),
			},
			{
				Column:    "preco",
				Type:      "DECIMAL",
				Nullable:  false,
				Precision: intPtr(10),
				Scale:     intPtr(2),
				Checks:    []string{"preco >= 0"},
			},
		},
		PrimaryKey: &PrimaryKeyInfo{
			Name:    "pk_produtos",
			Columns: []string{"id"},
		},
		CheckConstraints: []CheckConstraintInfo{
			{
				Name:    "chk_preco",
				Columns: []string{"preco"},
				Clause:  "preco >= 0",
			},
		},
	}

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("Erro ao serializar schema para JSON: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"table":"produtos"`) {
		t.Errorf("JSON não contém nome da tabela: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"length":100`) {
		t.Errorf("JSON não contém tamanho do campo: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"precision":10`) || !strings.Contains(jsonStr, `"scale":2`) {
		t.Errorf("JSON não contém precisão e escala: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"checks":[`) {
		t.Errorf("JSON não contém campo checks: %s", jsonStr)
	}
	var unmarshaled TableSchema
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Erro ao desserializar JSON de volta para TableSchema: %v", err)
	}
	if len(unmarshaled.Columns[2].Checks) != 1 || unmarshaled.Columns[2].Checks[0] != "preco >= 0" {
		t.Errorf("Checks desserializados incorretos: %v", unmarshaled.Columns[2].Checks)
	}
	if !strings.Contains(jsonStr, `"primary_key":{"name":"pk_produtos"`) {
		t.Errorf("JSON não contém primary_key: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, `"check_constraints":`) {
		t.Errorf("JSON não contém check_constraints: %s", jsonStr)
	}
	// OmitEmpty: foreign_keys e unique_constraints vazios não devem aparecer no JSON
	if strings.Contains(jsonStr, `"foreign_keys"`) {
		t.Errorf("JSON não deveria conter foreign_keys vazias com omitempty: %s", jsonStr)
	}
	if strings.Contains(jsonStr, `"unique_constraints"`) {
		t.Errorf("JSON não deveria conter unique_constraints vazias com omitempty: %s", jsonStr)
	}
}
