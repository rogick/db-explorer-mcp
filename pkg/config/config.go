package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ConnectionDetails struct {
	Type     string `json:"type"`               // oracle, sqlserver, postgres, mysql
	Mode     string `json:"mode"`               // readonly, normal, teste
	User     string `json:"user"`
	Password string `json:"password"`
	DSN      string `json:"dsn,omitempty"`      // Para Oracle
	Host     string `json:"host,omitempty"`     // Para SQL Server, Postgres, MySQL
	Server   string `json:"server,omitempty"`   // Fallback legado para SQL Server
	Port     int    `json:"port,omitempty"`
	Database string `json:"database,omitempty"`
	Instance string `json:"instance,omitempty"` // Opcional para SQL Server
}

type Config struct {
	Connections map[string]ConnectionDetails `json:"connections"`
}

func GetConfigPath() (string, error) {
	if envPath := os.Getenv("DB_EXPLORER_CONFIG_PATH"); envPath != "" {
		return filepath.Abs(envPath)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("não foi possível determinar o diretório home do usuário: %w", err)
	}
	return filepath.Join(homeDir, ".db-explorer-config.json"), nil
}

func LoadConfig() (*Config, error) {
	configPath, err := GetConfigPath()
	if err != nil {
		return &Config{Connections: make(map[string]ConnectionDetails)}, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{Connections: make(map[string]ConnectionDetails)}, nil
		}
		return nil, fmt.Errorf("erro ao ler arquivo de configuração: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("erro ao decodificar JSON do arquivo de configuração: %w", err)
	}

	if cfg.Connections == nil {
		cfg.Connections = make(map[string]ConnectionDetails)
	}

	return &cfg, nil
}

func SaveConfig(cfg *Config) error {
	configPath, err := GetConfigPath()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil {
		return fmt.Errorf("erro ao serializar configuração: %w", err)
	}

	if err := os.WriteFile(configPath, data, 0600); err != nil {
		return fmt.Errorf("erro ao salvar arquivo de configuração: %w", err)
	}

	return nil
}

func IsLocalHost(addressStr string) bool {
	if addressStr == "" {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(addressStr))
	return strings.Contains(lower, "localhost") ||
		strings.Contains(lower, "127.0.0.1") ||
		strings.Contains(lower, "::1") ||
		lower == "." ||
		strings.HasPrefix(lower, "(local)") ||
		strings.HasPrefix(lower, ".\\")
}

func ValidateModeHost(mode, addressStr string) error {
	if (mode == "normal" || mode == "teste") && !IsLocalHost(addressStr) {
		return fmt.Errorf("Segurança: O modo '%s' só é permitido para servidores locais (localhost ou 127.0.0.1). Para bases remotas, utilize o modo 'readonly'.", mode)
	}
	return nil
}
