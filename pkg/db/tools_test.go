package db

import (
	"fmt"
	"testing"

	"github.com/rogick/db-explorer-mcp/pkg/config"
)

func TestResolveOwner(t *testing.T) {
	tests := []struct {
		name          string
		conn          config.ConnectionDetails
		explicitOwner string
		tableName     string
		expectedOwner string
		expectedTable string
	}{
		{
			name: "Oracle padrão usa usuário em maiúsculo",
			conn: config.ConnectionDetails{
				Type: "oracle",
				User: "app_user",
			},
			explicitOwner: "",
			tableName:     "clientes",
			expectedOwner: "APP_USER",
			expectedTable: "clientes",
		},
		{
			name: "SQL Server padrão usa dbo",
			conn: config.ConnectionDetails{
				Type:     "sqlserver",
				Database: "minha_base",
			},
			explicitOwner: "",
			tableName:     "pedidos",
			expectedOwner: "dbo",
			expectedTable: "pedidos",
		},
		{
			name: "PostgreSQL padrão usa public",
			conn: config.ConnectionDetails{
				Type:     "postgres",
				Database: "postgres",
			},
			explicitOwner: "",
			tableName:     "produtos",
			expectedOwner: "public",
			expectedTable: "produtos",
		},
		{
			name: "MySQL padrão usa database da conexão",
			conn: config.ConnectionDetails{
				Type:     "mysql",
				Database: "loja_virtual",
			},
			explicitOwner: "",
			tableName:     "itens",
			expectedOwner: "loja_virtual",
			expectedTable: "itens",
		},
		{
			name: "Schema explícito em schema.table tem precedência máxima",
			conn: config.ConnectionDetails{
				Type: "postgres",
				User: "user",
			},
			explicitOwner: "public",
			tableName:     "financeiro.contas",
			expectedOwner: "financeiro",
			expectedTable: "contas",
		},
		{
			name: "Owner explícito tem precedência se table_name não tem schema",
			conn: config.ConnectionDetails{
				Type: "oracle",
				User: "default_user",
			},
			explicitOwner: "CUSTOM_SCHEMA",
			tableName:     "tabela",
			expectedOwner: "CUSTOM_SCHEMA",
			expectedTable: "tabela",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOwner, gotTable := ResolveOwner(tt.conn, tt.explicitOwner, tt.tableName)
			if gotOwner != tt.expectedOwner {
				t.Errorf("ResolveOwner() owner = %q, esperado %q", gotOwner, tt.expectedOwner)
			}
			if gotTable != tt.expectedTable {
				t.Errorf("ResolveOwner() table = %q, esperado %q", gotTable, tt.expectedTable)
			}
		})
	}
}

func TestPaginateResult_NoRepeatsOrSkips(t *testing.T) {
	// Cria 25 registros simulados
	allRows := make([]map[string]interface{}, 25)
	for i := 0; i < 25; i++ {
		allRows[i] = map[string]interface{}{
			"id":   i + 1,
			"nome": fmt.Sprintf("item_%d", i+1),
		}
	}
	cols := []string{"id", "nome"}

	// Página 1: limit 10, offset 0 (itens 1 a 10)
	p1 := PaginateResult(allRows, cols, 10, 0)
	if p1.ReturnedCount != 10 {
		t.Fatalf("Página 1: ReturnedCount esperado 10, obteve %d", p1.ReturnedCount)
	}
	if p1.TotalCount != 25 {
		t.Fatalf("Página 1: TotalCount esperado 25, obteve %d", p1.TotalCount)
	}
	if !p1.Truncated {
		t.Errorf("Página 1: Truncated esperado true, obteve false")
	}
	if p1.Rows[0]["id"] != 1 || p1.Rows[9]["id"] != 10 {
		t.Errorf("Página 1: faixa incorreta de IDs")
	}

	// Página 2: limit 10, offset 10 (itens 11 a 20)
	p2 := PaginateResult(allRows, cols, 10, 10)
	if p2.ReturnedCount != 10 {
		t.Fatalf("Página 2: ReturnedCount esperado 10, obteve %d", p2.ReturnedCount)
	}
	if !p2.Truncated {
		t.Errorf("Página 2: Truncated esperado true, obteve false")
	}
	if p2.Rows[0]["id"] != 11 || p2.Rows[9]["id"] != 20 {
		t.Errorf("Página 2: faixa incorreta de IDs")
	}

	// Verifica se NÃO repete nenhum ID entre página 1 e página 2
	seenP1 := make(map[int]bool)
	for _, r := range p1.Rows {
		seenP1[r["id"].(int)] = true
	}
	for _, r := range p2.Rows {
		id := r["id"].(int)
		if seenP1[id] {
			t.Errorf("Página 2 repetiu ID %d da Página 1!", id)
		}
	}

	// Página 3: limit 10, offset 20 (itens 21 a 25)
	p3 := PaginateResult(allRows, cols, 10, 20)
	if p3.ReturnedCount != 5 {
		t.Fatalf("Página 3: ReturnedCount esperado 5, obteve %d", p3.ReturnedCount)
	}
	if p3.Truncated {
		t.Errorf("Página 3 (última): Truncated esperado false, obteve true")
	}
	if p3.Rows[0]["id"] != 21 || p3.Rows[4]["id"] != 25 {
		t.Errorf("Página 3: faixa incorreta de IDs")
	}
}

func TestParsePostgresIndexDef(t *testing.T) {
	tests := []struct {
		name        string
		def         string
		nkeyatts    int
		natts       int
		expectedKey string
		expectedInc string
	}{
		{
			name:        "Índice simples btree",
			def:         "CREATE INDEX idx_users ON public.users USING btree (id, name DESC)",
			nkeyatts:    2,
			natts:       2,
			expectedKey: "id ASC, name DESC",
			expectedInc: "",
		},
		{
			name:        "Índice com INCLUDE e WHERE",
			def:         "CREATE UNIQUE INDEX idx_pedidos ON public.pedidos USING btree (cliente_id, data_pedido DESC) INCLUDE (valor, status) WHERE (ativo = true)",
			nkeyatts:    2,
			natts:       4,
			expectedKey: "cliente_id ASC, data_pedido DESC",
			expectedInc: "valor, status",
		},
		{
			name:        "Índice funcional",
			def:         "CREATE INDEX idx_lower_email ON public.users USING btree (lower((email)::text))",
			nkeyatts:    1,
			natts:       1,
			expectedKey: "lower((email)::text) ASC",
			expectedInc: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey, gotInc := parsePostgresIndexDef(tt.def, tt.nkeyatts, tt.natts)
			if gotKey != tt.expectedKey {
				t.Errorf("parsePostgresIndexDef key = %q, esperado %q", gotKey, tt.expectedKey)
			}
			if gotInc != tt.expectedInc {
				t.Errorf("parsePostgresIndexDef inc = %q, esperado %q", gotInc, tt.expectedInc)
			}
		})
	}
}

func TestSecurity_InjectionAttemptsBlocked(t *testing.T) {
	maliciousInputs := []string{
		"users'; DROP TABLE clientes; --",
		"users' OR 1=1 --",
		`users" OR "1"="1"`,
		"users` WHERE 1=1",
		"table/*inline comment*/",
		"table--comment",
		"table\x00inject",
	}

	exec := NewExecutor()

	for _, mal := range maliciousInputs {
		// list_constraints
		_, err := exec.ListConstraints(nil, "postgres", ListConstraintsOptions{
			TableName: mal,
		})
		if err == nil {
			t.Errorf("ListConstraints deveria falhar para input malicioso %q, mas passou", mal)
		}

		// list_indexes
		_, err = exec.ListIndexes(nil, "sqlserver", ListIndexesOptions{
			TableName: mal,
		})
		if err == nil {
			t.Errorf("ListIndexes deveria falhar para input malicioso %q, mas passou", mal)
		}

		// list_routines
		_, err = exec.ListRoutines(nil, "oracle", ListRoutinesOptions{
			NameLike: mal,
		})
		if err == nil {
			t.Errorf("ListRoutines deveria falhar para name_like malicioso %q, mas passou", mal)
		}

		// get_routine_source
		_, err = exec.GetRoutineSource(nil, "mysql", GetRoutineSourceOptions{
			RoutineName: mal,
		})
		if err == nil {
			t.Errorf("GetRoutineSource deveria falhar para rotina maliciosa %q, mas passou", mal)
		}
	}
}

func TestColumnsFormatUniformity(t *testing.T) {
	// Garante que as colunas exportadas em ConstraintColumns, IndexColumns e RoutineColumns
	// correspondem exatamente às especificações da tarefa
	expectedConstraintCols := []string{
		"owner", "table_name", "constraint_name", "constraint_type",
		"columns", "ref_owner", "ref_table", "ref_columns",
		"on_delete", "on_update", "status", "check_expression",
	}
	if len(ConstraintColumns) != len(expectedConstraintCols) {
		t.Fatalf("ConstraintColumns tamanho incorreto: %d vs %d", len(ConstraintColumns), len(expectedConstraintCols))
	}
	for i, c := range expectedConstraintCols {
		if ConstraintColumns[i] != c {
			t.Errorf("ConstraintColumns[%d] = %q, esperado %q", i, ConstraintColumns[i], c)
		}
	}

	expectedIndexCols := []string{
		"owner", "table_name", "index_name", "is_unique", "is_primary",
		"index_type", "columns", "included_columns", "predicate", "status",
	}
	if len(IndexColumns) != len(expectedIndexCols) {
		t.Fatalf("IndexColumns tamanho incorreto: %d vs %d", len(IndexColumns), len(expectedIndexCols))
	}
	for i, c := range expectedIndexCols {
		if IndexColumns[i] != c {
			t.Errorf("IndexColumns[%d] = %q, esperado %q", i, IndexColumns[i], c)
		}
	}

	expectedRoutineCols := []string{
		"owner", "routine_name", "routine_type", "language", "status",
		"created", "last_modified", "parent_table", "trigger_event", "trigger_timing",
	}
	if len(RoutineColumns) != len(expectedRoutineCols) {
		t.Fatalf("RoutineColumns tamanho incorreto: %d vs %d", len(RoutineColumns), len(expectedRoutineCols))
	}
	for i, c := range expectedRoutineCols {
		if RoutineColumns[i] != c {
			t.Errorf("RoutineColumns[%d] = %q, esperado %q", i, RoutineColumns[i], c)
		}
	}
}

func TestListRoutines_PackageOnlyInOracle(t *testing.T) {
	exec := NewExecutor()

	// Em Postgres, SQL Server e MySQL, PACKAGE deve retornar lista vazia sem erro
	for _, dbType := range []string{"postgres", "sqlserver", "mysql"} {
		res, err := exec.ListRoutines(nil, dbType, ListRoutinesOptions{
			RoutineType: "PACKAGE",
		})
		if err != nil {
			t.Errorf("Esperado nil erro para PACKAGE em %s, obteve %v", dbType, err)
		}
		if len(res) != 0 {
			t.Errorf("Esperado lista vazia para PACKAGE em %s, obteve %d", dbType, len(res))
		}
	}
}
