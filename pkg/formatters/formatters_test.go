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
