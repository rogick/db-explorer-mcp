package tempfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDetectLobExtension(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "XML simples",
			data:     []byte(`<?xml version="1.0"?><NFe><infNFe Id="123"></infNFe></NFe>`),
			expected: ".xml",
		},
		{
			name:     "XML com espaços iniciais",
			data:     []byte("   \n\t <evento><id>10</id></evento>"),
			expected: ".xml",
		},
		{
			name:     "JSON objeto",
			data:     []byte(`{"id": 1023, "evento": "NFE"}`),
			expected: ".json",
		},
		{
			name:     "JSON array",
			data:     []byte(`[{"id": 1}, {"id": 2}]`),
			expected: ".json",
		},
		{
			name:     "Binário com byte nulo",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x0A, 0x1A, 0x0A},
			expected: ".bin",
		},
		{
			name:     "Binário com UTF-8 inválido",
			data:     []byte{0xff, 0xfe, 0xfd, 0x80, 0x81},
			expected: ".bin",
		},
		{
			name:     "Texto comum",
			data:     []byte("Log de execução do processamento em lote - linha 1\nlinha 2"),
			expected: ".txt",
		},
		{
			name:     "Vazio",
			data:     []byte{},
			expected: ".txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectLobExtension(tt.data)
			if got != tt.expected {
				t.Errorf("DetectLobExtension() = %q; esperado %q", got, tt.expected)
			}
		})
	}
}

func TestCreateQueryResultFile(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManagerWithDir(tmpDir)

	content := `[{"id": 1, "nome": "Teste"}]`
	info, err := mgr.CreateQueryResultFile("crm_oracle", "json", content)
	if err != nil {
		t.Fatalf("Erro inesperado ao criar arquivo de resultado: %v", err)
	}

	if info.Extension != ".json" {
		t.Errorf("Extensão esperada .json, obteve %s", info.Extension)
	}
	if info.SizeBytes != int64(len(content)) {
		t.Errorf("Tamanho esperado %d, obteve %d", len(content), info.SizeBytes)
	}
	if !strings.HasPrefix(filepath.Base(info.FilePath), "query_crm_oracle_") {
		t.Errorf("Nome do arquivo inesperado: %s", filepath.Base(info.FilePath))
	}

	readBack, err := os.ReadFile(info.FilePath)
	if err != nil {
		t.Fatalf("Erro ao ler arquivo criado: %v", err)
	}
	if string(readBack) != content {
		t.Errorf("Conteúdo lido difere do esperado: %q != %q", string(readBack), content)
	}
}

func TestCreateQueryResultFileCustomPath(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManagerWithDir(tmpDir)

	customFile := filepath.Join(tmpDir, "sub", "meu_resultado.csv")
	content := "id,nome\n1,Item\n"
	info, err := mgr.CreateQueryResultFile("meu_db", "csv", content, customFile)
	if err != nil {
		t.Fatalf("Erro inesperado ao criar arquivo com customPath: %v", err)
	}

	if info.FilePath != customFile {
		t.Errorf("FilePath esperado %s, obteve %s", customFile, info.FilePath)
	}

	readBack, err := os.ReadFile(customFile)
	if err != nil {
		t.Fatalf("Erro ao ler arquivo customizado: %v", err)
	}
	if string(readBack) != content {
		t.Errorf("Conteúdo difere do esperado")
	}
}

func TestCreateLobFile(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManagerWithDir(tmpDir)

	xmlPayload := []byte(`<NFe><infNFe><ide><cUF>35</cUF></ide></infNFe></NFe>`)
	info, err := mgr.CreateLobFile("payload_xml", 1, xmlPayload)
	if err != nil {
		t.Fatalf("Erro ao criar arquivo LOB: %v", err)
	}

	if info.Extension != ".xml" {
		t.Errorf("Extensão esperada .xml, obteve %s", info.Extension)
	}
	if !strings.HasPrefix(filepath.Base(info.FilePath), "lob_payload_xml_row1_") {
		t.Errorf("Nome do arquivo LOB inesperado: %s", filepath.Base(info.FilePath))
	}

	readBack, err := os.ReadFile(info.FilePath)
	if err != nil {
		t.Fatalf("Erro ao ler arquivo LOB: %v", err)
	}
	if string(readBack) != string(xmlPayload) {
		t.Errorf("Conteúdo do LOB difere do esperado")
	}
}

func TestCleanupOldFiles(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewManagerWithDir(tmpDir)

	oldFile, err := mgr.CreateLobFile("antigo", 1, []byte("conteudo antigo"))
	if err != nil {
		t.Fatalf("Erro ao criar arquivo antigo: %v", err)
	}
	// Retrocede o mtime em 48 horas
	pastTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldFile.FilePath, pastTime, pastTime); err != nil {
		t.Fatalf("Erro ao alterar mtime: %v", err)
	}

	newFile, err := mgr.CreateLobFile("novo", 2, []byte("conteudo recente"))
	if err != nil {
		t.Fatalf("Erro ao criar arquivo novo: %v", err)
	}

	if err := mgr.CleanupOldFiles(24 * time.Hour); err != nil {
		t.Fatalf("Erro no CleanupOldFiles: %v", err)
	}

	if _, err := os.Stat(oldFile.FilePath); !os.IsNotExist(err) {
		t.Errorf("Arquivo antigo deveria ter sido removido")
	}
	if _, err := os.Stat(newFile.FilePath); err != nil {
		t.Errorf("Arquivo novo deveria continuar existindo: %v", err)
	}
}

