package db

import (
	"strings"

	"github.com/rogick/db-explorer-mcp/pkg/config"
)

type PagedResult struct {
	Rows          []map[string]interface{}
	Columns       []string
	ReturnedCount int
	TotalCount    int
	Offset        int
	Limit         int
	Truncated     bool
}

func PaginateResult(rows []map[string]interface{}, columns []string, limit, offset int) *PagedResult {
	totalCount := len(rows)
	if offset < 0 {
		offset = 0
	}
	if offset >= totalCount {
		return &PagedResult{
			Rows:          []map[string]interface{}{},
			Columns:       columns,
			ReturnedCount: 0,
			TotalCount:    totalCount,
			Offset:        offset,
			Limit:         limit,
			Truncated:     false,
		}
	}

	end := totalCount
	truncated := false
	if limit > 0 {
		if offset+limit < totalCount {
			end = offset + limit
			truncated = true
		}
	}

	pagedRows := rows[offset:end]
	return &PagedResult{
		Rows:          pagedRows,
		Columns:       columns,
		ReturnedCount: len(pagedRows),
		TotalCount:    totalCount,
		Offset:        offset,
		Limit:         limit,
		Truncated:     truncated,
	}
}

// ResolveOwner determina o owner e o nome limpo da tabela baseado nos parâmetros e tipo de conexão.
// Se table_name contiver 'schema.tabela', o schema explícito tem precedência.
// Se owner não for informado, usa o padrão acordado para o banco:
// - Oracle: User da conexão em maiúsculo
// - SQL Server: 'dbo'
// - PostgreSQL: 'public'
// - MySQL: Database da conexão
func ResolveOwner(connDetails config.ConnectionDetails, explicitOwner, tableName string) (string, string) {
	schemaFromTable, cleanTable := splitSchemaTable(tableName)
	if schemaFromTable != "" {
		return schemaFromTable, cleanTable
	}
	if strings.TrimSpace(explicitOwner) != "" {
		return strings.TrimSpace(explicitOwner), cleanTable
	}

	switch strings.ToLower(connDetails.Type) {
	case "oracle":
		if connDetails.User != "" {
			return strings.ToUpper(connDetails.User), cleanTable
		}
		return "", cleanTable
	case "sqlserver":
		return "dbo", cleanTable
	case "postgres":
		return "public", cleanTable
	case "mysql":
		return connDetails.Database, cleanTable
	default:
		return "", cleanTable
	}
}
