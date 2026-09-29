package db

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"testing"
)

type mockDriver struct{}

func (d *mockDriver) Open(name string) (driver.Conn, error) {
	total := 150
	if name == "short" {
		total = 50
	}
	return &mockConn{totalRows: total}, nil
}

type mockConn struct {
	totalRows int
}

func (c *mockConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare não implementado")
}

func (c *mockConn) Close() error {
	return nil
}

func (c *mockConn) Begin() (driver.Tx, error) {
	return nil, fmt.Errorf("tx não implementada")
}

func (c *mockConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	return &mockRows{total: c.totalRows}, nil
}

type mockRows struct {
	current int
	total   int
}

func (r *mockRows) Columns() []string {
	return []string{"id", "val"}
}

func (r *mockRows) Close() error {
	return nil
}

func (r *mockRows) Next(dest []driver.Value) error {
	if r.current >= r.total {
		return io.EOF
	}
	r.current++
	dest[0] = int64(r.current)
	dest[1] = fmt.Sprintf("linha_%d", r.current)
	return nil
}

func init() {
	sql.Register("mock_db", &mockDriver{})
}

func TestExecuteQuery_Truncated(t *testing.T) {
	db, err := sql.Open("mock_db", "normal")
	if err != nil {
		t.Fatalf("Erro ao abrir mock_db: %v", err)
	}
	defer db.Close()

	exec := NewExecutor()

	// 150 linhas no banco, limit de 100
	res, err := exec.ExecuteQuery(db, "SELECT * FROM test", 100)
	if err != nil {
		t.Fatalf("Erro inesperado em ExecuteQuery: %v", err)
	}

	if len(res.Rows) != 100 {
		t.Errorf("Esperado 100 linhas retornadas, obteve %d", len(res.Rows))
	}
	if res.ReturnedCount != 100 {
		t.Errorf("ReturnedCount esperado 100, obteve %d", res.ReturnedCount)
	}
	if res.TotalCount != 150 {
		t.Errorf("TotalCount esperado 150, obteve %d", res.TotalCount)
	}
	if !res.Truncated {
		t.Errorf("Truncated esperado true, obteve false")
	}
	if res.ExceededCeiling {
		t.Errorf("ExceededCeiling esperado false, obteve true")
	}
}

func TestExecuteQuery_NotTruncated(t *testing.T) {
	db, err := sql.Open("mock_db", "short") // 50 linhas
	if err != nil {
		t.Fatalf("Erro ao abrir mock_db: %v", err)
	}
	defer db.Close()

	exec := NewExecutor()

	// 50 linhas no banco, limit de 500
	res, err := exec.ExecuteQuery(db, "SELECT * FROM test", 500)
	if err != nil {
		t.Fatalf("Erro inesperado em ExecuteQuery: %v", err)
	}

	if len(res.Rows) != 50 {
		t.Errorf("Esperado 50 linhas retornadas, obteve %d", len(res.Rows))
	}
	if res.ReturnedCount != 50 {
		t.Errorf("ReturnedCount esperado 50, obteve %d", res.ReturnedCount)
	}
	if res.TotalCount != 50 {
		t.Errorf("TotalCount esperado 50, obteve %d", res.TotalCount)
	}
	if res.Truncated {
		t.Errorf("Truncated esperado false, obteve true")
	}
}

func TestExecuteQuery_ZeroLimit(t *testing.T) {
	db, err := sql.Open("mock_db", "normal") // 150 linhas
	if err != nil {
		t.Fatalf("Erro ao abrir mock_db: %v", err)
	}
	defer db.Close()

	exec := NewExecutor()

	// limit 0 significa sem limite (até o teto de segurança)
	res, err := exec.ExecuteQuery(db, "SELECT * FROM test", 0)
	if err != nil {
		t.Fatalf("Erro inesperado em ExecuteQuery: %v", err)
	}

	if len(res.Rows) != 150 {
		t.Errorf("Esperado todas as 150 linhas retornadas, obteve %d", len(res.Rows))
	}
	if res.TotalCount != 150 {
		t.Errorf("TotalCount esperado 150, obteve %d", res.TotalCount)
	}
	if res.Truncated {
		t.Errorf("Truncated esperado false, obteve true")
	}
}
