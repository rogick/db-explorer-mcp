package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/rogick/db-explorer-mcp/pkg/security"
)

type IndexResult struct {
	Owner           string `json:"owner"`
	TableName       string `json:"table_name"`
	IndexName       string `json:"index_name"`
	IsUnique        bool   `json:"is_unique"`
	IsPrimary       bool   `json:"is_primary"`
	IndexType       string `json:"index_type"`
	Columns         string `json:"columns"`
	IncludedColumns string `json:"included_columns"`
	Predicate       string `json:"predicate"`
	Status          string `json:"status"`
}

var IndexColumns = []string{
	"owner",
	"table_name",
	"index_name",
	"is_unique",
	"is_primary",
	"index_type",
	"columns",
	"included_columns",
	"predicate",
	"status",
}

func (idx IndexResult) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"owner":            idx.Owner,
		"table_name":       idx.TableName,
		"index_name":       idx.IndexName,
		"is_unique":        idx.IsUnique,
		"is_primary":       idx.IsPrimary,
		"index_type":       idx.IndexType,
		"columns":          idx.Columns,
		"included_columns": idx.IncludedColumns,
		"predicate":        idx.Predicate,
		"status":           idx.Status,
	}
}

type ListIndexesOptions struct {
	Owner     string
	TableName string
}

func (e *Executor) ListIndexes(db *sql.DB, dbType string, opts ListIndexesOptions) ([]IndexResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := security.ValidateIdentifier(opts.Owner); err != nil {
		return nil, err
	}
	if err := security.ValidateIdentifier(opts.TableName); err != nil {
		return nil, err
	}

	var results []IndexResult
	var err error

	switch strings.ToLower(dbType) {
	case "oracle":
		results, err = e.listIndexesOracle(ctx, db, opts)
	case "sqlserver":
		results, err = e.listIndexesSqlServer(ctx, db, opts)
	case "postgres":
		results, err = e.listIndexesPostgres(ctx, db, opts)
	case "mysql":
		results, err = e.listIndexesMysql(ctx, db, opts)
	default:
		return nil, fmt.Errorf("tipo de banco '%s' não suportado", dbType)
	}

	if err != nil {
		return nil, err
	}

	// Ordenação determinística: owner, table_name, index_name
	sort.Slice(results, func(i, j int) bool {
		if results[i].Owner != results[j].Owner {
			return results[i].Owner < results[j].Owner
		}
		if results[i].TableName != results[j].TableName {
			return results[i].TableName < results[j].TableName
		}
		return results[i].IndexName < results[j].IndexName
	})

	return results, nil
}

// --- ORACLE ---

func (e *Executor) listIndexesOracle(ctx context.Context, db *sql.DB, opts ListIndexesOptions) ([]IndexResult, error) {
	owner := strings.ToUpper(strings.TrimSpace(opts.Owner))
	table := strings.ToUpper(strings.TrimSpace(opts.TableName))

	var ownerParam interface{} = owner
	if owner == "" {
		ownerParam = nil
	}
	var tableParam interface{} = table
	if table == "" {
		tableParam = nil
	}

	// 1. Ler expressões funcionais de all_ind_expressions (LONG)
	exprMap := make(map[string]string)
	exprQuery := `
		SELECT index_owner, index_name, column_position, column_expression
		FROM all_ind_expressions
		WHERE (:1 IS NULL OR UPPER(index_owner) = :1)
		  AND (:2 IS NULL OR UPPER(table_name) = :2)`

	if exprRows, err := db.QueryContext(ctx, exprQuery, ownerParam, tableParam); err == nil {
		defer exprRows.Close()
		for exprRows.Next() {
			var o, idxName string
			var pos int
			var expr sql.NullString
			if err := exprRows.Scan(&o, &idxName, &pos, &expr); err == nil && expr.Valid {
				key := fmt.Sprintf("%s.%s.%d", o, idxName, pos)
				exprMap[key] = strings.TrimSpace(expr.String)
			}
		}
	}

	// 2. Ler índices e colunas
	query := `
		SELECT 
			i.owner,
			i.table_name,
			i.index_name,
			CASE WHEN i.uniqueness = 'UNIQUE' THEN 1 ELSE 0 END AS is_unique,
			CASE WHEN pk.constraint_type = 'P' THEN 1 ELSE 0 END AS is_primary,
			i.index_type,
			ic.column_name,
			ic.column_position,
			COALESCE(ic.descend, 'ASC') AS descend,
			CASE WHEN i.status = 'VALID' THEN 'ENABLED' ELSE 'DISABLED' END AS status
		FROM all_indexes i
		JOIN all_ind_columns ic 
			ON i.owner = ic.index_owner AND i.index_name = ic.index_name
		LEFT JOIN all_constraints pk 
			ON i.owner = pk.owner AND i.index_name = pk.index_name AND pk.constraint_type = 'P'
		WHERE (:1 IS NULL OR UPPER(i.owner) = :1)
		  AND (:2 IS NULL OR UPPER(i.table_name) = :2)
		ORDER BY i.owner, i.table_name, i.index_name, ic.column_position`

	rows, err := db.QueryContext(ctx, query, ownerParam, tableParam)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar índices no oracle: %w", err)
	}
	defer rows.Close()

	type indexGroup struct {
		owner     string
		tableName string
		indexName string
		isUnique  bool
		isPrimary bool
		indexType string
		status    string
		cols      []string
	}

	groupMap := make(map[string]*indexGroup)
	var groupOrder []string

	for rows.Next() {
		var o, t, idxName, idxType, colName, descend, status string
		var isUnique, isPrimary int
		var colPos int

		if err := rows.Scan(&o, &t, &idxName, &isUnique, &isPrimary, &idxType, &colName, &colPos, &descend, &status); err != nil {
			return nil, err
		}

		id := o + "." + idxName
		item, exists := groupMap[id]
		if !exists {
			item = &indexGroup{
				owner:     o,
				tableName: t,
				indexName: idxName,
				isUnique:  isUnique == 1,
				isPrimary: isPrimary == 1,
				indexType: idxType,
				status:    status,
			}
			groupMap[id] = item
			groupOrder = append(groupOrder, id)
		}

		// Se a coluna tiver expressão funcional, usa a expressão
		finalCol := colName
		exprKey := fmt.Sprintf("%s.%s.%d", o, idxName, colPos)
		if funcExpr, ok := exprMap[exprKey]; ok {
			finalCol = funcExpr
		}

		dir := strings.ToUpper(strings.TrimSpace(descend))
		if dir == "" || dir == "ASC" {
			dir = "ASC"
		} else {
			dir = "DESC"
		}

		item.cols = append(item.cols, fmt.Sprintf("%s %s", finalCol, dir))
	}

	var results []IndexResult
	for _, id := range groupOrder {
		item := groupMap[id]
		results = append(results, IndexResult{
			Owner:           item.owner,
			TableName:       item.tableName,
			IndexName:       item.indexName,
			IsUnique:        item.isUnique,
			IsPrimary:       item.isPrimary,
			IndexType:       item.indexType,
			Columns:         strings.Join(item.cols, ", "),
			IncludedColumns: "",
			Predicate:       "",
			Status:          item.status,
		})
	}

	return results, nil
}

// --- SQL SERVER ---

func (e *Executor) listIndexesSqlServer(ctx context.Context, db *sql.DB, opts ListIndexesOptions) ([]IndexResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	table := strings.TrimSpace(opts.TableName)

	query := `
		SELECT 
			s.name AS owner,
			t.name AS table_name,
			i.name AS index_name,
			i.is_unique,
			i.is_primary_key AS is_primary,
			i.type_desc AS index_type,
			c.name AS column_name,
			ic.is_descending_key,
			ic.is_included_column,
			COALESCE(i.filter_definition, '') AS predicate,
			CASE WHEN i.is_disabled = 1 THEN 'DISABLED' ELSE 'ENABLED' END AS status
		FROM sys.indexes i
		JOIN sys.tables t ON i.object_id = t.object_id
		JOIN sys.schemas s ON t.schema_id = s.schema_id
		JOIN sys.index_columns ic ON i.object_id = ic.object_id AND i.index_id = ic.index_id
		JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
		WHERE (@p1 = '' OR LOWER(s.name) = LOWER(@p1))
		  AND (@p2 = '' OR LOWER(t.name) = LOWER(@p2))
		  AND i.name IS NOT NULL
		ORDER BY s.name, t.name, i.name, ic.is_included_column, ic.key_ordinal, ic.index_column_id`

	rows, err := db.QueryContext(ctx, query, owner, table)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar índices no sql server: %w", err)
	}
	defer rows.Close()

	type indexGroup struct {
		owner     string
		tableName string
		indexName string
		isUnique  bool
		isPrimary bool
		indexType string
		predicate string
		status    string
		keyCols   []string
		incCols   []string
	}

	groupMap := make(map[string]*indexGroup)
	var groupOrder []string

	for rows.Next() {
		var o, t, idxName, idxType, colName, pred, status string
		var isUnique, isPrimary, isDesc, isInc bool

		if err := rows.Scan(&o, &t, &idxName, &isUnique, &isPrimary, &idxType, &colName, &isDesc, &isInc, &pred, &status); err != nil {
			return nil, err
		}

		id := o + "." + idxName
		item, exists := groupMap[id]
		if !exists {
			item = &indexGroup{
				owner:     o,
				tableName: t,
				indexName: idxName,
				isUnique:  isUnique,
				isPrimary: isPrimary,
				indexType: idxType,
				predicate: cleanCheckClause(pred),
				status:    status,
			}
			groupMap[id] = item
			groupOrder = append(groupOrder, id)
		}

		if isInc {
			item.incCols = append(item.incCols, colName)
		} else {
			dir := "ASC"
			if isDesc {
				dir = "DESC"
			}
			item.keyCols = append(item.keyCols, fmt.Sprintf("%s %s", colName, dir))
		}
	}

	var results []IndexResult
	for _, id := range groupOrder {
		item := groupMap[id]
		results = append(results, IndexResult{
			Owner:           item.owner,
			TableName:       item.tableName,
			IndexName:       item.indexName,
			IsUnique:        item.isUnique,
			IsPrimary:       item.isPrimary,
			IndexType:       item.indexType,
			Columns:         strings.Join(item.keyCols, ", "),
			IncludedColumns: strings.Join(item.incCols, ", "),
			Predicate:       item.predicate,
			Status:          item.status,
		})
	}

	return results, nil
}

// --- POSTGRESQL ---

func (e *Executor) listIndexesPostgres(ctx context.Context, db *sql.DB, opts ListIndexesOptions) ([]IndexResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	table := strings.TrimSpace(opts.TableName)

	query := `
		SELECT 
			n.nspname AS owner,
			tcl.relname AS table_name,
			icl.relname AS index_name,
			ix.indisunique AS is_unique,
			ix.indisprimary AS is_primary,
			am.amname AS index_type,
			ix.indnatts,
			ix.indnkeyatts,
			COALESCE(pg_get_expr(ix.indpred, ix.indrelid), '') AS predicate,
			CASE WHEN ix.indisvalid THEN 'ENABLED' ELSE 'DISABLED' END AS status,
			pg_get_indexdef(icl.oid) AS index_def
		FROM pg_index ix
		JOIN pg_class icl ON icl.oid = ix.indexrelid
		JOIN pg_class tcl ON tcl.oid = ix.indrelid
		JOIN pg_namespace n ON n.oid = tcl.relnamespace
		JOIN pg_am am ON am.oid = icl.relam
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND ($1 = '' OR LOWER(n.nspname) = LOWER($1))
		  AND ($2 = '' OR LOWER(tcl.relname) = LOWER($2))
		ORDER BY n.nspname, tcl.relname, icl.relname`

	rows, err := db.QueryContext(ctx, query, owner, table)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar índices no postgres: %w", err)
	}
	defer rows.Close()

	var results []IndexResult

	for rows.Next() {
		var o, t, idxName, idxType, pred, status, indexDef string
		var isUnique, isPrimary bool
		var natts, nkeyatts int

		if err := rows.Scan(&o, &t, &idxName, &isUnique, &isPrimary, &idxType, &natts, &nkeyatts, &pred, &status, &indexDef); err != nil {
			return nil, err
		}

		keyCols, incCols := parsePostgresIndexDef(indexDef, nkeyatts, natts)

		results = append(results, IndexResult{
			Owner:           o,
			TableName:       t,
			IndexName:       idxName,
			IsUnique:        isUnique,
			IsPrimary:       isPrimary,
			IndexType:       strings.ToUpper(idxType),
			Columns:         keyCols,
			IncludedColumns: incCols,
			Predicate:       cleanCheckClause(pred),
			Status:          status,
		})
	}

	return results, nil
}

// parsePostgresIndexDef extrai as colunas ordenadas (com ASC/DESC) e colunas INCLUDE de pg_get_indexdef
func parsePostgresIndexDef(def string, nkeyatts, natts int) (keyColsStr string, incColsStr string) {
	// Exemplo de def:
	// CREATE UNIQUE INDEX idx ON public.tbl USING btree (col1, col2 DESC) INCLUDE (col3, col4) WHERE (status = 'A')
	// Extrair bloco de colunas principais entre USING <am> ( ... )
	idxUsing := strings.Index(strings.ToUpper(def), " USING ")
	if idxUsing == -1 {
		return "", ""
	}

	rest := def[idxUsing+7:]
	idxOpenParen := strings.Index(rest, "(")
	if idxOpenParen == -1 {
		return "", ""
	}

	colsBlock, remainder := extractParenthesizedBlock(rest[idxOpenParen:])
	keyCols := splitAndNormalizeIndexColumns(colsBlock)

	// Extrair INCLUDE (...) se natts > nkeyatts ou se existir na string
	var incCols []string
	idxInclude := strings.Index(strings.ToUpper(remainder), "INCLUDE")
	if idxInclude != -1 {
		incBlock, _ := extractParenthesizedBlock(remainder[idxInclude:])
		rawInc := splitAndTrim(incBlock, ",")
		for _, c := range rawInc {
			incCols = append(incCols, strings.Trim(strings.TrimSpace(c), `"`))
		}
	}

	return strings.Join(keyCols, ", "), strings.Join(incCols, ", ")
}

func extractParenthesizedBlock(s string) (block string, remainder string) {
	start := strings.Index(s, "(")
	if start == -1 {
		return "", s
	}
	depth := 0
	for i := start; i < len(s); i++ {
		if s[i] == '(' {
			depth++
		} else if s[i] == ')' {
			depth--
			if depth == 0 {
				return s[start+1 : i], s[i+1:]
			}
		}
	}
	return s[start+1:], ""
}

func splitAndNormalizeIndexColumns(block string) []string {
	// Divide por vírgula respeitando parênteses de expressões
	var cols []string
	depth := 0
	start := 0
	for i := 0; i < len(block); i++ {
		if block[i] == '(' {
			depth++
		} else if block[i] == ')' {
			depth--
		} else if block[i] == ',' && depth == 0 {
			cols = append(cols, strings.TrimSpace(block[start:i]))
			start = i + 1
		}
	}
	if start < len(block) {
		cols = append(cols, strings.TrimSpace(block[start:]))
	}

	var res []string
	for _, c := range cols {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		upper := strings.ToUpper(c)
		if strings.HasSuffix(upper, " DESC") {
			base := strings.TrimSpace(c[:len(c)-5])
			base = strings.Trim(base, `"`)
			res = append(res, base+" DESC")
		} else if strings.HasSuffix(upper, " ASC") {
			base := strings.TrimSpace(c[:len(c)-4])
			base = strings.Trim(base, `"`)
			res = append(res, base+" ASC")
		} else {
			base := strings.Trim(c, `"`)
			res = append(res, base+" ASC")
		}
	}
	return res
}

// --- MYSQL ---

func (e *Executor) listIndexesMysql(ctx context.Context, db *sql.DB, opts ListIndexesOptions) ([]IndexResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	table := strings.TrimSpace(opts.TableName)

	query := `
		SELECT 
			TABLE_SCHEMA AS owner,
			TABLE_NAME AS table_name,
			INDEX_NAME AS index_name,
			CASE WHEN NON_UNIQUE = 0 THEN 1 ELSE 0 END AS is_unique,
			CASE WHEN INDEX_NAME = 'PRIMARY' THEN 1 ELSE 0 END AS is_primary,
			INDEX_TYPE AS index_type,
			COLUMN_NAME AS column_name,
			COALESCE(COLLATION, 'A') AS collation
		FROM INFORMATION_SCHEMA.STATISTICS
		WHERE TABLE_SCHEMA = ?
		  AND (? = '' OR LOWER(TABLE_NAME) = LOWER(?))
		ORDER BY TABLE_SCHEMA, TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`

	rows, err := db.QueryContext(ctx, query, owner, table, table)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar índices no mysql: %w", err)
	}
	defer rows.Close()

	type indexGroup struct {
		owner     string
		tableName string
		indexName string
		isUnique  bool
		isPrimary bool
		indexType string
		cols      []string
	}

	groupMap := make(map[string]*indexGroup)
	var groupOrder []string

	for rows.Next() {
		var o, t, idxName, idxType, colName, collation string
		var isUnique, isPrimary int

		if err := rows.Scan(&o, &t, &idxName, &isUnique, &isPrimary, &idxType, &colName, &collation); err != nil {
			return nil, err
		}

		id := o + "." + idxName
		item, exists := groupMap[id]
		if !exists {
			item = &indexGroup{
				owner:     o,
				tableName: t,
				indexName: idxName,
				isUnique:  isUnique == 1,
				isPrimary: isPrimary == 1,
				indexType: idxType,
			}
			groupMap[id] = item
			groupOrder = append(groupOrder, id)
		}

		dir := "ASC"
		if strings.ToUpper(collation) == "D" {
			dir = "DESC"
		}
		item.cols = append(item.cols, fmt.Sprintf("%s %s", colName, dir))
	}

	var results []IndexResult
	for _, id := range groupOrder {
		item := groupMap[id]
		results = append(results, IndexResult{
			Owner:           item.owner,
			TableName:       item.tableName,
			IndexName:       item.indexName,
			IsUnique:        item.isUnique,
			IsPrimary:       item.isPrimary,
			IndexType:       item.indexType,
			Columns:         strings.Join(item.cols, ", "),
			IncludedColumns: "",
			Predicate:       "",
			Status:          "ENABLED",
		})
	}

	return results, nil
}
