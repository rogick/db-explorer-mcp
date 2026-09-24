package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rogick/db-explorer-mcp/pkg/config"
)

func TestPrintConnectionDetails_NeverExposesPassword(t *testing.T) {
	secretPassword := "SuperSecretPassword123!@#"

	testCases := []struct {
		name     string
		alias    string
		conn     config.ConnectionDetails
		expected []string
	}{
		{
			name:  "Oracle connection with password",
			alias: "oracle_prod",
			conn: config.ConnectionDetails{
				Type:     "oracle",
				Mode:     "readonly",
				User:     "system",
				Password: secretPassword,
				DSN:      "192.168.1.100:1521/ORCL",
			},
			expected: []string{
				"Detalhes da conexão 'oracle_prod':",
				"Tipo:      oracle",
				"Modo:      readonly",
				"Usuário:   system",
				"Senha:     ********",
				"DSN:       192.168.1.100:1521/ORCL",
			},
		},
		{
			name:  "SQL Server connection with instance",
			alias: "sql_local",
			conn: config.ConnectionDetails{
				Type:     "sqlserver",
				Mode:     "normal",
				User:     "sa",
				Password: secretPassword,
				Host:     "localhost",
				Port:     1433,
				Instance: "SQLEXPRESS",
				Database: "sankhya",
			},
			expected: []string{
				"Detalhes da conexão 'sql_local':",
				"Tipo:      sqlserver",
				"Modo:      normal",
				"Usuário:   sa",
				"Senha:     ********",
				"Host:      localhost",
				"Porta:     1433",
				"Instância: SQLEXPRESS",
				"Database:  sankhya",
			},
		},
		{
			name:  "SQL Server fallback legacy Server field",
			alias: "sql_legacy",
			conn: config.ConnectionDetails{
				Type:     "sqlserver",
				Mode:     "teste",
				User:     "sa",
				Password: secretPassword,
				Server:   "127.0.0.1",
				Database: "master",
			},
			expected: []string{
				"Detalhes da conexão 'sql_legacy':",
				"Tipo:      sqlserver",
				"Modo:      teste",
				"Host:      127.0.0.1",
				"Database:  master",
				"Senha:     ********",
			},
		},
		{
			name:  "PostgreSQL connection",
			alias: "pg_test",
			conn: config.ConnectionDetails{
				Type:     "postgres",
				Mode:     "readonly",
				User:     "postgres",
				Password: secretPassword,
				Host:     "localhost",
				Port:     5432,
				Database: "appdb",
			},
			expected: []string{
				"Detalhes da conexão 'pg_test':",
				"Tipo:      postgres",
				"Modo:      readonly",
				"Usuário:   postgres",
				"Senha:     ********",
				"Host:      localhost",
				"Porta:     5432",
				"Database:  appdb",
			},
		},
		{
			name:  "MySQL connection without password",
			alias: "mysql_dev",
			conn: config.ConnectionDetails{
				Type:     "mysql",
				Mode:     "normal",
				User:     "root",
				Password: "",
				Host:     "localhost",
				Port:     3306,
				Database: "devdb",
			},
			expected: []string{
				"Detalhes da conexão 'mysql_dev':",
				"Tipo:      mysql",
				"Modo:      normal",
				"Usuário:   root",
				"Senha:     (não definida)",
				"Host:      localhost",
				"Porta:     3306",
				"Database:  devdb",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			printConnectionDetails(&buf, tc.alias, tc.conn)
			output := buf.String()

			// CRITICAL: The plain text password must NEVER appear in the output
			if strings.Contains(output, secretPassword) {
				t.Fatalf("SECURITY VIOLATION: Plaintext password %q leaked in output:\n%s", secretPassword, output)
			}

			// Verify all expected lines are present
			for _, exp := range tc.expected {
				if !strings.Contains(output, exp) {
					t.Errorf("Expected output to contain %q, but got:\n%s", exp, output)
				}
			}
		})
	}
}

func TestHandleList(t *testing.T) {
	cfg := &config.Config{
		Connections: map[string]config.ConnectionDetails{
			"SOFTRH_GUI_REMOTO": {Type: "oracle", Mode: "readonly", DSN: "192.168.1.100:1521/ORCL"},
			"SOFTRH_LOCAL":      {Type: "oracle", Mode: "normal", DSN: "localhost:1521/XE"},
			"sankhya_prod":      {Type: "postgres", Mode: "readonly", Host: "localhost", Database: "sankhya"},
		},
	}

	t.Run("List all when no args", func(t *testing.T) {
		var buf bytes.Buffer
		err := handleList(&buf, cfg, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Conexões configuradas:") ||
			!strings.Contains(out, "SOFTRH_GUI_REMOTO") ||
			!strings.Contains(out, "SOFTRH_LOCAL") ||
			!strings.Contains(out, "sankhya_prod") {
			t.Errorf("Expected all connections in list output, got:\n%s", out)
		}
	})

	t.Run("List with approximate name matching single connection shows details", func(t *testing.T) {
		var buf bytes.Buffer
		// "sankya" typo matching "sankhya_prod"
		err := handleList(&buf, cfg, []string{"sankya"})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Detalhes da conexão 'sankhya_prod':") {
			t.Errorf("Expected single match details, got:\n%s", out)
		}
	})

	t.Run("List with approximate name matching multiple connections lists matches", func(t *testing.T) {
		var buf bytes.Buffer
		err := handleList(&buf, cfg, []string{"softrh"})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "correspondências para 'softrh'") ||
			!strings.Contains(out, "SOFTRH_GUI_REMOTO") ||
			!strings.Contains(out, "SOFTRH_LOCAL") {
			t.Errorf("Expected multiple matches listed, got:\n%s", out)
		}
	})

	t.Run("List with non-existent connection returns error", func(t *testing.T) {
		var buf bytes.Buffer
		err := handleList(&buf, cfg, []string{"nao_existe_xyz"})
		if err == nil {
			t.Fatal("Expected error for non-existent connection, got nil")
		}
	})
}

func TestHandleShow(t *testing.T) {
	cfg := &config.Config{
		Connections: map[string]config.ConnectionDetails{
			"SOFTRH_GUI_REMOTO": {Type: "oracle", Mode: "readonly", DSN: "192.168.1.100:1521/ORCL"},
			"SOFTRH_LOCAL":      {Type: "oracle", Mode: "normal", DSN: "localhost:1521/XE"},
			"sankhya_prod":      {Type: "postgres", Mode: "readonly", Host: "localhost", Database: "sankhya"},
		},
	}

	t.Run("Show with approximate name matching single connection shows details", func(t *testing.T) {
		var buf bytes.Buffer
		err := handleShow(&buf, cfg, "remoto")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Detalhes da conexão 'SOFTRH_GUI_REMOTO':") {
			t.Errorf("Expected details for SOFTRH_GUI_REMOTO, got:\n%s", out)
		}
	})

	t.Run("Show with approximate name matching multiple connections shows details for all", func(t *testing.T) {
		var buf bytes.Buffer
		err := handleShow(&buf, cfg, "softrh")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Foram encontradas 2 conexões correspondentes a 'softrh'") ||
			!strings.Contains(out, "Detalhes da conexão 'SOFTRH_GUI_REMOTO':") ||
			!strings.Contains(out, "Detalhes da conexão 'SOFTRH_LOCAL':") {
			t.Errorf("Expected details for both matching connections, got:\n%s", out)
		}
	})

	t.Run("Show with typo matches via fuzzy search", func(t *testing.T) {
		var buf bytes.Buffer
		err := handleShow(&buf, cfg, "sankya")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Detalhes da conexão 'sankhya_prod':") {
			t.Errorf("Expected details for sankhya_prod, got:\n%s", out)
		}
	})

	t.Run("Show with empty alias returns error", func(t *testing.T) {
		var buf bytes.Buffer
		err := handleShow(&buf, cfg, "")
		if err == nil {
			t.Fatal("Expected error for empty alias, got nil")
		}
	})

	t.Run("Show with non-existent alias returns error", func(t *testing.T) {
		var buf bytes.Buffer
		err := handleShow(&buf, cfg, "desconhecido_123")
		if err == nil {
			t.Fatal("Expected error for non-existent alias, got nil")
		}
	})
}
