package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/microsoft/go-mssqldb"
	go_ora "github.com/sijms/go-ora/v2"

	"github.com/rogick/db-explorer-mcp/pkg/config"
	"github.com/rogick/db-explorer-mcp/pkg/tempfile"
	"unicode/utf8"
)

type Executor struct{}

func NewExecutor() *Executor {
	return &Executor{}
}

func (e *Executor) OpenConnection(details config.ConnectionDetails) (*sql.DB, string, error) {
	dbType := strings.ToLower(details.Type)
	var driverName string
	var dsn string

	switch dbType {
	case "oracle":
		driverName = "oracle"
		user := url.QueryEscape(details.User)
		pass := url.QueryEscape(details.Password)

		if details.DSN != "" {
			// Se o DSN já contiver host:porta/sid, montamos a URL do go-ora
			if strings.HasPrefix(details.DSN, "oracle://") {
				dsn = details.DSN
			} else {
				dsn = fmt.Sprintf("oracle://%s:%s@%s", user, pass, details.DSN)
			}
		} else {
			host := details.Host
			if host == "" {
				host = "localhost"
			}
			port := details.Port
			if port == 0 {
				port = 1521
			}
			sid := details.Database
			if sid == "" {
				sid = "XE"
			}
			dsn = fmt.Sprintf("oracle://%s:%s@%s:%d/%s", user, pass, host, port, sid)
		}

	case "sqlserver":
		driverName = "sqlserver"
		host := details.Host
		if host == "" && details.Server != "" {
			parts := strings.Split(details.Server, ":")
			host = parts[0]
			if len(parts) > 1 {
				p, _ := strconv.Atoi(parts[1])
				details.Port = p
			}
		}
		if host == "" {
			host = "localhost"
		}
		port := details.Port
		if port == 0 {
			port = 1433
		}

		user := url.QueryEscape(details.User)
		pass := url.QueryEscape(details.Password)

		if details.Instance != "" {
			dsn = fmt.Sprintf("sqlserver://%s:%s@%s:%d/%s?database=%s&encrypt=true&TrustServerCertificate=true",
				user, pass, host, port, details.Instance, details.Database)
		} else {
			dsn = fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s&encrypt=true&TrustServerCertificate=true",
				user, pass, host, port, details.Database)
		}

	case "postgres":
		driverName = "pgx"
		host := details.Host
		if host == "" {
			host = "localhost"
		}
		port := details.Port
		if port == 0 {
			port = 5432
		}
		database := details.Database
		if database == "" {
			database = "postgres"
		}
		user := url.QueryEscape(details.User)
		pass := url.QueryEscape(details.Password)

		dsn = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", user, pass, host, port, database)

	case "mysql":
		driverName = "mysql"
		host := details.Host
		if host == "" {
			host = "localhost"
		}
		port := details.Port
		if port == 0 {
			port = 3306
		}
		database := details.Database
		if database == "" {
			database = "mysql"
		}

		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", details.User, details.Password, host, port, database)

	default:
		return nil, "", fmt.Errorf("tipo de banco '%s' não suportado", dbType)
	}

	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, dbType, fmt.Errorf("falha ao abrir driver %s: %w", dbType, err)
	}

	if dbType == "oracle" {
		nlsParams := []struct{ key, value string }{
			{"NLS_DATE_FORMAT", "'DD/MM/YYYY HH24:MI:SS'"},
			{"NLS_TIMESTAMP_FORMAT", "'YYYY-MM-DD HH24:MI:SS.FF'"},
			{"NLS_NUMERIC_CHARACTERS", "'.,'"},
			{"NLS_LANGUAGE", "'BRAZILIAN PORTUGUESE'"},
		}
		for _, p := range nlsParams {
			if err := go_ora.AddSessionParam(db, p.key, p.value); err != nil {
				db.Close()
				return nil, dbType, fmt.Errorf("falha ao definir parâmetro NLS %s: %w", p.key, err)
			}
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, dbType, fmt.Errorf("falha ao conectar ao banco (%s): %w", dbType, err)
	}

	return db, dbType, nil
}

func (e *Executor) ListTables(db *sql.DB, dbType string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var query string
	switch dbType {
	case "oracle":
		query = "SELECT table_name FROM all_tables FETCH FIRST 500 ROWS ONLY"
	case "sqlserver":
		query = "SELECT table_name FROM information_schema.tables WHERE table_type = 'BASE TABLE'"
	case "postgres":
		query = "SELECT tablename FROM pg_catalog.pg_tables WHERE schemaname != 'pg_catalog' AND schemaname != 'information_schema'"
	case "mysql":
		query = "SHOW FULL TABLES WHERE Table_type = 'BASE TABLE'"
	default:
		return nil, fmt.Errorf("tipo de banco '%s' não suportado", dbType)
	}

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar tabelas: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		var dummyType string
		if dbType == "mysql" {
			if err := rows.Scan(&tableName, &dummyType); err != nil {
				return nil, err
			}
		} else {
			if err := rows.Scan(&tableName); err != nil {
				return nil, err
			}
		}
		tables = append(tables, tableName)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tables, nil
}

func (e *Executor) GetTableSchema(db *sql.DB, dbType string, tableName string, detailLevel ...string) (*TableSchema, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	level := "detailed"
	if len(detailLevel) > 0 {
		level = detailLevel[0]
	}
	level = NormalizeDetailLevel(level)

	switch strings.ToLower(dbType) {
	case "oracle":
		return e.getTableSchemaOracle(ctx, db, tableName, level)
	case "sqlserver":
		return e.getTableSchemaSqlServer(ctx, db, tableName, level)
	case "postgres":
		return e.getTableSchemaPostgres(ctx, db, tableName, level)
	case "mysql":
		return e.getTableSchemaMysql(ctx, db, tableName, level)
	default:
		return nil, fmt.Errorf("tipo de banco '%s' não suportado", dbType)
	}
}

const DefaultLOBThresholdBytes = 2048

type ExecuteQueryOptions struct {
	Limit        int
	Offset       int
	ExtractLOBs  bool
	LOBThreshold int
	TempManager  *tempfile.TempFileManager
}

type QueryResult struct {
	Rows            []map[string]interface{}
	Columns         []string
	ReturnedCount   int
	TotalCount      int
	Offset          int
	Truncated       bool
	ExceededCeiling bool
	LobFiles        []string
}

// IsLobColumnType verifica se o tipo de dados informado pelo driver SQL corresponde a um tipo LOB ou texto/binário longo.
func IsLobColumnType(dbTypeName string) bool {
	upper := strings.ToUpper(strings.TrimSpace(dbTypeName))
	switch upper {
	case "CLOB", "BLOB", "NCLOB", "LONG", "LONG RAW", "BFILE", "XMLTYPE",
		"TEXT", "BYTEA", "XML", "JSON", "JSONB",
		"VARCHAR(MAX)", "NVARCHAR(MAX)", "VARBINARY(MAX)", "NTEXT", "IMAGE",
		"LONGTEXT", "MEDIUMTEXT", "LONGBLOB", "MEDIUMBLOB":
		return true
	}
	return false
}

// IsDedicatedLobColumnType identifica tipos exclusivos de LOB/binário que sempre devem ser extraídos quando não vazios.
func IsDedicatedLobColumnType(dbTypeName string) bool {
	upper := strings.ToUpper(strings.TrimSpace(dbTypeName))
	switch upper {
	case "CLOB", "BLOB", "NCLOB", "LONG", "LONG RAW", "BFILE", "XMLTYPE",
		"BYTEA", "XML",
		"VARBINARY(MAX)", "NTEXT", "IMAGE",
		"LONGTEXT", "MEDIUMTEXT", "LONGBLOB", "MEDIUMBLOB":
		return true
	}
	return false
}

func isBinaryBytes(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	sampleSize := len(data)
	if sampleSize > 8192 {
		sampleSize = 8192
	}
	sample := data[:sampleSize]
	for _, b := range sample {
		if b == 0 {
			return true
		}
	}
	checkLen := sampleSize
	if checkLen > 4 {
		checkLen -= 4
	}
	return !utf8.Valid(sample[:checkLen])
}

func (e *Executor) ExecuteQuery(db *sql.DB, query string, limit int, offset int) (*QueryResult, error) {
	return e.ExecuteQueryWithOptions(db, query, ExecuteQueryOptions{
		Limit:  limit,
		Offset: offset,
	})
}

func (e *Executor) ExecuteQueryWithOptions(db *sql.DB, query string, opts ExecuteQueryOptions) (*QueryResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		// Tenta executar como comando sem retorno (INSERT, UPDATE, DELETE, DDL)
		res, execErr := db.ExecContext(ctx, query)
		if execErr != nil {
			return nil, fmt.Errorf("erro ao executar query: %w", execErr)
		}

		rowsAffected, _ := res.RowsAffected()
		cols := []string{"status", "rowsAffected"}
		record := map[string]interface{}{
			"status":       "success",
			"rowsAffected": rowsAffected,
		}
		return &QueryResult{
			Rows:          []map[string]interface{}{record},
			Columns:       cols,
			ReturnedCount: 1,
			TotalCount:    1,
			Offset:        0,
			Truncated:     false,
		}, nil
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	colTypeNames := make([]string, len(cols))
	if colTypes, err := rows.ColumnTypes(); err == nil && len(colTypes) == len(cols) {
		for i, ct := range colTypes {
			if ct != nil {
				colTypeNames[i] = ct.DatabaseTypeName()
			}
		}
	}

	lobThreshold := opts.LOBThreshold
	if lobThreshold <= 0 {
		lobThreshold = DefaultLOBThresholdBytes
	}

	var tempMgr *tempfile.TempFileManager
	if opts.ExtractLOBs {
		if opts.TempManager != nil {
			tempMgr = opts.TempManager
		} else {
			tempMgr = tempfile.NewManager()
		}
	}

	var results []map[string]interface{}
	var lobFiles []string
	count := 0
	totalCount := 0
	truncated := false
	exceededCeiling := false

	offset := opts.Offset
	if offset < 0 {
		offset = 0
	}

	maxFetchLimit := opts.Limit
	if maxFetchLimit <= 0 {
		maxFetchLimit = 50000 // teto de segurança para consultas sem limite
	}
	maxCountCeiling := 10000 // teto para continuar contando linhas adicionais
	var countStartTime time.Time

	for rows.Next() {
		totalCount++

		// Pula registros anteriores ao offset
		if totalCount <= offset {
			continue
		}

		if count < maxFetchLimit {
			count++
			scanArgs := make([]interface{}, len(cols))
			values := make([]interface{}, len(cols))
			for i := range values {
				scanArgs[i] = &values[i]
			}

			if err := rows.Scan(scanArgs...); err != nil {
				return nil, fmt.Errorf("erro ao escanear linha: %w", err)
			}

			rowMap := make(map[string]interface{}, len(cols))
			for i, col := range cols {
				val := values[i]
				if opts.ExtractLOBs && val != nil {
					var rawBytes []byte
					var isByteSlice bool
					var isCandidate bool

					switch v := val.(type) {
					case []byte:
						rawBytes = v
						isByteSlice = true
						isCandidate = true
					case string:
						rawBytes = []byte(v)
						isCandidate = true
					}

					if isCandidate && len(rawBytes) > 0 {
						shouldExtract := len(rawBytes) >= lobThreshold ||
							IsDedicatedLobColumnType(colTypeNames[i]) ||
							(isByteSlice && isBinaryBytes(rawBytes))

						if shouldExtract {
							info, lobErr := tempMgr.CreateLobFile(col, count, rawBytes)
							if lobErr == nil {
								rowMap[col] = fmt.Sprintf("[LOB salvo em: %s (%s)]", info.FilePath, info.ShortSize)
								lobFiles = append(lobFiles, info.FilePath)
								continue
							}
						}
					}
				}

				if b, ok := val.([]byte); ok {
					rowMap[col] = string(b)
				} else {
					rowMap[col] = val
				}
			}
			results = append(results, rowMap)
		} else {
			truncated = true
			if countStartTime.IsZero() {
				countStartTime = time.Now()
			}
			if totalCount >= maxCountCeiling || time.Since(countStartTime) > 2*time.Second {
				exceededCeiling = true
				break
			}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erro durante iteração das linhas: %w", err)
	}

	return &QueryResult{
		Rows:            results,
		Columns:         cols,
		ReturnedCount:   count,
		TotalCount:      totalCount,
		Offset:          offset,
		Truncated:       truncated,
		ExceededCeiling: exceededCeiling,
		LobFiles:        lobFiles,
	}, nil
}
