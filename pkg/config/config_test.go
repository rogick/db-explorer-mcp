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
