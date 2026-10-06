package security

import (
	"testing"
)

func TestIsSafeQueryReadonlyMode(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{"SELECT * FROM users", true},
		{"SELECT id, name FROM clients WHERE id = 1", true},
		{"WITH cte AS (SELECT 1 AS a) SELECT * FROM cte", true},
		{"SHOW TABLES", true},
		{"EXPLAIN SELECT * FROM orders", true},
		{"INSERT INTO users (name) VALUES ('test')", false},
		{"UPDATE users SET name = 'admin' WHERE id = 1", false},
		{"DELETE FROM users", false},
		{"DROP TABLE users", false},
		{"CREATE TABLE foo (id INT)", false},
		{"TRUNCATE TABLE users", false},
		{"SELECT * FROM users; DROP TABLE clients;", false},
		{"SELECT * FROM users -- DELETE FROM users", true}, // comentário seguro
		{"SELECT 'DELETE FROM users' FROM dual", true},     // string literal segura
	}

	for _, tt := range tests {
		res := IsSafeQuery(tt.query, "readonly")
		if res.IsSafe != tt.expected {
			t.Errorf("Readonly: IsSafeQuery(%q) = %v (%s); esperado %v", tt.query, res.IsSafe, res.ErrorMsg, tt.expected)
		}
	}
}

func TestIsSafeQueryNormalMode(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{"SELECT * FROM users", true},
		{"INSERT INTO users (name) VALUES ('test')", true},
		{"UPDATE users SET name = 'admin' WHERE id = 1", true},
		{"CREATE TABLE foo (id INT)", true},
		{"ALTER TABLE foo ADD col INT", true},
		{"DELETE FROM users", false},
		{"DROP TABLE users", false},
		{"TRUNCATE TABLE users", false},
		{"INSERT INTO logs VALUES ('DROP TABLE');", true}, // string literal 'DROP TABLE' em INSERT
		{"UPDATE status SET msg = 'DELETE';", true},       // string literal 'DELETE' em UPDATE
		{"DROP TABLE users; -- safe comment", false},
	}

	for _, tt := range tests {
		res := IsSafeQuery(tt.query, "normal")
		if res.IsSafe != tt.expected {
			t.Errorf("Normal: IsSafeQuery(%q) = %v (%s); esperado %v", tt.query, res.IsSafe, res.ErrorMsg, tt.expected)
		}
	}
}

func TestIsSafeQueryTesteMode(t *testing.T) {
	queries := []string{
		"SELECT * FROM users",
		"DROP TABLE users",
		"DELETE FROM users",
		"TRUNCATE TABLE users",
	}

	for _, q := range queries {
		res := IsSafeQuery(q, "teste")
		if !res.IsSafe {
			t.Errorf("Teste mode should allow query %q, got error: %s", q, res.ErrorMsg)
		}
	}
}

func TestValidateIdentifier(t *testing.T) {
	valid := []string{
		"users",
		"public.users",
		"dbo.orders",
		"MY_TABLE_123",
		"schema.table_name",
		"",
	}
	for _, id := range valid {
		if err := ValidateIdentifier(id); err != nil {
			t.Errorf("Esperado identificador válido para %q, obteve erro: %v", id, err)
		}
	}

	invalid := []string{
		"users; DROP TABLE clients;",
		"users' OR 1=1 --",
		`users" OR "1"="1"`,
		"users` WHERE 1=1",
		"table--comment",
		"table/*comment*/",
		"table\nname",
	}
	for _, id := range invalid {
		if err := ValidateIdentifier(id); err == nil {
			t.Errorf("Esperado erro para identificador malicioso %q, mas passou", id)
		}
	}
}

func TestValidateLikePattern(t *testing.T) {
	valid := []string{
		"PRC_%",
		"%user%",
		"fn_calc",
		"",
	}
	for _, p := range valid {
		if err := ValidateLikePattern(p); err != nil {
			t.Errorf("Esperado padrão LIKE válido para %q, obteve erro: %v", p, err)
		}
	}

	invalid := []string{
		"prc%; DROP TABLE x;",
		"prc' OR 1=1",
		"prc`",
	}
	for _, p := range invalid {
		if err := ValidateLikePattern(p); err == nil {
			t.Errorf("Esperado erro para padrão LIKE perigoso %q, mas passou", p)
		}
	}
}

