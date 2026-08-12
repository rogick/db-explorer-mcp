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
	_ "github.com/sijms/go-ora/v2"

	"github.com/rogick/db-explorer-mcp/pkg/config"
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

func (e *Executor) GetTableSchema(db *sql.DB, dbType string, tableName string) ([]map[string]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var query string
	var args []interface{}

	switch dbType {
	case "oracle":
		query = "SELECT column_name, data_type FROM all_tab_columns WHERE table_name = :1"
		args = append(args, strings.ToUpper(tableName))
	case "sqlserver":
		query = "SELECT column_name, data_type FROM information_schema.columns WHERE table_name = @p1"
		args = append(args, tableName)
	case "postgres":
		query = "SELECT column_name, data_type FROM information_schema.columns WHERE table_name = $1"
		args = append(args, tableName)
	case "mysql":
		query = "SELECT column_name, data_type FROM information_schema.columns WHERE table_name = ? AND table_schema = DATABASE()"
		args = append(args, tableName)
	default:
		return nil, fmt.Errorf("tipo de banco '%s' não suportado", dbType)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter schema da tabela: %w", err)
	}
	defer rows.Close()

	var schema []map[string]string
	for rows.Next() {
		var col, typ string
		if err := rows.Scan(&col, &typ); err != nil {
			return nil, err
		}
		schema = append(schema, map[string]string{
			"column": col,
			"type":   typ,
		})
	}

	return schema, nil
}

func (e *Executor) ExecuteQuery(db *sql.DB, query string) ([]map[string]interface{}, []string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		// Tenta executar como comando sem retorno (INSERT, UPDATE, DELETE, DDL)
		res, execErr := db.ExecContext(ctx, query)
		if execErr != nil {
			return nil, nil, fmt.Errorf("erro ao executar query: %w", err)
		}

		rowsAffected, _ := res.RowsAffected()
		cols := []string{"status", "rowsAffected"}
		record := map[string]interface{}{
			"status":       "success",
			"rowsAffected": rowsAffected,
		}
		return []map[string]interface{}{record}, cols, nil
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	var results []map[string]interface{}
	count := 0

	for rows.Next() && count < 100 {
		count++
		scanArgs := make([]interface{}, len(cols))
		values := make([]interface{}, len(cols))
		for i := range values {
			scanArgs[i] = &values[i]
		}

		if err := rows.Scan(scanArgs...); err != nil {
			return nil, nil, fmt.Errorf("erro ao escanear linha: %w", err)
		}

		rowMap := make(map[string]interface{}, len(cols))
		for i, col := range cols {
			val := values[i]
			if b, ok := val.([]byte); ok {
				rowMap[col] = string(b)
			} else {
				rowMap[col] = val
			}
		}
		results = append(results, rowMap)
	}

	return results, cols, nil
}
