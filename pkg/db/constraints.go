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

type ConstraintResult struct {
	Owner           string `json:"owner"`
	TableName       string `json:"table_name"`
	ConstraintName  string `json:"constraint_name"`
	ConstraintType  string `json:"constraint_type"` // PK, FK, UNIQUE, CHECK, DEFAULT
	Columns         string `json:"columns"`
	RefOwner        string `json:"ref_owner"`
	RefTable        string `json:"ref_table"`
	RefColumns      string `json:"ref_columns"`
	OnDelete        string `json:"on_delete"`
	OnUpdate        string `json:"on_update"`
	Status          string `json:"status"` // ENABLED, DISABLED
	CheckExpression string `json:"check_expression"`
}

var ConstraintColumns = []string{
	"owner",
	"table_name",
	"constraint_name",
	"constraint_type",
	"columns",
	"ref_owner",
	"ref_table",
	"ref_columns",
	"on_delete",
	"on_update",
	"status",
	"check_expression",
}

func (c ConstraintResult) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"owner":            c.Owner,
		"table_name":       c.TableName,
		"constraint_name":  c.ConstraintName,
		"constraint_type":  c.ConstraintType,
		"columns":          c.Columns,
		"ref_owner":        c.RefOwner,
		"ref_table":        c.RefTable,
		"ref_columns":      c.RefColumns,
		"on_delete":        c.OnDelete,
		"on_update":        c.OnUpdate,
		"status":           c.Status,
		"check_expression": c.CheckExpression,
	}
}

type ListConstraintsOptions struct {
	Owner          string
	TableName      string
	ConstraintType string // PK, FK, UNIQUE, CHECK, DEFAULT
	Direction      string // own (default) ou referenced_by
}

func (e *Executor) ListConstraints(db *sql.DB, dbType string, opts ListConstraintsOptions) ([]ConstraintResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	// Validação de identificadores contra injeção SQL
	if err := security.ValidateIdentifier(opts.Owner); err != nil {
		return nil, err
	}
	if err := security.ValidateIdentifier(opts.TableName); err != nil {
		return nil, err
	}

	opts.Direction = strings.ToLower(strings.TrimSpace(opts.Direction))
	if opts.Direction == "" {
		opts.Direction = "own"
	}
	if opts.Direction != "own" && opts.Direction != "referenced_by" {
		return nil, fmt.Errorf("direção inválida '%s'. Valores aceitos: 'own' ou 'referenced_by'", opts.Direction)
	}

	if opts.Direction == "referenced_by" && strings.TrimSpace(opts.TableName) == "" {
		return nil, fmt.Errorf("o parâmetro 'table_name' é obrigatório quando direction='referenced_by'")
	}

	opts.ConstraintType = strings.ToUpper(strings.TrimSpace(opts.ConstraintType))
	if opts.ConstraintType != "" {
		validTypes := map[string]bool{
			"PK":      true,
			"FK":      true,
			"UNIQUE":  true,
			"CHECK":   true,
			"DEFAULT": true,
		}
		if !validTypes[opts.ConstraintType] {
			return nil, fmt.Errorf("tipo de constraint '%s' inválido. Valores aceitos: PK, FK, UNIQUE, CHECK, DEFAULT", opts.ConstraintType)
		}
	}

	// Se direction=referenced_by e o usuário filtrou por um tipo diferente de FK, retorna lista vazia
	if opts.Direction == "referenced_by" && opts.ConstraintType != "" && opts.ConstraintType != "FK" {
		return []ConstraintResult{}, nil
	}

	var results []ConstraintResult
	var err error

	switch strings.ToLower(dbType) {
	case "oracle":
		results, err = e.listConstraintsOracle(ctx, db, opts)
	case "sqlserver":
		results, err = e.listConstraintsSqlServer(ctx, db, opts)
	case "postgres":
		results, err = e.listConstraintsPostgres(ctx, db, opts)
	case "mysql":
		results, err = e.listConstraintsMysql(ctx, db, opts)
	default:
		return nil, fmt.Errorf("tipo de banco '%s' não suportado", dbType)
	}

	if err != nil {
		return nil, err
	}

	// Ordenação determinística: owner, table_name, constraint_name, constraint_type
	sort.Slice(results, func(i, j int) bool {
		if results[i].Owner != results[j].Owner {
			return results[i].Owner < results[j].Owner
		}
		if results[i].TableName != results[j].TableName {
			return results[i].TableName < results[j].TableName
		}
		if results[i].ConstraintName != results[j].ConstraintName {
			return results[i].ConstraintName < results[j].ConstraintName
		}
		return results[i].ConstraintType < results[j].ConstraintType
	})

	return results, nil
}

// --- ORACLE ---

func (e *Executor) listConstraintsOracle(ctx context.Context, db *sql.DB, opts ListConstraintsOptions) ([]ConstraintResult, error) {
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

	type keyItem struct {
		owner          string
		tableName      string
		constraintName string
		constraintType string
		status         string
		onDelete       string
		refOwner       string
		refTable       string
		cols           []string
		refCols        []string
	}

	keyMap := make(map[string]*keyItem)
	var keyOrder []string

	if opts.Direction == "referenced_by" {
		query := `
			SELECT 
				c.owner,
				c.table_name,
				c.constraint_name,
				c.status,
				COALESCE(c.delete_rule, '') AS delete_rule,
				cc.column_name,
				r.owner AS ref_owner,
				r.table_name AS ref_table,
				COALESCE(r_cc.column_name, '') AS ref_column
			FROM all_constraints c
			JOIN all_cons_columns cc 
				ON c.owner = cc.owner AND c.constraint_name = cc.constraint_name
			JOIN all_constraints r 
				ON c.r_owner = r.owner AND c.r_constraint_name = r.constraint_name
			LEFT JOIN all_cons_columns r_cc 
				ON r.owner = r_cc.owner AND r.constraint_name = r_cc.constraint_name AND cc.position = r_cc.position
			WHERE c.constraint_type = 'R'
			  AND UPPER(r.table_name) = :1
			  AND (:2 IS NULL OR UPPER(r.owner) = :2)
			ORDER BY c.owner, c.table_name, c.constraint_name, cc.position`

		rows, err := db.QueryContext(ctx, query, table, ownerParam)
		if err != nil {
			return nil, fmt.Errorf("erro ao consultar constraints referenciadas no oracle: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var o, t, conName, status, delRule, col, refO, refT, refCol string
			if err := rows.Scan(&o, &t, &conName, &status, &delRule, &col, &refO, &refT, &refCol); err != nil {
				return nil, err
			}
			id := o + "." + conName
			item, exists := keyMap[id]
			if !exists {
				item = &keyItem{
					owner:          o,
					tableName:      t,
					constraintName: conName,
					constraintType: "FK",
					status:         status,
					onDelete:       delRule,
					refOwner:       refO,
					refTable:       refT,
				}
				keyMap[id] = item
				keyOrder = append(keyOrder, id)
			}
			item.cols = append(item.cols, col)
			if refCol != "" {
				item.refCols = append(item.refCols, refCol)
			}
		}
	} else {
		// direction = own
		query := `
			SELECT 
				c.owner,
				c.table_name,
				c.constraint_name,
				c.constraint_type,
				c.status,
				COALESCE(c.delete_rule, '') AS delete_rule,
				cc.column_name,
				COALESCE(r.owner, '') AS ref_owner,
				COALESCE(r.table_name, '') AS ref_table,
				COALESCE(r_cc.column_name, '') AS ref_column
			FROM all_constraints c
			JOIN all_cons_columns cc 
				ON c.owner = cc.owner AND c.constraint_name = cc.constraint_name
			LEFT JOIN all_constraints r 
				ON c.r_owner = r.owner AND c.r_constraint_name = r.constraint_name
			LEFT JOIN all_cons_columns r_cc 
				ON r.owner = r_cc.owner AND r.constraint_name = r_cc.constraint_name AND cc.position = r_cc.position
			WHERE (:1 IS NULL OR UPPER(c.owner) = :1)
			  AND (:2 IS NULL OR UPPER(c.table_name) = :2)
			  AND c.constraint_type IN ('P', 'R', 'U', 'C')
			ORDER BY c.owner, c.table_name, c.constraint_name, cc.position`

		rows, err := db.QueryContext(ctx, query, ownerParam, tableParam)
		if err != nil {
			return nil, fmt.Errorf("erro ao consultar constraints no oracle: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var o, t, conName, conType, status, delRule, col, refO, refT, refCol string
			if err := rows.Scan(&o, &t, &conName, &conType, &status, &delRule, &col, &refO, &refT, &refCol); err != nil {
				return nil, err
			}

			mappedType := ""
			switch conType {
			case "P":
				mappedType = "PK"
			case "R":
				mappedType = "FK"
			case "U":
				mappedType = "UNIQUE"
			case "C":
				mappedType = "CHECK"
			}

			id := o + "." + conName
			item, exists := keyMap[id]
			if !exists {
				item = &keyItem{
					owner:          o,
					tableName:      t,
					constraintName: conName,
					constraintType: mappedType,
					status:         status,
					onDelete:       delRule,
					refOwner:       refO,
					refTable:       refT,
				}
				keyMap[id] = item
				keyOrder = append(keyOrder, id)
			}
			item.cols = append(item.cols, col)
			if refCol != "" {
				item.refCols = append(item.refCols, refCol)
			}
		}
	}

	// 2. Para CHECK constraints, ler search_condition separadamente de all_constraints
	// (evita ORA-00997 com LONG e joins)
	checkConditions := make(map[string]string)
	if opts.Direction == "own" && (opts.ConstraintType == "" || opts.ConstraintType == "CHECK") {
		condQuery := `
			SELECT owner, constraint_name, search_condition
			FROM all_constraints
			WHERE (:1 IS NULL OR UPPER(owner) = :1)
			  AND (:2 IS NULL OR UPPER(table_name) = :2)
			  AND constraint_type = 'C'`

		if condRows, err := db.QueryContext(ctx, condQuery, ownerParam, tableParam); err == nil {
			defer condRows.Close()
			for condRows.Next() {
				var o, conName string
				var searchCond sql.NullString
				if err := condRows.Scan(&o, &conName, &searchCond); err == nil && searchCond.Valid {
					checkConditions[o+"."+conName] = strings.TrimSpace(searchCond.String)
				}
			}
		}
	}

	var results []ConstraintResult
	for _, id := range keyOrder {
		item := keyMap[id]

		if item.constraintType == "CHECK" {
			rawCond, hasCond := checkConditions[id]
			if !hasCond {
				continue
			}
			// Ignora check interno de NOT NULL do Oracle
			isNotNull := false
			for _, col := range item.cols {
				if isOracleNotNullCheck(rawCond, col) {
					isNotNull = true
					break
				}
			}
			if isNotNull {
				continue
			}
			item.onDelete = ""
			res := ConstraintResult{
				Owner:           item.owner,
				TableName:       item.tableName,
				ConstraintName:  item.constraintName,
				ConstraintType:  "CHECK",
				Columns:         strings.Join(item.cols, ", "),
				Status:          item.status,
				CheckExpression: cleanCheckClause(rawCond),
			}
			if opts.ConstraintType == "" || opts.ConstraintType == "CHECK" {
				results = append(results, res)
			}
			continue
		}

		if opts.ConstraintType != "" && item.constraintType != opts.ConstraintType {
			continue
		}

		res := ConstraintResult{
			Owner:          item.owner,
			TableName:      item.tableName,
			ConstraintName: item.constraintName,
			ConstraintType: item.constraintType,
			Columns:        strings.Join(item.cols, ", "),
			RefOwner:       item.refOwner,
			RefTable:       item.refTable,
			RefColumns:     strings.Join(item.refCols, ", "),
			OnDelete:       item.onDelete,
			Status:         item.status,
		}
		results = append(results, res)
	}

	// 3. DEFAULT constraints em all_tab_columns
	if opts.Direction == "own" && (opts.ConstraintType == "" || opts.ConstraintType == "DEFAULT") {
		defQuery := `
			SELECT owner, table_name, column_name, data_default
			FROM all_tab_columns
			WHERE (:1 IS NULL OR UPPER(owner) = :1)
			  AND (:2 IS NULL OR UPPER(table_name) = :2)
			  AND data_default IS NOT NULL
			ORDER BY owner, table_name, column_id`

		if defRows, err := db.QueryContext(ctx, defQuery, ownerParam, tableParam); err == nil {
			defer defRows.Close()
			for defRows.Next() {
				var o, t, col string
				var dataDef sql.NullString
				if err := defRows.Scan(&o, &t, &col, &dataDef); err == nil && dataDef.Valid {
					val := strings.TrimSpace(dataDef.String)
					if val != "" {
						results = append(results, ConstraintResult{
							Owner:           o,
							TableName:       t,
							ConstraintName:  fmt.Sprintf("DF_%s_%s", t, col),
							ConstraintType:  "DEFAULT",
							Columns:         col,
							Status:          "ENABLED",
							CheckExpression: cleanDefaultValue(val),
						})
					}
				}
			}
		}
	}

	return results, nil
}

// --- SQL SERVER ---

func (e *Executor) listConstraintsSqlServer(ctx context.Context, db *sql.DB, opts ListConstraintsOptions) ([]ConstraintResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	table := strings.TrimSpace(opts.TableName)

	var results []ConstraintResult

	// 1. PK e UNIQUE via sys.key_constraints (apenas se direction == "own")
	if opts.Direction == "own" && (opts.ConstraintType == "" || opts.ConstraintType == "PK" || opts.ConstraintType == "UNIQUE") {
		pkUqQuery := `
			SELECT 
				s.name AS owner,
				t.name AS table_name,
				kc.name AS constraint_name,
				kc.type AS constraint_type,
				c.name AS column_name,
				CASE WHEN kc.is_disabled = 1 THEN 'DISABLED' ELSE 'ENABLED' END AS status
			FROM sys.key_constraints kc
			INNER JOIN sys.tables t ON kc.parent_object_id = t.object_id
			INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
			INNER JOIN sys.index_columns ic ON kc.parent_object_id = ic.object_id AND kc.unique_index_id = ic.index_id
			INNER JOIN sys.columns c ON ic.object_id = c.object_id AND ic.column_id = c.column_id
			WHERE (@p1 = '' OR LOWER(s.name) = LOWER(@p1))
			  AND (@p2 = '' OR LOWER(t.name) = LOWER(@p2))
			ORDER BY s.name, t.name, kc.name, ic.key_ordinal`

		if rows, err := db.QueryContext(ctx, pkUqQuery, owner, table); err == nil {
			defer rows.Close()
			type keyGroup struct {
				owner     string
				tableName string
				conName   string
				conType   string
				status    string
				cols      []string
			}
			groupMap := make(map[string]*keyGroup)
			var groupOrder []string

			for rows.Next() {
				var o, t, conName, conType, col, status string
				if err := rows.Scan(&o, &t, &conName, &conType, &col, &status); err == nil {
					id := o + "." + conName
					item, exists := groupMap[id]
					if !exists {
						mappedType := "PK"
						if strings.TrimSpace(conType) == "UQ" {
							mappedType = "UNIQUE"
						}
						item = &keyGroup{
							owner:     o,
							tableName: t,
							conName:   conName,
							conType:   mappedType,
							status:    status,
						}
						groupMap[id] = item
						groupOrder = append(groupOrder, id)
					}
					item.cols = append(item.cols, col)
				}
			}

			for _, id := range groupOrder {
				item := groupMap[id]
				if opts.ConstraintType != "" && item.conType != opts.ConstraintType {
					continue
				}
				results = append(results, ConstraintResult{
					Owner:          item.owner,
					TableName:      item.tableName,
					ConstraintName: item.conName,
					ConstraintType: item.conType,
					Columns:        strings.Join(item.cols, ", "),
					Status:         item.status,
				})
			}
		}
	}

	// 2. FK via sys.foreign_keys (para direction "own" ou "referenced_by")
	if opts.ConstraintType == "" || opts.ConstraintType == "FK" {
		var fkQuery string
		var p1, p2 string

		if opts.Direction == "referenced_by" {
			fkQuery = `
				SELECT 
					s_parent.name AS owner,
					t_parent.name AS table_name,
					fk.name AS constraint_name,
					c_parent.name AS column_name,
					s_ref.name AS ref_owner,
					t_ref.name AS ref_table,
					c_ref.name AS ref_column,
					REPLACE(fk.delete_referential_action_desc, '_', ' ') AS on_delete,
					REPLACE(fk.update_referential_action_desc, '_', ' ') AS on_update,
					CASE WHEN fk.is_disabled = 1 THEN 'DISABLED' ELSE 'ENABLED' END AS status
				FROM sys.foreign_keys fk
				INNER JOIN sys.tables t_parent ON fk.parent_object_id = t_parent.object_id
				INNER JOIN sys.schemas s_parent ON t_parent.schema_id = s_parent.schema_id
				INNER JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
				INNER JOIN sys.columns c_parent ON fkc.parent_object_id = c_parent.object_id AND fkc.parent_column_id = c_parent.column_id
				INNER JOIN sys.tables t_ref ON fkc.referenced_object_id = t_ref.object_id
				INNER JOIN sys.schemas s_ref ON t_ref.schema_id = s_ref.schema_id
				INNER JOIN sys.columns c_ref ON fkc.referenced_object_id = c_ref.object_id AND fkc.referenced_column_id = c_ref.column_id
				WHERE (@p1 = '' OR LOWER(s_ref.name) = LOWER(@p1))
				  AND LOWER(t_ref.name) = LOWER(@p2)
				ORDER BY s_parent.name, t_parent.name, fk.name, fkc.constraint_column_id`
			p1 = owner
			p2 = table
		} else {
			fkQuery = `
				SELECT 
					s_parent.name AS owner,
					t_parent.name AS table_name,
					fk.name AS constraint_name,
					c_parent.name AS column_name,
					s_ref.name AS ref_owner,
					t_ref.name AS ref_table,
					c_ref.name AS ref_column,
					REPLACE(fk.delete_referential_action_desc, '_', ' ') AS on_delete,
					REPLACE(fk.update_referential_action_desc, '_', ' ') AS on_update,
					CASE WHEN fk.is_disabled = 1 THEN 'DISABLED' ELSE 'ENABLED' END AS status
				FROM sys.foreign_keys fk
				INNER JOIN sys.tables t_parent ON fk.parent_object_id = t_parent.object_id
				INNER JOIN sys.schemas s_parent ON t_parent.schema_id = s_parent.schema_id
				INNER JOIN sys.foreign_key_columns fkc ON fk.object_id = fkc.constraint_object_id
				INNER JOIN sys.columns c_parent ON fkc.parent_object_id = c_parent.object_id AND fkc.parent_column_id = c_parent.column_id
				INNER JOIN sys.tables t_ref ON fkc.referenced_object_id = t_ref.object_id
				INNER JOIN sys.schemas s_ref ON t_ref.schema_id = s_ref.schema_id
				INNER JOIN sys.columns c_ref ON fkc.referenced_object_id = c_ref.object_id AND fkc.referenced_column_id = c_ref.column_id
				WHERE (@p1 = '' OR LOWER(s_parent.name) = LOWER(@p1))
				  AND (@p2 = '' OR LOWER(t_parent.name) = LOWER(@p2))
				ORDER BY s_parent.name, t_parent.name, fk.name, fkc.constraint_column_id`
			p1 = owner
			p2 = table
		}

		if rows, err := db.QueryContext(ctx, fkQuery, p1, p2); err == nil {
			defer rows.Close()
			type fkGroup struct {
				owner     string
				tableName string
				conName   string
				refOwner  string
				refTable  string
				onDelete  string
				onUpdate  string
				status    string
				cols      []string
				refCols   []string
			}
			fkMap := make(map[string]*fkGroup)
			var fkOrder []string

			for rows.Next() {
				var o, t, conName, col, refO, refT, refCol, onDel, onUpd, status string
				if err := rows.Scan(&o, &t, &conName, &col, &refO, &refT, &refCol, &onDel, &onUpd, &status); err == nil {
					id := o + "." + conName
					item, exists := fkMap[id]
					if !exists {
						item = &fkGroup{
							owner:     o,
							tableName: t,
							conName:   conName,
							refOwner:  refO,
							refTable:  refT,
							onDelete:  onDel,
							onUpdate:  onUpd,
							status:    status,
						}
						fkMap[id] = item
						fkOrder = append(fkOrder, id)
					}
					item.cols = append(item.cols, col)
					item.refCols = append(item.refCols, refCol)
				}
			}

			for _, id := range fkOrder {
				item := fkMap[id]
				results = append(results, ConstraintResult{
					Owner:          item.owner,
					TableName:      item.tableName,
					ConstraintName: item.conName,
					ConstraintType: "FK",
					Columns:        strings.Join(item.cols, ", "),
					RefOwner:       item.refOwner,
					RefTable:       item.refTable,
					RefColumns:     strings.Join(item.refCols, ", "),
					OnDelete:       item.onDelete,
					OnUpdate:       item.onUpdate,
					Status:         item.status,
				})
			}
		}
	}

	// 3. CHECK via sys.check_constraints (direction == "own")
	if opts.Direction == "own" && (opts.ConstraintType == "" || opts.ConstraintType == "CHECK") {
		chkQuery := `
			SELECT 
				s.name AS owner,
				t.name AS table_name,
				cc.name AS constraint_name,
				COALESCE(c.name, '') AS column_name,
				cc.definition AS check_clause,
				CASE WHEN cc.is_disabled = 1 THEN 'DISABLED' ELSE 'ENABLED' END AS status
			FROM sys.check_constraints cc
			INNER JOIN sys.tables t ON cc.parent_object_id = t.object_id
			INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
			LEFT JOIN sys.columns c ON cc.parent_object_id = c.object_id AND cc.parent_column_id = c.column_id
			WHERE (@p1 = '' OR LOWER(s.name) = LOWER(@p1))
			  AND (@p2 = '' OR LOWER(t.name) = LOWER(@p2))
			ORDER BY s.name, t.name, cc.name`

		if rows, err := db.QueryContext(ctx, chkQuery, owner, table); err == nil {
			defer rows.Close()
			for rows.Next() {
				var o, t, conName, col, def, status string
				if err := rows.Scan(&o, &t, &conName, &col, &def, &status); err == nil {
					results = append(results, ConstraintResult{
						Owner:           o,
						TableName:       t,
						ConstraintName:  conName,
						ConstraintType:  "CHECK",
						Columns:         col,
						Status:          status,
						CheckExpression: cleanCheckClause(def),
					})
				}
			}
		}
	}

	// 4. DEFAULT via sys.default_constraints (direction == "own")
	if opts.Direction == "own" && (opts.ConstraintType == "" || opts.ConstraintType == "DEFAULT") {
		defQuery := `
			SELECT 
				s.name AS owner,
				t.name AS table_name,
				dc.name AS constraint_name,
				c.name AS column_name,
				dc.definition AS check_clause,
				CASE WHEN dc.is_disabled = 1 THEN 'DISABLED' ELSE 'ENABLED' END AS status
			FROM sys.default_constraints dc
			INNER JOIN sys.tables t ON dc.parent_object_id = t.object_id
			INNER JOIN sys.schemas s ON t.schema_id = s.schema_id
			INNER JOIN sys.columns c ON dc.parent_object_id = c.object_id AND dc.parent_column_id = c.column_id
			WHERE (@p1 = '' OR LOWER(s.name) = LOWER(@p1))
			  AND (@p2 = '' OR LOWER(t.name) = LOWER(@p2))
			ORDER BY s.name, t.name, dc.name`

		if rows, err := db.QueryContext(ctx, defQuery, owner, table); err == nil {
			defer rows.Close()
			for rows.Next() {
				var o, t, conName, col, def, status string
				if err := rows.Scan(&o, &t, &conName, &col, &def, &status); err == nil {
					results = append(results, ConstraintResult{
						Owner:           o,
						TableName:       t,
						ConstraintName:  conName,
						ConstraintType:  "DEFAULT",
						Columns:         col,
						Status:          status,
						CheckExpression: cleanDefaultValue(def),
					})
				}
			}
		}
	}

	return results, nil
}

// --- POSTGRESQL ---

func (e *Executor) listConstraintsPostgres(ctx context.Context, db *sql.DB, opts ListConstraintsOptions) ([]ConstraintResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	table := strings.TrimSpace(opts.TableName)

	var results []ConstraintResult

	// 1. Constraints via pg_constraint
	var consQuery string
	var p1, p2 string

	if opts.Direction == "referenced_by" {
		consQuery = `
			SELECT
				n.nspname AS owner,
				cl.relname AS table_name,
				c.conname AS constraint_name,
				'f' AS constraint_type,
				COALESCE((
					SELECT string_agg(a.attname, ', ' ORDER BY array_position(c.conkey, a.attnum))
					FROM pg_attribute a
					WHERE a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
				), '') AS columns,
				COALESCE(confn.nspname, '') AS ref_owner,
				COALESCE(confrel.relname, '') AS ref_table,
				COALESCE((
					SELECT string_agg(fa.attname, ', ' ORDER BY array_position(c.confkey, fa.attnum))
					FROM pg_attribute fa
					WHERE fa.attrelid = c.confrelid AND fa.attnum = ANY(c.confkey)
				), '') AS ref_columns,
				CASE c.confdeltype
					WHEN 'c' THEN 'CASCADE'
					WHEN 'n' THEN 'SET NULL'
					WHEN 'd' THEN 'SET DEFAULT'
					WHEN 'r' THEN 'RESTRICT'
					WHEN 'a' THEN 'NO ACTION'
					ELSE ''
				END AS on_delete,
				CASE c.confupdtype
					WHEN 'c' THEN 'CASCADE'
					WHEN 'n' THEN 'SET NULL'
					WHEN 'd' THEN 'SET DEFAULT'
					WHEN 'r' THEN 'RESTRICT'
					WHEN 'a' THEN 'NO ACTION'
					ELSE ''
				END AS on_update,
				CASE WHEN c.convalidated THEN 'ENABLED' ELSE 'DISABLED' END AS status,
				'' AS check_expression
			FROM pg_constraint c
			JOIN pg_class cl ON cl.oid = c.conrelid
			JOIN pg_namespace n ON n.oid = cl.relnamespace
			JOIN pg_class confrel ON confrel.oid = c.confrelid
			JOIN pg_namespace confn ON confn.oid = confrel.relnamespace
			WHERE c.contype = 'f'
			  AND ($1 = '' OR LOWER(confn.nspname) = LOWER($1))
			  AND LOWER(confrel.relname) = LOWER($2)
			ORDER BY n.nspname, cl.relname, c.conname`
		p1 = owner
		p2 = table
	} else {
		consQuery = `
			SELECT
				n.nspname AS owner,
				cl.relname AS table_name,
				c.conname AS constraint_name,
				c.contype AS constraint_type,
				COALESCE((
					SELECT string_agg(a.attname, ', ' ORDER BY array_position(c.conkey, a.attnum))
					FROM pg_attribute a
					WHERE a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
				), '') AS columns,
				COALESCE(confn.nspname, '') AS ref_owner,
				COALESCE(confrel.relname, '') AS ref_table,
				COALESCE((
					SELECT string_agg(fa.attname, ', ' ORDER BY array_position(c.confkey, fa.attnum))
					FROM pg_attribute fa
					WHERE fa.attrelid = c.confrelid AND fa.attnum = ANY(c.confkey)
				), '') AS ref_columns,
				CASE c.confdeltype
					WHEN 'c' THEN 'CASCADE'
					WHEN 'n' THEN 'SET NULL'
					WHEN 'd' THEN 'SET DEFAULT'
					WHEN 'r' THEN 'RESTRICT'
					WHEN 'a' THEN 'NO ACTION'
					ELSE ''
				END AS on_delete,
				CASE c.confupdtype
					WHEN 'c' THEN 'CASCADE'
					WHEN 'n' THEN 'SET NULL'
					WHEN 'd' THEN 'SET DEFAULT'
					WHEN 'r' THEN 'RESTRICT'
					WHEN 'a' THEN 'NO ACTION'
					ELSE ''
				END AS on_update,
				CASE WHEN c.convalidated THEN 'ENABLED' ELSE 'DISABLED' END AS status,
				CASE WHEN c.contype = 'c' THEN pg_get_constraintdef(c.oid) ELSE '' END AS check_expression
			FROM pg_constraint c
			JOIN pg_class cl ON cl.oid = c.conrelid
			JOIN pg_namespace n ON n.oid = cl.relnamespace
			LEFT JOIN pg_class confrel ON confrel.oid = c.confrelid
			LEFT JOIN pg_namespace confn ON confn.oid = confrel.relnamespace
			WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
			  AND ($1 = '' OR LOWER(n.nspname) = LOWER($1))
			  AND ($2 = '' OR LOWER(cl.relname) = LOWER($2))
			ORDER BY n.nspname, cl.relname, c.conname`
		p1 = owner
		p2 = table
	}

	rows, err := db.QueryContext(ctx, consQuery, p1, p2)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar constraints no postgres: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var o, t, conName, conType, cols, refO, refT, refCols, onDel, onUpd, status, chkExp string
		if err := rows.Scan(&o, &t, &conName, &conType, &cols, &refO, &refT, &refCols, &onDel, &onUpd, &status, &chkExp); err != nil {
			return nil, err
		}

		mappedType := ""
		switch conType {
		case "p":
			mappedType = "PK"
		case "f":
			mappedType = "FK"
		case "u":
			mappedType = "UNIQUE"
		case "c":
			mappedType = "CHECK"
			chkExp = cleanCheckClause(chkExp)
		}

		if opts.ConstraintType != "" && mappedType != opts.ConstraintType {
			continue
		}

		results = append(results, ConstraintResult{
			Owner:           o,
			TableName:       t,
			ConstraintName:  conName,
			ConstraintType:  mappedType,
			Columns:         cols,
			RefOwner:        refO,
			RefTable:        refT,
			RefColumns:      refCols,
			OnDelete:        onDel,
			OnUpdate:        onUpd,
			Status:          status,
			CheckExpression: chkExp,
		})
	}

	// 2. DEFAULT constraints via pg_attrdef (apenas direction "own")
	if opts.Direction == "own" && (opts.ConstraintType == "" || opts.ConstraintType == "DEFAULT") {
		defQuery := `
			SELECT
				n.nspname AS owner,
				cl.relname AS table_name,
				'df_' || cl.relname || '_' || a.attname AS constraint_name,
				a.attname AS columns,
				pg_get_expr(ad.adbin, ad.adrelid) AS check_expression
			FROM pg_attrdef ad
			JOIN pg_attribute a ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
			JOIN pg_class cl ON cl.oid = a.attrelid
			JOIN pg_namespace n ON n.oid = cl.relnamespace
			WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
			  AND a.attnum > 0
			  AND NOT a.attisdropped
			  AND ($1 = '' OR LOWER(n.nspname) = LOWER($1))
			  AND ($2 = '' OR LOWER(cl.relname) = LOWER($2))
			ORDER BY n.nspname, cl.relname, a.attnum`

		if defRows, err := db.QueryContext(ctx, defQuery, owner, table); err == nil {
			defer defRows.Close()
			for defRows.Next() {
				var o, t, conName, col, chkExp string
				if err := defRows.Scan(&o, &t, &conName, &col, &chkExp); err == nil {
					results = append(results, ConstraintResult{
						Owner:           o,
						TableName:       t,
						ConstraintName:  conName,
						ConstraintType:  "DEFAULT",
						Columns:         col,
						Status:          "ENABLED",
						CheckExpression: cleanDefaultValue(chkExp),
					})
				}
			}
		}
	}

	return results, nil
}

// --- MYSQL ---

func (e *Executor) listConstraintsMysql(ctx context.Context, db *sql.DB, opts ListConstraintsOptions) ([]ConstraintResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	table := strings.TrimSpace(opts.TableName)

	var results []ConstraintResult

	// 1. PK, FK, UNIQUE via TABLE_CONSTRAINTS + KEY_COLUMN_USAGE + REFERENTIAL_CONSTRAINTS
	var consQuery string
	var p1, p2 string

	if opts.Direction == "referenced_by" {
		consQuery = `
			SELECT 
				tc.TABLE_SCHEMA AS owner,
				tc.TABLE_NAME AS table_name,
				tc.CONSTRAINT_NAME AS constraint_name,
				tc.CONSTRAINT_TYPE AS constraint_type,
				kcu.COLUMN_NAME AS column_name,
				COALESCE(kcu.REFERENCED_TABLE_SCHEMA, '') AS ref_owner,
				COALESCE(kcu.REFERENCED_TABLE_NAME, '') AS ref_table,
				COALESCE(kcu.REFERENCED_COLUMN_NAME, '') AS ref_column,
				COALESCE(rc.DELETE_RULE, '') AS on_delete,
				COALESCE(rc.UPDATE_RULE, '') AS on_update
			FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
			JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
			  ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
			 AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA
			 AND tc.TABLE_NAME = kcu.TABLE_NAME
			LEFT JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS rc
			  ON tc.CONSTRAINT_NAME = rc.CONSTRAINT_NAME
			 AND tc.TABLE_SCHEMA = rc.CONSTRAINT_SCHEMA
			 AND tc.TABLE_NAME = rc.TABLE_NAME
			WHERE tc.CONSTRAINT_TYPE = 'FOREIGN KEY'
			  AND kcu.REFERENCED_TABLE_SCHEMA = ?
			  AND LOWER(kcu.REFERENCED_TABLE_NAME) = LOWER(?)
			ORDER BY tc.TABLE_SCHEMA, tc.TABLE_NAME, tc.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`
		p1 = owner
		p2 = table
	} else {
		consQuery = `
			SELECT 
				tc.TABLE_SCHEMA AS owner,
				tc.TABLE_NAME AS table_name,
				tc.CONSTRAINT_NAME AS constraint_name,
				tc.CONSTRAINT_TYPE AS constraint_type,
				kcu.COLUMN_NAME AS column_name,
				COALESCE(kcu.REFERENCED_TABLE_SCHEMA, '') AS ref_owner,
				COALESCE(kcu.REFERENCED_TABLE_NAME, '') AS ref_table,
				COALESCE(kcu.REFERENCED_COLUMN_NAME, '') AS ref_column,
				COALESCE(rc.DELETE_RULE, '') AS on_delete,
				COALESCE(rc.UPDATE_RULE, '') AS on_update
			FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
			JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE kcu
			  ON tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
			 AND tc.TABLE_SCHEMA = kcu.TABLE_SCHEMA
			 AND tc.TABLE_NAME = kcu.TABLE_NAME
			LEFT JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS rc
			  ON tc.CONSTRAINT_NAME = rc.CONSTRAINT_NAME
			 AND tc.TABLE_SCHEMA = rc.CONSTRAINT_SCHEMA
			 AND tc.TABLE_NAME = rc.TABLE_NAME
			WHERE tc.TABLE_SCHEMA = ?
			  AND (? = '' OR LOWER(tc.TABLE_NAME) = LOWER(?))
			ORDER BY tc.TABLE_SCHEMA, tc.TABLE_NAME, tc.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`
		p1 = owner
		p2 = table
	}

	if rows, err := db.QueryContext(ctx, consQuery, p1, p2); err == nil {
		defer rows.Close()
		type consGroup struct {
			owner     string
			tableName string
			conName   string
			conType   string
			refOwner  string
			refTable  string
			onDelete  string
			onUpdate  string
			cols      []string
			refCols   []string
		}
		cMap := make(map[string]*consGroup)
		var cOrder []string

		for rows.Next() {
			var o, t, conName, conType, col, refO, refT, refCol, onDel, onUpd string
			if err := rows.Scan(&o, &t, &conName, &conType, &col, &refO, &refT, &refCol, &onDel, &onUpd); err == nil {
				id := o + "." + conName
				item, exists := cMap[id]
				if !exists {
					mappedType := conType
					switch conType {
					case "PRIMARY KEY":
						mappedType = "PK"
					case "FOREIGN KEY":
						mappedType = "FK"
					}
					item = &consGroup{
						owner:     o,
						tableName: t,
						conName:   conName,
						conType:   mappedType,
						refOwner:  refO,
						refTable:  refT,
						onDelete:  onDel,
						onUpdate:  onUpd,
					}
					cMap[id] = item
					cOrder = append(cOrder, id)
				}
				item.cols = append(item.cols, col)
				if refCol != "" {
					item.refCols = append(item.refCols, refCol)
				}
			}
		}

		for _, id := range cOrder {
			item := cMap[id]
			if opts.ConstraintType != "" && item.conType != opts.ConstraintType {
				continue
			}
			results = append(results, ConstraintResult{
				Owner:          item.owner,
				TableName:      item.tableName,
				ConstraintName: item.conName,
				ConstraintType: item.conType,
				Columns:        strings.Join(item.cols, ", "),
				RefOwner:       item.refOwner,
				RefTable:       item.refTable,
				RefColumns:     strings.Join(item.refCols, ", "),
				OnDelete:       item.onDelete,
				OnUpdate:       item.onUpdate,
				Status:         "ENABLED",
			})
		}
	}

	// 2. CHECK constraints (MySQL 8.0.16+)
	// Se a tabela CHECK_CONSTRAINTS não existir em MySQL < 8.0.16, ignora o erro sem falhar
	if opts.Direction == "own" && (opts.ConstraintType == "" || opts.ConstraintType == "CHECK") {
		chkQuery := `
			SELECT 
				tc.TABLE_SCHEMA AS owner,
				tc.TABLE_NAME AS table_name,
				tc.CONSTRAINT_NAME AS constraint_name,
				cc.CHECK_CLAUSE AS check_expression
			FROM INFORMATION_SCHEMA.TABLE_CONSTRAINTS tc
			JOIN INFORMATION_SCHEMA.CHECK_CONSTRAINTS cc
			  ON tc.CONSTRAINT_NAME = cc.CONSTRAINT_NAME
			 AND tc.CONSTRAINT_SCHEMA = cc.CONSTRAINT_SCHEMA
			WHERE tc.CONSTRAINT_TYPE = 'CHECK'
			  AND tc.TABLE_SCHEMA = ?
			  AND (? = '' OR LOWER(tc.TABLE_NAME) = LOWER(?))
			ORDER BY tc.TABLE_SCHEMA, tc.TABLE_NAME, tc.CONSTRAINT_NAME`

		if chkRows, err := db.QueryContext(ctx, chkQuery, owner, table, table); err == nil {
			defer chkRows.Close()
			for chkRows.Next() {
				var o, t, conName, expr string
				if err := chkRows.Scan(&o, &t, &conName, &expr); err == nil {
					results = append(results, ConstraintResult{
						Owner:           o,
						TableName:       t,
						ConstraintName:  conName,
						ConstraintType:  "CHECK",
						Columns:         "",
						Status:          "ENABLED",
						CheckExpression: cleanCheckClause(expr),
					})
				}
			}
		}
	}

	// 3. DEFAULT constraints em INFORMATION_SCHEMA.COLUMNS
	if opts.Direction == "own" && (opts.ConstraintType == "" || opts.ConstraintType == "DEFAULT") {
		defQuery := `
			SELECT
				TABLE_SCHEMA AS owner,
				TABLE_NAME AS table_name,
				CONCAT('df_', TABLE_NAME, '_', COLUMN_NAME) AS constraint_name,
				COLUMN_NAME AS columns,
				COLUMN_DEFAULT AS check_expression
			FROM INFORMATION_SCHEMA.COLUMNS
			WHERE TABLE_SCHEMA = ?
			  AND (? = '' OR LOWER(TABLE_NAME) = LOWER(?))
			  AND COLUMN_DEFAULT IS NOT NULL
			ORDER BY TABLE_SCHEMA, TABLE_NAME, ORDINAL_POSITION`

		if defRows, err := db.QueryContext(ctx, defQuery, owner, table, table); err == nil {
			defer defRows.Close()
			for defRows.Next() {
				var o, t, conName, col string
				var defVal sql.NullString
				if err := defRows.Scan(&o, &t, &conName, &col, &defVal); err == nil && defVal.Valid {
					results = append(results, ConstraintResult{
						Owner:           o,
						TableName:       t,
						ConstraintName:  conName,
						ConstraintType:  "DEFAULT",
						Columns:         col,
						Status:          "ENABLED",
						CheckExpression: cleanDefaultValue(defVal.String),
					})
				}
			}
		}
	}

	return results, nil
}
