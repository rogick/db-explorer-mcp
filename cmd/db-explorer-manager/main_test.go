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
