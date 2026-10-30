package db

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/rogick/db-explorer-mcp/pkg/tempfile"
)

type mockDriver struct{}

func (d *mockDriver) Open(name string) (driver.Conn, error) {
	if name == "lob_test" {
		return &mockLobConn{}, nil
	}
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

	// 150 linhas no banco, limit de 100, offset 0
	res, err := exec.ExecuteQuery(db, "SELECT * FROM test", 100, 0)
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
	if res.Offset != 0 {
		t.Errorf("Offset esperado 0, obteve %d", res.Offset)
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

	// 50 linhas no banco, limit de 500, offset 0
	res, err := exec.ExecuteQuery(db, "SELECT * FROM test", 500, 0)
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

func TestExecuteQuery_Pagination(t *testing.T) {
	db, err := sql.Open("mock_db", "normal") // 150 linhas
	if err != nil {
		t.Fatalf("Erro ao abrir mock_db: %v", err)
	}
	defer db.Close()

	exec := NewExecutor()

	// Página 2: limit 50, offset 50 (linhas 51 a 100)
	res, err := exec.ExecuteQuery(db, "SELECT * FROM test", 50, 50)
	if err != nil {
		t.Fatalf("Erro inesperado em ExecuteQuery: %v", err)
	}

	if len(res.Rows) != 50 {
		t.Errorf("Esperado 50 linhas retornadas, obteve %d", len(res.Rows))
	}
	if res.ReturnedCount != 50 {
		t.Errorf("ReturnedCount esperado 50, obteve %d", res.ReturnedCount)
	}
	if res.Offset != 50 {
		t.Errorf("Offset esperado 50, obteve %d", res.Offset)
	}
	if res.TotalCount != 150 {
		t.Errorf("TotalCount esperado 150, obteve %d", res.TotalCount)
	}
	if !res.Truncated {
		t.Errorf("Truncated esperado true (ainda há mais 50 linhas), obteve false")
	}
	// Verifica se a primeira linha retornada é a de id 51
	if res.Rows[0]["id"] != int64(51) {
		t.Errorf("Primeira linha esperada id=51, obteve %v", res.Rows[0]["id"])
	}

	// Última página: limit 50, offset 100 (linhas 101 a 150)
	resLast, err := exec.ExecuteQuery(db, "SELECT * FROM test", 50, 100)
	if err != nil {
		t.Fatalf("Erro inesperado em ExecuteQuery na última página: %v", err)
	}
	if len(resLast.Rows) != 50 {
		t.Errorf("Esperado 50 linhas na última página, obteve %d", len(resLast.Rows))
	}
	if resLast.Truncated {
		t.Errorf("Truncated esperado false na última página, obteve true")
	}
	if resLast.Rows[0]["id"] != int64(101) {
		t.Errorf("Primeira linha esperada id=101, obteve %v", resLast.Rows[0]["id"])
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
	res, err := exec.ExecuteQuery(db, "SELECT * FROM test", 0, 0)
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

type mockLobConn struct{}

func (c *mockLobConn) Prepare(query string) (driver.Stmt, error) {
	return nil, fmt.Errorf("prepare não implementado")
}
func (c *mockLobConn) Close() error              { return nil }
func (c *mockLobConn) Begin() (driver.Tx, error) { return nil, fmt.Errorf("tx não implementada") }
func (c *mockLobConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	return &mockLobRows{}, nil
}

type mockLobRows struct {
	current int
}

func (r *mockLobRows) Columns() []string {
	return []string{"id", "tipo_evento", "payload_xml", "clob_curto", "blob_binario"}
}

func (r *mockLobRows) ColumnTypeDatabaseTypeName(index int) string {
	switch index {
	case 0:
		return "NUMBER"
	case 1:
		return "VARCHAR2"
	case 2:
		return "VARCHAR2"
	case 3:
		return "CLOB"
	case 4:
		return "BLOB"
	}
	return "VARCHAR2"
}

func (r *mockLobRows) Close() error { return nil }

func (r *mockLobRows) Next(dest []driver.Value) error {
	if r.current >= 2 {
		return io.EOF
	}
	r.current++
	dest[0] = int64(1022 + r.current)
	dest[1] = "NFE_EMISSAO"
	dest[2] = fmt.Sprintf("<NFe><id>%d</id><dados>%s</dados></NFe>", r.current, strings.Repeat("X", 2500))
	dest[3] = `{"resumo": "campo CLOB mesmo pequeno"}`
	dest[4] = []byte{0x89, 0x50, 0x4E, 0x47, 0x00, 0x01, 0x02}
	return nil
}

func TestExecuteQueryWithOptions_ExtractLOBs(t *testing.T) {
	dbConn, err := sql.Open("mock_db", "lob_test")
	if err != nil {
		t.Fatalf("Erro ao abrir mock_db: %v", err)
	}
	defer dbConn.Close()

	tmpDir := t.TempDir()
	tempMgr := tempfile.NewManagerWithDir(tmpDir)
	exec := NewExecutor()

	res, err := exec.ExecuteQueryWithOptions(dbConn, "SELECT * FROM eventos", ExecuteQueryOptions{
		Limit:        100,
		Offset:       0,
		ExtractLOBs:  true,
		LOBThreshold: 2048,
		TempManager:  tempMgr,
	})
	if err != nil {
		t.Fatalf("Erro em ExecuteQueryWithOptions: %v", err)
	}

	if len(res.Rows) != 2 {
		t.Fatalf("Esperado 2 linhas, obteve %d", len(res.Rows))
	}

	// Coluna escalar comum deve permanecer intacta
	if res.Rows[0]["tipo_evento"] != "NFE_EMISSAO" {
		t.Errorf("Coluna tipo_evento foi alterada indevidamente: %v", res.Rows[0]["tipo_evento"])
	}

	// payload_xml (> 2048 bytes) deve ter sido extraído para arquivo .xml
	xmlRef, ok := res.Rows[0]["payload_xml"].(string)
	if !ok || !strings.HasPrefix(xmlRef, "[LOB salvo em: ") || !strings.Contains(xmlRef, ".xml") {
		t.Errorf("Referência LOB para payload_xml inesperada: %v", res.Rows[0]["payload_xml"])
	}

	// clob_curto (tipo CLOB dedicado) deve ter sido extraído para arquivo .json
	clobRef, ok := res.Rows[0]["clob_curto"].(string)
	if !ok || !strings.HasPrefix(clobRef, "[LOB salvo em: ") || !strings.Contains(clobRef, ".json") {
		t.Errorf("Referência LOB para clob_curto inesperada: %v", res.Rows[0]["clob_curto"])
	}

	// blob_binario (tipo BLOB / binário) deve ter sido extraído para arquivo .bin
	blobRef, ok := res.Rows[0]["blob_binario"].(string)
	if !ok || !strings.HasPrefix(blobRef, "[LOB salvo em: ") || !strings.Contains(blobRef, ".bin") {
		t.Errorf("Referência LOB para blob_binario inesperada: %v", res.Rows[0]["blob_binario"])
	}

	// Devem ter sido gerados 6 arquivos LOB (3 colunas x 2 linhas)
	if len(res.LobFiles) != 6 {
		t.Errorf("Esperado 6 arquivos LOB gerados, obteve %d", len(res.LobFiles))
	}
	for _, f := range res.LobFiles {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("Arquivo LOB %s não existe no disco: %v", f, err)
		}
	}
}
