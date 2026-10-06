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

type RoutineResult struct {
	Owner         string `json:"owner"`
	RoutineName   string `json:"routine_name"`
	RoutineType   string `json:"routine_type"` // PROCEDURE, FUNCTION, TRIGGER, PACKAGE
	Language      string `json:"language"`
	Status        string `json:"status"` // VALID, INVALID, ENABLED, DISABLED
	Created       string `json:"created"`
	LastModified  string `json:"last_modified"`
	ParentTable   string `json:"parent_table"`
	TriggerEvent  string `json:"trigger_event"`
	TriggerTiming string `json:"trigger_timing"`
}

var RoutineColumns = []string{
	"owner",
	"routine_name",
	"routine_type",
	"language",
	"status",
	"created",
	"last_modified",
	"parent_table",
	"trigger_event",
	"trigger_timing",
}

func (r RoutineResult) ToMap() map[string]interface{} {
	return map[string]interface{}{
		"owner":          r.Owner,
		"routine_name":   r.RoutineName,
		"routine_type":   r.RoutineType,
		"language":       r.Language,
		"status":         r.Status,
		"created":        r.Created,
		"last_modified":  r.LastModified,
		"parent_table":   r.ParentTable,
		"trigger_event":  r.TriggerEvent,
		"trigger_timing": r.TriggerTiming,
	}
}

type RoutineSourceResult struct {
	Owner       string `json:"owner"`
	RoutineName string `json:"routine_name"`
	RoutineType string `json:"routine_type"`
	Source      string `json:"source"`
}

type ListRoutinesOptions struct {
	Owner       string
	RoutineType string // PROCEDURE, FUNCTION, TRIGGER, PACKAGE
	NameLike    string
	TableName   string // Apenas para TRIGGER
}

type GetRoutineSourceOptions struct {
	Owner       string
	RoutineName string
	RoutineType string
}

func (e *Executor) ListRoutines(db *sql.DB, dbType string, opts ListRoutinesOptions) ([]RoutineResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := security.ValidateIdentifier(opts.Owner); err != nil {
		return nil, err
	}
	if err := security.ValidateIdentifier(opts.TableName); err != nil {
		return nil, err
	}
	if err := security.ValidateLikePattern(opts.NameLike); err != nil {
		return nil, err
	}

	opts.RoutineType = strings.ToUpper(strings.TrimSpace(opts.RoutineType))
	if opts.RoutineType != "" {
		validTypes := map[string]bool{
			"PROCEDURE": true,
			"FUNCTION":  true,
			"TRIGGER":   true,
			"PACKAGE":   true,
		}
		if !validTypes[opts.RoutineType] {
			return nil, fmt.Errorf("tipo de rotina '%s' inválido. Valores aceitos: PROCEDURE, FUNCTION, TRIGGER, PACKAGE", opts.RoutineType)
		}
	}

	// PACKAGE só existe no Oracle. Nos outros 3 bancos, retorna lista vazia sem erro
	dbTypeLower := strings.ToLower(dbType)
	if opts.RoutineType == "PACKAGE" && dbTypeLower != "oracle" {
		return []RoutineResult{}, nil
	}

	// Se table_name foi informado e routine_type não for TRIGGER (ex: PROCEDURE), retorna vazio
	if opts.TableName != "" && opts.RoutineType != "" && opts.RoutineType != "TRIGGER" {
		return []RoutineResult{}, nil
	}

	var results []RoutineResult
	var err error

	switch dbTypeLower {
	case "oracle":
		results, err = e.listRoutinesOracle(ctx, db, opts)
	case "sqlserver":
		results, err = e.listRoutinesSqlServer(ctx, db, opts)
	case "postgres":
		results, err = e.listRoutinesPostgres(ctx, db, opts)
	case "mysql":
		results, err = e.listRoutinesMysql(ctx, db, opts)
	default:
		return nil, fmt.Errorf("tipo de banco '%s' não suportado", dbType)
	}

	if err != nil {
		return nil, err
	}

	// Ordenação determinística: owner, routine_type, routine_name
	sort.Slice(results, func(i, j int) bool {
		if results[i].Owner != results[j].Owner {
			return results[i].Owner < results[j].Owner
		}
		if results[i].RoutineType != results[j].RoutineType {
			return results[i].RoutineType < results[j].RoutineType
		}
		return results[i].RoutineName < results[j].RoutineName
	})

	return results, nil
}

func (e *Executor) GetRoutineSource(db *sql.DB, dbType string, opts GetRoutineSourceOptions) (*RoutineSourceResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := security.ValidateIdentifier(opts.Owner); err != nil {
		return nil, err
	}
	if err := security.ValidateIdentifier(opts.RoutineName); err != nil {
		return nil, err
	}

	if strings.TrimSpace(opts.RoutineName) == "" {
		return nil, fmt.Errorf("o parâmetro 'routine_name' é obrigatório")
	}

	opts.RoutineType = strings.ToUpper(strings.TrimSpace(opts.RoutineType))

	switch strings.ToLower(dbType) {
	case "oracle":
		return e.getRoutineSourceOracle(ctx, db, opts)
	case "sqlserver":
		return e.getRoutineSourceSqlServer(ctx, db, opts)
	case "postgres":
		return e.getRoutineSourcePostgres(ctx, db, opts)
	case "mysql":
		return e.getRoutineSourceMysql(ctx, db, opts)
	default:
		return nil, fmt.Errorf("tipo de banco '%s' não suportado", dbType)
	}
}

// --- ORACLE ---

func (e *Executor) listRoutinesOracle(ctx context.Context, db *sql.DB, opts ListRoutinesOptions) ([]RoutineResult, error) {
	owner := strings.ToUpper(strings.TrimSpace(opts.Owner))
	var ownerParam interface{} = owner
	if owner == "" {
		ownerParam = nil
	}

	var results []RoutineResult

	// 1. Procedures, Functions, Packages (se table_name não foi exigido)
	if opts.TableName == "" && (opts.RoutineType == "" || opts.RoutineType != "TRIGGER") {
		var typeFilter interface{} = nil
		if opts.RoutineType != "" {
			typeFilter = opts.RoutineType
		}

		var nameParam interface{} = nil
		if opts.NameLike != "" {
			nameParam = strings.ToUpper(opts.NameLike)
		}

		query := `
			SELECT 
				o.owner,
				o.object_name AS routine_name,
				o.object_type AS routine_type,
				'PL/SQL' AS language,
				o.status,
				COALESCE(TO_CHAR(o.created, 'YYYY-MM-DD HH24:MI:SS'), '') AS created,
				COALESCE(TO_CHAR(o.last_ddl_time, 'YYYY-MM-DD HH24:MI:SS'), '') AS last_modified
			FROM all_objects o
			WHERE (:1 IS NULL OR UPPER(o.owner) = UPPER(:1))
			  AND o.object_type IN ('PROCEDURE', 'FUNCTION', 'PACKAGE')
			  AND (:2 IS NULL OR UPPER(o.object_type) = UPPER(:2))
			  AND (:3 IS NULL OR UPPER(o.object_name) LIKE UPPER(:3))
			ORDER BY o.owner, o.object_type, o.object_name`

		rows, err := db.QueryContext(ctx, query, ownerParam, typeFilter, nameParam)
		if err != nil {
			return nil, fmt.Errorf("erro ao consultar rotinas no oracle: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var o, name, rType, lang, status, created, modified string
			if err := rows.Scan(&o, &name, &rType, &lang, &status, &created, &modified); err == nil {
				results = append(results, RoutineResult{
					Owner:        o,
					RoutineName:  name,
					RoutineType:  rType,
					Language:     lang,
					Status:       status,
					Created:      created,
					LastModified: modified,
				})
			}
		}
	}

	// 2. Triggers
	if opts.RoutineType == "" || opts.RoutineType == "TRIGGER" {
		var nameParam interface{} = nil
		if opts.NameLike != "" {
			nameParam = strings.ToUpper(opts.NameLike)
		}
		var tableParam interface{} = nil
		if opts.TableName != "" {
			tableParam = strings.ToUpper(opts.TableName)
		}

		trigQuery := `
			SELECT 
				t.owner,
				t.trigger_name AS routine_name,
				'TRIGGER' AS routine_type,
				'PL/SQL' AS language,
				t.status,
				COALESCE(TO_CHAR(o.created, 'YYYY-MM-DD HH24:MI:SS'), '') AS created,
				COALESCE(TO_CHAR(o.last_ddl_time, 'YYYY-MM-DD HH24:MI:SS'), '') AS last_modified,
				t.table_name AS parent_table,
				COALESCE(t.triggering_event, '') AS trigger_event,
				CASE 
					WHEN t.trigger_type LIKE 'BEFORE%' THEN 'BEFORE'
					WHEN t.trigger_type LIKE 'AFTER%' THEN 'AFTER'
					WHEN t.trigger_type LIKE 'INSTEAD OF%' THEN 'INSTEAD OF'
					ELSE t.trigger_type
				END AS trigger_timing
			FROM all_triggers t
			LEFT JOIN all_objects o 
				ON t.owner = o.owner AND t.trigger_name = o.object_name AND o.object_type = 'TRIGGER'
			WHERE (:1 IS NULL OR UPPER(t.owner) = UPPER(:1))
			  AND (:2 IS NULL OR UPPER(t.trigger_name) LIKE UPPER(:2))
			  AND (:3 IS NULL OR UPPER(t.table_name) = UPPER(:3))
			ORDER BY t.owner, t.trigger_name`

		rows, err := db.QueryContext(ctx, trigQuery, ownerParam, nameParam, tableParam)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var o, name, rType, lang, status, created, modified, parentTable, trigEvent, trigTiming string
				if err := rows.Scan(&o, &name, &rType, &lang, &status, &created, &modified, &parentTable, &trigEvent, &trigTiming); err == nil {
					results = append(results, RoutineResult{
						Owner:         o,
						RoutineName:   name,
						RoutineType:   rType,
						Language:      lang,
						Status:        status,
						Created:       created,
						LastModified:  modified,
						ParentTable:   parentTable,
						TriggerEvent:  strings.TrimSpace(trigEvent),
						TriggerTiming: strings.TrimSpace(trigTiming),
					})
				}
			}
		}
	}

	return results, nil
}

func (e *Executor) getRoutineSourceOracle(ctx context.Context, db *sql.DB, opts GetRoutineSourceOptions) (*RoutineSourceResult, error) {
	owner := strings.ToUpper(strings.TrimSpace(opts.Owner))
	name := strings.ToUpper(strings.TrimSpace(opts.RoutineName))
	var ownerParam interface{} = owner
	if owner == "" {
		ownerParam = nil
	}

	// 1. Verificar se o objeto existe e qual o tipo
	typeCheckQuery := `
		SELECT owner, object_name, object_type
		FROM all_objects
		WHERE (:1 IS NULL OR UPPER(owner) = UPPER(:1))
		  AND UPPER(object_name) = UPPER(:2)
		  AND object_type IN ('PROCEDURE', 'FUNCTION', 'PACKAGE', 'TRIGGER')
		ORDER BY CASE WHEN object_type = 'PACKAGE' THEN 1 ELSE 2 END`

	rows, err := db.QueryContext(ctx, typeCheckQuery, ownerParam, name)
	if err != nil {
		return nil, fmt.Errorf("erro ao verificar rotina no oracle: %w", err)
	}
	defer rows.Close()

	var actualOwner, actualName, actualType string
	found := false
	for rows.Next() {
		var o, n, t string
		if err := rows.Scan(&o, &n, &t); err == nil {
			if !found {
				actualOwner = o
				actualName = n
				actualType = t
				found = true
			}
			if opts.RoutineType != "" && strings.EqualFold(t, opts.RoutineType) {
				actualOwner = o
				actualName = n
				actualType = t
				found = true
				break
			}
		}
	}

	if !found {
		return nil, fmt.Errorf("rotina '%s' não encontrada", opts.RoutineName)
	}

	// 2. Buscar linhas do código-fonte em all_source
	srcQuery := `
		SELECT type, line, text
		FROM all_source
		WHERE owner = :1
		  AND name = :2
		ORDER BY CASE WHEN type = 'PACKAGE' THEN 1 WHEN type = 'PACKAGE BODY' THEN 2 ELSE 3 END, line`

	srcRows, err := db.QueryContext(ctx, srcQuery, actualOwner, actualName)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler fonte da rotina no oracle: %w", err)
	}
	defer srcRows.Close()

	var specLines []string
	var bodyLines []string
	var generalLines []string
	isWrapped := false

	for srcRows.Next() {
		var sType, text string
		var lineNum int
		if err := srcRows.Scan(&sType, &lineNum, &text); err == nil {
			trimmed := strings.TrimSpace(text)
			if lineNum <= 2 && strings.Contains(strings.ToLower(trimmed), "wrapped") {
				isWrapped = true
			}
			if sType == "PACKAGE" {
				specLines = append(specLines, text)
			} else if sType == "PACKAGE BODY" {
				bodyLines = append(bodyLines, text)
			} else {
				generalLines = append(generalLines, text)
			}
		}
	}

	if isWrapped {
		return nil, fmt.Errorf("fonte não disponível — objeto está compilado em formato WRAPPED")
	}

	var fullSource string
	if actualType == "PACKAGE" {
		if len(specLines) == 0 && len(bodyLines) == 0 {
			return nil, fmt.Errorf("fonte não disponível — sem privilégios de leitura em ALL_SOURCE")
		}
		var sb strings.Builder
		if len(specLines) > 0 {
			sb.WriteString("/* --- PACKAGE SPECIFICATION --- */\n")
			for _, l := range specLines {
				sb.WriteString(l)
			}
		}
		if len(bodyLines) > 0 {
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString("/* --- PACKAGE BODY --- */\n")
			for _, l := range bodyLines {
				sb.WriteString(l)
			}
		}
		fullSource = sb.String()
	} else {
		if len(generalLines) == 0 {
			return nil, fmt.Errorf("fonte não disponível — sem privilégios de leitura em ALL_SOURCE")
		}
		var sb strings.Builder
		for _, l := range generalLines {
			sb.WriteString(l)
		}
		fullSource = sb.String()
	}

	fullSource = strings.TrimSpace(fullSource)
	if fullSource == "" {
		return nil, fmt.Errorf("fonte não disponível — conteúdo retornado vazio")
	}

	return &RoutineSourceResult{
		Owner:       actualOwner,
		RoutineName: actualName,
		RoutineType: actualType,
		Source:      fullSource,
	}, nil
}

// --- SQL SERVER ---

func (e *Executor) listRoutinesSqlServer(ctx context.Context, db *sql.DB, opts ListRoutinesOptions) ([]RoutineResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	var results []RoutineResult

	nameLike := ""
	if opts.NameLike != "" {
		nameLike = opts.NameLike
	}

	// 1. Procedures e Functions
	if opts.TableName == "" && (opts.RoutineType == "" || opts.RoutineType != "TRIGGER") {
		procQuery := `
			SELECT 
				s.name AS owner,
				o.name AS routine_name,
				CASE 
					WHEN o.type = 'P' THEN 'PROCEDURE'
					WHEN o.type IN ('FN', 'IF', 'TF') THEN 'FUNCTION'
				END AS routine_type,
				'TSQL' AS language,
				'VALID' AS status,
				CONVERT(VARCHAR(19), o.create_date, 120) AS created,
				CONVERT(VARCHAR(19), o.modify_date, 120) AS last_modified
			FROM sys.objects o
			JOIN sys.schemas s ON o.schema_id = s.schema_id
			WHERE o.type IN ('P', 'FN', 'IF', 'TF')
			  AND (@p1 = '' OR LOWER(s.name) = LOWER(@p1))
			  AND (@p2 = '' OR LOWER(o.name) LIKE LOWER(@p2))
			  AND (@p3 = '' OR 
				  (@p3 = 'PROCEDURE' AND o.type = 'P') OR 
				  (@p3 = 'FUNCTION' AND o.type IN ('FN', 'IF', 'TF'))
			  )
			ORDER BY s.name, o.name`

		typeFilter := ""
		if opts.RoutineType == "PROCEDURE" || opts.RoutineType == "FUNCTION" {
			typeFilter = opts.RoutineType
		}

		if rows, err := db.QueryContext(ctx, procQuery, owner, nameLike, typeFilter); err == nil {
			defer rows.Close()
			for rows.Next() {
				var o, name, rType, lang, status, created, modified string
				if err := rows.Scan(&o, &name, &rType, &lang, &status, &created, &modified); err == nil {
					results = append(results, RoutineResult{
						Owner:        o,
						RoutineName:  name,
						RoutineType:  rType,
						Language:     lang,
						Status:       status,
						Created:      created,
						LastModified: modified,
					})
				}
			}
		}
	}

	// 2. Triggers
	if opts.RoutineType == "" || opts.RoutineType == "TRIGGER" {
		trigQuery := `
			SELECT 
				s.name AS owner,
				tr.name AS routine_name,
				'TRIGGER' AS routine_type,
				'TSQL' AS language,
				CASE WHEN tr.is_disabled = 1 THEN 'DISABLED' ELSE 'ENABLED' END AS status,
				CONVERT(VARCHAR(19), o.create_date, 120) AS created,
				CONVERT(VARCHAR(19), o.modify_date, 120) AS last_modified,
				t.name AS parent_table,
				SUBSTRING(
					CASE WHEN OBJECTPROPERTY(tr.object_id, 'ExecIsInsertTrigger') = 1 THEN ' / INSERT' ELSE '' END +
					CASE WHEN OBJECTPROPERTY(tr.object_id, 'ExecIsUpdateTrigger') = 1 THEN ' / UPDATE' ELSE '' END +
					CASE WHEN OBJECTPROPERTY(tr.object_id, 'ExecIsDeleteTrigger') = 1 THEN ' / DELETE' ELSE '' END,
					4, 100
				) AS trigger_event,
				CASE WHEN tr.is_instead_of_trigger = 1 THEN 'INSTEAD OF' ELSE 'AFTER' END AS trigger_timing
			FROM sys.triggers tr
			JOIN sys.objects o ON tr.object_id = o.object_id
			JOIN sys.tables t ON tr.parent_id = t.object_id
			JOIN sys.schemas s ON t.schema_id = s.schema_id
			WHERE (@p1 = '' OR LOWER(s.name) = LOWER(@p1))
			  AND (@p2 = '' OR LOWER(tr.name) LIKE LOWER(@p2))
			  AND (@p3 = '' OR LOWER(t.name) = LOWER(@p3))
			ORDER BY s.name, tr.name`

		tableName := strings.TrimSpace(opts.TableName)
		if rows, err := db.QueryContext(ctx, trigQuery, owner, nameLike, tableName); err == nil {
			defer rows.Close()
			for rows.Next() {
				var o, name, rType, lang, status, created, modified, parentTable, trigEvent, trigTiming string
				if err := rows.Scan(&o, &name, &rType, &lang, &status, &created, &modified, &parentTable, &trigEvent, &trigTiming); err == nil {
					results = append(results, RoutineResult{
						Owner:         o,
						RoutineName:   name,
						RoutineType:   rType,
						Language:      lang,
						Status:        status,
						Created:       created,
						LastModified:  modified,
						ParentTable:   parentTable,
						TriggerEvent:  strings.TrimSpace(trigEvent),
						TriggerTiming: strings.TrimSpace(trigTiming),
					})
				}
			}
		}
	}

	return results, nil
}

func (e *Executor) getRoutineSourceSqlServer(ctx context.Context, db *sql.DB, opts GetRoutineSourceOptions) (*RoutineSourceResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	name := strings.TrimSpace(opts.RoutineName)

	query := `
		SELECT 
			s.name AS owner,
			o.name AS routine_name,
			CASE 
				WHEN o.type = 'P' THEN 'PROCEDURE'
				WHEN o.type IN ('FN', 'IF', 'TF') THEN 'FUNCTION'
				WHEN o.type = 'TR' THEN 'TRIGGER'
				ELSE o.type
			END AS routine_type,
			m.definition,
			COALESCE(OBJECTPROPERTY(o.object_id, 'IsEncrypted'), 0) AS is_encrypted
		FROM sys.objects o
		JOIN sys.schemas s ON o.schema_id = s.schema_id
		LEFT JOIN sys.sql_modules m ON o.object_id = m.object_id
		WHERE (@p1 = '' OR LOWER(s.name) = LOWER(@p1))
		  AND LOWER(o.name) = LOWER(@p2)
		  AND o.type IN ('P', 'FN', 'IF', 'TF', 'TR')`

	rows, err := db.QueryContext(ctx, query, owner, name)
	if err != nil {
		return nil, fmt.Errorf("erro ao consultar fonte no sql server: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("rotina '%s' não encontrada", opts.RoutineName)
	}

	var actualOwner, actualName, actualType string
	var definition sql.NullString
	var isEncrypted int

	if err := rows.Scan(&actualOwner, &actualName, &actualType, &definition, &isEncrypted); err != nil {
		return nil, err
	}

	if isEncrypted == 1 {
		return nil, fmt.Errorf("fonte não disponível — objeto protegido com criptografia (WITH ENCRYPTION)")
	}

	if !definition.Valid || strings.TrimSpace(definition.String) == "" {
		return nil, fmt.Errorf("fonte não disponível — definição vazia ou sem permissão de visualização")
	}

	return &RoutineSourceResult{
		Owner:       actualOwner,
		RoutineName: actualName,
		RoutineType: actualType,
		Source:      strings.TrimSpace(definition.String),
	}, nil
}

// --- POSTGRESQL ---

func (e *Executor) listRoutinesPostgres(ctx context.Context, db *sql.DB, opts ListRoutinesOptions) ([]RoutineResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	nameLike := ""
	if opts.NameLike != "" {
		nameLike = opts.NameLike
	}

	var results []RoutineResult

	// 1. Funções e Procedures
	if opts.TableName == "" && (opts.RoutineType == "" || opts.RoutineType != "TRIGGER") {
		procQuery := `
			SELECT 
				n.nspname AS owner,
				p.proname AS routine_name,
				CASE WHEN p.prokind = 'p' THEN 'PROCEDURE' ELSE 'FUNCTION' END AS routine_type,
				l.lanname AS language,
				'VALID' AS status
			FROM pg_proc p
			JOIN pg_namespace n ON n.oid = p.pronamespace
			JOIN pg_language l ON l.oid = p.prolang
			WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
			  AND p.prokind IN ('f', 'p')
			  AND ($1 = '' OR LOWER(n.nspname) = LOWER($1))
			  AND ($2 = '' OR LOWER(p.proname) LIKE LOWER($2))
			  AND ($3 = '' OR 
				  ($3 = 'PROCEDURE' AND p.prokind = 'p') OR
				  ($3 = 'FUNCTION' AND p.prokind = 'f')
			  )
			ORDER BY n.nspname, p.proname`

		typeFilter := ""
		if opts.RoutineType == "PROCEDURE" || opts.RoutineType == "FUNCTION" {
			typeFilter = opts.RoutineType
		}

		if rows, err := db.QueryContext(ctx, procQuery, owner, nameLike, typeFilter); err == nil {
			defer rows.Close()
			for rows.Next() {
				var o, name, rType, lang, status string
				if err := rows.Scan(&o, &name, &rType, &lang, &status); err == nil {
					results = append(results, RoutineResult{
						Owner:       o,
						RoutineName: name,
						RoutineType: rType,
						Language:    lang,
						Status:      status,
					})
				}
			}
		}
	}

	// 2. Triggers (CRITICAL: NOT tr.tgisinternal para não trazer triggers internas de FK)
	if opts.RoutineType == "" || opts.RoutineType == "TRIGGER" {
		trigQuery := `
			SELECT 
				n.nspname AS owner,
				tr.tgname AS routine_name,
				'TRIGGER' AS routine_type,
				'PLPGSQL' AS language,
				CASE WHEN tr.tgenabled = 'D' THEN 'DISABLED' ELSE 'ENABLED' END AS status,
				c.relname AS parent_table,
				SUBSTRING(
					(CASE WHEN (tr.tgtype & 4) = 4 THEN ' / INSERT' ELSE '' END) ||
					(CASE WHEN (tr.tgtype & 16) = 16 THEN ' / UPDATE' ELSE '' END) ||
					(CASE WHEN (tr.tgtype & 8) = 8 THEN ' / DELETE' ELSE '' END) ||
					(CASE WHEN (tr.tgtype & 32) = 32 THEN ' / TRUNCATE' ELSE '' END),
					4
				) AS trigger_event,
				CASE 
					WHEN (tr.tgtype & 2) = 2 THEN 'BEFORE'
					WHEN (tr.tgtype & 64) = 64 THEN 'INSTEAD OF'
					ELSE 'AFTER'
				END AS trigger_timing
			FROM pg_trigger tr
			JOIN pg_class c ON tr.tgrelid = c.oid
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE NOT tr.tgisinternal
			  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
			  AND ($1 = '' OR LOWER(n.nspname) = LOWER($1))
			  AND ($2 = '' OR LOWER(tr.tgname) LIKE LOWER($2))
			  AND ($3 = '' OR LOWER(c.relname) = LOWER($3))
			ORDER BY n.nspname, tr.tgname`

		tableName := strings.TrimSpace(opts.TableName)
		if rows, err := db.QueryContext(ctx, trigQuery, owner, nameLike, tableName); err == nil {
			defer rows.Close()
			for rows.Next() {
				var o, name, rType, lang, status, parentTable, trigEvent, trigTiming string
				if err := rows.Scan(&o, &name, &rType, &lang, &status, &parentTable, &trigEvent, &trigTiming); err == nil {
					results = append(results, RoutineResult{
						Owner:         o,
						RoutineName:   name,
						RoutineType:   rType,
						Language:      lang,
						Status:        status,
						ParentTable:   parentTable,
						TriggerEvent:  strings.TrimSpace(trigEvent),
						TriggerTiming: strings.TrimSpace(trigTiming),
					})
				}
			}
		}
	}

	return results, nil
}

func (e *Executor) getRoutineSourcePostgres(ctx context.Context, db *sql.DB, opts GetRoutineSourceOptions) (*RoutineSourceResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	name := strings.TrimSpace(opts.RoutineName)

	// 1. Tenta buscar em pg_proc
	procQuery := `
		SELECT 
			n.nspname AS owner,
			p.proname AS routine_name,
			CASE WHEN p.prokind = 'p' THEN 'PROCEDURE' ELSE 'FUNCTION' END AS routine_type,
			pg_get_functiondef(p.oid) AS source
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND ($1 = '' OR LOWER(n.nspname) = LOWER($1))
		  AND LOWER(p.proname) = LOWER($2)`

	if rows, err := db.QueryContext(ctx, procQuery, owner, name); err == nil {
		defer rows.Close()
		if rows.Next() {
			var o, n, rType string
			var src sql.NullString
			if err := rows.Scan(&o, &n, &rType, &src); err == nil {
				if !src.Valid || strings.TrimSpace(src.String) == "" {
					return nil, fmt.Errorf("fonte não disponível — função interna ou sem permissão")
				}
				return &RoutineSourceResult{
					Owner:       o,
					RoutineName: n,
					RoutineType: rType,
					Source:      strings.TrimSpace(src.String),
				}, nil
			}
		}
	}

	// 2. Tenta buscar em pg_trigger
	trigQuery := `
		SELECT 
			n.nspname AS owner,
			tr.tgname AS routine_name,
			'TRIGGER' AS routine_type,
			pg_get_triggerdef(tr.oid, true) AS trigger_def,
			COALESCE(pg_get_functiondef(tr.tgfoid), '') AS func_def
		FROM pg_trigger tr
		JOIN pg_class c ON tr.tgrelid = c.oid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE NOT tr.tgisinternal
		  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
		  AND ($1 = '' OR LOWER(n.nspname) = LOWER($1))
		  AND LOWER(tr.tgname) = LOWER($2)`

	if rows, err := db.QueryContext(ctx, trigQuery, owner, name); err == nil {
		defer rows.Close()
		if rows.Next() {
			var o, n, rType, trigDef, funcDef string
			if err := rows.Scan(&o, &n, &rType, &trigDef, &funcDef); err == nil {
				full := trigDef
				if strings.TrimSpace(funcDef) != "" {
					full += "\n\n/* Função disparada pelo trigger: */\n" + funcDef
				}
				return &RoutineSourceResult{
					Owner:       o,
					RoutineName: n,
					RoutineType: rType,
					Source:      strings.TrimSpace(full),
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("rotina '%s' não encontrada", opts.RoutineName)
}

// --- MYSQL ---

func (e *Executor) listRoutinesMysql(ctx context.Context, db *sql.DB, opts ListRoutinesOptions) ([]RoutineResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	nameLike := ""
	if opts.NameLike != "" {
		nameLike = opts.NameLike
	}

	var results []RoutineResult

	// 1. Procedures e Functions em INFORMATION_SCHEMA.ROUTINES
	if opts.TableName == "" && (opts.RoutineType == "" || opts.RoutineType != "TRIGGER") {
		query := `
			SELECT 
				ROUTINE_SCHEMA AS owner,
				ROUTINE_NAME AS routine_name,
				ROUTINE_TYPE AS routine_type,
				COALESCE(ROUTINE_BODY, 'SQL') AS language,
				'VALID' AS status,
				DATE_FORMAT(CREATED, '%Y-%m-%d %H:%i:%s') AS created,
				DATE_FORMAT(LAST_ALTERED, '%Y-%m-%d %H:%i:%s') AS last_modified
			FROM INFORMATION_SCHEMA.ROUTINES
			WHERE ROUTINE_SCHEMA = ?
			  AND (? = '' OR LOWER(ROUTINE_NAME) LIKE LOWER(?))
			  AND (? = '' OR ROUTINE_TYPE = ?)
			ORDER BY ROUTINE_SCHEMA, ROUTINE_NAME`

		typeFilter := ""
		if opts.RoutineType == "PROCEDURE" || opts.RoutineType == "FUNCTION" {
			typeFilter = opts.RoutineType
		}

		if rows, err := db.QueryContext(ctx, query, owner, nameLike, nameLike, typeFilter, typeFilter); err == nil {
			defer rows.Close()
			for rows.Next() {
				var o, name, rType, lang, status, created, modified string
				if err := rows.Scan(&o, &name, &rType, &lang, &status, &created, &modified); err == nil {
					results = append(results, RoutineResult{
						Owner:        o,
						RoutineName:  name,
						RoutineType:  rType,
						Language:     lang,
						Status:       status,
						Created:      created,
						LastModified: modified,
					})
				}
			}
		}
	}

	// 2. Triggers em INFORMATION_SCHEMA.TRIGGERS
	if opts.RoutineType == "" || opts.RoutineType == "TRIGGER" {
		tableName := strings.TrimSpace(opts.TableName)
		trigQuery := `
			SELECT 
				TRIGGER_SCHEMA AS owner,
				TRIGGER_NAME AS routine_name,
				'TRIGGER' AS routine_type,
				'SQL' AS language,
				'ENABLED' AS status,
				COALESCE(DATE_FORMAT(CREATED, '%Y-%m-%d %H:%i:%s'), '') AS created,
				COALESCE(DATE_FORMAT(CREATED, '%Y-%m-%d %H:%i:%s'), '') AS last_modified,
				EVENT_OBJECT_TABLE AS parent_table,
				EVENT_MANIPULATION AS trigger_event,
				ACTION_TIMING AS trigger_timing
			FROM INFORMATION_SCHEMA.TRIGGERS
			WHERE TRIGGER_SCHEMA = ?
			  AND (? = '' OR LOWER(TRIGGER_NAME) LIKE LOWER(?))
			  AND (? = '' OR LOWER(EVENT_OBJECT_TABLE) = LOWER(?))
			ORDER BY TRIGGER_SCHEMA, TRIGGER_NAME`

		if rows, err := db.QueryContext(ctx, trigQuery, owner, nameLike, nameLike, tableName, tableName); err == nil {
			defer rows.Close()
			for rows.Next() {
				var o, name, rType, lang, status, created, modified, parentTable, trigEvent, trigTiming string
				if err := rows.Scan(&o, &name, &rType, &lang, &status, &created, &modified, &parentTable, &trigEvent, &trigTiming); err == nil {
					results = append(results, RoutineResult{
						Owner:         o,
						RoutineName:   name,
						RoutineType:   rType,
						Language:      lang,
						Status:        status,
						Created:       created,
						LastModified:  modified,
						ParentTable:   parentTable,
						TriggerEvent:  strings.TrimSpace(trigEvent),
						TriggerTiming: strings.TrimSpace(trigTiming),
					})
				}
			}
		}
	}

	return results, nil
}

func (e *Executor) getRoutineSourceMysql(ctx context.Context, db *sql.DB, opts GetRoutineSourceOptions) (*RoutineSourceResult, error) {
	owner := strings.TrimSpace(opts.Owner)
	name := strings.TrimSpace(opts.RoutineName)

	// 1. Tenta localizar na tabela ROUTINES
	checkQuery := `
		SELECT ROUTINE_SCHEMA, ROUTINE_NAME, ROUTINE_TYPE, ROUTINE_DEFINITION
		FROM INFORMATION_SCHEMA.ROUTINES
		WHERE ROUTINE_SCHEMA = ?
		  AND LOWER(ROUTINE_NAME) = LOWER(?)`

	if rows, err := db.QueryContext(ctx, checkQuery, owner, name); err == nil {
		defer rows.Close()
		if rows.Next() {
			var o, n, rType string
			var def sql.NullString
			if err := rows.Scan(&o, &n, &rType, &def); err == nil {
				// Tenta obter via SHOW CREATE para ter o cabeçalho completo
				var showStmt string
				if rType == "PROCEDURE" {
					showStmt = fmt.Sprintf("SHOW CREATE PROCEDURE `%s`.`%s`", o, n)
				} else {
					showStmt = fmt.Sprintf("SHOW CREATE FUNCTION `%s`.`%s`", o, n)
				}

				if showRows, err := db.QueryContext(ctx, showStmt); err == nil {
					defer showRows.Close()
					if showRows.Next() {
						var dummyName, dummyMode, createStmt string
						if err := showRows.Scan(&dummyName, &dummyMode, &createStmt); err == nil && strings.TrimSpace(createStmt) != "" {
							return &RoutineSourceResult{
								Owner:       o,
								RoutineName: n,
								RoutineType: rType,
								Source:      strings.TrimSpace(createStmt),
							}, nil
						}
					}
				}

				// Fallback para ROUTINE_DEFINITION
				if def.Valid && strings.TrimSpace(def.String) != "" {
					return &RoutineSourceResult{
						Owner:       o,
						RoutineName: n,
						RoutineType: rType,
						Source:      strings.TrimSpace(def.String),
					}, nil
				}

				return nil, fmt.Errorf("fonte não disponível — permissão insuficiente ou corpo não retornado")
			}
		}
	}

	// 2. Tenta localizar em TRIGGERS
	trigCheckQuery := `
		SELECT TRIGGER_SCHEMA, TRIGGER_NAME, ACTION_STATEMENT
		FROM INFORMATION_SCHEMA.TRIGGERS
		WHERE TRIGGER_SCHEMA = ?
		  AND LOWER(TRIGGER_NAME) = LOWER(?)`

	if tRows, err := db.QueryContext(ctx, trigCheckQuery, owner, name); err == nil {
		defer tRows.Close()
		if tRows.Next() {
			var o, n string
			var actStmt sql.NullString
			if err := tRows.Scan(&o, &n, &actStmt); err == nil {
				showStmt := fmt.Sprintf("SHOW CREATE TRIGGER `%s`.`%s`", o, n)
				if showRows, err := db.QueryContext(ctx, showStmt); err == nil {
					defer showRows.Close()
					if showRows.Next() {
						var createStmt string
						// SHOW CREATE TRIGGER retorna: Trigger, sql_mode, SQL Original Statement, ...
						// Pode ter até 6 colunas dependendo do MySQL
						cols, _ := showRows.Columns()
						scanArgs := make([]interface{}, len(cols))
						for i := range scanArgs {
							var val sql.NullString
							scanArgs[i] = &val
						}
						if err := showRows.Scan(scanArgs...); err == nil && len(scanArgs) >= 3 {
							if s, ok := scanArgs[2].(*sql.NullString); ok && s.Valid && strings.TrimSpace(s.String) != "" {
								createStmt = s.String
							}
						}
						if strings.TrimSpace(createStmt) != "" {
							return &RoutineSourceResult{
								Owner:       o,
								RoutineName: n,
								RoutineType: "TRIGGER",
								Source:      strings.TrimSpace(createStmt),
							}, nil
						}
					}
				}

				if actStmt.Valid && strings.TrimSpace(actStmt.String) != "" {
					return &RoutineSourceResult{
						Owner:       o,
						RoutineName: n,
						RoutineType: "TRIGGER",
						Source:      strings.TrimSpace(actStmt.String),
					}, nil
				}

				return nil, fmt.Errorf("fonte não disponível — permissão insuficiente ou instrução do trigger vazia")
			}
		}
	}

	return nil, fmt.Errorf("rotina '%s' não encontrada", opts.RoutineName)
}
