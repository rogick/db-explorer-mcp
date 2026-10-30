package tempfile

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const DefaultSubDir = "db-explorer-mcp"

// FileInfo armazena metadados sobre um arquivo temporário gerado.
type FileInfo struct {
	FilePath      string `json:"file_path"`
	SizeBytes     int64  `json:"size_bytes"`
	FormattedSize string `json:"formatted_size"`
	ShortSize     string `json:"short_size"`
	Extension     string `json:"extension"`
}

// TempFileManager gerencia a criação e limpeza de arquivos temporários para resultados de queries e LOBs.
type TempFileManager struct {
	baseDir string
}

// NewManager cria um TempFileManager apontando para o diretório temporário padrão do sistema.
func NewManager() *TempFileManager {
	return &TempFileManager{
		baseDir: filepath.Join(os.TempDir(), DefaultSubDir),
	}
}

// NewManagerWithDir cria um TempFileManager apontando para um diretório customizado (útil para testes).
func NewManagerWithDir(baseDir string) *TempFileManager {
	return &TempFileManager{
		baseDir: baseDir,
	}
}

// BaseDir retorna o diretório base configurado no gerenciador.
func (m *TempFileManager) BaseDir() string {
	return m.baseDir
}

func (m *TempFileManager) ensureBaseDir() error {
	return os.MkdirAll(m.baseDir, 0755)
}

var nonAlnumRegex = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func sanitizeIdentifier(s string) string {
	s = strings.TrimSpace(s)
	s = nonAlnumRegex.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if s == "" {
		return "item"
	}
	if len(s) > 40 {
		s = s[:40]
	}
	return strings.ToLower(s)
}

func randomSuffix() string {
	b := make([]byte, 3)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	}
	return hex.EncodeToString(b)
}

// ExtensionForFormat retorna a extensão de arquivo apropriada para o formato de saída da query.
func ExtensionForFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return ".json"
	case "csv":
		return ".csv"
	case "xml":
		return ".xml"
	case "md", "markdown", "llm":
		return ".md"
	case "toon":
		return ".toon"
	default:
		return ".txt"
	}
}

// DetectLobExtension inspeciona os bytes do LOB para inferir a extensão mais adequada (.xml, .json, .bin, .txt).
func DetectLobExtension(data []byte) string {
	if len(data) == 0 {
		return ".txt"
	}

	// Verifica se é binário: presença de byte nulo nos primeiros 8KB ou UTF-8 inválido
	sampleSize := len(data)
	if sampleSize > 8192 {
		sampleSize = 8192
	}
	sample := data[:sampleSize]

	for _, b := range sample {
		if b == 0 {
			return ".bin"
		}
	}

	if !utf8.Valid(sample) {
		// Cuidado com truncamento no meio de uma runa UTF-8 no limite de 8192 bytes:
		// Verifica se os primeiros bytes até sampleSize-4 são válidos
		checkLen := sampleSize
		if checkLen > 4 {
			checkLen -= 4
		}
		if !utf8.Valid(sample[:checkLen]) {
			return ".bin"
		}
	}

	trimmed := strings.TrimSpace(string(sample))
	if len(trimmed) == 0 {
		return ".txt"
	}

	if strings.HasPrefix(trimmed, "<") && strings.Contains(trimmed, ">") {
		return ".xml"
	}

	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return ".json"
	}

	return ".txt"
}

// FormatBytesShort formata um tamanho em bytes de forma enxuta (ex: "180 KB", "2.4 MB", "512 B").
func FormatBytesShort(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	kb := float64(bytes) / 1024.0
	if kb < 1024 {
		if kb >= 10 {
			return fmt.Sprintf("%.0f KB", kb)
		}
		return fmt.Sprintf("%.1f KB", kb)
	}
	mb := kb / 1024.0
	if mb < 1024 {
		return fmt.Sprintf("%.1f MB", mb)
	}
	gb := mb / 1024.0
	return fmt.Sprintf("%.2f GB", gb)
}

// FormatBytesDetailed formata o tamanho em bytes com valor legível e contagem exata (ex: "2.4 MB (2,516,582 bytes)").
func FormatBytesDetailed(bytes int64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B (%d bytes)", bytes, bytes)
	}
	short := FormatBytesShort(bytes)
	return fmt.Sprintf("%s (%s bytes)", short, formatIntWithCommas(bytes))
}

func formatIntWithCommas(n int64) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}
	var result []byte
	rem := len(str) % 3
	if rem > 0 {
		result = append(result, str[:rem]...)
		if len(str) > rem {
			result = append(result, ',')
		}
	}
	for i := rem; i < len(str); i += 3 {
		result = append(result, str[i:i+3]...)
		if i+3 < len(str) {
			result = append(result, ',')
		}
	}
	return string(result)
}

// CreateQueryResultFile grava o resultado completo de uma query em um arquivo temporário (ou no caminho customizado se informado).
func (m *TempFileManager) CreateQueryResultFile(dbAlias, format, content string, customPath ...string) (*FileInfo, error) {
	ext := ExtensionForFormat(format)
	var targetPath string

	if len(customPath) > 0 && strings.TrimSpace(customPath[0]) != "" {
		cp := strings.TrimSpace(customPath[0])
		info, err := os.Stat(cp)
		if err == nil && info.IsDir() {
			filename := fmt.Sprintf("query_%s_%s_%s%s",
				sanitizeIdentifier(dbAlias),
				time.Now().Format("20060102_150405"),
				randomSuffix(),
				ext,
			)
			targetPath = filepath.Join(cp, filename)
		} else {
			if err := os.MkdirAll(filepath.Dir(cp), 0755); err != nil {
				return nil, fmt.Errorf("erro ao criar diretório para '%s': %w", cp, err)
			}
			targetPath = cp
			if filepath.Ext(targetPath) != "" {
				ext = filepath.Ext(targetPath)
			}
		}
	} else {
		if err := m.ensureBaseDir(); err != nil {
			return nil, fmt.Errorf("erro ao criar diretório temporário '%s': %w", m.baseDir, err)
		}
		filename := fmt.Sprintf("query_%s_%s_%s%s",
			sanitizeIdentifier(dbAlias),
			time.Now().Format("20060102_150405"),
			randomSuffix(),
			ext,
		)
		targetPath = filepath.Join(m.baseDir, filename)
	}

	data := []byte(content)
	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return nil, fmt.Errorf("erro ao gravar arquivo '%s': %w", targetPath, err)
	}

	size := int64(len(data))
	return &FileInfo{
		FilePath:      targetPath,
		SizeBytes:     size,
		FormattedSize: FormatBytesDetailed(size),
		ShortSize:     FormatBytesShort(size),
		Extension:     ext,
	}, nil
}

// CreateLobFile grava o conteúdo de um campo LOB individual em arquivo temporário com extensão detectada automaticamente.
func (m *TempFileManager) CreateLobFile(columnName string, rowIdx int, data []byte) (*FileInfo, error) {
	if err := m.ensureBaseDir(); err != nil {
		return nil, fmt.Errorf("erro ao criar diretório temporário '%s': %w", m.baseDir, err)
	}

	ext := DetectLobExtension(data)
	filename := fmt.Sprintf("lob_%s_row%d_%s_%s%s",
		sanitizeIdentifier(columnName),
		rowIdx,
		time.Now().Format("20060102_150405"),
		randomSuffix(),
		ext,
	)
	targetPath := filepath.Join(m.baseDir, filename)

	if err := os.WriteFile(targetPath, data, 0644); err != nil {
		return nil, fmt.Errorf("erro ao gravar arquivo LOB '%s': %w", targetPath, err)
	}

	size := int64(len(data))
	return &FileInfo{
		FilePath:      targetPath,
		SizeBytes:     size,
		FormattedSize: FormatBytesDetailed(size),
		ShortSize:     FormatBytesShort(size),
		Extension:     ext,
	}, nil
}

// CleanupOldFiles remove arquivos no diretório temporário que sejam mais antigos que olderThan.
func (m *TempFileManager) CleanupOldFiles(olderThan time.Duration) error {
	entries, err := os.ReadDir(m.baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoff := time.Now().Add(-olderThan)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(m.baseDir, entry.Name()))
		}
	}
	return nil
}

