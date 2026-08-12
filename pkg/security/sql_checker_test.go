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
