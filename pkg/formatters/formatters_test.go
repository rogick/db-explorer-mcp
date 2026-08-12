package formatters

import (
	"strings"
	"testing"
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

func TestFormatOutputLLM(t *testing.T) {
	rows := []map[string]interface{}{
		{"id": 1, "name": "Alice"},
	}
	out, err := FormatOutput(rows, "llm", []string{"id", "name"})
	if err != nil {
		t.Fatalf("Erro inesperado: %v", err)
	}
	if !strings.Contains(out, "| id | name |") || !strings.Contains(out, "| 1 | Alice |") {
		t.Errorf("Saída LLM incorreta: %s", out)
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
