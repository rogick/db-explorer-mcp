package db

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

// TableSchema representa a estrutura detalhada de uma tabela.
type TableSchema struct {
	Table             string                 `json:"table"`
	Columns           []ColumnSchema         `json:"columns"`
	PrimaryKey        *PrimaryKeyInfo        `json:"primary_key,omitempty"`
	ForeignKeys       []ForeignKeyInfo       `json:"foreign_keys,omitempty"`
	UniqueConstraints []UniqueConstraintInfo `json:"unique_constraints,omitempty"`
	CheckConstraints  []CheckConstraintInfo  `json:"check_constraints,omitempty"`
}

// ColumnSchema representa as propriedades de uma coluna.
type ColumnSchema struct {
	Column       string   `json:"column"`                // Nome da coluna
	Type         string   `json:"type"`                  // Tipo de dado (ex: VARCHAR, INT, NUMBER)
	Nullable     bool     `json:"nullable"`              // Se a coluna aceita nulos
	Length       *int     `json:"length,omitempty"`      // Tamanho máximo em caracteres ou bytes
	Precision    *int     `json:"precision,omitempty"`   // Precisão numérica
	Scale        *int     `json:"scale,omitempty"`       // Escala numérica
	DefaultValue *string  `json:"default,omitempty"`     // Valor padrão / default
	PrimaryKey   bool     `json:"primary_key,omitempty"` // Se faz parte da chave primária
	Checks       []string `json:"checks,omitempty"`      // Checks/validações específicas deste campo
}

// PrimaryKeyInfo detalha a chave primária da tabela.
type PrimaryKeyInfo struct {
	Name    string   `json:"name,omitempty"`
	Columns []string `json:"columns"`
}

// ForeignKeyInfo detalha uma chave estrangeira e sua referência.
type ForeignKeyInfo struct {
	Name              string   `json:"name,omitempty"`
	Columns           []string `json:"columns"`
	ReferencedTable   string   `json:"referenced_table"`
	ReferencedColumns []string `json:"referenced_columns"`
}

// UniqueConstraintInfo detalha uma constraint de unicidade (UNIQUE).
type UniqueConstraintInfo struct {
	Name    string   `json:"name,omitempty"`
	Columns []string `json:"columns"`
}

// CheckConstraintInfo detalha uma regra de validação (CHECK constraint).
type CheckConstraintInfo struct {
	Name    string   `json:"name,omitempty"`
	Columns []string `json:"columns,omitempty"`
	Clause  string   `json:"clause"`
}

// NormalizeDetailLevel normaliza os níveis de detalhamento aceitos:
// 'basic' (apenas colunas, tipos, nullable e PK)
// 'standard' (inclui tamanho, precisão, valor padrão, PK, FK, Unique)
// 'detailed' (default: inclui tudo do standard + checks por campo e todas as constraints)
func NormalizeDetailLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "basic", "resumido", "summary", "simple":
		return "basic"
	case "standard", "padrao", "padrão", "normal", "medium":
		return "standard"
	case "detailed", "full", "completo", "all", "detalhado", "":
		return "detailed"
	default:
		return "detailed"
	}
}

func splitSchemaTable(tableName string) (schema string, table string) {
	tableName = strings.TrimSpace(tableName)
	tableName = strings.Trim(tableName, `"'`+"`")
	if idx := strings.Index(tableName, "."); idx != -1 {
		s := strings.Trim(strings.TrimSpace(tableName[:idx]), `"'`+"`")
		t := strings.Trim(strings.TrimSpace(tableName[idx+1:]), `"'`+"`")
		return s, t
	}
	return "", tableName
}

func isBalanced(s string) bool {
	depth := 0
	for _, ch := range s {
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

func cleanCheckClause(clause string) string {
	clause = strings.TrimSpace(clause)
	if strings.HasPrefix(strings.ToUpper(clause), "CHECK ") {
		clause = strings.TrimSpace(clause[6:])
	}
	for strings.HasPrefix(clause, "(") && strings.HasSuffix(clause, ")") {
		trimmed := strings.TrimSpace(clause[1 : len(clause)-1])
		if isBalanced(trimmed) {
			clause = trimmed
		} else {
			break
		}
	}
	return clause
}

func cleanDefaultValue(val string) string {
	val = strings.TrimSpace(val)
	for strings.HasPrefix(val, "(") && strings.HasSuffix(val, ")") {
		trimmed := strings.TrimSpace(val[1 : len(val)-1])
		if isBalanced(trimmed) {
			val = trimmed
		} else {
			break
		}
	}
	return val
}

func splitAndTrim(s, sep string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	var res []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			res = append(res, p)
		}
	}
	return res
}

func containsWord(text, word string) bool {
	if word == "" {
		return false
	}
	pattern := `\b` + regexp.QuoteMeta(word) + `\b`
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return strings.Contains(strings.ToLower(text), strings.ToLower(word))
	}
	return re.MatchString(text)
}

func isOracleNotNullCheck(cond, colName string) bool {
	condClean := strings.ToUpper(strings.TrimSpace(cond))
	condClean = strings.Trim(condClean, "()")
	condClean = strings.TrimSpace(condClean)

	colClean := strings.ToUpper(strings.TrimSpace(colName))
	pattern1 := fmt.Sprintf(`"%s" IS NOT NULL`, colClean)
	pattern2 := fmt.Sprintf(`%s IS NOT NULL`, colClean)
	return condClean == pattern1 || condClean == pattern2
}

func isByteLengthType(typ string) bool {
	t := strings.ToUpper(typ)
	return strings.Contains(t, "RAW") || strings.Contains(t, "BLOB") || strings.Contains(t, "BYTEA")
}

// getTableSchemaPostgres busca o schema detalhado no PostgreSQL
func (e *Executor) getTableSchemaPostgres(ctx context.Context, db *sql.DB, tableName string, detailLevel string) (*TableSchema, error) {
	schema, table := splitSchemaTable(tableName)

	colQuery := `
		SELECT 
			c.column_name, 
			c.data_type, 
			c.character_maximum_length, 
			c.numeric_precision, 
			c.numeric_scale, 
			c.is_nullable, 
			c.column_default
		FROM information_schema.columns c
		WHERE c.table_name = $1
		  AND ($2 = '' OR c.table_schema = $2)
		  AND c.table_schema NOT IN ('information_schema', 'pg_catalog')
		ORDER BY CASE WHEN c.table_schema = 'public' THEN 0 ELSE 1 END, c.ordinal_position`

	rows, err := db.QueryContext(ctx, colQuery, table, schema)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter colunas postgres: %w", err)
	}
	defer rows.Close()

	tableSchema := &TableSchema{
		Table:   tableName,
		Columns: make([]ColumnSchema, 0),
	}
	colIndexMap := make(map[string]int)

	for rows.Next() {
		var colName, dataType, isNullable string
		var charLen, numPrec, numScale sql.NullInt64
		var colDef sql.NullString

		if err := rows.Scan(&colName, &dataType, &charLen, &numPrec, &numScale, &isNullable, &colDef); err != nil {
			return nil, fmt.Errorf("erro ao escanear coluna postgres: %w", err)
		}

		c := ColumnSchema{
			Column:   colName,
			Type:     dataType,
			Nullable: strings.ToUpper(isNullable) == "YES",
		}

		if detailLevel != "basic" {
			if charLen.Valid && charLen.Int64 > 0 {
				l := int(charLen.Int64)
				c.Length = &l
			}
			if numPrec.Valid && numPrec.Int64 > 0 {
				p := int(numPrec.Int64)
				c.Precision = &p
			}
			if numScale.Valid {
				s := int(numScale.Int64)
				c.Scale = &s
			}
			if colDef.Valid && strings.TrimSpace(colDef.String) != "" {
				def := cleanDefaultValue(colDef.String)
				c.DefaultValue = &def
			}
		}

		colIndexMap[colName] = len(tableSchema.Columns)
		tableSchema.Columns = append(tableSchema.Columns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tableSchema.Columns) == 0 {
		return nil, fmt.Errorf("tabela '%s' não encontrada ou sem colunas acessíveis", tableName)
	}

	// Constraints (PK, FK, Unique, Check) via pg_constraint
	consQuery := `
		SELECT
			c.conname AS constraint_name,
			c.contype AS constraint_type,
			pg_get_constraintdef(c.oid) AS definition,
			COALESCE((
				SELECT string_agg(a.attname, ',' ORDER BY array_position(c.conkey, a.attnum))
				FROM pg_attribute a
				WHERE a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
			), '') AS columns,
			COALESCE(confrel.relname, '') AS foreign_table,
			COALESCE((
				SELECT string_agg(fa.attname, ',' ORDER BY array_position(c.confkey, fa.attnum))
				FROM pg_attribute fa
				WHERE fa.attrelid = c.confrelid AND fa.attnum = ANY(c.confkey)
			), '') AS foreign_columns
		FROM pg_constraint c
		JOIN pg_class cl ON cl.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = cl.relnamespace
		LEFT JOIN pg_class confrel ON confrel.oid = c.confrelid
		WHERE cl.relname = $1
		  AND ($2 = '' OR n.nspname = $2)
		  AND n.nspname NOT IN ('information_schema', 'pg_catalog')
		ORDER BY c.contype, c.conname`

	if cRows, err := db.QueryContext(ctx, consQuery, table, schema); err == nil {
		defer cRows.Close()
		for cRows.Next() {
			var conName, conType, def, colsStr, fTable, fColsStr string
			if err := cRows.Scan(&conName, &conType, &def, &colsStr, &fTable, &fColsStr); err != nil {
				continue
			}

			cols := splitAndTrim(colsStr, ",")
			fCols := splitAndTrim(fColsStr, ",")

			switch conType {
			case "p": // Primary Key
				for _, col := range cols {
					if idx, ok := colIndexMap[col]; ok {
						tableSchema.Columns[idx].PrimaryKey = true
					}
				}
				if detailLevel != "basic" {
					tableSchema.PrimaryKey = &PrimaryKeyInfo{
						Name:    conName,
						Columns: cols,
					}
				}
			case "f": // Foreign Key
				if detailLevel != "basic" {
					tableSchema.ForeignKeys = append(tableSchema.ForeignKeys, ForeignKeyInfo{
						Name:              conName,
						Columns:           cols,
						ReferencedTable:   fTable,
						ReferencedColumns: fCols,
					})
				}
			case "u": // Unique
				if detailLevel != "basic" {
					tableSchema.UniqueConstraints = append(tableSchema.UniqueConstraints, UniqueConstraintInfo{
						Name:    conName,
						Columns: cols,
					})
				}
			case "c": // Check
				if detailLevel == "detailed" {
					clause := cleanCheckClause(def)
					tableSchema.CheckConstraints = append(tableSchema.CheckConstraints, CheckConstraintInfo{
						Name:    conName,
						Columns: cols,
						Clause:  clause,
					})
					for _, col := range cols {
						if idx, ok := colIndexMap[col]; ok {
							tableSchema.Columns[idx].Checks = append(tableSchema.Columns[idx].Checks, clause)
						}
					}
				}
			}
		}
	}

	return tableSchema, nil
}

// getTableSchemaSqlServer busca o schema detalhado no SQL Server
func (e *Executor) getTableSchemaSqlServer(ctx context.Context, db *sql.DB, tableName string, detailLevel string) (*TableSchema, error) {
	schema, table := splitSchemaTable(tableName)

	colQuery := `
		SELECT 
			c.COLUMN_NAME, 
			c.DATA_TYPE, 
			c.CHARACTER_MAXIMUM_LENGTH, 
			c.NUMERIC_PRECISION, 
			c.NUMERIC_SCALE, 
			c.IS_NULLABLE, 
			c.COLUMN_DEFAULT
		FROM INFORMATION_SCHEMA.COLUMNS c
		WHERE c.TABLE_NAME = @p1
		  AND (@p2 = '' OR c.TABLE_SCHEMA = @p2)
		ORDER BY CASE WHEN c.TABLE_SCHEMA = 'dbo' THEN 0 ELSE 1 END, c.ORDINAL_POSITION`

	rows, err := db.QueryContext(ctx, colQuery, table, schema)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter colunas sqlserver: %w", err)
	}
	defer rows.Close()

	tableSchema := &TableSchema{
		Table:   tableName,
		Columns: make([]ColumnSchema, 0),
	}
	colIndexMap := make(map[string]int)

	for rows.Next() {
		var colName, dataType, isNullable string
		var charLen, numPrec, numScale sql.NullInt64
		var colDef sql.NullString

		if err := rows.Scan(&colName, &dataType, &charLen, &numPrec, &numScale, &isNullable, &colDef); err != nil {
			return nil, fmt.Errorf("erro ao escanear coluna sqlserver: %w", err)
		}

		c := ColumnSchema{
			Column:   colName,
			Type:     dataType,
			Nullable: strings.ToUpper(isNullable) == "YES",
		}

		if detailLevel != "basic" {
			if charLen.Valid && charLen.Int64 > 0 {
				l := int(charLen.Int64)
				c.Length = &l
			}
			if numPrec.Valid && numPrec.Int64 > 0 {
				p := int(numPrec.Int64)
				c.Precision = &p
			}
			if numScale.Valid {
				s := int(numScale.Int64)
				c.Scale = &s
			}
			if colDef.Valid && strings.TrimSpace(colDef.String) != "" {
				def := cleanDefaultValue(colDef.String)
				c.DefaultValue = &def
			}
		}

		colIndexMap[colName] = len(tableSchema.Columns)
		tableSchema.Columns = append(tableSchema.Columns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tableSchema.Columns) == 0 {
		return nil, fmt.Errorf("tabela '%s' não encontrada ou sem colunas acessíveis", tableName)
	}

	// 2. Chaves PK e Unique via sys.key_constraints
	pkUqQuery := `
		SELECT 
			kc.name AS constraint_name,
			kc.type AS constraint_type,
			c.name AS column_name
		FROM sys.key_constraints kc
		INNER JOIN sys.tables t ON kc.parent_object_id = t.object_id
		INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
		INNER JOIN sys.index_columns ic ON kc.parent_object_id = ic.object_id AND kc.unique_index_id = ic.index_id
		INNER JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
		WHERE t.name = @p1
		  AND (@p2 = '' OR s.name = @p2)
		ORDER BY kc.name, ic.key_ordinal`

	if pkRows, err := db.QueryContext(ctx, pkUqQuery, table, schema); err == nil {
		defer pkRows.Close()
		type keyEntry struct {
			conType string
			cols    []string
		}
		keyMap := make(map[string]*keyEntry)
		var keyOrder []string

		for pkRows.Next() {
			var conName, conType, colName string
			if err := pkRows.Scan(&conName, &conType, &colName); err == nil {
				if _, exists := keyMap[conName]; !exists {
					keyMap[conName] = &keyEntry{conType: strings.TrimSpace(conType)}
					keyOrder = append(keyOrder, conName)
				}
				keyMap[conName].cols = append(keyMap[conName].cols, colName)
			}
		}

		for _, name := range keyOrder {
			entry := keyMap[name]
			if entry.conType == "PK" {
				for _, col := range entry.cols {
					if idx, ok := colIndexMap[col]; ok {
						tableSchema.Columns[idx].PrimaryKey = true
					}
				}
				if detailLevel != "basic" {
					tableSchema.PrimaryKey = &PrimaryKeyInfo{
						Name:    name,
						Columns: entry.cols,
					}
				}
			} else if entry.conType == "UQ" && detailLevel != "basic" {
				tableSchema.UniqueConstraints = append(tableSchema.UniqueConstraints, UniqueConstraintInfo{
					Name:    name,
					Columns: entry.cols,
				})
			}
		}
	}

	if detailLevel == "basic" {
		return tableSchema, nil
	}

	// 3. Foreign Keys via sys.foreign_keys
	fkQuery := `
		SELECT 
			fk.name AS constraint_name,
			c_parent.name AS column_name,
			t_ref.name AS referenced_table,
			c_ref.name AS referenced_column
		FROM sys.foreign_keys fk
		INNER JOIN sys.tables t_parent ON fk.parent_object_id = t_parent.object_id
		INNER JOIN sys.schemas s_parent ON t_parent.schema_id = s_parent.schema_id
		INNER JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
		INNER JOIN sys.columns c_parent ON fkc.parent_object_id = c_parent.object_id AND fkc.parent_column_id = c_parent.column_id
		INNER JOIN sys.tables t_ref ON fkc.referenced_object_id = t_ref.object_id
		INNER JOIN sys.columns c_ref ON fkc.referenced_object_id = c_ref.object_id AND fkc.referenced_column_id = c_ref.column_id
		WHERE t_parent.name = @p1
		  AND (@p2 = '' OR s_parent.name = @p2)
		ORDER BY fk.name, fkc.constraint_column_id`

	if fkRows, err := db.QueryContext(ctx, fkQuery, table, schema); err == nil {
		defer fkRows.Close()
		type fkEntry struct {
			refTable string
			cols     []string
			refCols  []string
		}
		fkMap := make(map[string]*fkEntry)
		var fkOrder []string

		for fkRows.Next() {
			var conName, col, refTable, refCol string
			if err := fkRows.Scan(&conName, &col, &refTable, &refCol); err == nil {
				if _, exists := fkMap[conName]; !exists {
					fkMap[conName] = &fkEntry{refTable: refTable}
					fkOrder = append(fkOrder, conName)
				}
				fkMap[conName].cols = append(fkMap[conName].cols, col)
				fkMap[conName].refCols = append(fkMap[conName].refCols, refCol)
			}
		}

		for _, name := range fkOrder {
			e := fkMap[name]
			tableSchema.ForeignKeys = append(tableSchema.ForeignKeys, ForeignKeyInfo{
				Name:              name,
				Columns:           e.cols,
				ReferencedTable:   e.refTable,
				ReferencedColumns: e.refCols,
			})
		}
	}

	// 4. Check constraints via sys.check_constraints
	if detailLevel == "detailed" {
		chkQuery := `
			SELECT 
				cc.name AS constraint_name,
				COALESCE(c.name, '') AS column_name,
				cc.definition AS check_clause
			FROM sys.check_constraints cc
			INNER JOIN sys.tables t ON cc.parent_object_id = t.object_id
			INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
			LEFT JOIN sys.columns c ON cc.parent_object_id = c.object_id AND cc.parent_column_id = c.column_id
			WHERE t.name = @p1
			  AND (@p2 = '' OR s.name = @p2)
			ORDER BY cc.name`

		if chkRows, err := db.QueryContext(ctx, chkQuery, table, schema); err == nil {
			defer chkRows.Close()
			for chkRows.Next() {
				var conName, colName, def string
				if err := chkRows.Scan(&conName, &colName, &def); err == nil {
					clause := cleanCheckClause(def)
					var cols []string
					if colName != "" {
						cols = []string{colName}
						if idx, ok := colIndexMap[colName]; ok {
							tableSchema.Columns[idx].Checks = append(tableSchema.Columns[idx].Checks, clause)
						}
					}
					tableSchema.CheckConstraints = append(tableSchema.CheckConstraints, CheckConstraintInfo{
						Name:    conName,
						Columns: cols,
						Clause:  clause,
					})
				}
			}
		}
	}

	return tableSchema, nil
}

// getTableSchemaMysql busca o schema detalhado no MySQL
func (e *Executor) getTableSchemaMysql(ctx context.Context, db *sql.DB, tableName string, detailLevel string) (*TableSchema, error) {
	schema, table := splitSchemaTable(tableName)

	colQuery := `
		SELECT 
			c.COLUMN_NAME, 
			c.DATA_TYPE, 
			c.CHARACTER_MAXIMUM_LENGTH, 
			c.NUMERIC_PRECISION, 
			c.NUMERIC_SCALE, 
			c.IS_NULLABLE, 
			c.COLUMN_DEFAULT,
			c.COLUMN_KEY,
			c.EXTRA
		FROM INFORMATION_SCHEMA.COLUMNS c
		WHERE c.TABLE_NAME = ?
		  AND c.TABLE_SCHEMA = COALESCE(NULLIF(?, ''), DATABASE())
		ORDER BY c.ORDINAL_POSITION`

	rows, err := db.QueryContext(ctx, colQuery, table, schema)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter colunas mysql: %w", err)
	}
	defer rows.Close()

	tableSchema := &TableSchema{
		Table:   tableName,
		Columns: make([]ColumnSchema, 0),
	}
	colIndexMap := make(map[string]int)

	for rows.Next() {
		var colName, dataType, isNullable, colKey, extra string
		var charLen, numPrec, numScale sql.NullInt64
		var colDef sql.NullString

		if err := rows.Scan(&colName, &dataType, &charLen, &numPrec, &numScale, &isNullable, &colDef, &colKey, &extra); err != nil {
			return nil, fmt.Errorf("erro ao escanear coluna mysql: %w", err)
		}

		c := ColumnSchema{
			Column:     colName,
			Type:       dataType,
			Nullable:   strings.ToUpper(isNullable) == "YES",
			PrimaryKey: strings.ToUpper(colKey) == "PRI",
		}

		if detailLevel != "basic" {
			if charLen.Valid && charLen.Int64 > 0 {
				l := int(charLen.Int64)
				c.Length = &l
			}
			if numPrec.Valid && numPrec.Int64 > 0 {
				p := int(numPrec.Int64)
				c.Precision = &p
			}
			if numScale.Valid {
				s := int(numScale.Int64)
				c.Scale = &s
			}
			if colDef.Valid && strings.TrimSpace(colDef.String) != "" {
				def := cleanDefaultValue(colDef.String)
				c.DefaultValue = &def
			}
		}

		colIndexMap[colName] = len(tableSchema.Columns)
		tableSchema.Columns = append(tableSchema.Columns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tableSchema.Columns) == 0 {
		return nil, fmt.Errorf("tabela '%s' não encontrada ou sem colunas acessíveis", tableName)
	}

	if detailLevel == "basic" {
		return tableSchema, nil
	}

	// 2. Constraints (PK, FK, Unique) via TABLE_CONSTRAINTS + KEY_COLUMN_USAGE
	consQuery := `
		SELECT 
			tc.CONSTRAINT_NAME,
			tc.CONSTRAINT_TYPE,
			kcu.COLUMN_NAME,
			COALESCE(kcu.REFERENCED_TABLE_NAME, '') AS REFERENCED_TABLE,
			COALESCE(kcu.REFERENCED_COLUMN_NAME, '') AS REFERENCED_COLUMN
		FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
		JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
		  ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
		 AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA
		 AND tc.TABLE_NAME = kcu.TABLE_NAME
		WHERE tc.TABLE_NAME = ?
		  AND tc.TABLE_SCHEMA = COALESCE(NULLIF(?, ''), DATABASE())
		ORDER BY tc.CONSTRAINT_TYPE, tc.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`

	if cRows, err := db.QueryContext(ctx, consQuery, table, schema); err == nil {
		defer cRows.Close()
		type consEntry struct {
			conType  string
			cols     []string
			refTable string
			refCols  []string
		}
		consMap := make(map[string]*consEntry)
		var consOrder []string

		for cRows.Next() {
			var conName, conType, col, refTable, refCol string
			if err := cRows.Scan(&conName, &conType, &col, &refTable, &refCol); err == nil {
				if _, exists := consMap[conName]; !exists {
					consMap[conName] = &consEntry{
						conType:  conType,
						refTable: refTable,
					}
					consOrder = append(consOrder, conName)
				}
				consMap[conName].cols = append(consMap[conName].cols, col)
				if refCol != "" {
					consMap[conName].refCols = append(consMap[conName].refCols, refCol)
				}
			}
		}

		for _, name := range consOrder {
			e := consMap[name]
			switch e.conType {
			case "PRIMARY KEY":
				for _, col := range e.cols {
					if idx, ok := colIndexMap[col]; ok {
						tableSchema.Columns[idx].PrimaryKey = true
					}
				}
				tableSchema.PrimaryKey = &PrimaryKeyInfo{
					Name:    name,
					Columns: e.cols,
				}
			case "UNIQUE":
				tableSchema.UniqueConstraints = append(tableSchema.UniqueConstraints, UniqueConstraintInfo{
					Name:    name,
					Columns: e.cols,
				})
			case "FOREIGN KEY":
				tableSchema.ForeignKeys = append(tableSchema.ForeignKeys, ForeignKeyInfo{
					Name:              name,
					Columns:           e.cols,
					ReferencedTable:   e.refTable,
					ReferencedColumns: e.refCols,
				})
			}
		}
	}

	// 3. Checks (MySQL 8.0.16+)
	if detailLevel == "detailed" {
		chkQuery := `
			SELECT 
				tc.CONSTRAINT_NAME,
				cc.CHECK_CLAUSE
			FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
			JOIN INFORMATION_SCHEMA.CHECK_CONSTRAINTS cc
			  ON tc.CONSTRAINT_NAME = cc.CONSTRAINT_NAME
			 AND tc.CONSTRAINT_SCHEMA = cc.CONSTRAINT_SCHEMA
			WHERE tc.TABLE_NAME = ?
			  AND tc.TABLE_SCHEMA = COALESCE(NULLIF(?, ''), DATABASE())
			  AND tc.CONSTRAINT_TYPE = 'CHECK'
			ORDER BY tc.CONSTRAINT_NAME`

		if chkRows, err := db.QueryContext(ctx, chkQuery, table, schema); err == nil {
			defer chkRows.Close()
			for chkRows.Next() {
				var conName, clause string
				if err := chkRows.Scan(&conName, &clause); err == nil {
					cleanClause := cleanCheckClause(clause)
					var matchedCols []string
					for colName, idx := range colIndexMap {
						if containsWord(cleanClause, colName) {
							matchedCols = append(matchedCols, colName)
							tableSchema.Columns[idx].Checks = append(tableSchema.Columns[idx].Checks, cleanClause)
						}
					}
					tableSchema.CheckConstraints = append(tableSchema.CheckConstraints, CheckConstraintInfo{
						Name:    conName,
						Columns: matchedCols,
						Clause:  cleanClause,
					})
				}
			}
		}
	}

	return tableSchema, nil
}

// getTableSchemaOracle busca o schema detalhado no Oracle
func (e *Executor) getTableSchemaOracle(ctx context.Context, db *sql.DB, tableName string, detailLevel string) (*TableSchema, error) {
	owner, table := splitSchemaTable(tableName)
	owner = strings.ToUpper(owner)
	table = strings.ToUpper(table)

	colQuery := `
		SELECT 
			column_name, 
			data_type, 
			data_length, 
			char_length, 
			data_precision, 
			data_scale, 
			nullable, 
			data_default
		FROM all_tab_columns
		WHERE table_name = :1
		  AND (:2 IS NULL OR owner = :2)
		ORDER BY column_id`

	var ownerParam interface{} = owner
	if owner == "" {
		ownerParam = nil
	}

	rows, err := db.QueryContext(ctx, colQuery, table, ownerParam)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter colunas oracle: %w", err)
	}
	defer rows.Close()

	tableSchema := &TableSchema{
		Table:   tableName,
		Columns: make([]ColumnSchema, 0),
	}
	colIndexMap := make(map[string]int)

	for rows.Next() {
		var colName, dataType, nullable string
		var dataLen, charLen, dataPrec, dataScale sql.NullInt64
		var dataDef sql.NullString

		if err := rows.Scan(&colName, &dataType, &dataLen, &charLen, &dataPrec, &dataScale, &nullable, &dataDef); err != nil {
			return nil, fmt.Errorf("erro ao escanear coluna oracle: %w", err)
		}

		c := ColumnSchema{
			Column:   colName,
			Type:     dataType,
			Nullable: strings.ToUpper(nullable) == "Y",
		}

		if detailLevel != "basic" {
			if charLen.Valid && charLen.Int64 > 0 {
				l := int(charLen.Int64)
				c.Length = &l
			} else if dataLen.Valid && dataLen.Int64 > 0 && isByteLengthType(dataType) {
				l := int(dataLen.Int64)
				c.Length = &l
			}

			if dataPrec.Valid && dataPrec.Int64 > 0 {
				p := int(dataPrec.Int64)
				c.Precision = &p
			}
			if dataScale.Valid {
				s := int(dataScale.Int64)
				c.Scale = &s
			}
			if dataDef.Valid && strings.TrimSpace(dataDef.String) != "" {
				def := cleanDefaultValue(dataDef.String)
				c.DefaultValue = &def
			}
		}

		colIndexMap[colName] = len(tableSchema.Columns)
		tableSchema.Columns = append(tableSchema.Columns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(tableSchema.Columns) == 0 {
		return nil, fmt.Errorf("tabela '%s' não encontrada ou sem colunas acessíveis", tableName)
	}

	// 2. Constraints (PK, FK, Unique) - sem joins com LONG para total compatibilidade
	consQuery := `
		SELECT 
			c.constraint_name,
			c.constraint_type,
			cc.column_name,
			COALESCE(r.table_name, '') AS ref_table,
			COALESCE(r_cc.column_name, '') AS ref_column
		FROM all_constraints c
		JOIN all_cons_columns cc 
			ON c.owner = cc.owner AND c.constraint_name = cc.constraint_name
		LEFT JOIN all_constraints r 
			ON c.r_owner = r.owner AND c.r_constraint_name = r.constraint_name
		LEFT JOIN all_cons_columns r_cc 
			ON r.owner = r_cc.owner AND r.constraint_name = r_cc.constraint_name AND cc.position = r_cc.position
		WHERE c.table_name = :1
		  AND (:2 IS NULL OR c.owner = :2)
		  AND c.constraint_type IN ('P', 'R', 'U')
		ORDER BY c.constraint_type, c.constraint_name, cc.position`

	if cRows, err := db.QueryContext(ctx, consQuery, table, ownerParam); err == nil {
		defer cRows.Close()
		type consEntry struct {
			conType  string
			cols     []string
			refTable string
			refCols  []string
		}
		consMap := make(map[string]*consEntry)
		var consOrder []string

		for cRows.Next() {
			var conName, conType, col, refTable, refCol string
			if err := cRows.Scan(&conName, &conType, &col, &refTable, &refCol); err == nil {
				if _, exists := consMap[conName]; !exists {
					consMap[conName] = &consEntry{
						conType:  conType,
						refTable: refTable,
					}
					consOrder = append(consOrder, conName)
				}
				consMap[conName].cols = append(consMap[conName].cols, col)
				if refCol != "" {
					consMap[conName].refCols = append(consMap[conName].refCols, refCol)
				}
			}
		}

		for _, name := range consOrder {
			e := consMap[name]
			switch e.conType {
			case "P":
				for _, col := range e.cols {
					if idx, ok := colIndexMap[col]; ok {
						tableSchema.Columns[idx].PrimaryKey = true
					}
				}
				if detailLevel != "basic" {
					tableSchema.PrimaryKey = &PrimaryKeyInfo{
						Name:    name,
						Columns: e.cols,
					}
				}
			case "U":
				if detailLevel != "basic" {
					tableSchema.UniqueConstraints = append(tableSchema.UniqueConstraints, UniqueConstraintInfo{
						Name:    name,
						Columns: e.cols,
					})
				}
			case "R":
				if detailLevel != "basic" {
					tableSchema.ForeignKeys = append(tableSchema.ForeignKeys, ForeignKeyInfo{
						Name:              name,
						Columns:           e.cols,
						ReferencedTable:   e.refTable,
						ReferencedColumns: e.refCols,
					})
				}
			}
		}
	}

	if detailLevel == "detailed" {
		// 3. Check constraints do Oracle:
		// 3a. Mapear colunas de all_cons_columns para constraint_type = 'C'
		chkColsQuery := `
			SELECT cc.constraint_name, cc.column_name
			FROM all_cons_columns cc
			JOIN all_constraints c ON cc.owner = c.owner AND cc.constraint_name = c.constraint_name
			WHERE cc.table_name = :1
			  AND (:2 IS NULL OR cc.owner = :2)
			  AND c.constraint_type = 'C'`

		chkColMap := make(map[string][]string)
		if ccRows, err := db.QueryContext(ctx, chkColsQuery, table, ownerParam); err == nil {
			defer ccRows.Close()
			for ccRows.Next() {
				var conName, colName string
				if err := ccRows.Scan(&conName, &colName); err == nil {
					chkColMap[conName] = append(chkColMap[conName], colName)
				}
			}
		}

		// 3b. Buscar search_condition de all_constraints (sem joins para suportar LONG com segurança)
		chkCondQuery := `
			SELECT constraint_name, search_condition
			FROM all_constraints
			WHERE table_name = :1
			  AND (:2 IS NULL OR owner = :2)
			  AND constraint_type = 'C'`

		if condRows, err := db.QueryContext(ctx, chkCondQuery, table, ownerParam); err == nil {
			defer condRows.Close()
			for condRows.Next() {
				var conName string
				var searchCond sql.NullString
				if err := condRows.Scan(&conName, &searchCond); err == nil && searchCond.Valid {
					cond := strings.TrimSpace(searchCond.String)
					cols := chkColMap[conName]

					// Ignora checks automáticos de NOT NULL do Oracle
					isNotNull := false
					for _, col := range cols {
						if isOracleNotNullCheck(cond, col) {
							isNotNull = true
							break
						}
					}
					if isNotNull {
						continue
					}

					cleanClause := cleanCheckClause(cond)
					tableSchema.CheckConstraints = append(tableSchema.CheckConstraints, CheckConstraintInfo{
						Name:    conName,
						Columns: cols,
						Clause:  cleanClause,
					})
					for _, col := range cols {
						if idx, ok := colIndexMap[col]; ok {
							tableSchema.Columns[idx].Checks = append(tableSchema.Columns[idx].Checks, cleanClause)
						}
					}
				}
			}
		}
	}

	return tableSchema, nil
}
