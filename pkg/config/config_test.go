package config

import (
	"testing"
)

func TestIsLocalHost(t *testing.T) {
	tests := []struct {
		address  string
		expected bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"127.0.0.1:1521/XE", true},
		{"localhost:1433", true},
		{"::1", true},
		{".", true},
		{"(local)", true},
		{".\\SQLEXPRESS", true},
		{"192.168.1.100", false},
		{"db.company.com", false},
		{"oracle-prod.internal:1521/PROD", false},
	}

	for _, tt := range tests {
		result := IsLocalHost(tt.address)
		if result != tt.expected {
			t.Errorf("IsLocalHost(%q) = %v; esperado %v", tt.address, result, tt.expected)
		}
	}
}

func TestValidateModeHost(t *testing.T) {
	err := ValidateModeHost("normal", "localhost")
	if err != nil {
		t.Errorf("Esperado nil para normal em localhost, obteve: %v", err)
	}

	err = ValidateModeHost("normal", "192.168.1.10")
	if err == nil {
		t.Errorf("Esperado erro para normal em host remoto, obteve nil")
	}

	err = ValidateModeHost("readonly", "192.168.1.10")
	if err != nil {
		t.Errorf("Esperado nil para readonly em host remoto, obteve: %v", err)
	}
}

func TestGetConnectionCaseInsensitive(t *testing.T) {
	cfg := &Config{
		Connections: map[string]ConnectionDetails{
			"SOFTRH_GUI_REMOTO": {Type: "oracle", Mode: "readonly"},
			"modelosql-teste":   {Type: "sqlserver", Mode: "teste"},
		},
	}

	// 1. Busca exata
	conn, actualKey, found := cfg.GetConnection("SOFTRH_GUI_REMOTO")
	if !found || actualKey != "SOFTRH_GUI_REMOTO" || conn.Type != "oracle" {
		t.Errorf("Esperado encontrar 'SOFTRH_GUI_REMOTO', obteve found=%v, key=%s", found, actualKey)
	}

	// 2. Busca em minúsculas
	conn, actualKey, found = cfg.GetConnection("softrh_gui_remoto")
	if !found || actualKey != "SOFTRH_GUI_REMOTO" || conn.Type != "oracle" {
		t.Errorf("Esperado encontrar 'SOFTRH_GUI_REMOTO' buscando por minúsculas, obteve found=%v, key=%s", found, actualKey)
	}

	// 3. Busca em maiúsculas
	conn, actualKey, found = cfg.GetConnection("MODELOSQL-TESTE")
	if !found || actualKey != "modelosql-teste" || conn.Type != "sqlserver" {
		t.Errorf("Esperado encontrar 'modelosql-teste' buscando por maiúsculas, obteve found=%v, key=%s", found, actualKey)
	}

	// 4. Busca com case misto
	conn, actualKey, found = cfg.GetConnection("Modelosql-Teste")
	if !found || actualKey != "modelosql-teste" || conn.Type != "sqlserver" {
		t.Errorf("Esperado encontrar 'modelosql-teste' buscando por mixed case, obteve found=%v, key=%s", found, actualKey)
	}

	// 5. Inexistente
	_, _, found = cfg.GetConnection("nao_existe")
	if found {
		t.Errorf("Esperado não encontrar 'nao_existe', mas retornou found=true")
	}
}
